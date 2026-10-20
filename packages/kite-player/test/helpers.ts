import type { AudioLike } from "../src/core/player";
import type { DataSource, Lyrics, Playlist, Track } from "../src/core/types";

/** FakeAudio 模拟 <audio>:play/pause 同步派发事件,可以手动触发 ended / error。 */
export class FakeAudio implements AudioLike {
  src = "";
  currentTime = 0;
  duration = 200;
  paused = true;
  volume = 1;
  preload = "auto";
  srcHistory: string[] = [];
  private listeners = new Map<string, Set<() => void>>();

  play(): Promise<void> {
    if (this.src && !this.srcHistory.includes(this.src)) this.srcHistory.push(this.src);
    this.paused = false;
    this.emit("play");
    return Promise.resolve();
  }

  pause(): void {
    if (this.paused) return;
    this.paused = true;
    this.emit("pause");
  }

  addEventListener(type: string, fn: () => void): void {
    if (!this.listeners.has(type)) this.listeners.set(type, new Set());
    this.listeners.get(type)?.add(fn);
  }

  removeEventListener(type: string, fn: () => void): void {
    this.listeners.get(type)?.delete(fn);
  }

  emit(type: string): void {
    for (const fn of [...(this.listeners.get(type) ?? [])]) fn();
  }
}

export function track(id: string, over: Partial<Track> = {}): Track {
  return {
    id,
    title: "歌" + id,
    artists: [{ id: "a", name: "林间" }],
    album: { id: "al", name: "晴朗的周三", cover: "https://p1.music.126.net/c.jpg" },
    durationMs: 200_000,
    access: "free",
    playable: true,
    stream: `https://music.example.com/player/v1/tracks/${id}/stream?exp=1&sig=s`,
    lyrics: `https://music.example.com/player/v1/tracks/${id}/lyrics`,
    ...over,
  };
}

export const lyrics: Lyrics = {
  lines: [
    { time: 1000, duration: 3000, text: "第一句", translation: "first" },
    { time: 4000, duration: 3000, text: "第二句" },
    { time: 7000, text: "第三句" },
  ],
  chorus: [{ start: 62000, end: 92000 }],
  instrumental: false,
};

export function fakeSource(playlist: Playlist, extra: Record<string, Track> = {}): DataSource & { trackCalls: string[] } {
  const trackCalls: string[] = [];
  return {
    trackCalls,
    playlist: async () => playlist,
    track: async (id) => {
      trackCalls.push(id);
      const t = playlist.tracks.find((x) => x.id === id) ?? extra[id];
      if (!t) throw new Error("not found");
      return { ...t, stream: t.stream + "&fresh=1" };
    },
    lyrics: async () => lyrics,
  };
}

export const flush = () => new Promise((r) => setTimeout(r, 0));
