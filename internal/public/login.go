package public

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	authendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/auth"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/model"
	"github.com/VOD-Studio/kite/internal/termqr"
)

// pollInterval 是轮询二维码状态的间隔,与网易云网页端一致。
const pollInterval = 2 * time.Second

// Login 走网易云扫码登录,把站长的登录态写到 path。二维码与进度输出到 out。
// 二维码过期会自动换新的;ctx 结束(例如 Ctrl-C)时返回 ctx 的错误。
func Login(ctx context.Context, eng *engine.Engine, path string, out io.Writer) error {
	if path == "" {
		return errors.New("没有登录态文件路径:设置 KITE_PUBLIC_SESSION_FILE 或传 --file")
	}
	for {
		key, err := qrcodeKey(ctx, eng)
		if err != nil {
			return err
		}
		url := authendpoint.QrcodeURL(key)
		_, _ = fmt.Fprintf(out, "用网易云音乐 App 扫码登录站长账号:\n\n%s\n%s\n\n", termqr.Render(url), url)

		sess, expired, err := waitQrcode(ctx, eng, key, out)
		if err != nil {
			return err
		}
		if expired {
			_, _ = fmt.Fprintln(out, "二维码已过期,换一张新的。")
			continue
		}
		f := SessionFile{Cookie: sess.GetCookie(), UserID: sess.GetUserId(), Nickname: sess.GetNickname(), UpdatedAt: time.Now()}
		if err := WriteSessionFile(path, f); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "已登录:%s(ID %d),登录态写入 %s。运行中的 kite-server 会在 30 秒内自动读取。\n", f.Nickname, f.UserID, path)
		return nil
	}
}

func qrcodeKey(ctx context.Context, eng *engine.Engine) (string, error) {
	raw, _, err := eng.RawDoWithCookieAndInput(ctx, authendpoint.LoginQrcode, authendpoint.LoginQrcodeRequest(nil))
	if err != nil {
		return "", fmt.Errorf("获取二维码: %w", err)
	}
	return model.DecodeQrcodeKey(raw)
}

// waitQrcode 轮询直到确认登录(返回会话)或二维码过期(expired=true)。
func waitQrcode(ctx context.Context, eng *engine.Engine, key string, out io.Writer) (*mmpb.Session, bool, error) {
	scanned := false
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case <-t.C:
		}
		raw, setCookie, err := eng.RawDoWithCookieAndInput(ctx, authendpoint.CheckQrcode,
			authendpoint.CheckQrcodeRequest(&mmpb.CheckQrcodeRequest{Key: key}))
		if err != nil {
			return nil, false, fmt.Errorf("查询二维码状态: %w", err)
		}
		code, _, err := model.DecodeQrcodeStatus(raw)
		if err != nil {
			return nil, false, err
		}
		switch code {
		case mmpb.QrcodeCode_QRCODE_CODE_EXPIRED:
			return nil, true, nil
		case mmpb.QrcodeCode_QRCODE_CODE_SCANNED:
			if !scanned {
				scanned = true
				_, _ = fmt.Fprintln(out, "已扫码,请在手机上确认登录。")
			}
		case mmpb.QrcodeCode_QRCODE_CODE_CONFIRMED:
			if setCookie == "" {
				return nil, false, errors.New("上游确认了登录,但没有下发 cookie")
			}
			sess, err := model.DecodeLoginResponse(raw)
			if err != nil {
				sess = &mmpb.Session{}
			}
			sess.Cookie = setCookie
			return sess, false, nil
		}
	}
}
