# kite

Go 实现的网易云能力服务 + 命令行工具(单体仓库)。

两个产物共享同一套网易云核心(`internal/netease/`)与 service 层:gRPC + grpc-gateway 服务,以及直连 engine 的 CLI。

## 架构与代码边界

### 入口

- **`cmd/server`** —— gRPC(`:3722`)+ grpc-gateway REST(`:3721`)双 server,SIGINT/SIGTERM 优雅关闭。
- **`cmd/kite`** —— CLI 工具,直连 engine + endpoint 声明,**不经 gRPC**。
- **`cmd/kite-docs`** —— 命令树 markdown 生成器(`make docs` 调用)。
- **`cmd/demo`** —— 临时验证脚本,非交付产物。

### internal/ 分层

| 包 | 职责 |
|---|---|
| `internal/netease/` | 网易云核心:`engine`(请求引擎)/ `endpoint`(API 声明)/ `session`(会话管理)/ `model`(数据模型)。weapi/eapi 加密用 Go 标准库自实现,不依赖第三方音乐库。 |
| `internal/cli/` | CLI 命令按领域分包(`auth`/`song`/`album`/`artist`/`playlist`/`user`/`search`/`recommend`/`fm`)。`kit/` 是公共基础设施:渲染层、session、召回池。 |
| `internal/server/` | gRPC server 装配 + cookie 拦截器。 |
| `internal/service/` | service 层,**CLI 与 TUI 的共享边界**。 |
| `internal/cache/` `internal/store/` | Redis 缓存与会话存储(当前 noop 实现,后续接入)。 |
| `internal/infra/` | 基础设施装配。 |
| `observability/` | 日志 / OTel tracer / Prometheus 指标 / 敏感字段脱敏。 |
| `errors/` | 错误码与错误类型定义。 |

### 关键约束

- **proto 是 RPC 契约的唯一真相**。`proto/netease/music/v1/` 定义全部 RPC;改 RPC 必须先改 proto 再 `make proto` 重新生成。不要在 Go 代码里手写与 proto 冲突的类型。
- **CLI 与 TUI 平行不耦合**。两者都调 `internal/service/`,不互相直接调用。新域接入顺序是「先 CLI 后 TUI」——CLI 验证接口稳了再补 TUI 视图(见 `docs/adr/tui-client-architecture.md`)。
- **proto 未覆盖的能力走 CLI 直连**。评论 / MV / 排行榜 / 电台 / 网盘 / 签到等尚未在 proto 建模,但已通过 endpoint 声明接入 CLI。补 proto 时按「先 CLI 验证 → 再 proto 建模 → 最后 TUI」的节奏。

## 开发流与命令 (Makefile)

所有代码生成走根 `Makefile`:

- **`make proto`** —— 改 `proto/` 后重新生成 Go stub / gRPC service / gateway / OpenAPI 到 `gen/`。`gen/` **提交进版本控制**(被项目代码 `import` 硬依赖,clone 即可编译;与 `docs/cmd/` 同性质)。改 proto 后跑此命令再 `git add gen/` 提交,CI 守护 job 会校验漂移。决策见 [ADR](docs/adr/generated-code-commit-policy.md)。
- **`make proto-lint`** —— 检查 proto 文件规范。
- **`make docs`** —— 改命令树后重新生成 `docs/cmd/` 命令参考。**freshness 守护测试依赖此 target**,不跑会导致 `docs_freshness_test.go` 红。
- **`make clean`** —— 清除本地构建产物(`tmp/`、`bin/` 等)。**不清 `gen/`**——它已进版本控制(见上条),清了等于删跟踪文件。

裸命令:

```bash
go build ./cmd/server/    # 编译服务
go build ./cmd/kite/      # 编译 CLI
go test ./...             # 测试
go vet ./...              # 检查
```

### 环境依赖须知

- 后端需要 **Go 1.25**。若环境缺 Go,`make` 无提示直接失败。**不要自行安装 Go**,停下来告知用户并等待处理。
- proto 工具链:`buf`(Makefile 自动探测 `$GOPATH/bin/buf`)。改 proto 时需要本地有 buf。

## Agent skills

### 命令文档(生成真相)

`docs/cmd/` 由 `cmd/kite-docs` 通过 cobra `GenMarkdownTree` 生成,**不是手写**。命令语法的唯一真相是 `kite <命令> --help`,生成参考与之同步。**不要手改 `docs/cmd/` 下任何文件**——改命令树后跑 `make docs` 刷新。

### 文档入口

- **上手流程**:[`docs/kite-guide.md`](docs/kite-guide.md) —— 安装 / 登录 / 常用操作 / 别名 / 补全。
- **CLI 设计背景**:[`docs/kite-cli-design.md`](docs/kite-cli-design.md) —— 输出层、渲染层、退出码等架构决策。
- **功能路线图**:[`docs/kite-roadmap.md`](docs/kite-roadmap.md) —— 各 Phase(输出层 / 实用功能 / 配置 / 工程化 / TUI)状态与规划。
- **领域术语与决策索引**:[`CONTEXT.md`](CONTEXT.md) —— 项目领域词汇表 + 不可逆决策索引(访谈产出,后续实现引用)。
- **ADR**:[`docs/adr/`](docs/adr/) —— 架构决策记录(`tui-client-architecture`、`generated-code-commit-policy`)。
- **PRD**:[`docs/prd/`](docs/prd/) —— 具体 feature 规格(最新 PRD-0016 TUI 客户端)。

agent 友好原则:`--help` 是命令语法唯一真相。skill 或外部文档一律指向 `--help` 或生成参考,不要复制命令手册——复制即第二份真相,必然腐烂。

<!-- MANUAL: Any manually added notes below this line are preserved on regeneration -->

## 工作流(分支与提交)

### 分支命名

每个新任务 / 模块 / feature 先从 `main` 新建分支完成,不在 `main` 上直接开发。

格式:**`<type>/<scope>-<简述>`**

- **type** 对齐 Conventional Commits:`feat` / `fix` / `chore` / `docs` / `refactor` / `style` / `test` / `perf` / `hotfix`。
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

### 架构耦合

> 这是代码层面的原则,**不是 commit 拆分规则**。与原子性分开理解。

- **公共包不夹带领域业务逻辑**。`internal/cli/kit/`(渲染层 / session / 召回池)、`observability/`、`errors/` 这类公共基础设施,不写网易云 `song` / `playlist` 等领域特有逻辑。领域逻辑放对应 `internal/cli/<domain>/` 或 `internal/service/`。
  - 判断「是否公共」看**是否被多个领域引用**,而非位置。某领域私有逻辑一旦被第二个领域复用,应先 `refactor: 将 X 从 features/A 提到公共层` 单独提交,再在新领域接入。
- **service 层是 CLI / TUI 的共享边界**。两者都调 `internal/service/`,不互相直接调用。TUI view 工厂直接消费 service 接口,不经 CLI 命令层。
- **proto 是契约唯一真相**。Go 代码里不手写与 proto 冲突的类型;改 RPC 先改 proto 再生成。

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
