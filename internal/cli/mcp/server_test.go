package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	albumendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/album"
	artistendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/artist"
	authendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/auth"
	playlistendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/playlist"
	songendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/song"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// connect 用内存传输把一个 MCP 客户端接到 NewServer 上,返回客户端会话。
func connect(t *testing.T, d Deps) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverT, clientT := mcp.NewInMemoryTransports()
	ss, err := NewServer(d, "test").Connect(ctx, serverT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil).Connect(ctx, clientT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// fakeUpstream 按路径返回最小可映射的网易云响应,并记录最近一次请求的 Cookie 头。
func fakeUpstream(t *testing.T) (*httptest.Server, *atomic.Value) {
	t.Helper()
	var lastCookie atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastCookie.Store(r.Header.Get("Cookie"))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/search/get":
			_, _ = w.Write([]byte(`{"code":200,"result":{"songCount":1,"songs":[{"id":186016,"name":"晴天","artists":[{"id":6452,"name":"周杰伦"}],"album":{"id":18905,"name":"叶惠美"},"duration":269000}]}}`))
		case strings.HasSuffix(r.URL.Path, "/song/enhance/player/url/v1"):
			_, _ = w.Write([]byte(`{"code":200,"data":[{"id":186016,"url":"https://m801.music.126.net/x.mp3","br":320000,"size":1,"type":"mp3"}]}`))
		case r.URL.Path == "/weapi/v3/discovery/recommend/songs":
			_, _ = w.Write([]byte(`{"code":301,"msg":"需要登录"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &lastCookie
}

// 工具清单是对外契约:增删工具必须有意识地改这里。
func TestTools_ListedReadOnly(t *testing.T) {
	cs := connect(t, Deps{Engine: engine.New(), Cookie: func() string { return "" }})
	res, err := cs.ListTools(context.Background(), nil)
	require.NoError(t, err)

	var names []string
	for _, tl := range res.Tools {
		names = append(names, tl.Name)
		require.NotNil(t, tl.Annotations, tl.Name)
		require.True(t, tl.Annotations.ReadOnlyHint, "%s 必须标为只读", tl.Name)
		require.NotEmpty(t, tl.Description, tl.Name)
	}
	sort.Strings(names)
	require.Equal(t, []string{
		"daily_recommend", "get_album", "get_artist", "get_artist_top_songs", "get_lyric", "get_playlist",
		"get_song", "get_song_url", "hot_searches", "search_songs", "similar_songs",
	}, names)
}

// 只读硬约束:任何工具都不能绑定写类或登录类 endpoint。
func TestTools_NeverBindWriteEndpoints(t *testing.T) {
	forbidden := []engine.Meta{
		songendpoint.Like.Meta, songendpoint.Trash.Meta, songendpoint.DisallowRecommend.Meta,
		playlistendpoint.Subscribe.Meta, playlistendpoint.Create.Meta, playlistendpoint.Delete.Meta,
		playlistendpoint.UpdateName.Meta, playlistendpoint.UpdateDesc.Meta, playlistendpoint.UpdateTags.Meta,
		playlistendpoint.UpdateTracks.Meta,
		albumendpoint.Subscribe.Meta, albumendpoint.Unsubscribe.Meta, artistendpoint.Subscribe.Meta,
		authendpoint.SendCaptcha, authendpoint.LoginCellphone, authendpoint.LoginQrcode, authendpoint.CheckQrcode,
		authendpoint.Logout,
	}
	for _, tl := range tools {
		for _, f := range forbidden {
			require.NotEqual(t, f.Path, tl.Meta.Path, "工具 %s 绑定了写类 / 登录类 endpoint %s", tl.Name, f.Path)
		}
	}
}

func TestTools_CallThroughEngine(t *testing.T) {
	srv, lastCookie := fakeUpstream(t)
	cookie := ""
	cs := connect(t, Deps{
		Engine: engine.New(engine.WithBaseURL(srv.URL)),
		Cookie: func() string { return cookie },
	})
	ctx := context.Background()
	call := func(name string, args map[string]any) *mcp.CallToolResult {
		t.Helper()
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		require.NoError(t, err)
		return res
	}
	text := func(res *mcp.CallToolResult) string {
		t.Helper()
		require.Len(t, res.Content, 1)
		return res.Content[0].(*mcp.TextContent).Text
	}

	res := call("search_songs", map[string]any{"keyword": "晴天"})
	require.False(t, res.IsError, text(res))
	require.Contains(t, text(res), "晴天")
	require.Contains(t, text(res), "周杰伦")

	// 每次调用时读取 cookie:登录后无需重启即按账号请求上游。
	cookie = "MUSIC_U=vip"
	res = call("get_song_url", map[string]any{"song_id": 186016, "level": "exhigh"})
	require.False(t, res.IsError, text(res))
	require.Contains(t, text(res), "https://m801.music.126.net/x.mp3")
	require.Contains(t, lastCookie.Load(), "MUSIC_U=vip")

	// 音质写错:作为工具错误告诉 AI 可选值,而不是悄悄按 standard 返回。
	res = call("get_song_url", map[string]any{"song_id": 186016, "level": "flac"})
	require.True(t, res.IsError)
	require.Contains(t, text(res), "不支持的音质")

	// 上游报未登录:作为工具错误返回给 AI(而不是协议错误),消息可读。
	cookie = ""
	res = call("daily_recommend", nil)
	require.True(t, res.IsError)
	require.Contains(t, text(res), "登录")

	// 缺必填参数:由 schema 校验拦下,作为工具错误告诉 AI 缺了哪个参数。
	res = call("get_song", map[string]any{})
	require.True(t, res.IsError)
	require.Contains(t, text(res), "song_id")
}

// docs/mcp.md 的工具表与代码一致:每个工具都有一行,也没有已删除的工具残留。
func TestDocsListEveryTool(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/mcp.md")
	require.NoError(t, err)
	documented := map[string]bool{}
	for _, m := range regexp.MustCompile("(?m)^\\| `([a-z_]+)` \\|").FindAllStringSubmatch(string(raw), -1) {
		documented[m[1]] = true
	}
	for _, tl := range tools {
		require.True(t, documented[tl.Name], "docs/mcp.md 缺少工具 %s", tl.Name)
		delete(documented, tl.Name)
	}
	require.Empty(t, documented, "docs/mcp.md 列了不存在的工具")
}
