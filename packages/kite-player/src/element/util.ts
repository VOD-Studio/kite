import { createKiteSource } from "../core/source";
import { getPlayer, type Player } from "../core/player";
export { canPlay, unplayableMessage } from "../core/player";
import type { Track } from "../core/types";

/** $ 取 root 下必定存在的元素,找不到时抛错(说明模板与代码不一致)。 */
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

/**
 * 元素用到的播放核心。数据来源取自元素自己的 api 属性,没有时取页面里第一个带 api 的元素;
 * 都没有时由 whenSource 等待(例如页面之后用 JS 设置自定义数据来源)。
 */
export function playerFor(el: HTMLElement, api?: string | null): Player {
  const player = getPlayer();
  if (!player.hasSource()) {
    const url = api || el.ownerDocument?.querySelector("[api]")?.getAttribute("api");
    if (url) player.setSource(createKiteSource(url));
  }
  return player;
}

/** 卡片把封面「飞」进播放器时派发的事件(kite-player 负责动画)。 */
export const flyEvent = "kite-player:fly";
/** 歌单卡片请求在播放器里打开某个歌单。 */
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

export interface FlyDetail {
  rect: DOMRect;
  src: string;
}

export function emitFly(from: Element | null, src: string): void {
  if (!from || typeof window === "undefined") return;
  window.dispatchEvent(new CustomEvent<FlyDetail>(flyEvent, { detail: { rect: from.getBoundingClientRect(), src } }));
}

/** 尺寸为 allowed 之一,不认识的值按默认处理。 */
export function sizeOf<T extends string>(value: string | null | undefined, allowed: readonly T[], fallback: T): T {
  const v = (value ?? "").toLowerCase() as T;
  return allowed.includes(v) ? v : fallback;
}

/** readFlag / writeFlag 读写 localStorage 里的开关;不可用时(隐私模式等)用默认值。 */
export function readFlag(key: string, fallback: boolean): boolean {
  try {
    const v = localStorage.getItem(key);
    return v === null ? fallback : v === "1";
  } catch {
    return fallback;
  }
}

export function writeFlag(key: string, v: boolean): void {
  try {
    localStorage.setItem(key, v ? "1" : "0");
  } catch {
    // 只影响下次的默认值。
  }
}

/**
 * watchNarrow 在元素宽度 ≤ limit() 时给宿主加 data-narrow(卡片的紧凑布局),返回停止函数。
 * 用 ResizeObserver 而不是 CSS 容器查询:部分仍在用的旧内核(如旧版 X5)不支持容器查询。
 */
export function watchNarrow(el: HTMLElement, limit: () => number): () => void {
  const apply = (width: number) => el.toggleAttribute("data-narrow", width > 0 && width <= limit());
  if (typeof ResizeObserver === "undefined") {
    const onResize = () => apply(el.clientWidth);
    onResize();
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }
  const ro = new ResizeObserver((entries) => {
    for (const e of entries) apply(e.contentRect.width);
  });
  ro.observe(el);
  return () => ro.disconnect();
}
