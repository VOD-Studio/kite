// Package dj 提供电台(主播电台)命令。
//
// issue #33 第一阶段:直连 engine + endpoint,响应是 model 纯 Go 类型、尚未 proto
// 建模,走不了 kit.RenderExec(它要求 proto.Message),按 recent 命令先例自渲染
// 双态(WantJSON → JSON,否则表格)。
package dj

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"

	"github.com/VOD-Studio/kite/internal/cli/kit"
	"github.com/VOD-Studio/kite/internal/netease/endpoint/djradio"
	"github.com/VOD-Studio/kite/internal/netease/model"
)

// NewCommand 创建 dj 命令组。
func NewCommand(k *kit.Kit) *cobra.Command {
	c := &cobra.Command{
		Use:   "dj",
		Short: "主播电台相关命令",
		Args:  cobra.NoArgs,
	}
	c.AddCommand(newCateList(k), newPrograms(k), newProgramDetail(k), newSublist(k))
	// 第一阶段直连 endpoint,尚未 proto 建模(issue #33 第二阶段),统一标「已审视,无 rpc」。
	for _, sub := range c.Commands() {
		kit.AnnotateRpcs(sub)
	}
	return c
}

// render 双态输出:WantJSON → JSON(与 protojson 的多行缩进一致);否则人类可读。
func render[T any](k *kit.Kit, v T, human func(io.Writer, T)) error {
	if k.WantJSON() {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Errorf("序列化响应失败: %w", err)
		}
		fmt.Fprintln(k.OutWriter(), string(b))
		return nil
	}
	human(k.OutWriter(), v)
	return nil
}

// fmtDurationMs 毫秒 → m:ss(≥1h 用 h:mm:ss)。
func fmtDurationMs(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	total := int(ms / 1000)
	h, rem := total/3600, total%3600
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, rem/60, rem%60)
	}
	return fmt.Sprintf("%d:%02d", rem/60, rem%60)
}

// fmtDateMs 毫秒时间戳 → 日期(上游是毫秒时间戳,与 artist 专辑发行时间同处理)。
func fmtDateMs(ms int64) string {
	if ms <= 0 {
		return "-"
	}
	return time.UnixMilli(ms).Format("2006-01-02")
}

// fmtCount 大数缩写:万级以上用「x.x万」对齐中文习惯,否则原样。
func fmtCount(n int64) string {
	if n >= 100_000_000 {
		return fmt.Sprintf("%.1f亿", float64(n)/100_000_000)
	}
	if n >= 10_000 {
		return fmt.Sprintf("%.1f万", float64(n)/10_000)
	}
	return fmt.Sprintf("%d", n)
}

// anchorName 取主播昵称,空主播回退 "-"。
func anchorName(a *model.DjAnchor) string {
	if a == nil {
		return "-"
	}
	return a.Nickname
}

// writeRow 按列写出表格一行(列宽由调用方保证,这里只做拼接)。
func writeRow(w io.Writer, cols ...string) {
	fmt.Fprintln(w, strings.Join(cols, "  "))
}

func newCateList(k *kit.Kit) *cobra.Command {
	return &cobra.Command{
		Use:   "catelist",
		Short: "电台分类列表",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			resp, err := kit.Exec(k, k.CookieCtx(), djradio.CateList, &djradio.CateListRequest{})
			if err != nil {
				return err
			}
			return render(k, resp, func(w io.Writer, v *model.DjCategoryList) {
				if len(v.Categories) == 0 {
					fmt.Fprintln(w, "无分类")
					return
				}
				for _, c := range v.Categories {
					writeRow(w, fmt.Sprintf("%-6d", c.ID), c.Name)
				}
			})
		},
	}
}

func newPrograms(k *kit.Kit) *cobra.Command {
	var id int64
	var limit int
	var offset int
	var asc bool
	c := &cobra.Command{
		Use:   "programs",
		Short: "电台节目列表",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			rid, err := kit.ResolveID(id, args)
			if err != nil {
				return err
			}
			req := &djradio.ProgramsRequest{
				RadioID: rid, Limit: int32(limit), Offset: int32(offset), Asc: asc,
			}
			resp, err := kit.Exec(k, k.CookieCtx(), djradio.Programs, req)
			if err != nil {
				return err
			}
			return render(k, resp, func(w io.Writer, v *model.DjProgramPage) {
				if len(v.Programs) == 0 {
					fmt.Fprintln(w, "该电台无节目")
					return
				}
				fmt.Fprintf(w, "共 %d 期(本页 %d,%s)\n\n", v.Count, len(v.Programs), pageState(v.More))
				writeRow(w, "期数", padRight("ID", 12), padRight("节目", 30), padRight("时长", 7), padRight("播放", 9), "主播")
				for _, p := range v.Programs {
					writeRow(w,
						fmt.Sprintf("%-4d", p.SerialNum),
						padRight(fmt.Sprintf("%d", p.ID), 12),
						padRight(truncate(p.Name, 28), 30),
						padRight(fmtDurationMs(p.DurationMs), 7),
						padRight(fmtCount(p.ListenerCount), 9),
						anchorName(p.Anchor))
				}
			})
		},
	}
	c.Flags().Int64Var(&id, "id", 0, "电台 ID")
	c.Flags().IntVar(&limit, "limit", 10, "每页数量")
	c.Flags().IntVar(&offset, "offset", 0, "偏移量")
	c.Flags().BoolVar(&asc, "asc", false, "按上架时间升序(默认降序,最新在前)")
	return c
}

func newProgramDetail(k *kit.Kit) *cobra.Command {
	var id int64
	c := &cobra.Command{
		Use:   "program-detail",
		Short: "节目详情",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			pid, err := kit.ResolveID(id, args)
			if err != nil {
				return err
			}
			resp, err := kit.Exec(k, k.CookieCtx(), djradio.ProgramDetail, &djradio.ProgramDetailRequest{ProgramID: pid})
			if err != nil {
				return err
			}
			return render(k, resp, func(w io.Writer, p *model.DjProgram) {
				fmt.Fprintf(w, "id: %d\n", p.ID)
				fmt.Fprintf(w, "name: %s\n", p.Name)
				fmt.Fprintf(w, "serialNum: %d\n", p.SerialNum)
				fmt.Fprintf(w, "duration: %s\n", fmtDurationMs(p.DurationMs))
				fmt.Fprintf(w, "createTime: %s\n", fmtDateMs(p.CreateTimeMs))
				fmt.Fprintf(w, "listenerCount: %s\n", fmtCount(p.ListenerCount))
				fmt.Fprintf(w, "likedCount: %s\n", fmtCount(p.LikedCount))
				fmt.Fprintf(w, "commentCount: %s\n", fmtCount(p.CommentCount))
				fmt.Fprintf(w, "mainTrackId: %d\n", p.MainTrackID)
				fmt.Fprintf(w, "dj: %s\n", anchorName(p.Anchor))
				if p.Radio != nil {
					fmt.Fprintf(w, "radio: %d %s(%s/%s)\n", p.Radio.ID, p.Radio.Name, p.Radio.Category, p.Radio.SecondCategory)
				}
				if p.Description != "" {
					fmt.Fprintf(w, "description:\n  %s\n", strings.ReplaceAll(p.Description, "\n", "\n  "))
				}
			})
		},
	}
	c.Flags().Int64Var(&id, "id", 0, "节目 ID")
	return c
}

func newSublist(k *kit.Kit) *cobra.Command {
	var limit int
	var offset int
	c := &cobra.Command{
		Use:   "sublist",
		Short: "订阅的电台列表(登录态)",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := k.RequireLogin(); err != nil {
				return err
			}
			req := &djradio.SubscribedRequest{Limit: int32(limit), Offset: int32(offset)}
			resp, err := kit.Exec(k, k.CookieCtx(), djradio.Subscribed, req)
			if err != nil {
				return err
			}
			return render(k, resp, func(w io.Writer, v *model.DjSubscribedPage) {
				if len(v.DjRadios) == 0 {
					fmt.Fprintln(w, "未订阅任何电台")
					return
				}
				fmt.Fprintf(w, "共 %d 个(本页 %d,%s)\n\n", v.Count, len(v.DjRadios), pageState(v.More))
				writeRow(w, padRight("ID", 12), padRight("电台", 26), padRight("分类", 12), padRight("订阅", 9), padRight("节目", 7), "主播")
				for _, r := range v.DjRadios {
					cat := r.Category
					if r.SecondCategory != "" {
						cat = cat + "/" + r.SecondCategory
					}
					writeRow(w,
						padRight(fmt.Sprintf("%d", r.ID), 12),
						padRight(truncate(r.Name, 24), 26),
						padRight(truncate(cat, 10), 12),
						padRight(fmtCount(r.SubCount), 9),
						padRight(fmtCount(r.ProgramCount), 7),
						anchorName(r.Anchor))
				}
			})
		},
	}
	c.Flags().IntVar(&limit, "limit", 30, "每页数量")
	c.Flags().IntVar(&offset, "offset", 0, "偏移量")
	return c
}

// pageState 翻页状态提示。
func pageState(more bool) string {
	if more {
		return "还有更多"
	}
	return "已到末页"
}

// padRight 按显示宽度右补空格(runewidth:CJK 宽 2,中文不错位,同 kit.RenderHuman)。
func padRight(s string, width int) string {
	return runewidth.FillRight(s, width)
}

// truncate 超宽截断(runewidth 按显示宽度,表格列宽控制,同 kit.RenderHuman)。
func truncate(s string, width int) string {
	return runewidth.Truncate(s, width, "…")
}
