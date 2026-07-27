# kite TUI 客户端:三栏布局 + 插件式 view 注册 + shimmer 加载态

> supersede `play-screen-bubbletea.md`(原「单曲播放屏重写」范围)。播放屏升级为 TUI 客户端的一个视图,不再是独立产物。

owner 决策(2026-07-27 访谈):废弃现有播放屏的单曲视图设计,用 charm.land(bubbletea v2 + lipgloss)重做**完整 TUI 客户端**——侧边栏导航 + 列表主区 + 常驻 mini-player + 全屏播放页。不参考现有任何界面设计。CLI 子命令全部保留,与 TUI 平行;裸跑 onboarding 行为不变。

## Context

网易云 78 RPC 已全接入 CLI(song/playlist/album/artist/user/search/recommend/auth/fm 九域)。但 proto 还没覆盖网易云全部功能——评论、MV、排行榜、电台、网盘、签到等域尚未建模。TUI 架构必须满足两个约束:

1. **容纳完整能力面**:不能写死今天的 78 RPC 快照,要能渐进接入 proto 未建域。
2. **CLI 与 TUI 共存**:CLI 是工具型单点操作(输出导向),TUI 是连续体验(搜索→加队列→播放→下一首)。两者共享底层 service,但 TUI 不受 CLI 命令粒度束缚。

工作流约束:owner 的开发习惯是「先写 CLI 再写 TUI」——每域先验证 CLI 可用、接口稳了再补 TUI 视图。TUI 框架要在新域落地那天零改动接入。

## Considered Options

### 选项 A:插件式 view 注册(采用)

每个域是一个 view 模块,实现 bubbletea.Model,注册到导航表。侧边栏入口由注册表生成。新域(proto 补的评论/MV 等)落 CLI → 验证 → 落 TUI view + 注册一行,零框架改动。未建域在注册表里 `New=nil` 占位,侧边栏隐藏但架构位置留好。

- ✅ 完整能力面:注册表含全部域(含未建的 nil 占位),架构上是完整的
- ✅ 渐进接入:新域加一行注册,不动框架
- ✅ CLI/TUI 解耦:view 直接调 service 层,不经 CLI
- ✅ 「先 CLI 后 TUI」工作流天然支持:view 工厂在 CLI 落地后才填 `New`

### 选项 B:TUI 视图 = CLI 命令镜像(否决)

TUI 视图与 CLI 子命令一一对应,视图即命令的可视化包装。

- ❌ 表达力受限:队列/连续播放/mini-player 这些 CLI 难表达的场景会很别扭
- ❌ 强行耦合:CLI 是输出导向(表格/JSON),TUI 要结构化数据,绕一层渲染再解析很别扭

### 选项 C:硬编码今日 9 域(否决)

侧边栏入口写死今天的 78 RPC 对应的 9 个域,评论/MV 等未建域等 proto 补了再加。

- ❌ 架构腐化:每加一个域要改导航代码与侧边栏布局,短期快长期腐
- ❌ 与「完整架构」愿景不符:注册表永远只是今天的快照

## Consequences

### 架构

- `internal/tui` 包建立,三栏布局(侧边栏 + 主区 + mini-player)+ 全屏播放页。
- `internal/tui/view/registry.go` 是导航的单一真相:含全部域(已建 `New=工厂`,未建 `New=nil`)。
- 详情页多 tab 模型:评论/MV/相似等作为 tab 嵌入歌曲/歌单/专辑/歌手详情页,proto 补上后加 tab,不动侧边栏。
- 播放屏降级为 `internal/tui/playscreen/` 子包,是 TUI 客户端的一个视图(mini-player Enter 进入)。

### 依赖

- `charm.land/bubbletea/v2` + `charm.land/lipgloss/v2` + `charm.land/bubbles/v2` 进入 go.mod。
- 不引第三方图像缩放库(半块路径最近邻采样,Kitty 路径原图传输由协议端缩放)。

### 封面协议矩阵(沿 oh-my-pi)

- Kitty(ghostty/kitty)→ iTerm2 inline → 半块字符画,三级降级。
- transmit-once, place-many:View 输出永远不含 base64,Kitty 路径 base64 只在加载 Cmd 出现一次。
- 检测纯环境变量(不做 query-response 探测,避免启动阻塞)。
- `KITE_IMAGE_PROTOCOL=kitty|iterm2|halfblock|off` env 覆盖(对齐 oh-my-pi `PI_FORCE_IMAGE_PROTOCOL`)。

### 加载态:shimmer(沿 oh-my-pi)

加载态 = spinner 帧(unicode preset status 帧 `⣾⣽⣻⢿⡿⣟⣯⣷`,染 accent 色,80ms)+ 空格 + shimmer 文字。

shimmer 算法移植自 oh-my-pi `shimmer.ts` classic 模式:亮带从左向右扫过(30 cells/s 固定速度),带外 dim 可读,带内按余弦强度分 3 档(low/mid/high),high 加粗,同档 run 合并 ANSI。调色板用紫粉主题(low=#5a3a8a / mid=#9d6ff5 / high=#ee6ff8)。

### 行为对齐(不变项)

- 裸跑 `kite` → 工具型 onboarding(CONTEXT.md「工具型定位」不受影响)。
- `kite tui` 是新增入口,不抢占裸跑默认行为。
- CLI 子命令全部保留,与 TUI 平行。
- player.Player 接口不变(Play/Pause/Seek/Volume/Progress/State),TUI 只消费接口。

### 文档

- roadmap Phase D 段同步更新:原「第一步小验证 + 第二步全屏播放器」合并为「TUI 客户端」。
- CONTEXT.md 新增「TUI 客户端」「插件式 view 注册」「shimmer 加载态」词汇。
- PRD-0016 重写为完整 TUI 客户端规格(含待定细节的推荐默认值)。
