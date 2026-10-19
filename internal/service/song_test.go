package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/cache"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// 回归:GetSongURL 曾声明为匿名接口,服务端会丢掉调用方经 metadata 传来的 cookie,
// 已登录的 VIP 账号也只能拿到匿名结果(VIP 歌 url 为空)。
// 现在是可选登录:带 cookie 就发往上游,不带照常按匿名请求。
func TestGetSongURL_CookieOptional(t *testing.T) {
	var calls atomic.Int32
	var lastCookie atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		lastCookie.Store(r.Header.Get("Cookie"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"data":[{"id":1,"url":"https://m801.music.126.net/x.mp3","br":320000,"size":1,"type":"mp3"}]}`))
	}))
	t.Cleanup(srv.Close)

	eng := engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(srv.URL))
	songs := NewSongServer(eng)
	req := &mmpb.GetSongURLRequest{SongId: 1}

	resp, err := songs.GetSongURL(engine.WithCookie(context.Background(), "MUSIC_U=vip"), req)
	require.NoError(t, err)
	require.Equal(t, "https://m801.music.126.net/x.mp3", resp.GetUrl().GetUrl())
	require.Contains(t, lastCookie.Load(), "MUSIC_U=vip", "带 cookie 时应按该账号请求上游")

	_, err = songs.GetSongURL(context.Background(), req)
	require.NoError(t, err, "没有 cookie 不应报未授权")
	require.Equal(t, int32(2), calls.Load())
	require.NotContains(t, lastCookie.Load(), "MUSIC_U", "没有 cookie 时按匿名请求")
}
