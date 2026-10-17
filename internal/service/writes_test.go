package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/cache"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// 写类 RPC(歌单 7 个、歌手收藏、用户互关)走 ctx cookie 路径:cookie 来自拦截器注入的 ctx,
// 请求发往确定的上游路径,响应按各自的形状映射。这些 RPC 曾是「Meta + Request + Parse」三件套,
// 改为声明式 endpoint 后行为必须完全一致——本测试在改写前的代码上先绿,改写后仍绿。
func TestWriteRPCs_PathCookieAndMapping(t *testing.T) {
	var mu sync.Mutex
	var gotPaths, gotCookies []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPaths = append(gotPaths, r.URL.Path)
		gotCookies = append(gotCookies, r.Header.Get("Cookie"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/weapi/playlist/subscribe":
			_, _ = w.Write([]byte(`{"code":200,"subscribed":true}`))
		case "/weapi/playlist/create":
			_, _ = w.Write([]byte(`{"code":200,"id":999}`))
		case "/weapi/playlist/manipulate/tracks":
			_, _ = w.Write([]byte(`{"code":200,"body":[{"trackId":10},{"trackId":20}]}`))
		case "/weapi/artist/sub":
			_, _ = w.Write([]byte(`{"code":200,"sub":true}`))
		case "/weapi/user/getfollows/99":
			_, _ = w.Write([]byte(`{"code":200,"follow":[{"userId":1},{"userId":2}]}`))
		default:
			_, _ = w.Write([]byte(`{"code":200}`))
		}
	}))
	t.Cleanup(srv.Close)

	eng := engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(srv.URL))
	ctx := engine.WithCookie(context.Background(), "MUSIC_U=abc")
	pl, ar, us := NewPlaylistServer(eng), NewArtistServer(eng), NewUserServer(eng)

	tests := []struct {
		name     string
		wantPath string
		call     func(ctx context.Context) (any, error)
		check    func(t *testing.T, resp any)
	}{
		{"playlist.Subscribe", "/weapi/playlist/subscribe",
			func(ctx context.Context) (any, error) {
				return pl.Subscribe(ctx, &mmpb.SubscribeRequest{PlaylistId: 1})
			},
			func(t *testing.T, r any) { require.True(t, r.(*mmpb.SubscribeResponse).Subscribed) }},
		{"playlist.Create", "/weapi/playlist/create",
			func(ctx context.Context) (any, error) { return pl.Create(ctx, &mmpb.CreateRequest{Name: "n"}) },
			func(t *testing.T, r any) { require.Equal(t, int64(999), r.(*mmpb.CreateResponse).PlaylistId) }},
		{"playlist.Delete", "/weapi/playlist/delete",
			func(ctx context.Context) (any, error) { return pl.Delete(ctx, &mmpb.DeleteRequest{PlaylistId: 1}) },
			func(t *testing.T, r any) { require.NotNil(t, r) }},
		{"playlist.UpdateName", "/weapi/playlist/update/name",
			func(ctx context.Context) (any, error) {
				return pl.UpdateName(ctx, &mmpb.UpdateNameRequest{PlaylistId: 1, Name: "x"})
			},
			func(t *testing.T, r any) { require.NotNil(t, r) }},
		{"playlist.UpdateDesc", "/weapi/playlist/update/description",
			func(ctx context.Context) (any, error) {
				return pl.UpdateDesc(ctx, &mmpb.UpdateDescRequest{PlaylistId: 1, Desc: "x"})
			},
			func(t *testing.T, r any) { require.NotNil(t, r) }},
		{"playlist.UpdateTags", "/weapi/playlist/update/tags",
			func(ctx context.Context) (any, error) {
				return pl.UpdateTags(ctx, &mmpb.UpdateTagsRequest{PlaylistId: 1, Tags: "a;b"})
			},
			func(t *testing.T, r any) { require.NotNil(t, r) }},
		{"playlist.UpdateTracks", "/weapi/playlist/manipulate/tracks",
			func(ctx context.Context) (any, error) {
				return pl.UpdateTracks(ctx, &mmpb.UpdateTracksRequest{PlaylistId: 1, Op: mmpb.TracksOp_TRACKS_OP_ADD, TrackIds: []int64{10, 20}})
			},
			func(t *testing.T, r any) {
				require.Equal(t, []int64{10, 20}, r.(*mmpb.UpdateTracksResponse).BodyTrackIds)
			}},
		{"artist.ArtistSubscribe", "/weapi/artist/sub",
			func(ctx context.Context) (any, error) {
				return ar.ArtistSubscribe(ctx, &mmpb.ArtistSubscribeRequest{ArtistId: 5})
			},
			func(t *testing.T, r any) { require.True(t, r.(*mmpb.ArtistSubscribeResponse).Subscribed) }},
		{"user.FollowEachOther(命中)", "/weapi/user/getfollows/99",
			func(ctx context.Context) (any, error) {
				return us.FollowEachOther(ctx, &mmpb.FollowEachOtherRequest{UserId: 2, TargetUserId: 99})
			},
			func(t *testing.T, r any) { require.True(t, r.(*mmpb.FollowEachOtherResponse).FollowEachOther) }},
		{"user.FollowEachOther(未命中)", "/weapi/user/getfollows/99",
			func(ctx context.Context) (any, error) {
				return us.FollowEachOther(ctx, &mmpb.FollowEachOtherRequest{UserId: 3, TargetUserId: 99})
			},
			func(t *testing.T, r any) { require.False(t, r.(*mmpb.FollowEachOtherResponse).FollowEachOther) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mu.Lock()
			before := len(gotPaths)
			mu.Unlock()

			resp, err := tt.call(ctx)
			require.NoError(t, err)
			tt.check(t, resp)

			mu.Lock()
			defer mu.Unlock()
			require.Len(t, gotPaths, before+1, "应恰好请求上游一次")
			require.Equal(t, tt.wantPath, gotPaths[before])
			require.Contains(t, gotCookies[before], "MUSIC_U=abc", "ctx 里的 cookie 应发往上游")
		})

		// 写类 RPC 走 ctx cookie 路径,不走会话池:没有 cookie 时也照常请求上游(由上游判定登录态),
		// 而不是在本地先被拒绝。若误改成会话池路径(engine.Execute),无会话池 + 无 cookie 会本地返回未授权、
		// 根本不请求上游——带 cookie 的用例看不出区别,必须用匿名调用钉死路径选择。
		t.Run(tt.name+"/无cookie仍请求上游", func(t *testing.T) {
			mu.Lock()
			before := len(gotPaths)
			mu.Unlock()

			_, err := tt.call(context.Background())
			require.NoError(t, err)

			mu.Lock()
			defer mu.Unlock()
			require.Len(t, gotPaths, before+1, "ctx cookie 路径不做本地凭据短路")
			require.Equal(t, "__remember_me=true", gotCookies[before], "没有 cookie 时不带任何用户凭据")
		})
	}
}
