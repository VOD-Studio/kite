// Package user 提供用户相关命令。
package user

import (
	"github.com/spf13/cobra"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/cli/kit"
	"github.com/VOD-Studio/kite/internal/netease/compose"
	userendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/user"
)

// NewCommand 创建 user 命令组。
func NewCommand(k *kit.Kit) *cobra.Command {
	c := &cobra.Command{
		Use:   "user",
		Short: "用户相关命令",
		Args:  cobra.NoArgs,
	}
	c.AddCommand(
		newAccount(k), newDetail(k), newDetailByName(k), newSubCount(k),
		newPlaylists(k), newFollows(k), newFolloweds(k), newFollowEachOther(k),
		newRecord(k), newEvents(k), newLevel(k), newSimilar(k),
	)
	annotateUserRpcs(c)
	return c
}

// annotateUserRpcs 给 user 组各子命令打 rpc 注解(PRD-0014 #B 命令树守护)。
// 命名偏移:playlists↔UserPlaylist、follow-each-other↔FollowEachOther、similar↔SimilarUsers。
func annotateUserRpcs(c *cobra.Command) {
	rpcs := map[string][]string{
		"account":           {"UserService/Account"},
		"detail":            {"UserService/Detail"},
		"detail-by-name":    {"UserService/DetailByName"},
		"sub-count":         {"UserService/SubCount"},
		"playlists":         {"UserService/UserPlaylist"},
		"follows":           {"UserService/Follows"},
		"followeds":         {"UserService/Followeds"},
		"follow-each-other": {"UserService/FollowEachOther"},
		"record":            {"UserService/Record"},
		"events":            {"UserService/Events"},
		"level":             {"UserService/Level"},
		"similar":           {"UserService/SimilarUsers"},
	}
	for _, sub := range c.Commands() {
		if v, ok := rpcs[sub.Name()]; ok {
			kit.AnnotateRpcs(sub, v...)
		}
	}
}

// uidFlag 注册必填的 --uid flag(请求构造闭包读局部变量)。
func uidFlag(uid *int64) func(*cobra.Command) {
	return func(c *cobra.Command) {
		c.Flags().Int64Var(uid, "uid", 0, "用户 ID")
		_ = c.MarkFlagRequired("uid")
	}
}

func newAccount(k *kit.Kit) *cobra.Command {
	return kit.LoginReadCmd(k, "account", "当前账号信息(登录态)", "", userendpoint.Account,
		func(int64) *mmpb.AccountRequest { return &mmpb.AccountRequest{} })
}

func newDetail(k *kit.Kit) *cobra.Command {
	var uid int64
	return kit.ReadCmd(k, "detail", "用户详情", "", userendpoint.Detail,
		func(int64) *mmpb.DetailRequest { return &mmpb.DetailRequest{UserId: uid} }, uidFlag(&uid))
}

func newDetailByName(k *kit.Kit) *cobra.Command {
	var nickname string
	return kit.ReadCmd(k, "detail-by-name", "按昵称查用户 ID", "", userendpoint.DetailByName,
		func(int64) *mmpb.DetailByNameRequest { return &mmpb.DetailByNameRequest{Nickname: nickname} },
		func(c *cobra.Command) {
			c.Flags().StringVar(&nickname, "nickname", "", "用户昵称")
			_ = c.MarkFlagRequired("nickname")
		})
}

func newSubCount(k *kit.Kit) *cobra.Command {
	return kit.LoginReadCmd(k, "sub-count", "当前用户计数(登录态)", "", userendpoint.SubCount,
		func(int64) *mmpb.SubCountRequest { return &mmpb.SubCountRequest{} })
}

func newPlaylists(k *kit.Kit) *cobra.Command {
	var uid int64
	c := &cobra.Command{
		Use:   "playlists",
		Short: "用户歌单",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return kit.RenderCompose(k, compose.UserPlaylists, &mmpb.UserPlaylistRequest{UserId: uid})
		},
	}
	c.Flags().Int64Var(&uid, "uid", 0, "用户 ID")
	_ = c.MarkFlagRequired("uid")
	return c
}

func newFollows(k *kit.Kit) *cobra.Command {
	var uid int64
	var limit, offset int
	return kit.ReadCmd(k, "follows", "用户关注列表", "", userendpoint.Follows,
		func(int64) *mmpb.FollowsRequest { //nolint:gosec // 命令行分页参数,取值范围远小于 int32
			return &mmpb.FollowsRequest{UserId: uid, Limit: int32(limit), Offset: int32(offset)}
		},
		uidFlag(&uid), func(c *cobra.Command) {
			c.Flags().IntVar(&limit, "limit", 20, "返回数量")
			c.Flags().IntVar(&offset, "offset", 0, "偏移量")
		})
}

func newFolloweds(k *kit.Kit) *cobra.Command {
	var uid int64
	var limit, offset int
	return kit.ReadCmd(k, "followeds", "用户粉丝列表", "", userendpoint.Followeds,
		func(int64) *mmpb.FollowedsRequest {
			return &mmpb.FollowedsRequest{UserId: uid, Limit: int32(limit), Offset: int32(offset)}
		},
		uidFlag(&uid), func(c *cobra.Command) {
			c.Flags().IntVar(&limit, "limit", 20, "返回数量")
			c.Flags().IntVar(&offset, "offset", 0, "偏移量")
		})
}

func newFollowEachOther(k *kit.Kit) *cobra.Command {
	var uid, target int64
	return kit.ReadCmd(k, "follow-each-other", "是否互相关注", "", userendpoint.FollowEachOther,
		func(int64) *mmpb.FollowEachOtherRequest {
			return &mmpb.FollowEachOtherRequest{UserId: uid, TargetUserId: target}
		},
		uidFlag(&uid), func(c *cobra.Command) {
			c.Flags().Int64Var(&target, "target", 0, "对方用户 ID")
			_ = c.MarkFlagRequired("target")
		})
}

func newRecord(k *kit.Kit) *cobra.Command {
	var uid int64
	var typ int
	return kit.ReadCmd(k, "record", "听歌排行", "", userendpoint.Record,
		func(int64) *mmpb.RecordRequest { return &mmpb.RecordRequest{UserId: uid, Type: int32(typ)} },
		uidFlag(&uid), func(c *cobra.Command) {
			c.Flags().IntVar(&typ, "type", 0, "0=本周 1=全部")
		})
}

func newEvents(k *kit.Kit) *cobra.Command {
	var uid, lastID int64
	var limit int
	return kit.ReadCmd(k, "events", "用户动态", "", userendpoint.Events,
		func(int64) *mmpb.EventsRequest {
			return &mmpb.EventsRequest{UserId: uid, Limit: int32(limit), LastEventId: lastID}
		},
		uidFlag(&uid), func(c *cobra.Command) {
			c.Flags().IntVar(&limit, "limit", 30, "返回数量")
			c.Flags().Int64Var(&lastID, "last-event-id", 0, "偏移量(上一页最后一条动态 ID)")
		})
}

func newLevel(k *kit.Kit) *cobra.Command {
	var uid int64
	return kit.ReadCmd(k, "level", "用户等级", "", userendpoint.Level,
		func(int64) *mmpb.LevelRequest { return &mmpb.LevelRequest{UserId: uid} }, uidFlag(&uid))
}

func newSimilar(k *kit.Kit) *cobra.Command {
	return kit.ReadCmd(k, "similar", "相似用户(按歌曲)", "歌曲 ID", userendpoint.SimilarUsers,
		func(id int64) *mmpb.SimilarUsersRequest { return &mmpb.SimilarUsersRequest{SongId: id} })
}
