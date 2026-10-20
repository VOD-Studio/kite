package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/VOD-Studio/kite/internal/cache"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/session"
	"github.com/VOD-Studio/kite/internal/server"
)

func TestProbeAuthenticationContract(t *testing.T) {
	for _, tc := range []struct {
		name        string
		serverToken string
		probeToken  string
		wantError   string
	}{
		{"有效令牌贯通两种协议且不访问上游", "correct", "correct", ""},
		{"错误令牌不能把健康端点当成部署成功", "correct", "wrong", "LOGIN_REQUIRED"},
		{"意外关闭鉴权必须拒绝发布", "", "correct", "SERVER_AUTH_FAILED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var upstreamCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				upstreamCalls.Add(1)
				w.WriteHeader(http.StatusBadGateway)
			}))
			t.Cleanup(upstream.Close)
			eng := engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(upstream.URL))
			app, err := server.NewApp(server.Options{
				GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0", AuthToken: tc.serverToken,
			}, eng, session.NoopStore{})
			require.NoError(t, err)
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				require.NoError(t, app.Shutdown(ctx))
			})

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err = probe(ctx, "http://"+app.HTTPAddr(), app.GRPCAddr(), tc.probeToken)
			if tc.wantError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantError)
			}
			require.Zero(t, upstreamCalls.Load(), "部署检查不得依赖网易云是否在线")
		})
	}
}
