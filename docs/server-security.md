# 服务部署与访问控制

kite 的 gRPC / REST 服务面向其它项目调用(Rust、Go、TypeScript……),且服务端持有上游账号的登录态。
因此默认值偏安全:**只监听本机回环地址;要对外监听,必须配置访问令牌,否则拒绝启动。**

实现见 [`internal/server/auth.go`](../internal/server/auth.go)、[`internal/server/grpc.go`](../internal/server/grpc.go),
配置见 [`config/config.go`](../config/config.go)。

## 配置

全部走环境变量:

| 变量 | 默认 | 说明 |
|---|---|---|
| `KITE_SERVER_GRPC_ADDR` | `127.0.0.1:3722` | gRPC 监听地址 |
| `KITE_SERVER_HTTP_ADDR` | `127.0.0.1:3721` | REST(grpc-gateway)监听地址 |
| `KITE_SERVER_AUTH_TOKEN` | 空 | 访问令牌 |
| `KITE_SERVER_AUTH_TOKEN_FILE` | 空 | 从文件读取令牌(Docker / k8s secret 的惯用方式),首尾空白会被去掉 |

规则:

- 两个地址**都**是回环(`127.0.0.1`、`[::1]`、`localhost`)且没配令牌 → 不鉴权,本机进程直接调用。
- 任一地址不是回环(`0.0.0.0`、`:3722`、网卡 IP、域名)且没配令牌 → **拒绝启动**,错误信息会指出是哪个地址。
- 配了令牌 → 无论绑哪里都鉴权。
- `KITE_SERVER_AUTH_TOKEN` 与 `_FILE` 同时设置、或 `_FILE` 指向不存在 / 空文件 → 拒绝启动
  (宁可起不来,也不在以为有鉴权时裸奔)。

生成令牌:`openssl rand -hex 32`。

## 调用方式

令牌**只决定谁能调用本服务**,与上游网易云的登录态(`x-netease-cookie`)无关,两者互不替代。

| 协议 | 携带方式 |
|---|---|
| gRPC | metadata `authorization: Bearer <token>` |
| REST | 请求头 `Authorization: Bearer <token>`(grpc-gateway 转发给 gRPC,规则与 gRPC 一致) |

```bash
# gRPC(reflection 同样需要令牌)
grpcurl -plaintext -H "authorization: Bearer $KITE_TOKEN" 127.0.0.1:3722 list

# REST
curl -H "Authorization: Bearer $KITE_TOKEN" http://127.0.0.1:3721/api/v1/songs/347230
```

Rust(tonic):给生成的客户端挂拦截器。下面的拦截器用 `tonic` 0.14(edition 2024)编译,并在 CI 里对真实运行的服务端做互操作测试(`interop/rust/`)。

```rust
use tonic::{metadata::MetadataValue, Request, Status};

// `use<>`:返回的拦截器不借用 `token`,否则传入临时字符串(如从环境变量读取)时无法编译。
pub fn auth_interceptor(
    token: &str,
) -> Result<impl FnMut(Request<()>) -> Result<Request<()>, Status> + Clone + use<>, tonic::metadata::errors::InvalidMetadataValue>
{
    let value: MetadataValue<_> = format!("Bearer {token}").parse()?;
    Ok(move |mut req: Request<()>| {
        req.metadata_mut().insert("authorization", value.clone());
        Ok(req)
    })
}

// let client = SongServiceClient::with_interceptor(channel, auth_interceptor(&token)?);
```

缺少或错误的令牌返回 `UNAUTHENTICATED`(REST 401),`ErrorInfo.reason = SERVER_AUTH_FAILED`。
它与 `LOGIN_REQUIRED`(上游未登录)是两种问题,修复方式不同,见 [错误契约](grpc-errors.md)。

## 健康检查

不需要令牌,编排系统探活不携带凭据:

- gRPC:标准 `grpc.health.v1.Health`(`Check` / `Watch`)。`service` 留空查整体,也可查具体 service 名
  (如 `netease.music.v1.SongService`)。
- REST:`GET /healthz`,SERVING 返回 `200 {"status":"SERVING"}`,否则 `503 {"status":"NOT_SERVING"}`。

`grpcurl` 默认靠 reflection 取方法描述符,而 reflection 需要令牌,所以用 `grpcurl` 查健康检查时也要带 `-H`。
这不影响探活:k8s 的 gRPC 探针和带生成代码的客户端直接调用 `Health/Check`,不走 reflection。

收到 SIGINT / SIGTERM 后,服务**先**把健康状态置为 `NOT_SERVING`(让负载均衡摘流),再排空 REST 与 gRPC;
超过关闭期限(默认 5 秒)则强制停止。

Kubernetes 示例:

```yaml
livenessProbe:
  grpc: { port: 3722 }
```

## Docker

镜像内必须监听全部网卡,端口映射才通,所以镜像默认设置了 `KITE_SERVER_*_ADDR=0.0.0.0:...`——
**因此不带令牌启动镜像会直接失败**,这是有意的:

```bash
docker run --rm \
  -e KITE_SERVER_AUTH_TOKEN="$(openssl rand -hex 32)" \
  -p 127.0.0.1:3721:3721 -p 127.0.0.1:3722:3722 \
  ghcr.io/vod-studio/kite-server
```

端口映射建议显式写 `127.0.0.1:` 前缀;不写则 Docker 会把端口发布到宿主机全部网卡。

## 边界(诚实登记)

- **不含 TLS**:令牌在明文连接上传输。对外(非回环)暴露时,放在做 TLS 终结的反向代理 / 服务网格之后。
- **单一共享令牌**:没有按调用方区分身份、没有限流。多租户需求出现时再设计。
