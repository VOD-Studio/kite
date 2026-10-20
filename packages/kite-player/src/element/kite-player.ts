import { firstSungIndex, formatTime, lineUnits, wordProgress } from "../core/lyrics";
import { modeLabels, type Player } from "../core/player";
import type { PlayMode, PlayerState, Playlist, Track } from "../core/types";
import { blob, eq, icons } from "./icons";
import { playerStyles } from "./player-styles";
import { $, artists, canAnimate, cssEscape, esc, flyEvent, openEvent, openQueueEvent, playerFor, toastEvent, type FlyDetail, type OpenDetail } from "./util";

type Mode = "bar" | "orb" | "open";
type View = "cover" | "lyrics";

const template = /* html */ `
<style>${playerStyles}</style>
<div class="kp" part="root" data-mode="bar" data-view="cover" data-pm="loop" data-trans>
  <div class="bar" part="bar">
    <button class="disc expand" aria-label="展开播放器"><span class="ring"></span><span class="disc-img"><img alt=""></span></button>
    <button class="bar-text expand" aria-label="展开播放器"><span class="bar-title ell"></span><span class="bar-sub"><span class="ell"></span></span></button>
    <div class="bar-ctl">
      <button class="ib desk prev" aria-label="上一首">${icons.prev}</button>
      <button class="blob toggle" aria-label="播放">${blob}</button>
      <button class="ib next" aria-label="下一首">${icons.next}</button>
      <span class="bar-sep desk"></span>
      <button class="ib open-queue" aria-label="播放列表">${icons.queue}</button>
    </div>
  </div>

  <div class="stage" part="panel" role="dialog" aria-label="正在播放" hidden>
    <div class="amb"><img alt=""></div>
    <div class="scrim"></div>
    <div class="pages">
      <section class="page page-cover">
        <div class="meta enter"><span class="meta-title ell"></span><span class="meta-sub"><span class="ell meta-artist"></span><span class="tag access-tag" hidden></span></span></div>
      </section>
      <section class="page page-lyrics">
        <button class="head to-cover" aria-label="返回封面"><span class="head-title ell"></span><span class="head-sub ell"></span></button>
        <div class="lyrics" part="lyrics">
          <div class="lines" role="list"></div>
          <div class="lyr-empty" hidden>${eq}<span class="lyr-empty-text"></span></div>
          <button class="seek" aria-label="跳到这一句">${icons.play}<span></span></button>
        </div>
      </section>
    </div>
    <button class="cover" aria-label="查看歌词"><img alt=""><span class="badge" hidden><b></b><span></span></span></button>
    <div class="top enter">
      <button class="ib close" aria-label="收起">${icons.chevron}</button>
      <div class="dots"><button data-go="cover" aria-label="封面"></button><button data-go="lyrics" aria-label="歌词"></button></div>
      <button class="ib immersive" aria-label="沉浸模式">${icons.expand}</button>
    </div>
    <div class="ctl enter" part="controls">
      <div class="progress"><span class="ptrack"></span><span class="pchorus" hidden></span><span class="pfill"></span><span class="pthumb"></span><input type="range" min="0" max="1000" step="1" value="0" aria-label="播放进度"></div>
      <div class="times"><span class="t-cur">0:00</span><span class="t-dur">0:00</span></div>
      <div class="buttons">
        <button class="ib mode" aria-label="播放模式"><span class="m m-sequence">${icons.sequence}</span><span class="m m-loop">${icons.loop}</span><span class="m m-one">${icons.one}</span><span class="m m-shuffle">${icons.shuffle}</span></button>
        <button class="ib big prev" aria-label="上一首">${icons.prev}</button>
        <button class="blob toggle" aria-label="播放">${blob}</button>
        <button class="ib big next" aria-label="下一首">${icons.next}</button>
        <button class="ib open-queue" aria-label="播放列表">${icons.queue}</button>
      </div>
      <div class="extra">
        <button class="ib chorus-btn" aria-label="从副歌开始">${icons.wave}</button>
        <button class="ib trans-btn" aria-label="显示翻译" aria-pressed="true">${icons.lyric}</button>
      </div>
      <div class="tip"></div>
    </div>
  </div>

  <div class="backdrop"></div>
  <div class="sheet" part="queue" role="dialog" aria-label="播放列表">
    <div class="sheet-head">
      <span class="handle"></span>
      <div class="sheet-title"><span>播放列表</span><button class="ib close-queue" aria-label="关闭">${icons.close}</button></div>
      <div class="tabs" role="tablist"><span class="ind"></span></div>
      <button class="moderow mode-row"><span class="mode-icon"></span><span class="mode-label"></span><span class="src"></span></button>
    </div>
    <div class="list" role="list"></div>
  </div>
  <div class="toast" role="status" aria-live="polite"></div>
</div>`;

/** 换句时每往下一行多晚 35ms 开始移动,形成逐行错开的波浪。 */
const lyricStagger = 35;
/** 歌词页没有操作多久后隐藏播放控制。 */
const idleDelay = 3000;
/** 手动拖动歌词后多久回到当前行。 */
const resumeDelay = 2500;
/** 向下滚动多少像素后收起为圆形。 */
const collapseScroll = 24;

/**
 * <kite-player>:全站悬浮的播放器。迷你条 → 面板(桌面)/ 全屏(手机)→ 沉浸模式,
 * 外加播放列表抽屉。页面里的 <kite-track> / <kite-playlist> 与它共用同一个播放核心。
 *
 * 属性:api(公开模式地址)、playlists(播放列表里列出的歌单 ID,逗号分隔;第一个作为初始队列)、
 * theme(auto / light / dark)、collapse(scroll:滚动时自动收起;none:不收起)。
 */
export class KitePlayerElement extends HTMLElement {
  static observedAttributes = ["playlists", "theme"];

  private player!: Player;
  private root!: ShadowRoot;
  private kp!: HTMLElement;
  private ui = { mode: "bar" as Mode, view: "cover" as View, immersive: false, sheet: false, idle: false, mobile: false, trans: true, tab: "" };
  private cleanup: (() => void)[] = [];
  private lineEls: HTMLElement[] = [];
  private unitEls: HTMLElement[][] = [];
  private unitValues: number[] = [];
  private tops: number[] = [];
  private lastLine = -1;
  private manual = { active: false, offset: 0, startY: 0, startOffset: 0, moved: false, pointer: -1 };
  private timers = { idle: 0, resume: 0, toast: 0, tip: 0 };
  private scrollState = { last: 0, acc: 0 };
  private dragging = false;
  private sheetTracks: Track[] = [];
  private tabPlaylists = new Map<string, Playlist>();

  connectedCallback(): void {
    if (!this.root) {
      this.root = this.attachShadow({ mode: "open" });
      this.root.innerHTML = template;
      this.kp = $(this.root, ".kp");
      this.ui.trans = readFlag("kite-player:trans", true);
      this.kp.toggleAttribute("data-trans", this.ui.trans);
      $(this.root, ".trans-btn").setAttribute("aria-pressed", String(this.ui.trans));
      this.bindLocal();
    }
    this.player = playerFor(this);
    this.bindGlobal();
    this.render(this.player.state, null);
    this.cleanup.push(this.player.subscribe((s, prev) => this.render(s, prev)));
    this.cleanup.push(this.player.onTime((t) => this.onFrame(t)));
    void this.loadInitial();
  }

  disconnectedCallback(): void {
    for (const fn of this.cleanup) fn();
    this.cleanup = [];
    for (const t of Object.values(this.timers)) clearTimeout(t);
    this.unlockScroll();
  }

  attributeChangedCallback(name: string): void {
    if (!this.root || !this.isConnected) return;
    if (name === "playlists" && this.ui.sheet) void this.renderSheet();
  }

  /** open 展开播放器;view 可指定打开封面还是歌词。 */
  open(view?: View): void {
    if (view) this.setView(view);
    if (this.ui.mode === "open" || !this.player.state.current) return;
    const bar = $(this.root, ".bar").getBoundingClientRect();
    const disc = $(this.root, ".disc").getBoundingClientRect();
    this.setMode("open");
    const stage = $(this.root, ".stage");
    stage.hidden = false;
    this.lockScroll();
    requestAnimationFrame(() => this.layoutLines());
    if (canAnimate(stage)) {
      const to = stage.getBoundingClientRect();
      const radius = parseFloat(getComputedStyle(stage).borderTopLeftRadius) || 0;
      stage.animate(
        [{ clipPath: insetFrom(bar, to, bar.height / 2) }, { clipPath: `inset(0px 0px 0px 0px round ${radius}px)` }],
        { duration: 450, easing: "cubic-bezier(.2,.85,.25,1)" },
      );
      this.flipCover(disc, 430);
      this.root.querySelectorAll<HTMLElement>(".stage .enter, .page-lyrics .head").forEach((el, i) => {
        el.animate([{ opacity: 0, transform: "translateY(24px)" }, { opacity: 1, transform: "none" }], {
          duration: 380,
          delay: 100 + i * 60,
          easing: "cubic-bezier(.2,.85,.25,1)",
          fill: "backwards",
        });
      });
    }
    this.resetIdle();
    $(this.root, ".stage .close").focus({ preventScroll: true });
  }

  /** close 收起回迷你条。 */
  close(): void {
    if (this.ui.mode !== "open") return;
    this.closeSheet();
    const stage = $(this.root, ".stage");
    let done = false;
    const finish = () => {
      if (done) return;
      done = true;
      stage.hidden = true;
      this.setImmersive(false);
      this.setMode("bar");
      this.unlockScroll();
    };
    if (!canAnimate(stage)) {
      finish();
      return;
    }
    const from = stage.getBoundingClientRect();
    const bar = $(this.root, ".bar").getBoundingClientRect();
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

  // ---- 绑定 ----

  /** bindLocal 绑定 shadow DOM 内部的事件,只在第一次挂载时执行。 */
  private bindLocal(): void {
    const r = this.root;
    const on = <K extends keyof HTMLElementEventMap>(sel: string, type: K, fn: (e: HTMLElementEventMap[K], el: HTMLElement) => void) =>
      r.querySelectorAll<HTMLElement>(sel).forEach((el) => el.addEventListener(type, (e) => fn(e, el)));

    on(".expand", "click", () => (this.ui.mode === "orb" ? this.setMode("bar") : this.open("cover")));
    on(".toggle", "click", () => void this.player.toggle());
    on(".prev", "click", () => this.player.prev());
    on(".next", "click", () => this.player.next());
    on(".open-queue", "click", () => void this.openSheet());
    on(".close", "click", () => this.close());
    on(".dots button", "click", (_, el) => this.setView(el.dataset.go === "lyrics" ? "lyrics" : "cover"));
    on(".cover", "click", () => !this.ui.immersive && this.setView(this.ui.view === "cover" ? "lyrics" : "cover"));
    on(".to-cover", "click", () => this.setView("cover"));
    on(".immersive", "click", () => this.setImmersive(!this.ui.immersive, true));
    on(".mode", "click", () => this.cycleMode(true));
    on(".mode-row", "click", () => this.cycleMode(false));
    on(".chorus-btn", "click", () => this.player.playChorus());
    on(".trans-btn", "click", () => this.toggleTrans());
    on(".close-queue", "click", () => this.closeSheet());
    on(".backdrop", "click", () => this.closeSheet());
    on(".seek", "click", () => this.seekToHint());

    const range = $<HTMLInputElement>(r, ".progress input");
    range.addEventListener("input", () => {
      this.dragging = true;
      const pct = Number(range.value) / 1000;
      this.setPct(pct);
      $(r, ".t-cur").textContent = formatTime(pct * this.player.state.duration);
    });
    range.addEventListener("change", () => {
      this.dragging = false;
      this.player.seek((Number(range.value) / 1000) * this.player.state.duration);
    });

    const stage = $(r, ".stage");
    stage.addEventListener("keydown", (e) => {
      if (e.key !== "Escape") return;
      e.stopPropagation();
      if (this.ui.sheet) this.closeSheet();
      else if (this.ui.immersive) this.setImmersive(false, true);
      else this.close();
    });
    $(r, ".sheet").addEventListener("keydown", (e) => e.key === "Escape" && this.closeSheet());
    for (const type of ["pointermove", "pointerdown", "keydown", "wheel"] as const) {
      stage.addEventListener(type, () => this.resetIdle(), { passive: true });
    }
    this.bindSwipe($(r, ".pages"));
    this.bindLyricsScroll($(r, ".lyrics"));
    $(r, ".lines").addEventListener("click", (e) => {
      const line = (e.target as HTMLElement).closest<HTMLElement>(".line");
      if (!line || this.manual.moved) return;
      const l = this.player.state.lyrics?.lines[Number(line.dataset.i)];
      if (!l) return;
      this.endManual(true);
      this.player.seek(l.time / 1000);
      void this.player.play();
    });

  }

  /** bindGlobal 绑定 window / 媒体查询上的事件,每次挂载都绑,卸载时解绑。 */
  private bindGlobal(): void {
    const listen = (target: EventTarget, type: string, fn: (e: Event) => void, opts?: AddEventListenerOptions) => {
      target.addEventListener(type, fn, opts);
      this.cleanup.push(() => target.removeEventListener(type, fn, opts));
    };
    const mq = matchMedia("(max-width: 767px)");
    const onDevice = () => {
      this.ui.mobile = mq.matches;
      this.kp.dataset.device = mq.matches ? "mobile" : "desktop";
      if (mq.matches && this.ui.immersive) this.setImmersive(false);
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
      this.open(d.view);
      if (d.immersive && !this.ui.mobile) this.setImmersive(true, true);
    });
  }

  private bindSwipe(el: HTMLElement): void {
    let x0 = 0;
    let y0 = 0;
    let id = -1;
    el.addEventListener("pointerdown", (e) => {
      if (this.ui.immersive || (e.target as HTMLElement).closest(".lyrics")) return;
      id = e.pointerId;
      x0 = e.clientX;
      y0 = e.clientY;
    });
    el.addEventListener("pointerup", (e) => {
      if (e.pointerId !== id) return;
      id = -1;
      const dx = e.clientX - x0;
      if (Math.abs(dx) < 56 || Math.abs(dx) < Math.abs(e.clientY - y0) * 1.5) return;
      this.setView(dx < 0 ? "lyrics" : "cover");
    });
  }

  private bindLyricsScroll(box: HTMLElement): void {
    box.addEventListener(
      "wheel",
      (e) => {
        if (!this.lineEls.length) return;
        e.preventDefault();
        this.beginManual();
        this.manual.offset -= e.deltaY;
        this.positionLines(true);
        this.scheduleResume();
      },
      { passive: false },
    );
    box.addEventListener("pointerdown", (e) => {
      if (!this.lineEls.length || e.pointerType === "mouse") return;
      this.manual.pointer = e.pointerId;
      this.manual.startY = e.clientY;
      this.manual.startOffset = this.manual.offset;
      this.manual.moved = false;
    });
    box.addEventListener("pointermove", (e) => {
      if (e.pointerId !== this.manual.pointer) return;
      const dy = e.clientY - this.manual.startY;
      if (!this.manual.moved && Math.abs(dy) < 8) return;
      this.manual.moved = true;
      this.beginManual();
      this.manual.offset = this.manual.startOffset + dy;
      this.positionLines(true);
    });
    const up = (e: PointerEvent) => {
      if (e.pointerId !== this.manual.pointer) return;
      this.manual.pointer = -1;
      if (this.manual.moved) this.scheduleResume();
      setTimeout(() => (this.manual.moved = false), 0);
    };
    box.addEventListener("pointerup", up);
    box.addEventListener("pointercancel", up);
  }

  // ---- 状态 ----

  private setMode(mode: Mode): void {
    this.ui.mode = mode;
    this.kp.dataset.mode = mode;
  }

  private setView(view: View): void {
    if (this.ui.view === view) return;
    this.ui.view = view;
    this.kp.dataset.view = view;
    $(this.root, ".cover").setAttribute("aria-label", view === "cover" ? "查看歌词" : "返回封面");
    this.endManual(false);
    this.resetIdle();
    requestAnimationFrame(() => this.layoutLines());
  }

  private setImmersive(on: boolean, animate = false): void {
    if (this.ui.immersive === on || (on && this.ui.mobile)) return;
    const stage = $(this.root, ".stage");
    const before = stage.getBoundingClientRect();
    this.ui.immersive = on;
    this.kp.toggleAttribute("data-immersive", on);
    const btn = $(this.root, ".immersive");
    btn.innerHTML = on ? icons.shrink : icons.expand;
    btn.setAttribute("aria-label", on ? "退出沉浸模式" : "沉浸模式");
    this.resetIdle();
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
    const tip = $(this.root, ".tip");
    tip.textContent = modeLabels[mode];
    tip.classList.add("on");
    clearTimeout(this.timers.tip);
    this.timers.tip = window.setTimeout(() => tip.classList.remove("on"), 1400);
  }

  private toggleTrans(): void {
    this.ui.trans = !this.ui.trans;
    writeFlag("kite-player:trans", this.ui.trans);
    this.kp.toggleAttribute("data-trans", this.ui.trans);
    $(this.root, ".trans-btn").setAttribute("aria-pressed", String(this.ui.trans));
    this.layoutLines();
  }

  /** resetIdle:歌词页停止操作 3 秒后隐藏播放控制,任何操作让它回来。 */
  private resetIdle(): void {
    if (this.ui.idle) {
      this.ui.idle = false;
      this.kp.removeAttribute("data-idle");
    }
    clearTimeout(this.timers.idle);
    if (this.ui.mode !== "open" || this.ui.view !== "lyrics" || this.ui.immersive) return;
    this.timers.idle = window.setTimeout(() => {
      if (!this.player.state.playing || this.ui.view !== "lyrics" || this.ui.immersive || this.ui.sheet) return;
      this.ui.idle = true;
      this.kp.setAttribute("data-idle", "");
    }, idleDelay);
  }

  private onScroll(): void {
    const y = scrollY;
    const dy = y - this.scrollState.last;
    this.scrollState.last = y;
    if (this.getAttribute("collapse") === "none" || this.ui.mode === "open" || !this.player.state.current) return;
    this.scrollState.acc = Math.sign(dy) === Math.sign(this.scrollState.acc) ? this.scrollState.acc + dy : dy;
    if (this.scrollState.acc > collapseScroll && this.ui.mode === "bar") this.setMode("orb");
    else if (this.scrollState.acc < -collapseScroll && this.ui.mode === "orb") this.setMode("bar");
  }

  private async loadInitial(): Promise<void> {
    const first = this.playlistIds()[0];
    if (!first || this.player.state.queue.length > 0) return;
    await this.player.whenSource();
    try {
      const pl = await this.player.loadPlaylist(first);
      if (this.player.state.queue.length > 0) return;
      const i = pl.tracks.findIndex((t) => t.playable);
      this.player.setQueue(pl.tracks, Math.max(i, 0), { id: pl.id, name: pl.name }, false);
    } catch (err) {
      this.toast((err as Error).message || "歌单加载失败");
    }
  }

  private playlistIds(): string[] {
    return (this.getAttribute("playlists") ?? "")
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
  }

  // ---- 渲染 ----

  private render(s: PlayerState, prev: PlayerState | null): void {
    const r = this.root;
    if (!prev || s.current !== prev.current) this.renderTrack(s.current);
    if (!prev || s.playing !== prev.playing) {
      this.kp.classList.toggle("playing", s.playing);
      r.querySelectorAll(".toggle").forEach((b) => b.setAttribute("aria-label", s.playing ? "暂停" : "播放"));
      if (!s.playing) this.resetIdle();
    }
    if (!prev || s.mode !== prev.mode) {
      this.kp.dataset.pm = s.mode;
      $(r, ".mode").setAttribute("aria-label", "播放模式:" + modeLabels[s.mode]);
      this.renderModeRow(s.mode);
    }
    if (!prev || s.time !== prev.time || s.duration !== prev.duration) {
      const pct = s.duration > 0 ? Math.min(1, s.time / s.duration) : 0;
      if (!this.dragging) {
        this.setPct(pct);
        $(r, ".t-cur").textContent = formatTime(s.time);
        $<HTMLInputElement>(r, ".progress input").value = String(Math.round(pct * 1000));
      }
      $(r, ".t-dur").textContent = formatTime(s.duration);
      $(r, ".ring").style.setProperty("--pct", String(pct));
    }
    if (!prev || s.lyrics !== prev.lyrics || s.duration !== prev.duration) this.renderChorus(s);
    if (!prev || s.lyrics !== prev.lyrics || s.lyricsStatus !== prev.lyricsStatus) this.buildLines(s);
    else if (s.lyricIndex !== prev.lyricIndex) this.positionLines(false);
    if (!prev || s.lyricIndex !== prev.lyricIndex || s.lyrics !== prev.lyrics) this.renderTicker(s);
    if (this.ui.sheet && (!prev || s.index !== prev.index || s.queue !== prev.queue)) this.markSheetRows();
    if (prev && s.error && s.error !== prev.error) this.toast(s.error.message);
    this.kp.toggleAttribute("data-has-track", !!s.current);
  }

  private renderTrack(t: Track | null): void {
    const r = this.root;
    const cover = t?.album.cover ?? "";
    for (const img of r.querySelectorAll<HTMLImageElement>(".disc-img img, .cover img, .amb img")) {
      if (cover) img.src = cover;
      else img.removeAttribute("src");
    }
    const title = t?.title ?? "";
    const who = artists(t);
    $(r, ".bar-title").textContent = title;
    $(r, ".meta-title").textContent = title;
    $(r, ".head-title").textContent = title;
    $(r, ".head-sub").textContent = who;
    $(r, ".meta-artist").textContent = t ? [who, t.album.name].filter(Boolean).join(" · ") : "";
    const tag = $(r, ".access-tag");
    tag.hidden = !t || t.access === "free";
    tag.textContent = t?.access === "vip" ? "VIP" : t?.access === "paid" ? "数字专辑" : "";
    this.lastLine = -1;
    this.endManual(false);
    void this.renderBadge();
  }

  /** 封面左下角的音质标签,来自 /player/v1/info。 */
  private async renderBadge(): Promise<void> {
    const badge = $(this.root, ".badge");
    const labels: Record<string, [string, string]> = {
      standard: ["标准", "128 kbps"],
      exhigh: ["极高音质", "320 kbps"],
      lossless: ["无损", "FLAC"],
      hires: ["Hi-Res", "FLAC"],
    };
    try {
      const info = await this.player.info();
      const l = info && labels[info.quality];
      if (!l) return;
      $(badge, "b").textContent = l[0];
      $(badge, "span").textContent = l[1];
      badge.hidden = false;
    } catch {
      badge.hidden = true;
    }
  }

  private renderTicker(s: PlayerState): void {
    // 署名行(作词 / 作曲)期间播放条仍显示歌手,唱到第一句才换成歌词。
    const lines = s.lyrics?.lines ?? [];
    const line = s.lyricIndex >= 0 && s.lyricIndex >= firstSungIndex(lines) ? lines[s.lyricIndex]?.text : "";
    const text = line || artists(s.current);
    const box = $(this.root, ".bar-sub");
    if (box.textContent === text) return;
    box.innerHTML = `<span class="ell">${esc(text)}</span>`;
  }

  private renderChorus(s: PlayerState): void {
    const ch = s.lyrics?.chorus[0];
    const el = $(this.root, ".pchorus");
    const btn = $<HTMLButtonElement>(this.root, ".chorus-btn");
    btn.disabled = !ch;
    if (!ch || !(s.duration > 0)) {
      el.hidden = true;
      return;
    }
    el.hidden = false;
    el.style.left = `${(ch.start / 1000 / s.duration) * 100}%`;
    el.style.width = `${((ch.end - ch.start) / 1000 / s.duration) * 100}%`;
  }

  private renderModeRow(mode: PlayMode): void {
    $(this.root, ".mode-icon").innerHTML = icons[mode];
    $(this.root, ".mode-label").textContent = modeLabels[mode];
  }

  private setPct(pct: number): void {
    $(this.root, ".progress").style.setProperty("--pct", String(pct));
  }

  // ---- 歌词 ----

  private buildLines(s: PlayerState): void {
    const box = $(this.root, ".lines");
    const empty = $(this.root, ".lyr-empty");
    box.textContent = "";
    this.lineEls = [];
    this.unitEls = [];
    this.unitValues = [];
    const lyr = s.lyrics;
    if (!lyr || lyr.instrumental || lyr.lines.length === 0) {
      empty.hidden = false;
      $(empty, ".lyr-empty-text").textContent = !s.current
        ? ""
        : !lyr
          ? s.lyricsStatus === "failed"
            ? "歌词暂时加载不了"
            : "歌词加载中…"
          : lyr.instrumental
            ? "纯音乐,请安静欣赏"
            : "暂无歌词";
      return;
    }
    empty.hidden = true;
    const frag = document.createDocumentFragment();
    lyr.lines.forEach((line, i) => {
      const el = document.createElement("button");
      el.className = "line";
      el.setAttribute("role", "listitem");
      el.dataset.i = String(i);
      const units = lineUnits(line).map((u) => `<span class="u">${esc(u)}</span>`).join("");
      el.innerHTML = `<span class="lt">${units}</span>${line.translation ? `<span class="tr">${esc(line.translation)}</span>` : ""}`;
      frag.append(el);
      this.lineEls.push(el);
      this.unitEls.push([...el.querySelectorAll<HTMLElement>(".u")]);
    });
    box.append(frag);
    this.lastLine = -1;
    this.layoutLines();
  }

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
    const box = $(this.root, ".lyrics");
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
    if (s.duration > 0 && !this.dragging && this.ui.mode === "open") this.setPct(Math.min(1, t / s.duration));
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
    $(this.root, ".lyrics").classList.add("manual");
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
    this.root?.querySelector(".lyrics")?.classList.remove("manual");
    if (animate) this.positionLines(false);
  }

  private hintLine = -1;
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
    const hint = $(this.root, ".seek");
    hint.style.top = `${anchor + 4}px`;
    $(hint, "span").textContent = formatTime((line?.time ?? 0) / 1000);
  }

  private seekToHint(): void {
    const line = this.player.state.lyrics?.lines[this.hintLine];
    if (!line) return;
    this.endManual(true);
    this.player.seek(line.time / 1000);
    void this.player.play();
  }

  // ---- 播放列表抽屉 ----

  private async openSheet(playlist?: string): Promise<void> {
    if (!this.player.state.current && !this.playlistIds().length && !playlist) return;
    this.ui.sheet = true;
    this.kp.setAttribute("data-sheet", "");
    this.ui.tab = playlist ?? this.player.state.playlist?.id ?? this.playlistIds()[0] ?? "queue";
    await this.renderSheet();
    $(this.root, ".close-queue").focus({ preventScroll: true });
  }

  private closeSheet(): void {
    if (!this.ui.sheet) return;
    this.ui.sheet = false;
    this.kp.removeAttribute("data-sheet");
    this.resetIdle();
  }

  private tabIds(): string[] {
    const ids = this.playlistIds();
    const cur = this.player.state.playlist?.id;
    if (cur && !ids.includes(cur)) ids.unshift(cur);
    if (!cur && this.player.state.queue.length) ids.unshift("queue");
    return ids;
  }

  private async renderSheet(): Promise<void> {
    const tabs = $(this.root, ".tabs");
    const ids = this.tabIds();
    if (!ids.includes(this.ui.tab)) this.ui.tab = ids[0] ?? "queue";
    tabs.querySelectorAll("button").forEach((b) => b.remove());
    ids.forEach((id) => {
      const b = document.createElement("button");
      b.setAttribute("role", "tab");
      b.dataset.id = id;
      b.setAttribute("aria-selected", String(id === this.ui.tab));
      b.innerHTML = esc(this.tabName(id));
      b.addEventListener("click", () => {
        this.ui.tab = id;
        void this.renderSheet();
      });
      tabs.append(b);
    });
    const ind = $(tabs, ".ind");
    const at = Math.max(ids.indexOf(this.ui.tab), 0);
    ind.style.width = `calc((100% - 8px) / ${Math.max(ids.length, 1)})`;
    ind.style.transform = `translateX(${at * 100}%)`;
    tabs.hidden = ids.length < 2;

    const list = $(this.root, ".list");
    const tab = this.ui.tab;
    let tracks: Track[];
    let name = "";
    if (tab === "queue" || tab === this.player.state.playlist?.id) {
      tracks = this.player.state.queue;
      name = this.player.state.playlist?.name ?? "";
    } else {
      list.innerHTML = `<div class="empty">加载中…</div>`;
      try {
        const pl = await this.player.loadPlaylist(tab);
        this.tabPlaylists.set(tab, pl);
        tabs.querySelector<HTMLElement>(`button[data-id="${cssEscape(tab)}"]`)?.replaceChildren(document.createTextNode(pl.name));
        tracks = pl.tracks;
        name = pl.name;
      } catch (err) {
        list.innerHTML = `<div class="empty">${esc((err as Error).message || "歌单加载失败")}</div>`;
        return;
      }
      if (this.ui.tab !== tab) return;
    }
    $(this.root, ".src").textContent = name ? `来自「${name}」` : "";
    this.sheetTracks = tracks;
    list.innerHTML = tracks.length
      ? tracks.map((t, i) => this.rowHTML(t, i)).join("")
      : `<div class="empty">这里还没有歌</div>`;
    list.querySelectorAll<HTMLElement>(".row").forEach((row) => row.addEventListener("click", () => this.pickRow(Number(row.dataset.i))));
    this.markSheetRows();
  }

  private tabName(id: string): string {
    if (id === "queue") return "当前播放";
    if (id === this.player.state.playlist?.id) return this.player.state.playlist.name;
    return this.tabPlaylists.get(id)?.name ?? "歌单";
  }

  private rowHTML(t: Track, i: number): string {
    const tag = t.access === "vip" ? `<span class="rtag vip">VIP</span>` : t.access === "paid" ? `<span class="rtag paid">数字专辑</span>` : "";
    const sub = [artists(t), t.album.name].filter(Boolean).join(" · ");
    const right = t.playable ? `<span class="row-dur">${formatTime(t.durationMs / 1000)}</span>` : `<span class="lock">${icons.lock}</span>`;
    return `<button class="row${t.playable ? "" : " locked"}" role="listitem" data-i="${i}" style="--i:${Math.min(i, 12)}">
      <span class="row-main"><span class="row-title"><span class="ell">${esc(t.title)}</span></span><span class="row-sub">${tag}<span class="ell">${esc(sub)}</span></span></span>${right}</button>`;
  }

  private markSheetRows(): void {
    const cur = this.player.state.current?.id;
    this.root.querySelectorAll<HTMLElement>(".list .row").forEach((row) => {
      const t = this.sheetTracks[Number(row.dataset.i)];
      const isCur = !!t && t.id === cur;
      row.classList.toggle("cur", isCur);
      const title = $(row, ".row-title");
      const has = title.querySelector(".eq");
      if (isCur && !has) title.insertAdjacentHTML("beforeend", eq);
      if (!isCur && has) has.remove();
    });
  }

  private pickRow(i: number): void {
    const t = this.sheetTracks[i];
    if (!t) return;
    if (!t.playable) {
      this.toast(t.access === "vip" ? "这首是 VIP 歌曲:站长登录网易云后才能播放" : "这首歌暂时无法播放");
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
      this.player.setQueue(this.sheetTracks, i, pl ? { id: pl.id, name: pl.name } : null, true);
    }
  }

  // ---- 其它 ----

  private toast(message: string): void {
    const t = $(this.root, ".toast");
    t.textContent = message;
    t.classList.add("on");
    clearTimeout(this.timers.toast);
    this.timers.toast = window.setTimeout(() => t.classList.remove("on"), 2600);
  }

  /** flipCover 让大封面从迷你条的唱片位置放大到位(共享元素)。 */
  private flipCover(from: DOMRect, duration: number): void {
    const cover = $(this.root, ".cover");
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

  /** fly:文章卡片的封面沿弧线飞进播放器(横向先快后慢、纵向先慢后快)。 */
  private fly(d: FlyDetail): void {
    if (!d?.rect || !canAnimate(this.kp)) return;
    requestAnimationFrame(() => {
      const target = (this.ui.mode === "open" ? $(this.root, ".cover") : $(this.root, ".disc")).getBoundingClientRect();
      if (!target.width) return;
      const ghost = document.createElement("div");
      ghost.className = "ghost";
      ghost.innerHTML = `<div class="ghost-y"><div class="ghost-in"><img alt="" src="${esc(d.src)}"></div></div>`;
      this.kp.append(ghost);
      const y = $(ghost, ".ghost-y");
      const inner = $(ghost, ".ghost-in");
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
        .finished.then(() => {
          ghost.remove();
          $(this.root, this.ui.mode === "open" ? ".cover" : ".disc").animate(
            [{ transform: "scale(1)" }, { transform: "scale(1.14)" }, { transform: "scale(1)" }],
            { duration: 460, easing: "cubic-bezier(.2,.85,.25,1)" },
          );
        }, () => ghost.remove());
    });
  }

  private scrollLocked = false;
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

/** insetFrom 把 inner 矩形表示成相对 outer 的 clip-path inset(用于从迷你条长成面板)。 */
function insetFrom(inner: DOMRect, outer: DOMRect, radius: number): string {
  const t = inner.top - outer.top;
  const r = outer.right - inner.right;
  const b = outer.bottom - inner.bottom;
  const l = inner.left - outer.left;
  return `inset(${t}px ${r}px ${b}px ${l}px round ${radius}px)`;
}

function readFlag(key: string, fallback: boolean): boolean {
  try {
    const v = localStorage.getItem(key);
    return v === null ? fallback : v === "1";
  } catch {
    return fallback;
  }
}

function writeFlag(key: string, v: boolean): void {
  try {
    localStorage.setItem(key, v ? "1" : "0");
  } catch {
    // 只影响下次是否默认显示翻译。
  }
}
