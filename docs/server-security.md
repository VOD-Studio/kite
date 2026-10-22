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
| `KITE_SERVER_AUTH_TOKEN` | 空 | 访问令牌(单令牌) |
| `KITE_SERVER_AUTH_TOKEN_FILE` | 空 | 从文件读取令牌(Docker / k8s secret 的惯用方式),首尾空白会被去掉 |
| `KITE_SERVER_AUTH_TOKENS_FILE` | 空 | 多令牌文件:每行「名称:令牌」,空行与 `#` 注释忽略;用于按调用方发放令牌 |
| `KITE_SERVER_RATE_PER_MINUTE` | `0`(不限) | 每个令牌每分钟允许的请求数,超出返回 429(见下) |
| `KITE_SERVER_TLS_CERT` / `KITE_SERVER_TLS_KEY` | 空 | TLS 证书与私钥文件,两者必须同时设置;设置后 gRPC 与 REST 都走 TLS |

规则:

- 两个地址**都**是回环(`127.0.0.1`、`[::1]`、`localhost`)且没配令牌 → 不鉴权,本机进程直接调用。
- 任一地址不是回环(`0.0.0.0`、`:3722`、网卡 IP、域名)且没配令牌 → **拒绝启动**,错误信息会指出是哪个地址。
- 配了令牌 → 无论绑哪里都鉴权。
- 三个令牌来源(`KITE_SERVER_AUTH_TOKEN` / `_FILE` / `_TOKENS_FILE`)**只能设置一个**,同时设置、或文件不存在 / 为空 / 格式错误 → **拒绝启动**
  (宁可起不来,也不在以为有鉴权时裸奔)。
- 只配 `KITE_SERVER_TLS_CERT` 不配 `_KEY`(或反过来)、证书加载失败 → **拒绝启动**,不会静默退回明文。

生成令牌:`openssl rand -hex 32`。

## 多令牌与调用方身份

多个项目共用一个 kite 服务时,用 `KITE_SERVER_AUTH_TOKENS_FILE` 给每个调用方发独立令牌:

```bash
# /etc/kite/tokens
blog:9f1c…          # 博客站点
bot:3ad7…           # 机器人
```

每行「名称:令牌」,名称是限流与日志里的调用方身份(不能重复,同一令牌也不能发给两个调用方,否则拒绝启动)。调用方式与单令牌完全一致——`authorization: Bearer <令牌>`,客户端不需要任何改动。

- **吊销单个调用方**:从文件里删掉对应行,重启服务。其余调用方的令牌不受影响。
- **审计**:限流拒绝的日志(`server rate limited`)会带 `caller` 名称。
- 单令牌部署(只有 `KITE_SERVER_AUTH_TOKEN`)行为不变:等价于一个未命名的调用方。

## 限流

`KITE_SERVER_RATE_PER_MINUTE` 是**每个令牌**(单令牌部署则是全部调用)每分钟的请求额度,令牌桶语义:额度内突发可用,用完后匀速回填(每 `60/额度` 秒回 1 个)。默认 `0` 表示不限流,与旧版行为一致。

- 目的:服务端持有上游网易云的共享登录态,一个失控的调用方会把整个 session 池打进熔断 / 封禁——限流保护的是上游账号,按调用方隔离额度。
- 超出额度返回 `RESOURCE_EXHAUSTED`(REST 429),`ErrorInfo.reason = SERVER_RATE_LIMITED`(与上游限流的 `RATE_LIMITED` 区分,见 [错误契约](grpc-errors.md))。
- 健康检查(`grpc.health.v1.Health`、`GET /healthz`)不限流:编排探活被限流会让 LB 误摘流。
- 只限制 kite 服务本身;gRPC reflection 同样计入额度。

## TLS

设置 `KITE_SERVER_TLS_CERT` 与 `KITE_SERVER_TLS_KEY` 后,gRPC 与 REST 两个监听都走 TLS,令牌不再经明文连接传输:

```bash
KITE_SERVER_TLS_CERT=/etc/kite/cert.pem \
KITE_SERVER_TLS_KEY=/etc/kite/key.pem \
KITE_SERVER_AUTH_TOKEN="$(openssl rand -hex 32)" \
kite-server
```

- REST 地址从 `http://` 换成 `https://`;gRPC 客户端把 `grpc.WithTransportCredentials(insecure...)` 换成 `credentials.NewTLS(...)`(证书为系统信任链或自定义 `RootCAs` 时无需跳过验证)。
- gateway 到本进程 gRPC 的内部连接同样走 TLS(回环自拨,跳过证书域名校验)。
- **公开模式(`/player/v1`)不在此覆盖内**:设置 `KITE_PUBLIC_ADDR` 时开启的第三个独立 HTTP 监听(见[下文](#公开只读模式网页播放器))不走 TLS、仍为明文。网页播放器本就要求挂在 https 站点的反向代理之后,对外加密由那一层终结。
- 部署探针 `kite-probe` 走 TLS 部署时需设置:`KITE_PROBE_CACERT=<CA 证书文件>`(私有 CA,隐含开启 TLS,REST 与 gRPC 都用它验证服务端证书)或 `KITE_PROBE_TLS=true`(证书由公共 CA 签发,用系统信任链)。设置了任一变量后,`http://` 写法的探测地址自动按 `https://` 处理。证书需含探针所拨主机名(容器网络内即 `kite`)的 SAN。见 [`docs/deploy.md`](deploy.md)。
- 证书过期前轮换:替换文件后重启服务即可。
- 也可继续放在做 TLS 终结的反向代理 / 服务网格之后,两种方式不冲突。

## mTLS(评估结论:不做)

mTLS(要求每个调用方持客户端证书)能提供传输层的调用方身份,但当前不引入:

- 调用方身份已经由 bearer 令牌承载,且令牌可以随限流 / 审计 / 吊销需求独立轮换;mTLS 在此之上增加的只是「证书级别的身份」,而这层身份对单运营方部署没有额外收益。
- 代价是完整的 PKI 运维:给每个调用方签发 / 分发 / 轮换客户端证书,客户端也要管理证书生命周期。调用方是少数已知项目(sidecar / 受控内网)时,这套成本高于风险。
- 若未来出现多租户、零信任网络或合规要求(需要传输层双向认证),再评估;届时多令牌的「名称:令牌」结构可以平移为证书身份。

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

## 公开只读模式(网页播放器)

设置 `KITE_PUBLIC_ADDR` 时另开一个**不要求令牌**的 HTTP 监听(`/player/v1`),给网页播放器用。
它是上面规则里唯一刻意的例外,因此范围被收得很窄:只有 GET 接口;只返回 `KITE_PUBLIC_PLAYLISTS` /
`KITE_PUBLIC_SONGS` 白名单里的内容(白名单为空则拒绝启动);播放链接带 HMAC 签名并会过期;按来源 IP 限流;
跨域只允许 `KITE_PUBLIC_ORIGINS`。gRPC / REST 的令牌规则不受影响。这个监听**不启用 TLS**
(`KITE_SERVER_TLS_*` 只作用于 gRPC / REST 两个端口,见 [TLS](#tls)),对外暴露时放在 https 站点的
反向代理之后。配置与协议见 [player.md](player.md)。

## 边界(诚实登记)

- **令牌仍可由持有者转交**:多令牌区分的是「配置里登记的调用方」,不阻止一个调用方把令牌泄露给第三方;令牌泄露的处置是吊销重发。
- **限流按进程内计数**:额度存在内存里,重启清零,多副本部署时各副本独立计额(不共享)。当前单副本部署,出现多副本时再引入共享存储。
- **公开模式端口不含 TLS**:`/player/v1` 是独立 HTTP 监听,`KITE_SERVER_TLS_*` 不作用于它(见上),加密由其前置的反向代理终结。
- **mTLS 不做**(评估结论见上),传输层身份仍依赖 TLS + bearer 令牌。
