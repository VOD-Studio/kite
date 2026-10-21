package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	merrors "github.com/VOD-Studio/kite/errors"
	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/cache"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// upstream 是按路径路由的假网易云。weapi 请求体是加密的,看不到 offset 等参数,
// 所以歌曲详情返回歌单里的全部歌曲,由 Details 按请求的 ID 挑出这一页;
// 用户歌单按调用次数依次返回各页。
type upstream struct {
	t         *testing.T
	detail    []byte   // /weapi/v6/playlist/detail
	songs     []byte   // /weapi/v3/song/detail
	userPages [][]byte // /weapi/user/playlist,按调用次数依次返回

	mu      sync.Mutex
	cookies map[string][]string // path → 每次请求的 Cookie 头
	pageHit atomic.Int32
}

func (u *upstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u.mu.Lock()
	u.cookies[r.URL.Path] = append(u.cookies[r.URL.Path], r.Header.Get("Cookie"))
	u.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/weapi/v6/playlist/detail":
		_, _ = w.Write(u.detail)
	case "/weapi/v3/song/detail":
		_, _ = w.Write(u.songs)
	case "/weapi/user/playlist":
		i := int(u.pageHit.Add(1)) - 1
		_, _ = w.Write(u.userPages[min(i, len(u.userPages)-1)])
	default:
		u.t.Errorf("unexpected upstream path %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}

func (u *upstream) calls(path string) []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.cookies[path]...)
}

// poolStore 是存着一个「站点账号」的会话池:组合读取任何时候都不该借用它。
type poolStore struct{ asked atomic.Int32 }

func (p *poolStore) GetAvailable(context.Context, session.AuthRequirement) (*session.Session, error) {
	p.asked.Add(1)
	return &session.Session{UserID: "site", Cookie: "MUSIC_U=SITE_ACCOUNT"}, nil
}
func (p *poolStore) ReportSuccess(string)                         {}
func (p *poolStore) ReportFailure(string, error)                  {}
func (p *poolStore) Save(context.Context, *session.Session) error { return nil }
func (p *poolStore) ListAll(context.Context) ([]string, error)    { return nil, nil }

func newEngine(t *testing.T, u *upstream) (*engine.Engine, *poolStore) {
	t.Helper()
	u.t = t
	u.cookies = map[string][]string{}
	srv := httptest.NewServer(u)
	t.Cleanup(srv.Close)
	pool := &poolStore{}
	return engine.New(engine.WithBaseURL(srv.URL), engine.WithCache(cache.Noop{}), engine.WithSessions(pool)), pool
}

// playlistUpstream 用真实抓取的歌单详情(71 首,内嵌 tracks 只有 10 首)。
func playlistUpstream(t *testing.T) (*upstream, []int64) {
	t.Helper()
	detail, err := os.ReadFile("../endpoint/playlist/testdata/playlist_detail_v6_truncated.json")
	require.NoError(t, err)
	var d struct {
		Playlist struct {
			TrackIDs []struct {
				ID int64 `json:"id"`
			} `json:"trackIds"`
		} `json:"playlist"`
	}
	require.NoError(t, json.Unmarshal(detail, &d))
	ids := make([]int64, len(d.Playlist.TrackIDs))
	songs := make([]string, len(ids))
	for i, tr := range d.Playlist.TrackIDs {
		ids[i] = tr.ID
		songs[i] = fmt.Sprintf(`{"id":%d,"name":"song-%d","ar":[],"al":{},"dt":1000}`, tr.ID, tr.ID)
	}
	return &upstream{detail: detail, songs: []byte(`{"code":200,"songs":[` + strings.Join(songs, ",") + `]}`)}, ids
}

func songIDs(resp *mmpb.AllTracksResponse) []int64 {
	out := make([]int64, len(resp.GetSongs()))
	for i, s := range resp.GetSongs() {
		out[i] = s.GetId()
	}
	return out
}

// issue #89:offset 以前不生效(对截断的内嵌 tracks 切片,两页都是前 3 首)。
func TestPlaylistTracks_PagesFollowTrackIDs(t *testing.T) {
	t.Parallel()
	u, ids := playlistUpstream(t)
	eng, _ := newEngine(t, u)
	ctx := context.Background()

	var walked []int64
	for offset := int32(0); offset < 9; offset += 3 {
		resp, err := PlaylistTracks(ctx, eng, &mmpb.AllTracksRequest{PlaylistId: 2219770152, Limit: 3, Offset: offset})
		require.NoError(t, err)
		require.Equal(t, int32(71), resp.GetTotal(), "total 是歌单总曲目数,不是当前页长度")
		require.Equal(t, ids[offset:offset+3], songIDs(resp))
		walked = append(walked, songIDs(resp)...)
	}
	require.Equal(t, ids[:9], walked, "连续三页不重复、不跳过,保持歌单顺序")

	// 超出内嵌 tracks(10 首)的部分也能取到。
	resp, err := PlaylistTracks(ctx, eng, &mmpb.AllTracksRequest{PlaylistId: 2219770152, Limit: 30, Offset: 30})
	require.NoError(t, err)
	require.Equal(t, ids[30:60], songIDs(resp))
}

func TestPlaylistTracks_LastAndEmptyPages(t *testing.T) {
	t.Parallel()
	u, ids := playlistUpstream(t)
	eng, _ := newEngine(t, u)
	ctx := context.Background()

	last, err := PlaylistTracks(ctx, eng, &mmpb.AllTracksRequest{PlaylistId: 1, Limit: 5, Offset: 69})
	require.NoError(t, err)
	require.Equal(t, ids[69:], songIDs(last), "末页只有剩下的 2 首")

	before := len(u.calls("/weapi/v3/song/detail"))
	empty, err := PlaylistTracks(ctx, eng, &mmpb.AllTracksRequest{PlaylistId: 1, Limit: 5, Offset: 100})
	require.NoError(t, err)
	require.Empty(t, empty.GetSongs())
	require.Equal(t, int32(71), empty.GetTotal())
	require.Len(t, u.calls("/weapi/v3/song/detail"), before, "越界空页不请求歌曲详情")

	all, err := PlaylistTracks(ctx, eng, &mmpb.AllTracksRequest{PlaylistId: 1})
	require.NoError(t, err)
	require.Equal(t, ids, songIDs(all), "未指定 limit 时默认 100,71 首全部返回")
}

// userPlaylistsUpstream:两页上游数据。第一页额外带「喜欢的音乐」(与真实上游一致),
// 用户 7 创建 3 张(含喜欢的音乐)、收藏 2 张。
func userPlaylistsUpstream() *upstream {
	return &upstream{userPages: [][]byte{
		[]byte(`{"code":200,"more":true,"playlist":[
			{"id":100,"name":"喜欢的音乐","specialType":5,"creator":{"userId":7}},
			{"id":101,"name":"创建 A","creator":{"userId":7}},
			{"id":201,"name":"收藏 X","creator":{"userId":9}}]}`),
		[]byte(`{"code":200,"more":false,"playlist":[
			{"id":102,"name":"创建 B","creator":{"userId":7}},
			{"id":202,"name":"收藏 Y","creator":{"userId":8}}]}`),
	}}
}

func playlistIDs(resp *mmpb.UserPlaylistResponse) []int64 {
	out := make([]int64, len(resp.GetPlaylists()))
	for i, p := range resp.GetPlaylists() {
		out[i] = p.GetId()
	}
	return out
}

// issue #88:total 以前是当前页长度,随 limit / offset 变化;筛选又在上游分页之后做。
func TestUserPlaylists_TotalAndPagingPerFilter(t *testing.T) {
	t.Parallel()
	cases := []struct {
		filter mmpb.PlaylistFilter
		want   []int64
	}{
		{mmpb.PlaylistFilter_PLAYLIST_FILTER_ALL, []int64{100, 101, 201, 102, 202}},
		{mmpb.PlaylistFilter_PLAYLIST_FILTER_CREATED, []int64{100, 101, 102}},
		{mmpb.PlaylistFilter_PLAYLIST_FILTER_SUBSCRIBED, []int64{201, 202}},
	}
	for _, c := range cases {
		t.Run(c.filter.String(), func(t *testing.T) {
			t.Parallel()
			for _, limit := range []int32{1, 2, 3} {
				var walked []int64
				for offset := int32(0); ; offset += limit {
					// 每次都是一次独立的读取:上游从第一页重新给。
					u := userPlaylistsUpstream()
					eng, _ := newEngine(t, u)
					resp, err := UserPlaylists(context.Background(), eng, &mmpb.UserPlaylistRequest{
						UserId: 7, Filter: c.filter, Limit: limit, Offset: offset,
					})
					require.NoError(t, err)
					require.Equal(t, int32(len(c.want)), resp.GetTotal(), "limit=%d offset=%d", limit, offset)
					require.LessOrEqual(t, len(resp.GetPlaylists()), int(limit))
					if len(resp.GetPlaylists()) == 0 {
						require.GreaterOrEqual(t, int(offset), len(c.want), "只有越界才是空页")
						break
					}
					walked = append(walked, playlistIDs(resp)...)
				}
				require.Equal(t, c.want, walked, "limit=%d 完整遍历不重复、不跳过", limit)
			}
		})
	}
}

func TestUserPlaylists_FollowsMoreAndDedupes(t *testing.T) {
	t.Parallel()
	u := userPlaylistsUpstream()
	// 第二页重复了第一页的一张(上游翻页时列表有变动),按 ID 去重。
	u.userPages[1] = []byte(`{"code":200,"more":false,"playlist":[
		{"id":101,"name":"创建 A","creator":{"userId":7}},
		{"id":102,"name":"创建 B","creator":{"userId":7}}]}`)
	eng, _ := newEngine(t, u)

	resp, err := UserPlaylists(context.Background(), eng, &mmpb.UserPlaylistRequest{UserId: 7, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, []int64{100, 101, 201, 102}, playlistIDs(resp))
	require.Equal(t, int32(2), u.pageHit.Load(), "more=false 后不再翻页")
}

// 取满页数上限时上游仍说还有,返回错误而不是把不完整的列表当成完整结果(否则 total 偏小、后面的页是空的)。
func TestUserPlaylists_PageCapIsAnError(t *testing.T) {
	t.Parallel()
	u := &upstream{}
	for i := range maxPlaylistPages {
		u.userPages = append(u.userPages, []byte(fmt.Sprintf(`{"code":200,"more":true,"playlist":[{"id":%d,"creator":{"userId":7}}]}`, 1000+i)))
	}
	eng, _ := newEngine(t, u)

	_, err := UserPlaylists(context.Background(), eng, &mmpb.UserPlaylistRequest{UserId: 7})
	require.ErrorIs(t, err, merrors.ErrInvalidResponse)
	require.Equal(t, int32(maxPlaylistPages), u.pageHit.Load())
}

// issue #90:带了调用方 cookie 的读取用它,没带的匿名;任何情况下都不借用会话池里的站点账号。
func TestComposedReads_UseCallerCookieNeverThePool(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		cookie string
	}{
		{"账号 A", "MUSIC_U=ACCOUNT_A"},
		{"账号 B", "MUSIC_U=ACCOUNT_B"},
		{"匿名", ""},
	}
	u, _ := playlistUpstream(t)
	u.userPages = userPlaylistsUpstream().userPages
	eng, pool := newEngine(t, u)

	// 交错执行:A、B、匿名、再 A。
	for _, c := range append(cases, cases[0]) {
		ctx := engine.WithCookie(context.Background(), c.cookie)
		u.pageHit.Store(0)
		_, err := UserPlaylists(ctx, eng, &mmpb.UserPlaylistRequest{UserId: 7})
		require.NoError(t, err)
		_, err = PlaylistTracks(ctx, eng, &mmpb.AllTracksRequest{PlaylistId: 1, Limit: 3})
		require.NoError(t, err)
	}

	for _, path := range []string{"/weapi/user/playlist", "/weapi/v6/playlist/detail", "/weapi/v3/song/detail"} {
		got := u.calls(path)
		require.NotEmpty(t, got, path)
		var seq []string
		for _, h := range got {
			require.NotContains(t, h, "SITE_ACCOUNT", "%s 借用了会话池账号", path)
			switch {
			case strings.Contains(h, "ACCOUNT_A"):
				seq = append(seq, "A")
			case strings.Contains(h, "ACCOUNT_B"):
				seq = append(seq, "B")
			case strings.Contains(h, "MUSIC_U"):
				t.Fatalf("%s 出现了意外的 MUSIC_U:%s", path, h)
			default:
				seq = append(seq, "-")
			}
		}
		// 每轮的请求都带着那一轮自己的身份,不串。
		require.Equal(t, collapse(seq), []string{"A", "B", "-", "A"}, path)
	}
	require.Zero(t, pool.asked.Load(), "组合读取不该向会话池要账号")
}

// collapse 把连续相同的元素合成一个(一轮里可能有多次同一路径的请求)。
func collapse(seq []string) []string {
	var out []string
	for _, s := range seq {
		if len(out) == 0 || out[len(out)-1] != s {
			out = append(out, s)
		}
	}
	return out
}

func TestWindow(t *testing.T) {
	t.Parallel()
	cases := []struct {
		total, offset, limit, def int
		lo, hi                    int
	}{
		{10, 0, 3, 100, 0, 3},
		{10, 9, 3, 100, 9, 10},
		{10, 10, 3, 100, 10, 10},
		{10, 50, 3, 100, 10, 10},
		{10, -5, 3, 100, 0, 3},
		{10, 0, 0, 4, 0, 4},
		{5000, 0, 5000, 100, 0, MaxPageSize},
	}
	for _, c := range cases {
		lo, hi := window(c.total, int32(c.offset), int32(c.limit), c.def)
		require.Equal(t, [2]int{c.lo, c.hi}, [2]int{lo, hi}, "%+v", c)
	}
}
