// 从演示页(示例数据、离线)生成 README 用的截图,写到 docs/images/*.webp。
// 用法:npm run screenshots(先构建,再运行本脚本)。需要本机装有 Chrome / Chromium,
// 路径可用环境变量 CHROME 指定。截图依赖本机字体,不同机器的结果不会逐字节相同。
import { spawn } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { root, serveStatic } from "./static.mjs";

const out = join(root, "docs/images");
mkdirSync(out, { recursive: true });

const { server, port: serverPort } = await serveStatic();
const base = `http://127.0.0.1:${serverPort}/demo/index.html`;

// ---- Chrome(CDP) ----

const chromePath =
  process.env.CHROME ??
  ["/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "/usr/bin/google-chrome", "/usr/bin/chromium", "/usr/bin/chromium-browser"].find((p) => existsSync(p));
if (!chromePath) throw new Error("找不到 Chrome,用环境变量 CHROME 指定路径");
const profile = mkdtempSync(join(tmpdir(), "kite-player-shots-"));
const port = 9300 + Math.floor(Math.random() * 600);
const chrome = spawn(chromePath, ["--headless=new", `--remote-debugging-port=${port}`, `--user-data-dir=${profile}`, "--hide-scrollbars", "--no-first-run", "about:blank"], {
  stdio: "ignore",
});
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

let target;
for (let i = 0; i < 60 && !target; i++) {
  await sleep(200);
  try {
    target = (await (await fetch(`http://127.0.0.1:${port}/json/list`)).json()).find((t) => t.type === "page");
  } catch {
    // Chrome 还没起来。
  }
}
if (!target) throw new Error("Chrome 没有启动");
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((r) => ws.addEventListener("open", r, { once: true }));
let seq = 0;
const pending = new Map();
const events = [];
ws.addEventListener("message", (e) => {
  const msg = JSON.parse(e.data);
  if (msg.id && pending.has(msg.id)) {
    pending.get(msg.id)(msg);
    pending.delete(msg.id);
  } else if (msg.method) {
    for (const fn of events.splice(0)) fn(msg);
  }
});
const send = (method, params = {}) =>
  new Promise((resolve, reject) => {
    const id = ++seq;
    pending.set(id, (m) => (m.error ? reject(new Error(`${method}: ${m.error.message}`)) : resolve(m.result)));
    ws.send(JSON.stringify({ id, method, params }));
  });
const run = async (expr) => {
  const r = await send("Runtime.evaluate", { expression: expr, awaitPromise: true, returnByValue: true });
  if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description ?? r.exceptionDetails.text);
  return r.result.value;
};
await send("Page.enable");

// 页面里用的小工具:元素(含 kite-player 的 shadow DOM 部件)在文档里的位置。
const helpers = `
  window.$kp = () => document.querySelector("kite-player");
  window.$part = (sel) => $kp().shadowRoot.querySelector(sel);
  // 截局部时隐藏页面上的其它内容,让图只有目标元素和它的阴影。
  window.$clean = (keep) => {
    document.querySelector("main").style.visibility = "hidden";
    if (keep) { keep.style.visibility = "visible"; }
  };
  window.$rect = (el, pad) => { const r = el.getBoundingClientRect(); return { x: r.left + scrollX - pad, y: r.top + scrollY - pad, width: r.width + pad * 2, height: r.height + pad * 2 }; };
`;

/** open 打开演示页:viewport、主题、减少动态效果都按截图需要设置。 */
async function open({ width = 1280, height = 800, mobile = false, dark = false } = {}) {
  await send("Emulation.setDeviceMetricsOverride", { width, height, deviceScaleFactor: 2, mobile });
  await send("Emulation.setTouchEmulationEnabled", { enabled: mobile, maxTouchPoints: mobile ? 5 : 1 });
  await send("Emulation.setEmulatedMedia", { features: [{ name: "prefers-color-scheme", value: dark ? "dark" : "light" }] });
  const loaded = new Promise((r) => events.push(function wait(m) { if (m.method === "Page.loadEventFired") r(); else events.push(wait); }));
  await send("Page.navigate", { url: base + (dark ? "?theme=dark" : "") });
  await loaded;
  await run(helpers);
  await sleep(1200);
}

/** shot 截图:clip 为空时截整个视口。 */
async function shot(name, clipExpr) {
  const clip = clipExpr ? { ...(await run(clipExpr)), scale: 1 } : undefined;
  const { data } = await send("Page.captureScreenshot", { format: "webp", quality: 92, ...(clip ? { clip } : {}) });
  writeFileSync(join(out, `${name}.webp`), Buffer.from(data, "base64"));
  console.log("✓", name);
}

const play = (sec) => `(() => { const p = KitePlayer.getPlayer(); void p.play(); p.seek(${sec}); })()`;

try {
  // 页面全貌:展开的面板盖在卡片上。
  await open();
  await run(`${play(52)}; $kp().open("lyrics")`);
  await sleep(2600);
  await shot("hero");

  // 播放条、圆形
  await open();
  await run(play(20));
  await sleep(1500);
  await run("$clean()");
  await shot("bar", `($rect($part(".bar"), 24))`);
  await run("window.scrollTo(0, 300)");
  await sleep(150);
  await run("window.scrollTo(0, 700)");
  await sleep(900);
  await shot("orb", `($rect($part(".bar"), 24))`);

  // 面板:封面页、歌词页
  await open();
  await run(`${play(36)}; $clean(); $kp().open("cover")`);
  await sleep(1400);
  await shot("panel-cover", `($rect($part(".stage"), 24))`);
  await run(`$kp().shadowRoot.querySelector(".dots [data-go=lyrics]").click()`);
  await sleep(1500);
  await shot("panel-lyrics", `($rect($part(".stage"), 24))`);

  // 沉浸模式
  await run(`$kp().shadowRoot.querySelector(".immersive").click()`);
  await sleep(1800);
  await shot("immersive");

  // 播放列表抽屉
  await open();
  await run(`${play(10)}; $clean(); $kp().shadowRoot.querySelector(".bar .open-queue").click()`);
  await sleep(1200);
  await shot("queue", `((() => { const a = $part(".sheet").getBoundingClientRect(), b = $part(".bar").getBoundingClientRect(); const x = Math.min(a.left, b.left) - 24, y = a.top - 24; return { x: x + scrollX, y: y + scrollY, width: Math.max(a.right, b.right) + 24 - x, height: b.bottom + 24 - y }; })())`);

  // 手机
  await open({ width: 390, height: 844, mobile: true });
  await run(`${play(36)}; $kp().open("cover")`);
  await sleep(1500);
  await shot("mobile-cover");
  await run(`$kp().shadowRoot.querySelector(".dots [data-go=lyrics]").click()`);
  await sleep(1500);
  await shot("mobile-lyrics");
  await open({ width: 390, height: 844, mobile: true });
  await run(`window.scrollTo(0, document.querySelector("#tracks").offsetTop - 12)`);
  await sleep(800);
  await shot("mobile-cards");

  // 卡片(浅色、深色)
  for (const dark of [false, true]) {
    const suffix = dark ? "-dark" : "";
    await open({ dark });
    await run(play(30));
    await sleep(1500);
    const cards = [
      ["track-s", `kite-track[size="s"]`],
      ["track-m", `kite-track[song="t1"]`],
      ["track-l", `kite-track[size="l"]`],
      ["track-locked", `kite-track[song="t7"]`],
      ["playlist-m", `kite-playlist[playlist="p1"]`],
      ["playlist-l", `kite-playlist[size="l"]`],
    ];
    for (const [name, sel] of cards) {
      if (dark && name !== "track-m" && name !== "playlist-m") continue;
      await run(`(() => { const el = document.querySelector('${sel}'); el.scrollIntoView({ block: "center" }); document.querySelectorAll("kite-track, kite-playlist").forEach((c) => (c.style.visibility = "hidden")); $clean(el); })()`);
      await sleep(500);
      await shot(name + suffix, `($rect(document.querySelector('${sel}'), 16))`);
    }
  }
} finally {
  ws.close();
  const exited = new Promise((r) => chrome.once("exit", r));
  chrome.kill();
  await exited;
  server.close();
  try {
    rmSync(profile, { recursive: true, force: true });
  } catch {
    // 临时目录留给系统清理。
  }
}
