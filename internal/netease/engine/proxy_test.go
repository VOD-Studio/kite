// Package engine 的代理注入测试(PRD-0018)。
//
// 行为测试:httptest server 假扮 HTTP 代理,断言 engine 的请求以绝对 URI
// 形式抵达代理(HTTP 代理协议的正确形态),证明流量真的经代理转发。
package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWithProxyURL_RoutesViaProxy(t *testing.T) {
	t.Parallel()

	// 假代理:记录收到的请求 URI;对代理请求直接回 JSON(不真转发,
	// transport 视代理的响应为最终响应,足够证明路由)。
	var gotURI atomic.Value
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURI.Store(r.RequestURI)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200}`))
	}))
	defer proxy.Close()

	e := New(
		WithBaseURL("http://target.example"),
		WithProxyURL(mustParseURL(t, proxy.URL)),
	)

	meta := Meta{Path: "/api/test", Method: http.MethodGet, Crypto: CryptoNone}
	raw, err := e.RawDo(context.Background(), meta, nil)
	require.NoError(t, err)
	require.Contains(t, string(raw), "200")

	uri, _ := gotURI.Load().(string)
	require.Equal(t, "http://target.example/api/test", uri,
		"请求应以绝对 URI 形式抵达代理(HTTP 代理协议形态)")
}

// TestWithProxyURL_NilKeepsDefault nil 代理不注入自定义 transport
// (保留 ProxyFromEnvironment 环境变量层默认行为)。
func TestWithProxyURL_NilKeepsDefault(t *testing.T) {
	t.Parallel()

	e := New(WithProxyURL(nil))
	require.Nil(t, e.transport.client.Transport, "nil 代理应保持默认 transport 语义")
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}
