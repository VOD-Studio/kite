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

- **本地自检**:`make check`(gofmt + go mod tidy + vet + test + lint + proto-lint)。`make vuln`、`make proto-breaking`、`make verify-gen` 依赖网络或 git 基线,按需单独跑。
- **门禁只有一个**:`CI OK`(聚合 job)。新增 / 重命名 CI job 时在 `.github/workflows/ci.yml` 同步它的 `needs`;发布流程与 ruleset 的 required check 都只认这个名字。
- **工具版本的唯一来源是 `Makefile` 顶部**(golangci-lint / buf / govulncheck),不要在 workflow 里重复写。
- **门禁不依赖会自行变化的外部状态**:漏洞库、未固定版本的插件或 runner,都会让「没改代码」的 PR 变红。依赖漏洞只在 PR 改动 `go.mod` / `go.sum` 时才拦,全量扫描由 `scheduled.yml` 每日跑并开 issue。
- **release PR 的 CI**:release PR 由 `github-actions[bot]` 创建,`GITHUB_TOKEN` 触发的事件有两个坑——仓库的「首次贡献者需批准」策略会让它的 pull_request CI 停在 `action_required`(此时 required check `CI OK` 视为未满足、PR 保持 BLOCKED),且按 GitHub 规则这类事件原则上不会触发其它 workflow。`release-please.yml` 每次运行后调用 `scripts/release-pr-ci.sh`:有等待批准的运行就批准(这个 PR 由我们自己的 workflow 生成,批准是安全的,无需放宽仓库策略);轮询约 1 分钟仍没有任何 pull_request 运行,则回退为在 release 分支上 `workflow_dispatch` 一次 CI。逻辑有假 `gh` 的单元测试(`make scripts-check`)。若 release PR 仍是 BLOCKED,在 Actions 页手动重跑 `Release Please`。
- **镜像冒烟**:`Docker Build` job 在构建后跑 `scripts/docker-smoke.sh`(无令牌拒绝启动、`/healthz`、REST 鉴权),不访问外网。镜像的监听地址 / 令牌策略见 [`docs/server-security.md`](docs/server-security.md);`Dockerfile` 与 `Dockerfile.release` 的 `KITE_SERVER_*` 必须一致(有测试守护)。
- **Rust 互操作冒烟**:`Rust Interop` job 跑 `make interop-rust`(`scripts/rust-interop.sh`):`buf` 把 proto 编成描述符,`interop/rust` 据此生成 tonic 客户端,对真实运行的 `kite-server` 验证健康检查、令牌鉴权和 `ErrorInfo`。`docs/` 里的 Rust 示例与 `interop/rust/src/*.rs` 逐字一致(有测试守护):改示例先改文档,再原样同步。本地需要 `cargo`,较慢,不在 `make check` 里。
- **真实上游冒烟**:`make upstream-smoke` 对真实上游逐个调用只读 RPC,断言响应不是空消息(专抓「成功但返回空」)。手动运行、不进 CI——外部服务的状态会让门禁漂移。endpoint 的 MapResponse 单测应优先用从真实上游抓取的响应(`testdata/`,裁剪并去除个人信息),而不是手写夹具。
- **BSR 发布**:release 成功后 `bsr` job 调用 `bsr.yml`,把 `proto/` 推送到 `buf.build/vod-studio/kite`(版本 tag 作标签;`main` 标签只跟随最新正式版,补推旧版本不会让它倒退),供其它语言的项目直接依赖。令牌只在最后的推送一步可见,手动补推只接受已发布 release 的版本号。需要 BSR 上已有组织 `vod-studio` 和仓库 secret `BUF_TOKEN`;未配置时只给提示、跳过,不影响发布。手动补推:Actions → `BSR` → Run workflow(填 tag)。
- **生产部署**:release 成功后 `deploy` job 调用 `deploy.yml`,经 SSH 把新镜像部署到与 violet 同机的服务器(失败自动回滚)。服务器侧一次性安装、需要的仓库 secret、手动部署 / 回滚见 [`docs/deploy.md`](docs/deploy.md);未配置部署 secret 时只给提示、跳过,不影响发布。
- **发布**:release PR 合并后,`release-please.yml` 在同一 workflow 内直接调用 `release.yml`(release-please 用 `GITHUB_TOKEN` 打的 tag 不会触发 `on: push: tags`)。手动补发某个 tag 的产物:Actions → `Release` → Run workflow。

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

- **手动合并**,不勾 auto-merge。由人工 review approve 后在 UI 点合并。
- 合并方式:**squash merge**(单 commit 历史,与已有 PR 惯例一致)。
- 合并后:**自动删除分支**(`gh pr merge --squash --delete-branch`,合并即删远程 + 本地 feature 分支)。

## 报告安全问题

见 [`SECURITY.md`](SECURITY.md)——**不要在公开 issue 报安全漏洞**,走 GitHub 私有安全公告。

## 项目结构

详见 [`AGENTS.md`](AGENTS.md)「架构与代码边界」。依赖方向由 `internal/archtest` 测试守护;`gen/`、`docs/cmd/` 是生成产物,已进版本控制(不要手改)。
