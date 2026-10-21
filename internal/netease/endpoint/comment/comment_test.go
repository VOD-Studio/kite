package comment

import (
	"testing"

	"github.com/stretchr/testify/require"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// threadId 前缀是评论域的地基:拼错前缀上游 404,这里逐类型断言。
func TestThreadID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		typ  mmpb.CommentResourceType
		id   int64
		want string
	}{
		{mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_SONG, 347230, "R_SO_4_347230"},
		{mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_MV, 14514682, "R_MV_5_14514682"},
		{mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_PLAYLIST, 3778678, "A_PL_0_3778678"},
		{mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_ALBUM, 34209, "R_AL_3_34209"},
	}
	for _, c := range cases {
		got, err := threadID(c.typ, c.id)
		require.NoError(t, err)
		require.Equal(t, c.want, got)
	}
	_, err := threadID(mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_UNSPECIFIED, 1)
	require.Error(t, err, "未指定资源类型必须报错,不能把坏 threadId 打到上游")
}

// List 的分页换算按上游 sortType 分支:推荐用数值 cursor、热度用 normalHot# 前缀、
// 时间只认字符串 cursor(换算规则与 NeteaseCloudMusicApi comment/new 一致)。
func TestList_MapRequest(t *testing.T) {
	t.Parallel()

	song := mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_SONG

	// 推荐排序(默认):pageNo/pageSize 缺省补 1/20,cursor=(pageNo-1)*pageSize。
	params, err := List.MapRequest(&mmpb.CommentListRequest{ResourceId: 347230, ResourceType: song})
	require.NoError(t, err)
	require.Equal(t, "R_SO_4_347230", params["threadId"])
	require.Equal(t, 1, params["pageNo"])
	require.Equal(t, 20, params["pageSize"])
	require.Equal(t, 0, params["cursor"])
	require.Equal(t, 99, params["sortType"])
	require.Equal(t, true, params["showInner"])

	// 推荐第 3 页:cursor = 2*20。
	params, err = List.MapRequest(&mmpb.CommentListRequest{
		ResourceType: song, ResourceId: 347230, PageNo: 3, PageSize: 20,
	})
	require.NoError(t, err)
	require.Equal(t, 40, params["cursor"])

	// 热度排序:cursor 带 normalHot# 前缀。
	params, err = List.MapRequest(&mmpb.CommentListRequest{
		ResourceType: song, ResourceId: 347230, Sort: mmpb.CommentSortType_COMMENT_SORT_TYPE_HOT, PageNo: 2, PageSize: 30,
	})
	require.NoError(t, err)
	require.Equal(t, "normalHot#30", params["cursor"])
	require.Equal(t, 2, params["sortType"])

	// 时间排序:cursor 透传,缺省补 "0"。
	params, err = List.MapRequest(&mmpb.CommentListRequest{
		ResourceType: song, ResourceId: 347230, Sort: mmpb.CommentSortType_COMMENT_SORT_TYPE_TIME,
	})
	require.NoError(t, err)
	require.Equal(t, "0", params["cursor"])
	require.Equal(t, 3, params["sortType"])
	params, err = List.MapRequest(&mmpb.CommentListRequest{
		ResourceType: song, ResourceId: 347230, Sort: mmpb.CommentSortType_COMMENT_SORT_TYPE_TIME, Cursor: "1592474013389",
	})
	require.NoError(t, err)
	require.Equal(t, "1592474013389", params["cursor"])

	_, err = List.MapRequest(&mmpb.CommentListRequest{ResourceId: 1})
	require.Error(t, err, "未指定资源类型必须报错")
}

func TestHot_MapRequestAndPath(t *testing.T) {
	t.Parallel()

	album := mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_ALBUM
	req := &mmpb.HotCommentsRequest{ResourceType: album, ResourceId: 34209, Limit: 10, Offset: 20, BeforeTime: 1700000000000}
	params, err := Hot.MapRequest(req)
	require.NoError(t, err)
	require.Equal(t, int64(34209), params["rid"])
	require.Equal(t, int32(10), params["limit"])
	require.Equal(t, int32(20), params["offset"])
	require.Equal(t, int64(1700000000000), params["beforeTime"])
	require.Equal(t, "/weapi/v1/resource/hotcomments/R_AL_3_34209", Hot.PathFunc(req))

	_, err = Hot.MapRequest(&mmpb.HotCommentsRequest{ResourceId: 1})
	require.Error(t, err)
}

// 写接口 body 形状(2026-10 真实上游验证:匿名一律 code=301 拦截,形状按
// NeteaseCloudMusicApi comment / comment_like 声明)。
func TestWrite_MapRequest(t *testing.T) {
	t.Parallel()

	mv := mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_MV

	params, err := Like.MapRequest(&mmpb.LikeCommentRequest{ResourceType: mv, ResourceId: 14514682, CommentId: 5031795, Like: true})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"threadId": "R_MV_5_14514682", "commentId": int64(5031795)}, params)
	require.Equal(t, "/weapi/v1/comment/like", Like.PathFunc(&mmpb.LikeCommentRequest{Like: true}))
	require.Equal(t, "/weapi/v1/comment/unlike", Like.PathFunc(&mmpb.LikeCommentRequest{Like: false}))

	params, err = Add.MapRequest(&mmpb.AddCommentRequest{ResourceType: mv, ResourceId: 14514682, Content: "测试评论"})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"threadId": "R_MV_5_14514682", "content": "测试评论"}, params)

	params, err = Reply.MapRequest(&mmpb.ReplyCommentRequest{ResourceType: mv, ResourceId: 14514682, CommentId: 7, Content: "回复"})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"threadId": "R_MV_5_14514682", "commentId": int64(7), "content": "回复"}, params)

	params, err = Delete.MapRequest(&mmpb.DeleteCommentRequest{ResourceType: mv, ResourceId: 14514682, CommentId: 7})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"threadId": "R_MV_5_14514682", "commentId": int64(7)}, params)

	// Like 的响应回填请求值(上游响应仅 code)。
	resp, err := Like.MapResponse(&mmpb.LikeCommentRequest{Like: false}, []byte(`{"code":200}`))
	require.NoError(t, err)
	require.False(t, resp.GetLiked())
}

// liked 是 per-user 字段:读接口必须 AuthOptional(带 cookie 读已赞态、匿名读公开数据),
// 写接口必须 AuthLoggedIn(与收藏专辑同形)。
func TestComment_AuthLevels(t *testing.T) {
	t.Parallel()

	require.Equal(t, session.AuthOptional, List.Meta.Auth)
	require.Equal(t, session.AuthOptional, Hot.Meta.Auth)
	for _, e := range []struct {
		name string
		auth session.AuthRequirement
	}{{"Like", Like.Meta.Auth}, {"Add", Add.Meta.Auth}, {"Reply", Reply.Meta.Auth}, {"Delete", Delete.Meta.Auth}} {
		require.Equal(t, session.AuthLoggedIn, e.auth, e.name)
	}
}

// beReplied 为 null(实测所有样本均 null)与带回复引用两种形态都解得动;
// 带引用分支的形状出自上游文档口径,未在真实样本中出现过,故用合成夹具。
func TestList_MapResponse_SyntheticBeReplied(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"code": 200,
		"data": {
			"totalCount": 2,
			"hasMore": false,
			"cursor": "1700000000000",
			"comments": [
				{
					"commentId": 11, "content": "带回复", "time": 1700000000000, "timeStr": "2023-11-14",
					"likedCount": 3, "liked": true, "user": {"userId": 1, "nickname": "甲", "avatarUrl": "http://x/1.jpg", "vipType": 0},
					"beReplied": [{"beRepliedCommentId": 22, "content": "被回复", "user": {"userId": 2, "nickname": "乙", "avatarUrl": "http://x/2.jpg", "vipType": 11}}]
				},
				{"commentId": 33, "content": "无回复", "beReplied": null, "user": null}
			]
		}
	}`)
	resp, err := List.MapResponse(&mmpb.CommentListRequest{}, raw)
	require.NoError(t, err)
	require.Len(t, resp.GetComments(), 2)
	require.Equal(t, int64(2), resp.GetTotalCount())
	require.False(t, resp.GetHasMore())
	require.Equal(t, "1700000000000", resp.GetCursor())

	first := resp.GetComments()[0]
	require.True(t, first.GetLiked())
	require.Equal(t, "甲", first.GetUser().GetNickname())
	require.Len(t, first.GetBeReplied(), 1)
	require.Equal(t, int64(22), first.GetBeReplied()[0].GetCommentId())
	require.Equal(t, "乙", first.GetBeReplied()[0].GetUser().GetNickname())

	second := resp.GetComments()[1]
	require.Empty(t, second.GetBeReplied(), "null 的 beReplied 解成空切片,不报错")
	require.NotNil(t, second.GetUser(), "缺 user 时回填空 CommentUser,不 panic")
}
