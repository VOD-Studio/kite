import { describe, expect, it } from "vitest";
import { cubicBezier, frame, type MorphGeo } from "../src/element/morph";

const el = () => document.createElement("div");

function geo(): MorphGeo {
  const parts = { stage: el(), cover: el(), badge: null, text: el(), top: el(), ctl: el(), rest: [el()] };
  return {
    parts,
    box: { left: 0, top: 0, width: 390, height: 844 },
    bar: { left: 10, top: 774, width: 370, height: 58 },
    radius: 0,
    disc: { left: 18, top: 782, width: 42, height: 42 },
    cover: { left: 24, top: 72, width: 342, height: 342 },
    coverOrigin: { x: 24, y: 72 },
    badge: null,
    text: { left: 24, top: 438, width: 342, height: 54 },
    barText: { left: 70, top: 783, width: 180, height: 21 },
    textScale: 14 / 22,
  };
}

describe("展开 / 收起的形变", () => {
  it("进度 1 是面板到位的样子,进度 0 时封面在唱片上、卡片缩成播放条", () => {
    const g = geo();
    const open = frame(g, 1);
    expect(open.get(g.parts.stage)?.clipPath).toBe("inset(0px 0px 0px 0px round 0px)");
    expect(open.get(g.parts.cover)).toEqual({ translate: "0px 0px", scale: "1" });
    expect(open.get(g.parts.top)?.opacity).toBe("1");

    const bar = frame(g, 0);
    expect(bar.get(g.parts.stage)?.clipPath).toBe("inset(774px 10px 12px 10px round 29px)");
    expect(bar.get(g.parts.stage)?.opacity).toBe("0");
    const cover = bar.get(g.parts.cover);
    expect(cover?.scale).toBe(String(42 / 342));
    expect(cover?.translate).toBe(`${18 - 24}px ${782 - 72}px`);
    expect(bar.get(g.parts.text as HTMLElement)?.translate).toBe(`${70 - 24}px ${783 - 438}px`);
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
