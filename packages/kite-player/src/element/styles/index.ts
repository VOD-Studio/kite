// 样式源码是同目录的 .css 文件。Shadow DOM 的样式必须随 JS 分发(<script> 引入时也只需要一个文件),
// 所以构建时把它们压缩后内联成字符串(见 tsup.config.ts 的 rawCss)。
// 使用方只需覆盖 --kite-* 变量,或用 ::part() 微调;内部类名不是公开接口。
import { unsafeCSS } from "lit";
import base from "./base.css?raw";
import card from "./card.css?raw";
import cardDark from "./card.dark.css?raw";
import player from "./player.css?raw";
import playerDark from "./player.dark.css?raw";

/**
 * withDark 把只含 :host 规则的暗色变量展开成两份:写了 theme="dark" 时强制暗色,
 * 没写 theme="light" 时跟随系统。暗色变量因此只写一遍。
 */
export function withDark(dark: string): string {
  const on = (host: string) => dark.replace(/:host\b/g, host);
  return `${on(':host([theme="dark"])')}@media (prefers-color-scheme: dark){${on(':host(:not([theme="light"]))')}}`;
}

export const playerStyles = base + player + withDark(playerDark);
export const cardStyles = base + card + withDark(cardDark);

/** Lit 的样式表(浏览器支持时用 adoptedStyleSheets,同类元素共享一份)。源码是本包自己的 CSS。 */
export const playerSheet = unsafeCSS(playerStyles);
export const cardSheet = unsafeCSS(cardStyles);
