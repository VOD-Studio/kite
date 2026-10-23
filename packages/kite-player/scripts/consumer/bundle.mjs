import assert from "node:assert/strict";
import { realpath } from "node:fs/promises";
import { isAbsolute, relative, resolve, sep } from "node:path";
import { build } from "esbuild";

const root = await realpath(process.cwd());
const result = await build({
  entryPoints: ["browser.ts"],
  outfile: "browser.js",
  bundle: true,
  format: "esm",
  platform: "browser",
  target: "es2022",
  metafile: true,
});
for (const input of Object.keys(result.metafile.inputs)) {
  const path = relative(root, await realpath(resolve(input)));
  assert.ok(
    !isAbsolute(path) && path !== ".." && !path.startsWith(`..${sep}`),
    `Consumer bundled an input outside its installation: ${input}`,
  );
}
assert.ok(
  Object.keys(result.metafile.inputs).some((input) => input.includes("node_modules/@violetjs/kite-player/dist/")),
  "Browser must exercise the installed distribution",
);
console.log("External browser bundle accepted; every input is inside the clean consumer");
