// Package kit 的命令构造器。
//
// 读 / 写命令的骨架(--id flag 与位置参数、flag 注册、登录检查、写确认、渲染)
// 在这里只写一份,各域命令包声明 Use/Short/endpoint 与请求构造即可。
// 特殊命令(跨接口组合、多 rpc 的下载、解析复杂到喧宾夺主的 update-tracks)
// 仍手写 cobra.Command,构造器不做万能抽象。
package kit

import (
	"fmt"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"

	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// CmdFlags 注册命令的额外 flag(写调用方闭包的局部变量,请求构造时读取)。
type CmdFlags func(*cobra.Command)

// ReadCmd 构造读命令:ID 解析 → 请求构造 → RenderExec(endpoint)。
//
// idHelp 非空时注册可选 --id flag 并接受最多 1 个位置参数补位(#38);
// 空串表示无 ID 通道(NoArgs)。
func ReadCmd[Req any, Resp proto.Message](k *Kit, use, short, idHelp string, ep *engine.Endpoint[Req, Resp], req func(id int64) Req, flags ...CmdFlags) *cobra.Command {
	return cmdSkeleton(use, short, idHelp, req, func(r Req) error {
		return RenderExec(k, ep, r)
	}, flags)
}

// LoginReadCmd 同 ReadCmd,但执行前要求登录态(读当前账号数据的命令,
// 无写确认)。
func LoginReadCmd[Req any, Resp proto.Message](k *Kit, use, short, idHelp string, ep *engine.Endpoint[Req, Resp], req func(id int64) Req, flags ...CmdFlags) *cobra.Command {
	return cmdSkeleton(use, short, idHelp, req, func(r Req) error {
		if err := k.RequireLogin(); err != nil {
			return err
		}
		return RenderExec(k, ep, r)
	}, flags)
}

// WriteCmd 构造写命令:ID 解析 → 请求构造 → RequireLogin → ConfirmFatal → 执行。
//
//   - confirm 由请求生成交互确认文案(如「收藏歌单 123」)。
//   - done 为空时按三态渲染响应;非空时执行成功后打印该文案
//     (delete/update 这类响应无数据可渲染的写操作)。
//
// 请求构造放最前:--tracks 这类格式错误在登录检查与确认之前报出,
// 与既有手写命令的顺序一致。
func WriteCmd[Req any, Resp proto.Message](k *Kit, use, short, idHelp, done string, ep *engine.Endpoint[Req, Resp], confirm func(req Req) string, req func(id int64) Req, flags ...CmdFlags) *cobra.Command {
	return cmdSkeleton(use, short, idHelp, req, func(r Req) error {
		if err := k.RequireLogin(); err != nil {
			return err
		}
		if err := k.ConfirmFatal(confirm(r)); err != nil {
			return err
		}
		if done != "" {
			if _, err := Exec(k, k.CookieCtx(), ep, r); err != nil {
				return err
			}
			fmt.Println(done)
			return nil
		}
		return RenderExec(k, ep, r)
	}, flags)
}

// cmdSkeleton 是三个构造器共用的装配:命令骨架、--id/位置参数、flag 注册、RunE 顺序。
func cmdSkeleton[Req any](use, short, idHelp string, reqFn func(id int64) Req, run func(req Req) error, flags []CmdFlags) *cobra.Command {
	var id int64
	c := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			rid := int64(0)
			if idHelp != "" {
				var err error
				rid, err = ResolveID(id, args)
				if err != nil {
					return err
				}
			}
			return run(reqFn(rid))
		},
	}
	if idHelp != "" {
		c.Flags().Int64Var(&id, "id", 0, idHelp)
		c.Args = cobra.MaximumNArgs(1)
	}
	for _, f := range flags {
		f(c)
	}
	return c
}
