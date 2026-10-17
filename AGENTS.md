# kite

Go 实现的网易云能力服务 + 命令行工具(单体仓库)。分支 / commit / PR 规范见 [`CONTRIBUTING.md`](CONTRIBUTING.md),本文件只写架构边界与代码约束。

> 写作原则:这里的每条边界要么有测试守护(标注在句末),要么明确标成「约定」。没有守护也没有标注的断言不要加——它迟早与代码脱节。

## 产物与入口

| 入口 | 说明 |
|---|---|
| `cmd/server` | gRPC(`:3722`)+ grpc-gateway REST(`:3721`),默认只绑 `127.0.0.1`,对外监听必须配令牌;SIGINT/SIGTERM 优雅关闭。 |
| `cmd/kite` | CLI。**直连 engine + endpoint 声明,不经 gRPC / service。** |
| `cmd/kite-docs` | 命令树 markdown 生成器(`make docs` 调用)。 |
| `cmd/demo` | 进度条等终端组件的演示脚本,非交付产物、不发布。 |

## 分层

依赖方向只能自上而下。**由 `internal/archtest` 守护**,违规即红。

```
cmd/*
  │
  ├─ 前端 / 适配器(互相平行,不得互相依赖)
  │    internal/cli/**      CLI:cobra 命令树 + kit(渲染 / 进度 / Kit 装配)
  │    internal/tui/**      TUI(规划中,目录尚不存在)
  │    internal/server, internal/service   gRPC 适配器(service 只被 server 引用)
  │
  ├─ 前端共享的本地能力(不得依赖任何前端)
  │    internal/localstate  配置目录路径 / session.json / config.toml / 代理地址解析
  │    internal/recall      召回池(history.jsonl、frecency 打分、预热)
  │
  └─ 核心(不得依赖上面任何一层)
       internal/netease/engine    请求引擎:Execute / ExecuteWithCookie / 重试 / 熔断
       internal/netease/endpoint  API 声明(每接口一个包级 var)
       internal/netease/session   cookie 池接口
       internal/netease/model     数据模型
       errors/  gen/(proto 生成物)
```

### 关键约束

- **共享边界是 engine + endpoint,不是 service。** 三个前端都消费 endpoint 声明;执行语义只在 `engine.Execute` / `engine.ExecuteWithCookie` 定义,**调用方不得拷贝**。决策与取舍见 [ADR](docs/adr/shared-boundary-engine.md)。
- **`internal/service` 是 gRPC adapter**:方法恒一行调 engine,不放业务逻辑,不被 CLI / TUI 引用。
- **CLI 与 TUI 平行不耦合**:两者共用的东西必须先提到 CLI 之外(如 `localstate`、`recall`),再各自引用。禁止 `tui → cli`。
- **`internal/cli/kit` 只做 CLI 呈现**:渲染、进度、Kit 装配。与前端无关的东西(文件路径、会话、配置、召回池)不要放进来。
- **proto 是 RPC 契约的唯一真相**(`proto/netease/music/v1/`)。改 RPC 先改 proto 再 `make proto`;不要手写与 proto 冲突的类型。
- **proto ↔ CLI 一一对应由 `internal/cli/rpc_guard_test.go` 守护**:新增 rpc 要么接 CLI 命令,要么在 `rpcsWithoutCliAllowlist` 登记理由。
- **新域接入节奏**(约定):endpoint 声明 + CLI 直连验证 → proto 建模 → TUI 视图。尚未建模的域(评论 / MV / 排行榜 / 电台 / 网盘 / 签到等)由 issue #34 追踪;当前 RPC 数量以 `proto/` 为准,文档里不写死数字。

### 已知债务(诚实登记)

| 现状 | 影响 | 处理时机 |
|---|---|---|
| `cli/songdl` 依赖 `kit.Kit`(进度渲染、HTTP client) | TUI 无法复用下载核心 | TUI 首次需要下载时,先拆出不含渲染的核心 |
| `redis/`(`internal/cache/redis`、`internal/store/redis`、`internal/infra/redis`)与 `observability/` **已实现但未接入任何二进制** | `cmd/server` 实际用 `cache.Noop` + `session.NoopStore`,指标 / tracing 不生效 | 接入 `cmd/server`,或在确认不用时删除 |
| `internal/service` 无测试 | adapter 的行为只靠 engine 侧测试间接覆盖 | 改动 service 时顺手补 |

## 开发命令

所有代码生成走根 `Makefile`:

- **`make proto`** —— 改 `proto/` 后重新生成到 `gen/`。`gen/` **提交进版本控制**(代码硬依赖它,clone 即可编译),改完 `git add gen/`;CI 校验漂移。决策见 [ADR](docs/adr/generated-code-commit-policy.md)。
- **`make proto-lint`** —— 检查 proto 规范。
- **`make docs`** —— 改命令树后刷新 `docs/cmd/`。`TestDocsFreshness` 依赖它,不跑会红。
- **`make upstream-smoke`** —— 真实上游冒烟(需要外网,手动运行,不进 CI):对网易云真实接口逐个调用服务端 RPC,断言响应非空。**改了 endpoint 路径 / 解码器后必跑**——单元测试的夹具与解码器出自同一个假设,上游改了路径 / 字段名 / 类型都发现不了。
- **`make interop-rust`** —— Rust 客户端互操作冒烟(需要 `cargo`,较慢,CI 的 `Rust Interop` job 跑它)。改 proto、鉴权或错误契约后建议本地跑一次。
- **`make clean`** —— 清本地构建产物(`tmp/`、`bin/`)。不清 `gen/`。

```bash
go build ./cmd/server/ ./cmd/kite/
go test ./...
go vet ./...
make check    # push 前自检:gofmt + go mod tidy + vet + test + lint + proto-lint
```

CI 的约定(唯一门禁 `CI OK`、工具版本来源、发布流程)见 [`CONTRIBUTING.md`](CONTRIBUTING.md)「CI 约定」。

### 环境

- Go 版本**以 `go.mod` 为准**。环境缺 Go 时 `make` 会无提示失败——**不要自行安装 Go**,停下来告知用户。
- 改 proto 需要本地有 `buf`(Makefile 自动探测 `$GOPATH/bin/buf`)。

## 文档

- **`docs/cmd/` 是生成物,不要手改**,改命令树后跑 `make docs`。命令语法唯一真相是 `kite <命令> --help`;其它文档指向 `--help` 或生成参考,**不要复制命令手册**(复制即第二份真相)。
- 上手:[`docs/kite-guide.md`](docs/kite-guide.md) · CLI 设计:[`docs/kite-cli-design.md`](docs/kite-cli-design.md) · 路线图:[`docs/kite-roadmap.md`](docs/kite-roadmap.md)
- 对外契约:[`docs/grpc-errors.md`](docs/grpc-errors.md)(gRPC/REST 错误映射;改 `internal/server/errors.go` 的 reason 必须同步)· [`docs/server-security.md`](docs/server-security.md)(监听地址 / 访问令牌 / 健康检查;改 `internal/server/auth.go` 或 `config` 的 `KITE_SERVER_*` 必须同步)
- 领域术语与决策索引:[`CONTEXT.md`](CONTEXT.md) · ADR:[`docs/adr/`](docs/adr/) · PRD:[`docs/prd/`](docs/prd/)
- 改了架构边界,同步改本文件和 `internal/archtest` 的规则表,二者不一致即视为 bug。

## 提交前(摘要,细则见 CONTRIBUTING.md)

- 从 `main` 开分支,格式 `<type>/<scope>-<简述>`,分支单位是 feature / PRD,不是单个 issue。
- 中文 Conventional Commits,scope 用最内层模块,body 用 bullet 列事实;**本地 commit,不推送**。
- 一个 commit = 一个逻辑变更。
