# 在你的项目里接入 kite

kite-server 以 gRPC(默认 `:3722`)和 REST(grpc-gateway,默认 `:3721`)提供网易云能力。本文说明接入方需要知道的全部约定;部署方式见 [deploy.md](deploy.md)、[server-security.md](server-security.md)。

## 有哪些接口

接口清单都是从代码生成的,不需要手抄:

| 来源 | 内容 | 适合 |
|---|---|---|
| BSR 上生成的 SDK `buf.build/gen/go/vod-studio/kite` | 全部服务的强类型客户端 | **Go 项目(推荐)**:接口即 IDE 补全,用错编译期报错 |
| BSR 模块 [`buf.build/vod-studio/kite`](https://buf.build/vod-studio/kite) | proto 契约 | 其它语言:`buf generate` 生成客户端(Rust 示例见 [grpc-errors.md](grpc-errors.md)) |
| [`gen/openapi/kite.swagger.json`](../gen/openapi/kite.swagger.json) | 全部 REST 路由、参数、返回结构 | 走 REST 的调用方,可导入 Postman 或生成客户端 |
| [`proto/netease/music/v1/`](../proto/netease/music/v1/) | 每个 RPC 的语义说明(中文注释) | 需要读懂语义时 |

## 三个约定

1. **访问令牌**:每个请求带 `authorization: Bearer <令牌>`(REST 用同名 `Authorization` 头)。令牌决定「谁能调用 kite」,错误时原因码为 `SERVER_AUTH_FAILED`。
2. **网易云登录态**:需要以某个账号执行时,带 metadata `x-netease-cookie`(REST:`Grpc-Metadata-X-Netease-Cookie`)。cookie 由**接入方保存**,kite 不存储任何人的登录态。
   - 登录:`AuthService/LoginQrcode` 拿二维码 `url`(自己画二维码)→ 约 2 秒轮询 `CheckQrcode`;`CONFIRMED` 时返回包含 `user_id`、`cookie` 与账号资料的 `session`(REST 字段为 `userId`),接入方保存完整 `cookie`;`EXPIRED` 时重新取二维码。确认响应未附账号资料时,kite 使用本次登录的新 cookie 补查,不会沿用调用方的旧账号。上游缺少新凭据或返回无效账号资料时,返回 `UPSTREAM_INVALID_RESPONSE`,不会返回无凭据的 `CONFIRMED`。
   - 续期:定期调 `AuthService/RefreshLogin`,用返回的完整 cookie 替换旧值。
   - 退出:调 `AuthService/Logout`,**再删除自己保存的 cookie**(没带 cookie 时 Logout 也返回成功,不能据此判断已注销)。
   - 缺失或失效时原因码为 `LOGIN_REQUIRED`:提示用户重新登录。
3. **按原因码分支,不要解析错误文本**:错误以标准 gRPC status + `ErrorInfo` 返回,全部原因码见 [grpc-errors.md](grpc-errors.md)。

## 播放链接

`SongService/GetSongURL` 是**可选登录**:不带 cookie 时只有免费歌,VIP / 无版权的歌 `url` 为空;带 VIP 账号的 cookie 时按该账号权益返回。直链几十分钟后过期,**不要存库**:在用户点播放时再取(例如后端 302 跳转到直链)。

## 账号歌单

`UserService/UserPlaylist`、`PlaylistService/GetPlaylist`、`PlaylistService/AllTracks` 也是**可选登录**:

- 带本人的 cookie 时按该账号读,能看到私有歌单及其歌曲;不带时匿名,只有公开内容。服务端保存的账号不会被借给任何请求。
- 带 cookie 的结果按账号单独缓存,不同账号、匿名请求之间不会串数据。
- cookie 失效时上游按匿名返回公开内容,**不会**报 `LOGIN_REQUIRED`;要确认登录态,调 `AuthService/LoginStatus`(未登录时 `session` 为空)。
- 分页:`total` 是总数(`UserPlaylist` 为当前 `filter` 下的总数),不随 `limit` / `offset` 变化;每页不超过 `limit`(默认值与上限见 proto 注释),`offset` 超出总数时返回空页。

## Go 示例

依赖 BSR 生成的 SDK:

```bash
GOPROXY=https://buf.build/gen/go,https://proxy.golang.org GONOSUMDB=buf.build/gen/go \
  go get buf.build/gen/go/vod-studio/kite/grpc/go@latest buf.build/gen/go/vod-studio/kite/protocolbuffers/go@latest
```

下面的代码与 [`interop/go/kite.go`](../interop/go/kite.go) 逐字一致(测试守护),并在 CI 里对真实运行的 kite-server 执行(`make interop-go`):

```go
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
```

调用:

- `conn, err := kiteclient.Dial("kite:3722", token)`,`defer conn.Close()`;
- 需要账号时 `ctx = kiteclient.WithNeteaseCookie(ctx, cookie)`,再调任意 `musicv1grpc.New<Service>Client(conn)` 的方法;
- `kiteclient.Reason(err)` 判断原因码,如 `DailySongs` 里把 `LOGIN_REQUIRED` 转成「需要重新登录」。
