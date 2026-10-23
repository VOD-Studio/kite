import assert from "node:assert/strict";
import { readFile, realpath } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { formatTime, lineIndexAt, Player } from "@violetjs/kite-player";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import "@violetjs/kite-player/element";
import { KitePlayer, KitePlaylist, KiteTrack } from "@violetjs/kite-player/react";

const packagePath = fileURLToPath(import.meta.resolve("@violetjs/kite-player/package.json"));
assert.equal(
  await realpath(packagePath),
  packagePath,
  "The consumer must install the archive, not link repository sources",
);
const pkg = JSON.parse(await readFile(packagePath, "utf8"));
assert.equal(pkg.name, "@violetjs/kite-player");
assert.equal(formatTime(125), "2:05");
assert.equal(
  lineIndexAt(
    [
      { time: 0, text: "First" },
      { time: 4000, text: "Second" },
    ],
    4500,
  ),
  1,
);
const player = new Player({ storageKey: false, mediaSession: false });
assert.deepEqual(player.state.queue, []);
assert.equal(player.state.playing, false);
assert.equal(
  renderToStaticMarkup(createElement(KitePlayer, { playlists: "offline", theme: "dark", collapse: "none" })),
  '<kite-player playlists="offline" theme="dark" collapse="none"></kite-player>',
);
assert.equal(
  renderToStaticMarkup(createElement(KiteTrack, { song: "paper", size: "l" })),
  '<kite-track song="paper" size="l"></kite-track>',
);
assert.equal(
  renderToStaticMarkup(createElement(KitePlaylist, { playlist: "offline" })),
  '<kite-playlist playlist="offline"></kite-playlist>',
);
console.log("External ESM imports and React SSR accepted");
