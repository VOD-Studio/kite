# 网页播放器:自部署与接入

kite 自带一个网页播放器组件 [`@violetjs/kite-player`](../packages/kite-player/),和 kite-server 的**公开只读模式**配合使用:自己部署 kite,在任意网页里放几个标签就能播网易云歌单,不依赖任何第三方公共接口。

```
网页(静态 HTML 也行)
  <kite-player api="https://music.example.com" playlists="123456"></kite-player>
        │ HTTPS,跨域,不带令牌
  反向代理(负责 TLS)
        │
  kite-server 公开模式(/player/v1)── 白名单、签名播放链接、限流、可选的服务账号 VIP 登录态
```

有自己后端的站点(比如 violet)不需要开公开模式:后端照常用令牌调用 kite 的 gRPC / REST,再实现下文的 `/player/v1` 协议,或者给组件传一个[自定义数据源](../packages/kite-player/README.md#自定义数据来源)。

## 开启公开模式

公开模式在 kite-server 里,用环境变量开启。它**不要求访问令牌**,所以只暴露白名单里的内容:

| 变量 | 默认 | 说明 |
|---|---|---|
| `KITE_PUBLIC_ADDR` | 空(不开启) | 监听地址,如 `0.0.0.0:3723` |
| `KITE_PUBLIC_PLAYLISTS` | 空 | 允许公开的歌单 ID,逗号分隔。歌单里的歌自动允许 |
| `KITE_PUBLIC_SONGS` | 空 | 允许单独嵌入的歌曲 ID,逗号分隔 |
| `KITE_PUBLIC_ORIGINS` | 空 | 允许跨域调用的站点,如 `https://blog.example.com`;`*` 表示不限 |
| `KITE_PUBLIC_SESSION_FILE` | 空 | 单账号部署的网易云登录态文件;与 `KITE_PUBLIC_ACCOUNTS` 互斥 |
| `KITE_PUBLIC_ACCOUNTS` | 空 | 多账号部署的有序 JSON 数组:`[{"id":"primary","sessionFile":"/data/primary.json"},{"id":"secondary","sessionFile":"/data/secondary.json"}]` |
| `KITE_PUBLIC_SIGNING_KEY_FILE` | 空(每次启动随机) | 播放链接签名密钥文件,至少 16 字节 |
| `KITE_PUBLIC_RATE_PER_MINUTE` | `120` | 每个来源 IP 每分钟的请求数 |
| `KITE_PUBLIC_TRUST_PROXY` | `false` | 在反向代理之后时设为 `true`,按 `X-Forwarded-For` 限流 |
| `KITE_PUBLIC_QUALITY` | `exhigh` | 音质上限:`standard` / `exhigh` / `lossless` / `hires` |

规则:

- 设置了 `KITE_PUBLIC_ADDR` 但歌单和歌曲白名单都为空 → **拒绝启动**。任何一项格式不对也拒绝启动。
- 白名单之外的歌单、歌曲、歌词一律 `404 NOT_ALLOWED`。
- 带 `Origin` 头但不在 `KITE_PUBLIC_ORIGINS` 里的请求返回 `403 ORIGIN_NOT_ALLOWED`,防止别的网页盗用你的额度。`<audio>` 加载播放链接不带 `Origin`,不受影响。Origin 白名单挡不住脚本直接请求,挡脚本靠的是签名和限流。
- 播放链接带 HMAC 签名,6 小时后过期。不出现在白名单响应里的歌曲 ID 换不到上游直链。
- 公开模式不内置 TLS。HTTPS 站点必须经反向代理(如 Caddy)提供 HTTPS,否则浏览器会拦截混合内容。

## 服务账号的 VIP 登录态(可选)

服务账号由部署者配置,只用于为白名单内容取播放链接及维护自身登录态,不用于歌单、歌曲、歌词等读取。与 gRPC / REST 的通用托管账号集合隔离,不自动借用其它调用方的账号;听众不需要、也不能在公开接口登录自己的网易云账号。

### 配置与登录

**单账号部署**保留 `KITE_PUBLIC_SESSION_FILE=/data/session.json`,然后执行 `server public-login`。也可用 `server public-login --file /data/session.json` 显式写入指定文件。

**多账号部署**改用以下配置,不要同时设置 `KITE_PUBLIC_SESSION_FILE`:

```bash
export KITE_PUBLIC_ACCOUNTS='[{"id":"primary","sessionFile":"/data/primary.json"},{"id":"secondary","sessionFile":"/data/secondary.json"}]'
server public-login --account primary
server public-login --account secondary
```

容器里通过 `docker exec -it kite server public-login --account primary` 操作,每次扫码使用对应的部署者账号。参数详情以 `server public-login --help` 为准。

- 数组顺序决定轮询顺序。`id` 为 1–64 个字母、数字、下划线或连字符,首字符必须为字母或数字;ID 不得重复。`sessionFile` 必须是非空绝对路径,各账号不得使用同一路径(包含已有符号链接别名)。
- 无效 JSON、未知字段、空数组、重复账号/路径、两种账号配置并用都拒绝启动。未设置任何账号配置时按匿名播放。
- 多账号配置下登录必须用 `--account ID` 或显式 `--file 路径`;`--account` 只能选择已配置 ID,不能与 `--file` 同用。`--file` 只指定本地写入目标,不会把该文件自动加入运行中的公开服务。
- 登录态文件权限为 `0600`,放在持久卷中。重新扫码会原子替换对应文件,运行中的服务在 30 秒内读取,无需重启。账号列表或顺序变更需要更新配置并重启服务。
- 每个账号独立读取、每天独立续期一次,启动时对已超过一天的凭据先续期;新 Cookie 只写回该账号自己的文件。某个账号续期失败不会阻塞其它账号。
- 上游明确拒绝登录态时,仅该请求实际使用的凭据版本标为失效;旧播放/续期响应不会把较新的登录态失效或覆盖。失效文件不会因重复读取而重新激活,需重新扫码更新。日志包含文件路径,不包含 Cookie。

### 播放选择、权益与匿名行为

- 每个通过签名校验的播放请求按配置顺序轮询**当前可用**的账号,跳过缺失、不可读、无效或已失效的登录态。轮询范围是单个公开服务实例,不是按听众、IP、歌单或歌曲固定绑定;并发请求的顺序由服务端串行选择决定。
- 一次请求只使用一个账号。无歌曲权益/版权限制时返回 `404 UNPLAYABLE`;上游仅给试听时原样跳转并带 `X-Kite-Trial`;上游限流、网络等错误沿用 `502 UPSTREAM_ERROR`。这些结果都**不换号重试**,也不把无权益或限流当作账号失效。
- 上游明确报告选中账号登录态失效时,本次返回 `404 UNPLAYABLE`,不重新选择账号或匿名重试;后续请求跳过该失效版本。
- 没有可用账号时,只发送匿名请求,不携带任何部署者 Cookie,也不借用引擎会话池。免费歌曲仍可播放;VIP 曲目的 `playable` 预估为 `false`,实际播放仍以上游是否返回链接/试听为准。匿名也无链接则返回 `404 UNPLAYABLE`。
- `/player/v1/info.login` 保留 `none` / `active` / `expired`:至少一个可用账号为 `active`;没有可用账号且至少一个已失效为 `expired`;其它情况(未配置或文件均不可读/无有效凭据)为 `none`。它是聚合状态,不公布账号 ID、数量、昵称或 Cookie,`active` 不保证任何账号拥有歌曲权益。
- 播放链接签名仍绑定歌曲和过期时间,不绑定账号;公开请求中的账号 ID、Cookie 或通用托管账号请求头不能控制账号选择。播放直链缓存按实际凭据摘要、歌曲及音质隔离,凭据改变后不会复用旧账号的权益结果。播放跳转、状态和含 `playable` 的响应使用 `Cache-Control: no-store`,不让浏览器/CDN 绕过本次选择或复用旧状态;匿名元数据仍可在服务内共享缓存。

**多账号不是绕过上游风控、访问限额或版权限制的机制。** 所有听众仍共享这些部署者账号及其权益,访问量大时账号可能被判异常;多账号不保证降低此风险,版权与账号风险由部署者承担。

## 部署示例(Docker Compose + Caddy)

```yaml
services:
  kite:
    image: ghcr.io/vod-studio/kite-server:latest
    restart: unless-stopped
    environment:
      KITE_SERVER_AUTH_TOKEN_FILE: /run/secrets/kite_token   # gRPC / REST 仍需令牌
      KITE_PUBLIC_ADDR: 0.0.0.0:3723
      KITE_PUBLIC_PLAYLISTS: "123456,234567"
      KITE_PUBLIC_ORIGINS: https://blog.example.com
      KITE_PUBLIC_SESSION_FILE: /data/session.json
      KITE_PUBLIC_SIGNING_KEY_FILE: /data/signing.key
      KITE_PUBLIC_TRUST_PROXY: "true"
    volumes:
      - kite-data:/data
    secrets: [kite_token]

  caddy:
    image: caddy:2
    restart: unless-stopped
    ports: ["80:80", "443:443"]
    command: caddy reverse-proxy --from music.example.com --to kite:3723

volumes:
  kite-data:
secrets:
  kite_token:
    file: ./kite_token
```

签名密钥用 `openssl rand -hex 32 > signing.key` 生成后放进卷里;不配置时每次重启随机生成,旧的播放链接会失效,播放器会自动重新获取。

## 在页面里使用

```html
<script src="https://cdn.jsdelivr.net/npm/@violetjs/kite-player@0.1/dist/kite-player.iife.js"></script>

<!-- 每个页面一个(放在公共布局里);playlists 是播放列表里列出的歌单 -->
<kite-player api="https://music.example.com" playlists="123456,234567"></kite-player>

<!-- 页面任意位置:单曲卡片 / 歌单卡片 -->
<kite-track song="1973665667"></kite-track>
<kite-playlist playlist="123456"></kite-playlist>
```

- `api` 填反向代理对外的 HTTPS 地址;页面所在的站点要在 `KITE_PUBLIC_ORIGINS` 里。
- 卡片里的歌必须在白名单内:属于白名单歌单,或单独列在 `KITE_PUBLIC_SONGS`。
- 元素属性、卡片尺寸、主题与样式变量、React 组件与 hooks、自定义数据源,以及在其它框架(如 Rust 的 Dioxus)里使用,见[组件说明](../packages/kite-player/README.zh-CN.md)。

## 协议 `/player/v1`

所有接口都是 `GET`,返回 JSON;ID 一律是字符串。字段只增不改。

| 接口 | 说明 |
|---|---|
| `/player/v1/info` | `{version, login, quality}`;`login` 是所有已配置服务账号的聚合状态:`none` / `active` / `expired`,规则见上文 |
| `/player/v1/playlists/{id}` | 歌单 `{id, name, cover, trackCount, tracks: [Track]}`;`tracks` 按歌单顺序,最多 1000 首 |
| `/player/v1/tracks/{id}` | 单曲 `Track` |
| `/player/v1/tracks/{id}/lyrics` | 歌词 `{lines, chorus, instrumental}` |
| `/player/v1/tracks/{id}/stream?exp=&sig=` | 302 跳转到上游直链(https);服务账号对该歌无权益、上游只给试听片段时,响应带 `X-Kite-Trial: start=0; end=30000`(毫秒) |
| `/healthz` | 健康检查,不限流 |

`Track`:

```json
{
  "id": "1973665667",
  "title": "海屿你",
  "artists": [{ "id": "12138269", "name": "马也_Crabbit" }],
  "album": { "id": "149437227", "name": "海屿你", "cover": "https://p1.music.126.net/….jpg" },
  "durationMs": 230000,
  "access": "free",
  "playable": true,
  "stream": "/player/v1/tracks/1973665667/stream?exp=1792536000&sig=…",
  "lyrics": "/player/v1/tracks/1973665667/lyrics"
}
```

- `access`:`free` / `vip` / `paid`(数字专辑)。`playable` 是按站点当前登录态推断的结果,最终以播放链接为准。
- `stream` 是相对本服务的地址,直接作为 `<audio src>`;上游直链几十分钟后过期,播放中断时重新请求 `stream` 即可。
- `stream` 的 302 响应在**试听片段**时带 `X-Kite-Trial` 头(`start=0; end=30000`,毫秒):站点账号对该歌无权益时上游只给约 30 秒的片段,自建后端可据此区分完整版;完整版没有这个头。`<audio>` 不读响应头,组件本身不消费它。

歌词 `lines` 已经合并好原文、翻译、音译和逐字时间(毫秒):

```json
{
  "lines": [
    {
      "time": 13010, "duration": 4780, "text": "从不 主动 示弱",
      "translation": "",
      "words": [{ "time": 13010, "duration": 520, "text": "从" }]
    }
  ],
  "chorus": [{ "start": 63965, "end": 111475 }],
  "instrumental": false
}
```

错误统一是 `{"error": {"reason": "…", "message": "…"}}`,按 `reason` 分支,`message` 可直接展示给读者:

| reason | HTTP | 含义 |
|---|---|---|
| `NOT_ALLOWED` | 404 | 不在站点白名单里 |
| `NOT_FOUND` | 404 | 上游没有这个资源 |
| `UNPLAYABLE` | 404 | 当前无法播放(VIP 未登录、版权限制) |
| `LINK_EXPIRED` | 403 | 播放链接过期,重新获取歌单 / 歌曲 |
| `LINK_INVALID` | 403 | 播放链接签名无效 |
| `ORIGIN_NOT_ALLOWED` | 403 | 调用页面不在 `KITE_PUBLIC_ORIGINS` 里 |
| `INVALID_ID` | 400 | ID 不是正整数 |
| `RATE_LIMITED` | 429 | 请求太频繁,带 `Retry-After` |
| `UPSTREAM_ERROR` | 502 | 上游暂时不可用 |

有自己后端的应用实现同样的协议,播放器就能直接接入(白名单、签名和限流按自己的情况实现,带服务账号登录态时一定要限制可解析的歌曲范围)。
