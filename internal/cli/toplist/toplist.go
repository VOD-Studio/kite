// Package toplist 提供官方榜单命令(issue #29 第一阶段:直连 endpoint,不经 gRPC/service)。
package toplist

import (
	"github.com/spf13/cobra"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/cli/kit"
	toplistendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/toplist"
)

// NewCommand 创建 toplist 命令组。
func NewCommand(k *kit.Kit) *cobra.Command {
	c := &cobra.Command{
		Use:   "toplist",
		Short: "官方榜单",
		Args:  cobra.NoArgs,
	}
	c.AddCommand(newList(k), newDetail(k))
	annotateToplistRpcs(c)
	return c
}

// annotateToplistRpcs 给 toplist 组各子命令打 rpc 注解(PRD-0014 #B 命令树守护)。
// 第一阶段(#29)只有 endpoint 声明,ToplistService rpc 在第二阶段 proto 建模;
// 此处暂标空,rpc 落地后补 "ToplistService/Toplists" / "ToplistService/ToplistDetail"。
func annotateToplistRpcs(c *cobra.Command) {
	for _, sub := range c.Commands() {
		kit.AnnotateRpcs(sub)
	}
}

func newList(k *kit.Kit) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "全部榜单列表",
		Long: `列出全部官方榜单(64 个,含飙升/新歌/原创/热歌四个主榜与各媒体榜)。

榜单即歌单:取某榜完整曲目用 kite playlist detail --id <榜单ID>。`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return kit.RenderExec(k, toplistendpoint.List, &mmpb.ToplistsRequest{})
		},
	}
}

func newDetail(k *kit.Kit) *cobra.Command {
	return &cobra.Command{
		Use:   "detail",
		Short: "全部榜单详情(主榜含前三歌曲概要)",
		Long: `列出全部官方榜单详情,四个主榜(飙升/新歌/原创/热歌)各带前三歌曲概要
(上游只给歌名与歌手名串,不是完整歌曲)。

榜单即歌单:取某榜完整曲目用 kite playlist detail --id <榜单ID>。`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return kit.RenderExec(k, toplistendpoint.Detail, &mmpb.ToplistDetailRequest{})
		},
	}
}
