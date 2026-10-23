# Changelog

All notable changes to `@violetjs/kite-player` are documented here. Versions follow [Semantic Versioning](https://semver.org/); before 1.0.0, a minor version may include breaking changes.

## Unreleased

### Fixed

- Restore cover, disc and ambient images after a failed image request when the next track's artwork loads successfully.

### Build and release

- Validate a single packed tarball in an isolated consumer, covering TypeScript declarations, ESM, React SSR, bundling and Chromium interactions.
- Publish the verified main-CI tarball through an independent player release PR and tag, without rebuilding during publication.
- Upgrade Vitest, tsup and esbuild to remove the fixable vulnerabilities reported by the dependency audit.

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
