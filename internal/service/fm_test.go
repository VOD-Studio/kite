package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// memCache 是并发安全的内存缓存(与真实共享缓存语义一致:写入后可被后续读取命中)。
type memCache struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (c *memCache) Get(_ context.Context, k string) ([]byte, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[k]
	return v, ok, nil
}

func (c *memCache) Set(_ context.Context, k string, v []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string][]byte{}
	}
	c.m[k] = v
	return nil
}

func (c *memCache) Delete(_ context.Context, k string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, k)
	return nil
}

// 私人 FM 每次请求上游都会返回新的一批歌曲,客户端靠重复调用"取下一批";
// 即使 engine 挂了共享缓存,同一用户连续调用也必须每次都到达上游。
func TestGetPersonalFM_NotCached(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"data":[]}`))
	}))
	t.Cleanup(srv.Close)

	eng := engine.New(engine.WithCache(&memCache{}), engine.WithBaseURL(srv.URL))
	ctx := engine.WithCookie(context.Background(), "MUSIC_U=abc")
	s := NewFMServer(eng)

	for i := 0; i < 3; i++ {
		_, err := s.GetPersonalFM(ctx, &mmpb.GetPersonalFMRequest{})
		require.NoError(t, err)
	}
	require.Equal(t, int32(3), calls.Load(), "每次调用都应到达上游以取得新的一批")
}
