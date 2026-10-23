import { describe, expect, it } from "vitest";
import { cubicBezier, frame, type MorphGeo } from "../src/element/morph";

const el = () => document.createElement("div");
const at = (left: number, top: number, width: number, height: number) => ({ left, top, width, height });
const item = (box: ReturnType<typeof at>, fade: [number, number] = [0, 1]) => ({ el: el(), rect: box, o: { x: box.left, y: box.top }, fade });

function geo(): MorphGeo {
  const cover = { ...item(at(24, 72, 342, 342)), radius: 16, own: 1 };
  const meta = item(at(24, 438, 342, 54), [0.55, 0.35]);
  const ctl = item(at(24, 640, 342, 180), [0.62, 0.33]);
  const miniText = item(at(60, 9, 202, 40));
  const miniCtl = item(at(254, 9, 120, 40));
  const top = item(at(10, 10, 370, 40));
  return {
    parts: {
      stage: el(),
      shadow: null,
      cover: cover.el,
      badge: null,
      miniText: miniText.el,
      miniCtl: miniCtl.el,
      top: top.el,
      below: [meta, ctl],
      beside: null,
      rest: [],
    },
    box: at(0, 0, 390, 844),
    bar: at(10, 774, 370, 58),
    radius: 0,
    midRadius: 18,
    disc: at(18, 782, 42, 42),
    discAngle: 90,
    cover,
    badge: null,
    miniText,
    miniCtl,
    barText: at(70, 783, 202, 40),
    barCtl: at(264, 783, 108, 40),
    top,
    below: [meta, ctl],
    beside: null,
    rest: [],
    anchor: "card",
    drop: 84.4,
  };
}

describe("展开 / 收起的形变", () => {
  it("进度 1 是面板到位的样子", () => {
    const g = geo();
    const open = frame(g, 1);
    expect(open.get(g.parts.stage)?.clipPath).toBe("inset(0px 0px 0px 0px round 0px)");
    expect(open.get(g.cover.el)).toMatchObject({ translate: "0px 0px", scale: "1", rotate: "0deg", borderRadius: "16px" });
    expect(open.get(g.top.el)?.opacity).toBe("1");
    for (const it of g.below) expect(open.get(it.el)).toMatchObject({ translate: "0px 0px", scale: "1", opacity: "1" });
    expect(open.get(g.miniText!.el)?.opacity).toBe("0");
  });

  it("进度 0 时卡片是播放条,封面是唱片(圆、转着),播放条内容在原位", () => {
    const g = geo();
    const bar = frame(g, 0);
    expect(bar.get(g.parts.stage)).toMatchObject({ clipPath: "inset(774px 10px 12px 10px round 29px)", opacity: "0" });
    const cover = bar.get(g.cover.el);
    expect(cover?.scale).toBe(String(42 / 342));
    expect(cover?.rotate).toBe("90deg");
    // 圆角是唱片半径(换算到缩放前)。
    expect(parseFloat(cover?.borderRadius ?? "")).toBeCloseTo(21 / (42 / 342), 3);
    expect(bar.get(g.miniText!.el)).toMatchObject({ translate: "10px 774px", opacity: "1" });
    expect(bar.get(g.below[0]!.el)?.opacity).toBe("0");
  });

  it("封面先贴着卡片顶边,卡片够高后才下移;下方内容跟着封面底边", () => {
    const g = geo();
    const p = 0.15;
    const f = frame(g, p);
    const cardTop = 774 + (0 - 774) * p;
    const k = (42 + (342 - 42) * p) / 342;
    // 不转时封面左上角 = translate(绕中心转的补偿在 p ≥ 0.12 后为 0)。
    const [, ty] = (f.get(g.cover.el)?.translate ?? "").split(" ").map(parseFloat);
    expect(ty! - (782 - 774) - cardTop + 72).toBeCloseTo(0, 6);
    expect(f.get(g.cover.el)?.rotate).toBe("0deg");
    expect(Number(f.get(g.cover.el)?.scale)).toBeCloseTo(k, 6);
    // 卡片一离开播放条就不透明。
    expect(f.get(g.parts.stage)?.opacity).toBe("1");
  });

  it("缓动与 CSS 的 cubic-bezier 一致", () => {
    const linear = cubicBezier(0, 0, 1, 1);
    expect(linear(0.3)).toBeCloseTo(0.3, 3);
    const ease = cubicBezier(0.25, 0.1, 0.25, 1);
    expect(ease(0.5)).toBeCloseTo(0.8024, 3);
    expect(ease(0)).toBe(0);
    expect(ease(1)).toBe(1);
  });
});
