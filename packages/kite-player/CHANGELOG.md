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

- The bar and the panel share one width on desktop, so the panel grows upward out of the bar. Expanding and collapsing (tap, pull and immersive mode alike) moves each part on its own path: the card takes the cover's colors as soon as it leaves the bar, the cover grows out of the bar's disc (turning from round to square and upright) and stays near the card's top edge until the card is tall enough, the bar's title and buttons ride beside it and fade, and the title and controls appear below the cover, growing to full size. The desktop panel's shadow grows with the card instead of appearing at the end, and is lighter. Interrupting midway reverses from the current point.
- Immersive mode closes with the collapse button at the top left or Esc; the separate exit button is gone.
- Dragging the lyrics stops at the first and last line with a rubber band, keeps gliding after release, highlights the line at the anchor next to its time without a background block, and scrolls back to the current line after 2.5 seconds. Toggling translations fades them and moves the other lines in a wave around the current one. Translations use the same size as the original.
- On touch screens, dragging the lyrics tucks the controls away and tapping the lyrics toggles them instead of seeking. Hover styles apply only to devices that can hover.
- The background is the cover softened on a small canvas with two light spots and grain, dimmed per theme so white text stays readable; light and dark themes have their own glass for the bar and the sheet. Covers, the bar's disc and the background cross-fade on track changes.
- The large track card puts the lyrics in the right column under the title, aligned with the bottom of the cover; narrow cards center the play controls.
- Song tags (VIP, digital album, trial only) share one shape and differ only by color, in the queue and on the cards (`--kite-tag-*`).
- Lower rendering cost: progress bars, the cover and the quality badge move with transforms, and per-frame text and word-progress writes are skipped when unchanged.

### Fixed

- Cancel pending position restoration when changing tracks, replacing the audio source or destroying the player, so switching quality cannot seek a different track to the previous track's position.

## 0.1.0 - 2026-10-21

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
