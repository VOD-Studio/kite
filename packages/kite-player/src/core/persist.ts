import type { PlayMode, Track } from "./types";

/** 跨页面续播需要记住的内容(每个听众自己的浏览器里)。 */
export interface Snapshot {
  queue: Track[];
  index: number;
  time: number;
  mode: PlayMode;
  volume: number;
  playlist: { id: string; name: string } | null;
  savedAt: number;
}

/** 超过这个时间的快照不再恢复:播放链接早已过期,读者大概率也不记得听到哪了。 */
const maxAge = 7 * 24 * 3600 * 1000;

// 隐私模式、被禁用的存储都可能让 localStorage 抛错,读写都要兜底。
export function loadSnapshot(key: string): Snapshot | null {
  try {
    const raw = globalThis.localStorage?.getItem(key);
    if (!raw) return null;
    const snap = JSON.parse(raw) as Snapshot;
    if (!Array.isArray(snap.queue) || Date.now() - (snap.savedAt ?? 0) > maxAge) return null;
    return snap;
  } catch {
    return null;
  }
}

export function saveSnapshot(key: string, snap: Snapshot): void {
  try {
    globalThis.localStorage?.setItem(key, JSON.stringify(snap));
  } catch {
    // 存不了就不存,只影响跨页续播。
  }
}
