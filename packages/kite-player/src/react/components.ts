import { createElement, forwardRef, type HTMLAttributes } from "react";
import { define, type KitePlayerElement, type KitePlaylistElement, type KiteTrackElement } from "../element";

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
  /** scroll(默认):向下滚动时收成圆形;none:不收起。 */
  collapse?: "scroll" | "none";
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
