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

`ErrorInfo` 的其它字段:

- `domain`:恒为 `kite.music`。`reason` 只在该 domain 内唯一。
- `metadata["provider"]`:出错的音乐源(当前恒为 `netease`;接入更多源后用它区分是哪个源出的错)。

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

需要登录的接口通过请求元数据传入上游 cookie,不进 proto 字段:

- gRPC:metadata `x-netease-cookie`
- REST:请求头 `Grpc-Metadata-X-Netease-Cookie`

缺失或失效时返回 `LOGIN_REQUIRED`。

## Rust(tonic)示例

下面的代码用 `tonic` / `tonic-types` 0.14 编译并通过测试。测试里**包含一段由 Go 服务端
(`toStatusError`)产出的 `google.rpc.Status` 二进制,Rust 端成功解析出 `LOGIN_REQUIRED` /
`kite.music` / `provider=netease`**,即跨语言的线上格式已验证。

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
            "RATE_LIMITED" | "UPSTREAM_UNAVAILABLE" | "CIRCUIT_OPEN" => Action::RetryLater,
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
