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

// RefreshLogin 带着调用方的 cookie 请求上游,把上游经 Set-Cookie 下发的新值合并进原 cookie 返回;
// cookie 失效(上游 code=301)时返回未授权,调用方据此引导重新登录。
func TestRefreshLogin(t *testing.T) {
	var lastCookie atomic.Value
	var expired atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/weapi/login/token/refresh", r.URL.Path)
		lastCookie.Store(r.Header.Get("Cookie"))
		w.Header().Set("Content-Type", "application/json")
		if expired.Load() {
			_, _ = w.Write([]byte(`{"code":301}`))
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "MUSIC_U", Value: "renewed", Path: "/"})
		_, _ = w.Write([]byte(`{"code":200}`))
	}))
	t.Cleanup(srv.Close)

	eng := engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(srv.URL))
	auth := NewAuthServer(eng, nil)
	ctx := engine.WithCookie(context.Background(), "MUSIC_U=old; __csrf=c")

	resp, err := auth.RefreshLogin(ctx, &mmpb.RefreshLoginRequest{})
	require.NoError(t, err)
	require.Contains(t, lastCookie.Load(), "MUSIC_U=old", "应带着调用方的 cookie 请求上游")
	require.Equal(t, "MUSIC_U=renewed; __csrf=c", resp.GetCookie(), "新值替换旧值,其余项保留")

	expired.Store(true)
	_, err = auth.RefreshLogin(ctx, &mmpb.RefreshLoginRequest{})
	require.ErrorIs(t, err, merrors.ErrUnauthorized)
}
