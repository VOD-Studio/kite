import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createServer } from "node:http";
import { join } from "node:path";
import { chromium } from "playwright";

// Wait for the browser's actual finite animations, including shadow trees, rather than sleeping or cancelling them.
async function finishAnimations(page) {
  await page.evaluate(async () => {
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    const animations = new Set();
    const collect = (root) => {
      for (const element of root.querySelectorAll("*")) {
        for (const animation of element.getAnimations()) {
          if (Number.isFinite(animation.effect?.getComputedTiming().endTime)) animations.add(animation);
        }
        if (element.shadowRoot) collect(element.shadowRoot);
      }
    };
    collect(document);
    await Promise.all(
      [...animations].map((animation) =>
        animation.finished.catch((error) => {
          // A native transition may be replaced by the next layout; never cancel or fast-forward it ourselves.
          if (error.name !== "AbortError") throw error;
        }),
      ),
    );
  });
}

export async function checkBrowser(consumer) {
  const html = await readFile(join(consumer, "index.html"));
  const javascript = await readFile(join(consumer, "browser.js"));
  const server = createServer((request, response) => {
    if (request.url === "/") response.writeHead(200, { "Content-Type": "text/html; charset=utf-8" }).end(html);
    else if (request.url === "/browser.js")
      response.writeHead(200, { "Content-Type": "text/javascript; charset=utf-8" }).end(javascript);
    else response.writeHead(404).end();
  });
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const origin = `http://127.0.0.1:${server.address().port}`;
  let browser;
  try {
    browser = await chromium.launch({
      headless: true,
      executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || process.env.CHROME || undefined,
    });
    const context = await browser.newContext({
      viewport: { width: 1280, height: 900 },
      reducedMotion: "no-preference",
      serviceWorkers: "block",
    });
    const errors = [];
    await context.route("**/*", async (route) => {
      const request = route.request();
      if (new URL(request.url()).origin !== origin || request.resourceType() === "media") {
        errors.push(`Forbidden browser network request: ${request.url()}`);
        await route.abort();
      } else {
        await route.continue();
      }
    });
    const page = await context.newPage();
    page.setDefaultTimeout(10_000);
    page.on("pageerror", (error) => errors.push(error.message));
    page.on("console", (message) => {
      if (message.type() === "error") errors.push(message.text());
    });
    await page.goto(origin, { waitUntil: "networkidle" });
    const floating = page.locator("kite-player");
    const card = page.locator("kite-track");
    const playlist = page.locator("kite-playlist");
    const panel = floating.getByRole("dialog", { name: "正在播放", exact: true });
    const queue = floating.getByRole("dialog", { name: "播放列表", exact: true });
    await floating.getByRole("button", { name: "播放", exact: true }).waitFor();
    assert.deepEqual(
      await page.evaluate(() => ({
        current: window.consumerPlayer.state.current?.id,
        playing: window.consumerPlayer.state.playing,
        queue: window.consumerPlayer.state.queue.map((track) => track.id),
      })),
      { current: "paper", playing: false, queue: ["paper", "sea", "locked"] },
    );

    // Cross-component playback: a card starts the shared player and the browser's media session.
    await card.getByRole("button", { name: "播放 纸飞机", exact: true }).click();
    await floating.getByRole("button", { name: "暂停", exact: true }).waitFor();
    await page.waitForFunction(() => window.consumerPlayer.currentTime() > 0.1);
    assert.equal(await page.evaluate(() => navigator.mediaSession.playbackState), "playing");
    assert.equal(await page.evaluate(() => navigator.mediaSession.metadata.title), "纸飞机");
    await floating.getByRole("button", { name: "暂停", exact: true }).click();
    await card.getByRole("button", { name: "播放 纸飞机", exact: true }).waitFor();
    assert.equal(await page.evaluate(() => window.consumerPlayer.state.playing), false);
    assert.equal(await page.evaluate(() => navigator.mediaSession.playbackState), "paused");

    await floating.getByRole("button", { name: "下一首", exact: true }).click();
    await page.waitForFunction(() => window.consumerPlayer.state.current?.id === "sea");
    assert.equal(await page.evaluate(() => navigator.mediaSession.metadata.title), "海边的风");
    await floating.getByRole("button", { name: "播放列表", exact: true }).click();
    await queue.waitFor();
    await finishAnimations(page);
    assert.equal(await queue.getByRole("listitem").count(), 3);
    await queue.getByRole("listitem").filter({ hasText: "未解锁的歌" }).click();
    await floating.getByRole("status").filter({ hasText: "VIP" }).waitFor();
    assert.equal(await page.evaluate(() => window.consumerPlayer.state.current.id), "sea");
    await queue.getByRole("listitem").filter({ hasText: "纸飞机" }).click();
    await page.waitForFunction(
      () => window.consumerPlayer.state.current?.id === "paper" && window.consumerPlayer.state.playing,
    );
    await queue.getByRole("button", { name: "关闭", exact: true }).click();
    await finishAnimations(page);
    await floating.getByRole("button", { name: "暂停", exact: true }).click();

    // Public controls must expand, enter/leave immersive layout, and finish their real Chromium animations.
    await floating.getByRole("button", { name: "展开播放器", exact: true }).first().click();
    await panel.waitFor();
    await finishAnimations(page);
    const normal = await panel.boundingBox();
    assert.ok(normal && normal.width > 0 && normal.height > 0);
    await panel.getByRole("button", { name: "沉浸模式", exact: true }).click();
    await panel.getByRole("button", { name: "退出沉浸模式", exact: true }).waitFor();
    await finishAnimations(page);
    const immersive = await panel.boundingBox();
    assert.ok(
      immersive && immersive.width > normal.width && immersive.height >= normal.height,
      "Immersive mode must visibly enlarge the panel",
    );
    await panel.getByRole("button", { name: "退出沉浸模式", exact: true }).click();
    await panel.getByRole("button", { name: "沉浸模式", exact: true }).waitFor();
    await finishAnimations(page);
    const restored = await panel.boundingBox();
    assert.ok(
      restored && Math.abs(restored.width - normal.width) < 2 && Math.abs(restored.height - normal.height) < 2,
      "Leaving immersive mode restores the panel geometry",
    );

    await panel.getByRole("button", { name: "歌词", exact: true }).click();
    await finishAnimations(page);
    const secondLine = panel.getByRole("listitem").filter({ hasText: "风会记得它的名字" });
    await secondLine.waitFor();
    await secondLine.click();
    await page.waitForFunction(
      () => window.consumerPlayer.state.lyricIndex === 1 && window.consumerPlayer.state.playing,
    );
    const time = await page.evaluate(() => window.consumerPlayer.currentTime());
    assert.ok(time >= 20 && time < 24, `Clicking a lyric seeks to its timestamp, got ${time}`);
    await panel.getByRole("button", { name: "暂停", exact: true }).click();
    await panel.getByRole("button", { name: "显示翻译", exact: true }).click();
    assert.equal(
      await panel.getByRole("button", { name: "显示翻译", exact: true }).getAttribute("aria-pressed"),
      "false",
    );
    await panel.getByText("The wind remembers its name", { exact: true }).waitFor({ state: "hidden" });
    await panel.getByRole("button", { name: "显示翻译", exact: true }).click();
    assert.equal(
      await panel.getByRole("button", { name: "显示翻译", exact: true }).getAttribute("aria-pressed"),
      "true",
    );
    await panel.getByText("The wind remembers its name", { exact: true }).waitFor();
    await panel.getByRole("button", { name: "收起", exact: true }).click();
    await finishAnimations(page);
    await panel.waitFor({ state: "hidden" });
    await floating.getByRole("button", { name: "播放", exact: true }).waitFor();

    // Playlist cards and the next control share the queue, skipping tracks without playback rights.
    await playlist.getByRole("button", { name: "播放全部", exact: true }).click();
    await page.waitForFunction(
      () => window.consumerPlayer.state.current?.id === "paper" && window.consumerPlayer.state.playing,
    );
    await floating.getByRole("button", { name: "下一首", exact: true }).click();
    await page.waitForFunction(() => window.consumerPlayer.state.current?.id === "sea");
    await floating.getByRole("button", { name: "下一首", exact: true }).click();
    await page.waitForFunction(() => window.consumerPlayer.state.current?.id === "paper");
    await floating.getByRole("button", { name: "暂停", exact: true }).click();

    // A second expansion at a phone viewport catches stale desktop/immersive geometry after closing.
    await page.setViewportSize({ width: 390, height: 844 });
    await floating.getByRole("button", { name: "展开播放器", exact: true }).first().click();
    await panel.waitFor();
    await finishAnimations(page);
    const mobile = await panel.boundingBox();
    assert.ok(
      mobile && mobile.x >= -1 && mobile.x + mobile.width <= 391 && mobile.height > 0,
      "Mobile panel must fit the viewport",
    );
    await panel.getByRole("button", { name: "收起", exact: true }).click();
    await finishAnimations(page);
    await panel.waitFor({ state: "hidden" });
    assert.deepEqual(errors, [], "The installed player must run without browser errors or external/audio requests");
    console.log(
      "Chromium accepted: playback, media metadata, queue/rights, lyrics/seek, immersive transitions, cards and mobile reopen",
    );
  } finally {
    if (browser) await browser.close();
    await new Promise((resolve, reject) => server.close((error) => (error ? reject(error) : resolve())));
  }
}
