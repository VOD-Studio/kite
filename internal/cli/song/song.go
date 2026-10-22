// Package song 提供歌曲相关命令。
package song

import (
	"fmt"

	"github.com/spf13/cobra"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/cli/kit"
	songendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/song"
)

// NewCommand 创建 song 命令组。
func NewCommand(k *kit.Kit) *cobra.Command {
	c := &cobra.Command{
		Use:   "song",
		Short: "歌曲相关命令",
		Args:  cobra.NoArgs,
	}
	c.AddCommand(
		newDetail(k), newURL(k), newLyric(k), newWordLyric(k),
		newCheckAvailable(k), newQualityDetail(k), newLikeCount(k),
		newDynamicCover(k), newChorusTime(k), newCreatorInfo(k),
		newSimilarSongs(k),
		newLike(k), newTrash(k), newDisallowRecommend(k),
		newLikedList(k), newIsLike(k),
		newDownload(k),
	)
	annotateSongRpcs(c)
	return c
}

// annotateSongRpcs 给 song 组各子命令打 rpc 注解(PRD-0014 #B 命令树守护)。
// 值为 grpc rpc 短形式 Service/Method;download 多 rpc 逗号分隔。
// endpoint 变量名 ≠ rpc method 名(URL↔GetSongURL、WordLyricEP↔GetWordLyric)。
func annotateSongRpcs(c *cobra.Command) {
	rpcs := map[string][]string{
		"detail":             {"SongService/GetSongDetail"},
		"url":                {"SongService/GetSongURL"},
		"lyric":              {"SongService/GetLyric"},
		"word-lyric":         {"SongService/GetWordLyric"},
		"check-available":    {"SongService/CheckAvailable"},
		"quality-detail":     {"SongService/QualityDetail"},
		"like-count":         {"SongService/LikeCount"},
		"dynamic-cover":      {"SongService/DynamicCover"},
		"chorus-time":        {"SongService/ChorusTime"},
		"creator-info":       {"SongService/CreatorInfo"},
		"similar-songs":      {"SongService/SimilarSongs"},
		"like":               {"SongService/Like"},
		"trash":              {"SongService/Trash"},
		"disallow-recommend": {"SongService/DisallowRecommend"},
		"liked-list":         {"SongService/LikedList"},
		"is-like":            {"SongService/IsLike"},
		// download 消费多个读 rpc(fetchURL + fetchDetail)。
		"download": {"SongService/GetSongURL", "SongService/GetSongDetail"},
	}
	for _, sub := range c.Commands() {
		if v, ok := rpcs[sub.Name()]; ok {
			kit.AnnotateRpcs(sub, v...)
		}
	}
}

func newDetail(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "detail", "歌曲详情", "歌曲 ID", songendpoint.Detail,
		func(id int64) *mmpb.GetSongDetailRequest { return &mmpb.GetSongDetailRequest{SongId: id} })
}

func newURL(k *kit.Kit) *cobra.Command {
	var level int
	return kit.ReadCmd(k, "url", "歌曲播放地址", "歌曲 ID", songendpoint.URL,
		func(id int64) *mmpb.GetSongURLRequest {
			return &mmpb.GetSongURLRequest{SongId: id, Level: mmpb.SongLevel(level)}
		},
		func(c *cobra.Command) {
			c.Flags().IntVar(&level, "level", k.Config.Level, "音质: 1=standard 2=exhigh 3=lossless 4=hires")
		})
}

func newLyric(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "lyric", "歌词", "歌曲 ID", songendpoint.Lyric,
		func(id int64) *mmpb.GetLyricRequest { return &mmpb.GetLyricRequest{SongId: id} })
}

func newWordLyric(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "word-lyric", "逐字歌词", "歌曲 ID", songendpoint.WordLyricEP,
		func(id int64) *mmpb.GetWordLyricRequest { return &mmpb.GetWordLyricRequest{SongId: id} })
}

func newCheckAvailable(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "check-available", "检查歌曲是否可用", "歌曲 ID", songendpoint.CheckAvailable,
		func(id int64) *mmpb.CheckAvailableRequest { return &mmpb.CheckAvailableRequest{SongId: id} })
}

func newQualityDetail(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "quality-detail", "音质详情(eapi)", "歌曲 ID", songendpoint.QualityDetail,
		func(id int64) *mmpb.QualityDetailRequest { return &mmpb.QualityDetailRequest{SongId: id} })
}

func newLikeCount(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "like-count", "红心数(eapi)", "歌曲 ID", songendpoint.LikeCount,
		func(id int64) *mmpb.LikeCountRequest { return &mmpb.LikeCountRequest{SongId: id} })
}

func newDynamicCover(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "dynamic-cover", "动态封面(eapi)", "歌曲 ID", songendpoint.DynamicCover,
		func(id int64) *mmpb.DynamicCoverRequest { return &mmpb.DynamicCoverRequest{SongId: id} })
}

func newChorusTime(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "chorus-time", "副歌(eapi)", "歌曲 ID", songendpoint.ChorusTime,
		func(id int64) *mmpb.ChorusTimeRequest { return &mmpb.ChorusTimeRequest{SongId: id} })
}

func newCreatorInfo(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "creator-info", "创作者(eapi)", "歌曲 ID", songendpoint.CreatorInfo,
		func(id int64) *mmpb.CreatorInfoRequest { return &mmpb.CreatorInfoRequest{SongId: id} })
}

func newSimilarSongs(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "similar-songs", "相似歌曲", "歌曲 ID", songendpoint.SimilarSongs,
		func(id int64) *mmpb.SimilarSongsRequest { return &mmpb.SimilarSongsRequest{SongId: id} })
}

func newLike(k *kit.Kit) *cobra.Command {
	var on bool
	return kit.WriteCmd(k, "like", "红心/取消红心(写操作,登录态)", "歌曲 ID", "", songendpoint.Like,
		func(req *mmpb.LikeRequest) string {
			action := "取消红心"
			if req.GetLike() {
				action = "点红心"
			}
			return fmt.Sprintf("对歌曲 %d %s", req.GetSongId(), action)
		},
		func(id int64) *mmpb.LikeRequest { return &mmpb.LikeRequest{SongId: id, Like: on} },
		func(c *cobra.Command) {
			c.Flags().BoolVar(&on, "on", false, "红心(true)/取消(false)")
		})
}

func newTrash(k *kit.Kit) *cobra.Command {
	return kit.WriteCmd(k, "trash", "丢进垃圾桶,降推荐权重(写操作,登录态)", "歌曲 ID", "", songendpoint.Trash,
		func(req *mmpb.TrashRequest) string {
			return fmt.Sprintf("把歌曲 %d 丢进垃圾桶(降推荐权重)", req.GetSongId())
		},
		func(id int64) *mmpb.TrashRequest { return &mmpb.TrashRequest{SongId: id} })
}

func newDisallowRecommend(k *kit.Kit) *cobra.Command {
	return kit.WriteCmd(k, "disallow-recommend", "标记不感兴趣,影响日推(写操作,登录态)", "歌曲 ID", "",
		songendpoint.DisallowRecommend,
		func(req *mmpb.DisallowRecommendRequest) string {
			return fmt.Sprintf("标记歌曲 %d 不感兴趣(影响日推)", req.GetSongId())
		},
		func(id int64) *mmpb.DisallowRecommendRequest {
			return &mmpb.DisallowRecommendRequest{SongId: id}
		})
}

func newLikedList(k *kit.Kit) *cobra.Command {
	var uid int64
	return kit.LoginReadCmd(k, "liked-list", "红心歌曲列表(登录态)", "", songendpoint.LikedList,
		func(int64) *mmpb.LikedListRequest { return &mmpb.LikedListRequest{UserId: uid} },
		func(c *cobra.Command) {
			c.Flags().Int64Var(&uid, "uid", 0, "用户 ID")
			_ = c.MarkFlagRequired("uid")
		})
}

func newIsLike(k *kit.Kit) *cobra.Command {
	return kit.LoginReadCmd(k, "is-like", "是否已红心(登录态)", "歌曲 ID", songendpoint.IsLike,
		func(id int64) *mmpb.IsLikeRequest { return &mmpb.IsLikeRequest{SongId: id} })
}
