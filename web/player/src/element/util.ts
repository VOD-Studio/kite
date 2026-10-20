import { createKiteSource } from "../core/source";
import { getPlayer, type Player } from "../core/player";
import type { Track } from "../core/types";

/** 转义插入 innerHTML 的文本(歌名、歌手都来自上游,不可信)。 */
export function esc(s: string | undefined | null): string {
  return String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c] ?? c);
}

export function $<T extends Element = HTMLElement>(root: ParentNode, sel: string): T {
  const el = root.querySelector<T>(sel);
  if (!el) throw new Error("kite-player: 缺少元素 " + sel);
  return el;
}

export function artists(t: Track | null | undefined): string {
  return t ? t.artists.map((a) => a.name).join(" / ") : "";
}

export function prefersReducedMotion(): boolean {
  return typeof matchMedia !== "undefined" && matchMedia("(prefers-reduced-motion: reduce)").matches;
}

/** canAnimate:系统没开「减少动态效果」且浏览器支持 Web Animations 时才做形变动画。 */
export function canAnimate(el: Element): boolean {
  return typeof (el as HTMLElement).animate === "function" && !prefersReducedMotion();
}

/** cssEscape 转义属性选择器里的值(老环境没有 CSS.escape)。 */
export function cssEscape(v: string): string {
  return typeof CSS !== "undefined" && typeof CSS.escape === "function" ? CSS.escape(v) : v.replace(/["\\]/g, "\\$&");
}

/**
 * 元素用到的播放核心。数据来源取自元素自己的 api 属性,没有时取页面里第一个带 api 的元素;
 * 都没有时由 whenSource 等待(例如站点之后用 JS 设置自定义数据来源)。
 */
export function playerFor(el: HTMLElement): Player {
  const player = getPlayer();
  if (!player.hasSource()) {
    const api = el.getAttribute("api") ?? el.ownerDocument?.querySelector("[api]")?.getAttribute("api");
    if (api) player.setSource(createKiteSource(api));
  }
  return player;
}

/** 文章卡片把封面「飞」进播放器时派发的事件(kite-player 负责动画)。 */
export const flyEvent = "kite-player:fly";
/** 文章里的歌单卡片请求在播放器里打开某个歌单。 */
export const openQueueEvent = "kite-player:open-queue";
/** 卡片请求展开播放器(大卡片的「在播放器中打开」)。 */
export const openEvent = "kite-player:open";
/** 卡片请求显示一条提示(例如点了 VIP 歌)。 */
export const toastEvent = "kite-player:toast";

export interface OpenDetail {
  view?: "cover" | "lyrics";
  immersive?: boolean;
}

export function emit<T>(type: string, detail: T): void {
  if (typeof window !== "undefined") window.dispatchEvent(new CustomEvent<T>(type, { detail }));
}

/** 不能播放时给读者的说明。 */
export function unplayableMessage(t: Track): string {
  return t.access === "vip" ? "这首是 VIP 歌曲:站长登录网易云后才能播放" : "这首歌暂时无法播放(版权限制)";
}

export interface FlyDetail {
  rect: DOMRect;
  src: string;
}

export function emitFly(from: Element | null, src: string): void {
  if (!from || typeof window === "undefined") return;
  window.dispatchEvent(new CustomEvent<FlyDetail>(flyEvent, { detail: { rect: from.getBoundingClientRect(), src } }));
}

/** 尺寸为 s / m / l 之一,不认识的值按默认处理。 */
export function sizeOf(el: HTMLElement, allowed: string[], fallback: string): string {
  const v = (el.getAttribute("size") ?? "").toLowerCase();
  return allowed.includes(v) ? v : fallback;
}
