# Changelog

All notable changes to `@violetjs/kite-player` are documented here. Versions follow [Semantic Versioning](https://semver.org/); before 1.0.0, a minor version may include breaking changes.

## 0.1.0 - Unreleased

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
