// Package auth 提供 kite 的登录态命令:扫码/手机号登录、登录状态、登出。
package auth

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	merrors "github.com/VOD-Studio/kite/errors"
	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/cli/auth/qrtui"
	"github.com/VOD-Studio/kite/internal/cli/kit"
	"github.com/VOD-Studio/kite/internal/localstate"
	authendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/auth"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/termqr"
)

// NewLoginCommand 扫码登录:取二维码 → 轮询 → cookie 持久化到本地会话文件。
func NewLoginCommand(k *kit.Kit) *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "扫码登录,cookie 持久化到本地配置目录",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runLogin(k)
		},
	}
}

// NewSendCaptchaCommand 发送手机验证码。
func NewSendCaptchaCommand(k *kit.Kit) *cobra.Command {
	var phone string
	c := &cobra.Command{
		Use:   "send-captcha",
		Short: "发送短信验证码",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			_, err := kit.Exec(k, k.CookieCtx(), authendpoint.SendCaptcha, &mmpb.SendCaptchaRequest{Phone: phone})
			if err != nil {
				return err
			}
			fmt.Println("验证码已发送")
			return nil
		},
	}
	c.Flags().StringVar(&phone, "phone", "", "手机号(带区号,如 8613800138000)")
	_ = c.MarkFlagRequired("phone")
	return c
}

// NewLoginCellphoneCommand 手机号验证码登录,成功后同样落盘会话。
func NewLoginCellphoneCommand(k *kit.Kit) *cobra.Command {
	var phone, captcha string
	c := &cobra.Command{
		Use:   "login-cellphone",
		Short: "手机号验证码登录",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			resp, setCookie, err := kit.ExecWithSetCookie(k, k.CookieCtx(), authendpoint.LoginCellphone,
				&mmpb.LoginByCellphoneRequest{Phone: phone, Captcha: captcha})
			if err != nil {
				return err
			}
			return persistLogin(k, resp.GetSession(), setCookie)
		},
	}
	c.Flags().StringVar(&phone, "phone", "", "手机号(带区号,如 8613800138000)")
	c.Flags().StringVar(&captcha, "captcha", "", "短信验证码(先运行 send-captcha 获取)")
	_ = c.MarkFlagRequired("phone")
	_ = c.MarkFlagRequired("captcha")
	return c
}

// NewLoginStatusCommand 查看当前登录态。
// 人类模式 cookie 分段脱敏;--json/管道输出完整值(脚本取凭证是合法用途)。
func NewLoginStatusCommand(k *kit.Kit) *cobra.Command {
	return &cobra.Command{
		Use:   "login-status",
		Short: "查看当前登录态",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			resp, err := kit.Exec(k, k.CookieCtx(), authendpoint.LoginStatus, &mmpb.LoginStatusRequest{})
			if err != nil {
				return err
			}
			// 响应里没有账号信息 = 未登录(与旧实现的解码失败分支一致,报错而非渲染空会话)。
			sess := resp.GetSession()
			if sess == nil {
				return fmt.Errorf("未登录: %w", merrors.ErrUnauthorized)
			}
			sess.Cookie = k.CurrentCookie()
			if k.HumanOutput() {
				sess.Cookie = kit.MaskCookie(sess.Cookie)
				fmt.Fprintln(os.Stderr, "(cookie 已脱敏,完整值用 --json 查看)")
			}
			return k.Render(sess)
		},
	}
}

// NewLogoutCommand 登出并删除本地会话文件。
// 远端登出失败(如 cookie 已过期)仅告警不阻断本地清理。
func NewLogoutCommand(k *kit.Kit) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "登出并删除本地会话文件",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if cookie := k.CurrentCookie(); cookie != "" {
				if _, err := kit.Exec(k, k.CookieCtx(), authendpoint.Logout, &mmpb.LogoutRequest{}); err != nil {
					fmt.Fprintf(os.Stderr, "远端登出失败(继续清除本地会话): %v\n", err)
				}
			}
			if err := k.ClearSession(); err != nil {
				fmt.Fprintf(os.Stderr, "删除本地会话失败: %v\n", err)
			}
			fmt.Println("已登出,本地会话已清除")
			return nil
		},
	}
}

// NewLoginRefreshCommand 刷新登录态,把续期后的 cookie 写回本地会话文件。
// 只刷新会话文件里的登录态;NETEASE_COOKIE 环境变量的 cookie 由调用方自己保存,不在此处理。
func NewLoginRefreshCommand(k *kit.Kit) *cobra.Command {
	return &cobra.Command{
		Use:   "login-refresh",
		Short: "刷新登录态,延长本地会话的有效期",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			sess, err := k.LoadSession()
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			if sess.Cookie == "" {
				return fmt.Errorf("没有本地会话:先运行 kite login")
			}
			cookie, renewed, err := authendpoint.Refresh(engine.WithCookie(context.Background(), sess.Cookie), k.Engine())
			if err != nil {
				return err
			}
			sess.Cookie = cookie
			sess.SavedAt = time.Now().Format(time.RFC3339)
			if err := k.SaveSession(sess); err != nil {
				return fmt.Errorf("刷新成功但保存会话失败: %w", err)
			}
			if !renewed {
				fmt.Println("登录态有效,上游未下发新 cookie")
			} else {
				fmt.Println("登录态已刷新")
			}
			return nil
		},
	}
}

// persistLogin 登录成功后落盘会话并打印结果。sess 为登录接口解出的用户信息
// (可能为 nil,账号信息是可选项)。扫码登录和手机号登录共用。
func persistLogin(k *kit.Kit, sess *mmpb.Session, setCookie string) error {
	if setCookie == "" {
		return fmt.Errorf("登录成功但未拿到 cookie")
	}
	local := localstate.Session{
		Cookie:  setCookie,
		SavedAt: time.Now().Format(time.RFC3339),
	}
	if sess != nil {
		local.UserID = sess.GetUserId()
	}
	if err := k.SaveSession(local); err != nil {
		return fmt.Errorf("登录成功但保存会话失败: %w", err)
	}
	p, _ := localstate.SessionPath()
	fmt.Println("已登录")
	if local.UserID != 0 {
		fmt.Printf("用户 ID: %d\n", local.UserID)
	}
	fmt.Printf("会话已保存到 %s\n", p)
	fmt.Println("如需临时换号,设置 NETEASE_COOKIE 环境变量(优先级高于会话文件)")
	return nil
}

// runLogin 扫码登录:取二维码 → bubbletea 轮询界面 → cookie 持久化。
//
// 旧实现 fmt.Println 每次轮询堆一行,长时间等待或屏幕小时把二维码顶出视野。
// 新实现用 bubbletea:二维码块固定,spinner + 状态行原地刷新(qrtui 包)。
// CONFIRMED 的会话持久化在 bubbletea Program 退出后执行,避免 stdout 打印
// clobber 视图。详见 qrtui 包文档与 PRD-0016 后续。
func runLogin(k *kit.Kit) error {
	return runLoginWith(k, loginDeps{stdinIsTTY: func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }})
}

// loginDeps 注入依赖(测试 seam,沿 play.go playDeps 惯例)。
type loginDeps struct {
	stdinIsTTY func() bool
}

// runLoginWith 可测试入口:TTY 守卫 + 取 key + bubbletea 接管 + 持久化。
func runLoginWith(k *kit.Kit, deps loginDeps) error {
	// 0. TTY 守卫:扫码登录需要交互式终端(bubbletea 接管 stdin)。沿 play.go 模式。
	if !deps.stdinIsTTY() {
		return fmt.Errorf("%w:登录命令需要交互式终端,请直接运行而非管道", kit.ErrUsage)
	}

	ctx := k.CookieCtx()

	// 1. 取二维码 key(同步,失败直接返回)。
	resp, err := kit.Exec(k, ctx, authendpoint.LoginQrcode, &mmpb.LoginQrcodeRequest{})
	if err != nil {
		return err
	}
	key := resp.GetQrcode().GetKey()
	qrURL := authendpoint.QrcodeURL(key)

	// 2. bubbletea 接管终端:QR 固定 + spinner 状态行原地刷新。
	result, err := qrtui.Run(qrtui.Deps{
		QR:      termqr.Render(qrURL),
		QRURL:   qrURL,
		PollCtx: ctx,
		Check: func(ctx context.Context) (mmpb.QrcodeCode, *mmpb.Session, string, error) {
			resp, setCookie, err := kit.ExecWithSetCookie(k, ctx, authendpoint.CheckQrcode, &mmpb.CheckQrcodeRequest{Key: key})
			if err != nil {
				return 0, nil, "", err
			}
			return resp.GetCode(), resp.GetSession(), setCookie, nil
		},
	}, os.Stdout)
	if err != nil {
		return err
	}

	// 3. CONFIRMED 持久化(Program 退出后,终端已恢复,persistLogin 的 stdout 打印正常)。
	if result.Confirmed {
		return persistLogin(k, result.Session, result.SetCookie)
	}
	// 过期/超时:qrtui 已展示终态文案,这里不重复报错。
	return nil
}
