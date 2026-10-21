// Package comment 提供评论相关命令(#28 Phase A:直连 endpoint,不经 gRPC)。
package comment

import (
	"fmt"

	"github.com/spf13/cobra"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/cli/kit"
	commentendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/comment"
)

// NewCommand 创建 comment 命令组。
func NewCommand(k *kit.Kit) *cobra.Command {
	c := &cobra.Command{
		Use:   "comment",
		Short: "评论相关命令",
		Args:  cobra.NoArgs,
	}
	c.AddCommand(
		newList(k), newHot(k),
		newLike(k), newAdd(k), newReply(k), newDelete(k),
	)
	// 评论域 Phase A 尚无 proto rpc(#28 第二阶段建模);endpoint 直连,
	// 注解按「已审视,无 rpc」标空,第二阶段落 CommentService 后补注解。
	for _, sub := range c.Commands() {
		kit.AnnotateRpcs(sub)
	}
	return c
}

// strToResourceType 资源类型字符串转 enum。未知值报错(写操作打错类型
// 会作用到错误评论区,不静默回退)。
func strToResourceType(s string) (mmpb.CommentResourceType, error) {
	switch s {
	case "song":
		return mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_SONG, nil
	case "mv":
		return mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_MV, nil
	case "playlist":
		return mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_PLAYLIST, nil
	case "album":
		return mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_ALBUM, nil
	default:
		return 0, fmt.Errorf("未知资源类型 %q(可选: song/mv/playlist/album)", s)
	}
}

// strToSort 排序字符串转 enum,空串按推荐(上游默认)。
func strToSort(s string) (mmpb.CommentSortType, error) {
	switch s {
	case "", "recommend":
		return mmpb.CommentSortType_COMMENT_SORT_TYPE_RECOMMEND, nil
	case "hot":
		return mmpb.CommentSortType_COMMENT_SORT_TYPE_HOT, nil
	case "time":
		return mmpb.CommentSortType_COMMENT_SORT_TYPE_TIME, nil
	default:
		return 0, fmt.Errorf("未知排序 %q(可选: recommend/hot/time)", s)
	}
}

// withResource 注册 --type 资源类型与 --id 资源 ID flag(读/写命令共用)。
// --id 非必填,由位置参数补(同 album 的 withID 约定);--type 必填。
func withResource(c *cobra.Command, typ *string, id *int64) {
	c.Flags().StringVar(typ, "type", "", "资源类型: song/mv/playlist/album")
	_ = c.MarkFlagRequired("type")
	c.Flags().Int64Var(id, "id", 0, "资源 ID")
}

func newList(k *kit.Kit) *cobra.Command {
	var typ string
	var id int64
	var sort string
	var page, size int
	var cursor string
	c := &cobra.Command{
		Use:   "list",
		Short: "评论列表(推荐/热度/时间排序,分页)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			rid, err := kit.ResolveID(id, args)
			if err != nil {
				return err
			}
			rt, err := strToResourceType(typ)
			if err != nil {
				return err
			}
			st, err := strToSort(sort)
			if err != nil {
				return err
			}
			return kit.RenderExec(k, commentendpoint.List, &mmpb.CommentListRequest{
				ResourceType: rt, ResourceId: rid, Sort: st,
				PageNo: int32(page), PageSize: int32(size), Cursor: cursor,
			})
		},
	}
	withResource(c, &typ, &id)
	c.Flags().StringVar(&sort, "sort", "recommend", "排序: recommend/hot/time(时间排序翻页用 --cursor)")
	c.Flags().IntVar(&page, "page", 1, "页码,1 起")
	c.Flags().IntVar(&size, "size", 20, "每页数量")
	c.Flags().StringVar(&cursor, "cursor", "", "时间排序的翻页游标(上一页输出的 cursor)")
	return c
}

func newHot(k *kit.Kit) *cobra.Command {
	var typ string
	var id int64
	var limit, offset int
	var before int64
	c := &cobra.Command{
		Use:   "hot",
		Short: "热门评论",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			rid, err := kit.ResolveID(id, args)
			if err != nil {
				return err
			}
			rt, err := strToResourceType(typ)
			if err != nil {
				return err
			}
			return kit.RenderExec(k, commentendpoint.Hot, &mmpb.HotCommentsRequest{
				ResourceType: rt, ResourceId: rid,
				Limit: int32(limit), Offset: int32(offset), BeforeTime: before,
			})
		},
	}
	withResource(c, &typ, &id)
	c.Flags().IntVar(&limit, "limit", 20, "返回数量")
	c.Flags().IntVar(&offset, "offset", 0, "偏移量")
	c.Flags().Int64Var(&before, "before", 0, "取该毫秒时间之前的热评(翻页续传)")
	return c
}

func newLike(k *kit.Kit) *cobra.Command {
	var typ string
	var id, commentID int64
	var unlike bool
	c := &cobra.Command{
		Use:   "like",
		Short: "点赞/取消点赞评论(写操作,登录态)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			rid, err := kit.ResolveID(id, args)
			if err != nil {
				return err
			}
			rt, err := strToResourceType(typ)
			if err != nil {
				return err
			}
			if err := k.RequireLogin(); err != nil {
				return err
			}
			action := "点赞"
			if unlike {
				action = "取消点赞"
			}
			if err := k.ConfirmFatal(fmt.Sprintf("%s %s %d 的评论 %d", action, typ, rid, commentID)); err != nil {
				return err
			}
			return kit.RenderExec(k, commentendpoint.Like, &mmpb.LikeCommentRequest{
				ResourceType: rt, ResourceId: rid, CommentId: commentID, Like: !unlike,
			})
		},
	}
	withResource(c, &typ, &id)
	c.Flags().Int64Var(&commentID, "comment-id", 0, "评论 ID")
	_ = c.MarkFlagRequired("comment-id")
	c.Flags().BoolVar(&unlike, "unlike", false, "取消点赞(默认点赞)")
	return c
}

func newAdd(k *kit.Kit) *cobra.Command {
	var typ string
	var id int64
	var content string
	c := &cobra.Command{
		Use:   "add",
		Short: "发表评论(写操作,登录态)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			rid, err := kit.ResolveID(id, args)
			if err != nil {
				return err
			}
			rt, err := strToResourceType(typ)
			if err != nil {
				return err
			}
			if err := k.RequireLogin(); err != nil {
				return err
			}
			if err := k.ConfirmFatal(fmt.Sprintf("在 %s %d 发表评论:%s", typ, rid, content)); err != nil {
				return err
			}
			return kit.RenderExec(k, commentendpoint.Add, &mmpb.AddCommentRequest{
				ResourceType: rt, ResourceId: rid, Content: content,
			})
		},
	}
	withResource(c, &typ, &id)
	c.Flags().StringVar(&content, "content", "", "评论内容")
	_ = c.MarkFlagRequired("content")
	return c
}

func newReply(k *kit.Kit) *cobra.Command {
	var typ string
	var id, commentID int64
	var content string
	c := &cobra.Command{
		Use:   "reply",
		Short: "回复评论(写操作,登录态)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			rid, err := kit.ResolveID(id, args)
			if err != nil {
				return err
			}
			rt, err := strToResourceType(typ)
			if err != nil {
				return err
			}
			if err := k.RequireLogin(); err != nil {
				return err
			}
			if err := k.ConfirmFatal(fmt.Sprintf("回复 %s %d 的评论 %d:%s", typ, rid, commentID, content)); err != nil {
				return err
			}
			return kit.RenderExec(k, commentendpoint.Reply, &mmpb.ReplyCommentRequest{
				ResourceType: rt, ResourceId: rid, CommentId: commentID, Content: content,
			})
		},
	}
	withResource(c, &typ, &id)
	c.Flags().Int64Var(&commentID, "comment-id", 0, "被回复的评论 ID")
	_ = c.MarkFlagRequired("comment-id")
	c.Flags().StringVar(&content, "content", "", "回复内容")
	_ = c.MarkFlagRequired("content")
	return c
}

func newDelete(k *kit.Kit) *cobra.Command {
	var typ string
	var id, commentID int64
	c := &cobra.Command{
		Use:   "delete",
		Short: "删除自己的评论(写操作,登录态)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			rid, err := kit.ResolveID(id, args)
			if err != nil {
				return err
			}
			rt, err := strToResourceType(typ)
			if err != nil {
				return err
			}
			if err := k.RequireLogin(); err != nil {
				return err
			}
			if err := k.ConfirmFatal(fmt.Sprintf("删除 %s %d 的评论 %d(只能删自己发的)", typ, rid, commentID)); err != nil {
				return err
			}
			return kit.RenderExec(k, commentendpoint.Delete, &mmpb.DeleteCommentRequest{
				ResourceType: rt, ResourceId: rid, CommentId: commentID,
			})
		},
	}
	withResource(c, &typ, &id)
	c.Flags().Int64Var(&commentID, "comment-id", 0, "评论 ID")
	_ = c.MarkFlagRequired("comment-id")
	return c
}
