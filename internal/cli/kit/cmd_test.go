package kit

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// testEP 是构造器单测用的最小声明式 endpoint(不触发网络:RunE 在登录/确认处先返回)。
var testEP = &engine.Endpoint[*mmpb.LogoutRequest, *mmpb.LogoutResponse]{}

// TestCmdSkeleton_FlagsAndArgs 钉死构造器的命令骨架:
// 有 ID 通道 → --id flag + 最多 1 个位置参数;无 → NoArgs;额外 flag 原样注册。
func TestCmdSkeleton_FlagsAndArgs(t *testing.T) {
	var extra string
	c := ReadCmd(newTestKit(), "detail", "详情", "歌曲 ID", testEP,
		func(int64) *mmpb.LogoutRequest { return &mmpb.LogoutRequest{} },
		func(c *cobra.Command) { c.Flags().StringVar(&extra, "extra", "", "额外") })

	require.Equal(t, "detail", c.Use)
	require.Equal(t, "详情", c.Short)
	require.NotNil(t, c.Flags().Lookup("id"), "--id flag 应注册")
	require.NotNil(t, c.Flags().Lookup("extra"), "额外 flag 应注册")
	require.Error(t, c.Args(c, []string{"1", "2"}), "最多 1 个位置参数")
	require.NoError(t, c.Args(c, []string{"1"}))

	noID := ReadCmd(newTestKit(), "hot", "热门", "", testEP,
		func(int64) *mmpb.LogoutRequest { return &mmpb.LogoutRequest{} })
	require.Nil(t, noID.Flags().Lookup("id"))
	require.Error(t, noID.Args(noID, []string{"1"}), "无 ID 通道不接受位置参数")
}

// TestWriteCmd_OrderRequireLoginBeforeConfirm 写命令的顺序:未登录时先报
// ErrNotLogin(退出码 3),不进确认、不触网。
func TestWriteCmd_OrderRequireLoginBeforeConfirm(t *testing.T) {
	k := newTestKit() // 无 cookie
	c := WriteCmd(k, "trash", "丢垃圾桶", "歌曲 ID", "", testEP,
		func(*mmpb.LogoutRequest) string { return "确认文案" },
		func(int64) *mmpb.LogoutRequest { return &mmpb.LogoutRequest{} })

	err := c.RunE(c, []string{"1"})
	require.ErrorIs(t, err, ErrNotLogin, "未登录应先于确认报 ErrNotLogin")
}

// TestWriteCmd_NonInteractiveNeedsYes 登录态下非交互且无 --yes:确认处报用法错误。
func TestWriteCmd_NonInteractiveNeedsYes(t *testing.T) {
	t.Setenv("NETEASE_COOKIE", "MUSIC_U=t")
	k := newTestKit()
	c := WriteCmd(k, "trash", "丢垃圾桶", "歌曲 ID", "", testEP,
		func(*mmpb.LogoutRequest) string { return "确认文案" },
		func(int64) *mmpb.LogoutRequest { return &mmpb.LogoutRequest{} })

	err := c.RunE(c, []string{"1"})
	require.ErrorIs(t, err, ErrUsage)
}

// newTestKit 构造不触网的 Kit(stdout 走缓冲,engine 不执行)。
func newTestKit() *Kit {
	k := New()
	k.Out = &discardWriter{}
	return k
}

type discardWriter struct{}

func (*discardWriter) Write(p []byte) (int, error) { return len(p), nil }
