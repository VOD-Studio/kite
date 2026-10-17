package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	merrors "github.com/VOD-Studio/kite/errors"
	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/cache"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// 回归:cmd/server 的装配是 engine.New(WithCache(Noop)) 且不注入 session 池。
// 需要登录态的 RPC(GetDailyRecommend / GetPersonalFM / LikedList)经 engine.Execute 执行时,
// 曾因 engine 只认 session 池而忽略 interceptor 注入 ctx 的 cookie,恒返回「登录态失效」,
// 而服务层完全没有测试覆盖。
func TestLoggedInRPCs_UseCookieFromContext(t *testing.T) {
	var calls atomic.Int32
	var lastCookie atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		lastCookie.Store(r.Header.Get("Cookie"))
		w.Header().Set("Content-Type", "application/json")
		// 一个响应同时满足三个接口的最小映射:data.dailySongs / data(FM 数组)形状不同,
		// 这里只关心「是否带着 cookie 到达上游并成功映射」,各接口分别给形状。
		switch r.URL.Path {
		case "/weapi/v3/discovery/recommend/songs":
			_, _ = w.Write([]byte(`{"code":200,"data":{"dailySongs":[]}}`))
		case "/weapi/v1/radio/get":
			_, _ = w.Write([]byte(`{"code":200,"data":[]}`))
		default: // eapi 喜欢列表
			_, _ = w.Write([]byte(`{"code":200,"ids":[7,8]}`))
		}
	}))
	t.Cleanup(srv.Close)

	eng := engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(srv.URL))
	authed := engine.WithCookie(context.Background(), "MUSIC_U=abc")
	anon := context.Background() // 拦截器在无 metadata 时注入空串,等价于没有 cookie

	cases := []struct {
		name string
		call func(ctx context.Context) error
	}{
		{"GetDailyRecommend", func(ctx context.Context) error {
			_, err := NewRecommendServer(eng).GetDailyRecommend(ctx, &mmpb.GetDailyRecommendRequest{})
			return err
		}},
		{"GetPersonalFM", func(ctx context.Context) error {
			_, err := NewFMServer(eng).GetPersonalFM(ctx, &mmpb.GetPersonalFMRequest{})
			return err
		}},
		{"LikedList", func(ctx context.Context) error {
			_, err := NewSongServer(eng).LikedList(ctx, &mmpb.LikedListRequest{UserId: 1})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := calls.Load()
			require.NoError(t, tc.call(authed), "带 cookie 的请求应成功")
			require.Equal(t, before+1, calls.Load())
			require.Contains(t, lastCookie.Load(), "MUSIC_U=abc", "cookie 应发往上游")

			before = calls.Load()
			err := tc.call(anon)
			require.ErrorIs(t, err, merrors.ErrUnauthorized, "没有 cookie 应明确未授权")
			require.Equal(t, before, calls.Load(), "没有凭据时不应请求上游")
		})
	}
}
