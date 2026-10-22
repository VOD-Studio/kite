package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// 默认只监听本机回环:服务面向外部项目调用,默认值必须是安全的那一档。
func TestDefault_ServerBindsLoopback(t *testing.T) {
	cfg := Default()
	require.Equal(t, "127.0.0.1:3722", cfg.Server.GRPCAddr)
	require.Equal(t, "127.0.0.1:3721", cfg.Server.HTTPAddr)
	require.Empty(t, cfg.Server.AuthToken)
}

func TestLoad_ServerEnv(t *testing.T) {
	t.Setenv("KITE_SERVER_GRPC_ADDR", "0.0.0.0:9000")
	t.Setenv("KITE_SERVER_HTTP_ADDR", "0.0.0.0:9001")
	t.Setenv("KITE_SERVER_AUTH_TOKEN", "secret")
	t.Setenv("KITE_SERVER_AUTH_TOKEN_FILE", "/run/secrets/kite")
	t.Setenv("KITE_SERVER_RATE_PER_MINUTE", "600")
	t.Setenv("KITE_SERVER_TLS_CERT", "/certs/kite.pem")
	t.Setenv("KITE_SERVER_TLS_KEY", "/certs/kite.key")

	cfg := Load()
	require.Equal(t, "0.0.0.0:9000", cfg.Server.GRPCAddr)
	require.Equal(t, "0.0.0.0:9001", cfg.Server.HTTPAddr)
	require.Equal(t, "secret", cfg.Server.AuthToken)
	require.Equal(t, "/run/secrets/kite", cfg.Server.AuthTokenFile)
	require.Equal(t, 600, cfg.Server.RatePerMinute)
	require.Equal(t, "/certs/kite.pem", cfg.Server.TLSCertFile)
	require.Equal(t, "/certs/kite.key", cfg.Server.TLSKeyFile)
}

// 不配置时限流关闭、无 TLS:与旧版部署行为一致。
func TestDefault_ServerNoTLSNoRateLimit(t *testing.T) {
	cfg := Default()
	require.Zero(t, cfg.Server.RatePerMinute)
	require.Empty(t, cfg.Server.TLSCertFile)
	require.Empty(t, cfg.Server.TLSKeyFile)
	require.Empty(t, cfg.Server.AuthTokensFile)
}

// 启动日志里打印的配置摘要不得带 token。
func TestConfigString_NoSecret(t *testing.T) {
	cfg := Default()
	cfg.Server.AuthToken = "super-secret"
	cfg.Redis.Password = "redis-secret"
	require.NotContains(t, cfg.String(), "super-secret")
	require.NotContains(t, cfg.String(), "redis-secret")
}

func TestResolveAuthTokens(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
		return p
	}

	tests := []struct {
		name    string
		cfg     ServerConfig
		want    []AuthToken
		wantErr string
	}{
		{name: "都没配 → 不启用", cfg: ServerConfig{}, want: nil},
		{name: "直接给 token", cfg: ServerConfig{AuthToken: "abc"}, want: []AuthToken{{Token: "abc"}}},
		{name: "从文件读并去掉首尾空白", cfg: ServerConfig{AuthTokenFile: write("ok", "  abc\n")}, want: []AuthToken{{Token: "abc"}}},
		{name: "两者都配 → 歧义", cfg: ServerConfig{AuthToken: "a", AuthTokenFile: write("both", "b")}, wantErr: "只能设置一个"},
		{name: "单令牌与多令牌文件同配 → 歧义", cfg: ServerConfig{AuthToken: "a", AuthTokensFile: write("multi", "x:b")}, wantErr: "只能设置一个"},
		{name: "文件不存在 → 报错而不是裸奔", cfg: ServerConfig{AuthTokenFile: filepath.Join(dir, "missing")}, wantErr: "读取 token 文件"},
		{name: "文件为空 → 报错而不是裸奔", cfg: ServerConfig{AuthTokenFile: write("empty", " \n")}, wantErr: "为空"},
		{
			name: "多令牌文件:每行「名称:令牌」,忽略空行与注释",
			cfg:  ServerConfig{AuthTokensFile: write("tokens", "# 调用方令牌\nblog:t-blog\n\n  bot : t-bot \n")},
			want: []AuthToken{{Name: "blog", Token: "t-blog"}, {Name: "bot", Token: "t-bot"}},
		},
		{name: "多令牌文件不存在", cfg: ServerConfig{AuthTokensFile: filepath.Join(dir, "missing2")}, wantErr: "读取 tokens 文件"},
		{name: "多令牌文件为空 → 报错而不是裸奔", cfg: ServerConfig{AuthTokensFile: write("blank", "# 只有注释\n")}, wantErr: "为空"},
		{name: "行内没有分隔符", cfg: ServerConfig{AuthTokensFile: write("nosep", "blog")}, wantErr: "分隔符"},
		{name: "名称为空", cfg: ServerConfig{AuthTokensFile: write("noname", ":t")}, wantErr: "名称为空"},
		{name: "令牌为空", cfg: ServerConfig{AuthTokensFile: write("notoken", "blog:")}, wantErr: "令牌为空"},
		{name: "名称重复会让限流与吊销产生歧义", cfg: ServerConfig{AuthTokensFile: write("dupname", "a:t1\na:t2")}, wantErr: "重复"},
		{name: "令牌重复发给两个调用方", cfg: ServerConfig{AuthTokensFile: write("duptoken", "a:t1\nb:t1")}, wantErr: "重复"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.cfg.ResolveAuthTokens()
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
