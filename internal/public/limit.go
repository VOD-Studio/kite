package public

import (
	"sync"
	"time"
)

// limiter 是按 key(来源 IP)计的令牌桶:容量 = 每分钟额度,匀速回填。
// 公开模式没有令牌,限流是挡住脚本批量刷接口的主要手段(Origin 白名单只防别的网页盗用)。
type limiter struct {
	mu        sync.Mutex
	perSecond float64
	burst     float64
	buckets   map[string]*bucket
	lastSweep time.Time
	now       func() time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newLimiter(perMinute int, now func() time.Time) *limiter {
	return &limiter{
		perSecond: float64(perMinute) / 60,
		burst:     float64(perMinute),
		buckets:   map[string]*bucket{},
		now:       now,
	}
}

// allow 消耗 key 的一个令牌,没有可用令牌时返回 false。
func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
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
func (l *limiter) sweep(now time.Time) {
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
