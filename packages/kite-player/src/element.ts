import { KitePlayerElement } from "./element/kite-player";
import { KitePlaylistElement } from "./element/kite-playlist";
import { KiteTrackElement } from "./element/kite-track";

export * from "./index";
export { KitePlayerElement, KitePlaylistElement, KiteTrackElement };

/**
 * define 注册 <kite-player> / <kite-track> / <kite-playlist>。导入本模块时自动执行,
 * 重复导入或别的脚本先注册过同名元素时跳过(不会报错)。
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

define();

declare global {
  interface HTMLElementTagNameMap {
    "kite-player": KitePlayerElement;
    "kite-track": KiteTrackElement;
    "kite-playlist": KitePlaylistElement;
  }
}
