# @violetjs/kite-player

[English](https://github.com/VOD-Studio/kite/blob/main/packages/kite-player/README.md) | 简体中文

在任意网页里放网易云音乐的播放器,数据来自你自己部署的 [kite-server](https://github.com/VOD-Studio/kite)。页面里写几个标签,就有悬浮播放器、单曲卡片和歌单卡片,它们共用同一个播放核心。

<p align="center">
  <img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/hero.webp" width="820" alt="展开的播放器显示逐字歌词,背后是带单曲卡片的页面">
</p>

- **一个页面一个播放器。** 悬浮播放条、展开面板和每张卡片都是同一个播放核心的视图:在卡片里点播放,播放条跟着变。
- **逐字歌词**,带翻译;换句时逐行错开滚动,拖动歌词可以跳到那一句,一键从副歌开始。
- **四种播放模式**(顺序、列表循环、单曲循环、随机)、系统媒体控件(锁屏、耳机按键);跨页面记住队列和进度,但不会自动播放。
- **每个位置都有合适的形态。** 毛玻璃播放条随滚动收成转动的圆形;桌面上展开成面板,手机上全屏,另有沉浸模式;卡片有三种尺寸,窄的容器里自动换成紧凑布局。
- **不能播的歌会说明原因。** VIP、数字专辑的歌不会点了没反应。
- **与框架无关。** 用 [Lit](https://lit.dev) 写的标准自定义元素:原生 HTML、Vue、Svelte、Rust(Dioxus、Leptos)等都能用,另有 React 组件与 hooks。
- **静态页面只需一个脚本**,含 Lit 在内 gzip 后约 34 KB。

## 安装

```sh
npm install @violetjs/kite-player
```

或从 CDN 引入单文件版本(全局变量 `KitePlayer`):

```html
<script src="https://cdn.jsdelivr.net/npm/@violetjs/kite-player@0.1/dist/kite-player.iife.js"></script>
```

## 快速上手

```html
<!-- 每个页面一个,例如放在公共布局里 -->
<kite-player api="https://music.example.com" playlists="3778678"></kite-player>

<!-- 页面任意位置 -->
<kite-track song="1973665667"></kite-track>
<kite-playlist playlist="3778678"></kite-playlist>
```

用打包工具时,导入一次元素即可:

```js
import "@violetjs/kite-player/element"; // 注册 <kite-player>、<kite-track>、<kite-playlist>
```

`api` 是 kite-server 公开模式的地址,写在一个元素上即可,其它元素会自己找到。歌曲和歌单必须在服务端的白名单里。服务端怎么部署见 [docs/player.md](https://github.com/VOD-Studio/kite/blob/main/docs/player.md)。

## 各种形态

截图都来自 `demo/` 里的离线演示(歌曲是虚构的,封面是插画)。

### 播放条与圆形

播放条在页面右下角(手机上在底部),副标题显示当前歌词;向下滚动时收成带进度环、会转动的圆形。

<p>
  <img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/bar.webp" width="472" alt="悬浮播放条">
  <img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/orb.webp" width="112" alt="收成圆形的播放条">
</p>

### 面板

点封面或歌名展开。左右滑动或点上方的圆点在封面页和歌词页之间切换;播放时歌词页的控制条会自动隐藏。

<p>
  <img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/panel-cover.webp" width="380" alt="面板的封面页">
  <img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/panel-lyrics.webp" width="380" alt="面板的歌词页">
</p>

### 沉浸模式

<img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/immersive.webp" width="820" alt="沉浸模式">

### 播放列表

`playlists` 里的每个歌单是一个标签;不能播的歌点击时会说明原因。

<img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/queue.webp" width="420" alt="播放列表抽屉">

### 手机

<p>
  <img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/mobile-cover.webp" width="260" alt="手机全屏,封面页">
  <img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/mobile-lyrics.webp" width="260" alt="手机全屏,歌词页">
  <img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/mobile-cards.webp" width="260" alt="手机上的紧凑卡片">
</p>

### 单曲卡片

`size="s"`、`size="m"`(默认)和 `size="l"`:

<img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/track-s.webp" width="752" alt="小卡片">
<img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/track-m.webp" width="752" alt="带当前歌词的标准卡片">
<img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/track-l.webp" width="752" alt="大卡片">

不能播放的歌:

<img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/track-locked.webp" width="752" alt="锁定的 VIP 歌曲">

### 歌单卡片

`size="m"`(默认,列出前 4 首)和 `size="l"`(列出全部,可滚动):

<img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/playlist-m.webp" width="752" alt="标准歌单卡片">
<img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/playlist-l.webp" width="752" alt="大歌单卡片">

### 深色主题

默认跟随系统;`theme="dark"` / `theme="light"` 可以强制指定。

<img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/track-m-dark.webp" width="752" alt="深色主题的单曲卡片">
<img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/playlist-m-dark.webp" width="752" alt="深色主题的歌单卡片">

## 元素

### `<kite-player>`

悬浮播放器,每个页面放一个。

| 属性 | 说明 |
| --- | --- |
| `api` | kite-server 公开模式的地址,如 `https://music.example.com`。 |
| `playlists` | 播放列表里作为标签列出的歌单 ID,逗号分隔。第一个是初始队列(只加载,不自动播放)。 |
| `theme` | `auto`(默认,跟随系统)/ `light` / `dark`。 |
| `collapse` | `scroll`(默认,向下滚动时收成圆形)/ `none`。 |

方法:`open(view?: "cover" | "lyrics"): Promise<void>` 展开播放器;`close()` 收起。

### `<kite-track>`

| 属性 | 说明 |
| --- | --- |
| `song` | 歌曲 ID。 |
| `size` | `s`(一行)、`m`(默认,带当前歌词和完整控制)或 `l`(大卡片,封面环境色)。 |
| `theme`、`api` | 同 `<kite-player>`。 |

在卡片里点播放,会把这首插到当前歌后面;「从副歌听」直接从副歌开始。

### `<kite-playlist>`

| 属性 | 说明 |
| --- | --- |
| `playlist` | 歌单 ID。 |
| `size` | `m`(默认,前 4 首和「查看全部」)或 `l`(列出全部)。 |
| `theme`、`api` | 同 `<kite-player>`。 |

点任意一首,从那首开始播放整个歌单。

## 样式

元素用 Shadow DOM,页面的样式不会误伤它。要改外观,覆盖 CSS 变量或用 `::part()`:

```css
kite-player, kite-track, kite-playlist {
  --kite-accent: #3a7bd5; /* 强调色:进度、当前歌 */
  --kite-font: "LXGW WenKai", sans-serif;
}
kite-player {
  --kite-offset: 16px; /* 离视口边缘的距离 */
}
kite-track::part(card) {
  border-radius: 12px;
}
```

常用变量:`--kite-accent`、`--kite-font`、`--kite-fg`、`--kite-muted`、`--kite-card`、`--kite-sheet`、`--kite-offset`。可用的 part:`<kite-player>` 有 `root`、`bar`、`panel`、`lyrics`、`controls`、`queue`;两种卡片有 `card`。内部类名不是公开接口。

## React

```tsx
import { KitePlayer, KiteTrack, usePlayerState, usePlaybackTime } from "@violetjs/kite-player/react";

export function Layout({ children }) {
  return (
    <>
      {children}
      <KitePlayer api="https://music.example.com" playlists="3778678" />
    </>
  );
}

export function NowPlaying() {
  const current = usePlayerState((s) => s.current);
  const time = usePlaybackTime(); // 每帧更新,只在这个组件里重新渲染
  return current ? <p>{current.title} · {Math.floor(time)}s</p> : <KiteTrack song="1973665667" size="s" />;
}
```

`KitePlayer`、`KiteTrack`、`KitePlaylist` 渲染对应的自定义元素,ref 指向元素本身(可以调播放器的 `open()`)。`usePlayer`、`usePlayerState`、`usePlaybackTime` 与元素共用同一个播放核心,可以按自己的设计画界面,也可以和元素混用。两者都支持服务端渲染,元素在客户端第一次渲染时注册。

## 播放核心

不用元素时:

```js
import { createKiteSource, getPlayer } from "@violetjs/kite-player";

const player = getPlayer({ source: createKiteSource("https://music.example.com") });
await player.playPlaylist("3778678");
player.subscribe((state) => console.log(state.current?.title, state.time));
```

常用方法:`playPlaylist(id, start?)`、`playTrack(trackOrId)`、`play()`、`pause()`、`toggle()`、`next()`、`prev()`、`seek(秒)`、`playChorus()`、`setMode("sequence" | "loop" | "one" | "shuffle")`、`setVolume(0~1)`。`subscribe` 订阅每次状态变化,`onTime` 在播放时每帧回调。状态字段见 `PlayerState` 类型。

### 自定义数据来源

有自己后端的应用可以不用 kite-server,传入一个 `DataSource`:

```ts
import { getPlayer, type DataSource } from "@violetjs/kite-player";

const source: DataSource = {
  playlist: (id) => fetch(`/api/playlists/${id}`).then((r) => r.json()),
  track: (id) => fetch(`/api/tracks/${id}`).then((r) => r.json()),
  lyrics: (track) => fetch(track.lyrics).then((r) => r.json()),
};
getPlayer({ source });
```

返回的数据结构与 `/player/v1` 相同(`Playlist`、`Track`、`Lyrics` 类型)。演示页的 `demo/demo.js` 是一个完整的离线例子,包括模拟的音频元素。

## 其它框架

三个元素是标准的自定义元素。引入脚本(或 `import "@violetjs/kite-player/element"`)之后,在 Vue、Svelte、Astro、Dioxus、Leptos 等框架里像普通标签一样写 `<kite-player>`;怎么给自定义元素传属性,以各框架的文档为准。

## 运行环境

- 按 Chrome / Edge 87、Safari 14.1、Firefox 85 及以上的特性集编写:不用容器查询,`color-mix()` 只作为增强,有降级写法。
- npm 产物是给打包工具用的 ES2022 模块;单文件版本是 ES2020。
- 自带的类型定义需要 TypeScript 5 及以上。

## 许可证

[MIT](https://github.com/VOD-Studio/kite/blob/main/packages/kite-player/LICENSE)
