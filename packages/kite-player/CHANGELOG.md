# Changelog

All notable changes to `@violetjs/kite-player` are documented here. Versions follow [Semantic Versioning](https://semver.org/); before 1.0.0, a minor version may include breaking changes.

## Unreleased

### Added

- `position="right | left | center"` places the bar, the panel and the queue on one side on desktop, with `--kite-width` for their shared width and `--kite-offset-x` / `--kite-offset-y` for the margins (`--kite-offset` still works). Phones keep the bottom edge.
- `Track.quality` (optional): the best quality a track can play at; kite-server fills it on `/player/v1`. The cover shows a quality badge at `exhigh` and above, never for trial clips. Tapping the badge switches between high and standard quality and keeps playing from the same position; this needs the new optional `DataSource.streamUrl(track, quality)`, which the kite source implements with the stream's `quality` parameter. `PlayerState.highQuality`, `Player.setHighQuality()` and `Player.canSwitchQuality()` expose the setting, which is remembered with the queue. Turning it on lights up the cover's edge and slides the spec out from behind the label; turning it off shows dashes in the spec and tucks it back.
- Trial-only tracks: `Track.trial` (optional, filled by the data source; how kite-server surfaces it on `/player/v1` is pending #103) marks a stream as a trial clip; it still plays instead of being gated by `playable` alone, the bar, the queue rows and the cards label it 「仅试听」, and the marker survives snapshot restore.
- A neutral placeholder with a note wherever a cover is missing or fails to load (the bar, the panel, the lyrics header and the cards), colored by `--kite-ph-bg` and `--kite-ph-fg`.
- On phones, the bar can be pulled up to open and the cover page pulled down to close, with the card following the finger; the cover and lyrics pages follow the finger too.

### Changed

- The bar and panel share one desktop width. The panel itself morphs into the bar: the cover becomes the disc and its progress ring, the title and controls settle into bar layout, and the background and unscaled shadows gradually change from panel to glass throughout the motion. The real bar stays hidden until the panel lands, including while holding a phone drag at its endpoint. Button-triggered collapse eases out over 420ms; interrupting midway reverses from the current point.
- Immersive mode closes with the collapse button at the top left or Esc; the separate exit button is gone.
- Dragging the lyrics stops at the first and last line with a rubber band, keeps gliding after release, highlights the line at the anchor next to its time without a background block, and scrolls back to the current line after 2.5 seconds. Toggling translations fades them and moves the other lines in a wave around the current one. Translations use the same size as the original.
- On touch screens, dragging the lyrics tucks the controls away and tapping the lyrics toggles them instead of seeking. Hover styles apply only to devices that can hover.
- The background is the cover softened on a small canvas with two light spots and grain, dimmed per theme so white text stays readable; light and dark themes have their own glass for the bar and the sheet. Covers, the bar's disc and the background cross-fade on track changes.
- The large track card puts the lyrics in the right column under the title, aligned with the bottom of the cover; narrow cards center the play controls.
- Song tags (VIP, digital album, trial only) share one shape and differ only by color, in the queue and on the cards (`--kite-tag-*`).
- Lower rendering cost: progress bars, the cover and the quality badge move with transforms, and per-frame text and word-progress writes are skipped when unchanged.

### Fixed

- Keep the desktop bar and collapsed orb at the same 72px height, with shared disc padding and correct centering; phones retain their 58px height.
- Keep bar text and controls in a full-width content layer while the shell folds into an orb, preventing controls from sliding over the disc. Left, center and right positions share a quick 280ms fold with a horizontal jelly-like squash and rebound settling within 360ms; height, bottom edge and anchor stay fixed, and the disc remains inside the capsule.
- Cancel pending position restoration when changing tracks, replacing the audio source or destroying the player, so switching quality cannot seek a different track to the previous track's position.
- Restore cover, disc and ambient images after a failed image request when the next track's artwork loads successfully.

### Build and release

- Validate a single packed tarball in an isolated consumer, covering TypeScript declarations, ESM, React SSR, bundling and Chromium interactions.
- Publish the verified main-CI tarball through an independent player release PR and tag, without rebuilding during publication.
- Upgrade Vitest, tsup and esbuild to remove the fixable vulnerabilities reported by the dependency audit.

## [0.4.1](https://github.com/VOD-Studio/kite/compare/player-v0.4.0...player-v0.4.1) (2026-10-27)


### 问题修复

* **player:** React 组件声明补上 initial 属性 ([#152](https://github.com/VOD-Studio/kite/issues/152)) ([163627d](https://github.com/VOD-Studio/kite/commit/163627dfbded7d6992048ca196cf48b05a757b41))

## [0.4.0](https://github.com/VOD-Studio/kite/compare/player-v0.3.0...player-v0.4.0) (2026-10-27)


### 新功能

* **player:** 新增 initial 属性支持默认收起为圆盘 ([d26f6cc](https://github.com/VOD-Studio/kite/commit/d26f6cc5c35fd7e598cf2477f2cde430afaec598))


### 问题修复

* **player:** 兼容无 ResizeObserver 内核并修复连点 tab 的滚动竞态 ([895f90c](https://github.com/VOD-Studio/kite/commit/895f90ce69ed66682438293bc2ccbb25568471fa))
* **player:** 抽屉歌单标签改横滑并修复模式行对齐 ([a973ec5](https://github.com/VOD-Studio/kite/commit/a973ec5f70a534a09f032bc213a35c601562f0f9))
* **player:** 抽屉歌单标签改横滑并修复模式行对齐 ([9a1b57d](https://github.com/VOD-Studio/kite/commit/9a1b57d6dacfe47addb8420e2ce8e6e946181909))
* **player:** 歌单标签放得下时恢复均分布局 ([d80ec03](https://github.com/VOD-Studio/kite/commit/d80ec03d7056fbcc8ff15c5989ac013a86a0bfb1))


### 构建与依赖

* **deps:** bump happy-dom from 20.14.6 to 21.0.4 in /packages/kite-player ([17fc183](https://github.com/VOD-Studio/kite/commit/17fc1830373013764712594fa3c6abadb43ece4e))
* **deps:** bump happy-dom in /packages/kite-player ([824284d](https://github.com/VOD-Studio/kite/commit/824284d5dad43f1860469c008522f892cb0a1055))

## [0.3.0](https://github.com/VOD-Studio/kite/compare/player-v0.2.0...player-v0.3.0) (2026-10-23)


### 新功能

* **player:** 音质切换与 position 属性,修订展开收起、歌词滚动与光感背景 ([41386c2](https://github.com/VOD-Studio/kite/commit/41386c2a8a7bff8be53ab6b70e6e2ac05c45478e))

## [0.2.0](https://github.com/VOD-Studio/kite/compare/player-v0.1.0...player-v0.2.0) (2026-10-23)


### 新功能

* **player:** React 组件 KitePlayer / KiteTrack / KitePlaylist ([78e07fa](https://github.com/VOD-Studio/kite/commit/78e07fa7392c3f95729ff008afb10099119ebdaa))
* **player:** Track 支持试听标记与「仅试听」徽标 ([2b5277f](https://github.com/VOD-Studio/kite/commit/2b5277fab6ca1241e45e65382c1198dee701cd4f))
* **player:** Track 支持试听标记与「仅试听」徽标 ([a42d829](https://github.com/VOD-Studio/kite/commit/a42d8296d373985b95cdbc29b9e8e7e7217bfddc))
* **player:** 封面缺失或加载失败时显示占位 ([157fd73](https://github.com/VOD-Studio/kite/commit/157fd739727e74b937167a17839658aed5d580b8))
* **player:** 组件内禁划选文字与拖出图片 ([537b7d9](https://github.com/VOD-Studio/kite/commit/537b7d9c32e1d37e98aa2d419e4f884bef6ad847))
* **player:** 网页播放器 @violetjs/kite-player 与 kite-server 公开模式 ([621e44a](https://github.com/VOD-Studio/kite/commit/621e44a5e702f0a2819b40b87f7191b5054455af))
* **public:** 支持多个服务账号轮询播放 ([0523e04](https://github.com/VOD-Studio/kite/commit/0523e04cdbfa5ad47d009895d682101357e11db9))
* **public:** 支持多个服务账号轮询播放 ([527225e](https://github.com/VOD-Studio/kite/commit/527225e9e7271275c4db2d61129e59314bb15cdf)), closes [#119](https://github.com/VOD-Studio/kite/issues/119)


### 问题修复

* **player:** 封面加载失败后恢复后续歌曲图片 ([c4dae00](https://github.com/VOD-Studio/kite/commit/c4dae00f959f5c7c784c9ee9dc80ac66d7922fce))
* **player:** 抽屉标签显示歌单名,数字专辑标出原因,注册调用留在 element 入口 ([35190bb](https://github.com/VOD-Studio/kite/commit/35190bb12998fb76060f63f80f63be40015dcfaf))


### 代码重构

* **player:** element 入口移到 src/element/index.ts ([91c8ccc](https://github.com/VOD-Studio/kite/commit/91c8cccea0881bd4ee3b679e3c10fd97b47f8693))
* **player:** 三个元素改用 Lit,补旧内核降级 ([50cd382](https://github.com/VOD-Studio/kite/commit/50cd382c3c79efd564e4b91389687c690f2b25ad))
* **player:** 包目录挪到 packages/kite-player ([b6294df](https://github.com/VOD-Studio/kite/commit/b6294df79340d88a03754ebca6d1006e0caeae50))
* **player:** 样式拆成 CSS 文件,构建时压缩内联 ([7bada48](https://github.com/VOD-Studio/kite/commit/7bada4849897bf4a0bec3bad411f0a0b4bedc650))


### 构建与依赖

* **player:** 验收真实 npm 成品并修复构建链漏洞 ([689c420](https://github.com/VOD-Studio/kite/commit/689c4207fdba869082b2739c409e1cce57ff9115))

## 0.1.0

First public release.

### Added

- `<kite-player>`: a floating bar that shrinks to a disc while scrolling, an expanded panel on desktop and a full screen on phones, an immersive mode, and a queue sheet with one tab per playlist.
- Word-by-word lyrics with translations, a staggered scroll on line changes, drag-to-seek and a jump to the chorus.
- `<kite-track>` cards in three sizes (`s`, `m`, `l`) and `<kite-playlist>` cards in two (`m`, `l`), switching to a compact layout in narrow containers.
- A shared playback core: four play modes, skipping unplayable tracks, one retry with a fresh stream link, Media Session controls, and the queue and position remembered across pages without autoplaying.
- Light and dark themes (following the system by default), CSS custom properties and `::part()` for styling.
- React components (`KitePlayer`, `KiteTrack`, `KitePlaylist`) and hooks (`usePlayer`, `usePlayerState`, `usePlaybackTime`).
- `createKiteSource` for the kite-server public mode (`/player/v1`) and a `DataSource` interface for custom backends.
- ESM entry points with TypeScript declarations, and a single-file build for `<script>` tags.
