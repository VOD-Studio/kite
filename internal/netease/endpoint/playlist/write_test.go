// Package playlist 的写操作声明测试。
//
// 写操作的入参构造（网易云古怪格式）和 Meta（路径 / 加密 / 登录态）是重点验证对象。
package playlist

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// params 取 endpoint 的 MapRequest 结果。
func params[Req, Resp any](t *testing.T, ep *engine.Endpoint[Req, Resp], req Req) map[string]any {
	t.Helper()
	p, err := ep.MapRequest(req)
	require.NoError(t, err)
	return p
}

// resp 取 endpoint 的 MapResponse 结果。
func resp[Req, Resp any](t *testing.T, ep *engine.Endpoint[Req, Resp], req Req, raw string) Resp {
	t.Helper()
	r, err := ep.MapResponse(req, json.RawMessage(raw))
	require.NoError(t, err)
	return r
}

// 7 个写操作的执行元数据:全部 weapi POST + 需要登录态 + 不缓存。
// 路径是线上契约,改动即破坏,逐条钉死。
func TestWriteEndpoints_Meta(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		meta  engine.Meta
		cache bool
		path  string
	}{
		{"Subscribe", Subscribe.Meta, Subscribe.Cache != nil, "/weapi/playlist/subscribe"},
		{"Create", Create.Meta, Create.Cache != nil, "/weapi/playlist/create"},
		{"Delete", Delete.Meta, Delete.Cache != nil, "/weapi/playlist/delete"},
		{"UpdateName", UpdateName.Meta, UpdateName.Cache != nil, "/weapi/playlist/update/name"},
		{"UpdateDesc", UpdateDesc.Meta, UpdateDesc.Cache != nil, "/weapi/playlist/update/description"},
		{"UpdateTags", UpdateTags.Meta, UpdateTags.Cache != nil, "/weapi/playlist/update/tags"},
		{"UpdateTracks", UpdateTracks.Meta, UpdateTracks.Cache != nil, "/weapi/playlist/manipulate/tracks"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, engine.Meta{
				Path: tt.path, Method: "POST", Crypto: engine.CryptoWeAPI, Auth: session.AuthLoggedIn,
			}, tt.meta)
			require.False(t, tt.cache, "写操作不缓存")
		})
	}
}

// TestSubscribeRequest 收藏入参构造。
func TestSubscribeRequest(t *testing.T) {
	t.Parallel()

	p := params(t, Subscribe, &mmpb.SubscribeRequest{PlaylistId: 100})
	require.Equal(t, "100", p["id"])
}

// TestCreateRequest 新建歌单入参构造。
func TestCreateRequest(t *testing.T) {
	t.Parallel()

	p := params(t, Create, &mmpb.CreateRequest{Name: "我的歌单", Privacy: true})
	require.Equal(t, "我的歌单", p["name"])
	require.Equal(t, 1, p["privacy"])

	p = params(t, Create, &mmpb.CreateRequest{Name: "公开"})
	require.Equal(t, 0, p["privacy"])
}

// TestDeleteRequest 删除入参构造。
func TestDeleteRequest(t *testing.T) {
	t.Parallel()

	p := params(t, Delete, &mmpb.DeleteRequest{PlaylistId: 200})
	require.Equal(t, "[200]", p["ids"])
}

// TestUpdateTracksRequest_Add 添加歌曲入参。
func TestUpdateTracksRequest_Add(t *testing.T) {
	t.Parallel()

	p := params(t, UpdateTracks, &mmpb.UpdateTracksRequest{
		PlaylistId: 1, Op: mmpb.TracksOp_TRACKS_OP_ADD, TrackIds: []int64{10, 20, 30},
	})
	require.Equal(t, "add", p["op"])
	require.Equal(t, "1", p["pid"])
	require.Equal(t, "10,20,30", p["tracks"])
}

// TestUpdateTracksRequest_Del 删除歌曲入参。
func TestUpdateTracksRequest_Del(t *testing.T) {
	t.Parallel()

	p := params(t, UpdateTracks, &mmpb.UpdateTracksRequest{
		PlaylistId: 1, Op: mmpb.TracksOp_TRACKS_OP_DEL, TrackIds: []int64{5},
	})
	require.Equal(t, "del", p["op"])
	require.Equal(t, "5", p["tracks"])
}

// 没有歌曲时 tracks 为空串(不是 panic)。
func TestUpdateTracksRequest_Empty(t *testing.T) {
	t.Parallel()

	p := params(t, UpdateTracks, &mmpb.UpdateTracksRequest{PlaylistId: 1})
	require.Equal(t, "", p["tracks"])
}

// TestParseSubscribed 收藏响应解析。
func TestParseSubscribed(t *testing.T) {
	t.Parallel()

	r := resp(t, Subscribe, &mmpb.SubscribeRequest{}, `{"subscribed":true}`)
	require.True(t, r.Subscribed)
}

// TestParseCreateResponse 新建歌单响应解析。
func TestParseCreateResponse(t *testing.T) {
	t.Parallel()

	r := resp(t, Create, &mmpb.CreateRequest{}, `{"id":999}`)
	require.Equal(t, int64(999), r.PlaylistId)
}

// TestParseCreateResponse_PlaylistField playlist.id 字段回退。
func TestParseCreateResponse_PlaylistField(t *testing.T) {
	t.Parallel()

	r := resp(t, Create, &mmpb.CreateRequest{}, `{"playlist":{"id":888}}`)
	require.Equal(t, int64(888), r.PlaylistId)
}

// 写操作响应宽松解析:不是预期形状时按零值处理,不报错(保持原有语义)。
func TestWriteResponses_Lenient(t *testing.T) {
	t.Parallel()

	require.False(t, resp(t, Subscribe, &mmpb.SubscribeRequest{}, `not json`).Subscribed)
	require.Zero(t, resp(t, Create, &mmpb.CreateRequest{}, `[]`).PlaylistId)
	require.Empty(t, resp(t, UpdateTracks, &mmpb.UpdateTracksRequest{}, `{}`).BodyTrackIds)
}

// TestParseUpdateTracksResponse 添加/删除歌曲响应解析。
func TestParseUpdateTracksResponse(t *testing.T) {
	t.Parallel()

	r := resp(t, UpdateTracks, &mmpb.UpdateTracksRequest{}, `{"body":[{"trackId":1},{"trackId":2}]}`)
	require.Equal(t, []int64{1, 2}, r.BodyTrackIds)
}

// TestUpdateNameRequest 更新歌单名入参构造。
func TestUpdateNameRequest(t *testing.T) {
	t.Parallel()

	p := params(t, UpdateName, &mmpb.UpdateNameRequest{PlaylistId: 100, Name: "新名字"})
	require.Equal(t, "100", p["id"])
	require.Equal(t, "新名字", p["name"])
}

// TestUpdateDescRequest 更新歌单描述入参构造。
func TestUpdateDescRequest(t *testing.T) {
	t.Parallel()

	p := params(t, UpdateDesc, &mmpb.UpdateDescRequest{PlaylistId: 100, Desc: "新描述"})
	require.Equal(t, "100", p["id"])
	require.Equal(t, "新描述", p["desc"])
}

// TestUpdateTagsRequest 更新歌单标签入参构造。
func TestUpdateTagsRequest(t *testing.T) {
	t.Parallel()

	p := params(t, UpdateTags, &mmpb.UpdateTagsRequest{PlaylistId: 100, Tags: "流行;华语"})
	require.Equal(t, "100", p["id"])
	require.Equal(t, "流行;华语", p["tags"])
}
