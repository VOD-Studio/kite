// Package playlist 提供歌单相关命令。
package playlist

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/cli/kit"
	"github.com/VOD-Studio/kite/internal/netease/compose"
	playlistendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/playlist"
)

// NewCommand 创建 playlist 命令组。
func NewCommand(k *kit.Kit) *cobra.Command {
	c := &cobra.Command{
		Use:   "playlist",
		Short: "歌单相关命令",
		Args:  cobra.NoArgs,
	}
	c.AddCommand(
		newDetail(k), newTracks(k), newSubscribers(k),
		newHighQuality(k), newHighQualityTags(k), newCatList(k), newHot(k),
		newSimilar(k), newRelated(k),
		newSubscribe(k), newCreate(k), newDelete(k),
		newUpdateName(k), newUpdateDesc(k), newUpdateTags(k), newUpdateTracks(k),
		newDownload(k),
	)
	annotatePlaylistRpcs(c)
	return c
}

// annotatePlaylistRpcs 给 playlist 组各子命令打 rpc 注解(PRD-0014 #B 命令树守护)。
// 命名偏移:hot↔BrowseHot、related↔RelatedPlaylistRecommend。
// download 跨 service 消费 3 rpc(AllTracks 取曲目 + GetPlaylist 取歌单名 + per-song GetSongURL)。
func annotatePlaylistRpcs(c *cobra.Command) {
	rpcs := map[string][]string{
		"detail":           {"PlaylistService/GetPlaylist"},
		"tracks":           {"PlaylistService/AllTracks"},
		"subscribers":      {"PlaylistService/Subscribers"},
		"highquality":      {"PlaylistService/HighQuality"},
		"highquality-tags": {"PlaylistService/HighQualityTags"},
		"catlist":          {"PlaylistService/CatList"},
		"hot":              {"PlaylistService/BrowseHot"},
		"similar":          {"PlaylistService/SimilarPlaylists"},
		"related":          {"PlaylistService/RelatedPlaylistRecommend"},
		"subscribe":        {"PlaylistService/Subscribe"},
		"create":           {"PlaylistService/Create"},
		"delete":           {"PlaylistService/Delete"},
		"update-name":      {"PlaylistService/UpdateName"},
		"update-desc":      {"PlaylistService/UpdateDesc"},
		"update-tags":      {"PlaylistService/UpdateTags"},
		"update-tracks":    {"PlaylistService/UpdateTracks"},
		"download":         {"PlaylistService/AllTracks", "PlaylistService/GetPlaylist", "SongService/GetSongURL"},
	}
	for _, sub := range c.Commands() {
		if v, ok := rpcs[sub.Name()]; ok {
			kit.AnnotateRpcs(sub, v...)
		}
	}
}

// parseIDs 把 "1,2,3" 解析成 int64 切片,空串或含非数字时报错。
func parseIDs(s string) ([]int64, error) {
	if strings.TrimSpace(s) == "" {
		return nil, fmt.Errorf("--tracks 不能为空")
	}
	parts := strings.Split(s, ",")
	ids := make([]int64, 0, len(parts))
	for _, p := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("无效的歌曲 ID %q", strings.TrimSpace(p))
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func newDetail(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "detail", "歌单详情", "歌单 ID", playlistendpoint.GetPlaylist,
		func(id int64) *mmpb.GetPlaylistRequest { return &mmpb.GetPlaylistRequest{PlaylistId: id} })
}

func newTracks(k *kit.Kit) *cobra.Command {
	var id int64
	c := &cobra.Command{
		Use:   "tracks",
		Short: "歌单全部歌曲",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			rid, err := kit.ResolveID(id, args)
			if err != nil {
				return err
			}
			return kit.RenderCompose(k, compose.PlaylistTracks, &mmpb.AllTracksRequest{PlaylistId: rid})
		},
	}
	c.Flags().Int64Var(&id, "id", 0, "歌单 ID")
	return c
}

func newSubscribers(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "subscribers", "歌单收藏者", "歌单 ID", playlistendpoint.Subscribers,
		func(id int64) *mmpb.SubscribersRequest { return &mmpb.SubscribersRequest{PlaylistId: id} })
}

func newHighQuality(k *kit.Kit) *cobra.Command {
	var cat string
	var limit int
	return kit.ReadCmd(k, "highquality", "精品歌单", "", playlistendpoint.HighQuality,
		func(int64) *mmpb.HighQualityRequest { return &mmpb.HighQualityRequest{Cat: cat, Limit: int32(limit)} },
		func(c *cobra.Command) {
			c.Flags().StringVar(&cat, "cat", "全部", "分类")
			c.Flags().IntVar(&limit, "limit", 10, "返回数量")
		})
}

func newHighQualityTags(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "highquality-tags", "精品歌单标签", "", playlistendpoint.HighQualityTags,
		func(int64) *mmpb.HighQualityTagsRequest { return &mmpb.HighQualityTagsRequest{} })
}

func newCatList(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "catlist", "歌单分类列表", "", playlistendpoint.CatList,
		func(int64) *mmpb.CatListRequest { return &mmpb.CatListRequest{} })
}

func newHot(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "hot", "热门歌单", "", playlistendpoint.BrowseHot,
		func(int64) *mmpb.BrowseHotRequest { return &mmpb.BrowseHotRequest{} })
}

func newSimilar(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "similar", "相似歌单(按歌曲)", "歌曲 ID", playlistendpoint.SimilarPlaylists,
		func(id int64) *mmpb.SimilarPlaylistsRequest { return &mmpb.SimilarPlaylistsRequest{SongId: id} })
}

func newRelated(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "related", "相关歌单推荐", "歌单 ID", playlistendpoint.RelatedPlaylistRecommend,
		func(id int64) *mmpb.RelatedPlaylistRecommendRequest {
			return &mmpb.RelatedPlaylistRecommendRequest{PlaylistId: id}
		})
}

func newSubscribe(k *kit.Kit) *cobra.Command {
	return kit.WriteCmd(k, "subscribe", "收藏/取消收藏歌单(写操作,登录态)", "歌单 ID", "", playlistendpoint.Subscribe,
		func(req *mmpb.SubscribeRequest) string {
			return fmt.Sprintf("收藏/取消收藏歌单 %d", req.GetPlaylistId())
		},
		func(id int64) *mmpb.SubscribeRequest { return &mmpb.SubscribeRequest{PlaylistId: id} })
}

func newCreate(k *kit.Kit) *cobra.Command {
	var name string
	var privacy bool
	return kit.WriteCmd(k, "create", "创建歌单(写操作,登录态)", "", "", playlistendpoint.Create,
		func(req *mmpb.CreateRequest) string { return fmt.Sprintf("创建歌单 %s", req.GetName()) },
		func(int64) *mmpb.CreateRequest { return &mmpb.CreateRequest{Name: name, Privacy: privacy} },
		func(c *cobra.Command) {
			c.Flags().StringVar(&name, "name", "", "歌单名")
			c.Flags().BoolVar(&privacy, "privacy", false, "是否隐私")
			_ = c.MarkFlagRequired("name")
		})
}

func newDelete(k *kit.Kit) *cobra.Command {
	return kit.WriteCmd(k, "delete", "删除歌单(写操作,登录态)", "歌单 ID", "已删除", playlistendpoint.Delete,
		func(req *mmpb.DeleteRequest) string { return fmt.Sprintf("删除歌单 %d", req.GetPlaylistId()) },
		func(id int64) *mmpb.DeleteRequest { return &mmpb.DeleteRequest{PlaylistId: id} })
}

func newUpdateName(k *kit.Kit) *cobra.Command {
	var name string
	return kit.WriteCmd(k, "update-name", "歌单改名(写操作,登录态)", "歌单 ID", "已更新", playlistendpoint.UpdateName,
		func(req *mmpb.UpdateNameRequest) string {
			return fmt.Sprintf("把歌单 %d 改名为 %q", req.GetPlaylistId(), req.GetName())
		},
		func(id int64) *mmpb.UpdateNameRequest { return &mmpb.UpdateNameRequest{PlaylistId: id, Name: name} },
		func(c *cobra.Command) {
			c.Flags().StringVar(&name, "name", "", "新名称")
			_ = c.MarkFlagRequired("name")
		})
}

func newUpdateDesc(k *kit.Kit) *cobra.Command {
	var desc string
	return kit.WriteCmd(k, "update-desc", "修改歌单描述(写操作,登录态)", "歌单 ID", "已更新", playlistendpoint.UpdateDesc,
		func(req *mmpb.UpdateDescRequest) string {
			return fmt.Sprintf("修改歌单 %d 的描述", req.GetPlaylistId())
		},
		func(id int64) *mmpb.UpdateDescRequest { return &mmpb.UpdateDescRequest{PlaylistId: id, Desc: desc} },
		func(c *cobra.Command) {
			c.Flags().StringVar(&desc, "desc", "", "新描述")
			_ = c.MarkFlagRequired("desc")
		})
}

func newUpdateTags(k *kit.Kit) *cobra.Command {
	var tags string
	return kit.WriteCmd(k, "update-tags", "修改歌单标签(写操作,登录态)", "歌单 ID", "已更新", playlistendpoint.UpdateTags,
		func(req *mmpb.UpdateTagsRequest) string {
			return fmt.Sprintf("把歌单 %d 的标签改为 %q", req.GetPlaylistId(), req.GetTags())
		},
		func(id int64) *mmpb.UpdateTagsRequest { return &mmpb.UpdateTagsRequest{PlaylistId: id, Tags: tags} },
		func(c *cobra.Command) {
			c.Flags().StringVar(&tags, "tags", "", "标签,分号分隔")
			_ = c.MarkFlagRequired("tags")
		})
}

func newUpdateTracks(k *kit.Kit) *cobra.Command {
	var id int64
	var opStr, tracks string
	c := &cobra.Command{
		Use:   "update-tracks",
		Short: "歌单增删歌曲(写操作,登录态)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			rid, err := kit.ResolveID(id, args)
			if err != nil {
				return err
			}
			ids, err := parseIDs(tracks)
			if err != nil {
				return err
			}
			op := mmpb.TracksOp_TRACKS_OP_ADD
			opText := "添加"
			switch opStr {
			case "add":
			case "del":
				op = mmpb.TracksOp_TRACKS_OP_DEL
				opText = "删除"
			default:
				return fmt.Errorf("无效的 --op %q: 只支持 add/del", opStr)
			}
			if err := k.RequireLogin(); err != nil {
				return err
			}
			if err := k.ConfirmFatal(fmt.Sprintf("对歌单 %d %s歌曲 %v", rid, opText, ids)); err != nil {
				return err
			}
			return kit.RenderExec(k, playlistendpoint.UpdateTracks, &mmpb.UpdateTracksRequest{PlaylistId: rid, Op: op, TrackIds: ids})
		},
	}
	c.Flags().Int64Var(&id, "id", 0, "歌单 ID")
	c.Flags().StringVar(&opStr, "op", "add", "操作: add=添加 del=删除")
	c.Flags().StringVar(&tracks, "tracks", "", "歌曲 ID,逗号分隔")
	_ = c.MarkFlagRequired("tracks")
	return c
}
