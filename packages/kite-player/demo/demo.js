// kite-player 演示页的数据与音频。
//
// 默认离线运行:示例歌单 / 歌曲 / 歌词都在这里,封面是 covers/ 下的插画,播放用按真实时钟推进的模拟音频,
// 不访问任何服务、不放真实音乐。地址里带 ?api= 时改为连接真实的 kite-server 公开模式:
//   index.html?api=http://127.0.0.1:3793&playlists=歌单ID,歌单ID&songs=歌曲ID,歌曲ID,歌曲ID,歌曲ID&playlist=歌单ID,歌单ID
// 必须在 kite-player.iife.js 之后、页面里的元素出现之前执行(放在 <head> 里)。
(function () {
  "use strict";
  const params = new URLSearchParams(location.search);
  const api = params.get("api");

  if (api) {
    KitePlayer.getPlayer({ source: KitePlayer.createKiteSource(api), storageKey: "kite-player-demo" });
    // 把地址里的 ID 依次填进页面上的元素。
    document.addEventListener("DOMContentLoaded", function () {
      const fill = (selector, attr, list) => {
        const ids = (list || "").split(",").filter(Boolean);
        document.querySelectorAll(selector).forEach((el, i) => ids[i % ids.length] && el.setAttribute(attr, ids[i % ids.length]));
      };
      if (params.get("playlists")) document.querySelector("kite-player").setAttribute("playlists", params.get("playlists"));
      fill("kite-track", "song", params.get("songs"));
      fill("kite-playlist", "playlist", params.get("playlist") || params.get("playlists"));
      document.documentElement.dataset.live = "";
    });
    return;
  }

  // ---- 示例数据(歌名、歌手、歌词都是虚构的) ----

  const cover = (name) => new URL(`covers/${name}.svg`, document.baseURI).href;
  const lyricSets = {
    paper: {
      lines: [
        ["把今天折成一只纸飞机", "Fold today into a paper plane"],
        ["从窗口放出去", "and let it out the window"],
        ["风会记得它的名字", "the wind will remember its name"],
        ["也记得我们的", "and remember ours too"],
        ["云在下午慢慢地走", "clouds drift through the afternoon"],
        ["影子落在你的肩上", "a shadow rests on your shoulder"],
        ["我们不说再见", "we never say goodbye"],
        ["只说明天见", "only see you tomorrow"],
        ["纸飞机 往远处飞", "paper plane, fly far away"],
        ["带着没说出口的话", "carry the words we never said"],
        ["纸飞机 往远处飞", "paper plane, fly far away"],
        ["落在有光的地方", "land somewhere full of light"],
        ["飞过街角的书店", "past the bookshop on the corner"],
        ["飞过没写完的信", "past the letter left unfinished"],
        ["如果它落在你窗前", "if it lands outside your window"],
        ["请替我打开它", "please open it for me"],
        ["纸飞机 往远处飞", "paper plane, fly far away"],
        ["落在有光的地方", "land somewhere full of light"],
      ],
      chorus: [8, 12],
    },
    sea: {
      lines: [
        ["海边的风吹了一整夜"],
        ["把沙子吹成新的形状"],
        ["你站在浪退去的地方"],
        ["数着远处亮起的灯"],
        ["潮水来了又走"],
        ["像一句没讲完的话"],
        ["我把耳机分你一半"],
        ["歌里也有海的声音"],
        ["海边的风 吹散了夏天"],
        ["吹不散你的笑"],
        ["海边的风 一直吹"],
        ["吹到天亮"],
        ["回去的路有点长"],
        ["我们走得很慢"],
        ["海边的风 吹散了夏天"],
        ["吹不散你的笑"],
      ],
      chorus: [8, 12],
    },
    train: {
      lines: [
        ["慢车开过麦田"],
        ["窗外的风是绿色的"],
        ["我们把时间放在膝上"],
        ["一站一站数过去"],
        ["小站的灯亮了又灭"],
        ["有人下车 有人上来"],
        ["你靠着窗睡着了"],
        ["梦里也在摇晃"],
        ["慢一点 再慢一点"],
        ["让黄昏追上我们"],
        ["慢一点 再慢一点"],
        ["终点不用那么快"],
        ["铁轨唱着旧旧的歌"],
        ["云跟着我们跑"],
        ["慢一点 再慢一点"],
        ["终点不用那么快"],
      ],
      chorus: [8, 12],
    },
    bike: {
      lines: [
        ["单车穿过七月的街"],
        ["影子比我们先到"],
        ["汽水在车筐里摇晃"],
        ["冒着细细的泡"],
        ["你说风是甜的"],
        ["我说是西瓜味"],
        ["下坡的时候别刹车"],
        ["让夏天追不上"],
        ["单车与夏天 一路往前"],
        ["头发里都是阳光"],
        ["单车与夏天 一路往前"],
        ["把烦恼甩在后面"],
        ["蝉声铺满了整条路"],
        ["我们骑到天黑"],
        ["单车与夏天 一路往前"],
        ["把烦恼甩在后面"],
      ],
      chorus: [8, 12],
    },
    lamp: {
      lines: [
        ["夜灯还亮着"],
        ["书翻到第三十页"],
        ["窗外的车一辆一辆"],
        ["把城市的声音带走"],
        ["我想起你说过"],
        ["慢一点也没关系"],
        ["夜灯还亮着 照着没睡的人"],
        ["照着桌上那杯凉掉的茶"],
        ["夜灯还亮着 照着没睡的人"],
        ["照着明天要去的地方"],
        ["钟声敲过了十二点"],
        ["梦在门外等着"],
        ["夜灯还亮着 照着没睡的人"],
        ["照着明天要去的地方"],
      ],
      chorus: [6, 10],
    },
  };

  /** 把歌词文本排成带时间的行:前奏 8 秒,每句约 9 秒;没有逐字时间时播放器按整句时长均分。 */
  function makeLyrics(set, durationMs) {
    if (!set) return { lines: [], chorus: [], instrumental: true };
    const step = Math.min(9000, (durationMs - 14000) / set.lines.length);
    const lines = set.lines.map(([text, translation], i) => ({ time: 8000 + i * step, duration: step - 900, text, translation }));
    const [a, b] = set.chorus;
    return { lines, chorus: [{ start: lines[a].time, end: lines[b].time }], instrumental: false };
  }

  const tracks = {};
  function track(id, title, artist, album, art, minutes, lyricSet, access) {
    const durationMs = Math.round(minutes * 60000);
    tracks[id] = {
      id,
      title,
      artists: [{ id: artist, name: artist }],
      album: { id: album, name: album, cover: cover(art) },
      durationMs,
      access: access || "free",
      playable: !access,
      stream: `demo:${id}`,
      lyrics: `demo:${id}`,
      lyricsData: makeLyrics(lyricSets[lyricSet], durationMs),
    };
    return tracks[id];
  }

  const playlists = {
    p1: {
      id: "p1",
      name: "午后的歌",
      cover: cover("paper"),
      tracks: [
        track("t1", "纸飞机", "林间", "晴朗的周三", "paper", 3.2, "paper"),
        track("t2", "海边的风", "北岛与海", "海边的风", "sea", 3.55, "sea"),
        track("t3", "慢车", "小满", "慢车", "train", 4.1, "train"),
        track("t4", "单车与夏天", "白日梦乐队", "单车与夏天", "bike", 3.4, "bike"),
        track("t7", "雨季来信", "林间", "晴朗的周三", "paper", 3.9, "paper", "vip"),
        track("t5", "夜灯", "慢鱼", "夜灯", "lamp", 4.3, "lamp"),
        track("t8", "晴朗的周三", "林间", "晴朗的周三", "paper", 2.6, null),
        track("t6", "星群", "夏至", "星群", "night", 3.7, "paper", "paid"),
      ],
    },
    p2: {
      id: "p2",
      name: "夜里听",
      cover: cover("night"),
      tracks: [],
    },
  };
  playlists.p2.tracks = ["t5", "t6", "t3", "t2", "t1"].map((id) => tracks[id]);
  for (const pl of Object.values(playlists)) pl.trackCount = pl.tracks.length;

  const strip = ({ lyricsData, ...t }) => t;
  const source = {
    playlist: async (id) => {
      const pl = playlists[id];
      if (!pl) throw new Error("示例里没有这个歌单");
      return { ...pl, tracks: pl.tracks.map(strip) };
    },
    track: async (id) => {
      if (!tracks[id]) throw new Error("示例里没有这首歌");
      return strip(tracks[id]);
    },
    lyrics: async (t) => tracks[t.id].lyricsData,
    info: async () => ({ version: "demo", login: "none", quality: "exhigh" }),
  };

  /** DemoAudio 实现播放核心需要的 AudioLike:按真实时钟推进,派发与 <audio> 相同的事件。 */
  class DemoAudio {
    constructor() {
      this._src = "";
      this._base = 0;
      this._start = 0;
      this._playing = false;
      this._duration = NaN;
      this._listeners = {};
      this._timer = 0;
      this.volume = 1;
      this.preload = "none";
    }
    get src() {
      return this._src;
    }
    set src(value) {
      this._halt();
      this._src = value;
      this._base = 0;
      const t = tracks[String(value).replace(/^demo:/, "")];
      this._duration = t ? t.durationMs / 1000 : NaN;
      setTimeout(() => ["loadedmetadata", "durationchange", "canplay"].forEach((e) => this._emit(e)), 20);
    }
    get duration() {
      return this._duration;
    }
    get paused() {
      return !this._playing;
    }
    get currentTime() {
      if (!this._playing) return this._base;
      return Math.min(this._duration || Infinity, this._base + (performance.now() - this._start) / 1000);
    }
    set currentTime(value) {
      this._base = Math.max(0, Math.min(Number(value) || 0, this._duration || Infinity));
      this._start = performance.now();
      this._emit("timeupdate");
    }
    play() {
      if (!this._src) return Promise.reject(new Error("没有音源"));
      if (!this._playing) {
        this._start = performance.now();
        this._playing = true;
        this._timer = setInterval(() => this._tick(), 250);
        this._emit("play");
        this._emit("playing");
      }
      return Promise.resolve();
    }
    pause() {
      if (!this._playing) return;
      this._halt();
      this._emit("pause");
    }
    addEventListener(type, fn) {
      (this._listeners[type] = this._listeners[type] || new Set()).add(fn);
    }
    removeEventListener(type, fn) {
      if (this._listeners[type]) this._listeners[type].delete(fn);
    }
    _halt() {
      if (!this._playing) return;
      this._base = this.currentTime;
      this._playing = false;
      clearInterval(this._timer);
    }
    _tick() {
      this._emit("timeupdate");
      if (this._duration && this.currentTime >= this._duration) {
        this.pause();
        this._base = this._duration;
        this._emit("ended");
      }
    }
    _emit(type) {
      for (const fn of [...(this._listeners[type] || [])]) fn();
    }
  }

  KitePlayer.getPlayer({ source, audio: new DemoAudio(), storageKey: false });
})();
