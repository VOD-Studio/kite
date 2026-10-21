import { KitePlayerElement } from "./kite-player";
import { KitePlaylistElement } from "./kite-playlist";
import { KiteTrackElement } from "./kite-track";

/**
 * define 注册 <kite-player> / <kite-track> / <kite-playlist>。重复调用或别的脚本先注册过同名元素时跳过,
 * 服务端渲染(没有 customElements)时什么也不做。
 */
export function define(): void {
  if (typeof customElements === "undefined") return;
  const all: [string, CustomElementConstructor][] = [
    ["kite-player", KitePlayerElement],
    ["kite-track", KiteTrackElement],
    ["kite-playlist", KitePlaylistElement],
  ];
  for (const [name, ctor] of all) {
    if (!customElements.get(name)) customElements.define(name, ctor);
  }
}
