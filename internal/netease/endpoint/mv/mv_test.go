// Package mv 的接口声明测试。
//
// 夹具取自真实上游匿名响应(testdata,已裁剪):宽松解析在字段名 / 形状漂移时
// 会静默得到零值,只有真实形状的夹具能抓住(见 artist/real_shape_test.go 回归背景)。
package mv

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

// TestDetail_RealShape /mv/detail:元数据 + brs 多清晰度播放地址落位。
func TestDetail_RealShape(t *testing.T) {
	t.Parallel()

	resp, err := Detail.MapResponse(&mmpb.GetMvRequest{}, readFixture(t, "detail_weapi.json"))
	require.NoError(t, err)

	mv := resp.GetMv()
	require.Equal(t, int64(5436712), mv.GetId(), "广岛之恋 MV ID")
	require.Equal(t, "广岛之恋", mv.GetName())
	require.Equal(t, "莫文蔚", mv.GetArtistName())
	require.Equal(t, "1997-10-01", mv.GetPublishTime(), "发行日期为空说明 publishTime 字段名与上游不符")
	require.Equal(t, int64(378040), mv.GetDurationMs())
	require.Positive(t, mv.GetPlayCount(), "播放数为 0 说明 playCount 字段名与上游不符")
	require.Positive(t, mv.GetSubCount(), "收藏数为 0 说明 subCount 字段名与上游不符")
	require.Len(t, mv.GetArtists(), 2, "歌手数组为空说明 artists 字段名与上游不符")

	// brs 是多清晰度播放地址 map,解码按分辨率升序输出。
	urls := mv.GetUrls()
	require.Len(t, urls, 2)
	require.Equal(t, int32(240), urls[0].GetResolution())
	require.Equal(t, int32(480), urls[1].GetResolution())
	for _, u := range urls {
		require.NotEmpty(t, u.GetUrl(), "播放地址为空说明 brs 解析与上游不符")
	}
}

// TestTop_RealShape /mv/toplist:榜单条目 + 排行信息落位;顶层 duration 上游恒 0,
// 发行日期在嵌套 mv 子对象上(夹具保留了这一真实形状)。
func TestTop_RealShape(t *testing.T) {
	t.Parallel()

	resp, err := Top.MapResponse(&mmpb.TopMvsRequest{}, readFixture(t, "toplist_weapi.json"))
	require.NoError(t, err)

	require.False(t, resp.GetHasMore())
	require.Positive(t, resp.GetUpdateTimeMs(), "更新时间为 0 说明 updateTime 字段名与上游不符")
	mvs := resp.GetMvs()
	require.Len(t, mvs, 2)

	first := mvs[0]
	require.Equal(t, "中国人会飞", first.GetName())
	require.Equal(t, "Chalky Wong", first.GetArtistName())
	require.Equal(t, int32(5), first.GetLastRank(), "上期排名为 0 说明 lastRank 字段名与上游不符")
	require.Equal(t, int32(29), first.GetScore())
	require.Equal(t, "2026-08-07", first.GetPublishTime(), "发行日期为空说明未从嵌套 mv.publishTime 取值")
	require.Zero(t, first.GetDurationMs(), "排行榜条目顶层 duration 上游恒 0")
	require.Positive(t, first.GetPlayCount())
	require.NotEmpty(t, first.GetArtists())
	require.NotEmpty(t, first.GetCoverUrl())
}

// TestSimilar_RealShape /simi/mv:相关 MV 摘要列表落位。
func TestSimilar_RealShape(t *testing.T) {
	t.Parallel()

	resp, err := Similar.MapResponse(&mmpb.SimilarMvsRequest{}, readFixture(t, "simi_weapi.json"))
	require.NoError(t, err)

	mvs := resp.GetMvs()
	require.Len(t, mvs, 2)

	first := mvs[0]
	require.Equal(t, int64(142002), first.GetId())
	require.Equal(t, "超级面对面 第29期 陶喆：温情的我依然有姿态", first.GetName())
	require.Equal(t, "陶喆", first.GetArtistName())
	require.Equal(t, int64(539000), first.GetDurationMs(), "时长为 0 说明 duration 字段名与上游不符")
	require.Positive(t, first.GetPlayCount(), "播放数为 0 说明 playCount 字段名与上游不符")
	require.Len(t, first.GetArtists(), 2)
	require.Equal(t, "陶喆", first.GetArtists()[0].GetName())
}

// TestSubscribed_RealShape /mv/sublist 匿名调用的真实响应:code 200 + 空 data
// (上游对匿名不报错而是返回空列表)。条目解析与 /simi/mv 共用 DecodeMvSummaries,
// 元素形状由 TestSimilar_RealShape 的真实夹具守护。
func TestSubscribed_RealShape(t *testing.T) {
	t.Parallel()

	resp, err := Subscribed.MapResponse(&mmpb.SubscribedMvsRequest{}, readFixture(t, "sublist_weapi.json"))
	require.NoError(t, err)
	require.Empty(t, resp.GetMvs())
	require.False(t, resp.GetHasMore())
}

// TestMapRequest 各接口的请求参数形态(实测:detail 用 id、similar 用 mvid,
// 均为字符串;sublist limit 缺省 25)。
func TestMapRequest(t *testing.T) {
	t.Parallel()

	params, err := Detail.MapRequest(&mmpb.GetMvRequest{MvId: 5436712})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"id": "5436712"}, params)

	params, err = Similar.MapRequest(&mmpb.SimilarMvsRequest{MvId: 5436712})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"mvid": "5436712"}, params)

	params, err = Top.MapRequest(&mmpb.TopMvsRequest{})
	require.NoError(t, err)
	require.Empty(t, params)

	params, err = Subscribed.MapRequest(&mmpb.SubscribedMvsRequest{Limit: 10, Offset: 20})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"limit": int32(10), "offset": int32(20), "total": "true"}, params)

	params, err = Subscribed.MapRequest(&mmpb.SubscribedMvsRequest{})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"limit": int32(25), "offset": int32(0), "total": "true"}, params)
}
