// Package server 的错误映射。
//
// service 层直接返回 engine 的哨兵错误(merrors.ErrUnauthorized 等)。未映射时 gRPC 把它们都当
// codes.Unknown,grpc-gateway 对外统一成 HTTP 500——Rust 等其它语言的客户端分不清「未登录」
// 「资源不存在」「被限流」,也就没法做重试 / 重新登录。
//
// 这里在拦截器链最外层把它们映射为标准 gRPC status + google.rpc.ErrorInfo
// (reason 是稳定的机器可读标识,客户端据此分支;message 面向人)。映射表见 docs/grpc-errors.md。
package server

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	merrors "github.com/VOD-Studio/kite/errors"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// ErrorDomain 是 ErrorInfo.Domain。Reason 在该域内唯一,属于对外契约,不得随意改名。
const ErrorDomain = "kite.music"

// ErrorInfo.Reason 取值(对外契约)。
const (
	ReasonLoginRequired           = "LOGIN_REQUIRED"            // 未登录 / 登录态失效 / 无可用 session
	ReasonResourceNotFound        = "RESOURCE_NOT_FOUND"        // 资源不存在
	ReasonRateLimited             = "RATE_LIMITED"              // 被上游限流
	ReasonUpstreamUnavailable     = "UPSTREAM_UNAVAILABLE"      // 上游不可用(网络错误 / 5xx)
	ReasonCircuitOpen             = "CIRCUIT_OPEN"              // 熔断器打开,短路
	ReasonUpstreamInvalidResponse = "UPSTREAM_INVALID_RESPONSE" // 上游返回了无法解析的响应
	ReasonUnsupported             = "UNSUPPORTED"               // 不支持的平台 / 能力
	ReasonInternal                = "INTERNAL"                  // 未分类的内部错误
	ReasonCanceled                = "CANCELED"                  // 调用方取消了请求
	ReasonDeadlineExceeded        = "DEADLINE_EXCEEDED"         // 请求超时
	ReasonServerAuthFailed        = "SERVER_AUTH_FAILED"        // 缺少或错误的服务访问令牌(与上游登录无关)
	ReasonServerRateLimited       = "SERVER_RATE_LIMITED"       // 被本服务限流(与上游限流 RATE_LIMITED 无关)
	ReasonInvalidCredentials      = "INVALID_CREDENTIALS"       // 账号 ID 与 Cookie 冲突或账号选择不唯一
	ReasonAccountsDisabled        = "ACCOUNTS_DISABLED"         // 未配置托管账号存储
	ReasonAccountNotFound         = "ACCOUNT_NOT_FOUND"         // 托管账号不存在或已退出
	ReasonAccountForbidden        = "ACCOUNT_FORBIDDEN"         // 账号属于其他调用方
	ReasonAccountExpired          = "ACCOUNT_EXPIRED"           // 指定账号的登录态已失效
	ReasonAccountChanged          = "ACCOUNT_CHANGED"           // 凭据版本已改变,旧请求不可覆盖新登录
)

// 当前只有网易云一个 provider。多源改造(P2)后由 provider 注册信息提供。
const currentProvider = "netease"

// classify 把 service 返回的错误归类为 gRPC code 与稳定的 reason。
// 顺序有意义:更具体的先判(熔断先于通用的上游不可用)。
func classify(err error) (codes.Code, string) {
	switch {
	case errors.Is(err, merrors.ErrUnauthorized), errors.Is(err, session.ErrNoAvailableSession):
		return codes.Unauthenticated, ReasonLoginRequired
	case errors.Is(err, merrors.ErrNotFound):
		return codes.NotFound, ReasonResourceNotFound
	case errors.Is(err, merrors.ErrRateLimited):
		return codes.ResourceExhausted, ReasonRateLimited
	case errors.Is(err, engine.ErrCircuitOpen):
		return codes.Unavailable, ReasonCircuitOpen
	case errors.Is(err, merrors.ErrUpstreamUnavailable):
		return codes.Unavailable, ReasonUpstreamUnavailable
	case errors.Is(err, merrors.ErrInvalidResponse):
		return codes.Internal, ReasonUpstreamInvalidResponse
	case errors.Is(err, merrors.ErrUnsupportedPlatform):
		return codes.Unimplemented, ReasonUnsupported
	default:
		return codes.Internal, ReasonInternal
	}
}

// toStatusError 把 handler 返回的错误转成携带 ErrorInfo 的 gRPC status error。
//   - 已经是 gRPC status 的错误原样返回(不覆盖更具体的上层决定)。
//   - 请求 context 已结束 → Canceled / DeadlineExceeded。engine 的 transport 用 %v 包装底层错误,
//     错误链里没有 context 错误,必须看 ctx.Err()。
//   - 未分类的错误对外只给通用信息并记日志,避免把内部细节泄露给调用方。
func toStatusError(ctx context.Context, method string, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := status.FromError(err); ok {
		return err
	}

	var code codes.Code
	var reason, msg string
	switch ctxErr := ctx.Err(); {
	case errors.Is(ctxErr, context.Canceled):
		code, reason, msg = codes.Canceled, ReasonCanceled, "请求已取消"
	case errors.Is(ctxErr, context.DeadlineExceeded):
		code, reason, msg = codes.DeadlineExceeded, ReasonDeadlineExceeded, "请求超时"
	default:
		code, reason = classify(err)
		msg = err.Error()
		if reason == ReasonInternal {
			slog.ErrorContext(ctx, "unclassified error", slog.String("method", method), slog.String("error", err.Error()))
			msg = "内部错误"
		}
	}

	return statusError(code, reason, msg, map[string]string{"provider": currentProvider})
}

// statusError 构造携带 ErrorInfo 的 gRPC status error。所有对外错误都经此出口,
// 保证 domain 恒定、details 结构一致。
func statusError(code codes.Code, reason, msg string, metadata map[string]string) error {
	st := status.New(code, msg)
	if withInfo, err := st.WithDetails(&errdetails.ErrorInfo{
		Reason:   reason,
		Domain:   ErrorDomain,
		Metadata: metadata,
	}); err == nil {
		st = withInfo
	}
	return st.Err()
}

// ErrorInterceptor 把 handler 的错误统一映射为 gRPC status。须放在拦截器链最外层。
func ErrorInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	resp, err := handler(ctx, req)
	if err != nil {
		return resp, toStatusError(ctx, info.FullMethod, err)
	}
	return resp, nil
}
