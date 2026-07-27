# kite 项目上下文

> 领域术语表 + 不可逆决策的索引。实现细节不写在这里(那是 ADR 和代码的事)。
> 维护规则:术语在访谈中 resolved 时立即更新;ADR 只在「难反悔 + 无背景会困惑 + 真实取舍」三者齐备时才建。

## 领域术语

| 术语 | 定义 |
|---|---|
| **双产物** | kite 的两个交付物:`server`(gRPC + grpc-gateway 服务)和 `kite`(直连 engine 的 CLI)。共享 `internal/netease/` 核心与 `internal/service/`。 |
| **gen/** | `buf generate` 从 `proto/` 产出的 Go stub/gRPC service/gateway/OpenAPI。被项目代码 `import` **硬依赖**(不生成则 vet/test 红)。性质同 `docs/cmd/`(生成产物且已提交)。 |
| **release PR** | release-please 从 conventional commits 推导版本号后自动开的 PR,含待发布 CHANGELOG。合并即触发打 tag → GoReleaser 构建。 |

## 不可逆/高代价决策(ADR 候选)

| 决策 | 选择 | 日期 | ADR |
|---|---|---|---|
| LICENSE | MIT | 2026-07-27 | (无需 ADR,可逆且无惊奇) |
| gen/ 是否进版本控制 | **提交** + CI 守护 job | 2026-07-27 | 需 ADR(难反悔 + 有真实取舍) |
| 发版工作流 | release-please(版本号/changelog) + GoReleaser(构建/发布) | 2026-07-27 | (组合是 2026 标准,无惊奇) |
| 首版本号 | v0.1.0 | 2026-07-27 | (无需 ADR) |
| 镜像构建 | GoReleaser `dockers_v2` + 极简 Dockerfile(COPY 二进制,不重编译)+ distroless static-debian12 | 2026-07-27 | 需 ADR(老 dockers 弃用 + 双 Dockerfile 模式需背景) |
| 镜像仓库 | ghcr.io | 2026-07-27 | (无需 ADR,GitHub 项目首选无争议) |
| lint 严格度 | golangci-lint v2 中度(默认 6 + gosec/revive/bodyclose/misspell/unconvert/contextcheck/nilerr/gocritic),`only-new-issues: true` 避免历史债阻塞 | 2026-07-27 | (无需 ADR,可调) |
| 发布产物范围 | 只 `cmd/server` + `cmd/kite`;`cmd/kite-docs`(开发工具)、`cmd/demo`(非交付)不发 | 2026-07-27 | (无需 ADR,AGENTS.md 已明确) |
| CHANGELOG 位置 | 仓库根目录 `CHANGELOG.md` | 2026-07-27 | (无需 ADR,行业约定) |
| 治理文件 | 本次含 SECURITY.md + CONTRIBUTING.md(原 P3 提前) | 2026-07-27 | (无需 ADR) |

## 暂缓项(后续 phase)

| 议题 | 决定 | 原因 |
|---|---|---|
| Homebrew tap | 暂缓 | CLI 还在 v0.1,等稳定再上 brew;需单独建 tap 仓库 |
| distroless 替换现有 Dockerfile | 现有 Dockerfile 保留给本地开发,只新增发布用 Dockerfile | 不破坏现有 `docker build` 工作流 |
