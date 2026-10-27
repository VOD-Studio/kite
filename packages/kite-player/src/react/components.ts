import { createElement, forwardRef, type HTMLAttributes } from "react";
import { define } from "../element/define";
import type { KitePlayerElement } from "../element/kite-player";
import type { KitePlaylistElement } from "../element/kite-playlist";
import type { KiteTrackElement } from "../element/kite-track";

// 三个组件只是对应自定义元素的薄封装:属性都是字符串,React 18 / 19 都按 attribute 传下去。
// 元素在第一次渲染时注册(服务端渲染时 customElements 不存在,define 会跳过)。

type Theme = "auto" | "light" | "dark";
type ElementProps = Omit<HTMLAttributes<HTMLElement>, "children">;

export interface KitePlayerProps extends ElementProps {
  /** 公开模式地址,如 https://music.example.com。 */
  api?: string;
  /** 播放列表里列出的歌单 ID,逗号分隔;第一个是初始队列。 */
  playlists?: string;
  theme?: Theme;
  /**
   * 页面向下滚动时自动收成圆盘;设为 "none" 则保持展开。
   * @defaultValue "scroll"
   */
  collapse?: "scroll" | "none";
  /**
   * 首次挂载时的初始形态,仅在挂载时读取一次,之后不随属性变化重置。
   * @defaultValue "bar"
   */
  initial?: "bar" | "orb";
  /**
   * 播放器在桌面上的停靠侧;手机上始终固定为底部通栏。
   * @defaultValue "right"
   */
  position?: "right" | "left" | "center";
}

export interface KiteTrackProps extends ElementProps {
  /** 歌曲 ID。 */
  song: string;
  size?: "s" | "m" | "l";
  theme?: Theme;
  api?: string;
}

export interface KitePlaylistProps extends ElementProps {
  /** 歌单 ID。 */
  playlist: string;
  size?: "m" | "l";
  theme?: Theme;
  api?: string;
}

function wrap<E extends HTMLElement, P extends object>(tag: string, displayName: string) {
  const component = forwardRef<E, P>((props, ref) => {
    define();
    return createElement(tag, { ...props, ref });
  });
  component.displayName = displayName;
  return component;
}

/** <KitePlayer>:页面级的悬浮播放器,ref 拿到元素后可以调 open() / close()。 */
export const KitePlayer = wrap<KitePlayerElement, KitePlayerProps>("kite-player", "KitePlayer");
/** <KiteTrack>:单曲卡片。 */
export const KiteTrack = wrap<KiteTrackElement, KiteTrackProps>("kite-track", "KiteTrack");
/** <KitePlaylist>:歌单卡片。 */
export const KitePlaylist = wrap<KitePlaylistElement, KitePlaylistProps>("kite-playlist", "KitePlaylist");
