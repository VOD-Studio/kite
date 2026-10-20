import { formatTime } from "../core/lyrics";
import type { Player } from "../core/player";
import type { PlayerState, Playlist } from "../core/types";
import { icons } from "./icons";
import { cardStyles } from "./styles";
import { artists, emit, emitFly, esc, openQueueEvent, playerFor, sizeOf, toastEvent, unplayableMessage } from "./util";

/** 标准卡片列出的歌曲数。 */
const previewRows = 4;

/**
 * <kite-playlist playlist="歌单 ID" size="m | l">:文章里的歌单卡片。
 * m 列出前 4 首;l 列出全部(可滚动)。点任意一首从那首开始播放整个歌单。
 */
export class KitePlaylistElement extends HTMLElement {
  static observedAttributes = ["playlist", "size"];

  private root: ShadowRoot | null = null;
  private player!: Player;
  private list: Playlist | null = null;
  private unsub: (() => void)[] = [];
  private loadSeq = 0;

  connectedCallback(): void {
    if (!this.root) this.root = this.attachShadow({ mode: "open" });
    this.player = playerFor(this);
    this.unsub.push(this.player.subscribe((s) => this.update(s)));
    void this.load();
  }

  disconnectedCallback(): void {
    for (const fn of this.unsub) fn();
    this.unsub = [];
  }

  attributeChangedCallback(name: string, old: string | null, value: string | null): void {
    if (!this.root || old === value) return;
    if (name === "playlist") void this.load();
    else this.draw();
  }

  private async load(): Promise<void> {
    const id = this.getAttribute("playlist");
    const root = this.root;
    if (!id || !root) return;
    const seq = ++this.loadSeq;
    root.innerHTML = `<style>${cardStyles}</style><div class="skeleton" aria-busy="true"></div>`;
    try {
      await this.player.whenSource();
      const pl = await this.player.loadPlaylist(id);
      if (seq !== this.loadSeq) return;
      this.list = pl;
      this.draw();
    } catch (err) {
      if (seq !== this.loadSeq) return;
      root.innerHTML = `<style>${cardStyles}</style><div class="card pl"><div class="name">歌单暂时无法显示</div><div class="error">${esc((err as Error).message)}</div></div>`;
    }
  }

  private draw(): void {
    const pl = this.list;
    const root = this.root;
    if (!pl || !root) return;
    const big = sizeOf(this, ["m", "l"], "m") === "l";
    const rows = big ? pl.tracks : pl.tracks.slice(0, previewRows);
    const minutes = Math.round(pl.tracks.reduce((sum, t) => sum + t.durationMs, 0) / 60000);
    const covers = [pl.cover, pl.tracks[1]?.album.cover ?? pl.cover, pl.tracks[2]?.album.cover ?? pl.cover];
    root.innerHTML = `<style>${cardStyles}</style>
      <div class="card pl${big ? " big" : ""}" part="card">
        <div class="head">
          <div class="stack"><div class="cover c3"><img alt="" src="${esc(covers[2])}"></div><div class="cover c2"><img alt="" src="${esc(covers[1])}"></div><div class="cover c1"><img alt="" src="${esc(covers[0])}"></div></div>
          <div class="info"><div class="kicker">歌单</div><div class="name ell">${esc(pl.name)}</div><div class="meta ell">${pl.trackCount || pl.tracks.length} 首${minutes ? ` · ${minutes} 分钟` : ""}</div></div>
          <button class="playall" aria-label="播放全部">${icons.play}<span>播放全部</span></button>
        </div>
        <div class="rows" role="list">${rows
          .map(
            (t, i) => `<button class="row${t.playable ? "" : " locked"}" role="listitem" data-i="${i}">
              <span class="idx">${i + 1}</span>
              <span class="t"><b>${esc(t.title)}</b><span>${esc(artists(t))}</span></span>
              <span class="d">${t.playable ? formatTime(t.durationMs / 1000) : t.access === "vip" ? `<span class="tg vip">VIP</span>` : "—"}</span>
            </button>`,
          )
          .join("")}</div>
        ${!big && pl.tracks.length > previewRows ? `<button class="more">查看全部 ${pl.tracks.length} 首 ›</button>` : ""}
      </div>`;
    root.querySelector(".playall")?.addEventListener("click", () => this.play());
    root.querySelectorAll<HTMLElement>(".row").forEach((row) => row.addEventListener("click", () => this.play(Number(row.dataset.i))));
    root.querySelector(".more")?.addEventListener("click", () => emit(openQueueEvent, { playlist: pl.id }));
    this.update(this.player.state);
  }

  private play(index?: number): void {
    const pl = this.list;
    if (!pl) return;
    const t = index === undefined ? undefined : pl.tracks[index];
    if (t && !t.playable) {
      emit(toastEvent, unplayableMessage(t));
      return;
    }
    const s = this.player.state;
    if (t && s.current?.id === t.id) {
      void this.player.toggle();
      return;
    }
    emitFly(this.root?.querySelector(".c1") ?? null, pl.cover);
    void this.player.playPlaylist(pl.id, index);
  }

  private update(s: PlayerState): void {
    const pl = this.list;
    if (!pl || !this.root) return;
    const card = this.root.querySelector(".card");
    const inThis = s.playlist?.id === pl.id;
    card?.classList.toggle("cur", inThis);
    card?.classList.toggle("playing", inThis && s.playing);
    this.root.querySelectorAll<HTMLElement>(".row").forEach((row) => {
      row.classList.toggle("cur", inThis && pl.tracks[Number(row.dataset.i)]?.id === s.current?.id);
    });
  }
}
