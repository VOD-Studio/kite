package server

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeClock 是可手动推进的时钟,保证限流测试确定性(不睡眠)。
type fakeClock struct{ now time.Time }

func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func newFakeRateLimiter(perMinute int) (*rateLimiter, *fakeClock) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	return newRateLimiter(perMinute, func() time.Time { return clock.now }), clock
}

func TestRateLimiter_Allow(t *testing.T) {
	l, clock := newFakeRateLimiter(2) // 容量 2,每 30 秒回填 1 个

	require.True(t, l.allow("blog"))
	require.True(t, l.allow("blog"))
	require.False(t, l.allow("blog"), "超出额度被拒")

	// 额度按调用方隔离:blog 打满不影响 bot。
	require.True(t, l.allow("bot"))
	require.True(t, l.allow("bot"))
	require.False(t, l.allow("bot"))

	clock.Advance(30 * time.Second)
	require.True(t, l.allow("blog"), "30 秒后回填 1 个额度")
	require.False(t, l.allow("blog"), "回填速度不能快过配置")

	clock.Advance(10 * time.Minute)
	require.True(t, l.allow("blog"), "长时间静默后额度回满")
	require.True(t, l.allow("blog"))
	require.False(t, l.allow("blog"))
}

// 静默超过回满时间的桶会被清理,长时间运行 map 不只增不减。
func TestRateLimiter_Sweep(t *testing.T) {
	l, clock := newFakeRateLimiter(60) // 回满需 1 分钟
	require.True(t, l.allow("gone"))
	require.Len(t, l.buckets, 1)

	clock.Advance(2 * time.Minute)
	require.True(t, l.allow("other"))
	require.Len(t, l.buckets, 1, "已回满的旧桶应被清理")
}

func TestRateLimiter_UnaryInterceptor(t *testing.T) {
	const method = "/netease.music.v1.SongService/GetSongDetail"
	called := false
	handler := func(context.Context, any) (any, error) { called = true; return "ok", nil }
	info := &grpc.UnaryServerInfo{FullMethod: method}
	withCaller := func(name string) context.Context {
		return context.WithValue(context.Background(), callerCtxKey{}, name)
	}

	t.Run("未配置额度(nil)→ 放行", func(t *testing.T) {
		var l *rateLimiter
		resp, err := l.UnaryInterceptor(context.Background(), nil, info, handler)
		require.NoError(t, err)
		require.Equal(t, "ok", resp)
		require.True(t, called)
	})

	l, _ := newFakeRateLimiter(2)
	t.Run("额度内放行(打满额度)", func(t *testing.T) {
		for range 2 {
			_, err := l.UnaryInterceptor(withCaller("blog"), nil, info, handler)
			require.NoError(t, err)
		}
	})

	t.Run("超出 → RESOURCE_EXHAUSTED,handler 不执行", func(t *testing.T) {
		called = false
		_, err := l.UnaryInterceptor(withCaller("blog"), nil, info, handler)
		st, _ := status.FromError(err)
		require.Equal(t, codes.ResourceExhausted, st.Code())
		require.Equal(t, ReasonServerRateLimited, errorInfoOf(t, err).GetReason())
		require.Equal(t, ErrorDomain, errorInfoOf(t, err).GetDomain())
		require.False(t, called, "被限流的请求不得走到业务代码")
	})

	t.Run("额度按调用方隔离", func(t *testing.T) {
		_, err := l.UnaryInterceptor(withCaller("bot"), nil, info, handler)
		require.NoError(t, err)
	})

	t.Run("健康检查豁免", func(t *testing.T) {
		healthInfo := &grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}
		_, err := l.UnaryInterceptor(withCaller("blog"), nil, healthInfo, handler)
		require.NoError(t, err, "已打满的调用方探活不应被限流")
	})
}

func TestRateLimiter_StreamInterceptor(t *testing.T) {
	const method = "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo"
	l, _ := newFakeRateLimiter(1)
	info := &grpc.StreamServerInfo{FullMethod: method}
	called := false
	handler := func(any, grpc.ServerStream) error { called = true; return nil }

	require.NoError(t, l.StreamInterceptor(nil, fakeStream{ctx: context.Background()}, info, handler))
	require.True(t, called)

	called = false
	err := l.StreamInterceptor(nil, fakeStream{ctx: context.Background()}, info, handler)
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
	require.False(t, called)
}
