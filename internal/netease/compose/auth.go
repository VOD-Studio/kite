package compose

import (
	"context"
	"fmt"

	merrors "github.com/VOD-Studio/kite/errors"
	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	authendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/auth"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// CheckQrcode 轮询二维码状态;确认后返回新登录的凭据与账号,资料缺失时用新凭据补查。
//
// 状态与 803 体的账号提取在 authendpoint.CheckQrcode 的 MapResponse(宽容解码),
// 这里只做「确认后补全凭据」的组合:Set-Cookie 必须存在;账号资料缺失时用本次
// 登录的新凭据补查,不能沿用调用方的旧账号。
func CheckQrcode(ctx context.Context, eng *engine.Engine, req *mmpb.CheckQrcodeRequest) (*mmpb.CheckQrcodeResponse, error) {
	resp, cookie, err := engine.ExecuteWithSetCookie(eng, ctx, authendpoint.CheckQrcode, req)
	if err != nil {
		return nil, err
	}
	if resp.GetCode() != mmpb.QrcodeCode_QRCODE_CODE_CONFIRMED {
		return resp, nil
	}
	if cookie == "" {
		return nil, fmt.Errorf("%w: 扫码登录已确认但未返回凭据", merrors.ErrInvalidResponse)
	}

	sess := resp.GetSession()
	if sess == nil {
		// 803 响应可能只有确认状态;账号查询必须使用本次登录的 Cookie,不能沿用调用方的旧账号。
		status, err := engine.ExecuteWithCookie(eng, engine.WithCookie(ctx, cookie), authendpoint.LoginStatus, &mmpb.LoginStatusRequest{})
		if err != nil {
			return nil, err
		}
		sess = status.GetSession()
	}
	if sess.GetUserId() <= 0 {
		return nil, fmt.Errorf("%w: 扫码登录后的账号 ID 无效", merrors.ErrInvalidResponse)
	}
	sess.Cookie = cookie
	resp.Session = sess
	return resp, nil
}
