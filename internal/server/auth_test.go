package server

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func ctxWithAuth(values ...string) context.Context {
	md := metadata.MD{}
	for _, v := range values {
		md.Append("authorization", v)
	}
	return metadata.NewIncomingContext(context.Background(), md)
}

// single 等价于旧的单令牌配置:一个未命名的调用方。
func single(token string) []AuthToken { return []AuthToken{{Value: token}} }

func TestAuth_Authorize(t *testing.T) {
	const method = "/netease.music.v1.SongService/GetSongDetail"
	tests := []struct {
		name   string
		auth   *Auth
		ctx    context.Context
		method string
		ok     bool
	}{
		{"未启用 → 放行", NewAuth(nil), context.Background(), method, true},
		{"nil 鉴权器 → 放行", nil, context.Background(), method, true},
		{"令牌正确", NewAuth(single("s3cret")), ctxWithAuth("Bearer s3cret"), method, true},
		{"scheme 大小写不敏感", NewAuth(single("s3cret")), ctxWithAuth("bearer s3cret"), method, true},
		{"多个值里有一个正确", NewAuth(single("s3cret")), ctxWithAuth("Bearer nope", "Bearer s3cret"), method, true},
		{"没有 metadata", NewAuth(single("s3cret")), context.Background(), method, false},
		{"没有 authorization", NewAuth(single("s3cret")), metadata.NewIncomingContext(context.Background(), metadata.Pairs("x", "y")), method, false},
		{"令牌错误", NewAuth(single("s3cret")), ctxWithAuth("Bearer s3cre"), method, false},
		{"令牌多一位", NewAuth(single("s3cret")), ctxWithAuth("Bearer s3crett"), method, false},
		{"scheme 不是 Bearer", NewAuth(single("s3cret")), ctxWithAuth("Basic s3cret"), method, false},
		{"裸令牌无 scheme", NewAuth(single("s3cret")), ctxWithAuth("s3cret"), method, false},
		{"只有 scheme", NewAuth(single("s3cret")), ctxWithAuth("Bearer"), method, false},
		{"健康检查豁免", NewAuth(single("s3cret")), context.Background(), "/grpc.health.v1.Health/Check", true},
		{"健康检查 Watch 豁免", NewAuth(single("s3cret")), context.Background(), "/grpc.health.v1.Health/Watch", true},
		// 前缀匹配必须带斜杠边界,不能让「名字像 Health」的业务 service 绕过。
		{"名字相近的 service 不豁免", NewAuth(single("s3cret")), context.Background(), "/grpc.health.v1.HealthEvil/Check", false},
		{"reflection 不豁免", NewAuth(single("s3cret")), context.Background(), "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.auth.authorize(tt.ctx, tt.method)
			if tt.ok {
				require.NoError(t, err)
				return
			}
			st, _ := status.FromError(err)
			require.Equal(t, codes.Unauthenticated, st.Code())
			// 与「上游未登录」区分:客户端靠 reason 判断该改服务令牌还是重新登录网易云。
			require.Equal(t, ReasonServerAuthFailed, errorInfoOf(t, err).GetReason())
			require.Equal(t, ErrorDomain, errorInfoOf(t, err).GetDomain())
		})
	}
}

// 多令牌:每个令牌各自可用,身份(名称)注入 ctx 供限流与审计使用。
func TestAuth_Authorize_MultiTokens(t *testing.T) {
	const method = "/netease.music.v1.SongService/GetSongDetail"
	auth := NewAuth([]AuthToken{
		{Name: "blog", Value: "t-blog"},
		{Name: "bot", Value: "t-bot"},
	})

	for _, tc := range []struct{ token, caller string }{
		{"t-blog", "blog"},
		{"t-bot", "bot"},
	} {
		ctx, err := auth.authorize(ctxWithAuth("Bearer "+tc.token), method)
		require.NoError(t, err)
		require.Equal(t, tc.caller, CallerFromContext(ctx))
	}

	// 未配置的令牌(比如刚被吊销、从文件里删掉的)立刻失效。
	_, err := auth.authorize(ctxWithAuth("Bearer t-revoked"), method)
	require.Equal(t, codes.Unauthenticated, status.Code(err))

	// 单令牌部署:身份为空串,不影响可用性。
	one := NewAuth(single("t"))
	ctx, err := one.authorize(ctxWithAuth("Bearer t"), method)
	require.NoError(t, err)
	require.Empty(t, CallerFromContext(ctx))
}

// 鉴权失败时不得执行 handler(更不能触达 cookie 解析与上游)。
func TestAuth_UnaryInterceptor_BlocksHandler(t *testing.T) {
	called := false
	handler := func(context.Context, any) (any, error) { called = true; return "ok", nil }
	info := &grpc.UnaryServerInfo{FullMethod: "/x/Y"}

	_, err := NewAuth(single("t")).UnaryInterceptor(context.Background(), nil, info, handler)
	require.Error(t, err)
	require.False(t, called)

	resp, err := NewAuth(single("t")).UnaryInterceptor(ctxWithAuth("Bearer t"), nil, info, handler)
	require.NoError(t, err)
	require.Equal(t, "ok", resp)
	require.True(t, called)
}

type fakeStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f fakeStream) Context() context.Context { return f.ctx }

func TestAuth_StreamInterceptor(t *testing.T) {
	info := &grpc.StreamServerInfo{FullMethod: "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo"}
	called := false
	handler := func(any, grpc.ServerStream) error { called = true; return nil }

	err := NewAuth(single("t")).StreamInterceptor(nil, fakeStream{ctx: context.Background()}, info, handler)
	require.Error(t, err)
	require.False(t, called)

	require.NoError(t, NewAuth(single("t")).StreamInterceptor(nil, fakeStream{ctx: ctxWithAuth("Bearer t")}, info, handler))
	require.True(t, called)
}

func TestIsLoopbackAddr(t *testing.T) {
	tests := map[string]bool{
		"127.0.0.1:3722": true,
		"127.1.2.3:80":   true,
		"[::1]:3722":     true,
		"localhost:3722": true,
		"LOCALHOST:1":    true,
		":3722":          false, // 省略 host = 全部网卡
		"0.0.0.0:3722":   false,
		"[::]:3722":      false,
		"192.168.1.5:80": false,
		"example.com:80": false,
		"127.0.0.1":      false, // 缺端口:不是合法监听地址,保守视为非回环
		"":               false,
	}
	for addr, want := range tests {
		require.Equal(t, want, isLoopbackAddr(addr), addr)
	}
}

func TestCheckExposure(t *testing.T) {
	require.NoError(t, checkExposure(NewAuth(nil), "127.0.0.1:1", "localhost:2"), "回环无需鉴权")
	require.NoError(t, checkExposure(NewAuth(single("t")), "0.0.0.0:1", ":2"), "有鉴权可对外")

	err := checkExposure(NewAuth(nil), "127.0.0.1:1", "0.0.0.0:2")
	require.ErrorContains(t, err, "0.0.0.0:2", "错误要指出是哪个地址")
	require.ErrorContains(t, err, "KITE_SERVER_AUTH_TOKEN")
	require.Error(t, checkExposure(nil, ":3722"))
}

func TestDialTarget(t *testing.T) {
	tests := map[string]string{
		"127.0.0.1:3722": "127.0.0.1:3722",
		"[::1]:3722":     "[::1]:3722",
		"0.0.0.0:3722":   "127.0.0.1:3722",
		"[::]:3722":      "[::1]:3722",
		"10.0.0.5:3722":  "10.0.0.5:3722",
	}
	for in, want := range tests {
		require.Equal(t, want, dialTarget(tcpAddr(t, in)), in)
	}
}

func tcpAddr(t *testing.T, s string) net.Addr {
	t.Helper()
	a, err := net.ResolveTCPAddr("tcp", s)
	require.NoError(t, err)
	return a
}
