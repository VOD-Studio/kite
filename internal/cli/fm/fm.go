// Package fm 提供私人 FM 命令。
package fm

import (
	"github.com/spf13/cobra"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/cli/kit"
	fmendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/fm"
)

// NewCommand 创建 fm 命令(私人 FM 只有一个接口,直接作为可执行命令)。
func NewCommand(k *kit.Kit) *cobra.Command {
	c := kit.ReadCmd(k, "fm", "私人 FM", "", fmendpoint.GetPersonalFM,
		func(int64) *mmpb.GetPersonalFMRequest { return &mmpb.GetPersonalFMRequest{} })
	kit.AnnotateRpcs(c, "FMService/GetPersonalFM")
	return c
}
