import { type AudioLike, type DataSource, getPlayer, type Lyrics, type Track } from "@violetjs/kite-player";
import "@violetjs/kite-player/element";

const cover = `data:image/svg+xml,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"><rect width="64" height="64" fill="#7160ce"/></svg>')}`;
const tracks: Track[] = (
  [
    { id: "paper", title: "纸飞机", access: "free", playable: true },
    { id: "sea", title: "海边的风", access: "free", playable: true },
    { id: "locked", title: "未解锁的歌", access: "vip", playable: false },
  ] satisfies Pick<Track, "id" | "title" | "access" | "playable">[]
).map((track) => ({
  ...track,
  artists: [{ id: "artist", name: "离线乐队" }],
  album: { id: "album", name: "离线专辑", cover },
  durationMs: 60_000,
  stream: `offline:${track.id}`,
  lyrics: `offline:lyrics:${track.id}`,
}));
const lyrics: Lyrics = {
  instrumental: false,
  chorus: [{ start: 20_000, end: 40_000 }],
  lines: [
    { time: 0, duration: 20_000, text: "把今天折成纸飞机", translation: "Fold today into a paper plane" },
    { time: 20_000, duration: 20_000, text: "风会记得它的名字", translation: "The wind remembers its name" },
    { time: 40_000, duration: 20_000, text: "落在有光的地方", translation: "Land somewhere full of light" },
  ],
};
const source: DataSource = {
  async playlist(id) {
    if (id !== "offline") throw new Error(`Unknown offline playlist: ${id}`);
    return { id, name: "离线歌单", cover, trackCount: tracks.length, tracks };
  },
  async track(id) {
    const track = tracks.find((candidate) => candidate.id === id);
    if (!track) throw new Error(`Unknown offline track: ${id}`);
    return track;
  },
  async lyrics() {
    return lyrics;
  },
  async info() {
    return { version: "offline-consumer", login: "none", quality: "exhigh" };
  },
};

// A deterministic AudioLike adapter, like the public offline demo: real clock/events, no audio URL fetch.
class OfflineAudio extends EventTarget implements AudioLike {
  private source = "";
  private position = 0;
  private started = 0;
  private timer: number | undefined;
  readonly duration = 60;
  paused = true;
  volume = 1;
  preload = "none";

  get src() {
    return this.source;
  }
  set src(value: string) {
    this.pause();
    this.source = value;
    this.position = 0;
    queueMicrotask(() => {
      for (const type of ["loadedmetadata", "durationchange", "canplay"]) this.dispatchEvent(new Event(type));
    });
  }
  get currentTime() {
    return Math.min(this.duration, this.position + (this.paused ? 0 : (performance.now() - this.started) / 1000));
  }
  set currentTime(value: number) {
    this.position = Math.max(0, Math.min(value, this.duration));
    this.started = performance.now();
    this.dispatchEvent(new Event("timeupdate"));
  }
  async play() {
    if (!this.source.startsWith("offline:")) throw new Error("Only offline audio is allowed");
    if (!this.paused) return;
    this.started = performance.now();
    this.paused = false;
    this.dispatchEvent(new Event("play"));
    this.dispatchEvent(new Event("playing"));
    this.timer = setInterval(() => {
      this.dispatchEvent(new Event("timeupdate"));
      if (this.currentTime >= this.duration) {
        this.pause();
        this.dispatchEvent(new Event("ended"));
      }
    }, 50);
  }
  pause() {
    if (this.paused) return;
    this.position = this.currentTime;
    this.paused = true;
    clearInterval(this.timer);
    this.dispatchEvent(new Event("pause"));
  }
}

const player = getPlayer({ source, audio: new OfflineAudio(), storageKey: false });
// Expose only the public consumer API; assertions must not inspect the audio adapter or private element state.
Object.assign(window, { consumerPlayer: player });
const card = document.createElement("kite-track");
card.song = "paper";
card.size = "m";
const playlist = document.createElement("kite-playlist");
playlist.playlist = "offline";
const floating = document.createElement("kite-player");
floating.playlists = "offline";
floating.collapse = "none";
const main = document.querySelector("main");
if (!main) throw new Error("Missing consumer fixture main element");
main.append(card, playlist);
document.body.append(floating);
