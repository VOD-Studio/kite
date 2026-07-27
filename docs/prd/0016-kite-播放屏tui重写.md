# PRD: kite TUI 客户端(Phase D)

> 状态:📋 待实现
> 关联:[roadmap Phase D](../kite-roadmap.md)、[ADR: tui-client-architecture](../adr/tui-client-architecture.md)
> supersede:本 PRD 取代原「单曲播放屏重写」范围——播放屏升级为完整 TUI 客户端的一个视图。
> 范围:新增 `kite tui` 入口,用 bubbletea v2 + lipgloss 重做完整 TUI 客户端(侧边栏 + 列表 + mini-player + 全屏播放页)。CLI 子命令全部保留,与 TUI 平行;裸跑 onboarding 行为不变。

## Problem Statement

1. **播放屏是孤立的单曲视图**:`song play` 进去是单曲全屏播放,无浏览/队列/导航——用户想「看歌单挑歌播放」必须退出 TUI 跑 CLI 子命令,体验割裂。
2. **手写 ANSI 维护成本高**:`play.go` 内 playUI/statusRenderer/keyloop/csiComplete/readKey 手动维护光标序列,渲染与状态耦合,改样式=改终端字节序列。
3. **架构无法容纳 proto 未建域**:网易云 78 RPC 已全接入 CLI,但评论/MV/排行榜/电台等域 proto 还没建。TUI 架构必须能渐进接入新域,而不是写死今天的快照。

## Solution

用 bubbletea v2 + lipgloss 重做完整 TUI 客户端,**不参考现有任何界面设计**,采用 Charm 设计语言。核心架构决策见 [ADR: tui-client-architecture](../adr/tui-client-architecture.md)。

### 信息架构(三栏布局)

```
┌─────────────┬──────────────────────────────────────┐
│ 🔍 搜索     │                                      │
│ ── 发现 ──  │                                      │
│  每日推荐   │           主内容区                   │
│  私人 FM    │      (列表 / 网格 / 详情页)         │
│  新碟上架   │                                      │
│  精品歌单   │                                      │
│  新歌速递   │                                      │
│ ── 我的 ──  │                                      │
│  红心歌曲   │                                      │
│  我的歌单   │                                      │
│  收藏歌单   │                                      │
│  收藏专辑   │                                      │
│  最近播放   │                                      │
│ ── 设置 ──  │                                      │
│  账号       │                                      │
│  登录/登出  │                                      │
├─────────────┴──────────────────────────────────────┤
│ ▒▒ 封面  艺人 - 歌名  ━━━━●━━━━  01:23/03:45  ⏯ │  mini-player(常驻)
└────────────────────────────────────────────────────┘
```

- **左侧边栏**:分类导航树(发现/我的/设置 + 搜索置顶),选中切换主区视图。
- **中主区**:列表/网格,随侧边栏选中切换。列表项 Enter 进详情页(多 tab)或全屏播放页(歌曲)。
- **底 mini-player**:常驻,显示当前播放的封面缩略图 + 标题 + 进度 + 控制。Enter 进全屏播放页,q/Backspace 返回。

### 全屏播放页

mini-player Enter 进入,封面大图 + 歌词舞台,q/Backspace 返回列表。与 mini-player 是同一播放状态的两种视图(类 Spotify)。

### 详情页多 tab

列表项(歌单/专辑/歌手)Enter 进详情页,内含多个 tab:
- 歌单详情页:[曲目 / 简介 / 评论(占位)]
- 专辑详情页:[曲目 / 简介 / 评论(占位)]
- 歌手详情页:[热门歌曲 / 全部专辑 / 简介 / 相似]
- 歌曲详情:嵌入全屏播放页的 [歌词 / 创作者 / 相似歌曲 / 评论(占位)]

评论/MV 等 proto 未建域作为 tab 占位,proto 补上后加 tab,不动侧边栏。

## 架构决策(详见 ADR)

### 运行时架构(三层)

- **底层** = kite service 层(endpoint 声明 + engine,78 RPC 落点)
- **CLI** = 单点工具型操作,输出导向(表格/JSON),沿现状
- **TUI** = 连续体验,直接调 service 层拿结构化数据,不受 CLI 命令粒度束缚

「先 CLI 后 TUI」是**开发顺序**(每域先验证 CLI 可用、接口稳了再补 TUI 视图),不是运行时依赖。TUI 不调 CLI 命令,CLI 也不依赖 TUI。

### 扩展模型(插件式 view 注册)

```go
type View struct {
    ID       string         // 唯一标识,如 "daily-recommend"
    Title    string         // 侧边栏显示名
    Category Category       // Discover/My/Settings
    New      func(deps) bubbletea.Model  // 工厂,nil = 未实现
}

var registry = []View{
    {ID: "daily-recommend", Category: Discover, New: NewDailyRecommendView},
    {ID: "toplist", Category: Discover, New: nil /* TODO: proto 未建 */},
    // ...
}
// 侧边栏渲染时跳过 New==nil,但注册表保留条目作为架构占位
```

新域(proto 未来补的评论/MV/排行榜/电台)落 CLI → 验证 → 落 TUI view + 注册一行,零框架改动。未建域在注册表里有 ID + Category + TODO 注释(架构位置留好),`New==nil` 时侧边栏不渲染。

### 导航树(完整网易云能力面)

架构上完整,实现分批。✅ 已建 proto 域 / ⏳ proto 未建(注册表占位,侧边栏隐藏)/ ⚠️ proto 数据缺口:

```
🔍 搜索                     [搜索域 ✅]
── 发现 ──────────────────────
  每日推荐               [推荐域 ✅ 日推歌+日推歌单]
  私人 FM                [FM域 ✅]
  排行榜                 [⏳ proto未建:飙升/新歌/热歌/原创/歌手榜]
  新碟上架               [专辑域 ✅ shelf/newest/all]
  精品歌单               [歌单域 ✅ HighQuality+分类标签]
  新歌速递               [推荐域 ✅ RecommendNewSongs]
  MV                     [⏳ proto未建:MV详情/播放/评论/相关]
  电台/播客              [⏳ proto未建:DJRadio详情/节目/订阅/分类]
── 我的 ──────────────────────
  红心歌曲               [单曲域 ✅ LikedList → fan-out GetSongDetail]
  我的歌单               [用户域 ✅ UserPlaylist created]
  收藏歌单               [用户域 ✅ UserPlaylist subscribed]
  收藏专辑               [专辑域 ✅ SubscribedAlbums]
  关注歌手               [⚠️ proto缺口:有订阅写操作,无列表读接口]
  关注的人               [用户域 ✅ Follows]
  最近播放               [recall池 ✅ 零成本]
  音乐网盘               [⏳ proto未建:上传/列表/删除]
── 社交 ──────────────────────
  朋友动态               [用户域 ✅ Events]
  评论                   [⏳ proto未建:歌曲/专辑/歌单/MV评论,嵌详情页tab]
  私信/通知              [⏳ proto未建]
── 设置 ──────────────────────
  我的账号               [用户域 ✅ 账号/等级/播放记录]
  登录/切换/登出          [登录域 ✅]
  签到/云贝              [⏳ proto未建:签到/云贝任务]
```

### 双形态共存

- 裸跑 `kite` → 工具型 onboarding(不变,CONTEXT.md「工具型定位」保留)
- `kite tui` → 完整 TUI 客户端(新入口,不抢占裸跑默认行为)
- CLI 子命令(search/download/play/recommend 等)→ 全部保留,与 TUI 平行

## 视觉与加载态

### 视觉调性:封面驱动动态色

背景/边框/高亮/进度条随当前播放专辑封面取色。每首歌界面主色随封面变化。无封面用默认调色板(Charm 紫粉 `#7D56F4 → #EE6FF8`)。复用取色算法(解码图缩至 8×8 → HSL 过滤 → 主色+强调色)。

### 封面:真实图片(协议矩阵)

支持终端显示真实图片,渐进增强协议矩阵(参考 oh-my-pi):
1. **Kitty 图形协议**(ghostty/kitty):transmit-once, place-many,View 输出永远不含 base64
2. **iTerm2 inline**(OSC 1337):图像转义作静态行,bubbletea 行 diff 天然 transmit-once
3. **半块字符画**:U+2580 `▀`,最近邻采样,任意 truecolor 终端

检测纯环境变量(不做 query-response 探测,避免启动阻塞):
- `TERM=xterm-kitty`/`xterm-ghostty`/`KITTY_WINDOW_ID` → kitty
- `TERM_PROGRAM=iTerm.app`/`WEZTERM_EXECUTABLE` → iterm2
- 其余 → 半块

env 覆盖:`KITE_IMAGE_PROTOCOL=kitty|iterm2|halfblock|off`(对齐 oh-my-pi `PI_FORCE_IMAGE_PROTOCOL`)。

高度保持 fallback:三种渲染占用同一封面 rect,协议降级不改变布局行数。

### 加载态:shimmer(oh-my-pi 同款)

加载态 = spinner 帧(unicode preset status 帧 `⣾⣽⣻⢿⡿⣟⣯⣷`,染 accent 色,80ms 换帧)+ 空格 + shimmer 文字。

shimmer 算法(移植自 oh-my-pi `packages/coding-agent/src/modes/theme/shimmer.ts` 的 classic 模式):
- 亮带从左向右扫过,带外 dim(文字始终可读),带内按余弦强度分 3 档(low/mid/high),high 加粗
- 固定速度 30 cells/s(不是固定周期,长文不抖,每帧推进 ≤1 cell)
- 亮带半宽 6,padding 10,3 档阈值 0.65/0.22
- 同档连续字符合并 ANSI 输出(减少转义序列)

调色板用紫粉主题:`low=#5a3a8a`(暗紫)/ `mid=#9d6ff5`(中紫)/ `high=#ee6ff8`(亮粉)+ bold。spinner 染 `#ee6ff8`。

> 参考实现:mimo-blog 的 `mimo-music/internal/cli/auth/qrtui/shimmer.go`(扫码登录已落地,同款算法)。

## 待定细节(按推荐默认,实现时再调)

以下 grill 时未最终拍板,先按推荐默认值写进 PRD,标注可调:

### 队列模型(推荐:歌单/专辑顺序 + 手动加)

- 从歌单/专辑进入播放 → 队列 = 该歌单/专辑全部曲目,按顺序
- 单曲 `song play` → 单曲队列(无下一首)
- 随机/循环模式:顺序 / 单曲循环 / 随机 三档(键 `r` 切换)
- 手动加:列表项按 `a` 加到队列尾部,按 `A` 加到下一首

### 键位方案(推荐:全局键 + 视图局部键)

全局键:
- `Tab` / `Shift+Tab`:焦点切换(侧边栏 ↔ 主区 ↔ mini-player)
- `1-9`:快速跳侧边栏分类
- `q`:返回上一级 / 退出(根级)
- `Ctrl+C` / `Esc Esc`:退出

主区列表键:
- `↑↓` / `j k`:上下移动
- `Enter`:进详情页 / 播放(歌曲)
- `/`:搜索(任意视图呼出搜索框)
- `a` / `A`:加队列尾 / 加下一首

全屏播放页键(沿现有播放屏语义):
- `空格`:播放/暂停
- `← →`:∓10s(Shift ∓30s)
- `↑ ↓`:音量 ±5
- `m`:静音
- `n` / `p`:下一首 / 上一首
- `r`:循环模式切换
- `t`:歌词翻译开关(proto Lyric 已有 translation,零成本)
- `+` / `-`:歌词字号(小/中/大三档,存本地配置)
- `?`:help,`i`:info,`q`:返回

mini-player 键:
- `Enter`:进全屏播放页
- `空格`:播放/暂停

### mini-player 内容(推荐)

- 封面缩略图(20×10,走协议矩阵)
- 艺人 - 歌名
- 进度条(渐变,封面取色)
- 当前时间 / 总时长
- 播放/暂停图标 + 上下首图标(文字,不用 emoji)

### 登录态(推荐:未登录可浏览,写操作引导登录)

- 未登录进 `kite tui`:可浏览公开内容(推荐/搜索/新碟/精品歌单)
- 触发写操作(红心/收藏/创建歌单)或需登录的入口(每日推荐/FM/我的歌单)→ 弹登录引导(QR 扫码,复用 qrtui 包)
- 登录后不重启 TUI,原地刷新登录态

### 搜索交互(推荐:全局 `/` 呼出输入框)

- 任意视图按 `/` → 顶部弹出搜索输入框(bubbles/textinput)
- 结果分类型 tab:单曲 / 歌单 / 专辑 / 歌手 / 用户
- Enter 播放(单曲)或进详情页(其他)

### 播放交接(推荐:TUI 独占 player 实例)

- `kite tui` 启动时构造 player 实例,TUI 生命周期内独占
- `kite song play --id X`(CLI)独立运行,不与 TUI 共享实例(两者不同时跑)
- 未来若需 CLI 与 TUI 互通(IPC),单独立项

## 依赖选型

- **bubbletea v2**(`charm.land/bubbletea/v2`,v2 渲染管线对外部图像转义序列共存更友好)
- **lipgloss v2**(`charm.land/lipgloss/v2`,样式/布局)
- **bubbles v2**(`charm.land/bubbles/v2`,textinput/list/spinner 等组件)
- 图像解码用标准库 `image/jpeg`/`image/png`,不引第三方缩放库

> mimo-blog 的 `mimo-music` 已引入同款依赖栈(`charm.land/bubbletea/v2 v2.0.8` + `lipgloss/v2 v2.0.5`),可参考其 go.mod 锁定版本。

## 包结构

```
internal/tui/
├── client.go          # tea.Model 顶层:三栏布局 + 焦点管理
├── sidebar.go         # 侧边栏(从 registry 生成)
├── miniplayer.go      # 底部 mini-player
├── view/              # 各域视图(插件式注册)
│   ├── registry.go    # View 注册表(含未建域 nil 占位)
│   ├── search.go      # 搜索视图
│   ├── recommend.go   # 每日推荐
│   ├── playlist.go    # 歌单列表 + 详情页
│   ├── album.go       # 专辑
│   ├── ...
│   └── login.go       # TUI 内登录视图(复用 qrtui)
├── playscreen/        # 全屏播放页
│   ├── model.go       # 封面 + 歌词舞台 + 进度 + 浮层
│   ├── cover.go       # 协议矩阵(kitty/iterm2/半块)
│   ├── palette.go     # 封面取色
│   ├── lyric.go       # 歌词舞台
│   └── shimmer.go     # shimmer 加载态(从 qrtui 提取复用)
└── queue.go           # 播放队列模型
```

`internal/cli/song/play.go` 的播放逻辑(player 装配、音源解析)提取到共享层,TUI 与 CLI 共用;play.go 保留 CLI 入口(flag 解析、TTY 守卫),末端调共享层。

## Testing Decisions

好测试标准:只测外部行为(导航/键位分派/View 金线/降级链),不测动画数值与真实终端渲染。

- **Seam 1 — Model 纯化**:各 view 的 Init/Update/View 不直接碰 io;service 调用经 deps 注入(沿 play_test.go 的 deps 惯例)。
- **Seam 2 — 取色纯函数**:合成 image 断言 Palette。
- **Seam 3 — 封面渲染降级链**:渲染器接口 + 探测函数注入;断言检测矩阵、`KITE_IMAGE_PROTOCOL` 覆盖、transmit-once。
- **Seam 4 — shimmer 算法**:亮带形状、带外 tierLow、时间推进变色(参考 qrtui 的 shimmerText 测试)。
- **Seam 5 — 注册表**:未建域(New==nil)不渲染,已建域渲染;新增 view 注册后侧边栏自动出现。

不测的:真实 bubbletea Program 渲染(人工 smoke);harmonica 弹簧数值;真实 Kitty 转义在 ghostty 的效果。

## Out of Scope

- **逐字卡拉OK(yrc)**:后续增量 PRD。
- **频谱 FFT 可视化**、**sixel 图片协议**:成本与收益不成正比。
- **CLI 与 TUI 的 IPC 互通**:单独立项。
- **主题系统**(用户可切换配色):TUI 客户端用封面驱动动态色,主题系统后续。
- **play.go 手写 ANSI UI 的渐进迁移**:直接删除,工具未发布不做兼容(沿「不做旧命令兼容别名」惯例)。

## Further Notes

- 本 PRD supersede 原「单曲播放屏重写」范围——播放屏升级为 TUI 客户端的一个视图(playscreen 包),不再是独立产物。
- roadmap Phase D 段同步更新:原「第一步小验证 + 第二步全屏播放器」两步合并为「TUI 客户端」一步,范围扩大但架构统一。
- CONTEXT.md 新增「TUI 客户端」「插件式 view 注册」「shimmer 加载态」词汇。
- 人工 smoke 清单:侧边栏导航、各域列表加载、mini-player 常驻、全屏播放页(ghostty 封面高清 + shimmer 加载态 + 歌词翻译/字号)、搜索分类型、未登录浏览 + 写操作引导登录、tmux 内封面降级、`KITE_IMAGE_PROTOCOL=off`。
- 提交拆分建议(沿仓库原子性规范):
  1. go.mod 引入 charm 栈
  2. `internal/tui/playscreen` shimmer + 取色 + 封面渲染器(从 qrtui 提取复用)
  3. `internal/tui` 三栏布局骨架 + 注册表
  4. 各域 view(按域分批,每域一 commit)
  5. mini-player + 全屏播放页 + 队列
  6. CLI play.go 接入共享播放层 + 删除手写 ANSI
  7. 文档同步(CONTEXT.md / roadmap)
