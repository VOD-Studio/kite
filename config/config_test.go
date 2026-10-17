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

	cfg := Load()
	require.Equal(t, "0.0.0.0:9000", cfg.Server.GRPCAddr)
	require.Equal(t, "0.0.0.0:9001", cfg.Server.HTTPAddr)
	require.Equal(t, "secret", cfg.Server.AuthToken)
	require.Equal(t, "/run/secrets/kite", cfg.Server.AuthTokenFile)
}

// 启动日志里打印的配置摘要不得带 token。
func TestConfigString_NoSecret(t *testing.T) {
	cfg := Default()
	cfg.Server.AuthToken = "super-secret"
	cfg.Redis.Password = "redis-secret"
	require.NotContains(t, cfg.String(), "super-secret")
	require.NotContains(t, cfg.String(), "redis-secret")
}

func TestResolveAuthToken(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
		return p
	}

	tests := []struct {
		name    string
		cfg     ServerConfig
		want    string
		wantErr string
	}{
		{name: "都没配 → 不启用", cfg: ServerConfig{}, want: ""},
		{name: "直接给 token", cfg: ServerConfig{AuthToken: "abc"}, want: "abc"},
		{name: "从文件读并去掉首尾空白", cfg: ServerConfig{AuthTokenFile: write("ok", "  abc\n")}, want: "abc"},
		{name: "两者都配 → 歧义", cfg: ServerConfig{AuthToken: "a", AuthTokenFile: write("both", "b")}, wantErr: "只能设置一个"},
		{name: "文件不存在 → 报错而不是裸奔", cfg: ServerConfig{AuthTokenFile: filepath.Join(dir, "missing")}, wantErr: "读取 token 文件"},
		{name: "文件为空 → 报错而不是裸奔", cfg: ServerConfig{AuthTokenFile: write("empty", " \n")}, wantErr: "为空"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.cfg.ResolveAuthToken()
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
