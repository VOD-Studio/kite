// Pack exactly once. Only the bytes accepted by the external consumer become a CI artifact.
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { cp, mkdir, mkdtemp, readFile, realpath, rename, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { basename, isAbsolute, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { checkBrowser } from "./package-browser.mjs";

const root = fileURLToPath(new URL("..", import.meta.url));
const artifacts = join(root, "artifacts");
const npm = process.platform === "win32" ? "npm.cmd" : "npm";
const env = { ...process.env, npm_config_ignore_scripts: "true" };
delete env.NODE_PATH;

function run(command, args, cwd, capture = false) {
  const result = spawnSync(command, args, {
    cwd,
    env,
    encoding: "utf8",
    stdio: capture ? ["ignore", "pipe", "inherit"] : "inherit",
    timeout: 300_000,
  });
  if (result.error) throw result.error;
  assert.equal(result.status, 0, `${command} ${args.join(" ")} failed (${result.signal ?? result.status})`);
  return result.stdout?.trim();
}

function digest(bytes, algorithm, encoding = "hex") {
  return createHash(algorithm).update(bytes).digest(encoding);
}

const pkg = JSON.parse(await readFile(join(root, "package.json"), "utf8"));
const lock = JSON.parse(await readFile(join(root, "package-lock.json"), "utf8"));
const repository = await realpath(run("git", ["rev-parse", "--show-toplevel"], root, true));
const sourceSha = process.env.GITHUB_SHA ?? run("git", ["rev-parse", "HEAD"], root, true);
const runId = process.env.GITHUB_RUN_ID ?? "local";
const runAttempt = Number(process.env.GITHUB_RUN_ATTEMPT ?? "1");
assert.match(sourceSha, /^[0-9a-f]{40}$/);
assert.match(runId, /^(?:local|[1-9][0-9]*)$/);
assert.ok(Number.isSafeInteger(runAttempt) && runAttempt > 0, "Invalid GITHUB_RUN_ATTEMPT");
assert.equal(pkg.name, "@violetjs/kite-player");

await rm(artifacts, { recursive: true, force: true });
const temporary = await mkdtemp(join(tmpdir(), "kite-player-consumer-"));
try {
  const outside = relative(repository, await realpath(temporary));
  assert.ok(
    outside.startsWith(`..${process.platform === "win32" ? "\\" : "/"}`) || isAbsolute(outside),
    "Consumer must be outside the repository",
  );
  // Keep checkout links relative; only the publish candidate gets immutable repository URLs.
  const packageRoot = join(temporary, "package");
  await mkdir(packageRoot);
  for (const file of ["package.json", ...pkg.files]) {
    await cp(join(root, file), join(packageRoot, file), { recursive: true });
  }
  const imageBase = `https://raw.githubusercontent.com/VOD-Studio/kite/${sourceSha}/packages/kite-player/`;
  const readmeBase = `https://github.com/VOD-Studio/kite/blob/${sourceSha}/packages/kite-player/`;
  for (const file of ["README.md", "README.zh-CN.md"]) {
    const path = join(packageRoot, file);
    const readme = await readFile(path, "utf8");
    await writeFile(
      path,
      readme
        .replace(/src="(\.\/[^"]+)"/g, (_, path) => `src="${new URL(path, imageBase).href}"`)
        .replace(/\]\((\.\/[^)]+)\)/g, (_, path) => `](${new URL(path, readmeBase).href})`),
    );
  }
  const packed = JSON.parse(
    run(npm, ["pack", "--json", "--ignore-scripts", "--pack-destination", temporary], packageRoot, true),
  );
  assert.equal(packed.length, 1, "Expected exactly one package");
  const [entry] = packed;
  assert.equal(entry.name, pkg.name);
  assert.equal(entry.version, pkg.version);
  assert.equal(basename(entry.filename), entry.filename);
  const files = new Set(entry.files.map((file) => file.path));
  for (const file of files) {
    assert.ok(
      /^(?:dist\/[^\s]+|package\.json|README(?:\.zh-CN)?\.md|CHANGELOG\.md|LICENSE)$/.test(file),
      `Unexpected published file: ${file}`,
    );
    assert.ok(!file.split("/").includes(".."), `Unsafe published file: ${file}`);
  }
  const targets = [pkg.main, pkg.module, pkg.types, pkg.unpkg, pkg.jsdelivr];
  for (const value of Object.values(pkg.exports)) {
    targets.push(...(typeof value === "string" ? [value] : Object.values(value)));
  }
  for (const target of targets) {
    assert.ok(typeof target === "string" && target.startsWith("./"));
    assert.ok(files.has(target.slice(2)), `Missing published entry point: ${target}`);
  }
  for (const file of ["LICENSE", "README.md", "README.zh-CN.md", "CHANGELOG.md"]) {
    assert.ok(files.has(file), `Missing package documentation: ${file}`);
  }
  const tarball = join(temporary, "kite-player.tgz");
  await rename(join(temporary, entry.filename), tarball);
  const original = await readFile(tarball);
  const sha256 = digest(original, "sha256");
  const integrity = `sha512-${digest(original, "sha512", "base64")}`;
  assert.equal(entry.integrity, integrity, "npm pack integrity does not match the packed bytes");

  const consumer = join(temporary, "consumer");
  await cp(join(root, "scripts/consumer"), consumer, { recursive: true });
  const dependencies = { [pkg.name]: "file:../kite-player.tgz" };
  // Resolve explicit consumer tool/peer versions from the reviewed repository lock, never from its node_modules.
  for (const name of ["react", "react-dom", "@types/react", "@types/react-dom", "typescript", "esbuild"]) {
    const version = lock.packages[`node_modules/${name}`]?.version;
    assert.match(version ?? "", /^\d+\.\d+\.\d+$/);
    dependencies[name] = version;
  }
  await writeFile(
    join(consumer, "package.json"),
    `${JSON.stringify({ name: "kite-player-external-consumer", private: true, type: "module", dependencies }, null, 2)}\n`,
  );
  console.log("Installing the tarball in an isolated external consumer (install scripts disabled)");
  run(
    npm,
    ["install", "--ignore-scripts", "--no-audit", "--no-fund", "--registry=https://registry.npmjs.org"],
    consumer,
  );
  run(process.execPath, ["ssr.mjs"], consumer);
  run(
    process.execPath,
    [
      "node_modules/typescript/bin/tsc",
      "--noEmit",
      "--strict",
      "--module",
      "NodeNext",
      "--moduleResolution",
      "NodeNext",
      "--target",
      "ES2022",
      "--lib",
      "ES2022,DOM",
      "--jsx",
      "react-jsx",
      "browser.ts",
      "types.tsx",
    ],
    consumer,
  );
  run(process.execPath, ["bundle.mjs"], consumer);
  await checkBrowser(consumer);

  assert.equal(digest(await readFile(tarball), "sha256"), sha256, "Tarball changed during acceptance");
  await mkdir(artifacts);
  // The temporary consumer and bind-mounted artifact directory can be on different filesystems.
  const acceptedTarball = join(artifacts, "kite-player.tgz");
  await writeFile(acceptedTarball, original, { flag: "wx" });
  assert.equal(digest(await readFile(acceptedTarball), "sha256"), sha256, "Artifact differs from the accepted tarball");
  await writeFile(
    join(artifacts, "manifest.json"),
    `${JSON.stringify(
      {
        schemaVersion: 1,
        name: pkg.name,
        version: pkg.version,
        sourceSha,
        runId,
        runAttempt,
        tarball: "kite-player.tgz",
        sha256,
        integrity,
      },
      null,
      2,
    )}\n`,
  );
  console.log(`Accepted ${pkg.name}@${pkg.version}: ${sha256}`);
  console.log(`Artifacts: ${resolve(artifacts)}`);
} catch (error) {
  await rm(artifacts, { recursive: true, force: true });
  throw error;
} finally {
  await rm(temporary, { recursive: true, force: true });
}
