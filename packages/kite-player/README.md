# @violetjs/kite-player

在博客、个人站里放网易云音乐的播放器。数据来自你**自己部署**的 kite-server 公开模式(`/player/v1`),用法类似 APlayer + Meting:页面里写几个标签就能用。

- 一个页面只有一个播放核心:悬浮播放条、展开面板、文章里的歌曲 / 歌单卡片都是它的视图,在哪里操作都同步。
- 逐字歌词、副歌跳转、四种播放模式、系统媒体控件(锁屏、耳机按键);刷新页面后记住队列和进度,但不会自动播放。
- 播放条随页面滚动收成圆形,往回滚动再展开;桌面上展开成面板,手机上全屏;另有沉浸模式。
- VIP 歌曲:站长在 kite-server 上登录网易云后可以播放;没登录时卡片会说明原因,不会出现播不了的空白。

服务端怎么部署、怎么配置歌单白名单和站长登录,见 [docs/player.md](https://github.com/VOD-Studio/kite/blob/main/docs/player.md)。

## 快速开始

任何站点都可以直接用 `<script>` 引入(全局变量 `KitePlayer`):

```html
<script src="https://cdn.jsdelivr.net/npm/@violetjs/kite-player@0.1/dist/kite-player.iife.js"></script>

<!-- 全站一个,放在 layout 里 -->
<kite-player api="https://music.example.com" playlists="3778678"></kite-player>

<!-- 文章里 -->
<kite-track song="1973665667"></kite-track>
<kite-playlist playlist="3778678"></kite-playlist>
```

用打包工具时:

```sh
npm i @violetjs/kite-player
```

```js
import "@violetjs/kite-player/element"; // 注册 <kite-player> / <kite-track> / <kite-playlist>
```

`api` 只需写在一个元素上(一般是 `<kite-player>`),其它元素会找到页面上带 `api` 的那个。歌曲、歌单 ID 必须在 kite-server 的白名单里。

## 元素

### `<kite-player>`

全站悬浮的播放器。一个页面放一个。

| 属性 | 说明 |
|---|---|
| `api` | 公开模式的根地址,如 `https://music.example.com`。 |
| `playlists` | 播放列表里列出的歌单 ID,逗号分隔。第一个是初始队列(只加载,不自动播放)。 |
| `theme` | `auto`(默认,跟随系统)/ `light` / `dark`。 |
| `collapse` | `scroll`(默认,向下滚动时收成圆形)/ `none`(不收起)。 |

方法:`open("cover" \| "lyrics")` 展开到封面或歌词页,`close()` 收起。

### `<kite-track>`

文章里的单曲卡片。

| 属性 | 说明 |
|---|---|
| `song` | 歌曲 ID。 |
| `size` | `s`(一行,适合段落之间)/ `m`(默认,带当前歌词和完整控制)/ `l`(大卡片,适合整篇围绕一首歌)。 |
| `theme` | 同上。 |

点播放会把这首插到当前歌后面并开始播放;「从副歌听」直接跳到副歌。

### `<kite-playlist>`

文章里的歌单卡片。

| 属性 | 说明 |
|---|---|
| `playlist` | 歌单 ID。 |
| `size` | `m`(默认,列出前 4 首和「查看全部」)/ `l`(列出全部,可滚动)。 |
| `theme` | 同上。 |

点任意一首从那首开始播放整个歌单。

## 样式

元素用 Shadow DOM,站点的样式不会误伤它。要改外观,覆盖 CSS 变量或用 `::part()`:

```css
kite-player, kite-track, kite-playlist {
  --kite-accent: #3a7bd5;   /* 强调色:进度、当前歌 */
  --kite-font: "LXGW WenKai", sans-serif;
}
kite-player {
  --kite-offset: 16px;      /* 播放条离视口边缘的距离 */
}
kite-track::part(card) { border-radius: 12px; }
```

常用变量:`--kite-accent`、`--kite-font`、`--kite-fg`(正文色)、`--kite-muted`(次要文字)、`--kite-card`(卡片底色)、`--kite-sheet`(播放列表底色)、`--kite-offset`。

可用的 part:`<kite-player>` 有 `root`、`bar`、`panel`、`lyrics`、`controls`、`queue`;两种卡片有 `card`。内部类名不是公开接口,升级时可能变化。

## React

```tsx
import { usePlayer, usePlayerState, usePlaybackTime } from "@violetjs/kite-player/react";

function NowPlaying() {
  const player = usePlayer({ api: "https://music.example.com" });
  const current = usePlayerState((s) => s.current);
  const playing = usePlayerState((s) => s.playing);
  const time = usePlaybackTime(); // 每帧更新,只在这个组件里重新渲染
  if (!current) return null;
  return (
    <button onClick={() => player.toggle()}>
      {playing ? "暂停" : "播放"} {current.title} {Math.floor(time)}s
    </button>
  );
}
```

hooks 拿到的和元素是同一个播放核心,可以只用 hooks 按自己的设计画界面,也可以和元素混用(比如布局里放 `<kite-player>`,文章里用自己的 React 卡片)。服务端渲染时 hooks 返回初始状态,不会造成水合不一致。

## 播放核心

不用元素和 React 时,直接用核心:

```js
import { getPlayer, createKiteSource } from "@violetjs/kite-player";

const player = getPlayer({ source: createKiteSource("https://music.example.com") });
await player.playPlaylist("3778678");
player.subscribe((state) => console.log(state.current?.title, state.time));
```

常用方法:`playPlaylist(id, start?)`、`playTrack(trackOrId)`、`play()`、`pause()`、`toggle()`、`next()`、`prev()`、`seek(秒)`、`playChorus()`、`setMode("sequence" | "loop" | "one" | "shuffle")`、`setVolume(0~1)`;`subscribe` 订阅状态,`onTime` 每帧拿到播放秒数。状态字段见类型定义 `PlayerState`。

### 自定义数据来源

已经有自己后端的站点可以不用 kite-server,实现 `DataSource` 交给播放核心:

```ts
import { getPlayer, type DataSource } from "@violetjs/kite-player";

const source: DataSource = {
  playlist: (id) => fetch(`/api/playlists/${id}`).then((r) => r.json()),
  track: (id) => fetch(`/api/tracks/${id}`).then((r) => r.json()),
  lyrics: (track) => fetch(track.lyrics).then((r) => r.json()),
};
getPlayer({ source });
```

返回的数据结构与 `/player/v1` 相同(`Playlist`、`Track`、`Lyrics` 类型)。

## 其它框架

三个元素是标准的 Custom Elements,任何能渲染自定义标签的框架都能用(Vue、Svelte、Astro、Rust 的 Dioxus / Leptos 等):在页面里引入上面的 `<script>`,然后像普通 HTML 标签一样写 `<kite-player>`。框架里怎么写自定义标签和属性,以各框架的文档为准。

## 本地开发

```sh
npm ci
npm run check   # 类型检查 + 测试 + 构建
```

样式源码在 `src/element/styles/*.css`,构建时压缩后内联进 JS(Shadow DOM 的样式要随脚本分发);暗色变量只写在 `*.dark.css` 里,构建时展开成 `theme="dark"` 与跟随系统两份。

`demo/index.html` 是本地演示页:先用公开模式启动 kite-server(`KITE_PUBLIC_ADDR=127.0.0.1:3793`、`KITE_PUBLIC_PLAYLISTS=3778678`、`KITE_PUBLIC_ORIGINS=http://127.0.0.1:5174`),`npm run build` 后在本目录起一个静态服务器(如 `python3 -m http.server 5174`),打开 `http://127.0.0.1:5174/demo/index.html`。演示页里的单曲不在歌单白名单时,加到 `KITE_PUBLIC_SONGS`。

## 许可

MIT
