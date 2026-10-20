import { beforeEach, describe, expect, it } from "vitest";
import "../src/element";
import { Player } from "../src/core/player";
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

async function mount<T extends HTMLElement>(html: string): Promise<T> {
  document.body.innerHTML = html;
  await flush();
  await flush();
  return document.body.firstElementChild as T;
}

const shadow = (el: Element) => el.shadowRoot as ShadowRoot;

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
    await flush();
    expect(player.state.playing).toBe(true);
    expect(root.querySelector(".kp")?.classList.contains("playing")).toBe(true);

    root.querySelector<HTMLButtonElement>(".bar-text")?.click();
    expect(root.querySelector(".kp")?.getAttribute("data-mode")).toBe("open");
    expect(root.querySelector<HTMLElement>(".stage")?.hidden).toBe(false);
    expect(root.querySelector(".meta-title")?.textContent).toBe("纸飞机");

    root.querySelector<HTMLButtonElement>(".dots [data-go='lyrics']")?.click();
    expect(root.querySelector(".kp")?.getAttribute("data-view")).toBe("lyrics");
    expect(root.querySelectorAll(".lines .line")).toHaveLength(3);

    root.querySelector<HTMLButtonElement>(".stage .close")?.click();
    await new Promise((r) => setTimeout(r, 500));
    expect(root.querySelector(".kp")?.getAttribute("data-mode")).toBe("bar");
    expect(root.querySelector<HTMLElement>(".stage")?.hidden).toBe(true);
  });

  it("点歌词跳到那一句", async () => {
    const el = await mount(`<kite-player playlists="100"></kite-player>`);
    const root = shadow(el);
    root.querySelector<HTMLButtonElement>(".bar-text")?.click();
    await flush();
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
    expect(player.state.mode).toBe("one");
    expect(root.querySelector(".kp")?.getAttribute("data-pm")).toBe("one");
    expect(btn?.getAttribute("aria-label")).toContain("单曲循环");
  });

  it("播放列表抽屉:列出歌曲,VIP 歌不能播并给出提示", async () => {
    const el = await mount(`<kite-player playlists="100"></kite-player>`);
    const root = shadow(el);
    root.querySelector<HTMLButtonElement>(".bar .open-queue")?.click();
    await flush();
    expect(root.querySelector(".kp")?.hasAttribute("data-sheet")).toBe(true);
    const rows = root.querySelectorAll<HTMLButtonElement>(".list .row");
    expect(rows).toHaveLength(3);
    expect(rows[0]?.classList.contains("cur")).toBe(true);
    expect(rows[2]?.classList.contains("locked")).toBe(true);
    rows[2]?.click();
    expect(root.querySelector(".toast")?.textContent).toContain("VIP");
    rows[1]?.click();
    expect(player.state.current?.id).toBe("2");
  });
});

describe("<kite-track>", () => {
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
    await flush();
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
    await flush();
    await flush();
    expect(player.state.lyricIndex).toBe(0);
    expect(root.querySelector(".lyric-line")?.textContent).toBe("风吹过来");
    expect(root.querySelector(".lyric-line")?.classList.contains("live")).toBe(false);
  });

  it("VIP 歌标为不可播,不会开始播放", async () => {
    const el = await mount(`<kite-track song="3" size="s"></kite-track>`);
    const root = shadow(el);
    expect(root.querySelector(".card")?.classList.contains("locked")).toBe(true);
    expect(root.textContent).toContain("站长登录");
    root.querySelector<HTMLButtonElement>(".play")?.click();
    await flush();
    expect(player.state.current?.id).not.toBe("3");
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
    await flush();
    expect(player.state.playlist?.id).toBe("100");
    expect(player.state.current?.id).toBe("2");
    expect(rows[1]?.classList.contains("cur")).toBe(true);
  });
});
