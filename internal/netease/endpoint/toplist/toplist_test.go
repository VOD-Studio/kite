// Package toplist 的接口声明测试。
//
// 夹具取自真实上游匿名响应(testdata,已裁剪):宽松解析在字段名 / 形状漂移时
// 会静默得到零值,只有真实形状的夹具能抓住(见 artist/real_shape_test.go 回归背景)。
package toplist

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
)

// readFixture 读取 testdata 下的真实响应夹具。
func readFixture(t *testing.T, name string) json.RawMessage {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return b
}

// TestList_RealShape /toplist:list 非空、字段落位,tracks 全为 null(概要只在详情接口返回)。
func TestList_RealShape(t *testing.T) {
	t.Parallel()

	resp, err := List.MapResponse(&mmpb.ToplistsRequest{}, readFixture(t, "toplist_weapi.json"))
	require.NoError(t, err)
	require.Len(t, resp.GetToplists(), 3)

	first := resp.GetToplists()[0]
	require.Equal(t, int64(19723756), first.GetId(), "飙升榜 ID")
	require.Equal(t, "飙升榜", first.GetName())
	require.Equal(t, "每天更新", first.GetUpdateFrequency())
	require.NotEmpty(t, first.GetCoverUrl())
	require.Positive(t, first.GetPlayCount(), "播放数为 0 说明 playCount 字段名与上游不符")
	require.Equal(t, int32(100), first.GetTrackCount())
	for _, tl := range resp.GetToplists() {
		require.Empty(t, tl.GetPreviewTracks(), "/toplist 的概要应为空(真实响应里 tracks 恒为 null)")
	}
}

// TestDetail_RealShape /toplist/detail:四个主榜各带三条概要,其余榜单概要为空;
// 概要元素是 first=歌名/second=歌手名串,不是完整歌曲结构。
func TestDetail_RealShape(t *testing.T) {
	t.Parallel()

	resp, err := Detail.MapResponse(&mmpb.ToplistDetailRequest{}, readFixture(t, "toplist_detail_weapi.json"))
	require.NoError(t, err)
	require.Len(t, resp.GetToplists(), 5)

	withPreview := 0
	for _, tl := range resp.GetToplists() {
		switch tl.GetName() {
		case "飙升榜", "新歌榜", "原创榜", "热歌榜":
			withPreview++
			require.Len(t, tl.GetPreviewTracks(), 3, "%s 应带三条前三歌曲概要", tl.GetName())
			require.NotEmpty(t, tl.GetPreviewTracks()[0].GetName(), "概要歌名为空说明 first 字段名与上游不符")
			require.NotEmpty(t, tl.GetPreviewTracks()[0].GetArtists(), "概要歌手名串为空说明 second 字段名与上游不符")
		default:
			require.Empty(t, tl.GetPreviewTracks(), "非主榜不带概要")
		}
	}
	require.Equal(t, 4, withPreview, "夹具应含四个带概要的主榜")
}

// TestList_MapRequest 空参 POST,MapRequest 恒空 map。
func TestList_MapRequest(t *testing.T) {
	t.Parallel()

	for _, ep := range []struct {
		name string
		call func() (map[string]any, error)
	}{
		{"List", func() (map[string]any, error) { return List.MapRequest(&mmpb.ToplistsRequest{}) }},
		{"Detail", func() (map[string]any, error) { return Detail.MapRequest(&mmpb.ToplistDetailRequest{}) }},
	} {
		params, err := ep.call()
		require.NoError(t, err)
		require.Empty(t, params)
	}
}
