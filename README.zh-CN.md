# kite

[English](README.md) · 简体中文

在终端、应用、AI 工具和网页中使用网易云音乐。核心由 Go 实现，另有独立发布的网页播放器。

## 选择使用方式

| 我想要…… | 使用 | 从这里开始 |
| --- | --- | --- |
| 在终端搜索和管理音乐 | `kite` CLI，无需启动服务 | [命令行](#命令行) |
| 在自己的应用中调用音乐 API | 提供 gRPC 和 REST 的 `kite-server` | [服务](#服务) |
| 给 AI 客户端提供只读音乐工具 | 本地运行、通过 stdio 通信的 `kite mcp` | [AI 客户端](#ai-客户端) |
| 在网站中嵌入播放器 | 提供 Web Components 和 React 绑定的 `@violetjs/kite-player` | [网页播放器](#网页播放器) |

CLI 与服务共用同一套网易云请求引擎。网页播放器是独立的 npm 包，连接服务的公开只读 API，也可以接入你自己的后端。

## 快速上手

### 命令行

从 [GitHub Releases](https://github.com/VOD-Studio/kite/releases) 下载二进制，或使用 Go 安装（版本要求见 [`go.mod`](go.mod)）：

```sh
go install github.com/VOD-Studio/kite/cmd/kite@latest
kite --help
kite search "周杰伦"
```

需要账号相关功能时，运行 `kite login` 扫码登录。能否播放或下载取决于网易云账号权益和歌曲版权。

CLI 直接请求网易云，不需要启动 `kite-server`。完整上手流程见 [使用手册](docs/kite-guide.md)；命令和参数以 `kite <命令> --help` 或 [生成参考](docs/cmd/) 为准。

### 服务

从源码在本机启动：

```sh
git clone https://github.com/VOD-Studio/kite.git
cd kite
go run ./cmd/server
```

在另一个终端检查：

```sh
curl --fail http://127.0.0.1:3721/healthz
# {"status":"SERVING"}
```

| 接口 | 默认地址 |
| --- | --- |
| REST | `127.0.0.1:3721` |
| gRPC | `127.0.0.1:3722` |

**默认只允许本机访问；监听非回环地址时必须配置访问令牌，否则服务拒绝启动。** 服务访问令牌与网易云登录凭据用途不同，不能互相替代。

鉴权、账号管理和客户端示例见 [接入指南](docs/integration.md)。容器可使用公开镜像 `ghcr.io/vod-studio/kite-server`，配置见 [服务安全](docs/server-security.md)；生产上线流程见 [部署指南](docs/deploy.md)。

### AI 客户端

安装 CLI 后，在 Claude Code 中注册：

```sh
claude mcp add kite -- kite mcp
```

MCP 提供搜索、歌词、歌单等只读工具，使用本机 CLI 的登录态，不能修改音乐账号。Claude Desktop 和 Cursor 的配置见 [MCP 指南](docs/mcp.md)。

### 网页播放器

```sh
npm install @violetjs/kite-player
```

提供悬浮播放器、单曲与歌单卡片、逐字歌词和手机布局。可以在任意框架中使用自定义元素，也可以使用 React 组件与 hooks，或只用播放核心搭建自己的界面。

- [组件接入、截图与 API](packages/kite-player/README.zh-CN.md)
- [公开模式部署与 HTTP 协议](docs/player.md)

浏览器使用的 `/player/v1` 独立于需要鉴权的 gRPC/REST 服务，只开放白名单内容，并使用签名播放链接和限流。不要把服务访问令牌或网易云 Cookie 放进网页。

## 文档导航

详细指南和 CLI 帮助目前以简体中文提供。本项目概览和 [播放器 README](packages/kite-player/README.zh-CN.md) 均有中英文版本。

| 主题 | 参考 |
| --- | --- |
| CLI 使用 | [上手手册](docs/kite-guide.md) · [命令参考](docs/cmd/) |
| 应用接入 | [接入指南](docs/integration.md) · [gRPC/REST 错误契约](docs/grpc-errors.md) |
| 服务运维 | [安全与配置](docs/server-security.md) · [生产部署](docs/deploy.md) |
| AI 工具 | [MCP 配置](docs/mcp.md) · [AI 项目索引](llms.txt) |
| 网页播放器 | [组件说明](packages/kite-player/README.zh-CN.md) · [公开 API](docs/player.md) |
| 契约与设计 | [Protobuf 定义](proto/netease/music/v1/) · [Buf Schema Registry](https://buf.build/vod-studio/kite) · [ADR](docs/adr/) |
| 变更与计划 | [Go 变更记录](CHANGELOG.md) · [播放器变更记录](packages/kite-player/CHANGELOG.md) · [Issues](https://github.com/VOD-Studio/kite/issues) |

## 开发

Go 版本以 [`go.mod`](go.mod) 为准，Node.js 大版本以 [`Makefile`](Makefile) 为准。在仓库中执行：

```sh
go build ./cmd/server/ ./cmd/kite/
go test ./...
make player-demo  # 构建播放器并启动本地离线演示服务
```

演示使用虚构歌曲和模拟播放，不需要网易云登录态或运行中的 kite 服务。打开命令输出的地址即可。

Docker 工具链、代码生成、检查和发布流程见 [贡献指南](CONTRIBUTING.md)；包内工作流见 [播放器开发说明](packages/kite-player/DEVELOPMENT.md)。架构边界记录在 [AGENTS.md](AGENTS.md)，由测试守护。

## 许可证与安全

[MIT](LICENSE)。报告安全漏洞请遵循 [SECURITY.md](SECURITY.md)，不要提交公开 issue。
