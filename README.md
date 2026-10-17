# kite

Go 实现的网易云能力服务 + 命令行工具。

两个产物共享同一套网易云核心(`internal/netease/`):一个 gRPC + grpc-gateway 服务,一个直连 engine 的 CLI。

## 产物

### 服务 (`cmd/server`)

gRPC(强类型 RPC,默认 `127.0.0.1:3722`)+ grpc-gateway(REST 暴露,默认 `127.0.0.1:3721`)双 server,收到 SIGINT/SIGTERM 优雅关闭。

**默认只监听本机回环;对外监听必须配置访问令牌,否则拒绝启动。** 地址、令牌、健康检查、Docker 用法见 [服务部署与访问控制](docs/server-security.md)。

出网代理:server 的 engine 走默认 transport,尊重标准环境变量 `HTTPS_PROXY` / `HTTP_PROXY`(无需额外配置,PRD-0018)。

```bash
go run ./cmd/server      # 直接运行(仅本机可访问,无需令牌)
air                      # 热重载开发(.air.toml)
docker build -t kite .   # 容器镜像(仅 Dockerfile,无 compose);运行时必须设置 KITE_SERVER_AUTH_TOKEN
```

```bash
curl localhost:3721/healthz
# {"status":"SERVING"}
```

### CLI (`cmd/kite`)

直连 engine + endpoint 声明,**不经 gRPC**。网易云 9 域已全接入:登录(`auth`)、歌曲(`song`)、专辑(`album`)、歌手(`artist`)、歌单(`playlist`)、用户(`user`)、搜索(`search`)、推荐(`recommend`)、私人 FM(`fm`)。

```bash
go install github.com/VOD-Studio/kite/cmd/kite@latest
kite login               # 扫码登录
kite search 周杰伦        # 搜索(位置参数 ≡ --keyword)
kite song download 347230 --level 3   # 下载单曲(1标准 2较高 3无损 4Hi-Res)
```

上手流程见 [使用手册](docs/kite-guide.md);全命令参数查 `kite <命令> --help` 或 [生成参考](docs/cmd/)。

## 架构

共享 `internal/netease/` 核心(engine 请求引擎 + endpoint 接口声明),三个前端平行消费:

- **gRPC / REST 服务**(`cmd/server`):`internal/service` 是 gRPC adapter。
- **CLI**(`internal/cli/`):直连 engine,输出导向(表格 / JSON),按领域分包,`kit/` 是渲染层。
- **TUI**(`internal/tui/`,规划中):bubbletea v2 完整客户端,与 CLI 平行不耦合。

前端共享的本地能力(会话 / 配置 / 召回池)在 `internal/localstate`、`internal/recall`。分层规则见 [AGENTS.md](AGENTS.md),由测试守护。

gRPC 契约的唯一真相在 [`proto/netease/music/v1/`](proto/netease/music/v1/)。错误以标准 gRPC status + `ErrorInfo` 返回,映射与各语言客户端的处理方式见 [错误契约](docs/grpc-errors.md)。TUI 架构决策见 [ADR](docs/adr/tui-client-architecture.md)。

## 开发

所有代码生成走根 `Makefile`:

```bash
make proto       # 改 proto 后重新生成 Go stub / gateway / OpenAPI 到 gen/
make proto-lint  # 检查 proto 规范
make docs        # 改命令树后重新生成 docs/cmd/(freshness 守护测试依赖此)
make clean       # 清除生成产物
```

```bash
go build ./cmd/server/    # 编译服务
go build ./cmd/kite/      # 编译 CLI
go test ./...             # 测试
go vet ./...              # 检查
```

Go 版本以 `go.mod` 为准。若环境缺 Go,`make` 无提示直接失败。

## 技术栈

- Go + gRPC + grpc-gateway(契约优先,proto 定义 RPC)
- bubbletea v2 / lipgloss v2(TUI)、cobra(CLI 命令树)
- `observability/` 包提供 OTel / Prometheus / 日志脱敏(已实现,尚未接入 `cmd/server`)
- Redis 缓存与会话存储(当前 noop,后续接入)
- 网易云 weapi/eapi 加密自实现(Go 标准库,不依赖第三方音乐库)

## 文档

- [使用手册](docs/kite-guide.md) —— 上手流程(安装 / 登录 / 常用操作)
- [CLI 设计](docs/kite-cli-design.md) —— 架构决策与输出层设计
- [功能路线图](docs/kite-roadmap.md) —— 各 Phase 状态与规划
- [ADR](docs/adr/) —— 架构决策记录
- [全命令参考](docs/cmd/) —— 由命令树生成,与 `--help` 同步
