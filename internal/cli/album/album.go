// Package album 提供专辑相关命令。
package album

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/cli/kit"
	albumendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/album"
)

// NewCommand 创建 album 命令组。
func NewCommand(k *kit.Kit) *cobra.Command {
	c := &cobra.Command{
		Use:   "album",
		Short: "专辑相关命令",
		Args:  cobra.NoArgs,
	}
	c.AddCommand(
		newShelf(k), newNewest(k), newAll(k),
		newDetail(k), newDynamic(k), newSongQuality(k),
		newSubscribe(k), newUnsubscribe(k), newSubscribed(k),
	)
	annotateAlbumRpcs(c)
	return c
}

// annotateAlbumRpcs 给 album 组各子命令打 rpc 注解(PRD-0014 #B 命令树守护)。
// detail 走裸 RawDo(/weapi/v1/album/{id}),不经 endpoint,但对应 grpc rpc GetAlbum,
// 必须显式标注,否则反向 diff 会把 GetAlbum 误判为「无 CLI」。
// endpoint 变量名 ≠ rpc method 名(Subscribe↔SubscribeAlbum、Unsubscribe↔UnsubscribeAlbum)。
func annotateAlbumRpcs(c *cobra.Command) {
	rpcs := map[string][]string{
		"shelf":        {"AlbumService/NewAlbumShelf"},
		"newest":       {"AlbumService/NewestAlbums"},
		"all":          {"AlbumService/AllNewAlbums"},
		"detail":       {"AlbumService/GetAlbum"},
		"dynamic":      {"AlbumService/AlbumDynamic"},
		"song-quality": {"AlbumService/AlbumSongQuality"},
		"subscribe":    {"AlbumService/SubscribeAlbum"},
		"unsubscribe":  {"AlbumService/UnsubscribeAlbum"},
		"subscribed":   {"AlbumService/SubscribedAlbums"},
	}
	for _, sub := range c.Commands() {
		if v, ok := rpcs[sub.Name()]; ok {
			kit.AnnotateRpcs(sub, v...)
		}
	}
}

// strToArea 地区字符串转 enum。
func strToArea(s string) mmpb.AlbumArea {
	switch strings.ToUpper(s) {
	case "ZH":
		return mmpb.AlbumArea_ALBUM_AREA_ZH
	case "EA":
		return mmpb.AlbumArea_ALBUM_AREA_EA
	case "KR":
		return mmpb.AlbumArea_ALBUM_AREA_KR
	case "JP":
		return mmpb.AlbumArea_ALBUM_AREA_JP
	default:
		return mmpb.AlbumArea_ALBUM_AREA_ALL
	}
}

func newShelf(k *kit.Kit) *cobra.Command {
	var area string
	var limit int
	return kit.ReadCmd(k, "shelf", "新碟上架", "", albumendpoint.NewAlbumShelf,
		func(int64) *mmpb.NewAlbumShelfRequest {
			return &mmpb.NewAlbumShelfRequest{Area: strToArea(area), Limit: int32(limit)}
		},
		func(c *cobra.Command) {
			c.Flags().StringVar(&area, "area", "ALL", "地区: ALL/ZH/EA/KR/JP")
			c.Flags().IntVar(&limit, "limit", 10, "返回数量")
		})
}

func newNewest(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "newest", "最新专辑", "", albumendpoint.NewestAlbums,
		func(int64) *mmpb.NewestAlbumsRequest { return &mmpb.NewestAlbumsRequest{} })
}

func newAll(k *kit.Kit) *cobra.Command {
	var area string
	var limit, offset int
	return kit.ReadCmd(k, "all", "全部新碟", "", albumendpoint.AllNewAlbums,
		func(int64) *mmpb.AllNewAlbumsRequest {
			return &mmpb.AllNewAlbumsRequest{Area: strToArea(area), Limit: int32(limit), Offset: int32(offset)}
		},
		func(c *cobra.Command) {
			c.Flags().StringVar(&area, "area", "ALL", "地区: ALL/ZH/EA/KR/JP")
			c.Flags().IntVar(&limit, "limit", 10, "返回数量")
			c.Flags().IntVar(&offset, "offset", 0, "偏移量")
		})
}

func newDetail(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "detail", "专辑详情", "专辑 ID", albumendpoint.GetAlbum,
		func(id int64) *mmpb.GetAlbumRequest { return &mmpb.GetAlbumRequest{AlbumId: id} })
}

func newDynamic(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "dynamic", "专辑动态信息(收藏数等)", "专辑 ID", albumendpoint.AlbumDynamic,
		func(id int64) *mmpb.AlbumDynamicRequest { return &mmpb.AlbumDynamicRequest{AlbumId: id} })
}

func newSongQuality(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "song-quality", "专辑歌曲音质", "专辑 ID", albumendpoint.AlbumSongQuality,
		func(id int64) *mmpb.AlbumSongQualityRequest { return &mmpb.AlbumSongQualityRequest{AlbumId: id} })
}

func newSubscribe(k *kit.Kit) *cobra.Command {
	return kit.WriteCmd(k, "subscribe", "收藏专辑(写操作,登录态)", "专辑 ID", "", albumendpoint.Subscribe,
		func(req *mmpb.SubscribeAlbumRequest) string { return fmt.Sprintf("收藏专辑 %d", req.GetAlbumId()) },
		func(id int64) *mmpb.SubscribeAlbumRequest { return &mmpb.SubscribeAlbumRequest{AlbumId: id} })
}

func newUnsubscribe(k *kit.Kit) *cobra.Command {
	return kit.WriteCmd(k, "unsubscribe", "取消收藏专辑(写操作,登录态)", "专辑 ID", "", albumendpoint.Unsubscribe,
		func(req *mmpb.UnsubscribeAlbumRequest) string {
			return fmt.Sprintf("取消收藏专辑 %d", req.GetAlbumId())
		},
		func(id int64) *mmpb.UnsubscribeAlbumRequest { return &mmpb.UnsubscribeAlbumRequest{AlbumId: id} })
}

func newSubscribed(k *kit.Kit) *cobra.Command {
	var limit int
	return kit.LoginReadCmd(k, "subscribed", "已收藏专辑列表(登录态)", "", albumendpoint.SubscribedAlbums,
		func(int64) *mmpb.SubscribedAlbumsRequest { return &mmpb.SubscribedAlbumsRequest{Limit: int32(limit)} },
		func(c *cobra.Command) {
			c.Flags().IntVar(&limit, "limit", 25, "返回数量")
		})
}
