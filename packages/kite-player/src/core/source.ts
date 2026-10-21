import type { DataSource, Lyrics, Playlist, SourceInfo, Track } from "./types";

/** kite 公开接口返回的错误,reason 见 docs/player.md。 */
export class KiteError extends Error {
  readonly reason: string;
  readonly status: number;
  constructor(reason: string, message: string, status: number) {
    super(message);
    this.name = "KiteError";
    this.reason = reason;
    this.status = status;
  }
}

export interface KiteSourceOptions {
  /** 自定义 fetch(测试或 SSR 用)。 */
  fetch?: typeof fetch;
}

/**
 * createKiteSource 对接 kite-server 公开模式(/player/v1)。
 * api 是公开模式的根地址,如 https://music.example.com。
 */
export function createKiteSource(api: string, options: KiteSourceOptions = {}): DataSource {
  const base = api.replace(/\/+$/, "");
  const doFetch = options.fetch ?? ((input, init) => fetch(input, init));
  const abs = (path: string) => (/^https?:\/\//.test(path) ? path : base + path);

  async function get<T>(path: string): Promise<T> {
    let res: Response;
    try {
      res = await doFetch(abs(path), { headers: { Accept: "application/json" } });
    } catch {
      throw new KiteError("NETWORK", "连不上音乐服务,请检查网络", 0);
    }
    const body: unknown = await res.json().catch(() => null);
    if (!res.ok) {
      const err = (body as { error?: { reason?: string; message?: string } } | null)?.error;
      throw new KiteError(err?.reason ?? "HTTP_" + res.status, err?.message ?? "音乐服务返回了错误", res.status);
    }
    return body as T;
  }

  const fixTrack = (t: Track): Track => ({ ...t, stream: abs(t.stream), lyrics: abs(t.lyrics) });

  return {
    async playlist(id) {
      const pl = await get<Playlist>(`/player/v1/playlists/${encodeURIComponent(id)}`);
      return { ...pl, tracks: pl.tracks.map(fixTrack) };
    },
    async track(id) {
      return fixTrack(await get<Track>(`/player/v1/tracks/${encodeURIComponent(id)}`));
    },
    async lyrics(track) {
      return get<Lyrics>(track.lyrics);
    },
    async info() {
      return get<SourceInfo>("/player/v1/info");
    },
  };
}
