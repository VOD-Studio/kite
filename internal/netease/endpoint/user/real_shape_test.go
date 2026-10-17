package user

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
)

// 用从真实上游抓取的响应验证(说明见 artist/real_shape_test.go;个人信息已替换为合成数据)。
func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return b
}

// Detail:/weapi/v3/user/detail 对真实上游返回空响应,服务端因此恒报 NOT_FOUND;
// 可用的是把 uid 拼进 path 的 /weapi/v1/user/detail/{uid}。
func TestDetail_RealShape(t *testing.T) {
	t.Parallel()

	req := &mmpb.DetailRequest{UserId: 32953014}
	require.Equal(t, "/weapi/v1/user/detail/32953014", Detail.PathFunc(req))

	resp, err := Detail.MapResponse(req, readFixture(t, "user_detail_v1.json"))
	require.NoError(t, err)
	require.Equal(t, int64(32953014), resp.GetUser().GetUserId())
	require.NotEmpty(t, resp.GetUser().GetNickname())
	require.Positive(t, resp.GetUser().GetLevel())
}

// Follows:uid 放 body 的 /weapi/user/getfollows 对真实上游返回 404;uid 必须拼进 path,
// body 只带分页与排序。
func TestFollows_RealShape(t *testing.T) {
	t.Parallel()

	req := &mmpb.FollowsRequest{UserId: 32953014, Limit: 2}
	require.Equal(t, "/weapi/user/getfollows/32953014", Follows.PathFunc(req))

	params, err := Follows.MapRequest(req)
	require.NoError(t, err)
	require.Equal(t, int32(2), params["limit"])
	require.Equal(t, true, params["order"])
	require.NotContains(t, params, "uid", "uid 在 path 里,不在 body")

	resp, err := Follows.MapResponse(req, readFixture(t, "follows_v1.json"))
	require.NoError(t, err)
	require.Len(t, resp.GetFollows(), 2)
	require.NotEmpty(t, resp.GetFollows()[0].GetNickname())
}
