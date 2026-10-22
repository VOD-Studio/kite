import { beforeEach, describe, expect, it } from "vitest";
import "../src/element";
import { Player } from "../src/core/player";
import { KitePlayerElement } from "../src/element";
import { withDark } from "../src/element/styles";
import type { Playlist } from "../src/core/types";
import { FakeAudio, fakeSource, flush, track } from "./helpers";

const playlist: Playlist = {
  id: "100",
  name: "写作时听",
  cover: "https://p1.music.126.net/a.jpg",
  trackCount: 3,
  tracks: [track("1", { title: "纸飞机" }), track("2", { title: "慢车" }), track("3", { title: "雨季来信", access: "vip", playable: false })],
};

let player: Player;
let audio: FakeAudio;

beforeEach(() => {
  document.body.innerHTML = "";
  audio = new FakeAudio();
  player = new Player({ audio, source: fakeSource(playlist), storageKey: false, mediaSession: false });
  (globalThis as { __kitePlayer?: Player }).__kitePlayer = player;
});

type Lit = HTMLElement & { updateComplete: Promise<boolean> };

async function mount<T extends Lit = Lit>(html: string): Promise<T> {
  document.body.innerHTML = html;
  await flush();
  await flush();
  const el = document.body.firstElementChild as T;
  await el.updateComplete;
  return el;
}

/** settle 等播放核心的异步操作和元素的重新渲染都结束。 */
async function settle(el: Lit): Promise<void> {
  await flush();
  await el.updateComplete;
}

const shadow = (el: Element) => el.shadowRoot as ShadowRoot;

describe("样式", () => {
  it("暗色变量展开成强制暗色与跟随系统两份", () => {
    expect(withDark(":host{--kite-fg:#fff}")).toBe(
      ':host([theme="dark"]){--kite-fg:#fff}@media (prefers-color-scheme: dark){:host(:not([theme="light"])){--kite-fg:#fff}}',
    );
  });

  it("元素的样式来自 .css 源文件", () => {
    const css = String(KitePlayerElement.styles);
    expect(css).toContain("--kite-accent");
    expect(css).toContain('theme="dark"');
  });
});

describe("自定义元素", () => {
  it("注册三个元素", () => {
    expect(customElements.get("kite-player")).toBeDefined();
    expect(customElements.get("kite-track")).toBeDefined();
    expect(customElements.get("kite-playlist")).toBeDefined();
  });
});

describe("<kite-player>", () => {
  it("playlists 属性加载初始队列但不自动播放", async () => {
    const el = await mount(`<kite-player playlists="100"></kite-player>`);
    const root = shadow(el);
    expect(player.state.current?.id).toBe("1");
    expect(player.state.playing).toBe(false);
    expect(root.querySelector(".kp")?.hasAttribute("data-has-track")).toBe(true);
    expect(root.querySelector(".bar-title")?.textContent).toBe("纸飞机");
  });

  it("播放键切换播放,展开与收起面板", async () => {
    const el = await mount(`<kite-player playlists="100"></kite-player>`);
    const root = shadow(el);
    root.querySelector<HTMLButtonElement>(".bar .toggle")?.click();
    await settle(el);
    expect(player.state.playing).toBe(true);
    expect(root.querySelector(".kp")?.classList.contains("playing")).toBe(true);

    root.querySelector<HTMLButtonElement>(".bar-text")?.click();
    await settle(el);
    expect(root.querySelector(".kp")?.getAttribute("data-mode")).toBe("open");
    expect(root.querySelector<HTMLElement>(".stage")?.hidden).toBe(false);
    expect(root.querySelector(".meta-title")?.textContent).toBe("纸飞机");

    root.querySelector<HTMLButtonElement>(".dots [data-go='lyrics']")?.click();
    await settle(el);
    expect(root.querySelector(".kp")?.getAttribute("data-view")).toBe("lyrics");
    expect(root.querySelectorAll(".lines .line")).toHaveLength(3);

    root.querySelector<HTMLButtonElement>(".stage .close")?.click();
    await new Promise((r) => setTimeout(r, 500));
    await settle(el);
    expect(root.querySelector(".kp")?.getAttribute("data-mode")).toBe("bar");
    expect(root.querySelector<HTMLElement>(".stage")?.hidden).toBe(true);
  });

  it("点歌词跳到那一句", async () => {
    const el = await mount(`<kite-player playlists="100"></kite-player>`);
    const root = shadow(el);
    root.querySelector<HTMLButtonElement>(".bar-text")?.click();
    await settle(el);
    root.querySelectorAll<HTMLButtonElement>(".lines .line")[1]?.click();
    expect(player.state.time).toBe(4);
    expect(player.state.lyricIndex).toBe(1);
    expect(player.state.playing).toBe(true);
    audio.emit("loadedmetadata");
    expect(audio.currentTime).toBe(4);
  });

  it("播放模式按钮循环切换", async () => {
    const el = await mount(`<kite-player playlists="100"></kite-player>`);
    const root = shadow(el);
    const btn = root.querySelector<HTMLButtonElement>(".ctl .mode");
    btn?.click();
    await settle(el);
    expect(player.state.mode).toBe("one");
    expect(root.querySelector(".kp")?.getAttribute("data-pm")).toBe("one");
    expect(btn?.getAttribute("aria-label")).toContain("单曲循环");
  });

  it("面板封面缺失时隐藏碎图,由占位样式兜底", async () => {
    const bare = track("9", { title: "无封面", album: { id: "al", name: "晴朗的周三", cover: "" } });
    player.setSource(fakeSource({ ...playlist, tracks: [bare, track("2", { title: "慢车" })] }));
    const el = await mount(`<kite-player playlists="100"></kite-player>`);
    const root = shadow(el);
    root.querySelector<HTMLButtonElement>(".bar .toggle")?.click();
    await settle(el);
    for (const sel of [".cover img", ".amb img", ".disc-img img"]) {
      const img = root.querySelector<HTMLImageElement>(sel);
      expect(img?.hasAttribute("src"), sel).toBe(false);
      img?.dispatchEvent(new Event("error"));
      expect(img?.hidden, sel).toBe(true);
    }
  });

  it("播放列表抽屉:列出歌曲,VIP 歌不能播并给出提示", async () => {
    const el = await mount(`<kite-player playlists="100"></kite-player>`);
    const root = shadow(el);
    root.querySelector<HTMLButtonElement>(".bar .open-queue")?.click();
    await settle(el);
    expect(root.querySelector(".kp")?.hasAttribute("data-sheet")).toBe(true);
    const rows = root.querySelectorAll<HTMLButtonElement>(".list .row");
    expect(rows).toHaveLength(3);
    expect(rows[0]?.classList.contains("cur")).toBe(true);
    expect(rows[2]?.classList.contains("locked")).toBe(true);
    rows[2]?.click();
    await settle(el);
    expect(root.querySelector(".toast")?.textContent).toContain("VIP");
    rows[1]?.click();
    expect(player.state.current?.id).toBe("2");
  });

  it("抽屉打开后,其它歌单的标签显示歌单名", async () => {
    const el = await mount(`<kite-player playlists="100,200"></kite-player>`);
    const root = shadow(el);
    root.querySelector<HTMLButtonElement>(".bar .open-queue")?.click();
    await settle(el);
    await settle(el);
    const tabs = [...root.querySelectorAll<HTMLButtonElement>(".tabs button")].map((b) => b.textContent);
    expect(tabs).toEqual(["写作时听", "写作时听"]);
  });

  it("试听歌:播放条与抽屉行标「仅试听」,不锁定,点击照常播放", async () => {
    const trial = track("9", { title: "远方的信", access: "vip", playable: false, trial: true });
    player.setSource(fakeSource({ ...playlist, tracks: [trial, track("2", { title: "慢车" })] }));
    const el = await mount(`<kite-player playlists="100"></kite-player>`);
    const root = shadow(el);
    expect(player.state.current?.id).toBe("9"); // 初始队列把试听歌当能播的起点
    expect(root.querySelector(".bar-line .rtag.trial")?.textContent).toBe("仅试听");

    root.querySelector<HTMLButtonElement>(".bar .open-queue")?.click();
    await settle(el);
    const rows = root.querySelectorAll<HTMLButtonElement>(".list .row");
    expect(rows[0]?.classList.contains("locked")).toBe(false);
    expect(rows[0]?.querySelector(".rtag.trial")?.textContent).toBe("仅试听");
    expect(rows[0]?.querySelector(".lock")).toBeNull(); // 显示时长而不是锁
    rows[0]?.click();
    await settle(el);
    expect(player.state.playing).toBe(true);
    expect(root.querySelector(".toast")?.textContent ?? "").not.toContain("VIP");

    root.querySelector<HTMLButtonElement>(".bar-text")?.click(); // 展开面板,封面页的标签也带「仅试听」
    await settle(el);
    expect(root.querySelector(".access-tag")?.textContent).toBe("仅试听 · VIP");
  });
});

describe("<kite-track>", () => {
  it("封面缺失或加载失败时隐藏碎图,由占位样式兜底", async () => {
    const bare = track("9", { title: "无封面", album: { id: "al", name: "晴朗的周三", cover: "" } });
    player.setSource(fakeSource({ ...playlist, tracks: [bare] }));
    for (const size of ["s", "m", "l"]) {
      const el = await mount(`<kite-track song="9" size="${size}"></kite-track>`);
      const imgs = [...shadow(el).querySelectorAll<HTMLImageElement>(".cover img, .amb img")];
      expect(imgs.length, size).toBeGreaterThan(0);
      for (const img of imgs) {
        expect(img.hasAttribute("src"), size).toBe(false);
        img.dispatchEvent(new Event("error"));
        expect(img.hidden, size).toBe(true);
      }
    }
  });

  it("三种尺寸都能渲染,点播放与播放器同步", async () => {
    for (const size of ["s", "m", "l"]) {
      const el = await mount(`<kite-track song="2" size="${size}"></kite-track>`);
      const root = shadow(el);
      expect(root.querySelector(`.card.${size}`)).not.toBeNull();
      expect(root.textContent).toContain("慢车");
    }
    const el = await mount(`<kite-track song="2" size="m"></kite-track>`);
    const root = shadow(el);
    root.querySelector<HTMLButtonElement>(".play")?.click();
    await settle(el);
    expect(player.state.current?.id).toBe("2");
    expect(root.querySelector(".card")?.classList.contains("cur")).toBe(true);
    expect(root.querySelector(".card")?.classList.contains("playing")).toBe(true);
  });

  it("署名行期间预览第一句,开始播放后不会变空", async () => {
    const credited = {
      lines: [
        { time: 0, text: "作词 : 林间" },
        { time: 1000, text: "风吹过来" },
      ],
      chorus: [],
      instrumental: false,
    };
    player.setSource({ ...fakeSource(playlist), lyrics: async () => credited });
    const el = await mount(`<kite-track song="2" size="m"></kite-track>`);
    const root = shadow(el);
    expect(root.querySelector(".lyric-line")?.textContent).toBe("风吹过来");
    root.querySelector<HTMLButtonElement>(".play")?.click();
    await settle(el);
    await settle(el);
    expect(player.state.lyricIndex).toBe(0);
    expect(root.querySelector(".lyric-line")?.textContent).toBe("风吹过来");
    expect(root.querySelector(".lyric-line")?.classList.contains("live")).toBe(false);
  });

  it("VIP 歌标为不可播,不会开始播放", async () => {
    const el = await mount(`<kite-track song="3" size="s"></kite-track>`);
    const root = shadow(el);
    expect(root.querySelector(".card")?.classList.contains("locked")).toBe(true);
    expect(root.textContent).toContain("VIP 歌曲");
    root.querySelector<HTMLButtonElement>(".play")?.click();
    await settle(el);
    expect(player.state.current?.id).not.toBe("3");
  });

  it("试听歌卡片:不锁定,带「仅试听」标签,点击照常播放", async () => {
    const trial = track("9", { title: "远方的信", access: "vip", playable: false, trial: true });
    player.setSource(fakeSource({ ...playlist, tracks: [...playlist.tracks, trial] }));
    for (const size of ["s", "m", "l"]) {
      const el = await mount(`<kite-track song="9" size="${size}"></kite-track>`);
      const root = shadow(el);
      expect(root.querySelector(".card")?.classList.contains("locked"), size).toBe(false);
      expect(root.querySelector(".tg.trial")?.textContent, size).toBe("仅试听");
    }
    const el = await mount(`<kite-track song="9" size="m"></kite-track>`);
    const root = shadow(el);
    root.querySelector<HTMLButtonElement>(".play")?.click();
    await settle(el);
    expect(player.state.current?.id).toBe("9");
    expect(player.state.playing).toBe(true);
  });

  it("加载失败时显示原因", async () => {
    const el = await mount(`<kite-track song="404"></kite-track>`);
    expect(shadow(el).textContent).toContain("暂时无法显示");
  });
});

describe("<kite-playlist>", () => {
  it("列出曲目,点某一首从那首开始播放整个歌单", async () => {
    const el = await mount(`<kite-playlist playlist="100"></kite-playlist>`);
    const root = shadow(el);
    expect(root.querySelector(".name")?.textContent).toBe("写作时听");
    const rows = root.querySelectorAll<HTMLButtonElement>(".row");
    expect(rows).toHaveLength(3);
    rows[1]?.click();
    await settle(el);
    expect(player.state.playlist?.id).toBe("100");
    expect(player.state.current?.id).toBe("2");
    expect(rows[1]?.classList.contains("cur")).toBe(true);
  });

  it("试听行带「仅试听」徽标,点击照常播放", async () => {
    const trial = track("9", { title: "远方的信", access: "vip", playable: false, trial: true });
    player.setSource(fakeSource({ ...playlist, tracks: [trial, track("2", { title: "慢车" })] }));
    const el = await mount(`<kite-playlist playlist="100"></kite-playlist>`);
    const root = shadow(el);
    const rows = root.querySelectorAll<HTMLButtonElement>(".row");
    expect(rows[0]?.classList.contains("locked")).toBe(false);
    expect(rows[0]?.querySelector(".tg.trial")?.textContent).toBe("仅试听");
    rows[0]?.click();
    await settle(el);
    expect(player.state.current?.id).toBe("9");
    expect(player.state.playing).toBe(true);
  });
});

describe("紧凑布局", () => {
  it("卡片宽度不超过阈值时宿主带 data-narrow,尺寸变了重新判断", async () => {
    const observers: ((width: number) => void)[] = [];
    const original = globalThis.ResizeObserver;
    globalThis.ResizeObserver = class {
      constructor(private cb: ResizeObserverCallback) {}
      observe(): void {
        observers.push((width) => this.cb([{ contentRect: { width } } as ResizeObserverEntry], this as unknown as ResizeObserver));
      }
      unobserve(): void {}
      disconnect(): void {}
    } as unknown as typeof ResizeObserver;
    try {
      const el = await mount(`<kite-track song="2" size="m"></kite-track>`);
      const resize = observers.at(-1);
      resize?.(500);
      expect(el.hasAttribute("data-narrow")).toBe(true);
      resize?.(600);
      expect(el.hasAttribute("data-narrow")).toBe(false);
      resize?.(490);
      el.setAttribute("size", "s");
      resize?.(490);
      expect(el.hasAttribute("data-narrow"), "小卡片的阈值是 480").toBe(false);
    } finally {
      globalThis.ResizeObserver = original;
    }
  });
});
