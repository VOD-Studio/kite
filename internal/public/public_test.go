package public

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/VOD-Studio/kite/config"
	merrors "github.com/VOD-Studio/kite/errors"
	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/cache"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// fakeCatalog 是内存里的上游:歌单、歌曲、歌词、播放直链(匿名与登录态分开)。
type fakeCatalog struct {
	mu         sync.Mutex
	playlists  map[int64]*mmpb.Playlist
	songs      map[int64]*mmpb.Song
	lyrics     map[int64]*mmpb.Lyric
	words      map[int64]*mmpb.WordLyric
	anonURL    map[int64]string
	ownerURL   map[int64]string
	lastCookie string
	calls      map[string]int
}

func newFakeCatalog() *fakeCatalog {
	free := song(1, "纸飞机", 0)
	vip := song(2, "雨季来信", 1)
	return &fakeCatalog{
		playlists: map[int64]*mmpb.Playlist{
			100: {Id: 100, Name: "写作时听", CoverUrl: "http://p1.music.126.net/a.jpg", TrackCount: 2, Songs: []*mmpb.Song{free, vip}},
			200: {Id: 200, Name: "不在白名单"},
		},
		songs:    map[int64]*mmpb.Song{1: free, 2: vip, 3: song(3, "单曲", 0), 9: song(9, "不在白名单", 0)},
		lyrics:   map[int64]*mmpb.Lyric{1: {Lrc: "[00:01.00]第一句\n[00:05.50]第二句\n", Translated: "[00:01.00]first\n[00:05.52]second\n"}},
		words:    map[int64]*mmpb.WordLyric{},
		anonURL:  map[int64]string{1: "http://m801.music.126.net/1.mp3", 3: "https://m801.music.126.net/3.mp3"},
		ownerURL: map[int64]string{1: "http://m801.music.126.net/1.mp3", 2: "https://m801.music.126.net/2-vip.flac"},
		calls:    map[string]int{},
	}
}

func song(id int64, name string, fee int32) *mmpb.Song {
	return &mmpb.Song{Id: id, Name: name, Fee: fee, DurationMs: 200000,
		Artists: []*mmpb.Artist{{Id: 7, Name: "林间"}},
		Album:   &mmpb.Album{Id: 8, Name: "晴朗的周三", PicUrl: "http://p1.music.126.net/c.jpg"}}
}

func (f *fakeCatalog) hit(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[name]++
}

func (f *fakeCatalog) Playlist(_ context.Context, id int64) (*mmpb.Playlist, error) {
	f.hit("playlist")
	if p, ok := f.playlists[id]; ok {
		return p, nil
	}
	return nil, merrors.ErrNotFound
}

func (f *fakeCatalog) Song(_ context.Context, id int64) (*mmpb.Song, error) {
	if s, ok := f.songs[id]; ok {
		return s, nil
	}
	return nil, merrors.ErrNotFound
}

func (f *fakeCatalog) Lyric(_ context.Context, id int64) (*mmpb.Lyric, error) {
	if l, ok := f.lyrics[id]; ok {
		return l, nil
	}
	return &mmpb.Lyric{}, nil
}

func (f *fakeCatalog) WordLyric(_ context.Context, id int64) (*mmpb.WordLyric, error) {
	if w, ok := f.words[id]; ok {
		return w, nil
	}
	return nil, errors.New("无逐字歌词")
}

func (f *fakeCatalog) Chorus(context.Context, int64) ([]*mmpb.ChorusSegment, error) {
	return []*mmpb.ChorusSegment{{StartMs: 62000, EndMs: 92000}}, nil
}

func (f *fakeCatalog) StreamURL(_ context.Context, id int64, _ mmpb.SongLevel, cookie string) (string, error) {
	f.mu.Lock()
	f.lastCookie = cookie
	f.mu.Unlock()
	if cookie != "" {
		return f.ownerURL[id], nil
	}
	return f.anonURL[id], nil
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func testOptions() Options {
	return Options{
		Playlists:     []int64{100},
		Songs:         []int64{3},
		Origins:       []string{"https://blog.example.com"},
		SigningKey:    []byte("0123456789abcdef0123456789abcdef"),
		RatePerMinute: 1000,
		Level:         mmpb.SongLevel_SONG_LEVEL_EXHIGH,
		LinkTTL:       time.Hour,
	}
}

func newTestServer(t *testing.T, opts Options) (*Server, *fakeCatalog, *clock, http.Handler) {
	t.Helper()
	cat := newFakeCatalog()
	c := &clock{t: time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)}
	s := newServer(cat, opts, c.now)
	return s, cat, c, s.Handler()
}

func get(t *testing.T, h http.Handler, path string, hdr ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &v), rec.Body.String())
	return v
}

func reason(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	return decode[errorDTO](t, rec).Error.Reason
}

func TestParseOptions(t *testing.T) {
	base := config.PublicConfig{Addr: ":3723", Playlists: "100, 200", RatePerMinute: 60, Quality: "exhigh"}

	opts, err := ParseOptions(base)
	require.NoError(t, err)
	require.Equal(t, []int64{100, 200}, opts.Playlists)
	require.Len(t, opts.SigningKey, 32, "未配置密钥文件时随机生成")
	require.Equal(t, mmpb.SongLevel_SONG_LEVEL_EXHIGH, opts.Level)

	noList := base
	noList.Playlists = ""
	_, err = ParseOptions(noList)
	require.ErrorContains(t, err, "白名单", "没有白名单必须拒绝启动")

	badID := base
	badID.Songs = "12,abc"
	_, err = ParseOptions(badID)
	require.ErrorContains(t, err, "abc")

	origins := base
	origins.Origins = "https://a.example.com/, *"
	opts, err = ParseOptions(origins)
	require.NoError(t, err)
	require.Equal(t, []string{"https://a.example.com"}, opts.Origins)
	require.True(t, opts.AnyOrigin)

	badOrigin := base
	badOrigin.Origins = "blog.example.com"
	_, err = ParseOptions(badOrigin)
	require.Error(t, err)

	badQuality := base
	badQuality.Quality = "best"
	_, err = ParseOptions(badQuality)
	require.Error(t, err)

	keyFile := filepath.Join(t.TempDir(), "key")
	require.NoError(t, os.WriteFile(keyFile, []byte("short\n"), 0o600))
	shortKey := base
	shortKey.SigningKeyFile = keyFile
	_, err = ParseOptions(shortKey)
	require.ErrorContains(t, err, "太短")
}

func TestSigner(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	s := signer{key: []byte("k0123456789abcdef"), ttl: time.Hour, now: c.now}
	q := s.streamQuery(42)
	parts := strings.Split(q, "&")
	exp := strings.TrimPrefix(parts[0], "exp=")
	sig := strings.TrimPrefix(parts[1], "sig=")

	require.NoError(t, s.verify(42, exp, sig))
	require.ErrorIs(t, s.verify(43, exp, sig), errLinkInvalid, "签名绑定歌曲 ID")
	require.ErrorIs(t, s.verify(42, exp+"0", sig), errLinkInvalid, "改过期时间会让签名失效")
	c.add(2 * time.Hour)
	require.ErrorIs(t, s.verify(42, exp, sig), errLinkExpired)
}

func TestLimiter(t *testing.T) {
	c := &clock{t: time.Unix(0, 0)}
	l := newLimiter(60, c.now)
	for range 60 {
		require.True(t, l.allow("1.2.3.4"))
	}
	require.False(t, l.allow("1.2.3.4"), "额度用完")
	require.True(t, l.allow("5.6.7.8"), "按 IP 分开计")
	c.add(2 * time.Second)
	require.True(t, l.allow("1.2.3.4"), "每秒回填一个")
}

func TestPlaylist(t *testing.T) {
	_, _, _, h := newTestServer(t, testOptions())

	rec := get(t, h, "/player/v1/playlists/100")
	require.Equal(t, http.StatusOK, rec.Code)
	pl := decode[playlistDTO](t, rec)
	require.Equal(t, "写作时听", pl.Name)
	require.Equal(t, "https://p1.music.126.net/a.jpg", pl.Cover, "封面换成 https")
	require.Len(t, pl.Tracks, 2)
	require.Equal(t, "1", pl.Tracks[0].ID)
	require.True(t, pl.Tracks[0].Playable)
	require.Equal(t, "free", pl.Tracks[0].Access)
	require.Equal(t, "vip", pl.Tracks[1].Access)
	require.False(t, pl.Tracks[1].Playable, "没有站长登录态时 VIP 歌不可播")
	require.Contains(t, pl.Tracks[0].Stream, "/player/v1/tracks/1/stream?exp=")

	rec = get(t, h, "/player/v1/playlists/200")
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "NOT_ALLOWED", reason(t, rec), "不在白名单的歌单不能读")
}

func TestTrackAllowlist(t *testing.T) {
	_, _, _, h := newTestServer(t, testOptions())

	require.Equal(t, http.StatusOK, get(t, h, "/player/v1/tracks/1").Code, "白名单歌单里的歌")
	require.Equal(t, http.StatusOK, get(t, h, "/player/v1/tracks/3").Code, "单独列出的歌")
	rec := get(t, h, "/player/v1/tracks/9")
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "NOT_ALLOWED", reason(t, rec))
	require.Equal(t, http.StatusNotFound, get(t, h, "/player/v1/tracks/9/lyrics").Code, "歌词同样受白名单限制")
	require.Equal(t, http.StatusBadRequest, get(t, h, "/player/v1/tracks/abc").Code)
}

func streamPath(t *testing.T, h http.Handler, track string) string {
	t.Helper()
	tr := decode[trackDTO](t, get(t, h, "/player/v1/tracks/"+track))
	return tr.Stream
}

func TestStream(t *testing.T) {
	_, cat, c, h := newTestServer(t, testOptions())

	rec := get(t, h, streamPath(t, h, "1"))
	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, "https://m801.music.126.net/1.mp3", rec.Header().Get("Location"), "直链换成 https")
	require.Empty(t, cat.lastCookie, "没有站长登录态时按匿名取链接")

	vip := streamPath(t, h, "1")
	vip = strings.Replace(vip, "/tracks/1/", "/tracks/2/", 1)
	require.Equal(t, "LINK_INVALID", reason(t, get(t, h, vip)), "签名不能挪给别的歌")

	rec = get(t, h, "/player/v1/tracks/1/stream")
	require.Equal(t, http.StatusForbidden, rec.Code)

	expired := streamPath(t, h, "1")
	c.add(3 * time.Hour)
	require.Equal(t, "LINK_EXPIRED", reason(t, get(t, h, expired)))
}

func TestStreamVIPWithOwner(t *testing.T) {
	opts := testOptions()
	opts.SessionFile = filepath.Join(t.TempDir(), "session.json")
	_, _, _, h := newTestServer(t, opts)

	// 没有登录态文件:VIP 歌不可播,并说明原因。
	pl := decode[playlistDTO](t, get(t, h, "/player/v1/playlists/100"))
	rec := get(t, h, pl.Tracks[1].Stream)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "UNPLAYABLE", reason(t, rec))
	require.Contains(t, decode[errorDTO](t, rec).Error.Message, "站长登录")

	// 写入登录态后重新创建服务(文件会被立即读取)。
	require.NoError(t, WriteSessionFile(opts.SessionFile, SessionFile{Cookie: "MUSIC_U=owner", UserID: 1, UpdatedAt: time.Now()}))
	_, cat, _, h := newTestServer(t, opts)
	require.Equal(t, loginActive, decode[infoDTO](t, get(t, h, "/player/v1/info")).Login)
	pl = decode[playlistDTO](t, get(t, h, "/player/v1/playlists/100"))
	require.True(t, pl.Tracks[1].Playable)
	rec = get(t, h, pl.Tracks[1].Stream)
	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, "https://m801.music.126.net/2-vip.flac", rec.Header().Get("Location"))
	require.Equal(t, "MUSIC_U=owner", cat.lastCookie, "有登录态时按站长账号取链接")
}

func TestStreamTarget(t *testing.T) {
	got, ok := streamTarget("http://m801.music.126.net/x.mp3?k=1")
	require.True(t, ok)
	require.Equal(t, "https://m801.music.126.net/x.mp3?k=1", got)
	for _, bad := range []string{"https://evil.example.com/x.mp3", "https://music.126.net.evil.com/x", "javascript:alert(1)", "https://u:p@m801.music.126.net/x"} {
		_, ok := streamTarget(bad)
		require.False(t, ok, bad)
	}
}

func TestCORS(t *testing.T) {
	_, _, _, h := newTestServer(t, testOptions())

	rec := get(t, h, "/player/v1/info", "Origin", "https://blog.example.com")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "https://blog.example.com", rec.Header().Get("Access-Control-Allow-Origin"))

	rec = get(t, h, "/player/v1/info", "Origin", "https://evil.example.com")
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, "ORIGIN_NOT_ALLOWED", reason(t, rec))

	req := httptest.NewRequest(http.MethodOptions, "/player/v1/playlists/100", nil)
	req.Header.Set("Origin", "https://blog.example.com")
	pre := httptest.NewRecorder()
	h.ServeHTTP(pre, req)
	require.Equal(t, http.StatusNoContent, pre.Code)
	require.Contains(t, pre.Header().Get("Access-Control-Allow-Methods"), "GET")

	require.Equal(t, http.StatusOK, get(t, h, "/player/v1/info").Code, "<audio> 等不带 Origin 的请求不受影响")
}

func TestRateLimit(t *testing.T) {
	opts := testOptions()
	opts.RatePerMinute = 2
	_, _, _, h := newTestServer(t, opts)
	require.Equal(t, http.StatusOK, get(t, h, "/player/v1/info").Code)
	require.Equal(t, http.StatusOK, get(t, h, "/player/v1/info").Code)
	rec := get(t, h, "/player/v1/info")
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Equal(t, "RATE_LIMITED", reason(t, rec))
	require.Equal(t, http.StatusOK, get(t, h, "/healthz").Code, "健康检查不限流")
}

func TestCachesPlaylist(t *testing.T) {
	_, cat, c, h := newTestServer(t, testOptions())
	get(t, h, "/player/v1/playlists/100")
	get(t, h, "/player/v1/playlists/100")
	get(t, h, "/player/v1/tracks/1")
	require.Equal(t, 1, cat.calls["playlist"], "歌单与白名单校验共用缓存")
	c.add(playlistTTL + time.Second)
	get(t, h, "/player/v1/playlists/100")
	require.Equal(t, 2, cat.calls["playlist"], "过期后重新读取")
}

func TestLyrics(t *testing.T) {
	_, cat, _, h := newTestServer(t, testOptions())

	rec := get(t, h, "/player/v1/tracks/1/lyrics")
	require.Equal(t, http.StatusOK, rec.Code)
	ly := decode[lyricsDTO](t, rec)
	require.False(t, ly.Instrumental)
	require.Len(t, ly.Lines, 2)
	require.Equal(t, lineDTO{Time: 1000, Duration: 4500, Text: "第一句", Translation: "first"}, ly.Lines[0])
	require.Equal(t, "second", ly.Lines[1].Translation, "翻译时间差几十毫秒也能对齐")
	require.Equal(t, []segmentDTO{{Start: 62000, End: 92000}}, ly.Chorus)

	cat.words[3] = &mmpb.WordLyric{Lines: []*mmpb.WordLyricLine{{StartMs: 500, DurationMs: 900, Content: "单曲",
		Words: []*mmpb.WordLyricWord{{StartMs: 500, DurationMs: 400, Content: "单"}, {StartMs: 900, DurationMs: 500, Content: "曲"}}}}}
	ly = decode[lyricsDTO](t, get(t, h, "/player/v1/tracks/3/lyrics"))
	require.Len(t, ly.Lines, 1)
	require.Len(t, ly.Lines[0].Words, 2, "有逐字歌词时带每个字的时间")
}

func TestParseLRC(t *testing.T) {
	lines := parseLRC("[ar:林间]\n{\"t\":0,\"c\":[{\"tx\":\"作词\"}]}\n[00:10.5]a\n[01:02.345][00:03.00]b\n[00:04:20]c")
	require.Equal(t, []lrcLine{{3000, "b"}, {4200, "c"}, {10500, "a"}, {62345, "b"}}, lines)

	inst := buildLyrics(&mmpb.Lyric{Lrc: "[00:00.00]纯音乐,请欣赏"}, nil, nil)
	require.True(t, inst.Instrumental, "只有提示行的歌按纯音乐处理")
	require.Empty(t, inst.Lines)
	require.True(t, buildLyrics(&mmpb.Lyric{}, nil, nil).Instrumental, "没有歌词视为纯音乐")
}

func TestOwnerRefresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	require.NoError(t, WriteSessionFile(path, SessionFile{Cookie: "MUSIC_U=old; __csrf=x", UserID: 1}))
	c := &clock{t: time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)}
	o := newOwnerSession(path, c.now)
	require.Equal(t, loginActive, o.state())

	renew := func(ctx context.Context, _ engine.Meta, _ map[string]any) (json.RawMessage, string, error) {
		require.Equal(t, "MUSIC_U=old; __csrf=x", engine.CookieFromContext(ctx))
		return json.RawMessage(`{"code":200}`), "MUSIC_U=new", nil
	}
	require.NoError(t, o.refresh(context.Background(), renew))
	f, err := ReadSessionFile(path)
	require.NoError(t, err)
	require.Contains(t, f.Cookie, "MUSIC_U=new", "续期后写回新 cookie")
	require.Contains(t, f.Cookie, "__csrf=x", "保留没有被下发覆盖的字段")
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	expire := func(context.Context, engine.Meta, map[string]any) (json.RawMessage, string, error) {
		return nil, "", merrors.ErrUnauthorized
	}
	require.ErrorIs(t, o.refresh(context.Background(), expire), merrors.ErrUnauthorized)
	require.Equal(t, loginExpired, o.state())
	require.Empty(t, o.cookie(), "失效后按匿名播放")
}

// engineCatalog 取播放链接时只带注入的 cookie:有登录态时发给上游,匿名时不带 MUSIC_U。
func TestEngineCatalogStreamCookie(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Cookie")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"data":[{"id":1,"url":"http://m801.music.126.net/1.mp3","br":320000,"size":1,"type":"mp3"}]}`))
	}))
	t.Cleanup(srv.Close)
	cat := engineCatalog{eng: engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(srv.URL))}

	url, err := cat.StreamURL(context.Background(), 1, mmpb.SongLevel_SONG_LEVEL_EXHIGH, "MUSIC_U=owner")
	require.NoError(t, err)
	require.Equal(t, "http://m801.music.126.net/1.mp3", url)
	require.Contains(t, got, "MUSIC_U=owner")

	_, err = cat.StreamURL(context.Background(), 1, mmpb.SongLevel_SONG_LEVEL_EXHIGH, "")
	require.NoError(t, err)
	require.NotContains(t, got, "MUSIC_U")
}
