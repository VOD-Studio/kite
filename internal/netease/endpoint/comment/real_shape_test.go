package comment

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
)

// 用从真实上游抓取的响应验证(说明见 album/real_shape_test.go;夹具裁掉了
// 与解码无关的字段,昵称/头像替换为合成数据,encryptUserId 等标识字段剔除)。
//
// 两个读接口 2026-10 对真实上游匿名调用均可用:
//   - List:/eapi/v2/resource/comments(歌曲/专辑/MV/歌单四类资源实测均返回数据)
//   - Hot:/weapi/v1/resource/hotcomments/{threadId}
//
// 单元夹具与解码器出自同一个假设会互相印证成空——路径/信封/字段名以这里的
// 真实形状为准。
func TestList_RealShape(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("testdata/new_song.json")
	require.NoError(t, err)
	resp, err := List.MapResponse(&mmpb.CommentListRequest{
		ResourceType: mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_SONG, ResourceId: 347230,
	}, raw)
	require.NoError(t, err)
	require.NotEmpty(t, resp.GetComments(), "评论为空说明解码器与上游响应形状不符")
	require.Equal(t, int64(70425), resp.GetTotalCount())
	require.True(t, resp.GetHasMore())
	require.Equal(t, "1592474013389", resp.GetCursor())

	first := resp.GetComments()[0]
	require.Equal(t, int64(5031795), first.GetCommentId())
	require.Equal(t, "有些人死了，他还活着", first.GetContent())
	require.Equal(t, int64(1413809782401), first.GetTime())
	require.Equal(t, "2014-10-20", first.GetTimeStr())
	require.Equal(t, int64(224976), first.GetLikedCount())
	require.Equal(t, int32(68), first.GetReplyCount())
	require.False(t, first.GetLiked(), "匿名调用的 liked 应为 false")
	require.NotEmpty(t, first.GetUser().GetNickname(), "作者昵称为空说明 user 解码错了位置")
}

func TestHot_RealShape(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("testdata/hot_song.json")
	require.NoError(t, err)
	resp, err := Hot.MapResponse(&mmpb.HotCommentsRequest{
		ResourceType: mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_SONG, ResourceId: 347230,
	}, raw)
	require.NoError(t, err)
	require.NotEmpty(t, resp.GetHotComments(), "热评为空说明解码器与上游响应形状不符")
	require.Equal(t, int64(1450), resp.GetTotal())
	require.True(t, resp.GetHasMore())
	require.Empty(t, resp.GetTopComments())

	first := resp.GetHotComments()[0]
	require.Equal(t, int64(5031795), first.GetCommentId())
	require.Equal(t, int64(224976), first.GetLikedCount())
	// 热评接口的评论对象无顶层 replyCount、showFloorComment 也为 null(实测),
	// 回计数解出 0;列表接口才有(replyCount 兜底见 toComment)。
	require.Equal(t, int32(0), first.GetReplyCount())
	require.NotEmpty(t, first.GetUser().GetNickname())
}
