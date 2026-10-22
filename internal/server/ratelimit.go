// Package server 的按调用方限流。
//
// 服务端持有上游网易云的共享登录态,一个失控的调用方会把整个 session 池打进熔断 / 封禁。
// 限流以「通过鉴权后的调用方身份」(令牌名称)为单位,保护上游账号而不是单个请求的公平性;
// 这与公开模式按来源 IP 限流(internal/public/limit.go)是两个平行的适配器,
// 分层规则禁止 server 与 public 互相依赖,因此各自持有实现。
package server

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

// rateLimiter 是按调用方名称计的令牌桶:容量 = 每分钟额度,匀速回填。
// 未启用(未配置额度)时拦截器放行一切。now 可注入,测试用假时钟保证确定性。
type rateLimiter struct {
	mu        sync.Mutex
	perSecond float64
	burst     float64
	buckets   map[string]*rateBucket
	lastSweep time.Time
	now       func() time.Time
}

type rateBucket struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(perMinute int, now func() time.Time) *rateLimiter {
	return &rateLimiter{
		perSecond: float64(perMinute) / 60,
		burst:     float64(perMinute),
		buckets:   map[string]*rateBucket{},
		now:       now,
	}
}

// allow 消耗调用方的一个令牌,没有可用令牌时返回 false。
func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)

	b, ok := l.buckets[key]
	if !ok {
		b = &rateBucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens += now.Sub(b.last).Seconds() * l.perSecond
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// sweep 每分钟清掉已经回满的桶,避免长时间运行后 map 只增不减。
func (l *rateLimiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < time.Minute {
		return
	}
	l.lastSweep = now
	full := time.Duration(l.burst/l.perSecond) * time.Second
	for k, b := range l.buckets {
		if now.Sub(b.last) > full {
			delete(l.buckets, k)
		}
	}
}

// UnaryInterceptor 在鉴权之后按调用方限流。l 为 nil(未配置额度)时放行。
// 健康检查豁免:编排探活频繁且不带业务语义,被限流会让 LB 误摘流。
func (l *rateLimiter) UnaryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if err := l.check(ctx, info.FullMethod); err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

// StreamInterceptor 对流式 RPC(目前是 reflection)执行同样的限流。
func (l *rateLimiter) StreamInterceptor(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if err := l.check(ss.Context(), info.FullMethod); err != nil {
		return err
	}
	return handler(srv, ss)
}

func (l *rateLimiter) check(ctx context.Context, fullMethod string) error {
	if l == nil || strings.HasPrefix(fullMethod, healthMethodPrefix) {
		return nil
	}
	caller := CallerFromContext(ctx)
	if l.allow(caller) {
		return nil
	}
	slog.WarnContext(ctx, "server rate limited",
		slog.String("caller", caller), slog.String("method", fullMethod))
	return statusError(codes.ResourceExhausted, ReasonServerRateLimited,
		"调用过于频繁,已超出该令牌的限额,稍后再试", nil)
}
