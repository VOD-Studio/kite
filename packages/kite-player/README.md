# @violetjs/kite-player

English | [简体中文](https://github.com/VOD-Studio/kite/blob/main/packages/kite-player/README.zh-CN.md)

A music player for any web page, backed by your own [kite-server](https://github.com/VOD-Studio/kite) (NetEase Cloud Music). Put a few tags on a page and you get a floating player plus track and playlist cards, all driven by one playback core.

<p align="center">
  <img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/hero.webp" width="820" alt="The expanded player showing word-by-word lyrics over a page with track cards">
</p>

- **One player per page.** The floating bar, the expanded panel and every card are views of the same core: start a song from a card and the bar follows.
- **Word-by-word lyrics** with translations, a staggered scroll when the line changes, drag to seek, and a jump to the chorus.
- **Four play modes** (sequence, repeat all, repeat one, shuffle), media keys and lock-screen controls, and the queue and position are remembered across pages without autoplaying.
- **A shape for every place.** A glass bar that shrinks to a disc while you scroll, a panel on desktop, a full screen on phones, an immersive mode, and cards in three sizes that switch to a compact layout in narrow containers.
- **Honest about what cannot play.** VIP and paid tracks say why instead of failing silently, and trial-only clips are labeled 「仅试听」 but still play.
- **Framework-agnostic.** Standard custom elements built with [Lit](https://lit.dev): plain HTML, Vue, Svelte, Rust (Dioxus, Leptos) and more, plus React components and hooks.
- **One script for static pages**, about 34 KB gzipped, Lit included.

The interface text is in Simplified Chinese.

## Install

```sh
npm install @violetjs/kite-player
```

Or load the single-file build from a CDN (exposes a global `KitePlayer`):

```html
<script src="https://cdn.jsdelivr.net/npm/@violetjs/kite-player@0.1/dist/kite-player.iife.js"></script>
```

## Quick start

```html
<!-- One per page, e.g. in your shared layout -->
<kite-player api="https://music.example.com" playlists="3778678"></kite-player>

<!-- Anywhere on the page -->
<kite-track song="1973665667"></kite-track>
<kite-playlist playlist="3778678"></kite-playlist>
```

With a bundler, import the elements once:

```js
import "@violetjs/kite-player/element"; // registers <kite-player>, <kite-track>, <kite-playlist>
```

`api` is the address of your kite-server's public mode; set it on one element and the others find it. Songs and playlists must be on the server's allowlist. Deploying the server is covered in [docs/player.md](https://github.com/VOD-Studio/kite/blob/main/docs/player.md).

## Gallery

All screenshots come from the offline demo in `demo/` (made-up songs and illustrated covers).

### Bar and disc

The bar sits in the bottom-right corner by default (`position` moves it to the bottom left or bottom center; it runs along the bottom on phones). It shows the current line of the lyrics and shrinks to a spinning disc with a progress ring as you scroll down.

<p>
  <img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/bar.webp" width="472" alt="The floating bar">
  <img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/orb.webp" width="112" alt="The bar collapsed to a disc">
</p>

### Panel

Click the cover or the title to expand. Tap the quality badge on the cover to turn high quality on or off (off plays standard quality to save data, and the choice is remembered). Swipe or use the dots to switch between the cover and the lyrics; controls fade out on the lyrics page while music plays.

<p>
  <img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/panel-cover.webp" width="380" alt="Panel, cover page">
  <img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/panel-lyrics.webp" width="380" alt="Panel, lyrics page">
</p>

### Immersive mode

Enter it from the panel's top-right button; the collapse button at the top left (or Esc) goes straight back to the bar.

<img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/immersive.webp" width="820" alt="Immersive mode">

### Queue

Every playlist listed in `playlists` gets a tab. Locked tracks explain why when clicked.

<img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/queue.webp" width="420" alt="The queue sheet">

### Phone

Full screen, growing out of the bar and shrinking back into it; pull the bar up or the cover page down and the card follows your finger. Drag sideways to switch between the cover and the lyrics, with the cover following your finger into the lyrics header. Dragging the lyrics tucks the controls away and keeps gliding after you let go, stopping at the first or last line; the time of the line in focus shows on the right, and tapping it or that line jumps there. Tap the lyrics to show or hide the controls.

<p>
  <img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/mobile-cover.webp" width="260" alt="Full screen on a phone, cover page">
  <img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/mobile-lyrics.webp" width="260" alt="Full screen on a phone, lyrics page">
  <img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/mobile-cards.webp" width="260" alt="Cards in compact layout on a phone">
</p>

### Track cards

`size="s"`, `size="m"` (default) and `size="l"`:

<img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/track-s.webp" width="752" alt="Small track card">
<img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/track-m.webp" width="752" alt="Standard track card with the current lyric">
<img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/track-l.webp" width="752" alt="Large track card">

A track that cannot play:

<img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/track-locked.webp" width="752" alt="A locked VIP track">

### Playlist cards

`size="m"` (default, first four tracks) and `size="l"` (all tracks, scrollable):

<img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/playlist-m.webp" width="752" alt="Standard playlist card">
<img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/playlist-l.webp" width="752" alt="Large playlist card">

### Dark theme

Follows the system by default; force it with `theme="dark"` or `theme="light"`.

<img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/track-m-dark.webp" width="752" alt="Track card, dark theme">
<img src="https://raw.githubusercontent.com/VOD-Studio/kite/main/packages/kite-player/docs/images/playlist-m-dark.webp" width="752" alt="Playlist card, dark theme">

## Elements

### `<kite-player>`

The floating player. Use one per page.

| Attribute | Description |
| --- | --- |
| `api` | Address of the kite-server public mode, such as `https://music.example.com`. |
| `playlists` | Playlist IDs shown as tabs in the queue, comma-separated. The first one becomes the initial queue (loaded, not played). |
| `theme` | `auto` (default, follows the system), `light` or `dark`. |
| `collapse` | `scroll` (default, shrink to a disc when scrolling down) or `none`. |
| `position` | Which side on desktop: `right` (default), `left` or `center`. The bar, the panel and the queue all sit on that side and the panel only grows upward. Phones always use the bottom edge. |

Methods: `open(view?: "cover" | "lyrics"): Promise<void>` expands the player; `close()` collapses it.

### `<kite-track>`

| Attribute | Description |
| --- | --- |
| `song` | Song ID. |
| `size` | `s` (one line), `m` (default, with the current lyric and full controls) or `l` (large, with the cover's colors). |
| `theme`, `api` | Same as on `<kite-player>`. |

Playing a card inserts the song after the current one. The chorus button starts from the chorus.

### `<kite-playlist>`

| Attribute | Description |
| --- | --- |
| `playlist` | Playlist ID. |
| `size` | `m` (default, first four tracks and a link to the rest) or `l` (all tracks). |
| `theme`, `api` | Same as on `<kite-player>`. |

Clicking a track plays the whole playlist from that track.

## Styling

The elements use Shadow DOM, so page styles do not leak in. Override CSS custom properties or use `::part()`:

```css
kite-player, kite-track, kite-playlist {
  --kite-accent: #3a7bd5; /* progress, current track */
  --kite-font: "LXGW WenKai", sans-serif;
}
kite-player {
  --kite-width: 420px; /* width of the bar and the panel on desktop, 400px by default */
  --kite-offset: 16px; /* distance from the viewport edges; or set --kite-offset-x / --kite-offset-y */
}
kite-track::part(card) {
  border-radius: 12px;
}
```

Common properties: `--kite-accent`, `--kite-font`, `--kite-fg`, `--kite-muted`, `--kite-card`, `--kite-sheet`, `--kite-width`, `--kite-offset` (or `--kite-offset-x` and `--kite-offset-y`). Tracks without a cover show a neutral placeholder with a note, colored by `--kite-ph-bg` and `--kite-ph-fg`. Parts: `root`, `bar`, `panel`, `lyrics`, `controls` and `queue` on `<kite-player>`; `card` on both cards. Internal class names are not part of the API.

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
  const time = usePlaybackTime(); // updates every frame, only in this component
  return current ? <p>{current.title} · {Math.floor(time)}s</p> : <KiteTrack song="1973665667" size="s" />;
}
```

`KitePlayer`, `KiteTrack` and `KitePlaylist` render the custom elements and forward a ref to them (call `open()` on the player). `usePlayer`, `usePlayerState` and `usePlaybackTime` share the same core, so you can draw your own UI and mix it with the elements. Both work with server-side rendering; the elements register on the first client render.

## Playback core

Without the elements:

```js
import { createKiteSource, getPlayer } from "@violetjs/kite-player";

const player = getPlayer({ source: createKiteSource("https://music.example.com") });
await player.playPlaylist("3778678");
player.subscribe((state) => console.log(state.current?.title, state.time));
```

Methods: `playPlaylist(id, start?)`, `playTrack(trackOrId)`, `play()`, `pause()`, `toggle()`, `next()`, `prev()`, `seek(seconds)`, `playChorus()`, `setMode("sequence" | "loop" | "one" | "shuffle")`, `setVolume(0..1)`. `subscribe` receives every state change; `onTime` is called on every animation frame while playing. See the `PlayerState` type for all fields.

### Custom data source

Apps with their own backend can skip kite-server and pass a `DataSource`:

```ts
import { getPlayer, type DataSource } from "@violetjs/kite-player";

const source: DataSource = {
  playlist: (id) => fetch(`/api/playlists/${id}`).then((r) => r.json()),
  track: (id) => fetch(`/api/tracks/${id}`).then((r) => r.json()),
  lyrics: (track) => fetch(track.lyrics).then((r) => r.json()),
};
getPlayer({ source });
```

Responses use the same shapes as `/player/v1` (the `Playlist`, `Track` and `Lyrics` types). `quality` is the best quality the track can play at (`standard`, `exhigh`, `lossless` or `hires`) and may be left out; at `exhigh` and above the cover shows a quality badge. Also implement the optional `streamUrl(track, quality)` (a stream address at no more than that quality) and listeners can tap the badge to turn high quality off and save data; the kite-server public mode already supports it. A track may set `trial: true` when its stream is only a trial clip: it still plays, and the bar, the queue rows and the cards label it 「仅试听」. How kite-server surfaces this on `/player/v1` is pending #103, so today the field is filled by custom backends that can tell. The demo's `demo/demo.js` is a complete offline example, including a simulated audio element.

## Other frameworks

The three elements are standard custom elements. Load the script (or import `@violetjs/kite-player/element`) and write `<kite-player>` like any other tag in Vue, Svelte, Astro, Dioxus, Leptos and so on; see each framework's guide for passing attributes to custom elements.

## Requirements

- Written for the feature set of Chrome and Edge 87, Safari 14.1 and Firefox 85 or later. Container queries are not used, and `color-mix()` is only an enhancement with a fallback.
- The npm build is ES2022 modules for bundlers; the single-file build targets ES2020.
- TypeScript 5 or later for the bundled type definitions.

## License

[MIT](https://github.com/VOD-Studio/kite/blob/main/packages/kite-player/LICENSE)
