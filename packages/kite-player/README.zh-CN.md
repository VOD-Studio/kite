# @violetjs/kite-player

[English](./README.md) · 简体中文

网页音乐播放器，提供悬浮播放条、单曲与歌单卡片、逐字歌词。可以使用 Web Components、React 绑定，也可以只用播放核心。

<p align="center">
  <img src="./docs/images/hero.webp" width="820" alt="展开的播放器显示逐字歌词，旁边是单曲卡片">
</p>

- **一个页面，一个播放核心。** 播放条、面板和卡片共享队列与播放状态。
- **歌词与控制。** 翻译、歌词定位、副歌快捷播放、四种播放模式和系统媒体控件。
- **桌面与手机布局。** 收成圆形、展开为面板或全屏，也可进入沉浸模式；卡片适配窄容器。
- **连接自己的后端。** 使用自部署的 [kite-server](https://github.com/VOD-Studio/kite)，或实现自定义数据源。记住队列与进度，但不会自动播放。

播放器界面目前为简体中文。截图使用离线演示中的虚构歌曲，不连接真实音乐服务。

[快速上手](#快速上手) · [React](#react) · [元素](#元素) · [样式](#样式) · [播放核心](#播放核心) · [图库](#图库)

## 快速上手

### 接入前准备

播放器需要提供音乐数据的后端。使用 kite-server 时，先[开启公开模式](https://github.com/VOD-Studio/kite/blob/main/docs/player.md)，配置歌曲、歌单白名单，并允许网站的来源地址。`api` 填公开服务的 HTTPS 源站地址，**不是**需要鉴权的 REST 地址，也不带 `/player/v1` 后缀。

将下面的 `https://music.example.com` 和 ID 替换成你的部署地址与白名单内容。访问令牌和网易云 Cookie 只保留在服务端，不要放进页面。能否播放取决于上游账号权益和歌曲版权。

### 使用打包工具

```sh
npm install @violetjs/kite-player
```

在客户端入口导入一次：

```js
import "@violetjs/kite-player/element";
```

在公共布局放一个播放器，再按需添加卡片：

```html
<kite-player api="https://music.example.com" playlists="3778678"></kite-player>
<kite-track song="1973665667"></kite-track>
<kite-playlist playlist="3778678"></kite-playlist>
```

播放器上的 `api` 配置共享核心，卡片使用同一个数据源。原生 HTML、Vue、Svelte、Astro、Dioxus 和 Leptos 均可使用这些标准自定义元素；需要时按框架要求配置自定义元素支持。

### 静态页面

不需要打包工具。单文件版本包含 Lit，会注册自定义元素，并提供全局变量 `KitePlayer`：

```html
<script src="https://cdn.jsdelivr.net/npm/@violetjs/kite-player@0.3.0/dist/kite-player.iife.js"></script>
<kite-player api="https://music.example.com" playlists="3778678"></kite-player>
<kite-track song="1973665667"></kite-track>
```

## React

从 `@violetjs/kite-player/react` 导入，无需另外导入元素注册入口。

```tsx
import type { ReactNode } from "react";
import { KitePlayer, KiteTrack, usePlayerState, usePlaybackTime } from "@violetjs/kite-player/react";

export function Layout({ children }: { children: ReactNode }) {
  return (
    <>
      {children}
      <KitePlayer api="https://music.example.com" playlists="3778678" />
    </>
  );
}

export function NowPlaying() {
  const current = usePlayerState((state) => state.current);
  const time = usePlaybackTime(); // 每帧更新只影响这个组件。
  return current ? <p>{current.title} · {Math.floor(time)}s</p> : <KiteTrack song="1973665667" size="s" />;
}
```

`KitePlayer`、`KiteTrack` 和 `KitePlaylist` 包装自定义元素，并把 ref 转发给元素本身。`usePlayer`、`usePlayerState` 和 `usePlaybackTime` 与元素共享核心，可以混用自建界面和现成元素。React 入口支持服务端渲染，在客户端注册元素；使用 React Server Components 的框架需要把这些导出放在客户端组件中。

## 元素

### 共用属性

| 属性 | 值与行为 |
| --- | --- |
| `api` | 公开后端的源站地址，例如 `https://music.example.com`。设置在播放器上；只用卡片时，也可以设置在卡片上。 |
| `theme` | `auto`（默认，跟随系统）、`light` 或 `dark`。 |

### `<kite-player>`

每个页面放一个。会加载初始队列，但不自动开始播放。

| 属性 | 值与行为 |
| --- | --- |
| `playlists` | 逗号分隔的歌单 ID，在播放列表中显示为标签；第一个歌单是初始队列。 |
| `collapse` | `scroll`（默认，向下滚动时收成圆形）或 `none`。 |
| `position` | 桌面上为 `right`（默认）、`left` 或 `center`。播放条、面板和播放列表保持在同一侧；手机上始终位于底部。 |

方法：`open(view?: "cover" | "lyrics"): Promise<void>` 展开播放器；`close()` 收起。

### 卡片

| 元素 | 必填属性 | 尺寸 |
| --- | --- | --- |
| `<kite-track>` | `song`：歌曲 ID | `s`：单行；`m`（默认）：歌词与控制；`l`：带封面环境色的大卡片。 |
| `<kite-playlist>` | `playlist`：歌单 ID | `m`（默认）：前四首与查看全部入口；`l`：可滚动的完整列表。 |

播放单曲卡片会把歌曲插到当前歌曲之后，副歌按钮直接从副歌开始。点击歌单卡片中的歌曲，会从该曲开始播放整个歌单。无法播放时会说明原因，不会静默失败。

## 样式

Shadow DOM 将元素与页面样式隔离。通过 CSS 变量和公开 part 定制外观：

```css
kite-player, kite-track, kite-playlist {
  --kite-accent: #3a7bd5;
  --kite-font: "LXGW WenKai", sans-serif;
}
kite-player {
  --kite-width: 420px; /* 桌面播放条与面板宽度，默认 400px。 */
  --kite-offset: 16px;
}
kite-track::part(card) {
  border-radius: 12px;
}
```

| 定制项 | 变量 / part |
| --- | --- |
| 颜色与字体 | `--kite-accent`、`--kite-font`、`--kite-fg`、`--kite-muted`、`--kite-card`、`--kite-sheet` |
| 位置与宽度 | `--kite-width`、`--kite-offset`，或分别设置 `--kite-offset-x` / `--kite-offset-y` |
| 缺失封面的占位图 | `--kite-ph-bg`、`--kite-ph-fg` |
| 播放器 part | `root`、`bar`、`panel`、`lyrics`、`controls`、`queue` |
| 卡片 part | `card` |

内部类名不属于公开 API。

## 播放核心

自建界面时可以只导入核心，不注册元素。从用户操作中开始播放，以符合浏览器自动播放限制：

```html
<button id="play-music" type="button">播放歌单</button>
```

```js
import { createKiteSource, getPlayer } from "@violetjs/kite-player";

const player = getPlayer({ source: createKiteSource("https://music.example.com") });
const unsubscribe = player.subscribe((state) => console.log(state.current?.title));
document.getElementById("play-music").addEventListener("click", () => {
  void player.playPlaylist("3778678");
});
// 视图销毁时调用 unsubscribe()。
```

| 用途 | 方法 |
| --- | --- |
| 选择音乐 | `playPlaylist(id, start?)`、`playTrack(trackOrId)` |
| 播放控制 | `play()`、`pause()`、`toggle()`、`next()`、`prev()`、`seek(seconds)`、`playChorus()` |
| 播放偏好 | `setMode("sequence" \| "loop" \| "one" \| "shuffle")`、`setVolume(0..1)` |
| 订阅 | `subscribe(listener)` 订阅状态变化；`onTime(listener)` 获取逐帧播放时间 |

状态字段见导出的 `PlayerState` 类型。逐帧更新放在 `onTime` 或 React 的 `usePlaybackTime` 中，避免整个页面跟着重新渲染。

### 自定义数据源

连接自己的后端时，不用 `createKiteSource`，改为给 `getPlayer({ source })` 传入 `DataSource`：

| 方法 | 返回值 |
| --- | --- |
| `playlist(id)` | `Promise<Playlist>` |
| `track(id)` | `Promise<Track>` |
| `lyrics(track)` | `Promise<Lyrics>` |
| `info()`（可选） | `Promise<SourceInfo>` |
| `streamUrl(track, quality)`（可选） | 不超过指定音质的播放地址；提供后可切换音质。 |

数据结构以导出的[类型定义](https://github.com/VOD-Studio/kite/blob/main/packages/kite-player/src/core/types.ts)为准，`Track` 的 `stream` 和 `lyrics` 使用绝对地址。[离线演示](https://github.com/VOD-Studio/kite/blob/main/packages/kite-player/demo/demo.js)提供了包含模拟音频的完整数据源示例。

- `quality` 可选：`standard`、`exhigh`、`lossless` 或 `hires`。达到 `exhigh` 时封面显示音质标签；实现 `streamUrl` 后，听众可切换到标准音质节省流量。
- 后端确认播放流只是试听片段时，设置 `trial: true`。播放器照常播放并标记「仅试听」。kite-server 在播放响应的 `X-Kite-Trial` 头中提供试听信息，但播放器不会自动读取该响应头；自定义数据源需填充此字段。

## 图库

相关状态已拼接为单张图片，在窄页面中也不会拆成多行。点击图片可查看完整分辨率。

<details>
<summary>展开播放器布局、卡片与主题截图</summary>

### 播放条与圆形

悬浮播放条显示当前歌词，向下滚动时收成带进度环的旋转圆形。

<img src="./docs/images/bar-states.webp" width="600" alt="悬浮播放条与收起后的圆形状态，横向并排">

### 桌面面板

点击封面或标题展开，通过滑动或圆点切换封面与歌词页；歌词播放时控制区逐渐隐藏。数据源支持时，可点击音质标签切换音质。

<img src="./docs/images/panel-states.webp" width="820" alt="桌面播放器的封面页与歌词页，横向并排">

### 手机布局

全屏播放器支持封面与歌词滑动切换、拖动歌词定位，以及紧凑卡片。上拉播放条展开，下拉封面页收起；点击歌词显示或隐藏控制区。

<img src="./docs/images/mobile-states.webp" width="820" alt="手机封面页、歌词页与紧凑卡片，横向并排">

### 单曲卡片

按阅读顺序展示小、中、大尺寸和无法播放状态。

<img src="./docs/images/track-cards.webp" width="820" alt="小、中、大尺寸与无法播放的单曲卡片，两列排列">

### 歌单卡片

中尺寸显示前四首，大尺寸显示可滚动列表。

<img src="./docs/images/playlist-cards.webp" width="820" alt="中尺寸与大尺寸歌单卡片，横向并排">

### 深色主题

设置 `theme="dark"` 的单曲与歌单卡片。默认的 `auto` 跟随系统主题。

<img src="./docs/images/dark-cards.webp" width="820" alt="深色主题的单曲卡片与歌单卡片，横向并排">

### 沉浸模式

从面板右上角进入，通过收起按钮或 Escape 返回播放条。

<img src="./docs/images/immersive.webp" width="820" alt="沉浸模式中的封面与歌词">

### 播放列表

`playlists` 中的每个歌单各占一个标签，点击无法播放的歌曲会说明原因。

<img src="./docs/images/queue.webp" width="420" alt="播放条上方的播放列表抽屉">

</details>

## 兼容性与开发

- 浏览器需支持自定义元素和 Shadow DOM。npm 产物目标为 ES2022，单文件脚本目标为 ES2020。
- React 绑定需要 React 18 或更新版本；自带类型定义需要 TypeScript 5 或更新版本。
- 在仓库根目录运行 `make player-demo`，打开输出的地址即可体验离线演示。不需要后端或登录凭据，播放为模拟行为。
- [开发与截图工作流](https://github.com/VOD-Studio/kite/blob/main/packages/kite-player/DEVELOPMENT.md) · [变更记录](https://github.com/VOD-Studio/kite/blob/main/packages/kite-player/CHANGELOG.md) · [服务部署与协议](https://github.com/VOD-Studio/kite/blob/main/docs/player.md)

## 许可证

[MIT](https://github.com/VOD-Studio/kite/blob/main/packages/kite-player/LICENSE)
