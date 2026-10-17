package kit

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/VOD-Studio/kite/internal/localstate"
)

// TestUseProxy_DualPathInjection 守护双路径不变式(PRD-0018):
// UseProxy 后 engine 走代理,HTTPClient(下载路径)也带同一代理决策。
func TestUseProxy_DualPathInjection(t *testing.T) {
	t.Parallel()

	u, err := localstate.ParseProxyURL("http://127.0.0.1:7890")
	require.NoError(t, err)

	k := New()
	k.UseProxy(u, "flag")

	require.Equal(t, u, k.ProxyURL)
	require.Equal(t, "flag", k.ProxySource)
	// 下载路径:client 挂了代理 transport。
	tr, ok := k.HTTPClient().Transport.(*http.Transport)
	require.True(t, ok, "HTTPClient 应挂代理 transport")
	require.NotNil(t, tr.Proxy)
	// 未注入时退回 DefaultClient(环境变量层默认)。
	k2 := New()
	require.Equal(t, http.DefaultClient, k2.HTTPClient())
}
