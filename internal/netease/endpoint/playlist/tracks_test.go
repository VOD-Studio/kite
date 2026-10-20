package playlist

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// 用从真实上游抓取的响应验证(裁掉了与曲目 ID 无关的字段)。
func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return b
}

// 匿名请求 71 首的歌单时,详情只内嵌 10 首 tracks,trackIds 才是完整的(issue #89 的根因)。
func TestTrackIDs_RealShape_TracksTruncated(t *testing.T) {
	t.Parallel()

	raw := readFixture(t, "playlist_detail_v6_truncated.json")
	ids, err := TrackIDs.MapResponse(&mmpb.GetPlaylistRequest{PlaylistId: 2219770152}, raw)
	require.NoError(t, err)
	require.Len(t, ids.IDs, 71)
	require.Equal(t, []int64{1949119429, 3422989133, 3411054390}, ids.IDs[:3])

	pl, err := GetPlaylist.MapResponse(&mmpb.GetPlaylistRequest{PlaylistId: 2219770152}, raw)
	require.NoError(t, err)
	require.Len(t, pl.GetPlaylist().GetSongs(), 10, "内嵌 tracks 被截断,不能拿来分页")
	require.Equal(t, int32(71), pl.GetPlaylist().GetTrackCount())
}

func TestTrackIDs_MapRequest(t *testing.T) {
	t.Parallel()

	params, err := TrackIDs.MapRequest(&mmpb.GetPlaylistRequest{PlaylistId: 42})
	require.NoError(t, err)
	require.Equal(t, "42", params["id"])
}

// 账号的私有歌单要能带调用方 cookie 读,又不能借用会话池里保存的账号(issue #90)。
func TestPlaylistReads_OptionalAuth(t *testing.T) {
	t.Parallel()
	require.Equal(t, session.AuthOptional, TrackIDs.Meta.Auth)
	require.Equal(t, session.AuthOptional, GetPlaylist.Meta.Auth)
}
