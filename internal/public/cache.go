package public

import (
	"sync"
	"time"
)

// ttlCache 是进程内的过期缓存。公开模式面向所有听众,同一歌单 / 歌词会被反复请求,
// 缓存既减轻上游压力,也降低站点出口 IP 被上游风控的概率。
type ttlCache[V any] struct {
	mu    sync.Mutex
	items map[string]ttlEntry[V]
	max   int
	now   func() time.Time
}

type ttlEntry[V any] struct {
	value   V
	expires time.Time
}

func newTTLCache[V any](maxEntries int, now func() time.Time) *ttlCache[V] {
	return &ttlCache[V]{items: map[string]ttlEntry[V]{}, max: maxEntries, now: now}
}

func (c *ttlCache[V]) get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[key]
	if !ok || c.now().After(e.expires) {
		var zero V
		return zero, false
	}
	return e.value, true
}

func (c *ttlCache[V]) set(key string, v V, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if len(c.items) >= c.max {
		for k, e := range c.items {
			if now.After(e.expires) {
				delete(c.items, k)
			}
		}
		// 仍然满:随便丢一个(map 遍历顺序随机),容量上限比命中率重要。
		for k := range c.items {
			if len(c.items) < c.max {
				break
			}
			delete(c.items, k)
		}
	}
	c.items[key] = ttlEntry[V]{value: v, expires: now.Add(ttl)}
}
