package album

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
)

// 用从真实上游抓取的响应验证(说明见 artist/real_shape_test.go)。
//
// GetAlbum:id 放 body 的 /weapi/v1/album/detail 只返回 {code,msg},没有 album,
// 会静默解析成空专辑;id 必须拼进 path(/weapi/v1/album/{id})。
func TestGetAlbum_RealShape(t *testing.T) {
	t.Parallel()

	req := &mmpb.GetAlbumRequest{AlbumId: 34209}
	require.Equal(t, "/weapi/v1/album/34209", GetAlbum.PathFunc(req))

	raw, err := os.ReadFile("testdata/album_v1.json")
	require.NoError(t, err)
	resp, err := GetAlbum.MapResponse(req, raw)
	require.NoError(t, err)
	require.Equal(t, int64(34209), resp.GetAlbum().GetId())
	require.NotEmpty(t, resp.GetAlbum().GetName(), "专辑名不能为空——空说明解码器与上游响应形状不符")
	require.Len(t, resp.GetSongs(), 2)
}
