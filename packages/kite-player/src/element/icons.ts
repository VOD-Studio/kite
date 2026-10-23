// 图标:24 栅格,线宽 1.8,播放类实心。全部内联,不依赖图标字体或外部资源。
import { html, type SVGTemplateResult, svg, type TemplateResult } from "lit";

const stroke = (d: SVGTemplateResult, w = 1.8): TemplateResult =>
  html`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width=${w} stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${d}</svg>`;

/** 水滴形播放键的外形(64 栅格):顶边宽而平、右下收成圆钝的尖、左侧鼓出。 */
export const blobPath =
  "M16 5C26 1.5 42 1 51 3c8 1.8 11.5 9 11 19-.7 12-8 28-17 36-4.5 4.5-10.5 5-16 2.5C18 55.5 6 48 2.8 37-.5 25 5 10 16 5z";

/** 水滴播放键:同时包含播放与暂停两个图形,用 CSS 切换。 */
export const blob = html`<svg viewBox="0 0 64 64" aria-hidden="true"><path class="blob-shape" d=${blobPath}></path><path class="i-play" d="M26.5 19.3v21.4c0 1.3 1.4 2.1 2.5 1.4l16.8-10.7c1-.6 1-2.1 0-2.8L29 17.9c-1.1-.7-2.5.1-2.5 1.4z"></path><g class="i-pause"><rect x="23.6" y="18.5" width="5.6" height="21.5" rx="2.8"></rect><rect x="34.4" y="18.5" width="5.6" height="21.5" rx="2.8"></rect></g></svg>`;

export const icons = {
  prev: html`<svg viewBox="0 0 24 24" aria-hidden="true"><path fill="currentColor" d="M19 6.2v11.6c0 .8-.9 1.2-1.5.7l-7.7-5.8a.9.9 0 0 1 0-1.4l7.7-5.8c.6-.5 1.5-.1 1.5.7z"></path><rect fill="currentColor" x="5" y="5.5" width="2.4" height="13" rx="1.2"></rect></svg>`,
  next: html`<svg viewBox="0 0 24 24" aria-hidden="true"><path fill="currentColor" d="M5 6.2v11.6c0 .8.9 1.2 1.5.7l7.7-5.8a.9.9 0 0 0 0-1.4L6.5 5.5C5.9 5 5 5.4 5 6.2z"></path><rect fill="currentColor" x="16.6" y="5.5" width="2.4" height="13" rx="1.2"></rect></svg>`,
  queue: stroke(
    svg`<path d="M21 15V6"></path><path d="M18.5 18a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5z"></path><path d="M12 12H3M16 6H3M12 18H3"></path>`,
  ),
  chevron: stroke(svg`<path d="M6 9l6 6 6-6"></path>`, 2),
  expand: stroke(svg`<path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5"></path>`),
  shrink: stroke(svg`<path d="M9 4v5H4M15 4v5h5M9 20v-5H4M15 20v-5h5"></path>`),
  close: stroke(svg`<path d="M6 6l12 12M18 6L6 18"></path>`, 2),
  lock: stroke(svg`<rect x="5" y="11" width="14" height="10" rx="2"></rect><path d="M8 11V7a4 4 0 0 1 8 0v4"></path>`),
  wave: stroke(
    svg`<path d="M2 13a2 2 0 0 0 2-2V7a2 2 0 0 1 4 0v13a2 2 0 0 0 4 0V4a2 2 0 0 1 4 0v13a2 2 0 0 0 4 0v-4a2 2 0 0 1 2-2"></path>`,
  ),
  lyric: html`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><rect x="3.5" y="3.5" width="17" height="17" rx="4.5"></rect><text x="12" y="16.3" text-anchor="middle" font-size="10.5" fill="currentColor" stroke="none" font-family="PingFang SC, Noto Sans SC, sans-serif">词</text></svg>`,
  sequence: stroke(svg`<path d="M10 6h11M10 12h11M10 18h11M4 6h1v4M4 10h2M6 18H4c0-1 2-2 2-3s-1-1.5-2-1"></path>`),
  loop: stroke(
    svg`<path d="M17 2l4 4-4 4"></path><path d="M3 11v-1a4 4 0 0 1 4-4h14"></path><path d="M7 22l-4-4 4-4"></path><path d="M21 13v1a4 4 0 0 1-4 4H3"></path>`,
  ),
  one: stroke(
    svg`<path d="M17 2l4 4-4 4"></path><path d="M3 11v-1a4 4 0 0 1 4-4h14"></path><path d="M7 22l-4-4 4-4"></path><path d="M21 13v1a4 4 0 0 1-4 4H3"></path><path d="M11 10h1v4"></path>`,
  ),
  shuffle: stroke(
    svg`<path d="M18 14l4 4-4 4"></path><path d="M18 2l4 4-4 4"></path><path d="M2 18h1.97a4 4 0 0 0 3.3-1.7l5.45-7.6a4 4 0 0 1 3.3-1.7H22"></path><path d="M2 6h1.97a4 4 0 0 1 3.6 2.2"></path><path d="M22 18h-6.04a4 4 0 0 1-3.3-1.8l-.36-.45"></path>`,
  ),
  play: html`<svg viewBox="0 0 24 24" aria-hidden="true"><path fill="currentColor" d="M7 4.5v15l13-7.5z"></path></svg>`,
  note: stroke(
    svg`<path d="M9 18V5l12-2v13"></path><circle cx="6" cy="18" r="3"></circle><circle cx="18" cy="16" r="3"></circle>`,
  ),
};

/** 没有封面(或加载失败、还没加载完)时露出的占位:中性渐变 + 音符。放在 <img> 下面。 */
export const coverPh = html`<span class="ph" aria-hidden="true">${icons.note}</span>`;

/** 正在播放的均衡条。 */
export const eq = html`<span class="eq" aria-hidden="true"><i></i><i></i><i></i></span>`;
