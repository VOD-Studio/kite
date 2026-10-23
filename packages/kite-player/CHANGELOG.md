# Changelog

All notable changes to `@violetjs/kite-player` are documented here. Versions follow [Semantic Versioning](https://semver.org/); before 1.0.0, a minor version may include breaking changes.

## Unreleased

### Fixed

- Restore cover, disc and ambient images after a failed image request when the next track's artwork loads successfully.

### Build and release

- Validate a single packed tarball in an isolated consumer, covering TypeScript declarations, ESM, React SSR, bundling and Chromium interactions.
- Publish the verified main-CI tarball through an independent player release PR and tag, without rebuilding during publication.
- Upgrade Vitest, tsup and esbuild to remove the fixable vulnerabilities reported by the dependency audit.

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
- Trial-only tracks: `Track.trial` (optional, filled by the data source; how kite-server surfaces it on `/player/v1` is pending #103) marks a stream as a trial clip; it still plays instead of being gated by `playable` alone, the bar, the queue rows and the cards label it 「仅试听」, and the marker survives snapshot restore.
- Light and dark themes (following the system by default), CSS custom properties and `::part()` for styling.
- React components (`KitePlayer`, `KiteTrack`, `KitePlaylist`) and hooks (`usePlayer`, `usePlayerState`, `usePlaybackTime`).
- `createKiteSource` for the kite-server public mode (`/player/v1`) and a `DataSource` interface for custom backends.
- ESM entry points with TypeScript declarations, and a single-file build for `<script>` tags.
