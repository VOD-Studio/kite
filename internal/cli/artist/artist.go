// Package artist 提供歌手相关命令。
package artist

import (
	"fmt"

	"github.com/spf13/cobra"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/cli/kit"
	artistendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/artist"
)

// NewCommand 创建 artist 命令组。
func NewCommand(k *kit.Kit) *cobra.Command {
	c := &cobra.Command{
		Use:   "artist",
		Short: "歌手相关命令",
		Args:  cobra.NoArgs,
	}
	c.AddCommand(
		newDetail(k), newSongs(k), newTopSongs(k), newAlbums(k),
		newDesc(k), newSimilar(k), newFans(k), newToplist(k),
		newSubscribe(k),
	)
	annotateArtistRpcs(c)
	return c
}

// annotateArtistRpcs 给 artist 组各子命令打 rpc 注解(PRD-0014 #B 命令树守护)。
// albums 走裸 RawDo(/weapi/artist/albums/{id}),不经 endpoint,但对应 grpc rpc Albums,
// 必须显式标注。toplist↔TopArtists、subscribe↔ArtistSubscribe 为命名偏移。
func annotateArtistRpcs(c *cobra.Command) {
	rpcs := map[string][]string{
		"detail":    {"ArtistService/GetArtist"},
		"songs":     {"ArtistService/AllSongs"},
		"top-songs": {"ArtistService/TopSongs"},
		"albums":    {"ArtistService/Albums"},
		"desc":      {"ArtistService/Desc"},
		"similar":   {"ArtistService/Similar"},
		"fans":      {"ArtistService/Fans"},
		"toplist":   {"ArtistService/TopArtists"},
		"subscribe": {"ArtistService/ArtistSubscribe"},
	}
	for _, sub := range c.Commands() {
		if v, ok := rpcs[sub.Name()]; ok {
			kit.AnnotateRpcs(sub, v...)
		}
	}
}

func newDetail(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "detail", "歌手详情", "歌手 ID", artistendpoint.GetArtist,
		func(id int64) *mmpb.GetArtistRequest { return &mmpb.GetArtistRequest{ArtistId: id} })
}

func newSongs(k *kit.Kit) *cobra.Command {
	var limit, offset int
	return kit.ReadCmd(k, "songs", "歌手全部歌曲", "歌手 ID", artistendpoint.AllSongs,
		func(id int64) *mmpb.AllSongsRequest {
			return &mmpb.AllSongsRequest{ArtistId: id, Limit: int32(limit), Offset: int32(offset)}
		},
		func(c *cobra.Command) {
			c.Flags().IntVar(&limit, "limit", 20, "返回数量")
			c.Flags().IntVar(&offset, "offset", 0, "偏移量")
		})
}

func newTopSongs(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "top-songs", "歌手热门歌曲", "歌手 ID", artistendpoint.TopSongs,
		func(id int64) *mmpb.TopSongsRequest { return &mmpb.TopSongsRequest{ArtistId: id} })
}

func newAlbums(k *kit.Kit) *cobra.Command {
	var limit, offset int
	return kit.ReadCmd(k, "albums", "歌手专辑列表", "歌手 ID", artistendpoint.Albums,
		func(id int64) *mmpb.AlbumsRequest {
			return &mmpb.AlbumsRequest{ArtistId: id, Limit: int32(limit), Offset: int32(offset)}
		},
		func(c *cobra.Command) {
			c.Flags().IntVar(&limit, "limit", 10, "返回数量")
			c.Flags().IntVar(&offset, "offset", 0, "偏移量")
		})
}

func newDesc(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "desc", "歌手简介", "歌手 ID", artistendpoint.Desc,
		func(id int64) *mmpb.DescRequest { return &mmpb.DescRequest{ArtistId: id} })
}

func newSimilar(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "similar", "相似歌手", "歌手 ID", artistendpoint.Similar,
		func(id int64) *mmpb.SimilarRequest { return &mmpb.SimilarRequest{ArtistId: id} })
}

func newFans(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "fans", "歌手粉丝", "歌手 ID", artistendpoint.Fans,
		func(id int64) *mmpb.FansRequest { return &mmpb.FansRequest{ArtistId: id} })
}

func newToplist(k *kit.Kit) *cobra.Command {
	var limit, offset int
	return kit.ReadCmd(k, "toplist", "歌手榜", "", artistendpoint.TopArtists,
		func(int64) *mmpb.TopArtistsRequest {
			return &mmpb.TopArtistsRequest{Limit: int32(limit), Offset: int32(offset)}
		},
		func(c *cobra.Command) {
			c.Flags().IntVar(&limit, "limit", 10, "返回数量")
			c.Flags().IntVar(&offset, "offset", 0, "偏移量")
		})
}

func newSubscribe(k *kit.Kit) *cobra.Command {
	return kit.WriteCmd(k, "subscribe", "收藏/取消收藏歌手(写操作,登录态)", "歌手 ID", "", artistendpoint.Subscribe,
		func(req *mmpb.ArtistSubscribeRequest) string {
			return fmt.Sprintf("收藏/取消收藏歌手 %d", req.GetArtistId())
		},
		func(id int64) *mmpb.ArtistSubscribeRequest { return &mmpb.ArtistSubscribeRequest{ArtistId: id} })
}
