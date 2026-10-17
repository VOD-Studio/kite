// Package server 的服务访问鉴权。
//
// 与上游网易云的登录态(x-netease-cookie)是两回事:这里的 bearer token 决定「谁能调用本服务」。
// kite 的 gRPC/REST 服务可能被其它语言的项目当 sidecar 或远程服务使用,没有鉴权就等于把
// 服务端 session 池里的账号交给能连上端口的任何人。
//
// 策略(fail closed):
//   - 配了 token:除健康检查外,每个 RPC(含 reflection)都必须带 `authorization: Bearer <token>`;
//     REST 经 grpc-gateway 传同名 `Authorization` 头。
//   - 没配 token:只允许监听回环地址(见 checkExposure),本机进程间调用无需鉴权。
package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

// healthMethodPrefix 是 gRPC 标准健康检查的方法前缀。编排系统(k8s / LB)探活不携带凭据,
// 且健康状态不含敏感信息,因此豁免鉴权。
const healthMethodPrefix = "/grpc.health.v1.Health/"

// Auth 校验调用方携带的 bearer token。零值与 nil 都表示未启用,放行一切。
type Auth struct {
	// 存 token 的 SHA-256:比较定长摘要,避免 token 长度经耗时泄露。
	digest  [sha256.Size]byte
	enabled bool
}

// NewAuth 用 token 构造鉴权器;token 为空返回未启用的鉴权器。
func NewAuth(token string) *Auth {
	if token == "" {
		return &Auth{}
	}
	return &Auth{digest: sha256.Sum256([]byte(token)), enabled: true}
}

// Enabled 报告是否启用了鉴权。
func (a *Auth) Enabled() bool { return a != nil && a.enabled }

// authorize 校验 ctx 里的 incoming metadata。
func (a *Auth) authorize(ctx context.Context, fullMethod string) error {
	if !a.Enabled() || strings.HasPrefix(fullMethod, healthMethodPrefix) {
		return nil
	}
	md, _ := metadata.FromIncomingContext(ctx)
	for _, v := range md.Get("authorization") {
		scheme, token, ok := strings.Cut(v, " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") {
			continue
		}
		got := sha256.Sum256([]byte(strings.TrimSpace(token)))
		if subtle.ConstantTimeCompare(got[:], a.digest[:]) == 1 {
			return nil
		}
	}
	// 缺失与错误不区分,不给调用方额外信息。
	return statusError(codes.Unauthenticated, ReasonServerAuthFailed,
		"缺少或无效的服务访问令牌(authorization: Bearer <token>)", nil)
}

// UnaryInterceptor 鉴权 unary RPC。放在 ErrorInterceptor 之内、CookieInterceptor 之前:
// 未通过鉴权的请求不会走到任何业务代码,也不会被解析 cookie。
func (a *Auth) UnaryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if err := a.authorize(ctx, info.FullMethod); err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

// StreamInterceptor 鉴权流式 RPC(目前是 reflection)。unary 拦截器管不到流,
// 漏掉它会让 reflection 成为无需凭据的入口。
func (a *Auth) StreamInterceptor(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if err := a.authorize(ss.Context(), info.FullMethod); err != nil {
		return err
	}
	return handler(srv, ss)
}

// isLoopbackAddr 判断监听地址是否只绑本机回环。
// 空 host(":3722")、0.0.0.0、[::] 都监听全部网卡,不算回环;"localhost" 算。
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil || host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// checkExposure 拒绝「对外监听却没有鉴权」的配置。
func checkExposure(auth *Auth, addrs ...string) error {
	if auth.Enabled() {
		return nil
	}
	for _, addr := range addrs {
		if !isLoopbackAddr(addr) {
			return fmt.Errorf("监听地址 %q 对外开放但未配置访问令牌:"+
				"设置 KITE_SERVER_AUTH_TOKEN(或 KITE_SERVER_AUTH_TOKEN_FILE),或改为监听 127.0.0.1", addr)
		}
	}
	return nil
}
