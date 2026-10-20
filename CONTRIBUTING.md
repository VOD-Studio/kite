# 贡献指南

感谢你有兴趣为 kite 贡献!本文档是**分支、commit、PR 规范的权威来源**(人和 agent 共用)。架构边界与代码约束见 [`AGENTS.md`](AGENTS.md),二者不重复。

## 开发环境

- **Go 版本以 `go.mod` 为准**(当前 1.26)。缺 Go 时 `make` 会直接失败。
- **buf**:改 `proto/` 时需要(Makefile 自动探测 `$GOPATH/bin/buf`)。安装见 [buf.build](https://buf.build/docs/installation/)。

## 常用命令

```bash
make proto       # 改 proto 后重新生成 gen/(gen/ 已进版本控制,改完记得 git add)
make docs        # 改命令树后刷新 docs/cmd/(freshness 守护测试依赖此 target)
go build ./cmd/server/    # 编译服务
go build ./cmd/kite/      # 编译 CLI
go test ./...             # 测试
go vet ./...              # 检查
```

## CI 约定

- **本地自检**:`make check`(gofmt + go mod tidy + ShellCheck/脚本回归 + vet + test + lint + proto-lint)。部署事务和安装器回归需要 Linux,macOS 会明确打印 skip,由 Linux CI 补齐。`make vuln`、`make proto-breaking`、`make verify-gen` 依赖网络或 git 基线,按需单独跑。
- **门禁只有一个**:`CI OK`。增减 / 重命名 job 时同步 `.github/workflows/ci.yml` 的 `needs` 和 `scripts/ci-results.sh`;失败、取消、意外跳过都不能变绿。发布只接受指定源码 SHA 在本仓 `ci.yml` 的成功 main push 运行,不能用同名外部检查替代。
- **工具版本的唯一来源**:Go 以 `go.mod` 为准;golangci-lint / buf / govulncheck / GoReleaser / Cosign 以 `Makefile` 顶部为准。第三方 Actions 固定完整 commit SHA,镜像 base 固定 digest;不要在 workflow 重复维护工具版本。
- **漏洞门禁**:PR 只在依赖变化时扫描,`scheduled.yml` 每日全量扫描并报告。已知修复版本的漏洞必须失败,不能因为它高于当前工具链就当成「尚未发布」;无已知修复版本要产生 advisory / issue,扫描器安装、执行和报告解析故障必须显式失败。
- **执行隔离与排队**:使用固定 `ubuntu-24.04` 托管 runner,不把未受信 PR 送入持久化自托管环境。PR 的旧 CI 可以取消;main 按 SHA 保留发布所需证据;发版、BSR、生产部署各自使用 `queue: max` 串行排队,不丢弃中间等待项。
- **release PR 的 CI**:`GITHUB_TOKEN` 创建 PR 不保证触发后续 workflow。`scripts/release-pr-ci.sh` 只接受本仓 `github-actions[bot]` 创建、目标为 main 的固定 release 分支;等待 / 批准 / 补派发 `ci.yml` 时核验准确 HEAD 和 workflow,动作前再次检查 HEAD。已有成功 producer 不重复派发,其它 bot 或同名 fork 分支不能借此获批。
- **镜像冒烟**:`Docker Build` 验证开发镜像;`Release Check` 从 GoReleaser snapshot 的真实二进制构建 `Dockerfile.release` 后运行同一安全冒烟。验证无令牌拒绝启动、健康检查及 REST 鉴权,不用 Dockerfile 文本相等冒充运行时验证。正式公开前还验证发布镜像双架构;生产部署另外运行完整 REST/gRPC 探针。
- **Go 互操作冒烟**:`Go Interop` job 跑 `make interop-go`(`scripts/go-interop.sh`):`interop/go` 依赖 BSR 上由 proto 生成的 Go SDK(接入方的真实路径),对真实运行的 `kite-server` 验证令牌鉴权与 `LOGIN_REQUIRED`;`docs/integration.md` 的 Go 示例与 `interop/go/kite.go` 逐字一致(测试守护)。SDK 版本锁在 `interop/go/go.mod`,随 BSR 新版本按需升级。
- **Rust 互操作冒烟**:`Rust Interop` job 跑 `make interop-rust`(`scripts/rust-interop.sh`):`buf` 把 proto 编成描述符,`interop/rust` 据此生成 tonic 客户端,对真实运行的 `kite-server` 验证健康检查、令牌鉴权和 `ErrorInfo`。`docs/` 里的 Rust 示例与 `interop/rust/src/*.rs` 逐字一致(有测试守护):改示例先改文档,再原样同步。本地需要 `cargo`,较慢,不在 `make check` 里。
- **网页播放器**:`Player` job 跑 `make player-check`(`web/player` 的 `npm ci` + 类型检查 + 测试 + 构建),Node 大版本取自 `Makefile` 顶部的 `NODE_VERSION`。它不在 `make check` 里(只改 Go 不需要 Node),改了 `web/player` 后本地跑一次。
- **npm 发布**:`@violetjs/kite-player` 只手动发布——改 `web/player/package.json` 的 `version` 并合入 `main`,再 Actions → `npm` → Run workflow(选 `main`),或 `gh workflow run npm.yml --ref main`。workflow 只从 `main` 发布、拒绝已发布过的版本,预发布版本(带 `-`)发到 `next` dist-tag。不用 git tag 触发:GoReleaser 按上一个 tag 生成 kite 的 changelog,另一套 tag 会混进去;发布对应的 commit 由 npm provenance 记录。需要仓库 secret `NPM_TOKEN`(对 `@violetjs` scope 有发布权限)。
- **真实上游冒烟**:`make upstream-smoke` 对真实上游逐个调用只读 RPC,断言响应不是空消息(专抓「成功但返回空」)。手动运行、不进 CI——外部服务的状态会让门禁漂移。endpoint 的 MapResponse 单测应优先用从真实上游抓取的响应(`testdata/`,裁剪并去除个人信息),而不是手写夹具。
- **BSR 发布**:公开且验真通过的 release 才推送到 `buf.build/vod-studio/kite`。精确版本标签始终保留,`main` 仅跟随数值最大的正式版,补旧版不会倒退。`BUF_TOKEN` 只在推送一步可见;缺失必须失败,不能显示成功跳过。手动补推:Actions → `BSR` → Run workflow,在 main 上指定已公开 tag。
- **生产部署**:release 公开后自动调用 `deploy.yml`,不增加日常人工批准。只从 main 工作流读取 `production` environment 的专用 SSH secrets,先检查宿主机部署器版本,再执行验签、切换、探针及失败回滚。缺 secret、旧部署器或健康失败均明确变红,不能假成功。管理员首次安装、权限治理和回滚见 [`docs/deploy.md`](docs/deploy.md)。
- **发布完成的含义**:合并 release PR 批准发版,release-please 建 draft 和 tag,同一 workflow 调用 `release.yml`;所有跨平台档案、签名校验和、双架构镜像、签名部署安装包完成并验真后才公开 release。只有最大正式版推进 `latest`;minor 标签也只前进。手动重跑公开版只验原产物和恢复适用标签,不覆盖已发布字节;失败的 draft 可以继续完成。公开版缺损需发布修正版,不能悄悄重建旧版。
- **仓库配置不是 YAML**:`python3 scripts/github-settings.py` 默认只读审计;管理员加 `--apply` 才应用主分支 / tag ruleset、Actions 权限与 production 分支白名单。脚本不读取、迁移或删除 secret 值,不移除已有人工审批规则;发现与自动部署冲突时必须先由管理员处理。

## 工作流(分支与提交)

### 分支命名

每个新任务 / 模块 / feature 先从 `main` 新建分支完成,不在 `main` 上直接开发。

**分支单位是 feature / PRD,不是单个 issue**。一个 PRD 拆出的多个子 issue 属于同一工作单元,共用一个分支,在同一 PR 里推进(子 issue 在该分支上作为独立的原子 commit 落地,见下文「原子性」)。只有当 issue 之间**无依赖且分属不同 feature** 时才各自开分支。「一 issue 一分支一 PR」是过度拆分,与 commit 原子性反对的精神一致——会让 review 队列膨胀、强耦合 issue 难以协调合并顺序。

判据:若一组 issue 共享同一个 PRD 父节点,或互相有 blocked-by 依赖,它们就该在同一个分支。

格式:**`<type>/<scope>-<简述>`**

- **type** 对齐 Conventional Commits:`feat` / `fix` / `chore` / `docs` / `refactor` / `style` / `test` / `perf` / `build` / `ci` / `revert` / `hotfix`。PR 标题同样必须符合(CI 的 PR Title 检查):squash merge 后它就是 `main` 上的 commit message,release-please 靠它生成版本号与 CHANGELOG。
- **scope** 指向最内层模块(`netease` / `cli` / `server` / `proto` / `tui`)或文件 / 区域名(`readme` / `ci` / `deps`)。可省略(scope 难以定位单模块时)。
- 全小写,`/` 分段,`-` 连词。无空格、无大写、无特殊字符。
- 简述用英文或拼音,短而清晰。

✅ 正确:
- `docs/readme-rewrite`
- `feat/cli-lyric-flag`
- `fix/netease-fee-field`
- `chore/deps-bump-bubbletea`
- `refactor/server-shutdown`

❌ 错误:
- `feature/这是一个很长的中文分支名`(冗长 + 中文)
- `update`(无 type 无 scope)
- `Fix/Login`(大写)
- `new-feature`(无 scope,scope 难定位时可省,但能定位就该写)

### Commit message 格式

- 中文 Conventional Commits:`feat(cli): 添加新功能` / `fix(netease): 修复接口 bug` / `refactor(server): ...`。
- **scope 指向最内层模块**,不叠加冗余前缀。
  - ✅ `fix(netease): artist albums fee 字段补全`
  - ✅ `feat(cli): song play 支持歌词同步`
  - ❌ `fix(api/netease): ...`(`api` 是冗余前缀,kite 是单体仓库)
  - ❌ `feat(kite): ...`(改动集中在一个子模块时,scope 用模块名而非项目名)
- **body 用 bullet points 列改动事实**,不写散文,不夹带主观评判与设计论证。决策过程写 PR 描述或 ADR。
- **本地 commit,不推送**。PR 由人工 review 后合并。

### 原子性

> 业界共识([fnune](https://fnune.com/2018/02/19/git-best-practices-atomic-commits/)、[Fagner Brack](https://fagnerbrack.com/one-commit-one-change-3d10b10cebbf)):**一个 commit = 一个逻辑变更**。

核心判据(只这三问,不列优先级表):

1. **单独 revert 会让构建坏掉吗?** 会 → 拆得不够,或拆错了粒度。
2. **三个月后看标题能一眼知道做了什么吗?** 不能 → 标题太笼统,或一个 commit 混了多件事。
3. **它服务的是同一个任务吗?** 一个任务可跨多文件、多层;混多个无关任务才该拆。

**反对过度拆分**。原子性服务于可读性与可回滚,不是为了「切碎」。例:「给 `song play` 加 `--lyric` 同步歌词」是一个完整任务,可以一个 commit 含 flag 解析 + 歌词获取 + 播放器集成。强行拆成「加 flag」「加歌词获取」「接入播放器」三个 commit,每个单独看都不完整,反而破坏可读性。

**draft 阶段宽松**。开发中可以先随便提交(WIP、调试片段都行),最后用 `git rebase -i` 整理成原子 commit。不要求边写边原子——要求的是**最终进入 main / PR 的历史是原子的**。

### PR 与 issue 规范

**Issue 标题**:

- 格式 **`[scope] 描述`**,scope 用最内层模块名(同 commit scope 规则)。
- ✅ `[netease] artist albums 接口 fee 字段补全`
- ✅ `[cli] song download 断点续传失效`
- ❌ `vertical: 补全挂载`(暴露实现方法论,不该出现在标题)
- 连续任务序列(如某 feature 的 T1–Tn)带 `Tn` 编号;独立 feature 不带编号。

**Issue labels**:

- **默认不加 label**。仅当改动性质明确匹配 GitHub 内置语义 label 时才加(如 `bug`、`documentation`)。
- 不加 `ready-for-agent` 等流程性 label——对人工 review 无信息量。

**PR 创建**(`gh pr create`):

- **assignees**:`--assignee @me`(当前 gh 账号)。
- **reviewers**:仓库全部 collaborator——`--reviewer DefectingCat,xunrua,JingpengZhang`。
- **labels**:默认不加(同 issue 规则)。
- **base**:指向 `main`(仓库默认分支)。

**合并**:

- **手动合并**,不启用 auto-merge。审批是协作约定,不是强制要求作者无法给自己完成的审批数量。
- **业务 PR 自选 Merge / Squash / Rebase**:需要保留原子提交时选 Merge 或 Rebase;Squash 只在 main 留一个汇总提交。PR 标题和所有非 merge 原子提交都必须符合上面的 Conventional Commits,确保任意合并方式都能正确推导版本。
- **release PR 使用 Squash**:这是版本 / changelog 的发布批准记录,不是业务提交归档。普通 PR 合 main 只累计待发布变化;合并 `chore(main): release ...` PR 才启动正式发版与自动部署,不混淆这两类合并。
- **分支清理按需执行**:合并后确认无未交付修改再删除;Squash / Rebase 后不要仅凭 `git branch --merged` 判断是否已合入。仓库治理脚本不擅自改变自动删分支偏好。

## 报告安全问题

见 [`SECURITY.md`](SECURITY.md)——**不要在公开 issue 报安全漏洞**,走 GitHub 私有安全公告。

## 项目结构

详见 [`AGENTS.md`](AGENTS.md)「架构与代码边界」。依赖方向由 `internal/archtest` 测试守护;`gen/`、`docs/cmd/` 是生成产物,已进版本控制(不要手改)。
