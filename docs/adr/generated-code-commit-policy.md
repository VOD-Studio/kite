# gen/ 生成产物提交进版本控制

2026-07-27 决策(访谈见 `CONTEXT.md`)。

## Context

`gen/` 是 `buf generate` 从 `proto/` 产出的 Go 代码(Go message stub、gRPC service、grpc-gateway reverse proxy、OpenAPI spec)。项目代码 `import "github.com/VOD-Studio/kite/gen/go/netease/music/v1"` 是**硬依赖**——不生成则 `go vet ./...`、`go test ./...` 直接报错:

```
no required module provides package github.com/VOD-Studio/kite/gen/go/netease/music/v1
```

此前 `gen/` 被 `.gitignore` 排除。这导致两个问题:

1. 新成员 clone 后必须先装 buf 并跑 `make proto` 才能编译,IDE 索引、`go doc` 在生成前全部失效。
2. 插件版本漂移会让不同开发者/CI 产出略有不同的代码,无守护时易出鬼影问题。

## Considered Options

### 选项 A:`gen/` 提交进版本控制 + CI 守护 job(采用)

将 `gen/` 从 `.gitignore` 移除并提交。CI 新增守护 job:`make proto && git diff --exit-code -- gen/`,有 diff 即 fail,防止「proto 改了但忘了重新生成提交」的漂移。

- ✅ clone 即编译,零前置步骤,IDE 立即可用
- ✅ `go get`/`go install@version` 的生态预期:Go 模块不期望有「先生成」这一步
- ✅ 插件版本漂移被守护 job 兜住(全队产物一致)
- ✅ PR review 能看到 proto 改动对生成代码的影响
- ✅ 与本仓库 `docs/cmd/`(cobra 生成,已提交)同性质、同待遇
- ❌ proto 改动会产出大 diff;需在 PR 规范里标注「gen/ 是生成产物,主要看 proto」

### 选项 B:`gen/` 保持 gitignore,CI 内每次生成(否决)

**适用边界**:此选项仅适合生成产物**不被代码 import**(非硬依赖)的场景——如纯文档、OpenAPI spec 仅供查阅。本项目的 `gen/` 被 import,不属此列。

不提交 `gen/`,CI 每个 job 先跑 `make proto`。

- ✅ 仓库 diff 干净
- ❌ 新成员 clone 后必须装 buf 才能 vet/test/IDE 工作,首次体验差
- ❌ 本地 `go vet` 默认红(除非记得先生成),开发体验差
- ❌ 与已提交的 `docs/cmd/` 不一致(同样是生成产物,两种待遇)
- ❌ 不符合 protobuf 作者 Kenton Varda 划定的边界:「**源码构建者自带工具链,但对外发布的包应包含生成产物**」——本项目 `gen/` 被 import,属发布包一侧

## Decision

采用选项 A。`gen/` 提交进版本控制,配 CI 守护 job 防漂移。

### 判定依据(权重排序)

1. **硬依赖性质**(决定性):`gen/` 被 `import`,不生成则构建失败。Kenton Varda(protobuf 作者)在 Stack Overflow 明确:这种情况下生成产物应随发布包分发,让消费者无需特殊工具。来源:[stackoverflow.com/questions/41186798](https://stackoverflow.com/questions/41186798)
2. **Go 生态先例**:Kubernetes(`kubernetes/api`)、etcd(`etcd-io/etcd`)均提交 `.pb.go` 并配 verify 守护(etcd 的 `verify-genproto`)。两者都是发布型 Go 项目,与 kite 同类。CockroachDB 选择 gitignore,但它把生成代码当「内部实现细节」非发布包,场景不同。
3. **插件版本漂移风险**:Go protobuf 插件版本间产出略有差异,不提交又不锁版本时 CI 不稳定。来源:[jbrandhorst.com/post/plugin-versioning](https://jbrandhorst.com/post/plugin-versioning/)
4. **仓库内一致性**:`docs/cmd/`(89 个文件,cobra `GenMarkdownTree` 生成)已提交且 `docs_freshness_test.go` 守护。`gen/` 性质完全相同,应一致对待。

### 配套约束

- 改 `proto/` 后必须 `make proto && git add gen/` 一起提交,否则 CI 守护 job 红。
- `make clean` **不再清 `gen/`**(会删跟踪文件),改为清 `tmp/`/`bin/`。
- PR review 时 `gen/` 的大 diff 标注为「生成产物,主要看 proto」。

## Consequences

- 反悔成本高:改回 gitignore 要 `git rm -r --cached gen/` 并重写历史(若已发布)。
- proto 改动的 PR diff 变大,但 review 聚焦 proto 即可。
- CI 守护 job(下一个 issue 添加)是此决策的执行保障,缺它则漂移无法被发现。

## 反向条件

若未来 `gen/` 不再被项目代码 `import`(变成纯辅助/文档),那时改回 gitignore 才更合理。当前不是此情形。
