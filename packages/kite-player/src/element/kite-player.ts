import { html, LitElement, nothing, type PropertyValues, type TemplateResult } from "lit";
import { classMap } from "lit/directives/class-map.js";
import { guard } from "lit/directives/guard.js";
import { ifDefined } from "lit/directives/if-defined.js";
import { keyed } from "lit/directives/keyed.js";
import { repeat } from "lit/directives/repeat.js";
import { styleMap } from "lit/directives/style-map.js";
import { firstSungIndex, formatTime, lineUnits, wordProgress } from "../core/lyrics";
import { modeLabels, type Player } from "../core/player";
import type { Lyrics, PlayerState, Playlist, Track } from "../core/types";
import { blob, eq, icons } from "./icons";
import { playerSheet } from "./styles";
import {
  $,
  artists,
  canPlay,
  canAnimate,
  flyEvent,
  openEvent,
  openQueueEvent,
  playerFor,
  readFlag,
  toastEvent,
  unplayableMessage,
  writeFlag,
  hideBroken,
  type FlyDetail,
  type OpenDetail,
} from "./util";

type Mode = "bar" | "orb" | "open";
type View = "cover" | "lyrics";

/** 换句时每往下一行多晚 35ms 开始移动,形成逐行错开的波浪。 */
const lyricStagger = 35;
/** 歌词页没有操作多久后隐藏播放控制。 */
const idleDelay = 3000;
/** 手动拖动歌词后多久回到当前行。 */
const resumeDelay = 2500;
/** 向下滚动多少像素后收起为圆形。 */
const collapseScroll = 24;

/** 封面左下角的音质标签,来自数据来源的 info()。 */
const qualityLabels: Record<string, [string, string]> = {
  standard: ["标准", "128 kbps"],
  exhigh: ["极高音质", "320 kbps"],
  lossless: ["无损", "FLAC"],
  hires: ["Hi-Res", "FLAC"],
};

interface Ui {
  mode: Mode;
  view: View;
  immersive: boolean;
  sheet: boolean;
  idle: boolean;
  mobile: boolean;
  trans: boolean;
  /** 面板是否在 DOM 里显示(收起动画结束后才隐藏,所以不等于 mode === "open")。 */
  stageShown: boolean;
  toastText: string;
  toastOn: boolean;
  tipText: string;
  tipOn: boolean;
  badge: [string, string] | null;
  dragging: boolean;
  dragPct: number;
  // 播放列表抽屉
  tab: string;
  /** 第一次打开后才渲染抽屉内容。 */
  sheetReady: boolean;
  sheetStatus: "ready" | "loading" | "error";
  sheetMessage: string;
  sheetTracks: Track[];
  sheetName: string;
  /** 每次重新取列表时加一,让行重新创建(入场动画重新播放)。 */
  sheetKey: number;
}

/**
 * <kite-player>:页面级的悬浮播放器。迷你条 → 面板(桌面)/ 全屏(手机)→ 沉浸模式,
 * 外加播放列表抽屉。页面里的 <kite-track> / <kite-playlist> 与它共用同一个播放核心。
 *
 * 属性:api(公开模式地址)、playlists(播放列表里列出的歌单 ID,逗号分隔;第一个作为初始队列)、
 * theme(auto / light / dark)、collapse(scroll:滚动时自动收起;none:不收起)。
 */
export class KitePlayerElement extends LitElement {
  static override styles = playerSheet;
  static override properties = {
    api: { type: String, reflect: true },
    playlists: { type: String },
    theme: { type: String, reflect: true },
    collapse: { type: String },
  };

  declare api: string | null;
  declare playlists: string;
  declare theme: string | null;
  declare collapse: string | null;

  private player!: Player;
  private s!: PlayerState;
  /** 上一次 updated() 时的播放状态,用来判断这次哪些部分变了。 */
  private shown: PlayerState | null = null;
  private ui: Ui = {
    mode: "bar",
    view: "cover",
    immersive: false,
    sheet: false,
    idle: false,
    mobile: false,
    trans: readFlag("kite-player:trans", true),
    stageShown: false,
    toastText: "",
    toastOn: false,
    tipText: "",
    tipOn: false,
    badge: null,
    dragging: false,
    dragPct: 0,
    tab: "",
    sheetReady: false,
    sheetStatus: "ready",
    sheetMessage: "",
    sheetTracks: [],
    sheetName: "",
    sheetKey: 0,
  };
  private cleanup: (() => void)[] = [];
  private lineEls: HTMLElement[] = [];
  private unitEls: HTMLElement[][] = [];
  private unitValues: number[] = [];
  private tops: number[] = [];
  private lastLine = -1;
  private manual = { active: false, offset: 0, startY: 0, startOffset: 0, moved: false, pointer: -1 };
  private swipe = { x: 0, y: 0, pointer: -1 };
  private hintLine = -1;
  private timers = { idle: 0, resume: 0, toast: 0, tip: 0 };
  private scrollState = { last: 0, acc: 0 };
  private scrollLocked = false;
  private tabPlaylists = new Map<string, Playlist>();

  constructor() {
    super();
    this.api = null;
    this.playlists = "";
    this.theme = null;
    this.collapse = null;
  }

  override connectedCallback(): void {
    super.connectedCallback();
    this.player = playerFor(this, this.api);
    this.s = this.player.state;
    this.bindGlobal();
    this.cleanup.push(
      this.player.subscribe((s) => {
        this.s = s;
        this.requestUpdate();
      }),
      this.player.onTime((t) => this.onFrame(t)),
    );
    void this.loadInitial();
    void this.loadBadge();
  }

  override disconnectedCallback(): void {
    super.disconnectedCallback();
    for (const fn of this.cleanup) fn();
    this.cleanup = [];
    for (const t of Object.values(this.timers)) clearTimeout(t);
    this.unlockScroll();
  }

  /** open 展开播放器;view 可指定打开封面还是歌词。 */
  async open(view?: View): Promise<void> {
    if (view) this.setView(view);
    if (this.ui.mode === "open" || !this.player.state.current) return;
    const bar = this.q(".bar").getBoundingClientRect();
    const disc = this.q(".disc").getBoundingClientRect();
    this.patch({ mode: "open", stageShown: true });
    this.lockScroll();
    await this.updateComplete;
    const stage = this.q(".stage");
    requestAnimationFrame(() => this.layoutLines());
    if (canAnimate(stage)) {
      const to = stage.getBoundingClientRect();
      const radius = parseFloat(getComputedStyle(stage).borderTopLeftRadius) || 0;
      stage.animate([{ clipPath: insetFrom(bar, to, bar.height / 2) }, { clipPath: `inset(0px 0px 0px 0px round ${radius}px)` }], {
        duration: 450,
        easing: "cubic-bezier(.2,.85,.25,1)",
      });
      this.flipCover(disc, 430);
      this.renderRoot.querySelectorAll<HTMLElement>(".stage .enter, .page-lyrics .head").forEach((el, i) => {
        el.animate([{ opacity: 0, transform: "translateY(24px)" }, { opacity: 1, transform: "none" }], {
          duration: 380,
          delay: 100 + i * 60,
          easing: "cubic-bezier(.2,.85,.25,1)",
          fill: "backwards",
        });
      });
    }
    this.resetIdle();
    this.q(".stage .close").focus({ preventScroll: true });
  }

  /** close 收起回迷你条。 */
  close(): void {
    if (this.ui.mode !== "open") return;
    this.closeSheet();
    const stage = this.q(".stage");
    let done = false;
    const finish = () => {
      if (done) return;
      done = true;
      this.patch({ immersive: false, stageShown: false, mode: "bar" });
      this.unlockScroll();
    };
    if (!canAnimate(stage)) {
      finish();
      return;
    }
    const from = stage.getBoundingClientRect();
    const bar = this.q(".bar").getBoundingClientRect();
    const radius = parseFloat(getComputedStyle(stage).borderTopLeftRadius) || 0;
    stage
      .animate([{ clipPath: `inset(0px 0px 0px 0px round ${radius}px)` }, { clipPath: insetFrom(bar, from, bar.height / 2) }], {
        duration: 400,
        easing: "cubic-bezier(.5,0,.25,1)",
      })
      .finished.then(finish, finish);
    // 动画被打断或环境不派发 finished 时也要收起。
    setTimeout(finish, 450);
    stage.querySelectorAll<HTMLElement>(".pages, .cover, .ctl, .top").forEach((el) =>
      el.animate([{ transform: "none", filter: "blur(0px)" }, { transform: "translateY(40px) scale(.72)", filter: "blur(10px)" }], {
        duration: 400,
        easing: "cubic-bezier(.5,0,.25,1)",
      }),
    );
  }

  // ---- 渲染 ----

  protected override render(): TemplateResult {
    const s = this.s;
    const ui = this.ui;
    const t = s.current;
    return html`<div
      class=${classMap({ kp: true, playing: s.playing })}
      part="root"
      data-mode=${ui.mode}
      data-view=${ui.view}
      data-pm=${s.mode}
      data-device=${ui.mobile ? "mobile" : "desktop"}
      ?data-trans=${ui.trans}
      ?data-immersive=${ui.immersive}
      ?data-idle=${ui.idle}
      ?data-sheet=${ui.sheet}
      ?data-has-track=${!!t}
    >
      ${this.renderBar(s, t)} ${this.renderStage(s, t)}
      <div class="backdrop" @click=${this.closeSheet}></div>
      ${this.renderSheet(s)}
      <div class=${classMap({ toast: true, on: ui.toastOn })} role="status" aria-live="polite">${ui.toastText}</div>
    </div>`;
  }

  private renderBar(s: PlayerState, t: Track | null): TemplateResult {
    const pct = s.duration > 0 ? Math.min(1, s.time / s.duration) : 0;
    const ticker = this.tickerText(s);
    return html`<div class="bar" part="bar">
      <button class="disc expand" aria-label="展开播放器" @click=${this.onExpand}>
        <span class="ring" style=${styleMap({ "--pct": String(pct) })}></span><span class="disc-img"><img alt="" src=${ifDefined(t?.album.cover || undefined)} @error=${hideBroken} /></span>
      </button>
      <button class="bar-text expand" aria-label="展开播放器" @click=${this.onExpand}>
        <span class="bar-line"
          ><span class="bar-title ell">${t?.title ?? ""}</span>${t?.trial ? html`<span class="rtag trial">仅试听</span>` : nothing}</span
        ><span class="bar-sub">${keyed(ticker, html`<span class="ell">${ticker}</span>`)}</span>
      </button>
      <div class="bar-ctl">
        <button class="ib desk prev" aria-label="上一首" @click=${this.prev}>${icons.prev}</button>
        <button class="blob toggle" aria-label=${s.playing ? "暂停" : "播放"} @click=${this.toggle}>${blob}</button>
        <button class="ib next" aria-label="下一首" @click=${this.next}>${icons.next}</button>
        <span class="bar-sep desk"></span>
        <button class="ib open-queue" aria-label="播放列表" @click=${this.onOpenQueue}>${icons.queue}</button>
      </div>
    </div>`;
  }

  private renderStage(s: PlayerState, t: Track | null): TemplateResult {
    const ui = this.ui;
    const cover = t?.album.cover || undefined;
    const who = artists(t);
    const tag = [t?.trial ? "仅试听" : "", t?.access === "vip" ? "VIP" : t?.access === "paid" ? "数字专辑" : ""].filter(Boolean).join(" · ");
    const lyr = s.lyrics;
    const empty = !lyr || lyr.instrumental || lyr.lines.length === 0;
    const chorus = lyr?.chorus[0];
    const chorusStyle = chorus && s.duration > 0
      ? { left: `${(chorus.start / 1000 / s.duration) * 100}%`, width: `${((chorus.end - chorus.start) / 1000 / s.duration) * 100}%` }
      : null;
    return html`<div
      class="stage"
      part="panel"
      role="dialog"
      aria-label="正在播放"
      ?hidden=${!ui.stageShown}
      @keydown=${this.onStageKey}
      @pointermove=${this.resetIdle}
      @pointerdown=${this.resetIdle}
      @wheel=${{ handleEvent: () => this.resetIdle(), passive: true }}
    >
      <div class="amb"><img alt="" src=${ifDefined(cover)} @error=${hideBroken} /></div>
      <div class="scrim"></div>
      <div class="pages" @pointerdown=${this.onSwipeStart} @pointerup=${this.onSwipeEnd}>
        <section class="page page-cover">
          <div class="meta enter">
            <span class="meta-title ell">${t?.title ?? ""}</span><span class="meta-sub"
              ><span class="ell meta-artist">${t ? [who, t.album.name].filter(Boolean).join(" · ") : ""}</span><span class="tag access-tag" ?hidden=${!tag}>${tag}</span></span
            >
          </div>
        </section>
        <section class="page page-lyrics">
          <button class="head to-cover" aria-label="返回封面" @click=${this.toCover}><span class="head-title ell">${t?.title ?? ""}</span><span class="head-sub ell">${who}</span></button>
          <div
            class="lyrics"
            part="lyrics"
            @wheel=${{ handleEvent: (e: WheelEvent) => this.onLyricsWheel(e), passive: false }}
            @pointerdown=${this.onLyricsDown}
            @pointermove=${this.onLyricsMove}
            @pointerup=${this.onLyricsUp}
            @pointercancel=${this.onLyricsUp}
          >
            <div class="lines" role="list" @click=${this.onLineClick}>${guard([lyr], () => renderLines(lyr))}</div>
            <div class="lyr-empty" ?hidden=${!empty}>${eq}<span class="lyr-empty-text">${empty ? this.emptyText(s) : ""}</span></div>
            <button class="seek" aria-label="跳到这一句" @click=${this.seekToHint}>${icons.play}<span></span></button>
          </div>
        </section>
      </div>
      <button class="cover" aria-label=${ui.view === "cover" ? "查看歌词" : "返回封面"} @click=${this.onCoverClick}>
        <img alt="" src=${ifDefined(cover)} @error=${hideBroken} /><span class="badge" ?hidden=${!ui.badge}><b>${ui.badge?.[0] ?? ""}</b><span>${ui.badge?.[1] ?? ""}</span></span>
      </button>
      <div class="top enter">
        <button class="ib close" aria-label="收起" @click=${this.close}>${icons.chevron}</button>
        <div class="dots"><button data-go="cover" aria-label="封面" @click=${this.toCover}></button><button data-go="lyrics" aria-label="歌词" @click=${this.toLyrics}></button></div>
        <button class="ib immersive" aria-label=${ui.immersive ? "退出沉浸模式" : "沉浸模式"} @click=${this.onImmersiveClick}>${ui.immersive ? icons.shrink : icons.expand}</button>
      </div>
      <div class="ctl enter" part="controls">
        <div class="progress">
          <span class="ptrack"></span><span class="pchorus" ?hidden=${!chorusStyle} style=${styleMap(chorusStyle ?? {})}></span><span class="pfill"></span><span class="pthumb"></span
          ><input type="range" min="0" max="1000" step="1" value="0" aria-label="播放进度" @input=${this.onRangeInput} @change=${this.onRangeChange} />
        </div>
        <div class="times"><span class="t-cur">${formatTime(ui.dragging ? ui.dragPct * s.duration : s.time)}</span><span class="t-dur">${formatTime(s.duration)}</span></div>
        <div class="buttons">
          <button class="ib mode" aria-label=${"播放模式:" + modeLabels[s.mode]} @click=${this.onModeClick}>
            <span class="m m-sequence">${icons.sequence}</span><span class="m m-loop">${icons.loop}</span><span class="m m-one">${icons.one}</span><span class="m m-shuffle">${icons.shuffle}</span>
          </button>
          <button class="ib big prev" aria-label="上一首" @click=${this.prev}>${icons.prev}</button>
          <button class="blob toggle" aria-label=${s.playing ? "暂停" : "播放"} @click=${this.toggle}>${blob}</button>
          <button class="ib big next" aria-label="下一首" @click=${this.next}>${icons.next}</button>
          <button class="ib open-queue" aria-label="播放列表" @click=${this.onOpenQueue}>${icons.queue}</button>
        </div>
        <div class="extra">
          <button class="ib chorus-btn" aria-label="从副歌开始" ?disabled=${!chorus} @click=${this.playChorus}>${icons.wave}</button>
          <button class="ib trans-btn" aria-label="显示翻译" aria-pressed=${String(ui.trans)} @click=${this.toggleTrans}>${icons.lyric}</button>
        </div>
        <div class=${classMap({ tip: true, on: ui.tipOn })}>${ui.tipText}</div>
      </div>
    </div>`;
  }

  private renderSheet(s: PlayerState): TemplateResult {
    const ui = this.ui;
    const ids = ui.sheetReady ? this.tabIds() : [];
    const at = Math.max(ids.indexOf(ui.tab), 0);
    return html`<div class="sheet" part="queue" role="dialog" aria-label="播放列表" @keydown=${this.onSheetKey}>
      <div class="sheet-head">
        <span class="handle"></span>
        <div class="sheet-title"><span>播放列表</span><button class="ib close-queue" aria-label="关闭" @click=${this.closeSheet}>${icons.close}</button></div>
        <div class="tabs" role="tablist" ?hidden=${ui.sheetReady && ids.length < 2}>
          <span
            class="ind"
            style=${styleMap(ui.sheetReady ? { width: `calc((100% - 8px) / ${Math.max(ids.length, 1)})`, transform: `translateX(${at * 100}%)` } : {})}
          ></span
          >${ids.map((id) => html`<button role="tab" data-id=${id} aria-selected=${String(id === ui.tab)} @click=${() => this.pickTab(id)}>${this.tabName(id)}</button>`)}
        </div>
        <button class="moderow mode-row" @click=${this.onModeRowClick}>
          <span class="mode-icon">${icons[s.mode]}</span><span class="mode-label">${modeLabels[s.mode]}</span><span class="src">${ui.sheetName ? `来自「${ui.sheetName}」` : ""}</span>
        </button>
      </div>
      <div class="list" role="list">${this.renderRows(s)}</div>
    </div>`;
  }

  private renderRows(s: PlayerState): unknown {
    const ui = this.ui;
    if (!ui.sheetReady) return nothing;
    if (ui.sheetStatus === "loading") return html`<div class="empty">加载中…</div>`;
    if (ui.sheetStatus === "error") return html`<div class="empty">${ui.sheetMessage}</div>`;
    if (!ui.sheetTracks.length) return html`<div class="empty">这里还没有歌</div>`;
    const cur = s.current?.id;
    return keyed(
      ui.sheetKey,
      repeat(
        ui.sheetTracks,
        (t, i) => `${i}:${t.id}`,
        (t, i) => {
          const isCur = t.id === cur;
          const tag = [
            t.trial ? html`<span class="rtag trial">仅试听</span>` : nothing,
            t.access === "vip" ? html`<span class="rtag vip">VIP</span>` : t.access === "paid" ? html`<span class="rtag paid">数字专辑</span>` : nothing,
          ];
          const sub = [artists(t), t.album.name].filter(Boolean).join(" · ");
          return html`<button class=${classMap({ row: true, locked: !canPlay(t), cur: isCur })} role="listitem" data-i=${i} style="--i:${Math.min(i, 12)}" @click=${() => this.pickRow(i)}>
            <span class="row-main"
              ><span class="row-title"><span class="ell">${t.title}</span>${isCur ? eq : nothing}</span><span class="row-sub">${tag}<span class="ell">${sub}</span></span></span
            >${canPlay(t) ? html`<span class="row-dur">${formatTime(t.durationMs / 1000)}</span>` : html`<span class="lock">${icons.lock}</span>`}
          </button>`;
        },
      ),
    );
  }

  private tickerText(s: PlayerState): string {
    // 署名行(作词 / 作曲)期间播放条仍显示歌手,唱到第一句才换成歌词。
    const lines = s.lyrics?.lines ?? [];
    const line = s.lyricIndex >= 0 && s.lyricIndex >= firstSungIndex(lines) ? lines[s.lyricIndex]?.text : "";
    return line || artists(s.current);
  }

  private emptyText(s: PlayerState): string {
    const lyr = s.lyrics;
    if (!s.current) return "";
    if (!lyr) return s.lyricsStatus === "failed" ? "歌词暂时加载不了" : "歌词加载中…";
    return lyr.instrumental ? "纯音乐,请安静欣赏" : "暂无歌词";
  }

  /** updated:模板更新之后的命令式部分——歌词行的位置、进度条、提示。 */
  protected override updated(changed: PropertyValues): void {
    super.updated(changed);
    const s = this.s;
    const prev = this.shown;
    this.shown = s;
    if (changed.has("playlists") && this.ui.sheet) void this.loadSheet();
    if (!prev || s.current !== prev.current) {
      this.lastLine = -1;
      this.endManual(false);
    }
    if (!prev || s.time !== prev.time || s.duration !== prev.duration) {
      if (!this.ui.dragging) {
        const pct = s.duration > 0 ? Math.min(1, s.time / s.duration) : 0;
        this.setPct(pct);
        this.q<HTMLInputElement>(".progress input").value = String(Math.round(pct * 1000));
      }
    }
    if (!prev || s.lyrics !== prev.lyrics || s.lyricsStatus !== prev.lyricsStatus) {
      this.lineEls = [...this.renderRoot.querySelectorAll<HTMLElement>(".lines .line")];
      this.unitEls = this.lineEls.map((el) => [...el.querySelectorAll<HTMLElement>(".u")]);
      this.unitValues = [];
      this.lastLine = -1;
      this.layoutLines();
    } else if (s.lyricIndex !== prev.lyricIndex) {
      this.positionLines(false);
    }
    if (prev && s.playing !== prev.playing && !s.playing) this.resetIdle();
    if (prev && s.error && s.error !== prev.error) this.toast(s.error.message);
  }

  // ---- 事件 ----

  private onExpand = (): void => {
    if (this.ui.mode === "orb") this.patch({ mode: "bar" });
    else void this.open("cover");
  };
  private toggle = (): void => void this.player.toggle();
  private prev = (): void => this.player.prev();
  private next = (): void => this.player.next();
  private playChorus = (): void => this.player.playChorus();
  private onOpenQueue = (): void => void this.openSheet();
  private toCover = (): void => this.setView("cover");
  private toLyrics = (): void => this.setView("lyrics");
  private onCoverClick = (): void => {
    if (!this.ui.immersive) this.setView(this.ui.view === "cover" ? "lyrics" : "cover");
  };
  private onImmersiveClick = (): void => void this.setImmersive(!this.ui.immersive, true);
  private onModeClick = (): void => this.cycleMode(true);
  private onModeRowClick = (): void => this.cycleMode(false);

  private onRangeInput = (e: Event): void => {
    const pct = Number((e.target as HTMLInputElement).value) / 1000;
    this.patch({ dragging: true, dragPct: pct });
    this.setPct(pct);
  };

  private onRangeChange = (e: Event): void => {
    const pct = Number((e.target as HTMLInputElement).value) / 1000;
    this.patch({ dragging: false });
    this.player.seek(pct * this.player.state.duration);
  };

  private onStageKey = (e: KeyboardEvent): void => {
    this.resetIdle();
    if (e.key !== "Escape") return;
    e.stopPropagation();
    if (this.ui.sheet) this.closeSheet();
    else if (this.ui.immersive) void this.setImmersive(false, true);
    else this.close();
  };

  private onSheetKey = (e: KeyboardEvent): void => {
    if (e.key === "Escape") this.closeSheet();
  };

  private onSwipeStart = (e: PointerEvent): void => {
    if (this.ui.immersive || (e.target as HTMLElement).closest(".lyrics")) return;
    this.swipe = { x: e.clientX, y: e.clientY, pointer: e.pointerId };
  };

  private onSwipeEnd = (e: PointerEvent): void => {
    if (e.pointerId !== this.swipe.pointer) return;
    this.swipe.pointer = -1;
    const dx = e.clientX - this.swipe.x;
    if (Math.abs(dx) < 56 || Math.abs(dx) < Math.abs(e.clientY - this.swipe.y) * 1.5) return;
    this.setView(dx < 0 ? "lyrics" : "cover");
  };

  private onLyricsWheel(e: WheelEvent): void {
    if (!this.lineEls.length) return;
    e.preventDefault();
    this.beginManual();
    this.manual.offset -= e.deltaY;
    this.positionLines(true);
    this.scheduleResume();
  }

  private onLyricsDown = (e: PointerEvent): void => {
    if (!this.lineEls.length || e.pointerType === "mouse") return;
    this.manual.pointer = e.pointerId;
    this.manual.startY = e.clientY;
    this.manual.startOffset = this.manual.offset;
    this.manual.moved = false;
  };

  private onLyricsMove = (e: PointerEvent): void => {
    if (e.pointerId !== this.manual.pointer) return;
    const dy = e.clientY - this.manual.startY;
    if (!this.manual.moved && Math.abs(dy) < 8) return;
    this.manual.moved = true;
    this.beginManual();
    this.manual.offset = this.manual.startOffset + dy;
    this.positionLines(true);
  };

  private onLyricsUp = (e: PointerEvent): void => {
    if (e.pointerId !== this.manual.pointer) return;
    this.manual.pointer = -1;
    if (this.manual.moved) this.scheduleResume();
    setTimeout(() => (this.manual.moved = false), 0);
  };

  private onLineClick = (e: Event): void => {
    const line = (e.target as HTMLElement).closest<HTMLElement>(".line");
    if (!line || this.manual.moved) return;
    const l = this.player.state.lyrics?.lines[Number(line.dataset.i)];
    if (!l) return;
    this.endManual(true);
    this.player.seek(l.time / 1000);
    void this.player.play();
  };

  /** bindGlobal 绑定 window / 媒体查询上的事件,每次挂载都绑,卸载时解绑。 */
  private bindGlobal(): void {
    const listen = (target: EventTarget, type: string, fn: (e: Event) => void, opts?: AddEventListenerOptions) => {
      target.addEventListener(type, fn, opts);
      this.cleanup.push(() => target.removeEventListener(type, fn, opts));
    };
    const mq = matchMedia("(max-width: 767px)");
    const onDevice = () => {
      this.patch({ mobile: mq.matches });
      if (mq.matches && this.ui.immersive) void this.setImmersive(false);
      requestAnimationFrame(() => this.layoutLines());
    };
    onDevice();
    listen(mq, "change", onDevice);
    listen(window, "resize", () => this.layoutLines(), { passive: true });
    this.scrollState.last = scrollY;
    listen(window, "scroll", () => this.onScroll(), { passive: true });
    listen(window, flyEvent, (e) => this.fly((e as CustomEvent<FlyDetail>).detail));
    listen(window, openQueueEvent, (e) => void this.openSheet((e as CustomEvent<{ playlist?: string }>).detail?.playlist));
    listen(window, toastEvent, (e) => this.toast(String((e as CustomEvent<string>).detail ?? "")));
    listen(window, openEvent, (e) => {
      const d = (e as CustomEvent<OpenDetail>).detail ?? {};
      void this.open(d.view).then(() => {
        if (d.immersive && !this.ui.mobile) void this.setImmersive(true, true);
      });
    });
  }

  // ---- 状态 ----

  /** patch 改界面状态并安排一次重新渲染。 */
  private patch(p: Partial<Ui>): void {
    Object.assign(this.ui, p);
    this.requestUpdate();
  }

  /** q 取 shadow DOM 里必定存在的元素。 */
  private q<T extends Element = HTMLElement>(sel: string): T {
    return $<T>(this.renderRoot, sel);
  }

  private setView(view: View): void {
    if (this.ui.view === view) return;
    this.patch({ view });
    this.endManual(false);
    this.resetIdle();
    requestAnimationFrame(() => this.layoutLines());
  }

  private async setImmersive(on: boolean, animate = false): Promise<void> {
    if (this.ui.immersive === on || (on && this.ui.mobile)) return;
    const stage = this.q(".stage");
    const before = stage.getBoundingClientRect();
    this.patch({ immersive: on });
    this.resetIdle();
    await this.updateComplete;
    if (animate && on && canAnimate(stage)) {
      const after = stage.getBoundingClientRect();
      stage.animate([{ clipPath: insetFrom(before, after, 28) }, { clipPath: "inset(0px 0px 0px 0px round 0px)" }], {
        duration: 600,
        easing: "cubic-bezier(.2,.85,.25,1)",
      });
    }
    setTimeout(() => this.layoutLines(), 520);
    requestAnimationFrame(() => this.layoutLines());
  }

  private cycleMode(showTip: boolean): void {
    const mode = this.player.cycleMode();
    if (!showTip) return;
    this.patch({ tipText: modeLabels[mode], tipOn: true });
    clearTimeout(this.timers.tip);
    this.timers.tip = window.setTimeout(() => this.patch({ tipOn: false }), 1400);
  }

  private toggleTrans = async (): Promise<void> => {
    const trans = !this.ui.trans;
    writeFlag("kite-player:trans", trans);
    this.patch({ trans });
    await this.updateComplete;
    this.layoutLines();
  };

  /** resetIdle:歌词页停止操作 3 秒后隐藏播放控制,任何操作让它回来。 */
  private resetIdle = (): void => {
    if (this.ui.idle) this.patch({ idle: false });
    clearTimeout(this.timers.idle);
    if (this.ui.mode !== "open" || this.ui.view !== "lyrics" || this.ui.immersive) return;
    this.timers.idle = window.setTimeout(() => {
      if (!this.player.state.playing || this.ui.view !== "lyrics" || this.ui.immersive || this.ui.sheet) return;
      this.patch({ idle: true });
    }, idleDelay);
  };

  private onScroll(): void {
    const y = scrollY;
    const dy = y - this.scrollState.last;
    this.scrollState.last = y;
    if (this.collapse === "none" || this.ui.mode === "open" || !this.player.state.current) return;
    this.scrollState.acc = Math.sign(dy) === Math.sign(this.scrollState.acc) ? this.scrollState.acc + dy : dy;
    if (this.scrollState.acc > collapseScroll && this.ui.mode === "bar") this.patch({ mode: "orb" });
    else if (this.scrollState.acc < -collapseScroll && this.ui.mode === "orb") this.patch({ mode: "bar" });
  }

  private async loadInitial(): Promise<void> {
    const first = this.playlistIds()[0];
    if (!first || this.player.state.queue.length > 0) return;
    await this.player.whenSource();
    try {
      const pl = await this.player.loadPlaylist(first);
      if (this.player.state.queue.length > 0) return;
      const i = pl.tracks.findIndex(canPlay);
      this.player.setQueue(pl.tracks, Math.max(i, 0), { id: pl.id, name: pl.name }, false);
    } catch (err) {
      this.toast((err as Error).message || "歌单加载失败");
    }
  }

  private async loadBadge(): Promise<void> {
    await this.player.whenSource();
    const info = await this.player.info().catch(() => null);
    const label = info ? qualityLabels[info.quality] : undefined;
    if (label) this.patch({ badge: label });
  }

  private playlistIds(): string[] {
    return (this.playlists ?? "")
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
  }

  private setPct(pct: number): void {
    this.q(".progress").style.setProperty("--pct", String(pct));
  }

  // ---- 歌词 ----

  /** layoutLines 量出每行的高度,算好每行的位置。面板隐藏时量不到,打开后再算。 */
  private layoutLines(): void {
    if (!this.isConnected || !this.lineEls.length || this.ui.mode !== "open") return;
    let top = 0;
    this.tops = this.lineEls.map((el) => {
      const t = top;
      top += el.offsetHeight + 14;
      return t;
    });
    this.positionLines(true);
  }

  /**
   * positionLines 把当前行移到锚点。换句时每行的 transition-delay 按离当前行的距离递增,
   * 下方的行晚一点出发,行距先拉开再合拢(参考网易云的歌词滚动)。
   */
  private positionLines(immediate: boolean): void {
    if (!this.isConnected || !this.lineEls.length || !this.tops.length) return;
    const { lyricIndex } = this.player.state;
    const cur = Math.max(lyricIndex, 0);
    const box = this.q(".lyrics");
    const anchor = box.clientHeight * (this.ui.immersive ? 0.36 : 0.28);
    const base = anchor - (this.tops[cur] ?? 0) + this.manual.offset;
    const forward = cur >= this.lastLine;
    this.lineEls.forEach((el, i) => {
      const steps = forward ? i - cur + 1 : cur - i + 1;
      const delay = immediate || this.manual.active ? 0 : Math.min(Math.max(steps, 0), 12) * lyricStagger;
      el.style.transitionDuration = immediate ? "0ms" : "";
      el.style.transitionDelay = `${delay}ms`;
      el.style.transform = `translateY(${(this.tops[i] ?? 0) + base}px)`;
      el.classList.toggle("cur", i === lyricIndex);
      el.classList.toggle("far", Math.abs(i - cur) >= 3);
    });
    if (lyricIndex !== this.lastLine) {
      this.unitValues = [];
      this.unitEls[this.lastLine]?.forEach((u) => u.style.removeProperty("--p"));
    }
    this.lastLine = cur;
    if (this.manual.active) this.updateHint(base, anchor);
  }

  private onFrame(t: number): void {
    const s = this.player.state;
    if (!this.hasUpdated) return;
    if (s.duration > 0 && !this.ui.dragging && this.ui.mode === "open") this.setPct(Math.min(1, t / s.duration));
    if (this.ui.mode !== "open" || (this.ui.view !== "lyrics" && !this.ui.immersive)) return;
    const line = s.lyrics?.lines[s.lyricIndex];
    const units = this.unitEls[s.lyricIndex];
    if (!line || !units) return;
    const p = wordProgress(line, t * 1000);
    units.forEach((u, k) => {
      const v = Math.round((p[k] ?? 0) * 100) / 100;
      if (this.unitValues[k] === v) return;
      this.unitValues[k] = v;
      u.style.setProperty("--p", String(v));
    });
  }

  private beginManual(): void {
    if (this.manual.active) return;
    this.manual.active = true;
    this.q(".lyrics").classList.add("manual");
  }

  private scheduleResume(): void {
    clearTimeout(this.timers.resume);
    this.timers.resume = window.setTimeout(() => this.endManual(true), resumeDelay);
  }

  private endManual(animate: boolean): void {
    clearTimeout(this.timers.resume);
    if (!this.manual.active && this.manual.offset === 0) return;
    this.manual.active = false;
    this.manual.offset = 0;
    this.renderRoot.querySelector(".lyrics")?.classList.remove("manual");
    if (animate) this.positionLines(false);
  }

  private updateHint(base: number, anchor: number): void {
    let best = 0;
    let bestD = Infinity;
    this.tops.forEach((t, i) => {
      const d = Math.abs(t + base - anchor);
      if (d < bestD) {
        bestD = d;
        best = i;
      }
    });
    this.hintLine = best;
    const line = this.player.state.lyrics?.lines[best];
    const hint = this.q(".seek");
    hint.style.top = `${anchor + 4}px`;
    $(hint, "span").textContent = formatTime((line?.time ?? 0) / 1000);
  }

  private seekToHint = (): void => {
    const line = this.player.state.lyrics?.lines[this.hintLine];
    if (!line) return;
    this.endManual(true);
    this.player.seek(line.time / 1000);
    void this.player.play();
  };

  // ---- 播放列表抽屉 ----

  private async openSheet(playlist?: string): Promise<void> {
    if (!this.player.state.current && !this.playlistIds().length && !playlist) return;
    this.patch({ sheet: true, tab: playlist ?? this.player.state.playlist?.id ?? this.playlistIds()[0] ?? "queue" });
    await this.loadSheet();
    await this.updateComplete;
    this.q(".close-queue").focus({ preventScroll: true });
  }

  private closeSheet = (): void => {
    if (!this.ui.sheet) return;
    this.patch({ sheet: false });
    this.resetIdle();
  };

  private pickTab(id: string): void {
    this.patch({ tab: id });
    void this.loadSheet();
  }

  private tabIds(): string[] {
    const ids = this.playlistIds();
    const cur = this.player.state.playlist?.id;
    if (cur && !ids.includes(cur)) ids.unshift(cur);
    if (!cur && this.player.state.queue.length) ids.unshift("queue");
    return ids;
  }

  /** loadSheet 取当前标签的歌曲:当前队列直接用,其它歌单向数据来源取。 */
  private async loadSheet(): Promise<void> {
    const ids = this.tabIds();
    const tab = ids.includes(this.ui.tab) ? this.ui.tab : (ids[0] ?? "queue");
    const s = this.player.state;
    this.prefetchTabNames(ids);
    if (tab === "queue" || tab === s.playlist?.id) {
      this.patch({ tab, sheetReady: true, sheetStatus: "ready", sheetTracks: s.queue, sheetName: s.playlist?.name ?? "", sheetKey: this.ui.sheetKey + 1 });
      return;
    }
    this.patch({ tab, sheetReady: true, sheetStatus: "loading", sheetName: "" });
    try {
      const pl = await this.player.loadPlaylist(tab);
      this.tabPlaylists.set(tab, pl);
      if (this.ui.tab !== tab) return;
      this.patch({ sheetStatus: "ready", sheetTracks: pl.tracks, sheetName: pl.name, sheetKey: this.ui.sheetKey + 1 });
    } catch (err) {
      if (this.ui.tab !== tab) return;
      this.patch({ sheetStatus: "error", sheetMessage: (err as Error).message || "歌单加载失败" });
    }
  }

  /** prefetchTabNames 取其它标签的歌单(播放核心会缓存),标签上显示歌单名而不是「歌单」。 */
  private prefetchTabNames(ids: string[]): void {
    for (const id of ids) {
      if (id === "queue" || id === this.player.state.playlist?.id || this.tabPlaylists.has(id)) continue;
      this.player.loadPlaylist(id).then(
        (pl) => {
          this.tabPlaylists.set(id, pl);
          this.requestUpdate();
        },
        () => {
          // 取不到时标签显示「歌单」,点开时再报错。
        },
      );
    }
  }

  private tabName(id: string): string {
    if (id === "queue") return "当前播放";
    if (id === this.player.state.playlist?.id) return this.player.state.playlist.name;
    return this.tabPlaylists.get(id)?.name ?? "歌单";
  }

  private pickRow(i: number): void {
    const t = this.ui.sheetTracks[i];
    if (!t) return;
    if (!canPlay(t)) {
      this.toast(unplayableMessage(t));
      return;
    }
    const s = this.player.state;
    if (s.current?.id === t.id) {
      void this.player.toggle();
      return;
    }
    if (this.ui.tab === "queue" || this.ui.tab === s.playlist?.id) {
      this.player.setQueue(s.queue, i, s.playlist, true);
    } else {
      const pl = this.tabPlaylists.get(this.ui.tab);
      this.player.setQueue(this.ui.sheetTracks, i, pl ? { id: pl.id, name: pl.name } : null, true);
    }
  }

  // ---- 其它 ----

  private toast(message: string): void {
    this.patch({ toastText: message, toastOn: true });
    clearTimeout(this.timers.toast);
    this.timers.toast = window.setTimeout(() => this.patch({ toastOn: false }), 2600);
  }

  /** flipCover 让大封面从迷你条的唱片位置放大到位(共享元素)。 */
  private flipCover(from: DOMRect, duration: number): void {
    const cover = this.q(".cover");
    const to = cover.getBoundingClientRect();
    if (!to.width || !canAnimate(cover)) return;
    const s = from.width / to.width;
    const radius = getComputedStyle(cover).borderTopLeftRadius;
    cover.animate(
      [
        { transformOrigin: "0 0", transform: `translate(${from.left - to.left}px, ${from.top - to.top}px) scale(${s})`, borderRadius: `${from.width / 2 / s}px` },
        { transformOrigin: "0 0", transform: "none", borderRadius: radius },
      ],
      { duration, easing: "cubic-bezier(.2,.85,.25,1)" },
    );
  }

  /** fly:卡片的封面沿弧线飞进播放器(横向先快后慢、纵向先慢后快)。 */
  private fly(d: FlyDetail): void {
    const kp = this.renderRoot.querySelector<HTMLElement>(".kp");
    if (!d?.rect || !kp || !canAnimate(kp)) return;
    requestAnimationFrame(() => {
      const target = this.q(this.ui.mode === "open" ? ".cover" : ".disc").getBoundingClientRect();
      if (!target.width) return;
      // 动画期间的临时元素,不进模板。
      const ghost = document.createElement("div");
      ghost.className = "ghost";
      const y = document.createElement("div");
      y.className = "ghost-y";
      const inner = document.createElement("div");
      inner.className = "ghost-in";
      const img = document.createElement("img");
      img.alt = "";
      img.src = d.src;
      inner.append(img);
      y.append(inner);
      ghost.append(y);
      kp.append(ghost);
      const opts = { duration: 600, fill: "forwards" as const };
      ghost.animate([{ transform: `translateX(${d.rect.left}px)` }, { transform: `translateX(${target.left}px)` }], { ...opts, easing: "cubic-bezier(.25,.8,.35,1)" });
      y.animate([{ transform: `translateY(${d.rect.top}px)` }, { transform: `translateY(${target.top}px)` }], { ...opts, easing: "cubic-bezier(.55,0,.85,.45)" });
      inner
        .animate(
          [
            { width: `${d.rect.width}px`, height: `${d.rect.height}px`, borderRadius: "12px" },
            { width: `${target.width}px`, height: `${target.height}px`, borderRadius: `${target.width / 2}px` },
          ],
          { ...opts, easing: "cubic-bezier(.2,.85,.25,1)" },
        )
        .finished.then(
          () => {
            ghost.remove();
            this.q(this.ui.mode === "open" ? ".cover" : ".disc").animate([{ transform: "scale(1)" }, { transform: "scale(1.14)" }, { transform: "scale(1)" }], {
              duration: 460,
              easing: "cubic-bezier(.2,.85,.25,1)",
            });
          },
          () => ghost.remove(),
        );
    });
  }

  private lockScroll(): void {
    if (!this.ui.mobile || this.scrollLocked) return;
    this.scrollLocked = true;
    document.documentElement.style.overflow = "hidden";
  }

  private unlockScroll(): void {
    if (!this.scrollLocked) return;
    this.scrollLocked = false;
    document.documentElement.style.overflow = "";
  }
}

/** renderLines 渲染歌词行;换一份歌词时整体重建(keyed),不复用上一首的行与逐字状态。 */
function renderLines(lyr: Lyrics | null): unknown {
  if (!lyr || lyr.instrumental || lyr.lines.length === 0) return nothing;
  return keyed(
    lyr,
    lyr.lines.map(
      (line, i) =>
        html`<button class="line" role="listitem" data-i=${i}><span class="lt">${lineUnits(line).map((u) => html`<span class="u">${u}</span>`)}</span>${line.translation
            ? html`<span class="tr">${line.translation}</span>`
            : nothing}</button>`,
    ),
  );
}

/** insetFrom 把 inner 矩形表示成相对 outer 的 clip-path inset(用于从迷你条长成面板)。 */
function insetFrom(inner: DOMRect, outer: DOMRect, radius: number): string {
  const t = inner.top - outer.top;
  const r = outer.right - inner.right;
  const b = outer.bottom - inner.bottom;
  const l = inner.left - outer.left;
  return `inset(${t}px ${r}px ${b}px ${l}px round ${radius}px)`;
}
