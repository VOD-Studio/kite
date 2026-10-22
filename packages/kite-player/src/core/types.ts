/** 歌手。 */
export interface Artist {
  id: string;
  name: string;
}

/** 专辑。cover 是 https 图片地址。 */
export interface Album {
  id: string;
  name: string;
  cover: string;
}

/** 付费类型:免费、VIP、数字专辑。 */
export type Access = "free" | "vip" | "paid";

/** 一首歌。stream / lyrics 是可直接请求的绝对地址。 */
export interface Track {
  id: string;
  title: string;
  artists: Artist[];
  album: Album;
  durationMs: number;
  access: Access;
  /** 按服务端网易云账号的登录态推断能否播放,最终以播放链接为准。 */
  playable: boolean;
  /**
   * 仅试听:stream 只是试听片段(服务账号对该歌无权益时上游只给约 30 秒),仍可正常播放。
   * 由数据来源填充;kite-server 公开模式如何透传试听信息待 #103 定案(公开协议字段只增不改,缺省视为完整版)。
   */
  trial?: boolean;
  /** 播放地址,直接作为 <audio src>。 */
  stream: string;
  /** 歌词接口地址。 */
  lyrics: string;
}

export interface Playlist {
  id: string;
  name: string;
  cover: string;
  trackCount: number;
  tracks: Track[];
}

/** 逐字歌词里的一个字(毫秒)。 */
export interface LyricWord {
  time: number;
  duration: number;
  text: string;
}

/** 一行歌词(毫秒)。有逐字时间时 words 非空。 */
export interface LyricLine {
  time: number;
  duration?: number;
  text: string;
  translation?: string;
  romanized?: string;
  words?: LyricWord[];
}

export interface Lyrics {
  lines: LyricLine[];
  /** 副歌区段(毫秒)。 */
  chorus: { start: number; end: number }[];
  /** 纯音乐,没有歌词。 */
  instrumental: boolean;
}

export interface SourceInfo {
  version: string;
  /** 服务账号聚合状态:none 无可读凭据、active 至少一个可用(不保证歌曲权益)、expired 无可用且有失效账号。 */
  login: "none" | "active" | "expired";
  quality: string;
}

/**
 * 数据来源。默认实现是 createKiteSource(对接 kite-server 公开模式);
 * 有自己后端的应用可以传入自己的实现。
 */
export interface DataSource {
  playlist(id: string): Promise<Playlist>;
  track(id: string): Promise<Track>;
  lyrics(track: Track): Promise<Lyrics>;
  info?(): Promise<SourceInfo>;
}

/** 播放模式:顺序(播完停)、列表循环、单曲循环、随机。 */
export type PlayMode = "sequence" | "loop" | "one" | "shuffle";

export interface PlayerError {
  reason: string;
  message: string;
}

export interface PlayerState {
  queue: Track[];
  index: number;
  current: Track | null;
  /** 当前队列来自哪个歌单(没有则为 null)。 */
  playlist: { id: string; name: string } | null;
  playing: boolean;
  loading: boolean;
  /** 当前播放位置(秒)。 */
  time: number;
  /** 时长(秒)。 */
  duration: number;
  mode: PlayMode;
  volume: number;
  lyrics: Lyrics | null;
  /** 当前歌的歌词加载状态;failed 时 lyrics 为 null。 */
  lyricsStatus: "idle" | "loading" | "ready" | "failed";
  /** 当前歌词行下标,-1 表示还没到第一行。 */
  lyricIndex: number;
  error: PlayerError | null;
}
