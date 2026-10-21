import { define } from "./element/define";
import { KitePlayerElement } from "./element/kite-player";
import { KitePlaylistElement } from "./element/kite-playlist";
import { KiteTrackElement } from "./element/kite-track";

export * from "./index";
export { define, KitePlayerElement, KitePlaylistElement, KiteTrackElement };

// 导入本模块即注册元素(package.json 的 sideEffects 只列了这个入口,注册调用必须留在这里)。
define();

declare global {
  interface HTMLElementTagNameMap {
    "kite-player": KitePlayerElement;
    "kite-track": KiteTrackElement;
    "kite-playlist": KitePlaylistElement;
  }
}
