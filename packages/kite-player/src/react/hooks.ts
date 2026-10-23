import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from "react";
import { getPlayer, type Player } from "../core/player";
import { createKiteSource } from "../core/source";
import type { DataSource, PlayerState } from "../core/types";

export interface UsePlayerOptions {
  /** 公开模式地址;只在页面还没有设置数据来源时生效。 */
  api?: string;
  /** 自定义数据来源(有自己后端的应用)。 */
  source?: DataSource;
}

/**
 * usePlayer 返回页面级的播放核心。它与 <kite-player> 等元素是同一个实例,
 * 可以只用 hooks 按自己的设计系统画界面,也可以和元素混用。
 */
export function usePlayer(options: UsePlayerOptions = {}): Player {
  const player = getPlayer();
  const { api, source } = options;
  if (!player.hasSource()) {
    if (source) player.setSource(source);
    else if (api) player.setSource(createKiteSource(api));
  }
  return player;
}

/**
 * usePlayerState 订阅播放状态。传 selector 时只在选中的值变化时重新渲染。
 * time 每秒更新约 4 次;需要每帧更新(逐字歌词、平滑进度)时用 usePlaybackTime。
 */
export function usePlayerState(): PlayerState;
export function usePlayerState<T>(selector: (s: PlayerState) => T): T;
export function usePlayerState<T>(selector?: (s: PlayerState) => T): T | PlayerState {
  const player = getPlayer();
  const select = selector ?? ((s: PlayerState) => s);
  const subscribe = useCallback((fn: () => void) => player.subscribe(fn), [player]);
  // 服务端渲染时没有播放状态,返回初始值,避免水合不一致。
  return useSyncExternalStore(
    subscribe,
    () => select(player.state),
    () => select(player.state),
  );
}

/** usePlaybackTime 在播放时每帧返回当前秒数(只在调用它的组件里触发重新渲染)。 */
export function usePlaybackTime(): number {
  const player = getPlayer();
  const [time, setTime] = useState(() => player.currentTime());
  const last = useRef(time);
  useEffect(() => {
    const stop = player.onTime((t) => {
      if (Math.abs(t - last.current) < 0.03) return;
      last.current = t;
      setTime(t);
    });
    const unsub = player.subscribe((s, prev) => {
      if (s.time !== prev.time) {
        last.current = s.time;
        setTime(s.time);
      }
    });
    return () => {
      stop();
      unsub();
    };
  }, [player]);
  return time;
}
