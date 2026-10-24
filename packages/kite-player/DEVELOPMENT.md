# 开发说明

使用说明见 [README](./README.zh-CN.md),服务端见 [docs/player.md](../../docs/player.md)。本文件说明如何开发、验证与发布,不进入 npm 包。

## 常用命令

开发需要 Node.js(大版本以仓库根 `Makefile` 的 `NODE_VERSION` 为准)与 npm。在本目录执行:

```sh
npm ci
npm run check         # 类型检查 + Biome + 测试 + 构建(仓库根 make player-check)
npm run build         # 输出 dist:ESM + 类型声明 + 单文件 IIFE
npm test              # vitest + happy-dom
npm run screenshots   # 构建后从演示页生成 docs/images/*.webp(README 的配图)
npm run demo          # 构建后起离线演示页看效果(见下文)
```

## 演示页

`demo/index.html` 默认离线运行:`demo/demo.js` 提供示例歌单、歌词和一个按真实时钟推进的模拟音频,封面是 `demo/covers/` 里的插画,不访问任何服务。构建并起静态服务:

```sh
npm run demo                  # 或仓库根的 make player-demo
# http://127.0.0.1:5174/demo/index.html
HOST=0.0.0.0 npm run demo     # 同一局域网的手机打开打印出的地址(make player-demo DEMO_HOST=0.0.0.0)
```

宽度不超过 767px 时是手机布局:桌面浏览器用开发者工具的设备模式,手势(拖动翻页、拖动歌词、点按)要开触摸模拟。动画以真机为准:无头 Chrome 默认软件光栅,面板第一次显示时会丢帧。

连接真实的 kite-server 时在地址后加参数:`?api=http://127.0.0.1:3793&playlists=歌单ID,歌单ID&songs=歌曲ID,…&playlist=歌单ID,…`。kite-server 需要开公开模式(`KITE_PUBLIC_ADDR`、`KITE_PUBLIC_PLAYLISTS`,`KITE_PUBLIC_ORIGINS` 包含演示页的地址)。

## 截图

`scripts/screenshots.mjs` 自己起静态服务,用 CDP 驱动本机的 Chrome(路径可用环境变量 `CHROME` 指定)打开演示页。先截取各个状态,再用同一个浏览器的 canvas 拼图,只将九张 README 成品写到 `docs/images/`,不需要额外图像处理依赖:

| 成品 | 内容 |
| --- | --- |
| `hero.webp`、`immersive.webp`、`queue.webp` | 演示全貌、沉浸模式、播放列表,各保留单张 |
| `bar-states.webp`、`panel-states.webp` | 播放条与圆形、封面与歌词面板,分别横向拼接 |
| `mobile-states.webp` | 手机封面、歌词、紧凑卡片三列拼接 |
| `track-cards.webp` | 小、中、大尺寸与不可播放状态,按阅读顺序排成两列 |
| `playlist-cards.webp`、`dark-cards.webp` | 歌单尺寸对照、深色卡片,分别横向拼接 |

单独的状态截图只在内存中保留,不提交第二套素材。中英文 README 共用这些成品,不要重新放多个 `<img>` 来模拟并排:GitHub 和 npm 的窄内容区会把它们拆成多行。README 保留首图,其余图库放在使用说明之后直接展示,不使用折叠块。

改了界面后运行 `npm run screenshots` 并检查成品,同时查看中英文 README 的桌面和窄屏渲染。截图依赖本机字体,不同机器的结果不会逐字节相同,所以不做逐字节比对。

## 目录

| 目录 | 内容 |
| --- | --- |
| `src/core/` | 播放核心:`Player`(队列、播放模式、错误重试、媒体控件、快照)、数据来源、歌词工具,不依赖 DOM 渲染 |
| `src/element/` | 三个 Lit 元素、图标、工具函数 |
| `src/element/styles/` | 样式源码:`base.css`(共用)、`player.css`、`card.css`,暗色变量在 `*.dark.css` |
| `src/react/` | React 组件(`components.ts`)与 hooks(`hooks.ts`) |
| `test/` | 核心、元素、React 的测试 |
| `demo/` | 离线演示页 |
| `scripts/` | 演示页的静态服务、截图脚本、npm 成品与独立消费者验收 |
| `docs/images/` | README 配图(不进 npm 包) |

## 约定

- **样式写在 `.css` 文件里**,经 `?raw` 导入:构建时由 `tsup.config.ts` 的 rawCss 压缩成字符串,测试时由 Vite 处理。暗色变量只写一遍,`styles/index.ts` 的 `withDark` 展开成 `theme="dark"` 与跟随系统两份。
- **模板由状态声明**:元素的 `render()` 只读播放状态和界面状态;需要每帧变化的部分(逐字高亮、进度、歌词行位移)在 `updated()` 和 `onTime` 回调里命令式处理,不要绑进模板,否则会被每次重新渲染覆盖。
- **兼容旧内核**:不用容器查询(卡片的紧凑布局由 `util.ts` 的 `watchNarrow` 设置 `data-narrow`),`color-mix()` 必须放在 `@supports` 里并给出降级写法(值里有 `var()` 时,不支持的浏览器不会退回上一条声明)。
- **文案面向听众**,不假设页面是博客;播放器不能播放时的说明统一用 `unplayableMessage`。
- 修改公开行为后在 `CHANGELOG.md` 的未发布版本下记一条。

## 发布

发布与恢复以仓库 [CONTRIBUTING「发布与恢复」](../../CONTRIBUTING.md#发布与恢复) 为准:player 使用独立 release PR 与 `player-v*` tag,版本由 release-please 管理;发布消费 main CI 已验收的同一份 tgz,不重新构建或打包。

源码 README 的图片与语言切换使用相对路径,让本地预览和 GitHub 当前分支使用同一份素材,不依赖尚未合入 `main` 的远程图片。

`make player-package-check` 在仓库外复制发布文件,仅将这份临时副本中的 README 相对图片和文档链接转换为源码提交 SHA 对应的绝对 URL,再 pack 一次并验收。源码 README 不回写,图片仍不进入 npm 包;发布端继续消费同一份已验收 tgz,不重新打包。这样 npm 不依赖 monorepo 相对路径解析,旧版本也不会被 `main` 后续的图片改动影响。

普通 `npm pack` / `npm publish` 不执行这一步链接转换,不能替代正式发布入口。线上图片由 GitHub 托管,要求源码提交及图片已推送到公开仓库;正式发布消费 main CI 的产物,满足这一前提。本地预览始终直接使用工作区图片。

验证源码预览时不要改写图片 URL 或拦截请求替换成本地素材:直接渲染 README,确认图库无需点击即可展示且全部图片实际加载。发布成品需检查解包后的双语 README,确认图片和语言链接绑定本次源码 SHA。
