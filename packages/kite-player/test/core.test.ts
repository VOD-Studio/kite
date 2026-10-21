import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createKiteSource, KiteError } from "../src/core/source";
import { firstSungIndex, formatTime, lineIndexAt, wordProgress } from "../src/core/lyrics";
import { Player } from "../src/core/player";
import { Store } from "../src/core/store";
import type { Playlist } from "../src/core/types";
import { FakeAudio, fakeSource, flush, lyrics, track } from "./helpers";

const playlist: Playlist = {
  id: "100",
  name: "写作时听",
  cover: "https://p1.music.126.net/a.jpg",
  trackCount: 4,
  tracks: [track("1"), track("2"), track("3", { access: "vip", playable: false }), track("4")],
};

function newPlayer(storageKey: string | false = false) {
  const audio = new FakeAudio();
  const source = fakeSource(playlist);
  const player = new Player({ audio, source, storageKey, mediaSession: false });
  return { audio, source, player };
}

beforeEach(() => localStorage.clear());
afterEach(() => vi.restoreAllMocks());

describe("lyrics", () => {
  it("按时间找当前行", () => {
    expect(lineIndexAt(lyrics.lines, 0)).toBe(-1);
    expect(lineIndexAt(lyrics.lines, 1000)).toBe(0);
    expect(lineIndexAt(lyrics.lines, 6999)).toBe(1);
    expect(lineIndexAt(lyrics.lines, 99_000)).toBe(2);
  });

  it("逐字进度:有逐字时间按字,没有时按行时长均分", () => {
    const words = { time: 0, text: "你好", words: [{ time: 0, duration: 100, text: "你" }, { time: 100, duration: 100, text: "好" }] };
    expect(wordProgress(words, 150)).toEqual([1, 0.5]);
    expect(wordProgress({ time: 0, duration: 400, text: "四个字儿" }, 150)).toEqual([1, 0.5, 0, 0]);
  });

  it("预览跳过开头的署名行,正文里带冒号的不算", () => {
    const at = (texts: string[]) => firstSungIndex(texts.map((text, i) => ({ time: i * 1000, text })));
    expect(at(["作词 : 林间", "制作人 : 郑润泽", "风吹过来"])).toBe(2);
    expect(at(["风吹过来", "他说:等等"])).toBe(0);
    expect(at(["作曲 : 林间"])).toBe(0);
  });

  it("时间格式", () => {
    expect(formatTime(0)).toBe("0:00");
    expect(formatTime(214.9)).toBe("3:34");
    expect(formatTime(Number.NaN)).toBe("0:00");
  });
});

describe("store", () => {
  it("值没变时不通知", () => {
    const s = new Store({ a: 1, b: "x" });
    const fn = vi.fn();
    s.subscribe(fn);
    s.set({ a: 1 });
    expect(fn).not.toHaveBeenCalled();
    s.set({ a: 2 });
    expect(fn).toHaveBeenCalledTimes(1);
    expect(s.get()).toEqual({ a: 2, b: "x" });
  });
});

describe("createKiteSource", () => {
  it("把相对地址补成绝对地址,错误带 reason", async () => {
    const fetchMock = vi.fn(async (url: string) => {
      if (url.endsWith("/playlists/100")) {
        return new Response(JSON.stringify({ ...playlist, tracks: [{ ...track("1"), stream: "/player/v1/tracks/1/stream?x", lyrics: "/player/v1/tracks/1/lyrics" }] }));
      }
      return new Response(JSON.stringify({ error: { reason: "NOT_ALLOWED", message: "不在白名单" } }), { status: 404 });
    });
    const source = createKiteSource("https://music.example.com/", { fetch: fetchMock as unknown as typeof fetch });
    const pl = await source.playlist("100");
    expect(pl.tracks[0]?.stream).toBe("https://music.example.com/player/v1/tracks/1/stream?x");
    expect(fetchMock.mock.calls[0]?.[0]).toBe("https://music.example.com/player/v1/playlists/100");
    const err = await source.track("9").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(KiteError);
    expect((err as KiteError).reason).toBe("NOT_ALLOWED");
    expect((err as KiteError).message).toBe("不在白名单");
  });

  it("网络错误转成 NETWORK", async () => {
    const source = createKiteSource("https://x", { fetch: (async () => { throw new TypeError("fail"); }) as unknown as typeof fetch });
    await expect(source.info?.()).rejects.toMatchObject({ reason: "NETWORK" });
  });
});

describe("Player", () => {
  it("播放歌单:设队列、取歌词、开始播放", async () => {
    const { player, audio } = newPlayer();
    await player.playPlaylist("100");
    await flush();
    const s = player.state;
    expect(s.current?.id).toBe("1");
    expect(s.playlist).toEqual({ id: "100", name: "写作时听" });
    expect(s.playing).toBe(true);
    expect(audio.src).toBe(playlist.tracks[0]?.stream);
    expect(s.lyrics?.lines).toHaveLength(3);
  });

  it("下一首跳过不能播的 VIP 歌", async () => {
    const { player } = newPlayer();
    await player.playPlaylist("100", 1);
    player.next();
    expect(player.state.current?.id).toBe("4");
  });

  it("列表循环在末尾回到开头;顺序播放播完停下", async () => {
    const { player, audio } = newPlayer();
    await player.playPlaylist("100", 3);
    audio.emit("ended");
    expect(player.state.current?.id).toBe("1");

    player.setMode("sequence");
    await player.playPlaylist("100", 3);
    audio.emit("ended");
    expect(player.state.current?.id).toBe("4");
    expect(player.state.playing).toBe(false);
  });

  it("单曲循环播完重放同一首", async () => {
    const { player, audio } = newPlayer();
    await player.playPlaylist("100", 1);
    player.setMode("one");
    audio.currentTime = 199;
    audio.emit("ended");
    expect(player.state.current?.id).toBe("2");
    expect(audio.currentTime).toBe(0);
  });

  it("随机播放不会选到当前或不能播的歌", async () => {
    const { player } = newPlayer();
    await player.playPlaylist("100", 0);
    player.setMode("shuffle");
    for (let i = 0; i < 20; i++) {
      const before = player.state.current?.id;
      player.next();
      expect(player.state.current?.id).not.toBe(before);
      expect(player.state.current?.id).not.toBe("3");
    }
  });

  it("模式按 顺序 → 列表循环 → 单曲循环 → 随机 循环切换", () => {
    const { player } = newPlayer();
    player.setMode("sequence");
    expect([player.cycleMode(), player.cycleMode(), player.cycleMode(), player.cycleMode()]).toEqual(["loop", "one", "shuffle", "sequence"]);
  });

  it("上一首:播放超过 3 秒先回到开头", async () => {
    const { player, audio } = newPlayer();
    await player.playPlaylist("100", 1);
    audio.currentTime = 10;
    player.prev();
    expect(player.state.current?.id).toBe("2");
    expect(audio.currentTime).toBe(0);
    player.prev();
    expect(player.state.current?.id).toBe("1");
  });

  it("直接播放 VIP 歌:不发起请求,给出原因", async () => {
    const { player, audio } = newPlayer();
    await player.playPlaylist("100", 0);
    const srcBefore = audio.src;
    player.setQueue(playlist.tracks, 2, null, false);
    await player.play();
    expect(player.state.error?.reason).toBe("UNPLAYABLE");
    expect(player.state.error?.message).toContain("VIP");
    expect(audio.src).toBe(srcBefore);
  });

  it("播放出错:重新取一次播放链接并回到原位置,再失败才报错", async () => {
    const { player, audio, source } = newPlayer();
    await player.playPlaylist("100", 0);
    audio.currentTime = 42;
    audio.emit("error");
    await flush();
    expect(source.trackCalls).toEqual(["1"]);
    expect(audio.src).toContain("fresh=1");
    audio.emit("loadedmetadata");
    expect(audio.currentTime).toBe(42);
    audio.emit("error");
    await flush();
    expect(player.state.error?.reason).toBe("PLAYBACK_FAILED");
  });

  it("重取的链接恢复播放后,同一首歌之后再出错仍会重取", async () => {
    const { player, audio, source } = newPlayer();
    await player.playPlaylist("100", 0);
    audio.emit("error");
    await flush();
    audio.emit("loadedmetadata");
    audio.emit("playing");
    // 播放了一段之后(例如暂停太久)链接又过期
    audio.emit("error");
    await flush();
    expect(source.trackCalls).toEqual(["1", "1"]);
    expect(player.state.error).toBeNull();
  });

  it("插播单曲:放到当前歌后面", async () => {
    const { player } = newPlayer();
    await player.playPlaylist("100", 0);
    await player.playTrack(track("9", { title: "单曲" }));
    expect(player.state.current?.id).toBe("9");
    expect(player.state.queue.map((t) => t.id)).toEqual(["1", "9", "2", "3", "4"]);
  });

  it("从副歌开始", async () => {
    const { player, audio } = newPlayer();
    await player.playPlaylist("100", 0);
    await flush();
    player.playChorus();
    expect(audio.currentTime).toBe(62);
  });

  it("记住队列与进度,下一个页面恢复但不自动播放", async () => {
    const first = newPlayer("kite-test");
    await first.player.playPlaylist("100", 1);
    first.player.setMode("shuffle");
    first.audio.currentTime = 30;
    first.audio.emit("timeupdate");
    first.player.pause();
    first.player.setVolume(0.5);

    const audio = new FakeAudio();
    const next = new Player({ audio, source: fakeSource(playlist), storageKey: "kite-test", mediaSession: false });
    expect(next.state.current?.id).toBe("2");
    expect(next.state.mode).toBe("shuffle");
    expect(next.state.playing).toBe(false);
    expect(audio.volume).toBe(0.5);
    await next.play();
    audio.emit("loadedmetadata");
    expect(audio.currentTime).toBe(30);
  });

  it("恢复时还没有数据来源:setSource 之后补上歌词", async () => {
    const first = newPlayer("kite-test");
    await first.player.playPlaylist("100", 1);
    first.player.pause();

    // 页面上的元素先创建播放核心,api 属性稍后才设置数据来源。
    const next = new Player({ audio: new FakeAudio(), storageKey: "kite-test", mediaSession: false });
    expect(next.state.current?.id).toBe("2");
    expect(next.state.lyricsStatus).toBe("loading");
    next.setSource(fakeSource(playlist));
    await flush();
    expect(next.state.lyrics?.lines).toHaveLength(3);
    expect(next.state.lyricsStatus).toBe("ready");
  });

  it("歌词加载失败:标记 failed,不影响播放", async () => {
    const source = { ...fakeSource(playlist), lyrics: async () => Promise.reject(new Error("boom")) };
    const player = new Player({ audio: new FakeAudio(), source, storageKey: false, mediaSession: false });
    await player.playPlaylist("100");
    await flush();
    expect(player.state.playing).toBe(true);
    expect(player.state.lyrics).toBeNull();
    expect(player.state.lyricsStatus).toBe("failed");
    await expect(player.whenLyrics("1")).resolves.toBeNull();
  });

  it("localStorage 抛错时照常工作", async () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("quota");
    });
    const { player } = newPlayer("kite-test");
    await player.playPlaylist("100");
    expect(player.state.current?.id).toBe("1");
  });
});
