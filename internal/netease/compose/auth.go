package compose

import (
	"context"
	"fmt"

	merrors "github.com/VOD-Studio/kite/errors"
	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	authendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/auth"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/model"
)

// CheckQrcode 轮询二维码状态;确认后返回新登录的凭据与账号,资料缺失时用新凭据补查。
func CheckQrcode(ctx context.Context, eng *engine.Engine, req *mmpb.CheckQrcodeRequest) (*mmpb.CheckQrcodeResponse, error) {
	raw, cookie, err := eng.RawDoWithCookieAndInput(ctx, authendpoint.CheckQrcode, authendpoint.CheckQrcodeRequest(req))
	if err != nil {
		return nil, err
	}
	code, message, err := model.DecodeQrcodeStatus(raw)
	if err != nil {
		return nil, err
	}
	resp := &mmpb.CheckQrcodeResponse{Code: code, Message: message}
	if code != mmpb.QrcodeCode_QRCODE_CODE_CONFIRMED {
		return resp, nil
	}
	if cookie == "" {
		return nil, fmt.Errorf("%w: 扫码登录已确认但未返回凭据", merrors.ErrInvalidResponse)
	}

	sess, err := model.DecodeLoginResponse(raw)
	if err != nil {
		// 803 响应可能只有确认状态;账号查询必须使用本次登录的 Cookie,不能沿用调用方的旧账号。
		raw, _, err = eng.RawDoWithCookieAndInput(engine.WithCookie(ctx, cookie), authendpoint.LoginStatus, authendpoint.LoginStatusRequest(nil))
		if err != nil {
			return nil, err
		}
		sess, err = model.DecodeLoginResponse(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: 扫码登录后的账号资料无效", merrors.ErrInvalidResponse)
		}
	}
	if sess.UserId <= 0 {
		return nil, fmt.Errorf("%w: 扫码登录后的账号 ID 无效", merrors.ErrInvalidResponse)
	}
	sess.Cookie = cookie
	resp.Session = sess
	return resp, nil
}
