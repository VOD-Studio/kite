# kite 项目上下文

> 领域术语表 + 不可逆决策的索引。实现细节不写在这里(那是 ADR 和代码的事)。
> 维护规则:术语在访谈中 resolved 时立即更新;ADR 只在「难反悔 + 无背景会困惑 + 真实取舍」三者齐备时才建。

## 领域术语

| 术语 | 定义 |
|---|---|
| **双产物** | kite 的两个交付物:`server`(gRPC + grpc-gateway 服务)和 `kite`(直连 engine 的 CLI)。共享 `internal/netease/` 核心与 `internal/service/`。 |
| **gen/** | `buf generate` 从 `proto/` 产出的 Go stub/gRPC service/gateway/OpenAPI。被项目代码 `import` **硬依赖**(不生成则 vet/test 红)。性质同 `docs/cmd/`(生成产物且已提交)。 |
| **release PR** | release-please 从 conventional commits 推导版本号后自动开的 PR,含待发布 CHANGELOG。合并即触发打 tag → GoReleaser 构建。 |
| **配置(config.toml)** | 用户偏好的持久化,只收「已存在 flag 或硬编码默认值」的 key,不造新语义。与**状态文件**(session.json / history.jsonl,程序运行产生)同住配置目录但性质不同:配置可删可重建,状态不可。 |

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
| config 优先级 | `--json` > 非TTY自动JSON > config.toml > 内置默认;管道 JSON 契约**不被 config 覆盖**(config 是机器本地偏好,契约是全局的) | 2026-08-16 | (无需 ADR,Phase A 脚本契约的自然延伸) |
| config get 输出 | 单 key 裸打**生效值**(不带格式,可在脚本中使用,gh/jj/git 惯例);无参列表带「来源」列;未知 key 报错退非零(key 是固定枚举,拼错要早暴露) | 2026-08-16 | (无需 ADR,业界惯例) |
| config 校验原则 | 所有校验在 `set` 写盘前完成;`concurrency` 上限 16;`download_dir` 只存不校验存在性 | 2026-08-16 | (无需 ADR) |
| config 文件机制 | tmp+rename 原子写、0600/0700、不保留注释(整文件重写);文件不存在=正常态;**坏文件(解析失败/key 越界)=硬错误**拒绝执行(静默回退会掩盖用户设置);读取侧复用 set 同一套校验函数 | 2026-08-16 | (无需 ADR,git 同款行为) |
| config 周边集成 | doctor 加 config 检查项;config 命令组进 help「工具」分组;**不进 onboarding 场景推送**(带问题来找的功能,主动推是噪音) | 2026-08-16 | (无需 ADR) |
| 治理文件 | 本次含 SECURITY.md + CONTRIBUTING.md(原 P3 提前) | 2026-07-27 | (无需 ADR) |

## 暂缓项(后续 phase)

| 议题 | 决定 | 原因 |
|---|---|---|
| Homebrew tap | 暂缓 | CLI 还在 v0.1,等稳定再上 brew;需单独建 tap 仓库 |
| HTTP 代理(proxy) | config v1 之后单独排期 | 需改造 engine HTTP client,是独立 feature(`--proxy` flag + config key + engine 三处联动),不裹进 config 第一版「纯默认值」边界 |
| distroless 替换现有 Dockerfile | 现有 Dockerfile 保留给本地开发,只新增发布用 Dockerfile | 不破坏现有 `docker build` 工作流 |
