# kite

English · [简体中文](README.zh-CN.md)

NetEase Cloud Music for your terminal, applications, AI tools, and web pages. Built in Go, with a separately published web player.

## Choose your entry point

| I want to… | Use | Start here |
| --- | --- | --- |
| Search and manage music from a terminal | `kite` CLI; no server required | [CLI](#cli) |
| Call music APIs from an application | `kite-server` with gRPC and REST | [Server](#server) |
| Give an AI client read-only music tools | `kite mcp`, running locally over stdio | [AI clients](#ai-clients) |
| Embed a player on a website | `@violetjs/kite-player`, with Web Components and React bindings | [Web player](#web-player) |

The CLI and server share the same NetEase request engine. The web player is an independent npm package; it connects to the server's public, read-only API or your own backend.

## Quick start

### CLI

Download a binary from [GitHub Releases](https://github.com/VOD-Studio/kite/releases), or install with Go (required version: [`go.mod`](go.mod)):

```sh
go install github.com/VOD-Studio/kite/cmd/kite@latest
kite --help
kite search "周杰伦"
```

Run `kite login` to sign in with a QR code when you need account-specific features. Playback and download availability depend on your NetEase account and the track's rights.

The CLI connects directly to NetEase; you do not need to run `kite-server`. See the [getting-started guide](docs/kite-guide.md). For commands and flags, use `kite <command> --help` or the [generated reference](docs/cmd/).

### Server

Run locally from source:

```sh
git clone https://github.com/VOD-Studio/kite.git
cd kite
go run ./cmd/server
```

In another terminal:

```sh
curl --fail http://127.0.0.1:3721/healthz
# {"status":"SERVING"}
```

| Interface | Default address |
| --- | --- |
| REST | `127.0.0.1:3721` |
| gRPC | `127.0.0.1:3722` |

**Local-only by default. Binding to a non-loopback address requires an access token; otherwise the server refuses to start.** Server access tokens and NetEase login credentials serve different purposes.

Use the [integration guide](docs/integration.md) for authentication, account handling, and client examples. For containers, use the published image `ghcr.io/vod-studio/kite-server` with the configuration in [server security](docs/server-security.md); production rollout is covered in [deployment](docs/deploy.md).

### AI clients

After installing the CLI, register it with Claude Code:

```sh
claude mcp add kite -- kite mcp
```

MCP exposes read-only tools for search, lyrics, playlists, and more. It uses your local CLI login state and cannot modify your music account. Claude Desktop and Cursor configuration: [MCP guide](docs/mcp.md).

### Web player

```sh
npm install @violetjs/kite-player
```

A floating player, track and playlist cards, word-by-word lyrics, and mobile layouts. Use custom elements in any framework, React components and hooks, or the playback core with your own UI.

- [Component setup, screenshots, and API](packages/kite-player/README.md)
- [Public-mode deployment and HTTP contract](docs/player.md)

The browser-facing `/player/v1` API is separate from the authenticated gRPC/REST service. It exposes only allowlisted content, with signed playback links and rate limiting. Never put server access tokens or NetEase cookies in page markup.

## Documentation

Detailed guides and CLI help are currently in Simplified Chinese. Both this overview and the [player README](packages/kite-player/README.md) are available in English and Chinese.

| Topic | Reference |
| --- | --- |
| CLI usage | [Getting started](docs/kite-guide.md) · [Command reference](docs/cmd/) |
| Application integration | [Client guide](docs/integration.md) · [gRPC/REST errors](docs/grpc-errors.md) |
| Server operations | [Security and configuration](docs/server-security.md) · [Production deployment](docs/deploy.md) |
| AI tools | [MCP setup](docs/mcp.md) · [Project index for agents](llms.txt) |
| Web player | [Package documentation](packages/kite-player/README.md) · [Public API](docs/player.md) |
| Contracts and design | [Protobuf definitions](proto/netease/music/v1/) · [Buf Schema Registry](https://buf.build/vod-studio/kite) · [ADRs](docs/adr/) |
| Changes and planned work | [Go changelog](CHANGELOG.md) · [Player changelog](packages/kite-player/CHANGELOG.md) · [Issues](https://github.com/VOD-Studio/kite/issues) |

## Development

Use the Go version in [`go.mod`](go.mod) and the Node.js major version in [`Makefile`](Makefile). From a checkout:

```sh
go build ./cmd/server/ ./cmd/kite/
go test ./...
make player-demo  # Build and open a local, offline player demo server
```

The demo uses fictional songs and simulated playback; it needs neither NetEase credentials nor a running kite server. Open the URL printed by the command.

See [CONTRIBUTING](CONTRIBUTING.md) for the Docker toolchain, code generation, checks, and release process; see [player development](packages/kite-player/DEVELOPMENT.md) for package-specific workflows. Architecture boundaries are documented in [AGENTS.md](AGENTS.md) and enforced by tests.

## License and security

[MIT](LICENSE). For vulnerability reports, follow [SECURITY.md](SECURITY.md) rather than opening a public issue.
