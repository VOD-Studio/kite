// Package kit 的代理解析与校验测试(PRD-0018)。
package localstate

import (
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
