// Package kiteclient 演示 Go 项目如何接入 kite-server:连接、带令牌、带网易云 cookie、按错误原因分支。
package kiteclient

import (
	"context"
	"errors"

	"buf.build/gen/go/vod-studio/kite/grpc/go/netease/music/v1/musicv1grpc"
	musicv1 "buf.build/gen/go/vod-studio/kite/protocolbuffers/go/netease/music/v1"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Dial 连接 kite-server,每个请求自动带访问令牌。
// 明文连接只适用于同机 / 内网;跨公网请在前面加 TLS 终止(kite-server 不内置 TLS)。
func Dial(addr, token string) (*grpc.ClientConn, error) {
	auth := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
	return grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(auth))
}

// WithNeteaseCookie 让这次调用以该网易云账号执行。
// 播放链接(VIP 权益)、每日推荐、刷新登录态等需要;不带则按匿名执行。
func WithNeteaseCookie(ctx context.Context, cookie string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "x-netease-cookie", cookie)
}

// Reason 取 kite 错误的原因码(ErrorInfo.reason),如 LOGIN_REQUIRED、SERVER_AUTH_FAILED;
// 不是 kite 的结构化错误时返回空串。全部原因码见 docs/grpc-errors.md。
func Reason(err error) string {
	for _, d := range status.Convert(err).Details() {
		if info, ok := d.(*errdetails.ErrorInfo); ok {
			return info.GetReason()
		}
	}
	return ""
}

// ErrReLogin 表示保存的网易云登录态已失效,需要重新扫码登录。
var ErrReLogin = errors.New("网易云登录态失效,需要重新登录")

// DailySongs 以登录账号取每日推荐;登录态缺失或失效时返回 ErrReLogin。
func DailySongs(ctx context.Context, conn *grpc.ClientConn, cookie string) ([]*musicv1.Song, error) {
	res, err := musicv1grpc.NewRecommendServiceClient(conn).
		GetDailyRecommend(WithNeteaseCookie(ctx, cookie), &musicv1.GetDailyRecommendRequest{})
	if Reason(err) == "LOGIN_REQUIRED" {
		return nil, ErrReLogin
	}
	if err != nil {
		return nil, err
	}
	return res.GetSongs(), nil
}
