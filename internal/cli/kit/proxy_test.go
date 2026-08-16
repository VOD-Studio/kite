// Package kit 的代理解析与校验测试(PRD-0018)。
package kit

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseProxyURL_Valid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw       string
		wantProxy bool
	}{
		{"http://127.0.0.1:7890", true},
		{"https://proxy.corp.example:8443", true},
		{"socks5://127.0.0.1:7891", true},
		{"http://user:pass@127.0.0.1:7890", true}, // 认证信息由 URL 语法天然支持
		{"http://proxy.lan", true},                // 端口可缺省
		{"", false},                               // 空 = 未设置(回落环境变量)
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			t.Parallel()
			u, err := ParseProxyURL(tc.raw)
			require.NoError(t, err)
			if tc.wantProxy {
				require.NotNil(t, u)
				require.Equal(t, tc.raw, u.String())
			} else {
				require.Nil(t, u, "空串应解析为 nil(未设置)")
			}
		})
	}
}

// TestParseProxyURL_Rejects scheme 必须显式且在白名单内,host 非空(PRD-0018)。
func TestParseProxyURL_Rejects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw     string
		wantSub string // 错误应包含的引导线索
	}{
		// 无 scheme:url.Parse 会把 127.0.0.1 当非法 scheme 报错,提示补前缀。
		{"127.0.0.1:7890", "http://"},
		{"://missing-scheme", ""},
		{"http://", "host"},        // scheme 对但 host 空
		{"ftp://127.0.0.1:21", ""}, // scheme 白名单外
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			t.Parallel()
			_, err := ParseProxyURL(tc.raw)
			require.Error(t, err)
			if tc.wantSub != "" {
				require.Contains(t, err.Error(), tc.wantSub)
			}
		})
	}
}

// TestUseProxy_DualPathInjection 守护双路径不变式(PRD-0018):
// UseProxy 后 engine 走代理,HTTPClient(下载路径)也带同一代理决策。
func TestUseProxy_DualPathInjection(t *testing.T) {
	t.Parallel()

	u, err := ParseProxyURL("http://127.0.0.1:7890")
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
