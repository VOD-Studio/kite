import { createElement } from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { KitePlayer, KitePlaylist, KiteTrack } from "../src/react";

describe("React 组件", () => {
  it("渲染成对应的自定义元素,属性按 attribute 输出(服务端渲染也可用)", () => {
    const out = renderToString(
      createElement("div", null, [
        createElement(KitePlayer, { key: "p", api: "https://music.example.com", playlists: "1,2", theme: "dark" }),
        createElement(KiteTrack, { key: "t", song: "3", size: "l" }),
        createElement(KitePlaylist, { key: "l", playlist: "4" }),
      ]),
    );
    expect(out).toContain('<kite-player api="https://music.example.com" playlists="1,2" theme="dark">');
    expect(out).toContain('<kite-track song="3" size="l">');
    expect(out).toContain('<kite-playlist playlist="4">');
  });
});

describe("usePlayerState", () => {
  it("selector 返回新对象时快照保持稳定,不相关的状态变化不触发重新渲染", async () => {
    const { act } = await import("react");
    const { createRoot } = await import("react-dom/client");
    const { Player } = await import("../src/core/player");
    const { usePlayerState } = await import("../src/react");
    const { FakeAudio, fakeSource, track } = await import("./helpers");
    (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    const playlist = { id: "1", name: "x", cover: "", trackCount: 1, tracks: [track("1")] };
    const player = new Player({ audio: new FakeAudio(), source: fakeSource(playlist), storageKey: false, mediaSession: false });
    (globalThis as { __kitePlayer?: unknown }).__kitePlayer = player;

    const errors: unknown[] = [];
    const spy = vi.spyOn(console, "error").mockImplementation((...args) => void errors.push(args));
    let renders = 0;
    function Probe() {
      const s = usePlayerState((st) => ({ playing: st.playing }));
      renders++;
      return createElement("span", null, String(s.playing));
    }
    const host = document.createElement("div");
    const root = createRoot(host);
    await act(async () => root.render(createElement(Probe)));
    const afterMount = renders;
    await act(async () => player.store.set({ volume: 0.5 })); // 与 playing 无关
    expect(renders).toBe(afterMount);
    await act(async () => player.store.set({ playing: true }));
    expect(host.textContent).toBe("true");
    await act(async () => root.unmount());
    spy.mockRestore();
    expect(errors, "React 不应报 getSnapshot 未缓存").toEqual([]);
  });
});
