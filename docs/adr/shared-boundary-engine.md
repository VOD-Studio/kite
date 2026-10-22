# CLI / TUI 的共享边界是 engine + endpoint,不是 service

> 修订 `tui-client-architecture.md` 中「view 直接调 service 层」的表述。

## Context

早期文档(AGENTS.md、CONTEXT.md、tui-client-architecture)都写「`internal/service/` 是 CLI 与 TUI 的共享边界」。核对代码后事实相反:

- `internal/service` 的方法签名是 gRPC 的 `*XxxRequest → *XxxResponse`,每个方法恒一行 `engine.Execute(...)`,本质是 gRPC adapter,只被 `internal/server` 引用。
- CLI 从一开始就直连 `engine` + `endpoint` 声明(`kit.RenderExec` / `kit.Exec`),从未经过 service。
- 结果:`kit.Exec` 是 `service.executeOverride` 的拷贝,两份执行逻辑可以悄悄分叉(拷贝漏掉了 `PathFunc`)。

## Decision

共享边界是 **`internal/netease/engine` + `internal/netease/endpoint`**。

- endpoint 是声明式的(Meta + MapRequest + MapResponse),engine 是唯一的执行语义定义处(`Execute` / `ExecuteWithCookie`)。
- CLI、TUI、gRPC service 是三个平行的前端/适配器,都消费 endpoint 声明,谁也不拷贝执行逻辑。
- `internal/service` 保持 gRPC adapter 的定位,不承载业务逻辑,不被 CLI/TUI 引用。
- 与前端无关的本地能力(会话文件、配置、召回池)放在 `internal/localstate`、`internal/recall`,不放 `internal/cli/kit`。
- 依赖方向由 `internal/archtest` 守护。

## Considered Options

### 引入真正的 use-case 层(暂不采用)

在 engine 之上建一层与传输无关的用例(队列、连续播放、跨接口组合),CLI/TUI/gRPC 都调它。

- ✅ TUI 的有状态逻辑有归属。
- ❌ TUI 尚未存在,现在抽象是猜需求;现有 78 个 RPC 都是单接口调用,没有组合逻辑可抽。
- 触发条件:TUI 第一次出现需要在多个 view 间共享的有状态编排(播放队列、跨域组合)时,在 `internal/` 下新建独立包承载,**不改造 `internal/service`**。

**2026-10 更新:跨接口组合的触发条件已出现。** issue #88 / #89 要求「按 trackIds 分页取歌曲」「取全量用户歌单后再筛选、分页」,
一次读取需要多次上游调用。按上面的约定新建 `internal/netease/compose` 承载,没有改造 `internal/service`
(它仍是一行调用);读取编排里的每一步仍是 endpoint 声明、经 `engine.Execute` 执行,执行语义没有第二份。

扫码确认后补查账号资料也由 `compose` 编排。认证接口需要读取 `Set-Cookie`,沿用 `engine.RawDoWithCookieAndInput`;
账号查询使用本次确认返回的新 cookie,完整会话才交给 gRPC adapter 返回。

## Consequences

- 新增 endpoint 声明后,CLI 与 gRPC 两侧各自接线,执行语义自动一致。
- TUI view 直接用 `engine.Execute` / `ExecuteWithCookie` + endpoint 声明;需要本地状态时用 `localstate` / `recall`。
- `cli/songdl`(下载)目前依赖 `kit.Kit` 的进度渲染与 HTTP client,TUI 复用时需先把核心下载逻辑从进度渲染里拆出来——登记为已知债务,TUI 首次需要下载时处理。
