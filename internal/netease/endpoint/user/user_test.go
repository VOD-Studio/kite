// Package user 的 endpoint MapResponse 测试。
package user

import (
	"encoding/json"
	"testing"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/session"
	"github.com/stretchr/testify/require"
)

// TestDetail_MapResponse 用户详情解析。
func TestDetail_MapResponse(t *testing.T) {
	t.Parallel()

	fixture := `{"code":200,"level":8,"profile":{"userId":123,"nickname":"alice","avatarUrl":"http://a.jpg","gender":1,"signature":"hi","followeds":100,"follows":50}}`

	resp, err := Detail.MapResponse(&mmpb.DetailRequest{}, json.RawMessage(fixture))
	require.NoError(t, err)
	require.Equal(t, int64(123), resp.User.UserId)
	require.Equal(t, "alice", resp.User.Nickname)
	require.Equal(t, int32(8), resp.User.Level)
	require.Equal(t, int64(100), resp.User.Followeds)
}

// TestSubCount_MapResponse 用户统计解析。
func TestSubCount_MapResponse(t *testing.T) {
	t.Parallel()

	fixture := `{"code":200,"playlistCount":5,"djRadiosCount":2,"mvCount":10,"artistCount":3,"newAlbumsCount":1}`

	resp, err := SubCount.MapResponse(&mmpb.SubCountRequest{}, json.RawMessage(fixture))
	require.NoError(t, err)
	require.Equal(t, int32(5), resp.Count.PlaylistCount)
	require.Equal(t, int32(10), resp.Count.MvCount)
}

// TestPlaylistPage_RealShape 用户歌单的一页按上游顺序原样解析,不筛选、不计总数(真实响应,昵称已替换)。
//
// 上游第一页会在 limit 之外额外插入「喜欢的音乐」:这份 limit=3 的响应有 4 条。
// 筛选与分页在 compose.UserPlaylists 里做,见 compose 的测试。
func TestPlaylistPage_RealShape(t *testing.T) {
	t.Parallel()

	page, err := PlaylistPage.MapResponse(&mmpb.UserPlaylistRequest{UserId: 129593031}, readFixture(t, "user_playlist_page_v1.json"))
	require.NoError(t, err)
	require.True(t, page.More)
	require.Len(t, page.Playlists, 4, "limit=3 的第一页多出「喜欢的音乐」")
	require.Equal(t, int64(162525487), page.Playlists[0].GetId())
	require.Equal(t, int64(129593031), page.Playlists[0].GetCreator().GetId())
	require.Equal(t, int32(3672), page.Playlists[0].GetTrackCount())
}

func TestPlaylistPage_MapRequest(t *testing.T) {
	t.Parallel()

	params, err := PlaylistPage.MapRequest(&mmpb.UserPlaylistRequest{UserId: 7, Limit: 1000, Offset: 2000})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"uid": "7", "limit": 1000, "offset": int32(2000)}, params)

	params, err = PlaylistPage.MapRequest(&mmpb.UserPlaylistRequest{UserId: 7})
	require.NoError(t, err)
	require.Equal(t, 30, params["limit"], "未指定 limit 时用上游默认的 30")
}

// 账号歌单要能带调用方的 cookie 读私有歌单,又不能借用会话池里保存的账号(issue #90)。
func TestPlaylistPage_OptionalAuth(t *testing.T) {
	t.Parallel()
	require.Equal(t, session.AuthOptional, PlaylistPage.Meta.Auth)
	require.Nil(t, PlaylistPage.Cache, "响应不是 proto,不走 engine 缓存")
}
