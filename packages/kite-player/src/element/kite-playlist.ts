import { html, LitElement, nothing, type PropertyValues, type TemplateResult } from "lit";
import { classMap } from "lit/directives/class-map.js";
import { formatTime } from "../core/lyrics";
import type { Player } from "../core/player";
import type { PlayerState, Playlist } from "../core/types";
import { coverPh, icons } from "./icons";
import { cardSheet } from "./styles";
import { artists, canPlay, emit, emitFly, onCoverError, onCoverLoad, openQueueEvent, playerFor, sizeOf, toastEvent, unplayableMessage, watchNarrow } from "./util";

/** 标准卡片列出的歌曲数。 */
const previewRows = 4;
/** 卡片窄于这个宽度时用紧凑布局。 */
const narrowAt = 520;

/**
 * <kite-playlist playlist="歌单 ID" size="m | l">:歌单卡片。
 * m 列出前 4 首;l 列出全部(可滚动)。点任意一首从那首开始播放整个歌单。
 */
export class KitePlaylistElement extends LitElement {
  static override styles = cardSheet;
  static override properties = {
    playlist: { type: String },
    size: { type: String },
    theme: { type: String, reflect: true },
    api: { type: String, reflect: true },
  };

  declare playlist: string | null;
  declare size: string | null;
  declare theme: string | null;
  declare api: string | null;

  private player!: Player;
  private s!: PlayerState;
  private list: Playlist | null = null;
  private status: "loading" | "ready" | "error" = "loading";
  private errorText = "";
  private loadSeq = 0;
  private cleanup: (() => void)[] = [];

  constructor() {
    super();
    this.playlist = null;
    this.size = null;
    this.theme = null;
    this.api = null;
  }

  override connectedCallback(): void {
    super.connectedCallback();
    this.player = playerFor(this, this.api);
    this.s = this.player.state;
    this.cleanup.push(
      this.player.subscribe((s) => {
        this.s = s;
        this.requestUpdate();
      }),
      watchNarrow(this, () => narrowAt),
    );
  }

  override disconnectedCallback(): void {
    super.disconnectedCallback();
    for (const fn of this.cleanup) fn();
    this.cleanup = [];
  }

  protected override willUpdate(changed: PropertyValues): void {
    if (changed.has("playlist")) void this.load();
  }

  private async load(): Promise<void> {
    const id = this.playlist;
    if (!id) return;
    const seq = ++this.loadSeq;
    this.status = "loading";
    this.requestUpdate();
    try {
      await this.player.whenSource();
      const pl = await this.player.loadPlaylist(id);
      if (seq !== this.loadSeq) return;
      this.list = pl;
      this.status = "ready";
    } catch (err) {
      if (seq !== this.loadSeq) return;
      this.status = "error";
      this.errorText = (err as Error).message;
    }
    this.requestUpdate();
  }

  protected override render(): TemplateResult {
    const pl = this.list;
    if (this.status === "loading" || (!pl && this.status !== "error")) return html`<div class="skeleton" aria-busy="true"></div>`;
    if (this.status === "error" || !pl) return html`<div class="card pl"><div class="name">歌单暂时无法显示</div><div class="error">${this.errorText}</div></div>`;
    const big = sizeOf(this.size, ["m", "l"] as const, "m") === "l";
    const rows = big ? pl.tracks : pl.tracks.slice(0, previewRows);
    const minutes = Math.round(pl.tracks.reduce((sum, t) => sum + t.durationMs, 0) / 60000);
    const covers = [pl.cover, pl.tracks[1]?.album.cover ?? pl.cover, pl.tracks[2]?.album.cover ?? pl.cover];
    const inThis = this.s.playlist?.id === pl.id;
    const cur = this.s.current?.id;
    return html`<div class=${classMap({ card: true, pl: true, big, cur: inThis, playing: inThis && this.s.playing })} part="card">
      <div class="head">
        <div class="stack">
          <div class="cover c3">${coverPh}<img alt="" src=${covers[2] || nothing} @error=${onCoverError} @load=${onCoverLoad} /></div>
          <div class="cover c2">${coverPh}<img alt="" src=${covers[1] || nothing} @error=${onCoverError} @load=${onCoverLoad} /></div>
          <div class="cover c1">${coverPh}<img alt="" src=${covers[0] || nothing} @error=${onCoverError} @load=${onCoverLoad} /></div>
        </div>
        <div class="info">
          <div class="kicker">歌单</div>
          <div class="name ell">${pl.name}</div>
          <div class="meta ell">${pl.trackCount || pl.tracks.length} 首${minutes ? ` · ${minutes} 分钟` : ""}</div>
        </div>
        <button class="playall" aria-label="播放全部" @click=${() => this.play()}>${icons.play}<span>播放全部</span></button>
      </div>
      <div class="rows" role="list">
        ${rows.map(
          (t, i) => html`<button class=${classMap({ row: true, locked: !canPlay(t), cur: inThis && t.id === cur })} role="listitem" data-i=${i} @click=${() => this.play(i)}>
            <span class="idx">${i + 1}</span>
            <span class="t"
              ><b>${t.title}</b>${t.trial ? html`<span class="tg trial">仅试听</span>` : nothing}<span>${artists(t)}</span></span
            >
            <span class="d"
              >${canPlay(t)
                ? formatTime(t.durationMs / 1000)
                : t.access === "vip"
                  ? html`<span class="tg vip">VIP</span>`
                  : t.access === "paid"
                    ? html`<span class="tg out">数字专辑</span>`
                    : "—"}</span
            >
          </button>`,
        )}
      </div>
      ${!big && pl.tracks.length > previewRows ? html`<button class="more" @click=${this.openQueue}>查看全部 ${pl.tracks.length} 首 ›</button>` : nothing}
    </div>`;
  }

  private openQueue = (): void => {
    if (this.list) emit(openQueueEvent, { playlist: this.list.id });
  };

  private play(index?: number): void {
    const pl = this.list;
    if (!pl) return;
    const t = index === undefined ? undefined : pl.tracks[index];
    if (t && !canPlay(t)) {
      emit(toastEvent, unplayableMessage(t));
      return;
    }
    if (t && this.player.state.current?.id === t.id) {
      void this.player.toggle();
      return;
    }
    emitFly(this.renderRoot.querySelector(".c1"), pl.cover);
    void this.player.playPlaylist(pl.id, index);
  }
}
