# gRPC / REST 错误契约

kite 的 gRPC 服务面向多种语言的客户端(Rust、Go、TypeScript……)。错误以**标准 gRPC status** 返回,
并携带 `google.rpc.ErrorInfo`;客户端按 `ErrorInfo.reason` 决定重试 / 重新登录 / 放弃,
不要解析 `message`(面向人,可能含上游细节,措辞会变)。

实现见 [`internal/server/errors.go`](../internal/server/errors.go);映射由拦截器链最外层的
`ErrorInterceptor` 完成,经 grpc-gateway 的 REST 接口同样受益。

## 映射表

| `reason` | gRPC code | REST 状态码 | 含义 | 客户端建议 |
|---|---|---|---|---|
| `LOGIN_REQUIRED` | `UNAUTHENTICATED` | 401 | 未登录 / 登录态失效 / 没有可用 session | 重新登录后重试;**不要**原样重试 |
| `RESOURCE_NOT_FOUND` | `NOT_FOUND` | 404 | 资源不存在:上游业务码为不存在,或上游正常应答但结果为空(歌曲 / 用户 ID 无效、昵称查不到、没有可用播放 URL 等) | 放弃 |
| `RATE_LIMITED` | `RESOURCE_EXHAUSTED` | 429 | 被上游限流 | 退避后重试 |
| `UPSTREAM_UNAVAILABLE` | `UNAVAILABLE` | 503 | 上游不可用(网络错误 / 5xx) | 退避后重试 |
| `CIRCUIT_OPEN` | `UNAVAILABLE` | 503 | 本服务熔断器打开,请求被短路 | 稍后重试 |
| `UPSTREAM_INVALID_RESPONSE` | `INTERNAL` | 500 | 上游返回了形状不符预期的响应(JSON 解码失败、缺字段)。已带明确类别的错误(如不存在)不会落入此项 | 不要盲目重试,可能是上游接口变了 |
| `UNSUPPORTED` | `UNIMPLEMENTED` | 501 | 不支持的平台 / 能力 | 放弃 |
| `INTERNAL` | `INTERNAL` | 500 | 未分类的内部错误(`message` 固定为「内部错误」,细节只在服务端日志) | 放弃或上报 |
| `CANCELED` | `CANCELED` | 499 | 调用方取消了请求 | — |
| `DEADLINE_EXCEEDED` | `DEADLINE_EXCEEDED` | 504 | 请求超时 | 按需重试 |
| `SERVER_AUTH_FAILED` | `UNAUTHENTICATED` | 401 | 缺少或错误的**服务访问令牌**(与上游登录无关,见 [服务部署与访问控制](server-security.md)) | 检查令牌配置;**不要**重试,更不要去重新登录网易云 |
| `SERVER_RATE_LIMITED` | `RESOURCE_EXHAUSTED` | 429 | 被**本服务**限流:该令牌超出了 `KITE_SERVER_RATE_PER_MINUTE` 额度(与上游限流的 `RATE_LIMITED` 无关) | 退避后重试;长期超限请向服务部署方申请更高额度或独立令牌 |
| `ACCOUNT_NOT_FOUND` | `NOT_FOUND` | 404 | 显式选择的托管账号不存在或已删除 | 删除本地保存的 ID,重新登录 |
| `ACCOUNT_FORBIDDEN` | `PERMISSION_DENIED` | 403 | 账号属于其它服务调用方 | 使用自己的账号及令牌,不要重试 |
| `ACCOUNT_EXPIRED` | `UNAUTHENTICATED` | 401 | 所选托管账号已过期或被上游确认未登录 | 重新登录,或显式 Logout 删除记录 |
| `ACCOUNT_CHANGED` | `ABORTED` | 409 | 并发登录/刷新改变了记录,旧快照的刷新、过期或删除被拒绝 | 重新读取当前状态后决定操作,不要盲目重放 |
| `ACCOUNTS_DISABLED` | `FAILED_PRECONDITION` | 400 | 选择了账号 ID,但未配置 `KITE_SERVER_ACCOUNTS_FILE` | 请部署方启用托管存储,或使用直传 cookie |
| `INVALID_CREDENTIALS` | `INVALID_ARGUMENT` | 400 | 同时携带 cookie 和账号 ID,或账号 ID 为空/多个值 | 每次仅传一种凭据,账号 ID 必须是单个非空值 |

`ErrorInfo` 的其它字段:

- `domain`:恒为 `kite.music`。`reason` 只在该 domain 内唯一。
- `metadata["provider"]`:出错的音乐源(当前恒为 `netease`;接入更多源后用它区分是哪个源出的错)。
  与具体音乐源无关的错误(如 `SERVER_AUTH_FAILED`)没有该字段。

`reason` 与 `domain` 是**对外契约**:新增取值是兼容变更,改名或删除是破坏性变更。

## REST 响应体

grpc-gateway 把 status 序列化为 JSON,`details` 里就是 `ErrorInfo`:

```json
{
  "code": 16,
  "message": "登录态失效: code=301",
  "details": [
    {
      "@type": "type.googleapis.com/google.rpc.ErrorInfo",
      "reason": "LOGIN_REQUIRED",
      "domain": "kite.music",
      "metadata": { "provider": "netease" }
    }
  ]
}
```

## 凭据

凭据通过 metadata 传入,不进请求 proto 字段;以下两种模式互斥:

| 模式 | gRPC metadata | REST 请求头 |
|---|---|---|
| 直传 cookie | `x-netease-cookie` | `Grpc-Metadata-X-Netease-Cookie` |
| 托管账号 ID | `x-kite-account-id` | `Grpc-Metadata-X-Kite-Account-Id` |

同时传两种凭据、空或多个账号 ID 返回 `INVALID_CREDENTIALS`。不配置 `KITE_SERVER_ACCOUNTS_FILE` 时保留直传行为,选择账号 ID 返回 `ACCOUNTS_DISABLED`。账号只能由其所属服务调用方使用,不会自动挑选或回退到其它账号;完整所有权与持久化规则见 [接入指南](integration.md)及[服务安全](server-security.md)。
登录引导接口 `SendCaptcha`、`LoginQrcode`、`LoginByCellphone`、`CheckQrcode` 不接受账号选择器,携带账号 ID 也返回 `INVALID_CREDENTIALS`;这些调用应省略账号 ID 头。

直传模式需要登录的接口缺失或失效返回 `LOGIN_REQUIRED`。少数接口为**可选登录**(如 `SongService/GetSongURL`、`UserService/UserPlaylist`、`PlaylistService/GetPlaylist`、`PlaylistService/AllTracks`,以 proto 为准):无凭据按匿名执行;带 cookie 按该账号执行,上游可能在 cookie 失效时按匿名返回公开内容。显式选择不存在、无权或已过期的托管账号则分别报账号原因码,即使接口允许匿名也不会悄悄降级。

托管 `LoginStatus` 的空登录态或上游未授权将该版本账号标为过期,返回 `ACCOUNT_EXPIRED`;一般网络错误及权益不足不会使账号过期。并发新登录使旧快照失效时返回 `ACCOUNT_CHANGED`。存储加载失败会拒绝启动,运行期持久化失败返回脱敏的 `INTERNAL`,不报告成功且不发布未落盘变更。

服务访问令牌(`authorization: Bearer ...`)决定「谁能调用 kite」和托管账号归属,不替代账号凭据;访问令牌错误为 `SERVER_AUTH_FAILED`,不是网易云重新登录能解决的错误。

下面的 Rust 示例保留为直传模式分类示例;托管客户端还应将 `ACCOUNT_EXPIRED` 归为重新登录,并分别处理上表其它账号原因码。

## Rust(tonic)示例

下面的代码用 `tonic` / `tonic-types` 0.14 编译,**在 CI 里对真实运行的 kite-server 执行**
(`interop/rust/`,`make interop-rust`):用 proto 生成 tonic 客户端,断言无令牌得到 `SERVER_AUTH_FAILED`、
有令牌但未登录得到 `LOGIN_REQUIRED`,并用下面的 `classify` 分类。本文的 Rust 代码块与
`interop/rust/src/classify.rs` 逐字一致(测试守护),不会与实际可编译的代码脱节。

```rust
use tonic::{Code, Status};
use tonic_types::StatusExt;

#[derive(Debug, PartialEq)]
pub enum Action {
    Relogin,
    RetryLater,
    GiveUp,
}

/// 按 kite 的 ErrorInfo.reason 决定动作。
pub fn classify(status: &Status) -> Action {
    let details = status.get_error_details();
    match details.error_info() {
        Some(info) if info.domain == "kite.music" => match info.reason.as_str() {
            "LOGIN_REQUIRED" => Action::Relogin,
            "RATE_LIMITED" | "SERVER_RATE_LIMITED" | "UPSTREAM_UNAVAILABLE" | "CIRCUIT_OPEN" => Action::RetryLater,
            _ => Action::GiveUp,
        },
        // 没有 ErrorInfo(例如网络层错误)时退回 gRPC code。
        _ => match status.code() {
            Code::Unavailable => Action::RetryLater,
            _ => Action::GiveUp,
        },
    }
}
```
