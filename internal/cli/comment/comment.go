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

// resourceTypeName enum → 展示名(确认文案与 --help 默认值用)。
var resourceTypeName = map[mmpb.CommentResourceType]string{
	mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_SONG:     "song",
	mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_MV:       "mv",
	mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_PLAYLIST: "playlist",
	mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_ALBUM:    "album",
}

// sortTypeName enum → 展示名。
var sortTypeName = map[mmpb.CommentSortType]string{
	mmpb.CommentSortType_COMMENT_SORT_TYPE_RECOMMEND: "recommend",
	mmpb.CommentSortType_COMMENT_SORT_TYPE_HOT:       "hot",
	mmpb.CommentSortType_COMMENT_SORT_TYPE_TIME:      "time",
}

// resourceTypeValue --type 的 pflag.Value:枚举合法性校验前移到 flag 解析期,
// 坏值由 Set 报错(root 的 SetFlagErrorFunc 统一包 ErrUsage,退出码 2),
// 构造器的 req 闭包拿到的即已验证枚举,不必在 RunE 里手写校验样板。
type resourceTypeValue struct{ v mmpb.CommentResourceType }

func (f *resourceTypeValue) String() string { return resourceTypeName[f.v] }
func (f *resourceTypeValue) Type() string   { return "string" } // help 显示 --type string,与迁移前一致
func (f *resourceTypeValue) Set(s string) error {
	rt, err := strToResourceType(s)
	if err != nil {
		return err
	}
	f.v = rt
	return nil
}

// sortTypeValue --sort 的 pflag.Value,机制同 resourceTypeValue。
type sortTypeValue struct{ v mmpb.CommentSortType }

func (f *sortTypeValue) String() string { return sortTypeName[f.v] }
func (f *sortTypeValue) Type() string   { return "string" } // 同上,help 显示 --sort string
func (f *sortTypeValue) Set(s string) error {
	st, err := strToSort(s)
	if err != nil {
		return err
	}
	f.v = st
	return nil
}

// withTypeFlags 注册必填 --type(其余命令私有 flag 由调用方续接)。
func withTypeFlags(rt *resourceTypeValue) kit.CmdFlags {
	return func(c *cobra.Command) {
		c.Flags().Var(rt, "type", "资源类型: song/mv/playlist/album")
		_ = c.MarkFlagRequired("type")
	}
}

func newList(k *kit.Kit) *cobra.Command {
	var rt resourceTypeValue
	var st sortTypeValue
	var page, size int
	var cursor string
	st.v = mmpb.CommentSortType_COMMENT_SORT_TYPE_RECOMMEND
	return kit.ReadCmd(k, "list", "评论列表(推荐/热度/时间排序,分页)", "资源 ID", commentendpoint.List,
		func(id int64) *mmpb.CommentListRequest {
			return &mmpb.CommentListRequest{
				ResourceType: rt.v, ResourceId: id, Sort: st.v,
				PageNo: int32(page), PageSize: int32(size), Cursor: cursor,
			}
		},
		withTypeFlags(&rt),
		func(c *cobra.Command) {
			c.Flags().Var(&st, "sort", "排序: recommend/hot/time(时间排序翻页用 --cursor)")
			c.Flags().IntVar(&page, "page", 1, "页码,1 起")
			c.Flags().IntVar(&size, "size", 20, "每页数量")
			c.Flags().StringVar(&cursor, "cursor", "", "时间排序的翻页游标(上一页输出的 cursor)")
		})
}

func newHot(k *kit.Kit) *cobra.Command {
	var rt resourceTypeValue
	var limit, offset int
	var before int64
	return kit.ReadCmd(k, "hot", "热门评论", "资源 ID", commentendpoint.Hot,
		func(id int64) *mmpb.HotCommentsRequest {
			return &mmpb.HotCommentsRequest{
				ResourceType: rt.v, ResourceId: id,
				Limit: int32(limit), Offset: int32(offset), BeforeTime: before,
			}
		},
		withTypeFlags(&rt),
		func(c *cobra.Command) {
			c.Flags().IntVar(&limit, "limit", 20, "返回数量")
			c.Flags().IntVar(&offset, "offset", 0, "偏移量")
			c.Flags().Int64Var(&before, "before", 0, "取该毫秒时间之前的热评(翻页续传)")
		})
}

func newLike(k *kit.Kit) *cobra.Command {
	var rt resourceTypeValue
	var commentID int64
	var unlike bool
	return kit.WriteCmd(k, "like", "点赞/取消点赞评论(写操作,登录态)", "资源 ID", "", commentendpoint.Like,
		func(req *mmpb.LikeCommentRequest) string {
			action := "点赞"
			if unlike {
				action = "取消点赞"
			}
			return fmt.Sprintf("%s %s %d 的评论 %d",
				action, resourceTypeName[req.GetResourceType()], req.GetResourceId(), req.GetCommentId())
		},
		func(id int64) *mmpb.LikeCommentRequest {
			return &mmpb.LikeCommentRequest{
				ResourceType: rt.v, ResourceId: id, CommentId: commentID, Like: !unlike,
			}
		},
		withTypeFlags(&rt),
		func(c *cobra.Command) {
			c.Flags().Int64Var(&commentID, "comment-id", 0, "评论 ID")
			_ = c.MarkFlagRequired("comment-id")
			c.Flags().BoolVar(&unlike, "unlike", false, "取消点赞(默认点赞)")
		})
}

func newAdd(k *kit.Kit) *cobra.Command {
	var rt resourceTypeValue
	var content string
	return kit.WriteCmd(k, "add", "发表评论(写操作,登录态)", "资源 ID", "", commentendpoint.Add,
		func(req *mmpb.AddCommentRequest) string {
			return fmt.Sprintf("在 %s %d 发表评论:%s",
				resourceTypeName[req.GetResourceType()], req.GetResourceId(), req.GetContent())
		},
		func(id int64) *mmpb.AddCommentRequest {
			return &mmpb.AddCommentRequest{ResourceType: rt.v, ResourceId: id, Content: content}
		},
		withTypeFlags(&rt),
		func(c *cobra.Command) {
			c.Flags().StringVar(&content, "content", "", "评论内容")
			_ = c.MarkFlagRequired("content")
		})
}

func newReply(k *kit.Kit) *cobra.Command {
	var rt resourceTypeValue
	var commentID int64
	var content string
	return kit.WriteCmd(k, "reply", "回复评论(写操作,登录态)", "资源 ID", "", commentendpoint.Reply,
		func(req *mmpb.ReplyCommentRequest) string {
			return fmt.Sprintf("回复 %s %d 的评论 %d:%s",
				resourceTypeName[req.GetResourceType()], req.GetResourceId(), req.GetCommentId(), req.GetContent())
		},
		func(id int64) *mmpb.ReplyCommentRequest {
			return &mmpb.ReplyCommentRequest{
				ResourceType: rt.v, ResourceId: id, CommentId: commentID, Content: content,
			}
		},
		withTypeFlags(&rt),
		func(c *cobra.Command) {
			c.Flags().Int64Var(&commentID, "comment-id", 0, "被回复的评论 ID")
			_ = c.MarkFlagRequired("comment-id")
			c.Flags().StringVar(&content, "content", "", "回复内容")
			_ = c.MarkFlagRequired("content")
		})
}

func newDelete(k *kit.Kit) *cobra.Command {
	var rt resourceTypeValue
	var commentID int64
	return kit.WriteCmd(k, "delete", "删除自己的评论(写操作,登录态)", "资源 ID", "", commentendpoint.Delete,
		func(req *mmpb.DeleteCommentRequest) string {
			return fmt.Sprintf("删除 %s %d 的评论 %d(只能删自己发的)",
				resourceTypeName[req.GetResourceType()], req.GetResourceId(), req.GetCommentId())
		},
		func(id int64) *mmpb.DeleteCommentRequest {
			return &mmpb.DeleteCommentRequest{ResourceType: rt.v, ResourceId: id, CommentId: commentID}
		},
		withTypeFlags(&rt),
		func(c *cobra.Command) {
			c.Flags().Int64Var(&commentID, "comment-id", 0, "评论 ID")
			_ = c.MarkFlagRequired("comment-id")
		})
}
