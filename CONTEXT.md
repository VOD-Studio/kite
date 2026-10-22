# kite 项目上下文

> 领域术语表 + 不可逆决策的索引。实现细节不写在这里(那是 ADR 和代码的事)。
> 维护规则:术语在访谈中 resolved 时立即更新;ADR 只在「难反悔 + 无背景会困惑 + 真实取舍」三者齐备时才建。

## 领域术语

| 术语 | 定义 |
|---|---|
| **Go 产品** | `kite` CLI 与 `kite-server` 服务共同组成的发布单元,共享核心和同一版本。两种交付物不等于两个独立版本。 |
| **player 产品** | 网页播放器 `@violetjs/kite-player`,是独立于 Go 产品的发布单元;双方通过公开播放器协议协作,不要求版本号相等。 |
| **gen/** | `buf generate` 从 `proto/` 产出的 Go stub/gRPC service/gateway/OpenAPI。被项目代码 `import` **硬依赖**(不生成则 vet/test 红)。性质同 `docs/cmd/`(生成产物且已提交)。 |
| **release PR** | release-please 为指定发布单元推导版本和变更记录的 PR。Squash 合并表示批准该产品发版;只有 Go 产品的公开发布进入服务部署,player 发布不部署 server。业务 PR 合并本身不是发布批准。 |
| **部署器** | 管理员验签后安装的宿主机特权程序,与应用镜像独立升级。CI 只有受限应用部署权限;普通 release 不能更新部署器。能力版本定义兼容性,内容摘要标识实际安装构建。 |
| **配置(config.toml)** | 用户偏好的持久化,只收「已存在 flag 或硬编码默认值」的 key,不造新语义。与**状态文件**(session.json / history.jsonl,程序运行产生)同住配置目录但性质不同:配置可删可重建,状态不可。 |

## 不可逆/高代价决策(ADR 候选)

| 决策 | 选择 | 日期 | ADR |
|---|---|---|---|
| LICENSE | MIT | 2026-07-27 | (无需 ADR,可逆且无惊奇) |
| gen/ 是否进版本控制 | **提交** + CI 守护 job | 2026-07-27 | 需 ADR(难反悔 + 有真实取舍) |
| 发版工作流 | release-please 为 Go 与 player 分别生成版本和 release PR;GoReleaser 发布 Go 产物,npm 发布已验收的 player tarball | 2026-10-22 | 约束与恢复见 [CONTRIBUTING](CONTRIBUTING.md#发布与恢复) |
| 首版本号 | v0.1.0 | 2026-07-27 | (无需 ADR) |
| 镜像构建 | GoReleaser `dockers_v2` + 极简 Dockerfile(COPY 二进制,不重编译)+ distroless static-debian12 | 2026-07-27 | 需 ADR(老 dockers 弃用 + 双 Dockerfile 模式需背景) |
| 镜像仓库 | ghcr.io | 2026-07-27 | (无需 ADR,GitHub 项目首选无争议) |
| lint 严格度 | golangci-lint v2 中度(默认 6 + gosec/revive/bodyclose/misspell/unconvert/contextcheck/nilerr/gocritic),`only-new-issues: true` 避免历史债阻塞 | 2026-07-27 | (无需 ADR,可调) |
| 发布产物范围 | Go 产品包含 CLI / server 档案及服务镜像;`kite-probe` 仅随镜像和运维安装包分发。player 另行发布 npm 包;`kite-docs`、`demo` 不发布 | 2026-10-22 | Go 镜像见 [release-image-strategy](docs/adr/release-image-strategy.md) |
| CHANGELOG 位置 | Go 在根 `CHANGELOG.md`,player 在 `packages/kite-player/CHANGELOG.md`;独立版本和 tag,不改写已公开历史 | 2026-10-22 | 见 [CONTRIBUTING](CONTRIBUTING.md#发布与恢复) |
| config 优先级 | `--json` > 非TTY自动JSON > config.toml > 内置默认;管道 JSON 契约**不被 config 覆盖**(config 是机器本地偏好,契约是全局的) | 2026-08-16 | (无需 ADR,Phase A 脚本契约的自然延伸) |
| config get 输出 | 单 key 裸打**生效值**(不带格式,可在脚本中使用,gh/jj/git 惯例);无参列表带「来源」列;未知 key 报错退非零(key 是固定枚举,拼错要早暴露) | 2026-08-16 | (无需 ADR,业界惯例) |
| config 校验原则 | 所有校验在 `set` 写盘前完成;`workers` 上限 5(与 `--workers` flag 契约一致);`download_dir` 只存不校验存在性 | 2026-08-16 | (无需 ADR) |
| config 文件机制 | 原子写;文件不存在=正常态;**坏文件=硬错误**拒绝执行(静默回退会掩盖用户设置,doctor 与 config path 豁免保修复闭环);读取侧复用 set 同一套校验 | 2026-08-16 | (无需 ADR,git 同款行为;机制细节见 PRD-0017) |
| config 周边集成 | doctor 加 config 检查项;config 命令组进 help「工具」分组;**不进 onboarding 场景推送**(带问题来找的功能,主动推是噪音) | 2026-08-16 | (无需 ADR) |
| proxy 优先级链 | `--proxy` > config `proxy` > 环境变量 > 直连;显式设置**整体替换**环境变量层(不混合态);不做显式禁用哨兵(禁用在 shell 层 `env -u` 解决) | 2026-08-16 | (无需 ADR,git `http.proxy` 同款覆盖语义;详见 PRD-0018) |
| proxy 范围与防坑 | 协议白名单 http/https/socks5,scheme 必须显式;`config set proxy ""` = **清除 key**(空值有真实语义,区别于 download_dir);**API 与音频下载必须同走一个代理决策**(下载不经 engine,只改一侧必分裂) | 2026-08-16 | (无需 ADR;机制见 PRD-0018) |
| 前端共享边界 | engine + endpoint(不是 service);本地能力放 `localstate` / `recall`;依赖方向由 `internal/archtest` 守护 | 2026-10-17 | [shared-boundary-engine](docs/adr/shared-boundary-engine.md) |
| 治理文件 | 本次含 SECURITY.md + CONTRIBUTING.md(原 P3 提前) | 2026-07-27 | (无需 ADR) |

## 暂缓项(后续 phase)

| 议题 | 决定 | 原因 |
|---|---|---|
| Homebrew tap | 暂缓 | CLI 还在 v0.1,等稳定再上 brew;需单独建 tap 仓库 |
| distroless 替换现有 Dockerfile | 现有 Dockerfile 保留给本地开发,只新增发布用 Dockerfile | 不破坏现有 `docker build` 工作流 |
