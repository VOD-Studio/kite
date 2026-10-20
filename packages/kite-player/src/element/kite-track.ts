import { firstSungIndex, formatTime, lineUnits, wordProgress } from "../core/lyrics";
import type { Player } from "../core/player";
import type { Lyrics, PlayerState, Track } from "../core/types";
import { cardStyles } from "./card-styles";
import { blob, eq, icons } from "./icons";
import { artists, emit, emitFly, esc, openEvent, playerFor, sizeOf, toastEvent, unplayableMessage, type OpenDetail } from "./util";

/**
 * <kite-track song="歌曲 ID" size="s | m | l">:文章里的单曲卡片。
 * s 一行小卡片;m 标准卡片(默认,带当前歌词与完整控制);l 大卡片(封面环境色、滚动歌词)。
 * 与 <kite-player> 共用播放核心:在卡片里播放,右下角播放器同步。
 */
export class KiteTrackElement extends HTMLElement {
  static observedAttributes = ["song", "size"];

  private root: ShadowRoot | null = null;
  private player!: Player;
  private track: Track | null = null;
  private lyrics: Lyrics | null = null;
  private size = "m";
  private unsub: (() => void)[] = [];
  private shownLine = -2;
  private shownLyrics: Lyrics | null = null;
  private units: HTMLElement[] = [];
  private dragging = false;
  private loadSeq = 0;

  connectedCallback(): void {
    if (!this.root) this.root = this.attachShadow({ mode: "open" });
    this.player = playerFor(this);
    this.unsub.push(
      this.player.subscribe((s, prev) => this.update(s, prev)),
      this.player.onTime((t) => this.onFrame(t)),
    );
    void this.load();
  }

  disconnectedCallback(): void {
    for (const fn of this.unsub) fn();
    this.unsub = [];
  }

  attributeChangedCallback(name: string, old: string | null, value: string | null): void {
    if (!this.root || old === value) return;
    if (name === "song") void this.load();
    else this.draw();
  }

  private async load(): Promise<void> {
    const id = this.getAttribute("song");
    const root = this.root;
    if (!id || !root) return;
    const seq = ++this.loadSeq;
    root.innerHTML = `<style>${cardStyles}</style><div class="skeleton" aria-busy="true"></div>`;
    try {
      await this.player.whenSource();
      const track = await this.player.loadTrack(id);
      const lyrics = sizeOf(this, ["s", "m", "l"], "m") === "s" ? null : await this.player.lyricsFor(track).catch(() => null);
      if (seq !== this.loadSeq) return;
      this.track = track;
      this.lyrics = lyrics;
      this.draw();
    } catch (err) {
      if (seq !== this.loadSeq) return;
      root.innerHTML = `<style>${cardStyles}</style><div class="card s"><div class="main"><div class="title">${esc("这首歌暂时无法显示")}</div><div class="error">${esc((err as Error).message)}</div></div></div>`;
    }
  }

  private draw(): void {
    const t = this.track;
    const root = this.root;
    if (!t || !root) return;
    this.size = sizeOf(this, ["s", "m", "l"], "m");
    const html = this.size === "s" ? this.htmlS(t) : this.size === "l" ? this.htmlL(t) : this.htmlM(t);
    root.innerHTML = `<style>${cardStyles}</style>${html}`;
    root.querySelector(".play")?.addEventListener("click", () => void this.play(false));
    root.querySelector(".chorus")?.addEventListener("click", () => void this.play(true));
    root.querySelector(".prev")?.addEventListener("click", () => this.player.prev());
    root.querySelector(".next")?.addEventListener("click", () => this.player.next());
    root.querySelector(".open")?.addEventListener("click", () => void this.openPlayer());
    const range = root.querySelector<HTMLInputElement>(".progress input");
    range?.addEventListener("input", () => {
      this.dragging = true;
      this.setPct(Number(range.value) / 1000);
    });
    range?.addEventListener("change", () => {
      this.dragging = false;
      this.player.seek((Number(range.value) / 1000) * this.player.state.duration);
    });
    this.shownLine = -2;
    this.update(this.player.state, null);
  }

  private async play(chorus: boolean): Promise<void> {
    const t = this.track;
    if (!t) return;
    if (!t.playable) {
      emit(toastEvent, unplayableMessage(t));
      return;
    }
    const s = this.player.state;
    if (s.current?.id === t.id && !chorus) {
      await this.player.toggle();
      return;
    }
    if (s.current?.id !== t.id) emitFly(this.root?.querySelector(".cover") ?? null, t.album.cover);
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

  // ---- 各尺寸的结构 ----

  private tags(t: Track, out = "out"): string {
    if (t.access === "vip") return `<span class="tg vip">VIP</span>`;
    if (t.access === "paid") return `<span class="tg ${out}">数字专辑</span>`;
    return "";
  }

  private progress(t: Track): string {
    return `<div class="progress" hidden><span class="t-cur num">0:00</span><span class="bar"><b></b></span><span class="t-dur num">${formatTime(t.durationMs / 1000)}</span><input type="range" min="0" max="1000" value="0" aria-label="播放进度"></div>`;
  }

  private htmlS(t: Track): string {
    const locked = !t.playable;
    const sub = locked
      ? `${icons.lock}<span class="ell">${esc(unplayableMessage(t))}</span>`
      : `<span class="ell">${esc([artists(t), t.album.name, formatTime(t.durationMs / 1000)].filter(Boolean).join(" · "))}</span>`;
    return `<div class="card s${locked ? " locked" : ""}" part="card">
      <div class="cover"><img alt="" src="${esc(t.album.cover)}"></div>
      <div class="main">
        <div class="title">${eq}<span class="ell">${esc(t.title)}</span><span class="who ell">${esc(artists(t))}</span></div>
        <div class="sub">${sub}</div>
        ${this.progress(t)}
      </div>
      <div class="actions">${locked ? "" : `<button class="chip chorus" aria-label="从副歌开始播放 ${esc(t.title)}">${icons.wave}副歌</button>`}<button class="blob play" aria-label="播放 ${esc(t.title)}">${blob}</button></div>
    </div>`;
  }

  private htmlM(t: Track): string {
    const locked = !t.playable;
    return `<div class="card m${locked ? " locked" : ""}" part="card">
      <div class="cover"><img alt="" src="${esc(t.album.cover)}"></div>
      <div class="main">
        <div class="head"><div style="flex:1;min-width:0"><div class="title ell">${esc(t.title)}</div><div class="who"><span class="ell">${esc([artists(t), t.album.name].filter(Boolean).join(" · "))}</span>${this.tags(t)}</div></div>${locked ? "" : `<button class="chip chorus">${icons.wave}从副歌听</button>`}</div>
        <div class="lyric"><div class="lyric-line ell"></div><div class="lyric-tr ell"></div></div>
        ${locked ? `<div class="error">${esc(unplayableMessage(t))}</div>` : ""}
        <div class="controls"><button class="ib prev" aria-label="上一首" hidden>${icons.prev}</button><button class="blob play" aria-label="播放 ${esc(t.title)}">${blob}</button><button class="ib next" aria-label="下一首" hidden>${icons.next}</button>${this.progress(t).replace(" hidden", "")}</div>
      </div>
    </div>`;
  }

  private htmlL(t: Track): string {
    const locked = !t.playable;
    return `<div class="card l${locked ? " locked" : ""}" part="card">
      <div class="amb"><img alt="" src="${esc(t.album.cover)}"></div><div class="scrim"></div>
      <div class="top">
        <div class="cover"><img alt="" src="${esc(t.album.cover)}"></div>
        <div class="info"><div class="kicker">单曲</div><div class="title ell">${esc(t.title)}</div><div class="who ell">${esc([artists(t), t.album.name].filter(Boolean).join(" · "))}</div><div class="tags">${this.tags(t)}</div>${locked ? `<div class="error" style="color:rgba(255,255,255,.75)">${esc(unplayableMessage(t))}</div>` : `<button class="chip chorus">${icons.wave}从副歌听</button>`}</div>
        ${locked ? "" : `<button class="ib open" aria-label="在播放器中打开" style="color:rgba(255,255,255,.85)">${icons.expand}</button>`}
      </div>
      <div class="lyric"><div class="lyric-prev ell"></div><div class="lyric-line"></div><div class="lyric-tr ell"></div><div class="lyric-next ell"></div></div>
      <div class="controls"><button class="ib prev" aria-label="上一首" hidden style="color:#fff">${icons.prev}</button><button class="blob play" aria-label="播放 ${esc(t.title)}">${blob}</button><button class="ib next" aria-label="下一首" hidden style="color:#fff">${icons.next}</button>${this.progress(t).replace(" hidden", "")}</div>
    </div>`;
  }

  // ---- 状态同步 ----

  private update(s: PlayerState, prev: PlayerState | null): void {
    const t = this.track;
    const root = this.root;
    const card = root?.querySelector<HTMLElement>(".card");
    if (!t || !root || !card) return;
    const cur = s.current?.id === t.id;
    card.classList.toggle("cur", cur);
    card.classList.toggle("playing", cur && s.playing);
    root.querySelector(".play")?.setAttribute("aria-label", `${cur && s.playing ? "暂停" : "播放"} ${t.title}`);
    for (const el of root.querySelectorAll<HTMLElement>(".prev, .next")) el.hidden = !cur;
    const progress = root.querySelector<HTMLElement>(".progress");
    if (this.size === "s" && progress) {
      progress.hidden = !cur;
      const sub = root.querySelector<HTMLElement>(".sub");
      if (sub) sub.hidden = cur && t.playable;
    }
    if (!cur) this.setPct(0);
    else if (!prev || s.time !== prev.time || s.duration !== prev.duration) this.onFrame(s.time);
    const kicker = root.querySelector(".kicker");
    if (kicker) kicker.textContent = cur ? "单曲 · 正在播放" : "单曲";
    if (this.size !== "s") this.renderLyric(s, cur);
  }

  private renderLyric(s: PlayerState, cur: boolean): void {
    const root = this.root;
    if (!root) return;
    // 刚切到这首时播放器的歌词还在加载,先用卡片自己预取的同一首歌词。
    const lyr = cur ? (s.lyrics ?? this.lyrics) : this.lyrics;
    const lines = lyr?.lines ?? [];
    // 开头的署名行(作词 / 作曲)不当作「正在唱」,唱到第一句之前一直预览第一句。
    const sung = firstSungIndex(lines);
    const idx = cur && s.lyrics ? s.lyricIndex : -1;
    const live = idx >= 0 && idx >= sung;
    const key = live ? idx : -1;
    if (key === this.shownLine && lyr === this.shownLyrics) return;
    this.shownLine = key;
    this.shownLyrics = lyr;
    const at = live ? idx : sung;
    const line = lines[at];
    const lineEl = root.querySelector<HTMLElement>(".lyric-line");
    if (!lineEl) return;
    const instrumental = lyr?.instrumental;
    if (!line) {
      lineEl.textContent = instrumental ? "纯音乐" : "";
      this.units = [];
    } else if (live) {
      lineEl.classList.add("live");
      lineEl.innerHTML = lineUnits(line).map((u) => `<span class="u">${esc(u)}</span>`).join("");
      this.units = [...lineEl.querySelectorAll<HTMLElement>(".u")];
    } else {
      lineEl.classList.remove("live");
      lineEl.textContent = line.text;
      this.units = [];
    }
    const tr = root.querySelector(".lyric-tr");
    if (tr) tr.textContent = line?.translation ?? "";
    const prevEl = root.querySelector(".lyric-prev");
    if (prevEl) prevEl.textContent = live && idx > sung ? (lines[idx - 1]?.text ?? "") : "";
    const nextEl = root.querySelector(".lyric-next");
    if (nextEl) nextEl.textContent = lines[at + 1]?.text ?? "";
  }

  private onFrame(t: number): void {
    const s = this.player.state;
    if (!this.track || s.current?.id !== this.track.id || !this.root) return;
    if (!this.dragging && s.duration > 0) this.setPct(Math.min(1, t / s.duration));
    const cur = this.root.querySelector(".t-cur");
    if (cur) cur.textContent = formatTime(t);
    const dur = this.root.querySelector(".t-dur");
    if (dur && s.duration > 0) dur.textContent = formatTime(s.duration);
    const line = s.lyrics?.lines[s.lyricIndex];
    if (line && this.units.length) {
      const p = wordProgress(line, t * 1000);
      this.units.forEach((u, k) => u.style.setProperty("--p", String(Math.round((p[k] ?? 0) * 100) / 100)));
    }
  }

  private setPct(pct: number): void {
    const root = this.root;
    if (!root) return;
    const bar = root.querySelector<HTMLElement>(".bar");
    bar?.style.setProperty("--pct", String(pct));
    const input = root.querySelector<HTMLInputElement>(".progress input");
    if (input && !this.dragging) input.value = String(Math.round(pct * 1000));
  }
}

