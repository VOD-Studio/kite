// 播放条 ↔ 面板 / 全屏 / 沉浸模式的展开与收起:每个元素各走各的路线(参考网易云)。
//
// 卡片的可见区域在播放条与面板之间插值,顶边跟着进度(或手指)走;封面飞向播放条的唱片,
// 歌名飞向播放条的歌名并缩到同样的字号,其余内容随顶边下移并淡出。内容不整体缩放,
// 所以中途不会出现「整块缩在左上角、右边一大片空白」的样子。
//
// 元素只用 translate / scale / opacity 三个独立属性(与元素自己的 transform 叠加,不覆盖),
// 可以交给合成线程;卡片形状用 clip-path。动画按缓动曲线采样成关键帧,线性播放。

export interface Box {
  left: number;
  top: number;
  width: number;
  height: number;
}

/** 参与形变的元素。text 是封面页的歌名区或歌词页顶部的歌名;rest 是随顶边下移并淡出的其它内容。 */
export interface MorphParts {
  stage: HTMLElement;
  cover: HTMLElement;
  badge: HTMLElement | null;
  text: HTMLElement | null;
  top: HTMLElement;
  ctl: HTMLElement;
  rest: HTMLElement[];
}

export interface MorphGeo {
  parts: MorphParts;
  /** 面板(到位后)与播放条的矩形。 */
  box: Box;
  bar: Box;
  /** 面板到位后的圆角。 */
  radius: number;
  /** 封面的终点:播放条上的唱片(去掉进度环)。 */
  disc: Box;
  cover: Box;
  /** 封面没有 transform 时左上角的位置(translate / scale 绕它计算)。 */
  coverOrigin: { x: number; y: number };
  badge: Box | null;
  text: Box | null;
  /** 歌名的终点:播放条上的歌名,以及两者字号之比。 */
  barText: Box;
  textScale: number;
}

type Style = Partial<Record<"translate" | "scale" | "opacity" | "clipPath", string>>;

const rect = (el: Element): Box => {
  const r = el.getBoundingClientRect();
  return { left: r.left, top: r.top, width: r.width, height: r.height };
};

const clamp01 = (v: number) => Math.min(1, Math.max(0, v));
const lerp = (a: number, b: number, p: number) => a + (b - a) * p;

/** measure 在面板到位(没有形变)时量出所有位置。面板没显示出来时返回 null。 */
export function measure(parts: MorphParts, bar: Element, disc: Element, barTitle: Element, fg: Element): MorphGeo | null {
  const box = rect(parts.stage);
  const b = rect(bar);
  if (!box.height || !b.height) return null;
  const d = rect(disc);
  // 唱片外圈 3px 是进度环,封面落在里面的圆上。
  const discBox = { left: d.left + 3, top: d.top + 3, width: d.width - 6, height: d.height - 6 };
  const cover = rect(parts.cover);
  const fgBox = fg.getBoundingClientRect();
  const coverEl = parts.cover;
  const text = parts.text && parts.text.offsetParent ? rect(parts.text) : null;
  const fontOf = (el: Element | null) => (el ? parseFloat(getComputedStyle(el).fontSize) || 16 : 16);
  const textTitle = parts.text?.firstElementChild ?? null;
  return {
    parts,
    box,
    bar: b,
    radius: parseFloat(getComputedStyle(parts.stage).borderTopLeftRadius) || 0,
    disc: discBox,
    cover,
    coverOrigin: { x: fgBox.left + coverEl.offsetLeft, y: fgBox.top + coverEl.offsetTop },
    badge: parts.badge ? rect(parts.badge) : null,
    text,
    barText: rect(barTitle),
    textScale: fontOf(barTitle) / fontOf(textTitle),
  };
}

/** frame 进度 p(0 = 播放条,1 = 面板)时每个元素的样式。 */
export function frame(g: MorphGeo, p: number): Map<HTMLElement, Style> {
  const q = 1 - p;
  const { box, bar, parts } = g;
  const out = new Map<HTMLElement, Style>();
  const top = (bar.top - box.top) * q;
  const right = (box.left + box.width - (bar.left + bar.width)) * q;
  const bottom = (box.top + box.height - (bar.top + bar.height)) * q;
  const left = (bar.left - box.left) * q;
  const radius = lerp(bar.height / 2, g.radius, p);
  out.set(parts.stage, {
    clipPath: `inset(${top}px ${right}px ${bottom}px ${left}px round ${radius}px)`,
    // 最后 12% 淡出,露出下面的播放条(展开时反过来)。
    opacity: String(clamp01(p / 0.12)),
  });

  // 封面:矩形从唱片插值到原位。
  const c = g.cover;
  const cl = lerp(g.disc.left, c.left, p);
  const ct = lerp(g.disc.top, c.top, p);
  const cs = lerp(g.disc.width, c.width, p) / c.width;
  const o = g.coverOrigin;
  out.set(parts.cover, {
    translate: `${cl - o.x - cs * (c.left - o.x)}px ${ct - o.y - cs * (c.top - o.y)}px`,
    scale: String(cs),
  });
  // 音质标签跟着封面走,先淡出。
  if (parts.badge && g.badge) {
    const b = g.badge;
    out.set(parts.badge, {
      translate: `${cl + (b.left - c.left) * cs - b.left}px ${ct + (b.top - c.top) * cs - b.top}px`,
      scale: String(cs),
      opacity: String(clamp01((p - 0.4) / 0.6)),
    });
  }
  // 歌名:飞向播放条的歌名,缩到同样的字号;快到时淡出,与播放条的歌名交叉。
  if (parts.text && g.text) {
    const t = g.text;
    out.set(parts.text, {
      translate: `${lerp(g.barText.left, t.left, p) - t.left}px ${lerp(g.barText.top, t.top, p) - t.top}px`,
      scale: String(lerp(g.textScale, 1, p)),
      opacity: String(clamp01((p - 0.08) / 0.4)),
    });
  }
  // 顶栏、控制区与其它内容随卡片顶边下移,依次淡出(越靠下越晚走、越早消失,被卡片底边裁掉)。
  out.set(parts.top, { translate: `0px ${top}px`, opacity: String(clamp01((p - 0.6) / 0.4)) });
  out.set(parts.ctl, { translate: `0px ${top * 0.9}px`, opacity: String(clamp01((p - 0.45) / 0.55)) });
  for (const el of parts.rest) out.set(el, { translate: `0px ${top}px`, opacity: String(clamp01((p - 0.5) / 0.5)) });
  return out;
}

/** apply 把某一帧直接写到元素上(跟手时用)。 */
export function apply(frames: Map<HTMLElement, Style>): void {
  for (const [el, style] of frames) Object.assign(el.style, style);
}

/** clear 去掉 apply 写上的样式。 */
export function clear(parts: MorphParts | null): void {
  if (!parts) return;
  for (const el of [parts.stage, parts.cover, parts.badge, parts.text, parts.top, parts.ctl, ...parts.rest]) {
    if (!el) continue;
    for (const k of ["translate", "scale", "opacity", "clip-path"]) el.style.removeProperty(k);
  }
}

/**
 * play 从进度 from 动到 to:按缓动曲线采样成关键帧,每个元素一组线性关键帧,
 * 这样 translate / scale / opacity 可以由合成线程播放。返回各元素的动画(第一个是卡片)。
 */
export function play(g: MorphGeo, from: number, to: number, duration: number, ease: (t: number) => number): Animation[] {
  const steps = 16;
  const frames = Array.from({ length: steps + 1 }, (_, i) => frame(g, from + (to - from) * ease(i / steps)));
  const els = [...(frames[0]?.keys() ?? [])];
  return els.map((el) =>
    el.animate(
      frames.map((f) => f.get(el) ?? {}),
      { duration, easing: "linear", fill: to === 0 ? "forwards" : "none" },
    ),
  );
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
