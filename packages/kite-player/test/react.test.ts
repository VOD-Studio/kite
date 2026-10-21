import { createElement } from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
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
