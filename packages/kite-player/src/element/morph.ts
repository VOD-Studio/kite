// 播放条 ↔ 面板 / 全屏 / 沉浸模式的展开与收起:每个元素各走各的路线(参考网易云)。
//
// 进度 p:0 是播放条,1 是面板到位。卡片的可见区域在两者之间插值,顶边跟着进度(或手指)走,
// 一离开播放条就换成封面铺色的背景、圆角收到卡片的圆角。
// - 封面从播放条的唱片长出来:前 12% 由圆变方、转正;大小随进度线性变大,
//   先贴着卡片左上角,卡片够高之后(p 0.2 → 0.7)才下移到自己的位置。
// - 播放条的歌名和按钮留在卡片里:与封面垂直居中,歌名被封面推着往右,很快淡出。
// - 封面下方的歌名区、控制区跟着封面底边走,从 0.6 倍放大到原大并淡入;顶栏随卡片顶边最后出现。
// - 桌面面板的阴影是单独一层,跟着卡片缩放、随进度变深,到位后与面板自己的阴影一致。
//
// 元素只用 translate / scale / rotate / opacity 这几个独立属性(与元素自己的 transform 叠加),
// 可以交给合成线程;卡片形状用 clip-path,封面圆角用 border-radius,另起动画在主线程跑。
// 动画按缓动曲线采样成关键帧,线性播放。所有参与的元素 transform-origin 都是左上角。

export interface Box {
  left: number;
  top: number;
  width: number;
  height: number;
}

/** 参与形变的元素。 */
export interface MorphParts {
  stage: HTMLElement;
  /** 桌面面板的阴影层;手机、沉浸模式没有。 */
  shadow: HTMLElement | null;
  cover: HTMLElement;
  badge: HTMLElement | null;
  /** 卡片里的播放条内容(歌名与按钮),展开初期留在封面旁边。 */
  miniText: HTMLElement | null;
  miniCtl: HTMLElement | null;
  top: HTMLElement;
  /** 跟着封面底边走的元素(封面页的歌名区、控制区),以及各自淡入的区间 [开始, 时长]。 */
  below: Follower[];
  /** 跟着封面右边走的元素(歌词页顶部的歌名)。 */
  beside: Follower | null;
  /** 跟着卡片顶边走的元素(歌词、歌词页的控制区等)。 */
  rest: Follower[];
}

export interface Follower {
  el: HTMLElement;
  fade: [number, number];
}

/** 量出来的元素:位置(含自身 transform)与缩放原点(不含自身 transform 的左上角)。 */
interface Item {
  el: HTMLElement;
  rect: Box;
  o: { x: number; y: number };
  fade: [number, number];
}

export interface MorphGeo {
  parts: MorphParts;
  /** 面板(到位后)与播放条的矩形。 */
  box: Box;
  bar: Box;
  /** 面板到位后的圆角、展开途中卡片的圆角。 */
  radius: number;
  midRadius: number;
  /** 封面的起点:播放条上的唱片(去掉进度环),以及唱片当前转过的角度。 */
  disc: Box;
  discAngle: number;
  cover: Item & { radius: number; own: number };
  badge: Item | null;
  miniText: Item | null;
  miniCtl: Item | null;
  barText: Box | null;
  barCtl: Box | null;
  top: Item;
  below: Item[];
  beside: Item | null;
  rest: Item[];
  /** below / rest 横向缩放的中心:卡片中线(面板、手机)或封面中线(沉浸模式)。 */
  anchor: "card" | "cover";
  /** 下方内容淡入时额外往下错开的距离(随进度收回)。 */
  drop: number;
}

export type Style = Partial<Record<"translate" | "scale" | "rotate" | "opacity" | "clipPath" | "borderRadius", string>>;

const rect = (el: Element): Box => {
  const r = el.getBoundingClientRect();
  return { left: r.left, top: r.top, width: r.width, height: r.height };
};

const clamp01 = (v: number) => Math.min(1, Math.max(0, v));
const lerp = (a: number, b: number, p: number) => (p <= 0 ? a : p >= 1 ? b : a + (b - a) * p);
const smooth = (t: number) => t * t * (3 - 2 * t);
const fadeIn = (p: number, [start, span]: [number, number]) => clamp01((p - start) / span);

/** origin 元素自身 transform 之前的左上角:量到的位置减去自身 transform 的平移(transform-origin 是左上角)。 */
function item(el: HTMLElement, fade: [number, number] = [0, 1]): Item {
  const r = rect(el);
  const m = /matrix(3d)?\(([^)]+)\)/.exec(getComputedStyle(el).transform);
  const v = m?.[2]?.split(",").map(Number) ?? [];
  const [e, f] = m?.[1] ? [v[12] ?? 0, v[13] ?? 0] : [v[4] ?? 0, v[5] ?? 0];
  return { el, rect: r, o: { x: r.left - e, y: r.top - f }, fade };
}

export interface MeasureOptions {
  bar: Element;
  disc: Element;
  barText: Element | null;
  barCtl: Element | null;
  discAngle: number;
  midRadius: number;
  anchor: "card" | "cover";
}

/** measure 在面板到位(没有形变)时量出所有位置。面板没显示出来时返回 null。 */
export function measure(parts: MorphParts, opts: MeasureOptions): MorphGeo | null {
  const box = rect(parts.stage);
  const bar = rect(opts.bar);
  if (!box.height || !bar.height) return null;
  const d = rect(opts.disc);
  // 唱片外圈 3px 是进度环,封面落在里面的圆上。
  const disc = { left: d.left + 3, top: d.top + 3, width: d.width - 6, height: d.height - 6 };
  const cover = item(parts.cover);
  const own = parts.cover.offsetWidth ? cover.rect.width / parts.cover.offsetWidth : 1;
  // 播放条收成圆形(桌面滚动时)放不下歌名,卡片里就不显示播放条内容。
  const mini = bar.width > 160 && opts.barText && opts.barCtl && parts.miniText && parts.miniCtl;
  return {
    parts,
    box,
    bar,
    radius: parseFloat(getComputedStyle(parts.stage).borderTopLeftRadius) || 0,
    midRadius: opts.midRadius,
    disc,
    discAngle: opts.discAngle,
    cover: { ...cover, radius: (parseFloat(getComputedStyle(parts.cover).borderTopLeftRadius) || 0) * own, own },
    badge: parts.badge ? item(parts.badge) : null,
    miniText: mini ? item(parts.miniText!) : null,
    miniCtl: mini ? item(parts.miniCtl!) : null,
    barText: mini ? rect(opts.barText!) : null,
    barCtl: mini ? rect(opts.barCtl!) : null,
    top: item(parts.top),
    below: parts.below.map((f) => item(f.el, f.fade)),
    beside: parts.beside ? item(parts.beside.el, parts.beside.fade) : null,
    rest: parts.rest.map((f) => item(f.el, f.fade)),
    anchor: opts.anchor,
    drop: box.height * 0.1,
  };
}

/** place 让元素(量到的左上角在 it.rect)以 k 倍缩放后左上角落在 (x, y) 时的 translate。 */
const place = (it: Item, x: number, y: number, k: number) =>
  `${x - it.o.x - k * (it.rect.left - it.o.x)}px ${y - it.o.y - k * (it.rect.top - it.o.y)}px`;

/** frame 进度 p(0 = 播放条,1 = 面板)时每个元素的样式。 */
export function frame(g: MorphGeo, p: number): Map<HTMLElement, Style> {
  const q = 1 - p;
  const { box, bar, parts } = g;
  const out = new Map<HTMLElement, Style>();

  // 卡片:四边各自线性插值;圆角先由胶囊收到卡片圆角,最后 15% 再变成面板的圆角。
  const cLeft = lerp(bar.left, box.left, p);
  const cTop = lerp(bar.top, box.top, p);
  const cRight = lerp(bar.left + bar.width, box.left + box.width, p);
  const cBottom = lerp(bar.top + bar.height, box.top + box.height, p);
  const r = p < 0.06 ? lerp(bar.height / 2, g.midRadius, p / 0.06) : p > 0.85 ? lerp(g.midRadius, g.radius, (p - 0.85) / 0.15) : g.midRadius;
  out.set(parts.stage, {
    clipPath: `inset(${cTop - box.top}px ${box.left + box.width - cRight}px ${box.top + box.height - cBottom}px ${cLeft - box.left}px round ${r}px)`,
    // 一离开播放条就换成封面铺色的背景(收起时最后几帧才露出播放条)。
    opacity: String(clamp01(p / 0.03)),
  });
  if (parts.shadow) {
    out.set(parts.shadow, {
      translate: `${cLeft - box.left}px ${cTop - box.top}px`,
      scale: `${(cRight - cLeft) / box.width} ${(cBottom - cTop) / box.height}`,
      opacity: String(p * p),
    });
  }

  // 封面:大小线性变化;横向贴着卡片左边,纵向先贴顶边、卡片够高后再下移到原位;前 12% 转正。
  const C = g.cover;
  const cs = lerp(g.disc.width, C.rect.width, p);
  const k = cs / C.rect.width;
  const ch = C.rect.height * k;
  const x = cLeft + lerp(g.disc.left - bar.left, C.rect.left - box.left, p);
  const y = cTop + lerp(g.disc.top - bar.top, C.rect.top - box.top, smooth(clamp01((p - 0.2) / 0.5)));
  const deg = g.discAngle * (1 - clamp01(p / 0.12));
  const a = (deg * Math.PI) / 180;
  // 绕封面中心转:先算出不转时中心相对原点的位置,再转过去。
  const vx = k * (C.rect.left + C.rect.width / 2 - C.o.x);
  const vy = k * (C.rect.top + C.rect.height / 2 - C.o.y);
  const tx = x + cs / 2 - C.o.x - (vx * Math.cos(a) - vy * Math.sin(a));
  const ty = y + ch / 2 - C.o.y - (vx * Math.sin(a) + vy * Math.cos(a));
  const small = Math.min(8, C.radius);
  const rv = p < 0.08 ? lerp(g.disc.width / 2, small, p / 0.08) : lerp(small, C.radius, (p - 0.08) / 0.92);
  out.set(parts.cover, {
    translate: `${tx}px ${ty}px`,
    scale: String(k),
    rotate: `${deg}deg`,
    borderRadius: `${rv / (k * C.own)}px`,
  });

  // 音质标签跟着封面,后半程淡入。
  if (g.badge) {
    const b = g.badge;
    out.set(b.el, {
      translate: place(b, x + (b.rect.left - C.rect.left) * k, y + (b.rect.top - C.rect.top) * k, k),
      scale: String(k),
      opacity: String(clamp01((p - 0.45) / 0.4)),
    });
  }

  // 播放条内容:与封面垂直居中;歌名在封面右边,按钮贴着卡片右边。
  const rowDy = y + ch / 2 - (bar.top + bar.height / 2);
  if (g.miniText && g.barText) {
    const dx = x + cs - (g.disc.left + g.disc.width);
    out.set(g.miniText.el, { translate: place(g.miniText, g.barText.left + dx, g.barText.top + rowDy, 1), opacity: String(clamp01(1 - p / 0.22)) });
  }
  if (g.miniCtl && g.barCtl) {
    const dx = cRight - (bar.left + bar.width);
    out.set(g.miniCtl.el, { translate: place(g.miniCtl, g.barCtl.left + dx, g.barCtl.top + rowDy, 1), opacity: String(clamp01(1 - p / 0.26)) });
  }

  // 顶栏随卡片顶边,最后出现。
  const T = g.top;
  out.set(T.el, { translate: place(T, T.rect.left + (cLeft + cRight - box.left * 2 - box.width) / 2, cTop + (T.rect.top - box.top), 1), opacity: String(clamp01((p - 0.5) / 0.4)) });

  // 下方内容:从 0.6 倍放大到原大,横向以卡片(或封面)中线为中心,淡入时从下面一点收上来。
  const kl = lerp(0.6, 1, p);
  const ax = g.anchor === "cover" ? x + cs / 2 : (cLeft + cRight) / 2;
  const axEnd = g.anchor === "cover" ? C.rect.left + C.rect.width / 2 : box.left + box.width / 2;
  const coverBottom = y + ch;
  const coverBottomEnd = C.rect.top + C.rect.height;
  for (const it of g.below) {
    out.set(it.el, {
      translate: place(it, ax + (it.rect.left - axEnd) * kl, coverBottom + (it.rect.top - coverBottomEnd) * kl + q * g.drop, kl),
      scale: String(kl),
      opacity: String(fadeIn(p, it.fade)),
    });
  }
  if (g.beside) {
    const it = g.beside;
    out.set(it.el, {
      translate: place(it, x + cs + (it.rect.left - C.rect.left - C.rect.width), y + ch / 2 + (it.rect.top - C.rect.top - C.rect.height / 2), 1),
      opacity: String(fadeIn(p, it.fade)),
    });
  }
  for (const it of g.rest) {
    out.set(it.el, {
      translate: place(it, ax + (it.rect.left - axEnd) * kl, cTop + (it.rect.top - box.top) * kl + q * g.drop, kl),
      scale: String(kl),
      opacity: String(fadeIn(p, it.fade)),
    });
  }
  return out;
}

/** apply 把某一帧直接写到元素上(跟手时用)。 */
export function apply(frames: Map<HTMLElement, Style>): void {
  for (const [el, style] of frames) Object.assign(el.style, style);
}

/** clear 去掉 apply 写上的样式。 */
export function clear(parts: MorphParts | null): void {
  if (!parts) return;
  const els = [parts.stage, parts.shadow, parts.cover, parts.badge, parts.miniText, parts.miniCtl, parts.top, parts.beside?.el, ...parts.below.map((f) => f.el), ...parts.rest.map((f) => f.el)];
  for (const el of els) {
    if (!el) continue;
    for (const k of ["translate", "scale", "rotate", "opacity", "clip-path", "border-radius"]) el.style.removeProperty(k);
  }
}

/** 合成线程能播放的属性;其余(clip-path、border-radius)另起一个动画,不拖累前者。 */
const composited = new Set(["translate", "scale", "rotate", "opacity"]);

/**
 * play 从进度 from 动到 to:按缓动曲线采样成关键帧,每个元素一组线性关键帧。
 * 返回各元素的动画(第一个是卡片的)。
 */
export function play(g: MorphGeo, from: number, to: number, duration: number, ease: (t: number) => number): Animation[] {
  const steps = 24;
  const frames = Array.from({ length: steps + 1 }, (_, i) => frame(g, from + (to - from) * ease(i / steps)));
  const els = [...(frames[0]?.keys() ?? [])];
  const timing: KeyframeAnimationOptions = { duration, easing: "linear", fill: to === 0 ? "forwards" : "none" };
  const anims: Animation[] = [];
  for (const el of els) {
    const keys = Object.keys(frames[0]?.get(el) ?? {}) as (keyof Style)[];
    for (const group of [keys.filter((k) => composited.has(k)), keys.filter((k) => !composited.has(k))]) {
      if (!group.length) continue;
      anims.push(el.animate(frames.map((f) => Object.fromEntries(group.map((k) => [k, f.get(el)?.[k] ?? ""]))), timing));
    }
  }
  return anims;
}

/** cubicBezier 返回与 CSS cubic-bezier() 相同的缓动函数(牛顿迭代 + 二分兜底)。 */
export function cubicBezier(x1: number, y1: number, x2: number, y2: number): (t: number) => number {
  const ax = 3 * x1 - 3 * x2 + 1;
  const bx = 3 * x2 - 6 * x1;
  const cx = 3 * x1;
  const ay = 3 * y1 - 3 * y2 + 1;
  const by = 3 * y2 - 6 * y1;
  const cy = 3 * y1;
  const sx = (t: number) => ((ax * t + bx) * t + cx) * t;
  const sy = (t: number) => ((ay * t + by) * t + cy) * t;
  const dx = (t: number) => (3 * ax * t + 2 * bx) * t + cx;
  return (x: number) => {
    if (x <= 0) return 0;
    if (x >= 1) return 1;
    let t = x;
    for (let i = 0; i < 6; i++) {
      const d = dx(t);
      if (Math.abs(d) < 1e-6) break;
      const next = t - (sx(t) - x) / d;
      if (Math.abs(next - t) < 1e-6) {
        t = next;
        break;
      }
      t = next;
    }
    if (t < 0 || t > 1 || Math.abs(sx(t) - x) > 1e-4) {
      let lo = 0;
      let hi = 1;
      t = x;
      for (let i = 0; i < 30; i++) {
        if (sx(t) < x) lo = t;
        else hi = t;
        t = (lo + hi) / 2;
      }
    }
    return sy(t);
  };
}
