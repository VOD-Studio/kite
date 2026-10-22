import { html, LitElement, nothing, type PropertyValues, type TemplateResult } from "lit";
import { classMap } from "lit/directives/class-map.js";
import { keyed } from "lit/directives/keyed.js";
import { firstSungIndex, formatTime, lineUnits, wordProgress } from "../core/lyrics";
import type { Player } from "../core/player";
import type { LyricLine, Lyrics, PlayerState, Track } from "../core/types";
import { blob, coverPh, eq, icons } from "./icons";
import { cardSheet } from "./styles";
import { artists, canPlay, emit, emitFly, onCoverError, onCoverLoad, openEvent, playerFor, sizeOf, toastEvent, unplayableMessage, watchNarrow, type OpenDetail } from "./util";

const sizes = ["s", "m", "l"] as const;
type Size = (typeof sizes)[number];

/** 卡片窄于这个宽度时用紧凑布局(小卡片 480,其它 520)。 */
const narrowAt: Record<Size, number> = { s: 480, m: 520, l: 520 };

/** 卡片里显示的那一句:正在唱的(live)或唱到第一句之前预览的第一句。 */
interface LyricView {
  key: number;
  line: LyricLine | undefined;
  live: boolean;
  prev: string;
  next: string;
  instrumental: boolean;
}

/**
 * <kite-track song="歌曲 ID" size="s | m | l">:单曲卡片。
 * s 一行小卡片;m 标准卡片(默认,带当前歌词与完整控制);l 大卡片(封面环境色、滚动歌词)。
 * 与 <kite-player> 共用播放核心:在卡片里播放,页面上的播放器同步。
 */
export class KiteTrackElement extends LitElement {
  static override styles = cardSheet;
  static override properties = {
    song: { type: String },
    size: { type: String },
    theme: { type: String, reflect: true },
    api: { type: String, reflect: true },
  };

  declare song: string | null;
  declare size: string | null;
  declare theme: string | null;
  declare api: string | null;

  private player!: Player;
  private s!: PlayerState;
  private track: Track | null = null;
  /** 卡片自己预取的歌词,用于还没播放时预览。 */
  private lyrics: Lyrics | null = null;
  private status: "loading" | "ready" | "error" = "loading";
  private errorText = "";
  private loadSeq = 0;
  private dragging = false;
  private units: HTMLElement[] = [];
  private cleanup: (() => void)[] = [];

  constructor() {
    super();
    this.song = null;
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
      this.player.onTime((t) => this.onFrame(t)),
      watchNarrow(this, () => narrowAt[this.cardSize()]),
    );
  }

  override disconnectedCallback(): void {
    super.disconnectedCallback();
    for (const fn of this.cleanup) fn();
    this.cleanup = [];
  }

  protected override willUpdate(changed: PropertyValues): void {
    if (changed.has("song")) void this.load();
  }

  private cardSize(): Size {
    return sizeOf(this.size, sizes, "m");
  }

  private async load(): Promise<void> {
    const id = this.song;
    if (!id) return;
    const seq = ++this.loadSeq;
    this.status = "loading";
    this.requestUpdate();
    try {
      await this.player.whenSource();
      const track = await this.player.loadTrack(id);
      const lyrics = this.cardSize() === "s" ? null : await this.player.lyricsFor(track).catch(() => null);
      if (seq !== this.loadSeq) return;
      this.track = track;
      this.lyrics = lyrics;
      this.status = "ready";
    } catch (err) {
      if (seq !== this.loadSeq) return;
      this.status = "error";
      this.errorText = (err as Error).message;
    }
    this.requestUpdate();
  }

  // ---- 渲染 ----

  protected override render(): TemplateResult {
    const t = this.track;
    if (this.status === "loading" || (!t && this.status !== "error")) return html`<div class="skeleton" aria-busy="true"></div>`;
    if (this.status === "error" || !t) {
      return html`<div class="card s"><div class="main"><div class="title">这首歌暂时无法显示</div><div class="error">${this.errorText}</div></div></div>`;
    }
    const size = this.cardSize();
    const cur = this.s.current?.id === t.id;
    const classes = classMap({ card: true, [size]: true, locked: !canPlay(t), cur, playing: cur && this.s.playing });
    if (size === "s") return this.renderS(t, cur, classes);
    return size === "l" ? this.renderL(t, cur, classes) : this.renderM(t, cur, classes);
  }

  private renderS(t: Track, cur: boolean, classes: ReturnType<typeof classMap>): TemplateResult {
    const locked = !canPlay(t);
    const sub = locked
      ? html`${icons.lock}<span class="ell">${unplayableMessage(t)}</span>`
      : html`<span class="ell">${[artists(t), t.album.name, formatTime(t.durationMs / 1000)].filter(Boolean).join(" · ")}</span>`;
    return html`<div class=${classes} part="card">
      <div class="cover">${coverPh}<img alt="" src=${t.album.cover || nothing} @error=${onCoverError} @load=${onCoverLoad} /></div>
      <div class="main">
        <div class="title">${eq}<span class="ell">${t.title}</span>${t.trial ? html`<span class="tg trial">仅试听</span>` : nothing}<span class="who ell">${artists(t)}</span></div>
        <div class="sub" ?hidden=${cur && canPlay(t)}>${sub}</div>
        ${this.renderProgress(t, !cur)}
      </div>
      <div class="actions">
        ${locked ? nothing : html`<button class="chip chorus" aria-label=${`从副歌开始播放 ${t.title}`} @click=${this.onChorus}>${icons.wave}副歌</button>`}<button
          class="blob play"
          aria-label=${this.playLabel(t, cur)}
          @click=${this.onPlay}
        >
          ${blob}
        </button>
      </div>
    </div>`;
  }

  private renderM(t: Track, cur: boolean, classes: ReturnType<typeof classMap>): TemplateResult {
    const locked = !canPlay(t);
    const lv = this.lyricView(cur);
    return html`<div class=${classes} part="card">
      <div class="cover">${coverPh}<img alt="" src=${t.album.cover || nothing} @error=${onCoverError} @load=${onCoverLoad} /></div>
      <div class="main">
        <div class="head">
          <div style="flex:1;min-width:0">
            <div class="title ell">${t.title}</div>
            <div class="who"><span class="ell">${[artists(t), t.album.name].filter(Boolean).join(" · ")}</span>${this.tags(t)}</div>
          </div>
          ${locked ? nothing : html`<button class="chip chorus" @click=${this.onChorus}>${icons.wave}从副歌听</button>`}
        </div>
        <div class="lyric">${this.renderLine(lv, true)}<div class="lyric-tr ell">${lv.line?.translation ?? ""}</div></div>
        ${locked ? html`<div class="error">${unplayableMessage(t)}</div>` : nothing}
        <div class="controls">
          <button class="ib prev" aria-label="上一首" ?hidden=${!cur} @click=${this.onPrev}>${icons.prev}</button
          ><button class="blob play" aria-label=${this.playLabel(t, cur)} @click=${this.onPlay}>${blob}</button
          ><button class="ib next" aria-label="下一首" ?hidden=${!cur} @click=${this.onNext}>${icons.next}</button>${this.renderProgress(t, false)}
        </div>
      </div>
    </div>`;
  }

  private renderL(t: Track, cur: boolean, classes: ReturnType<typeof classMap>): TemplateResult {
    const locked = !canPlay(t);
    const lv = this.lyricView(cur);
    return html`<div class=${classes} part="card">
      <div class="amb"><img alt="" src=${t.album.cover || nothing} @error=${onCoverError} /></div>
      <div class="scrim"></div>
      <div class="top">
        <div class="cover">${coverPh}<img alt="" src=${t.album.cover || nothing} @error=${onCoverError} @load=${onCoverLoad} /></div>
        <div class="info">
          <div class="kicker">${cur ? "单曲 · 正在播放" : "单曲"}</div>
          <div class="title ell">${t.title}</div>
          <div class="who ell">${[artists(t), t.album.name].filter(Boolean).join(" · ")}</div>
          <div class="tags">${this.tags(t)}</div>
          ${locked
            ? html`<div class="error" style="color:rgba(255,255,255,.75)">${unplayableMessage(t)}</div>`
            : html`<button class="chip chorus" @click=${this.onChorus}>${icons.wave}从副歌听</button>`}
        </div>
        ${locked ? nothing : html`<button class="ib open" aria-label="在播放器中打开" style="color:rgba(255,255,255,.85)" @click=${this.onOpen}>${icons.expand}</button>`}
      </div>
      <div class="lyric">
        <div class="lyric-prev ell">${lv.prev}</div>
        ${this.renderLine(lv, false)}
        <div class="lyric-tr ell">${lv.line?.translation ?? ""}</div>
        <div class="lyric-next ell">${lv.next}</div>
      </div>
      <div class="controls">
        <button class="ib prev" aria-label="上一首" ?hidden=${!cur} style="color:#fff" @click=${this.onPrev}>${icons.prev}</button
        ><button class="blob play" aria-label=${this.playLabel(t, cur)} @click=${this.onPlay}>${blob}</button
        ><button class="ib next" aria-label="下一首" ?hidden=${!cur} style="color:#fff" @click=${this.onNext}>${icons.next}</button>${this.renderProgress(t, false)}
      </div>
    </div>`;
  }

  /** renderLine:正在唱的句子拆成逐字的 span(换句时重建,不复用上一句的进度)。 */
  private renderLine(lv: LyricView, ell: boolean): TemplateResult {
    const classes = classMap({ "lyric-line": true, ell, live: lv.live });
    if (!lv.line) return html`<div class=${classes}>${lv.instrumental ? "纯音乐" : ""}</div>`;
    if (!lv.live) return html`<div class=${classes}>${lv.line.text}</div>`;
    const units = lineUnits(lv.line);
    return html`<div class=${classes}>${keyed(lv.key, units.map((u) => html`<span class="u">${u}</span>`))}</div>`;
  }

  private renderProgress(t: Track, hidden: boolean): TemplateResult {
    return html`<div class="progress" ?hidden=${hidden}>
      <span class="t-cur num">0:00</span><span class="bar"><b></b></span><span class="t-dur num">${formatTime(this.durationOf(t))}</span
      ><input type="range" min="0" max="1000" value="0" aria-label="播放进度" @input=${this.onRangeInput} @change=${this.onRangeChange} />
    </div>`;
  }

  private tags(t: Track): unknown {
    return html`${t.trial ? html`<span class="tg trial">仅试听</span>` : nothing}${t.access === "vip"
      ? html`<span class="tg vip">VIP</span>`
      : t.access === "paid"
        ? html`<span class="tg out">数字专辑</span>`
        : nothing}`;
  }

  private playLabel(t: Track, cur: boolean): string {
    return `${cur && this.s.playing ? "暂停" : "播放"} ${t.title}`;
  }

  private durationOf(t: Track): number {
    return this.s.current?.id === t.id && this.s.duration > 0 ? this.s.duration : t.durationMs / 1000;
  }

  /**
   * lyricView 算出卡片里显示的那一句。开头的署名行(作词 / 作曲)不当作「正在唱」,
   * 唱到第一句之前一直预览第一句;刚切到这首时播放器的歌词还在加载,先用卡片自己预取的。
   */
  private lyricView(cur: boolean): LyricView {
    const s = this.s;
    const lyr = cur ? (s.lyrics ?? this.lyrics) : this.lyrics;
    const lines = lyr?.lines ?? [];
    const sung = firstSungIndex(lines);
    const idx = cur && s.lyrics ? s.lyricIndex : -1;
    const live = idx >= 0 && idx >= sung;
    const at = live ? idx : sung;
    return {
      key: live ? idx : -1,
      line: lines[at],
      live,
      prev: live && idx > sung ? (lines[idx - 1]?.text ?? "") : "",
      next: lines[at + 1]?.text ?? "",
      instrumental: !!lyr?.instrumental,
    };
  }

  protected override updated(changed: PropertyValues): void {
    // 尺寸变了而宽度没变时 ResizeObserver 不会触发,紧凑布局的阈值要在这里重新判断。
    if (changed.has("size")) this.toggleAttribute("data-narrow", this.clientWidth > 0 && this.clientWidth <= narrowAt[this.cardSize()]);
    this.units = [...this.renderRoot.querySelectorAll<HTMLElement>(".lyric-line.live .u")];
    const t = this.track;
    if (!t) return;
    if (this.s.current?.id === t.id) this.onFrame(this.player.currentTime());
    else this.setPct(0);
  }

  // ---- 事件 ----

  private onPlay = (): void => void this.play(false);
  private onChorus = (): void => void this.play(true);
  private onPrev = (): void => this.player.prev();
  private onNext = (): void => this.player.next();
  private onOpen = (): void => void this.openPlayer();

  private onRangeInput = (e: Event): void => {
    this.dragging = true;
    this.setPct(Number((e.target as HTMLInputElement).value) / 1000);
  };

  private onRangeChange = (e: Event): void => {
    this.dragging = false;
    this.player.seek((Number((e.target as HTMLInputElement).value) / 1000) * this.player.state.duration);
  };

  private async play(chorus: boolean): Promise<void> {
    const t = this.track;
    if (!t) return;
    if (!canPlay(t)) {
      emit(toastEvent, unplayableMessage(t));
      return;
    }
    const s = this.player.state;
    if (s.current?.id === t.id && !chorus) {
      await this.player.toggle();
      return;
    }
    if (s.current?.id !== t.id) emitFly(this.renderRoot.querySelector(".cover"), t.album.cover);
    await this.player.playTrack(t);
    if (chorus) {
      await this.player.whenLyrics(t.id);
      this.player.playChorus();
    }
  }

  private async openPlayer(): Promise<void> {
    const t = this.track;
    if (!t) return;
    if (this.player.state.current?.id !== t.id) await this.play(false);
    emit<OpenDetail>(openEvent, { view: "lyrics", immersive: true });
  }

  // ---- 每帧 ----

  private onFrame(t: number): void {
    const s = this.player.state;
    if (!this.track || s.current?.id !== this.track.id || !this.hasUpdated) return;
    if (!this.dragging && s.duration > 0) this.setPct(Math.min(1, t / s.duration));
    // 文字和逐字进度没变时不写,写一次就要重新排版 / 绘制。
    const cur = this.renderRoot.querySelector(".t-cur");
    const time = formatTime(t);
    if (cur && cur.textContent !== time) cur.textContent = time;
    const line = s.lyrics?.lines[s.lyricIndex];
    if (line && this.units.length) {
      const p = wordProgress(line, t * 1000);
      this.units.forEach((u, k) => {
        const v = String(Math.round((p[k] ?? 0) * 100) / 100);
        if (u.style.getPropertyValue("--p") !== v) u.style.setProperty("--p", v);
      });
    }
  }

  private setPct(pct: number): void {
    this.renderRoot.querySelector<HTMLElement>(".bar")?.style.setProperty("--pct", String(pct));
    const input = this.renderRoot.querySelector<HTMLInputElement>(".progress input");
    if (input && !this.dragging) input.value = String(Math.round(pct * 1000));
  }
}
