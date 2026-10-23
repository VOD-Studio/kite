import { html, LitElement, nothing, type PropertyValues, type TemplateResult } from "lit";
import { classMap } from "lit/directives/class-map.js";
import { guard } from "lit/directives/guard.js";
import { ifDefined } from "lit/directives/if-defined.js";
import { keyed } from "lit/directives/keyed.js";
import { repeat } from "lit/directives/repeat.js";
import { styleMap } from "lit/directives/style-map.js";
import { firstSungIndex, formatTime, lineUnits, wordProgress } from "../core/lyrics";
import { modeLabels, type Player } from "../core/player";
import type { Lyrics, PlayerState, Playlist, Quality, Track } from "../core/types";
import { blob, coverPh, eq, icons } from "./icons";
import {
  apply as applyMorph,
  clear as clearMorph,
  cubicBezier,
  type MorphGeo,
  type MorphParts,
  measure,
  frame as morphFrame,
  play as playMorph,
} from "./morph";
import { playerSheet } from "./styles";
import {
  $,
  artists,
  canAnimate,
  canPlay,
  type FlyDetail,
  flyEvent,
  type OpenDetail,
  onCoverError,
  onCoverLoad,
  openEvent,
  openQueueEvent,
  playerFor,
  readFlag,
  toastEvent,
  unplayableMessage,
  writeFlag,
} from "./util";

type Mode = "bar" | "orb" | "open";
type View = "cover" | "lyrics";
type Position = "left" | "center" | "right";
/** 歌词行怎么移到新位置:瞬间、跟随(无错开)、换句的逐行波浪、以当前句为中心向两侧错开(翻译开关)。 */
type Motion = "instant" | "follow" | "wave" | "center";

/** 换句时每往下一行多晚 35ms 开始移动,形成逐行错开的波浪。 */
const lyricStagger = 35;
/** 歌词页没有操作多久后隐藏播放控制:桌面鼠标一动就回来,所以短一些;手机要点歌词才回来。 */
const idleDelay = { desktop: 3000, mobile: 5000 };
/** 手动拖动歌词后多久回到当前行。 */
const resumeDelay = 2500;
/** 向下滚动多少像素后收起为圆形。 */
const collapseScroll = 24;
/** 手指移动多少像素后判定为拖动(之前算点按)。 */
const dragSlop = 8;
/** 松手时横向速度超过它(px/ms)就翻到下一页,不看拖了多远。 */
const flingSpeed = 0.35;
/** 展开 / 收起的时长与缓动(见 morph.ts)。 */
const easeOut = "cubic-bezier(.2,.85,.25,1)";
const morphOpen = { duration: 460, ease: cubicBezier(0.2, 0.85, 0.25, 1) };
const morphClose = { duration: 380, ease: cubicBezier(0.4, 0, 0.2, 1) };

// 封面页 ↔ 歌词页的进度。注册成不继承的属性:只写在用到它的几个元素上,改动时不会让整棵歌词树重新计算样式。
// 浏览器不支持时退回普通变量,照样能用。同一页面加载两份本包时第二次注册会抛错,忽略即可。
try {
  globalThis.CSS?.registerProperty?.({ name: "--kite-pg", syntax: "<number>", inherits: false, initialValue: "0" });
} catch {
  // 已经注册过。
}
/** 上拉展开 / 下拉收起:松手时甩动速度超过它(px/ms)就按方向完成,不看拖了多远。 */
const pullSpeed = 0.5;
/** 歌词拖过首句 / 末句之后的跟手比例与最多能拖出去多远。 */
const rubberRatio = 0.35;
const rubberMax = 80;
/** 拖歌词松手后的惯性:速度按这个时间常数(ms)指数衰减,最多滑这么久。 */
const flingTau = 325;
const flingMax = 1200;

/**
 * 封面左下角的音质标签(参考网易云):[音质名, 规格]。只有极高及以上才标,标准音质、试听片段、没有信息时不显示。
 * 开启高音质时是金色音质名 + 斜切的深色规格;关掉时只剩描边的音质名。
 */
const qualityBadges: Partial<Record<Quality, [string, string]>> = {
  exhigh: ["极高音质", "320kbps"],
  lossless: ["无损音质", "16bit FLAC"],
  hires: ["Hi-Res", "24bit FLAC"],
};

function qualityBadge(t: Track | null | undefined): [string, string] | null {
  if (!t || t.trial || !t.quality) return null;
  return qualityBadges[t.quality] ?? null;
}

const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v));

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
  /** 上一次显示过的音质标签:标签淡出时文字不能先变空。 */
  lastBadge: [string, string];
}

/**
 * <kite-player>:页面级的悬浮播放器。迷你条 → 面板(桌面)/ 全屏(手机)→ 沉浸模式,
 * 外加播放列表抽屉。页面里的 <kite-track> / <kite-playlist> 与它共用同一个播放核心。
 *
 * 属性:api(公开模式地址)、playlists(播放列表里列出的歌单 ID,逗号分隔;第一个作为初始队列)、
 * theme(auto / light / dark)、collapse(scroll:滚动时自动收起;none:不收起)、
 * position(桌面上放在 left / center / right,默认 right;手机始终底部通栏)。
 */
export class KitePlayerElement extends LitElement {
  static override styles = playerSheet;
  static override properties = {
    api: { type: String, reflect: true },
    playlists: { type: String },
    theme: { type: String, reflect: true },
    collapse: { type: String },
    position: { type: String, reflect: true },
  };

  declare api: string | null;
  declare playlists: string;
  declare theme: string | null;
  declare collapse: string | null;
  declare position: Position | null;

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
    dragging: false,
    dragPct: 0,
    tab: "",
    sheetReady: false,
    sheetStatus: "ready",
    sheetMessage: "",
    sheetTracks: [],
    sheetName: "",
    lastBadge: ["", ""],
  };
  private cleanup: (() => void)[] = [];
  private lineEls: HTMLElement[] = [];
  private unitEls: HTMLElement[][] = [];
  private unitValues: number[] = [];
  private tops: number[] = [];
  private lastLine = -1;
  private manual = { active: false, offset: 0, startOffset: 0 };
  /**
   * 面板上的一次触摸手势:先按移动方向定轴,横向拖动翻页(封面 ↔ 歌词,跟手),
   * 在歌词上纵向拖动是手动滚歌词,没移动就是点按。
   */
  private gesture = {
    pointer: -1,
    x: 0,
    y: 0,
    axis: "" as "" | "x" | "y",
    onLyrics: false,
    from: 0,
    width: 1,
    pg: 0,
    vx: 0,
    vy: 0,
    lastX: 0,
    lastY: 0,
    lastT: 0,
  };
  /**
   * 上拉展开 / 下拉收起的跟手状态(手机):卡片顶边跟着手指,p 是形变进度(0 播放条,1 全屏)。
   * geo 在面板显示出来之后才有,在那之前的移动只记下位置。
   */
  private pull: {
    pointer: number;
    opening: boolean;
    startY: number;
    lastY: number;
    lastT: number;
    vy: number;
    p: number;
    geo: MorphGeo | null;
  } | null = null;
  /** 正在播放的形变:用来在中途打断时算出当前进度,从那里接着走。 */
  private morphRun: { from: number; to: number; duration: number; ease: (t: number) => number } | null = null;
  private morphParts: MorphParts | null = null;
  /** 播放条上一次按下的位置,用来判断是不是在往上拉。 */
  private barPress = { pointer: -1, x: 0, y: 0 };
  /** 每次显示 / 收起面板加一,过期的收起回调据此放弃。 */
  private stageToken = 0;
  private momentum = 0;
  /** 拖动或点按已经处理过时,吞掉随后的 click,避免松手处的按钮再触发一次。 */
  private swallowClick = false;
  private morphing: Animation[] = [];
  private ambSrc = "";
  private hintLine = -1;
  private timers = { idle: 0, resume: 0, toast: 0, tip: 0, swallow: 0 };
  private scrollState = { last: 0, acc: 0 };
  private scrollLocked = false;
  private tabPlaylists = new Map<string, Playlist>();

  constructor() {
    super();
    this.api = null;
    this.playlists = "";
    this.theme = null;
    this.collapse = null;
    this.position = null;
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
  }

  override disconnectedCallback(): void {
    super.disconnectedCallback();
    for (const fn of this.cleanup) fn();
    this.cleanup = [];
    for (const t of Object.values(this.timers)) clearTimeout(t);
    this.unlockScroll();
  }

  /** open 展开播放器;view 可指定打开封面还是歌词。收起到一半时再点,从当前位置展开回来。 */
  async open(view?: View): Promise<void> {
    if (view) this.setView(view);
    if (this.ui.mode === "open" || !this.player.state.current) return;
    const from = this.ui.stageShown ? this.currentP() : 0;
    await this.showStage();
    const geo = this.measureMorph();
    if (geo && this.runMorph(geo, from, 1, morphOpen.duration * (1 - from), morphOpen.ease) && from === 0)
      this.holdFirstFrame();
    this.resetIdle();
    this.q(".stage .close").focus({ preventScroll: true });
  }

  /** close 收起回播放条(沉浸模式也一样)。展开到一半时再点,从当前位置收回去。 */
  close(): void {
    if (this.ui.mode !== "open") return;
    this.closeSheet();
    const p = this.morphing.length ? this.currentP() : 1;
    this.collapseFrom(this.measureMorph(), p, morphClose.duration * p, morphClose.ease);
  }

  /** showStage 把面板放进页面(还没有动画),每次展开或开始上拉时调用。 */
  private async showStage(): Promise<void> {
    this.stageToken++;
    this.cancelMorph();
    this.patch({ mode: "open", stageShown: true });
    this.lockScroll();
    await this.updateComplete;
    this.measureCover();
    this.paintAmbient();
    requestAnimationFrame(() => this.layoutLines());
  }

  /** hideStage 收起动画结束后把面板移出页面。 */
  private hideStage(): void {
    this.cancelMorph();
    clearMorph(this.morphParts);
    this.patch({ immersive: false, stageShown: false });
    this.unlockScroll();
  }

  /** collapseFrom 从进度 p 收起到播放条:播放条先在面板下面出现,面板的各部分飞回播放条后淡出。 */
  private collapseFrom(geo: MorphGeo | null, p: number, duration: number, ease: (t: number) => number): void {
    const token = ++this.stageToken;
    this.patch({ mode: "bar", idle: false });
    const finish = () => {
      if (token === this.stageToken) this.hideStage();
    };
    const anim = geo ? this.runMorph(geo, p, 0, Math.max(120, duration), ease) : null;
    if (!anim) {
      finish();
      return;
    }
    anim.finished.then(finish, finish);
    // 环境不派发 finished 时也要收起。
    setTimeout(finish, Math.max(120, duration) + 80);
  }

  /** measureMorph 去掉形变留下的样式后,量出面板到位时各部分的位置。 */
  private measureMorph(): MorphGeo | null {
    this.cancelMorph();
    clearMorph(this.morphParts);
    const immersive = this.ui.immersive;
    const lyricsView = this.ui.view === "lyrics" && !immersive;
    const badge = this.q(".badge");
    const shadow = this.q(".stage-shadow");
    const late: [number, number] = [0.62, 0.33];
    const parts: MorphParts = {
      stage: this.q(".stage"),
      shadow: getComputedStyle(shadow).display === "none" ? null : shadow,
      cover: this.q(".cover"),
      badge: badge.hasAttribute("data-show") && !lyricsView ? badge : null,
      miniText: this.q<HTMLElement>(".mini .bar-text"),
      miniCtl: this.q<HTMLElement>(".mini .bar-ctl"),
      top: this.q(".top"),
      below: lyricsView
        ? []
        : [
            { el: this.q(".meta"), fade: [0.55, 0.35] },
            { el: this.q(".ctl"), fade: late },
          ],
      beside: lyricsView ? { el: this.q(".head"), fade: [0.3, 0.4] } : null,
      rest: lyricsView
        ? [
            { el: this.q(".lyrics"), fade: [0.55, 0.35] },
            { el: this.q(".lyr-trans"), fade: late },
            { el: this.q(".ctl"), fade: late },
          ]
        : immersive
          ? [{ el: this.q(".lyrics"), fade: [0.55, 0.35] }]
          : [],
    };
    this.morphParts = parts;
    // 唱片转到哪了:封面从唱片长出来时从这个角度转正。
    const spin = this.q(".bar .disc-img").getAnimations?.()[0];
    const turned = ((Number(spin?.currentTime ?? 0) % 9000) / 9000) * 360;
    return measure(parts, {
      bar: this.q(".bar"),
      disc: this.q(".bar .disc"),
      barText: this.q(".bar .bar-text"),
      barCtl: this.q(".bar .bar-ctl"),
      discAngle: turned > 180 ? turned - 360 : turned,
      midRadius: this.ui.mobile ? 18 : 28,
      anchor: immersive ? "cover" : "card",
    });
  }

  /** runMorph 从进度 from 动到 to。返回卡片的动画,不能做动画时返回 null。 */
  private runMorph(
    geo: MorphGeo,
    from: number,
    to: number,
    duration: number,
    ease: (t: number) => number,
  ): Animation | null {
    this.cancelMorph();
    clearMorph(geo.parts);
    if (!canAnimate(geo.parts.stage)) return null;
    this.morphing = playMorph(geo, from, to, Math.max(1, duration), ease);
    this.morphRun = { from, to, duration: Math.max(1, duration), ease };
    return this.morphing[0] ?? null;
  }

  /** currentP 正在播放的形变走到哪了(0 播放条,1 面板)。 */
  private currentP(): number {
    const run = this.morphRun;
    const anim = this.morphing[0];
    if (!run || !anim) return this.ui.mode === "open" ? 1 : 0;
    const t = clamp(Number(anim.currentTime ?? 0) / run.duration, 0, 1);
    return run.from + (run.to - run.from) * run.ease(t);
  }

  /**
   * holdFirstFrame:面板第一次显示的那一帧要排版、绘制整块内容,可能很慢;先停在起点,
   * 等这一帧画完再开始,否则动画时间在这一帧里就过去了大半,看起来是直接跳到展开。
   */
  private holdFirstFrame(): void {
    const anims = this.morphing;
    for (const a of anims) a.pause();
    requestAnimationFrame(() =>
      requestAnimationFrame(() => {
        for (const a of anims) if (a.playState === "paused") a.play();
      }),
    );
  }

  private cancelMorph(): void {
    for (const a of this.morphing) a.cancel();
    this.morphing = [];
    this.morphRun = null;
  }

  /** measureCover 算出歌词页小封面与大封面的比例,封面在两页之间只用 transform 缩放。 */
  private measureCover(): void {
    const cover = this.renderRoot.querySelector<HTMLElement>(".cover");
    const stage = this.renderRoot.querySelector<HTMLElement>(".stage");
    if (!cover || !stage || !cover.offsetWidth) return;
    const small = parseFloat(getComputedStyle(stage).getPropertyValue("--ss")) || 48;
    const k = String(Math.round((small / cover.offsetWidth) * 10000) / 10000);
    if (cover.style.getPropertyValue("--kite-ck") !== k) {
      cover.style.setProperty("--kite-ck", k);
      this.renderRoot.querySelector<HTMLElement>(".badge")?.style.setProperty("--kite-ck", k);
    }
  }

  // ---- 上拉展开 / 下拉收起(手机,跟手) ----

  /** 播放条上按下:记下位置;往上拉超过判定距离才开始跟手展开,横向或往下就不管。 */
  private onBarDown = (e: PointerEvent): void => {
    if (
      e.pointerType === "mouse" ||
      !this.ui.mobile ||
      this.ui.mode === "open" ||
      !this.player.state.current ||
      this.pull
    )
      return;
    this.barPress = { pointer: e.pointerId, x: e.clientX, y: e.clientY };
    const move = (ev: PointerEvent) => {
      if (ev.pointerId !== this.barPress.pointer) return;
      const dx = ev.clientX - this.barPress.x;
      const dy = ev.clientY - this.barPress.y;
      if (this.pull) {
        this.movePull(ev);
        return;
      }
      if (Math.hypot(dx, dy) < dragSlop) return;
      if (dy < 0 && Math.abs(dy) > Math.abs(dx)) {
        this.swallow();
        void this.startPull(ev, true);
      } else stop();
    };
    const up = (ev: PointerEvent) => {
      if (ev.pointerId !== this.barPress.pointer) return;
      stop();
      if (this.pull) this.endPull(ev.type === "pointercancel");
    };
    const stop = () => {
      this.barPress.pointer = -1;
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      window.removeEventListener("pointercancel", up);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
    window.addEventListener("pointercancel", up);
  };

  /** startPull 开始跟手:展开时先把面板放进页面(从播放条大小开始),收起时面板已经在。 */
  private async startPull(e: PointerEvent, opening: boolean): Promise<void> {
    this.pull = {
      pointer: e.pointerId,
      opening,
      startY: e.clientY,
      lastY: e.clientY,
      lastT: e.timeStamp,
      vy: 0,
      p: opening ? 0 : 1,
      geo: null,
    };
    if (opening) await this.showStage();
    if (!this.pull) return;
    this.pull.geo = this.measureMorph();
    this.q(".stage").classList.add("morph");
    this.updatePull();
  }

  private movePull(e: PointerEvent): void {
    const pl = this.pull;
    if (!pl || e.pointerId !== pl.pointer) return;
    const dt = e.timeStamp - pl.lastT;
    if (dt > 0) pl.vy = 0.7 * ((e.clientY - pl.lastY) / dt) + 0.3 * pl.vy;
    pl.lastY = e.clientY;
    pl.lastT = e.timeStamp;
    this.updatePull();
  }

  /** updatePull 卡片顶边跟着手指:进度 = 顶边移动的距离 ÷ 播放条到面板顶边的距离。 */
  private updatePull(): void {
    const pl = this.pull;
    if (!pl?.geo) return;
    const travel = pl.geo.bar.top - pl.geo.box.top || 1;
    const moved = (pl.lastY - pl.startY) / travel;
    pl.p = clamp(pl.opening ? -moved : 1 - moved, 0, 1);
    applyMorph(morphFrame(pl.geo, pl.p));
  }

  /** endPull 松手:拖过判定距离或甩得够快就完成,否则回到原来的样子;剩下的路程按比例缩短时长。 */
  private endPull(cancelled = false): void {
    const pl = this.pull;
    this.pull = null;
    if (!pl) return;
    this.q(".stage").classList.remove("morph");
    const fling = performance.now() - pl.lastT < 100 ? pl.vy : 0;
    const toOpen =
      !cancelled && (pl.opening ? pl.p > 1 / 3 || fling < -pullSpeed : !(pl.p < 0.75 || fling > pullSpeed));
    if (!pl.geo) {
      if (!toOpen) this.hideStage();
      return;
    }
    if (toOpen) {
      this.patch({ mode: "open" });
      if (!this.runMorph(pl.geo, pl.p, 1, Math.max(160, (pl.opening ? 460 : 300) * (1 - pl.p)), morphOpen.ease))
        clearMorph(pl.geo.parts);
      this.resetIdle();
    } else {
      this.collapseFrom(
        pl.geo,
        pl.p,
        Math.max(160, (pl.opening ? 300 : 380) * pl.p),
        pl.opening ? morphOpen.ease : morphClose.ease,
      );
    }
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
      data-pos=${this.position === "left" || this.position === "center" ? this.position : "right"}
      ?data-trans=${ui.trans}
      ?data-immersive=${ui.immersive}
      ?data-idle=${ui.idle}
      ?data-sheet=${ui.sheet}
      ?data-has-track=${!!t}
    >
      ${this.renderBar(s, t)}
      <div class="stage-shadow" aria-hidden="true"></div>
      ${this.renderStage(s, t)}
      <div class="backdrop" @click=${this.closeSheet}></div>
      ${this.renderSheet(s)}
      <div class=${classMap({ toast: true, on: ui.toastOn })} role="status" aria-live="polite">${ui.toastText}</div>
    </div>`;
  }

  private renderBar(s: PlayerState, t: Track | null): TemplateResult {
    const pct = s.duration > 0 ? Math.min(1, s.time / s.duration) : 0;
    const ticker = this.tickerText(s);
    const cover = t?.album.cover || undefined;
    return html`<div class="bar" part="bar" @pointerdown=${this.onBarDown} @click=${{ handleEvent: (e: Event) => this.onStageClickCapture(e), capture: true }}>
      <button class="disc expand" aria-label="展开播放器" ?data-empty=${!cover} @click=${this.onExpand}>
        <span class="ring" style=${styleMap({ "--pct": String(pct) })}></span
        ><span class="disc-img">${coverPh}<img alt="" draggable="false" src=${ifDefined(cover)} @error=${onCoverError} @load=${onCoverLoad} /></span>
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
    const tag = [t?.trial ? "仅试听" : "", t?.access === "vip" ? "VIP" : t?.access === "paid" ? "数字专辑" : ""]
      .filter(Boolean)
      .join(" · ");
    const badge = qualityBadge(t);
    const lyr = s.lyrics;
    const empty = !lyr || lyr.instrumental || lyr.lines.length === 0;
    const chorus = lyr?.chorus[0];
    const chorusStyle =
      chorus && s.duration > 0
        ? {
            left: `${(chorus.start / 1000 / s.duration) * 100}%`,
            width: `${((chorus.end - chorus.start) / 1000 / s.duration) * 100}%`,
          }
        : null;
    return html`<div
      class="stage"
      part="panel"
      role="dialog"
      aria-label="正在播放"
      ?hidden=${!ui.stageShown}
      @keydown=${this.onStageKey}
      @pointerdown=${this.onStageDown}
      @pointermove=${this.onStageMove}
      @pointerup=${this.onStageUp}
      @pointercancel=${this.onStageCancel}
      @click=${{ handleEvent: (e: Event) => this.onStageClickCapture(e), capture: true }}
      @wheel=${{ handleEvent: () => this.resetIdle(), passive: true }}
    >
      <div class="amb"><canvas width="32" height="48"></canvas><span class="veil"></span><span class="spot spot-a"></span><span class="spot spot-b"></span><span class="dim"></span><span class="grain"></span></div>
      <div class="scrim"></div>
      <div class="fg">
        <div class="pages">
          <section class="page page-cover">
            <div class="meta">
              <span class="meta-title ell">${t?.title ?? ""}</span><span class="meta-sub"
                ><span class="ell meta-artist">${t ? [who, t.album.name].filter(Boolean).join(" · ") : ""}</span><span class="tag access-tag" ?hidden=${!tag}>${tag}</span></span
              >
            </div>
          </section>
          <section class="page page-lyrics">
            <button class="head to-cover" aria-label="返回封面" @click=${this.toCover}><span class="head-title ell">${t?.title ?? ""}</span><span class="head-sub ell">${who}</span></button>
            <div class="lyrics" part="lyrics" @wheel=${{ handleEvent: (e: WheelEvent) => this.onLyricsWheel(e), passive: false }}>
              <div class="lines" role="list" @click=${this.onLineClick}>${guard([lyr], () => renderLines(lyr))}</div>
              <div class="lyr-empty" ?hidden=${!empty}>${eq}<span class="lyr-empty-text">${empty ? this.emptyText(s) : ""}</span></div>
              <button class="seek" aria-label="跳到这一句" @click=${this.seekToHint}>${icons.play}<span></span></button>
            </div>
            <button class="ib trans-btn lyr-trans" aria-label="显示翻译" aria-pressed=${String(ui.trans)} @click=${this.toggleTrans}>${icons.lyric}</button>
          </section>
        </div>
        <button class="cover" aria-label=${ui.view === "cover" ? "查看歌词" : "返回封面"} ?data-badge=${!!badge} @click=${this.onCoverClick}>
          ${coverPh}<img alt="" draggable="false" src=${ifDefined(cover)} @error=${onCoverError} @load=${onCoverLoad} /><span class="cover-shade"></span><span class="cover-rim"></span>
        </button>
        ${this.renderBadge(s, badge)}
        <div class="mini" aria-hidden="true">
          <span class="bar-text"
            ><span class="bar-line"><span class="bar-title ell">${t?.title ?? ""}</span>${t?.trial ? html`<span class="rtag trial">仅试听</span>` : nothing}</span
            ><span class="bar-sub"><span class="ell">${this.tickerText(s)}</span></span></span
          ><span class="bar-ctl"
            ><span class="ib desk">${icons.prev}</span><span class="blob">${blob}</span><span class="ib">${icons.next}</span><span class="bar-sep desk"></span
            ><span class="ib">${icons.queue}</span></span
          >
        </div>
        <div class="top">
          <button class="ib close" aria-label="收起" @click=${this.close}>${icons.chevron}</button>
          <div class="dots"><button data-go="cover" aria-label="封面" @click=${this.toCover}></button><button data-go="lyrics" aria-label="歌词" @click=${this.toLyrics}></button></div>
          <button class="ib immersive" aria-label=${ui.immersive ? "退出沉浸模式" : "沉浸模式"} @click=${this.onImmersiveClick}>${ui.immersive ? icons.shrink : icons.expand}</button>
        </div>
        <div class="ctl" part="controls">
          <div class="progress">
            <span class="ptrack"></span><span class="pchorus" ?hidden=${!chorusStyle} style=${styleMap(chorusStyle ?? {})}></span><span class="pfill"></span><span class="pknob"><span class="pthumb"></span></span
            ><input type="range" min="0" max="1000" step="1" value="0" aria-label="播放进度" @input=${this.onRangeInput} @change=${this.onRangeChange} />
          </div>
          <div class="times"><span class="t-cur">${formatTime(ui.dragging ? ui.dragPct * s.duration : s.time)}</span><span class="t-dur">${formatTime(s.duration)}</span></div>
          <div class="buttons">
            <button class="ib mode" aria-label=${`播放模式:${modeLabels[s.mode]}`} @click=${this.onModeClick}>
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
      </div>
    </div>`;
  }

  private renderSheet(s: PlayerState): TemplateResult {
    const ui = this.ui;
    const ids = ui.sheetReady ? this.tabIds() : [];
    const at = Math.max(ids.indexOf(ui.tab), 0);
    return html`<div class="sheet" part="queue" role="dialog" aria-label="播放列表" @keydown=${this.onSheetKey}>
      <div class="sheet-head">
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
    return repeat(
      ui.sheetTracks,
      (t, i) => `${i}:${t.id}`,
      (t, i) => {
        const isCur = t.id === cur;
        const tag = [
          t.trial ? html`<span class="rtag trial">仅试听</span>` : nothing,
          t.access === "vip"
            ? html`<span class="rtag vip">VIP</span>`
            : t.access === "paid"
              ? html`<span class="rtag paid">数字专辑</span>`
              : nothing,
        ];
        const sub = [artists(t), t.album.name].filter(Boolean).join(" · ");
        return html`<button class=${classMap({ row: true, locked: !canPlay(t), cur: isCur })} role="listitem" data-i=${i} @click=${() => this.pickRow(i)}>
          <span class="row-main"
            ><span class="row-title"><span class="ell">${t.title}</span>${isCur ? eq : nothing}</span><span class="row-sub">${tag}<span class="ell">${sub}</span></span></span
          >${canPlay(t) ? html`<span class="row-dur">${formatTime(t.durationMs / 1000)}</span>` : html`<span class="lock">${icons.lock}</span>`}
        </button>`;
      },
    );
  }

  /**
   * renderBadge 封面左下角的音质标签。数据来源能按较低音质给地址时可以点按切换高音质(参考网易云),
   * 否则只做展示。开关的动画由 CSS 按 data-on 过渡:金色底淡入,规格从音质名后面滑出;
   * 关闭时规格里的数字先变成「-」再收回去。
   */
  private renderBadge(s: PlayerState, badge: [string, string] | null): TemplateResult {
    if (badge) this.ui.lastBadge = badge;
    const [label, detail] = badge ?? this.ui.lastBadge;
    const can = this.player.canSwitchQuality();
    const on = s.highQuality || !can;
    return html`<button
      class="badge"
      ?data-on=${on}
      ?data-show=${!!badge}
      ?disabled=${!can || !badge}
      aria-hidden=${badge ? "false" : "true"}
      aria-pressed=${can ? String(on) : nothing}
      aria-label=${can ? `${label}:${on ? "已开启" : "已关闭"},点按切换` : `${label} ${detail}`}
      @click=${this.onBadgeClick}
    >
      <span class="bq-label"><span class="bq-gold"></span><span class="bq-text">${label}</span><span class="bq-sheen"></span></span
      ><span class="bq-detail">${on ? detail : detail.replace(/\d/g, "-")}</span><span class="bq-streak"></span>
    </button>`;
  }

  /** 点音质标签:开关高音质,提示切换结果(参考网易云「已为您关闭超高清音频」)。 */
  private onBadgeClick = (): void => {
    const badge = qualityBadge(this.player.state.current);
    if (!badge || !this.player.canSwitchQuality()) return;
    const on = !this.player.state.highQuality;
    this.player.setHighQuality(on);
    this.toast(on ? `已开启${badge[0]}` : `已关闭${badge[0]},改用标准音质`);
    if (on) void this.updateComplete.then(() => this.lightUp());
  };

  /** lightUp 开启高音质时:封面描边亮一下,音质名上扫过高光,标签底边跑过一道光。 */
  private lightUp(): void {
    const rim = this.renderRoot.querySelector<HTMLElement>(".cover-rim");
    const streak = this.renderRoot.querySelector<HTMLElement>(".bq-streak");
    const badge = this.renderRoot.querySelector<HTMLElement>(".badge");
    if (rim && canAnimate(rim)) {
      rim.animate([{ opacity: 0 }, { opacity: 1, offset: 0.16 }, { opacity: 1, offset: 0.36 }, { opacity: 0 }], {
        duration: 680,
        easing: "ease-out",
      });
    }
    this.sheen();
    if (streak && badge && canAnimate(streak)) {
      streak.animate(
        [
          { transform: "translateX(-100%)", opacity: 0 },
          { opacity: 1, offset: 0.2 },
          { opacity: 1, offset: 0.75 },
          { transform: `translateX(${badge.offsetWidth}px)`, opacity: 0 },
        ],
        { duration: 460, delay: 200, easing: "cubic-bezier(.4,0,.2,1)" },
      );
    }
  }

  /** sheen 音质标签开启时金色上扫过一道高光。 */
  private sheen(): void {
    const el = this.renderRoot.querySelector<HTMLElement>(".bq-sheen");
    if (!el || !canAnimate(el)) return;
    el.animate([{ transform: "translateX(-120%) skewX(-20deg)" }, { transform: "translateX(260%) skewX(-20deg)" }], {
      duration: 560,
      delay: 160,
      easing: "cubic-bezier(.4,0,.2,1)",
    });
  }

  /** revealBadge 换歌后标签出现或换了规格:规格从音质名后面重新滑出,再扫一道高光。 */
  private revealBadge(): void {
    const detail = this.renderRoot.querySelector<HTMLElement>(".badge[data-on] .bq-detail");
    if (!detail || !canAnimate(detail)) return;
    detail.animate(
      [
        { transform: "translateX(-100%)", opacity: 0 },
        { transform: "translateX(-100%)", opacity: 1, offset: 0.15 },
        { transform: "none", opacity: 1 },
      ],
      { duration: 340, delay: 120, easing: easeOut, fill: "backwards" },
    );
    this.sheen();
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
      // 换歌时封面交叉淡化(参考网易云:标题立即换,封面约 300ms 淡入);音质标签换了就重新展开。
      const old = prev?.current;
      if (old) {
        const oldCover = old.album.cover || "";
        const newBadge = qualityBadge(s.current);
        if (oldCover !== (s.current?.album.cover || "")) {
          this.crossfade(".disc-img", oldCover);
          if (this.ui.stageShown) this.crossfade(".cover", oldCover);
        }
        if (this.ui.stageShown && newBadge && String(newBadge) !== String(qualityBadge(old))) this.revealBadge();
      }
      if (this.ui.stageShown) this.paintAmbient(true);
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
      this.positionLines("wave");
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
    else this.close();
  };

  private onSheetKey = (e: KeyboardEvent): void => {
    if (e.key === "Escape") this.closeSheet();
  };

  /**
   * 面板上的手势只处理触摸与触控笔。鼠标(桌面)用圆点、点封面切页,用滚轮滚歌词,
   * 鼠标一动播放控制就回来。
   */
  private onStageDown = (e: PointerEvent): void => {
    this.swallowClick = false;
    const target = e.target as HTMLElement;
    if (e.pointerType === "mouse" || target.closest(".ctl")) this.resetIdle();
    if (
      e.pointerType === "mouse" ||
      this.gesture.pointer !== -1 ||
      this.pull ||
      target.closest(".ctl, .seek, .lyr-trans")
    )
      return;
    const onLyrics = !!target.closest(".lyrics") && (this.ui.view === "lyrics" || this.ui.immersive);
    // 正在惯性滑行的歌词被按住就停下。
    if (onLyrics) cancelAnimationFrame(this.momentum);
    this.gesture = {
      pointer: e.pointerId,
      x: e.clientX,
      y: e.clientY,
      axis: "",
      onLyrics,
      from: this.ui.view === "lyrics" ? 1 : 0,
      width: this.q(".stage").clientWidth || 1,
      pg: 0,
      vx: 0,
      vy: 0,
      lastX: e.clientX,
      lastY: e.clientY,
      lastT: e.timeStamp,
    };
  };

  private onStageMove = (e: PointerEvent): void => {
    if (e.pointerType === "mouse") {
      this.resetIdle();
      return;
    }
    if (this.pull) {
      this.movePull(e);
      return;
    }
    const g = this.gesture;
    if (e.pointerId !== g.pointer) return;
    const dx = e.clientX - g.x;
    const dy = e.clientY - g.y;
    if (!g.axis) {
      if (Math.hypot(dx, dy) < dragSlop) return;
      g.axis = Math.abs(dx) > Math.abs(dy) ? "x" : "y";
      const stage = this.q(".stage");
      // 往下拉收起(手机):封面页任何位置,歌词页只在顶栏和歌名上(歌词区的纵向拖动是滚歌词)。
      const onTop = !!(e.target as HTMLElement).closest(".top, .head");
      if (
        g.axis === "y" &&
        dy > 0 &&
        this.ui.mobile &&
        !this.ui.immersive &&
        (!g.onLyrics || onTop) &&
        (this.ui.view === "cover" || onTop)
      ) {
        g.pointer = -1;
        this.capture(stage, e.pointerId);
        this.swallow();
        void this.startPull(e, false);
        return;
      }
      // 沉浸模式没有分页;纵向只在歌词上有意义。
      if ((g.axis === "x" && this.ui.immersive) || (g.axis === "y" && !(g.onLyrics && this.lineEls.length))) {
        g.pointer = -1;
        return;
      }
      this.capture(stage, e.pointerId);
      if (g.axis === "x") {
        stage.classList.add("paging");
      } else {
        this.beginManual();
        this.manual.startOffset = this.manual.offset;
        this.hideControls();
      }
    }
    const dt = e.timeStamp - g.lastT;
    if (dt > 0) {
      g.vx = 0.7 * ((e.clientX - g.lastX) / dt) + 0.3 * g.vx;
      g.vy = 0.7 * ((e.clientY - g.lastY) / dt) + 0.3 * g.vy;
    }
    g.lastX = e.clientX;
    g.lastY = e.clientY;
    g.lastT = e.timeStamp;
    if (g.axis === "x") {
      // 封面与歌词之间的进度跟着手指走(0 封面,1 歌词),CSS 用 --kite-pg 插值页面、封面与控制区。
      g.pg = clamp(g.from - dx / g.width, 0, 1);
      for (const el of this.pagingParts()) el.style.setProperty("--kite-pg", String(g.pg));
    } else {
      // 首句到末句都能停在锚点;再往外拖有阻力,最多拖出去 80px。
      this.manual.offset = this.rubber(this.manual.startOffset + dy);
      this.positionLines("instant");
    }
  };

  private onStageUp = (e: PointerEvent): void => {
    if (e.pointerType === "mouse") return;
    if (this.pull && e.pointerId === this.pull.pointer) {
      this.endPull();
      return;
    }
    const g = this.gesture;
    if (e.pointerId !== g.pointer) return;
    g.pointer = -1;
    // 松手前停顿过就不算甩动。
    const recent = e.timeStamp - g.lastT < 100;
    if (g.axis === "x") {
      // 甩得够快就按方向翻页,否则看过没过一半。
      const fling = recent ? g.vx : 0;
      this.endPaging(fling < -flingSpeed ? "lyrics" : fling > flingSpeed ? "cover" : g.pg > 0.5 ? "lyrics" : "cover");
      this.swallow();
    } else if (g.axis === "y") {
      this.fling(recent ? g.vy : 0);
      this.swallow();
    } else if (g.onLyrics && this.ui.view === "lyrics" && !this.ui.immersive) {
      // 手机上点歌词切换播放控制的显示(参考网易云);滚动中点焦点句(▶ 时间旁那句)就跳过去。
      const line = (e.target as HTMLElement).closest<HTMLElement>(".line");
      if (this.manual.active && line && Number(line.dataset.i) === this.hintLine) this.seekToHint();
      else if (this.ui.idle) this.resetIdle();
      else this.hideControls();
      this.swallow();
    }
  };

  private onStageCancel = (e: PointerEvent): void => {
    if (this.pull && e.pointerId === this.pull.pointer) {
      this.endPull(true);
      return;
    }
    const g = this.gesture;
    if (e.pointerId !== g.pointer) return;
    g.pointer = -1;
    if (g.axis === "x") this.endPaging(this.ui.view);
    else if (g.axis === "y") this.fling(0);
  };

  private capture(el: HTMLElement, pointer: number): void {
    try {
      el.setPointerCapture(pointer);
    } catch {
      // 指针已经结束时拿不到捕获,照常处理。
    }
  }

  /** 手势已经处理过这次按下:吞掉随后的 click(触摸的 click 可能晚于 pointerup 一个任务才到)。 */
  private swallow(): void {
    this.swallowClick = true;
    clearTimeout(this.timers.swallow);
    this.timers.swallow = window.setTimeout(() => (this.swallowClick = false), 400);
  }

  private onStageClickCapture(e: Event): void {
    if (!this.swallowClick) return;
    this.swallowClick = false;
    e.stopPropagation();
    e.preventDefault();
  }

  /** pagingParts 随封面页 ↔ 歌词页进度变化的元素(--kite-pg 只写在它们上面)。 */
  private pagingParts(): HTMLElement[] {
    return [...this.renderRoot.querySelectorAll<HTMLElement>(".page, .cover, .badge, .ctl, .extra")];
  }

  /** endPaging 松手:去掉跟手的 --kite-pg,交给 CSS 过渡滑到目标页。 */
  private endPaging(view: View): void {
    this.q(".stage").classList.remove("paging");
    for (const el of this.pagingParts()) el.style.removeProperty("--kite-pg");
    this.setView(view);
  }

  /** 滚轮滚歌词:每一格 120ms 平滑过渡,首句和末句停在锚点(滚轮没有回弹,直接停住)。 */
  private onLyricsWheel(e: WheelEvent): void {
    if (!this.lineEls.length) return;
    e.preventDefault();
    cancelAnimationFrame(this.momentum);
    this.beginManual();
    const [lo, hi] = this.manualBounds();
    this.manual.offset = clamp(this.manual.offset - e.deltaY, lo, hi);
    this.positionLines("follow", 120);
    this.scheduleResume();
  }

  /** 点歌词跳到那一句:鼠标点击与键盘。触摸的点按在 onStageUp 里切换播放控制,不会走到这里。 */
  private onLineClick = (e: Event): void => {
    const line = (e.target as HTMLElement).closest<HTMLElement>(".line");
    if (!line) return;
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
    listen(
      window,
      openQueueEvent,
      (e) => void this.openSheet((e as CustomEvent<{ playlist?: string }>).detail?.playlist),
    );
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

  /**
   * setImmersive 进入 / 退出沉浸模式。两种布局差别太大,不做逐个元素的位移:先淡出内容,
   * 这一帧直接换布局(去掉过渡),再让容器在面板与整屏之间过渡、内容淡入。
   * 进入时整屏的可见区域从面板的位置长开;退出时面板从整屏大小缩回(背景是模糊色块,拉伸看不出来)。
   */
  private async setImmersive(on: boolean, animate = false): Promise<void> {
    if (this.ui.immersive === on || (on && this.ui.mobile)) return;
    const stage = this.q(".stage");
    const fg = this.q(".fg");
    if (!animate || !this.ui.stageShown || !canAnimate(stage)) {
      this.patch({ immersive: on });
      this.resetIdle();
      await this.updateComplete;
      this.layoutLines();
      return;
    }
    this.cancelMorph();
    const radius = parseFloat(getComputedStyle(stage).borderTopLeftRadius) || 0;
    const before = stage.getBoundingClientRect();
    await fg
      .animate([{ opacity: 1 }, { opacity: 0 }], { duration: 120, easing: "ease-in", fill: "forwards" })
      .finished.catch(() => undefined);
    stage.classList.add("snap");
    this.patch({ immersive: on });
    this.resetIdle();
    await this.updateComplete;
    this.layoutLines();
    const after = stage.getBoundingClientRect();
    requestAnimationFrame(() => stage.classList.remove("snap"));
    for (const a of fg.getAnimations()) a.cancel();
    if (on) {
      stage.animate(
        [{ clipPath: insetFrom(before, after, radius) }, { clipPath: "inset(0px 0px 0px 0px round 0px)" }],
        { duration: 420, easing: easeOut },
      );
    } else {
      const endRadius = parseFloat(getComputedStyle(stage).borderTopLeftRadius) || 0;
      stage.animate(
        [
          {
            transformOrigin: "0 0",
            transform: `translate(${before.left - after.left}px, ${before.top - after.top}px) scale(${before.width / after.width}, ${before.height / after.height})`,
            borderRadius: "0px",
          },
          { transformOrigin: "0 0", transform: "none", borderRadius: `${endRadius}px` },
        ],
        { duration: 360, easing: easeOut },
      );
    }
    fg.animate([{ opacity: 0 }, { opacity: 1 }], {
      duration: on ? 260 : 220,
      delay: on ? 120 : 160,
      easing: "ease-out",
      fill: "backwards",
    });
  }

  private cycleMode(showTip: boolean): void {
    const mode = this.player.cycleMode();
    if (!showTip) return;
    this.patch({ tipText: modeLabels[mode], tipOn: true });
    clearTimeout(this.timers.tip);
    this.timers.tip = window.setTimeout(() => this.patch({ tipOn: false }), 1400);
  }

  /**
   * toggleTrans 显示 / 隐藏翻译(参考网易云):隐藏时译文先淡出,各行再收拢;显示时各行先让位,译文随后淡入。
   * 行的移动以当前句为中心向两侧逐行错开,当前句停在锚点不动。
   */
  private toggleTrans = async (): Promise<void> => {
    const trans = !this.ui.trans;
    writeFlag("kite-player:trans", trans);
    const animate = this.ui.mode === "open" && this.lineEls.length > 0 && canAnimate(this.q(".lyrics"));
    const trs = animate ? [...this.renderRoot.querySelectorAll<HTMLElement>(".lines .tr")] : [];
    if (!trans && trs.length) {
      await Promise.all(
        trs.map(
          (tr) =>
            tr.animate([{ opacity: 1 }, { opacity: 0 }], { duration: 160, easing: "ease-in", fill: "forwards" })
              .finished,
        ),
      ).catch(() => undefined);
    }
    this.patch({ trans });
    await this.updateComplete;
    for (const tr of trs) for (const a of tr.getAnimations()) a.cancel();
    this.layoutLines(animate ? "center" : "instant");
    if (trans) {
      for (const tr of trs) {
        tr.animate(
          [
            { opacity: 0, translate: "0 -4px" },
            { opacity: 1, translate: "0 0" },
          ],
          { duration: 240, delay: 120, easing: "ease-out", fill: "backwards" },
        );
      }
    }
  };

  /**
   * resetIdle 显示歌词页的播放控制,一段时间没操作后再隐藏。桌面上鼠标一动就调用;
   * 手机上只有点歌词、碰控制区才调用,拖动歌词时立即隐藏(hideControls)。
   */
  private resetIdle = (): void => {
    if (this.ui.idle) this.patch({ idle: false });
    clearTimeout(this.timers.idle);
    if (this.ui.mode !== "open" || this.ui.view !== "lyrics" || this.ui.immersive) return;
    this.timers.idle = window.setTimeout(
      () => {
        if (!this.player.state.playing || this.ui.view !== "lyrics" || this.ui.immersive || this.ui.sheet) return;
        this.patch({ idle: true });
      },
      this.ui.mobile ? idleDelay.mobile : idleDelay.desktop,
    );
  };

  private hideControls(): void {
    clearTimeout(this.timers.idle);
    if (this.ui.mode === "open" && this.ui.view === "lyrics" && !this.ui.immersive && !this.ui.idle)
      this.patch({ idle: true });
  }

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
  private layoutLines(motion: Motion = "instant"): void {
    if (!this.isConnected || !this.lineEls.length || this.ui.mode !== "open") return;
    let top = 0;
    this.tops = this.lineEls.map((el) => {
      const t = top;
      top += el.offsetHeight + 14;
      return t;
    });
    if (this.manual.active) {
      const [lo, hi] = this.manualBounds();
      this.manual.offset = clamp(this.manual.offset, lo, hi);
    }
    this.positionLines(motion);
  }

  /**
   * positionLines 把当前行(手动滚动时加上偏移)移到锚点。motion 决定怎么过去:
   * wave 是换句,锚点以下每行晚 35ms 出发,行距先拉开再合拢(参考网易云);center 是翻译开关,
   * 以当前句为中心向两侧每行错开 25ms;follow 是跟随(滚轮、回弹),没有错开;instant 是跟手。
   */
  private positionLines(motion: Motion = "wave", ms = 0): void {
    if (!this.isConnected || !this.lineEls.length || !this.tops.length) return;
    const { lyricIndex } = this.player.state;
    const cur = Math.max(lyricIndex, 0);
    // 锚点按面板高度算,不按歌词区:播放控制隐藏时歌词区变高,当前行不能跟着跳。
    const anchor = this.ui.immersive ? this.q(".lyrics").clientHeight * 0.36 : this.q(".stage").clientHeight * 0.16;
    const base = anchor - (this.tops[cur] ?? 0) + this.manual.offset;
    const forward = cur >= this.lastLine;
    this.lineEls.forEach((el, i) => {
      let duration = "";
      let delay = 0;
      if (motion === "instant") duration = "0ms";
      else if (motion === "follow") duration = `${ms}ms`;
      else if (motion === "center") {
        duration = "420ms";
        delay = Math.min(Math.abs(i - cur), 12) * 25;
      } else if (!this.manual.active) {
        const steps = forward ? i - cur + 1 : cur - i + 1;
        delay = Math.min(Math.max(steps, 0), 12) * lyricStagger;
      }
      el.style.transitionDuration = duration;
      el.style.transitionDelay = `${delay}ms`;
      el.style.transform = `translateY(${(this.tops[i] ?? 0) + base}px)`;
      el.classList.toggle("cur", i === lyricIndex);
      el.classList.toggle("far", Math.abs(i - cur) >= 3);
    });
    if (lyricIndex !== this.lastLine) {
      this.unitValues = [];
      this.unitEls[this.lastLine]?.forEach((u) => {
        u.style.removeProperty("--p");
      });
    }
    this.lastLine = cur;
    if (this.manual.active) this.updateHint(base, anchor);
  }

  /** manualBounds 手动滚动偏移的范围:首句最多到锚点(上限),末句最多到锚点(下限)。 */
  private manualBounds(): [number, number] {
    const cur = Math.max(this.player.state.lyricIndex, 0);
    const at = this.tops[cur] ?? 0;
    return [at - (this.tops[this.tops.length - 1] ?? 0), at];
  }

  /** rubber 越过首句 / 末句的部分只按 0.35 跟手,最多拖出去 80px。 */
  private rubber(offset: number): number {
    const [lo, hi] = this.manualBounds();
    if (offset > hi) return hi + Math.min(rubberMax, (offset - hi) * rubberRatio);
    if (offset < lo) return lo - Math.min(rubberMax, (lo - offset) * rubberRatio);
    return offset;
  }

  /**
   * fling 拖歌词松手:越界了就回弹;否则按松手速度惯性滑行(指数减速,最长 1.2s),碰到首句或末句停住回弹。
   * 停下后再等 2.5s 滚回当前句。
   */
  private fling(v: number): void {
    cancelAnimationFrame(this.momentum);
    const [lo, hi] = this.manualBounds();
    if (this.manual.offset > hi || this.manual.offset < lo) {
      this.manual.offset = clamp(this.manual.offset, lo, hi);
      this.positionLines("follow", 360);
      this.scheduleResume();
      return;
    }
    if (Math.abs(v) < 0.05 || !canAnimate(this.q(".lyrics"))) {
      this.scheduleResume();
      return;
    }
    const start = performance.now();
    let last = start;
    let vel = v;
    const step = (now: number) => {
      const dt = Math.min(32, now - last);
      last = now;
      vel *= Math.exp(-dt / flingTau);
      const [min, max] = this.manualBounds();
      const next = this.manual.offset + vel * dt;
      if (next > max || next < min) {
        this.manual.offset = clamp(next, min, max);
        this.positionLines("follow", 360);
        this.scheduleResume();
        return;
      }
      this.manual.offset = next;
      this.positionLines("instant");
      if (Math.abs(vel) < 0.02 || now - start > flingMax) {
        this.scheduleResume();
        return;
      }
      this.momentum = requestAnimationFrame(step);
    };
    this.momentum = requestAnimationFrame(step);
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

  /** endManual 结束手动滚动:去掉焦点句,animate 时与换句一样逐行滚回当前句。 */
  private endManual(animate: boolean): void {
    clearTimeout(this.timers.resume);
    cancelAnimationFrame(this.momentum);
    if (!this.manual.active && this.manual.offset === 0) return;
    this.manual.active = false;
    this.manual.offset = 0;
    this.lineEls[this.hintLine]?.classList.remove("focus");
    this.hintLine = -1;
    this.renderRoot.querySelector(".lyrics")?.classList.remove("manual");
    if (animate) this.positionLines("wave");
  }

  /**
   * updateHint 滚动中锚点上的那句是焦点句:用当前句的样式(参考网易云,没有背景色块),
   * 右侧的 ▶ 时间与它垂直居中,点它或点焦点句就跳过去。
   */
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
    if (best !== this.hintLine) {
      this.lineEls[this.hintLine]?.classList.remove("focus");
      this.lineEls[best]?.classList.add("focus");
      this.hintLine = best;
    }
    const line = this.player.state.lyrics?.lines[best];
    const hint = this.q(".seek");
    hint.style.top = `${anchor + (this.lineEls[best]?.offsetHeight ?? 34) / 2 - 11}px`;
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
      this.patch({
        tab,
        sheetReady: true,
        sheetStatus: "ready",
        sheetTracks: s.queue,
        sheetName: s.playlist?.name ?? "",
      });
      return;
    }
    this.patch({ tab, sheetReady: true, sheetStatus: "loading", sheetName: "" });
    try {
      const pl = await this.player.loadPlaylist(tab);
      this.tabPlaylists.set(tab, pl);
      if (this.ui.tab !== tab) return;
      this.patch({ sheetStatus: "ready", sheetTracks: pl.tracks, sheetName: pl.name });
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

  /**
   * crossfade 换歌时把上一首的封面(没有封面时是占位)盖在新的上面淡出。
   * 等新图加载好再淡,否则网络慢时中间会露出占位又跳回。临时元素不进模板。
   */
  private crossfade(sel: string, oldSrc: string): void {
    const box = this.renderRoot.querySelector<HTMLElement>(sel);
    const img = box?.querySelector<HTMLImageElement>(":scope > img");
    if (!box || !img || !canAnimate(box)) return;
    box.querySelectorAll(":scope > .xfade").forEach((el) => {
      el.remove();
    });
    const ghost = document.createElement("span");
    ghost.className = "xfade";
    if (oldSrc && !img.hasAttribute("data-err")) {
      const old = document.createElement("img");
      old.alt = "";
      old.draggable = false;
      old.src = oldSrc;
      ghost.append(old);
    } else {
      const ph = box.querySelector(":scope > .ph");
      if (ph) ghost.append(ph.cloneNode(true));
    }
    box.append(ghost);
    let started = false;
    const start = () => {
      if (started) return;
      started = true;
      ghost
        .animate([{ opacity: 1 }, { opacity: 0 }], { duration: 320, easing: "ease-out", fill: "forwards" })
        .finished.then(
          () => ghost.remove(),
          () => ghost.remove(),
        );
    };
    if (!img.getAttribute("src") || img.complete) start();
    else {
      img.addEventListener("load", start, { once: true });
      img.addEventListener("error", start, { once: true });
      setTimeout(start, 1500);
    }
  }

  /**
   * paintAmbient 用当前封面画面板的光感背景:整张封面缩到 40×40,取中间一块缩到 8×12,再放大到 32×48 的画布,
   * 画布由 CSS 拉满面板。两次双线性缩放就是一层很重的模糊,不需要每帧做滤镜,展开动画不掉帧。
   * 封面允许跨域读取(网易云图床返回 Access-Control-Allow-Origin: *)时,再取右下角的颜色作为光斑,
   * 并按平均亮度算出还要压暗多少,让背景落到主题的目标亮度(--kite-amb-target),白字和小字都看得清;
   * 不允许读取时按 --kite-amb-dim 的默认值压暗。没有封面或加载失败时显示中性底色。
   * fade 时旧背景淡出(面板开着时换歌)。
   */
  private paintAmbient(fade = false): void {
    const src = this.player.state.current?.album.cover ?? "";
    if (src === this.ambSrc) return;
    this.ambSrc = src;
    const amb = this.renderRoot.querySelector<HTMLElement>(".amb");
    const canvas = amb?.querySelector<HTMLCanvasElement>(":scope > canvas:not(.xfade)");
    const ctx = canvas?.getContext("2d");
    if (!amb || !canvas || !ctx) return;
    const draw = (img: HTMLImageElement | null, readable: boolean) => {
      if (this.ambSrc !== src) return;
      if (fade && canAnimate(canvas)) {
        const old = canvas.cloneNode() as HTMLCanvasElement;
        old.classList.add("xfade");
        old.getContext("2d")?.drawImage(canvas, 0, 0);
        canvas.after(old);
        old
          .animate([{ opacity: 1 }, { opacity: 0 }], { duration: 600, easing: "ease-out", fill: "forwards" })
          .finished.then(
            () => old.remove(),
            () => old.remove(),
          );
      }
      ctx.clearRect(0, 0, canvas.width, canvas.height);
      amb.toggleAttribute("data-empty", !img);
      amb.style.removeProperty("--amb-dim");
      amb.style.removeProperty("--amb-tint");
      if (!img) return;
      const mid = document.createElement("canvas");
      mid.width = mid.height = 40;
      const tiny = document.createElement("canvas");
      tiny.width = 8;
      tiny.height = 12;
      const mctx = mid.getContext("2d", { willReadFrequently: true });
      const tctx = tiny.getContext("2d");
      if (!mctx || !tctx) return;
      mctx.imageSmoothingQuality = tctx.imageSmoothingQuality = ctx.imageSmoothingQuality = "high";
      mctx.drawImage(img, 0, 0, 40, 40);
      tctx.drawImage(mid, 8, 4, 24, 32, 0, 0, tiny.width, tiny.height);
      ctx.drawImage(tiny, 0, 0, canvas.width, canvas.height);
      if (!readable) return;
      try {
        const css = getComputedStyle(amb);
        const veil = parseVeil(css.getPropertyValue("--kite-amb-veil"));
        const target = parseFloat(css.getPropertyValue("--kite-amb-target")) || 0.12;
        const { luminance, tint } = palette(mctx.getImageData(0, 0, 40, 40).data, veil);
        // 叠黑色 a 后各通道乘以 (1 - a),亮度约乘以 (1 - a)^2.2。
        const dim = luminance > target ? 1 - (target / luminance) ** (1 / 2.2) : 0;
        amb.style.setProperty("--amb-dim", String(Math.round(Math.min(dim, 0.7) * 100) / 100));
        amb.style.setProperty("--amb-tint", tint);
      } catch {
        // 画布被跨域图片污染时读不到像素,只用默认的压暗。
      }
    };
    if (!src) {
      draw(null, false);
      return;
    }
    // 先按跨域方式加载以便取色;图床不允许跨域时改用普通方式,只画背景。
    const load = (cors: boolean) => {
      const img = new Image();
      if (cors) img.crossOrigin = "anonymous";
      img.decoding = "async";
      img.onload = () => draw(img, cors);
      img.onerror = () => (cors ? load(false) : draw(null, false));
      img.src = ambientUrl(src);
    };
    load(true);
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
      img.draggable = false;
      img.src = d.src;
      inner.append(img);
      y.append(inner);
      ghost.append(y);
      kp.append(ghost);
      const opts = { duration: 600, fill: "forwards" as const };
      ghost.animate([{ transform: `translateX(${d.rect.left}px)` }, { transform: `translateX(${target.left}px)` }], {
        ...opts,
        easing: "cubic-bezier(.25,.8,.35,1)",
      });
      y.animate([{ transform: `translateY(${d.rect.top}px)` }, { transform: `translateY(${target.top}px)` }], {
        ...opts,
        easing: "cubic-bezier(.55,0,.85,.45)",
      });
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
            this.q(this.ui.mode === "open" ? ".cover" : ".disc").animate(
              [{ transform: "scale(1)" }, { transform: "scale(1.14)" }, { transform: "scale(1)" }],
              {
                duration: 460,
                easing: "cubic-bezier(.2,.85,.25,1)",
              },
            );
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
        html`<button class="line" role="listitem" data-i=${i}><span class="lt">${lineUnits(line).map((u) => html`<span class="u">${u}</span>`)}</span>${
          line.translation ? html`<span class="tr">${line.translation}</span>` : nothing
        }</button>`,
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

/** ambientUrl 背景只需要很小的图:网易云图床支持 ?param=宽y高 缩略,其它地址原样使用。 */
function ambientUrl(src: string): string {
  if (!/(^|\.)music\.126\.net\//.test(src.replace(/^https?:\/\//, ""))) return src;
  return src + (src.includes("?") ? "&" : "?") + "param=96y96";
}

/** parseVeil 读出薄雾的颜色与不透明度(rgba(...)),读不出来时当作没有薄雾。 */
function parseVeil(value: string): [number, number, number, number] {
  const m = value.match(/rgba?\(([^)]+)\)/);
  const n = m?.[1]?.split(",").map((v) => parseFloat(v)) ?? [];
  return n.length >= 3 ? [n[0] ?? 0, n[1] ?? 0, n[2] ?? 0, n[3] ?? 1] : [0, 0, 0, 0];
}

/** palette 从 40×40 的像素里算叠上薄雾之后的平均相对亮度,以及右下四分之一的平均色(光斑颜色)。 */
function palette(px: Uint8ClampedArray, veil: [number, number, number, number]): { luminance: number; tint: string } {
  const lin = (c: number) => {
    const v = c / 255;
    return v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
  };
  let lum = 0;
  let n = 0;
  let tr = 0;
  let tg = 0;
  let tb = 0;
  let tn = 0;
  for (let y = 0; y < 40; y++) {
    for (let x = 0; x < 40; x++) {
      const i = (y * 40 + x) * 4;
      const r = px[i] ?? 0;
      const g = px[i + 1] ?? 0;
      const b = px[i + 2] ?? 0;
      const [vr, vg, vb, va] = veil;
      lum +=
        0.2126 * lin(r * (1 - va) + vr * va) +
        0.7152 * lin(g * (1 - va) + vg * va) +
        0.0722 * lin(b * (1 - va) + vb * va);
      n++;
      if (x >= 20 && y >= 20) {
        tr += r;
        tg += g;
        tb += b;
        tn++;
      }
    }
  }
  const avg = (c: number) => Math.round(c / Math.max(tn, 1));
  return { luminance: lum / Math.max(n, 1), tint: `rgb(${avg(tr)}, ${avg(tg)}, ${avg(tb)})` };
}
