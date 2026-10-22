// Package mv 提供 MV 命令(issue #32 第一阶段:直连 endpoint,不经 gRPC/service)。
//
// 上游现状(2026-10 实测):独立的 /mv/url 接口已下线(weapi / eapi 均业务 404),
// 多清晰度播放地址由 /mv/detail 的 brs 字段返回,`kite mv url` 即经 Detail 取;
// MV 排行的现行接口是 /mv/toplist(老 /top/mv 已业务 404)。
package mv

import (
	"fmt"

	"github.com/spf13/cobra"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/cli/kit"
	mvendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/mv"
)

// NewCommand 创建 mv 命令组。
func NewCommand(k *kit.Kit) *cobra.Command {
	c := &cobra.Command{
		Use:   "mv",
		Short: "MV 详情 / 播放地址 / 排行 / 相关 / 收藏",
		Args:  cobra.NoArgs,
	}
	c.AddCommand(newDetail(k), newURL(k), newTop(k), newSimilar(k), newSubscribed(k))
	annotateMvRpcs(c)
	return c
}

// annotateMvRpcs 给 mv 组各子命令打 rpc 注解(PRD-0014 #B 命令树守护)。
// 第一阶段(#32)只有 endpoint 声明,MvService rpc 在第二阶段 proto 建模;
// 此处暂标空,rpc 落地后补 "MvService/GetMv" 等。
func annotateMvRpcs(c *cobra.Command) {
	for _, sub := range c.Commands() {
		kit.AnnotateRpcs(sub)
	}
}

// simple 构造一个单 --id 参数的读命令(--id 或位置参数二选一,#38)。
func simple(use, short string, run func(id int64) error) *cobra.Command {
	var id int64
	c := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			rid, err := kit.ResolveID(id, args)
			if err != nil {
				return err
			}
			return run(rid)
		},
	}
	c.Flags().Int64Var(&id, "id", 0, "MV ID")
	return c
}

func newDetail(k *kit.Kit) *cobra.Command {
	return simple("detail", "MV 详情", func(id int64) error {
		return kit.RenderExec(k, mvendpoint.Detail, &mmpb.GetMvRequest{MvId: id})
	})
}

// newURL 取多清晰度播放地址:上游独立 /mv/url 已下线,地址由 /mv/detail
// 的 brs 字段返回,故底层仍是 Detail endpoint。
func newURL(k *kit.Kit) *cobra.Command {
	return simple("url", "MV 播放地址(多清晰度)", func(id int64) error {
		resp, err := kit.Exec(k, k.CookieCtx(), mvendpoint.Detail, &mmpb.GetMvRequest{MvId: id})
		if err != nil {
			return err
		}
		if k.HumanOutput() {
			// 逐行一档(240P: <url>),方便直接复制;完整结构用 --json。
			// 不走 RenderHuman:渲染层不展开二层嵌套的 repeated 字段。
			mv := resp.GetMv()
			for _, u := range mv.GetUrls() {
				fmt.Fprintf(k.OutWriter(), "%dP\t%s\n", u.GetResolution(), u.GetUrl())
			}
			return nil
		}
		return k.Render(resp)
	})
}

func newTop(k *kit.Kit) *cobra.Command {
	return &cobra.Command{
		Use:   "top",
		Short: "MV 排行榜",
		Long: `列出 MV 排行榜(全榜 50 条,含上期排名与排行分)。

上游现行接口为 /mv/toplist(老 /top/mv 已下线);条目时长上游恒不返回。`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return kit.RenderExec(k, mvendpoint.Top, &mmpb.TopMvsRequest{})
		},
	}
}

func newSimilar(k *kit.Kit) *cobra.Command {
	return simple("similar", "相关 MV", func(id int64) error {
		return kit.RenderExec(k, mvendpoint.Similar, &mmpb.SimilarMvsRequest{MvId: id})
	})
}

func newSubscribed(k *kit.Kit) *cobra.Command {
	var limit int
	var offset int
	c := &cobra.Command{
		Use:   "subscribed",
		Short: "收藏的 MV 列表(登录态)",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := k.RequireLogin(); err != nil {
				return err
			}
			return kit.RenderExec(k, mvendpoint.Subscribed, &mmpb.SubscribedMvsRequest{
				Limit: int32(limit), Offset: int32(offset),
			})
		},
	}
	c.Flags().IntVar(&limit, "limit", 25, "返回数量")
	c.Flags().IntVar(&offset, "offset", 0, "偏移量")
	return c
}
