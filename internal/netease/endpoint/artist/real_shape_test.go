package artist

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
)

// 用从真实上游抓取的响应(testdata,已裁剪)验证 endpoint,而不是手写「我以为的形状」。
// 回归背景:上游改过响应形状 / 字段名后,宽松解析会静默得到零值,RPC「成功但返回空」,
// 手写夹具永远发现不了——手写夹具与解码器是同一个人的同一个假设。
func readFixture(t *testing.T, name string) json.RawMessage {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return b
}

// GetArtist:id 必须拼进 path(/weapi/v1/artist/{id});id 放 body 或换用 head/info 形态,
// 响应形状与解码器不符,会静默解析成空歌手。
func TestGetArtist_RealShape(t *testing.T) {
	t.Parallel()

	req := &mmpb.GetArtistRequest{ArtistId: 6452}
	require.Equal(t, "/weapi/v1/artist/6452", GetArtist.PathFunc(req))

	resp, err := GetArtist.MapResponse(req, readFixture(t, "artist_v1.json"))
	require.NoError(t, err)
	require.Equal(t, int64(6452), resp.GetArtist().GetId())
	require.NotEmpty(t, resp.GetArtist().GetName(), "歌手名不能为空——空说明解码器与上游响应形状不符")
	require.NotEmpty(t, resp.GetArtist().GetPicUrl())
	require.Len(t, resp.GetHotSongs(), 2)
	require.NotEmpty(t, resp.GetHotSongs()[0].GetName())
}

// Fans:上游把 data.fansCount 改名为 data.fansCnt。
func TestFans_RealShape(t *testing.T) {
	t.Parallel()

	resp, err := Fans.MapResponse(&mmpb.FansRequest{ArtistId: 6452}, readFixture(t, "fans_count.json"))
	require.NoError(t, err)
	require.Positive(t, resp.GetFans(), "粉丝数为 0 说明字段名与上游不符")
}
