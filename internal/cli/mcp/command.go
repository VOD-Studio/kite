package mcp

import (
	"github.com/spf13/cobra"

	"github.com/VOD-Studio/kite/internal/cli/kit"
	"github.com/VOD-Studio/kite/internal/cli/version"
)

// NewCommand 构造 kite mcp:以 stdio MCP 服务运行,供 AI 客户端调用只读的网易云接口。
func NewCommand(k *kit.Kit) *cobra.Command {
	c := &cobra.Command{
		Use:   "mcp",
		Short: "以 MCP 服务(stdio)运行,供 Claude / Cursor 等 AI 客户端调用只读接口",
		Long: `以 MCP 服务(stdio 传输)运行,把搜索、歌曲 / 歌词 / 播放链接、歌单、专辑、歌手、热搜、
每日推荐等只读接口暴露给 AI 客户端。不提供任何写操作。

由 AI 客户端启动,不要手动运行。客户端配置示例(Claude Code):

  claude mcp add kite -- kite mcp

登录态用本机的 kite login(环境变量 NETEASE_COOKIE 优先);每次调用时读取,重新登录后无需重启客户端。`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return Run(cmd.Context(), Deps{Engine: k.Engine(), Cookie: k.CurrentCookie}, version.LoadVersion().Version)
		},
	}
	kit.AnnotateRpcs(c, RPCs()...)
	return c
}
