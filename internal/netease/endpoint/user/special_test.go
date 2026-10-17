// Package user 的特殊接口测试。
package user

import (
	"encoding/json"
	"testing"

	merrors "github.com/VOD-Studio/kite/errors"
	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/session"
	"github.com/stretchr/testify/require"
)

// TestDetailByName_MapResponse 昵称搜索结果解析(取第一条 userId)。
func TestDetailByName_MapResponse(t *testing.T) {
	t.Parallel()

	fixture := `{"result":{"userprofiles":[{"userId":32014612,"nickname":"网易云音乐"}],"userprofileCount":1},"code":200}`

	resp, err := DetailByName.MapResponse(&mmpb.DetailByNameRequest{Nickname: "网易云音乐"}, json.RawMessage(fixture))
	require.NoError(t, err)
	require.Equal(t, int64(32014612), resp.UserId)
}

// TestDetailByName_MapResponse_Empty 昵称无结果时报错。
func TestDetailByName_MapResponse_Empty(t *testing.T) {
	t.Parallel()

	fixture := `{"result":{"userprofiles":[],"userprofileCount":0},"code":200}`

	_, err := DetailByName.MapResponse(&mmpb.DetailByNameRequest{Nickname: "不存在的昵称"}, json.RawMessage(fixture))
	require.ErrorIs(t, err, merrors.ErrNotFound)
}

// TestFollowEachOther_Meta path 是动态的(target uid 拼进 URL),匿名 cookie,不缓存。
func TestFollowEachOther_Meta(t *testing.T) {
	t.Parallel()

	require.Equal(t, session.AuthAnonymous, FollowEachOther.Meta.Auth)
	require.Equal(t, engine.CryptoWeAPI, FollowEachOther.Meta.Crypto)
	require.Equal(t, "POST", FollowEachOther.Meta.Method)
	require.Nil(t, FollowEachOther.Cache)
	require.Equal(t, "/weapi/user/getfollows/99",
		FollowEachOther.PathFunc(&mmpb.FollowEachOtherRequest{UserId: 2, TargetUserId: 99}),
		"target uid 拼进 path;user_id 不参与 path")
}

// TestFollowEachOther_MapRequest 最多拉前 100 条做包含判断。
func TestFollowEachOther_MapRequest(t *testing.T) {
	t.Parallel()

	params, err := FollowEachOther.MapRequest(&mmpb.FollowEachOtherRequest{UserId: 2, TargetUserId: 99})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"offset": 0, "limit": 100, "order": true}, params)
}

// TestParseFollowEachOther 关注列表包含判断。
func TestParseFollowEachOther(t *testing.T) {
	t.Parallel()

	fixture := `{"follow":[{"userId":1},{"userId":2}],"code":200}`

	hit, err := FollowEachOther.MapResponse(&mmpb.FollowEachOtherRequest{UserId: 2, TargetUserId: 99}, json.RawMessage(fixture))
	require.NoError(t, err)
	require.True(t, hit.FollowEachOther)

	miss, err := FollowEachOther.MapResponse(&mmpb.FollowEachOtherRequest{UserId: 3, TargetUserId: 99}, json.RawMessage(fixture))
	require.NoError(t, err)
	require.False(t, miss.FollowEachOther)
}

// 响应不是 JSON:返回错误(引擎会把它归为 ErrInvalidResponse → UPSTREAM_INVALID_RESPONSE)。
func TestParseFollowEachOther_InvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := FollowEachOther.MapResponse(&mmpb.FollowEachOtherRequest{UserId: 2}, json.RawMessage(`not json`))
	require.Error(t, err)
}
