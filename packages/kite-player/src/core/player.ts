import { lineIndexAt } from "./lyrics";
import { loadSnapshot, saveSnapshot } from "./persist";
import { Store } from "./store";
import type { DataSource, Lyrics, PlayMode, Playlist, PlayerState, SourceInfo, Track } from "./types";

/** Player 用到的 <audio> 能力子集,测试时可以换成假实现。 */
export interface AudioLike {
  src: string;
  currentTime: number;
  readonly duration: number;
  readonly paused: boolean;
  volume: number;
  preload?: string;
  play(): Promise<void>;
  pause(): void;
  addEventListener(type: string, fn: () => void): void;
  removeEventListener(type: string, fn: () => void): void;
}

export interface PlayerOptions {
  /** 数据来源;也可以之后用 setSource 设置。 */
  source?: DataSource;
  /** 自定义音频元素(测试用);默认新建一个 Audio。 */
  audio?: AudioLike;
  /** 跨页续播用的 localStorage key;false 表示不记住。 */
  storageKey?: string | false;
  /** 是否接入系统媒体控件(锁屏、耳机按键)。 */
  mediaSession?: boolean;
}

const modes: PlayMode[] = ["sequence", "loop", "one", "shuffle"];

export const modeLabels: Record<PlayMode, string> = {
  sequence: "顺序播放",
  loop: "列表循环",
  one: "单曲循环",
  shuffle: "随机播放",
};

/** 「上一首」时,播放超过这个秒数先回到开头。 */
const restartThreshold = 3;

const initialState: PlayerState = {
  queue: [],
  index: -1,
  current: null,
  playlist: null,
  playing: false,
  loading: false,
  time: 0,
  duration: 0,
  mode: "loop",
  volume: 1,
  lyrics: null,
  lyricsStatus: "idle",
  lyricIndex: -1,
  error: null,
  highQuality: true,
};

/**
 * Player 是页面里唯一的播放核心:一个 <audio>、一个队列。
 * 所有界面(播放条、面板、卡片、React 组件)都只是它的视图,在哪里操作都同步。
 */
export class Player {
  readonly store = new Store<PlayerState>(initialState);
  private audio: AudioLike | null;
  private source: DataSource | null;
  private readonly storageKey: string | false;
  private readonly timeListeners = new Set<(timeSec: number) => void>();
  private playlists = new Map<string, Promise<Playlist>>();
  private infoPromise: Promise<SourceInfo | null> | null = null;
  private sourceWaiters: (() => void)[] = [];
  private frame = 0;
  private retried = false;
  private pendingSeek: number | null = null;
  /** 音频当前加载的是哪首歌、哪种音质(歌曲 ID + 音质),换歌或切换音质时才重新设置 src。 */
  private srcFor = "";
  /**
   * 切换音质时换了 src,音频会先从 0 秒报进度,等元数据加载完才回到原位置。这期间不更新进度,
   * 否则进度条、歌词会先跳回开头再跳回来。
   */
  private holdTime = false;
  private lastSave = 0;
  private readonly unbind: (() => void)[] = [];

  constructor(options: PlayerOptions = {}) {
    this.source = options.source ?? null;
    this.storageKey = options.storageKey ?? "kite-player";
    this.audio = options.audio ?? (typeof Audio !== "undefined" ? new Audio() : null);
    if (this.audio) {
      this.audio.preload = "none";
      this.bindAudio(this.audio);
    }
    this.restore();
    this.unbind.push(this.store.subscribe((s, prev) => this.onState(s, prev)));
    if (options.mediaSession !== false) this.bindMediaSession();
  }

  get state(): PlayerState {
    return this.store.get();
  }

  subscribe(fn: (state: PlayerState, prev: PlayerState) => void): () => void {
    return this.store.subscribe(fn);
  }

  /** onTime 在播放时每帧回调当前秒数(给进度、逐字歌词这类需要平滑更新的界面用)。 */
  onTime(fn: (timeSec: number) => void): () => void {
    this.timeListeners.add(fn);
    return () => this.timeListeners.delete(fn);
  }

  /** currentTime 返回精确的播放位置(秒)。 */
  currentTime(): number {
    return this.audio?.currentTime ?? this.state.time;
  }

  setSource(source: DataSource): void {
    this.source = source;
    this.playlists.clear();
    this.infoPromise = null;
    for (const fn of this.sourceWaiters.splice(0)) fn();
    // 从快照恢复的歌在构造时还没有数据来源,歌词到这里才能加载。
    const { current, lyrics } = this.state;
    if (current && !lyrics) void this.loadLyrics(current);
  }

  /** whenSource 等到数据来源设置好(页面里的卡片可能比 <kite-player> 先初始化)。 */
  whenSource(): Promise<void> {
    if (this.source) return Promise.resolve();
    return new Promise((resolve) => this.sourceWaiters.push(resolve));
  }

  hasSource(): boolean {
    return this.source !== null;
  }

  /** loadPlaylist 读取歌单(同一歌单只请求一次)。 */
  loadPlaylist(id: string): Promise<Playlist> {
    const source = this.requireSource();
    let p = this.playlists.get(id);
    if (!p) {
      p = source.playlist(id);
      p.catch(() => this.playlists.delete(id));
      this.playlists.set(id, p);
    }
    return p;
  }

  /** info 返回数据来源的服务信息(音质、服务端网易云账号的登录态);来源不支持时返回 null。 */
  info(): Promise<SourceInfo | null> {
    if (!this.infoPromise) {
      const fn = this.source?.info;
      this.infoPromise = fn ? fn.call(this.source).catch(() => null) : Promise.resolve(null);
    }
    return this.infoPromise;
  }

  /** lyricsFor 读取某首歌的歌词(不必是当前播放的歌,给卡片预览用)。 */
  lyricsFor(track: Track): Promise<Lyrics> {
    return this.requireSource().lyrics(track);
  }

  /** whenLyrics 等当前歌的歌词加载完(已加载则立即返回)。 */
  whenLyrics(trackId: string): Promise<Lyrics | null> {
    const s = this.state;
    if (s.current?.id === trackId && (s.lyrics || s.lyricsStatus === "failed")) return Promise.resolve(s.lyrics);
    return new Promise((resolve) => {
      const stop = this.subscribe((st) => {
        if (st.current?.id !== trackId) {
          stop();
          resolve(null);
        } else if (st.lyrics || st.lyricsStatus === "failed") {
          stop();
          resolve(st.lyrics);
        }
      });
    });
  }

  /** loadTrack 读取单曲(用于页面里单独嵌入的歌)。 */
  loadTrack(id: string): Promise<Track> {
    return this.requireSource().track(id);
  }

  /** playPlaylist 把歌单设为队列并播放;start 不传时从第一首能播的开始。 */
  async playPlaylist(id: string, start?: number): Promise<void> {
    const pl = await this.loadPlaylist(id);
    const index = start ?? pl.tracks.findIndex(canPlay);
    this.setQueue(pl.tracks, Math.max(index, 0), { id: pl.id, name: pl.name }, true);
  }

  /** setQueue 替换队列。autoplay 为 false 时只切换不播放。 */
  setQueue(tracks: Track[], index: number, playlist: { id: string; name: string } | null = null, autoplay = false): void {
    this.store.set({ queue: tracks, playlist });
    this.jump(index, autoplay);
  }

  /** playTrack 播放一首歌:已在队列里就跳过去,否则插到当前歌后面。 */
  async playTrack(track: Track | string): Promise<void> {
    const t = typeof track === "string" ? await this.loadTrack(track) : track;
    const { queue, index, current } = this.state;
    if (current?.id === t.id) {
      await this.play();
      return;
    }
    const at = queue.findIndex((q) => q.id === t.id);
    if (at >= 0) {
      this.jump(at, true);
      return;
    }
    const next = queue.slice();
    next.splice(index + 1, 0, t);
    this.store.set({ queue: next });
    this.jump(index + 1, true);
  }

  async play(): Promise<void> {
    const { current } = this.state;
    if (!current || !this.audio) return;
    if (!canPlay(current)) {
      this.store.set({ error: unplayable(current), playing: false });
      return;
    }
    this.ensureSrc(current);
    this.store.set({ error: null });
    try {
      await this.audio.play();
    } catch (err) {
      // 浏览器拦截了自动播放(NotAllowedError)或加载被打断:回到暂停态,等读者点击。
      if ((err as Error)?.name !== "AbortError") this.store.set({ playing: false, loading: false });
    }
  }

  pause(): void {
    this.audio?.pause();
  }

  toggle(): Promise<void> | void {
    return this.state.playing ? this.pause() : this.play();
  }

  /** next 下一首;auto 表示播完自动切换(顺序模式在末尾会停下)。 */
  next(auto = false): void {
    const i = this.pick(1, auto);
    if (i === null) {
      if (auto) {
        this.pause();
        this.seek(0);
      }
      return;
    }
    this.jump(i, true);
  }

  prev(): void {
    if (this.currentTime() > restartThreshold) {
      this.seek(0);
      return;
    }
    const i = this.pick(-1, false);
    if (i !== null) this.jump(i, true);
  }

  seek(sec: number): void {
    const t = Math.max(0, sec);
    if (this.audio && this.state.current && this.srcFor.startsWith(this.state.current.id + ":")) {
      this.audio.currentTime = t;
    } else {
      this.pendingSeek = t;
    }
    this.store.set({ time: t, lyricIndex: this.lyricIndexAt(t) });
    this.emitTime(t);
  }

  /** playChorus 从副歌开始播。没有副歌信息时什么都不做。 */
  playChorus(): void {
    const start = this.state.lyrics?.chorus[0]?.start;
    if (start === undefined) return;
    this.seek(start / 1000);
    void this.play();
  }

  setMode(mode: PlayMode): void {
    this.store.set({ mode });
  }

  cycleMode(): PlayMode {
    const next = modes[(modes.indexOf(this.state.mode) + 1) % modes.length] ?? "loop";
    this.setMode(next);
    return next;
  }

  /** canSwitchQuality:数据来源能按较低音质给出播放地址时,才能切换高音质。 */
  canSwitchQuality(): boolean {
    return typeof this.source?.streamUrl === "function";
  }

  /**
   * setHighQuality 开关高音质。播放中切换时从当前位置用新音质接着播,暂停时下次播放才生效。
   * 数据来源不支持 streamUrl 时只记住偏好,不影响播放。
   */
  setHighQuality(on: boolean): void {
    if (this.state.highQuality === on) return;
    this.store.set({ highQuality: on });
    this.save();
    const track = this.state.current;
    if (!track || !this.audio || !this.srcFor.startsWith(track.id + ":") || !this.canSwitchQuality()) return;
    const wasPlaying = this.state.playing;
    this.pendingSeek = this.audio.currentTime || this.state.time;
    this.holdTime = this.pendingSeek > 0;
    this.ensureSrc(track, true);
    if (wasPlaying) void this.play();
  }

  setVolume(volume: number): void {
    const v = Math.min(1, Math.max(0, volume));
    if (this.audio) this.audio.volume = v;
    this.store.set({ volume: v });
  }

  destroy(): void {
    this.pause();
    if (typeof cancelAnimationFrame !== "undefined") cancelAnimationFrame(this.frame);
    for (const fn of this.unbind) fn();
  }

  // ---- 内部 ----

  private requireSource(): DataSource {
    if (!this.source) throw new Error("kite-player: 还没有设置数据来源(api 属性或 setSource)");
    return this.source;
  }

  private jump(index: number, autoplay: boolean): void {
    const track = this.state.queue[index] ?? null;
    this.retried = false;
    this.holdTime = false;
    this.pendingSeek = null;
    this.store.set({
      index: track ? index : -1,
      current: track,
      time: 0,
      duration: track ? track.durationMs / 1000 : 0,
      lyrics: null,
      lyricsStatus: track ? "loading" : "idle",
      lyricIndex: -1,
      error: track && !canPlay(track) ? unplayable(track) : null,
      loading: false,
    });
    if (!track) return;
    void this.loadLyrics(track);
    if (!canPlay(track)) {
      this.audio?.pause();
      // 自动切到这首却不能播:顺着方向继续找下一首能播的。
      if (autoplay && this.state.queue.some(canPlay)) {
        const i = this.pick(1, false);
        if (i !== null && i !== index) this.jump(i, true);
      }
      return;
    }
    if (this.audio && autoplay) {
      this.ensureSrc(track, true);
      void this.play();
    } else if (this.audio && this.srcFor && !this.srcFor.startsWith(track.id + ":")) {
      this.audio.pause();
      this.srcFor = "";
    }
  }

  /** pick 按播放模式选下一首 / 上一首能播的歌;没有可选时返回 null。 */
  private pick(dir: 1 | -1, auto: boolean): number | null {
    const { queue, index, mode } = this.state;
    const n = queue.length;
    if (n === 0) return null;
    if (mode === "shuffle") {
      const candidates = queue.map((t, i) => (canPlay(t) && i !== index ? i : -1)).filter((i) => i >= 0);
      if (candidates.length === 0) return null;
      return candidates[Math.floor(Math.random() * candidates.length)] ?? null;
    }
    for (let step = 1; step <= n; step++) {
      const raw = index + dir * step;
      if (auto && mode === "sequence" && (raw >= n || raw < 0)) return null;
      const i = ((raw % n) + n) % n;
      if (canPlay(queue[i])) return i;
    }
    return null;
  }

  /** streamOf 这首歌按当前音质偏好的播放地址。 */
  private streamOf(track: Track): string {
    const fn = this.source?.streamUrl;
    return !this.state.highQuality && fn ? fn.call(this.source, track, "standard") : track.stream;
  }

  private ensureSrc(track: Track, force = false): void {
    if (!this.audio) return;
    const key = `${track.id}:${this.state.highQuality ? "hq" : "sd"}`;
    if (!force && this.srcFor === key) return;
    this.srcFor = key;
    this.audio.src = this.streamOf(track);
    if (this.pendingSeek !== null && this.pendingSeek > 0) {
      // 元数据加载完才能设置进度。
      const t = this.pendingSeek;
      const onMeta = () => {
        this.audio?.removeEventListener("loadedmetadata", onMeta);
        if (this.audio) this.audio.currentTime = t;
        this.holdTime = false;
      };
      this.audio.addEventListener("loadedmetadata", onMeta);
    }
    this.pendingSeek = null;
  }

  private async loadLyrics(track: Track): Promise<void> {
    // 还没有数据来源时保持 loading,setSource 会再调一次。
    if (!this.source) return;
    try {
      const lyrics = await this.source.lyrics(track);
      if (this.state.current?.id !== track.id) return;
      this.store.set({ lyrics, lyricsStatus: "ready", lyricIndex: lineIndexAt(lyrics.lines, this.currentTime() * 1000) });
    } catch {
      // 没有歌词不影响播放,界面显示加载失败。
      if (this.state.current?.id === track.id) this.store.set({ lyricsStatus: "failed" });
    }
  }

  private lyricIndexAt(sec: number): number {
    const lines = this.state.lyrics?.lines;
    return lines ? lineIndexAt(lines, sec * 1000) : -1;
  }

  private bindAudio(audio: AudioLike): void {
    const on = (type: string, fn: () => void) => {
      audio.addEventListener(type, fn);
      this.unbind.push(() => audio.removeEventListener(type, fn));
    };
    on("play", () => {
      this.store.set({ playing: true });
      this.startFrames();
    });
    on("pause", () => this.store.set({ playing: false, loading: false }));
    on("waiting", () => this.store.set({ loading: true }));
    on("playing", () => this.store.set({ loading: false, error: null }));
    on("canplay", () => this.store.set({ loading: false }));
    on("timeupdate", () => {
      if (this.holdTime) return;
      const t = audio.currentTime;
      this.store.set({ time: t, lyricIndex: this.lyricIndexAt(t) });
      if (!this.frame) this.emitTime(t);
    });
    on("durationchange", () => {
      if (Number.isFinite(audio.duration) && audio.duration > 0) this.store.set({ duration: audio.duration });
    });
    on("ended", () => this.onEnded());
    on("error", () => void this.onError());
  }

  private onEnded(): void {
    if (this.state.mode === "one") {
      this.seek(0);
      void this.play();
      return;
    }
    this.next(true);
  }

  /**
   * onError:上游直链几十分钟后过期,暂停太久或从快照恢复后再播会失败。
   * 第一次失败时重新取一次播放链接并回到原位置;再失败才报错。
   */
  private async onError(): Promise<void> {
    const track = this.state.current;
    this.holdTime = false;
    if (!track || !this.audio || !this.srcFor.startsWith(track.id + ":")) return;
    if (this.retried) {
      this.store.set({
        playing: false,
        loading: false,
        error: { reason: "PLAYBACK_FAILED", message: "这首歌暂时无法播放" },
      });
      return;
    }
    this.retried = true;
    const at = this.audio.currentTime || this.state.time;
    let fresh = track;
    try {
      if (this.source) fresh = await this.source.track(track.id);
    } catch {
      // 拿不到新的单曲信息就用原链接再试一次(服务端会重新解析直链)。
    }
    if (this.state.current?.id !== track.id) return;
    if (fresh !== track) this.replaceCurrent(fresh);
    this.pendingSeek = at;
    this.ensureSrc(fresh, true);
    void this.play();
  }

  private replaceCurrent(track: Track): void {
    const queue = this.state.queue.slice();
    if (this.state.index >= 0) queue[this.state.index] = track;
    this.store.set({ queue, current: track });
  }

  private startFrames(): void {
    if (typeof requestAnimationFrame === "undefined" || this.frame) return;
    const tick = () => {
      if (!this.state.playing || !this.audio) {
        this.frame = 0;
        return;
      }
      if (this.holdTime) {
        this.frame = requestAnimationFrame(tick);
        return;
      }
      const t = this.audio.currentTime;
      this.emitTime(t);
      const li = this.lyricIndexAt(t);
      if (li !== this.state.lyricIndex) this.store.set({ lyricIndex: li });
      this.frame = requestAnimationFrame(tick);
    };
    this.frame = requestAnimationFrame(tick);
  }

  private emitTime(t: number): void {
    for (const fn of this.timeListeners) fn(t);
  }

  private onState(s: PlayerState, prev: PlayerState): void {
    if (s.current !== prev.current) this.updateMediaMetadata(s.current);
    if (s.playing !== prev.playing && typeof navigator !== "undefined" && navigator.mediaSession) {
      navigator.mediaSession.playbackState = s.playing ? "playing" : "paused";
    }
    const structural =
      s.queue !== prev.queue || s.index !== prev.index || s.mode !== prev.mode || s.volume !== prev.volume;
    const now = Date.now();
    if (structural || (s.time !== prev.time && now - this.lastSave > 5000)) {
      this.lastSave = now;
      this.save();
    }
  }

  private save(): void {
    if (!this.storageKey) return;
    const { queue, index, time, mode, volume, playlist, highQuality } = this.state;
    saveSnapshot(this.storageKey, { queue, index, time, mode, volume, playlist, highQuality, savedAt: Date.now() });
  }

  private restore(): void {
    if (!this.storageKey) return;
    const snap = loadSnapshot(this.storageKey);
    if (snap?.highQuality === false) this.store.set({ highQuality: false });
    if (!snap || snap.queue.length === 0) return;
    const current = snap.queue[snap.index] ?? null;
    this.store.set({
      queue: snap.queue,
      index: current ? snap.index : -1,
      current,
      playlist: snap.playlist,
      mode: snap.mode,
      volume: snap.volume,
      time: snap.time,
      duration: current ? current.durationMs / 1000 : 0,
      lyricsStatus: current ? "loading" : "idle",
    });
    if (this.audio) this.audio.volume = snap.volume;
    this.pendingSeek = snap.time;
    if (current) void this.loadLyrics(current);
  }

  private bindMediaSession(): void {
    if (typeof navigator === "undefined" || !navigator.mediaSession) return;
    const ms = navigator.mediaSession;
    const set = (action: MediaSessionAction, fn: MediaSessionActionHandler) => {
      try {
        ms.setActionHandler(action, fn);
      } catch {
        // 有的浏览器不支持部分动作。
      }
    };
    set("play", () => void this.play());
    set("pause", () => this.pause());
    set("previoustrack", () => this.prev());
    set("nexttrack", () => this.next());
    set("seekto", (d) => {
      if (d.seekTime !== undefined) this.seek(d.seekTime);
    });
  }

  private updateMediaMetadata(track: Track | null): void {
    if (typeof navigator === "undefined" || !navigator.mediaSession || typeof MediaMetadata === "undefined") return;
    navigator.mediaSession.metadata = track
      ? new MediaMetadata({
          title: track.title,
          artist: track.artists.map((a) => a.name).join(" / "),
          album: track.album.name,
          artwork: track.album.cover ? [{ src: track.album.cover, sizes: "512x512" }] : [],
        })
      : null;
  }
}

/** canPlay:这首歌能不能播。playable 是数据来源按登录态的推断;trial 表示只有试听片段,
 * 但 stream 可播。试听不再是 playable 一个布尔能拦下的,两者满足其一即可。 */
export function canPlay(track: Track | null | undefined): boolean {
  return !!track && (track.playable || track.trial === true);
}

/** unplayableMessage 是歌曲不能播放时给听众的说明。 */
export function unplayableMessage(track: Track): string {
  return track.access === "vip" ? "这首是 VIP 歌曲,当前服务没有登录 VIP 账号" : "这首歌暂时无法播放(版权限制)";
}

function unplayable(track: Track): { reason: string; message: string } {
  return { reason: "UNPLAYABLE", message: unplayableMessage(track) };
}

/** 页面级单例:同一页面里的所有播放器界面共用它。 */
export function getPlayer(options: PlayerOptions = {}): Player {
  const g = globalThis as { __kitePlayer?: Player };
  if (!g.__kitePlayer) {
    g.__kitePlayer = new Player(options);
  } else if (options.source && !g.__kitePlayer.hasSource()) {
    g.__kitePlayer.setSource(options.source);
  }
  return g.__kitePlayer;
}
