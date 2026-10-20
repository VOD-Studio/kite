# 网页播放器:自部署与接入

kite 自带一个网页播放器组件 [`@violetjs/kite-player`](../web/player/),和 kite-server 的**公开只读模式**配合使用:站长自己部署 kite,在博客里放一个标签就能播网易云歌单,不依赖任何第三方公共接口。

```
博客页面(静态 HTML 也行)
  <kite-player api="https://music.example.com" playlist="123456"></kite-player>
        │ HTTPS,跨域,不带令牌
  反向代理(负责 TLS)
        │
  kite-server 公开模式(/player/v1)── 白名单、签名播放链接、限流、可选站长 VIP 登录态
```

有自己后端的站点(比如 violet)不需要开公开模式:后端照常用令牌调用 kite 的 gRPC / REST,再实现下文的 `/player/v1` 协议,或者给组件传一个自定义数据源。

## 开启公开模式

公开模式在 kite-server 里,用环境变量开启。它**不要求访问令牌**,所以只暴露白名单里的内容:

| 变量 | 默认 | 说明 |
|---|---|---|
| `KITE_PUBLIC_ADDR` | 空(不开启) | 监听地址,如 `0.0.0.0:3723` |
| `KITE_PUBLIC_PLAYLISTS` | 空 | 允许公开的歌单 ID,逗号分隔。歌单里的歌自动允许 |
| `KITE_PUBLIC_SONGS` | 空 | 允许单独嵌入的歌曲 ID,逗号分隔 |
| `KITE_PUBLIC_ORIGINS` | 空 | 允许跨域调用的站点,如 `https://blog.example.com`;`*` 表示不限 |
| `KITE_PUBLIC_SESSION_FILE` | 空 | 站长网易云登录态文件,配置后 VIP 歌按站长账号播放 |
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

## 站长 VIP 登录态(可选)

不登录时只能播免费歌,VIP 歌在播放器里置灰并说明原因。要播 VIP 歌,用站长自己的 VIP 账号扫码登录一次:

```bash
# 容器里(登录态文件放在挂载卷上)
docker exec -it kite server public-login
# 或指定文件
server public-login --file /data/kite/session.json
```

- 登录态写入 `KITE_PUBLIC_SESSION_FILE`(权限 0600),运行中的 kite-server 30 秒内自动读取,无需重启。
- 每天自动续期一次,新 cookie 写回同一个文件。失效时 `/player/v1/info` 的 `login` 变为 `expired`,VIP 歌回退为不可播,日志提示重新登录。
- 站长的登录态只用于给白名单内容取播放链接,不用于任何其它接口。
- **所有访客都在用站长的账号播放 VIP 歌曲**。访问量大时账号可能被上游判为异常,版权风险也由站长承担。

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

## 协议 `/player/v1`

所有接口都是 `GET`,返回 JSON;ID 一律是字符串。字段只增不改。

| 接口 | 说明 |
|---|---|
| `/player/v1/info` | `{version, login, quality}`;`login` 是 `none` / `active` / `expired` |
| `/player/v1/playlists/{id}` | 歌单 `{id, name, cover, trackCount, tracks: [Track]}` |
| `/player/v1/tracks/{id}` | 单曲 `Track` |
| `/player/v1/tracks/{id}/lyrics` | 歌词 `{lines, chorus, instrumental}` |
| `/player/v1/tracks/{id}/stream?exp=&sig=` | 302 跳转到上游直链(https) |
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

自己有后端的站点实现同样的协议,播放器就能直接接入(白名单、签名和限流按自己的情况实现,带站长登录态时一定要限制可解析的歌曲范围)。
