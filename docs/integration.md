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
2. **网易云登录态**:凭据有两种互斥模式,均不放进请求 message:
   - **直传 cookie(默认)**:未配置 `KITE_SERVER_ACCOUNTS_FILE` 时,登录返回的 `session.cookie` 由接入方保存。gRPC 用 `x-netease-cookie`,REST 用 `Grpc-Metadata-X-Netease-Cookie`。续期调用 `AuthService/RefreshLogin`,用返回的完整 cookie 替换旧值;退出调用 `AuthService/Logout` 后删除自己保存的 cookie。没带凭据时 Logout 也可成功,不能据此判断已注销。需要登录的接口缺失或失效时返回 `LOGIN_REQUIRED`。
   - **托管账号(可选)**:部署方配置 `KITE_SERVER_ACCOUNTS_FILE` 后,手机号登录或确认二维码登录把账号持久化,返回 `session.account_id`(REST:`accountId`),`session.cookie` 为空。接入方保存这个不透明 ID,gRPC 用 `x-kite-account-id`,REST 用 `Grpc-Metadata-X-Kite-Account-Id` 显式选择;不传凭据仍按匿名执行,不会自动借用任何托管账号。
   - 二维码流程:`AuthService/LoginQrcode` 拿 `url` 自行画二维码,约 2 秒轮询 `CheckQrcode`;`CONFIRMED` 时保存对应模式的凭据,`EXPIRED` 时重新取二维码。确认响应未附资料时使用本次登录的新 cookie 补查,不会沿用旧账号;缺少新凭据或资料无效返回 `UPSTREAM_INVALID_RESPONSE`。
   - cookie 与账号 ID **不得同时发送**;账号 ID 为空或重复发送多个值返回 `INVALID_CREDENTIALS`。未启用托管存储却选择账号返回 `ACCOUNTS_DISABLED`。
   - 登录引导接口 `SendCaptcha`、`LoginQrcode`、`LoginByCellphone`、`CheckQrcode` 不接受账号选择器;携带 `x-kite-account-id` 返回 `INVALID_CREDENTIALS`,登录时应省略该头。
3. **按原因码分支,不要解析错误文本**:错误以标准 gRPC status + `ErrorInfo` 返回,全部原因码见 [grpc-errors.md](grpc-errors.md)。

## 托管账号的归属与生命周期

账号 ID 不是服务访问令牌,只能由其所属调用方使用。命名令牌的所有者是 `caller:<名称>`,同名轮换令牌保留归属;单令牌来源(`KITE_SERVER_AUTH_TOKEN` 或 `_FILE`)共用 `single`;无鉴权回环部署的全部本机客户端共用 `local`。调用方不能通过 metadata 指定所有者。同一调用方重新登录同一网易云用户保留账号 ID、更新凭据;不同调用方登录同一用户得到独立 ID。多个账号可同时保存,每次请求必须显式选一个。

- `LoginStatus` 带账号 ID 时返回同一 `account_id`,不返回 cookie。确定未登录的空状态或上游未授权会将该快照标为过期,返回 `ACCOUNT_EXPIRED`;普通网络故障或权益错误不会使账号过期。
- 显式调用 `RefreshLogin` 才续期:成功先持久化合并后的 cookie,响应仅返回 `account_id`,`cookie` 为空。没有后台自动续期保证。
- 显式调用 `Logout` 成功意味着记录已经持久化删除;过期账号可以直接本地删除,无需访问上游。直传 cookie 刷新或登出若精确匹配当前调用方的活跃托管记录,也会同步其持久化生命周期。
- 不存在的 ID 返回 `ACCOUNT_NOT_FOUND`,其它调用方的 ID 返回 `ACCOUNT_FORBIDDEN`,过期账号返回 `ACCOUNT_EXPIRED`,均不会回退到其它账号或匿名。并发重新登录后,旧请求不能覆盖、过期或删除新凭据,而返回 `ACCOUNT_CHANGED`;重新读取当前状态再决定操作,不要盲目重放。
- 持久化失败返回脱敏的 `INTERNAL`,不会报告保存/退出成功或把未落盘状态发布出来。重启后从同一文件恢复账号及归属;文件运维要求见 [server-security.md](server-security.md)。

以下是独立的托管 REST 示例(先从登录响应的 `session.accountId` 保存 `KITE_ACCOUNT_ID`,不要再带 cookie):

```bash
curl -X POST \
  -H "Authorization: Bearer $KITE_TOKEN" \
  -H "Grpc-Metadata-X-Kite-Account-Id: $KITE_ACCOUNT_ID" \
  -H "Content-Type: application/json" -d '{}' \
  http://127.0.0.1:3721/api/v1/auth/refresh
```

响应的 `accountId` 仍为所选账号,没有可用 cookie。以下 Go 示例保留为直传模式示例;托管模式改用上述账号 ID metadata,并按[错误契约](grpc-errors.md)处理账号原因码。

## 播放链接

`SongService/GetSongURL` 是**可选登录**:不带凭据时只有免费歌,VIP / 无版权的歌 `url` 为空;直传 cookie 或显式选择托管账号时按该账号权益返回。直链几十分钟后过期,**不要存库**:在用户点播放时再取(例如后端 302 跳转到直链)。

账号对该歌无权益(未购数字专辑、版权受限等)时,上游可能只返回约 30 秒的**试听片段**:`url.trial` 为 `true` 并带 `trialStartMs` / `trialEndMs`(毫秒),`url` 只覆盖试听区间;上游未提供试听信息时 `trial` 为 `false` 且两个区间字段保持 0。不要把 `url` 非空当作可完整播放,按 `trial` 分支。响应同时回传上游逐条的 `code` 与 `fee`。

## 账号歌单

`UserService/UserPlaylist`、`PlaylistService/GetPlaylist`、`PlaylistService/AllTracks` 也是**可选登录**:

- 带本人的 cookie 或拥有的托管账号 ID 时按该账号读,能看到私有歌单及其歌曲;不带时匿名,只有公开内容。服务端保存的账号不会被借给任何请求。
- 已认证结果按凭据身份隔离缓存,不同账号、不同调用方与匿名请求之间不会串数据;缓存键不包含明文 cookie。
- 直传 cookie 失效时上游可能按匿名返回公开内容,不报 `LOGIN_REQUIRED`;可调 `AuthService/LoginStatus` 确认(未登录时 `session` 为空)。显式选择已过期的托管账号始终返回 `ACCOUNT_EXPIRED`,不会匿名降级。
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
