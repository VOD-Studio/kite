// Package auth 定义登录认证接口的声明。
//
// 与其它域一致,每个接口都是声明式 engine.Endpoint;区别只在执行入口:
// 登录类接口需要响应的 Set-Cookie,走 engine.ExecuteWithSetCookie(同样不经
// session 池、不缓存),service 与 CLI 各一行调用。session 池的写入是 service
// 的装配职责,留在调用方。例外:Refresh 把「刷新 + 合并 cookie」收在这里,
// gRPC service 与 CLI 共用,语义只定义一处。
package auth

import (
	"context"
	"encoding/json"
	"fmt"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/model"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// weapiMeta 构造 weapi POST + Anonymous 的 Meta(auth 接口统一参数:
// 凭据只经 ctx cookie 携带,不经 session 池选取)。
func weapiMeta(path string) engine.Meta {
	return engine.Meta{Path: path, Method: "POST", Crypto: engine.CryptoWeAPI, Auth: session.AuthAnonymous}
}

// SendCaptcha 发送验证码。
var SendCaptcha = &engine.Endpoint[*mmpb.SendCaptchaRequest, *mmpb.SendCaptchaResponse]{
	Meta:    weapiMeta("/weapi/sms/captcha/sent"),
	NewResp: func() *mmpb.SendCaptchaResponse { return &mmpb.SendCaptchaResponse{} },
	MapRequest: func(req *mmpb.SendCaptchaRequest) (map[string]any, error) {
		return map[string]any{"cellphone": req.GetPhone(), "ctcode": "86"}, nil
	},
	MapResponse: func(_ *mmpb.SendCaptchaRequest, _ json.RawMessage) (*mmpb.SendCaptchaResponse, error) {
		return &mmpb.SendCaptchaResponse{}, nil
	},
}

// LoginCellphone 手机号登录。响应解码出用户信息(Session.Cookie 由调用方
// 从 Set-Cookie 回填),存 session 池是 service 的职责。
var LoginCellphone = &engine.Endpoint[*mmpb.LoginByCellphoneRequest, *mmpb.LoginByCellphoneResponse]{
	Meta:    weapiMeta("/weapi/login/cellphone"),
	NewResp: func() *mmpb.LoginByCellphoneResponse { return &mmpb.LoginByCellphoneResponse{} },
	MapRequest: func(req *mmpb.LoginByCellphoneRequest) (map[string]any, error) {
		return map[string]any{
			"phone":         req.GetPhone(),
			"captcha":       req.GetCaptcha(),
			"countrycode":   "86",
			"rememberLogin": "true",
		}, nil
	},
	MapResponse: func(_ *mmpb.LoginByCellphoneRequest, raw json.RawMessage) (*mmpb.LoginByCellphoneResponse, error) {
		sess, err := model.DecodeLoginResponse(raw)
		if err != nil {
			return nil, err
		}
		return &mmpb.LoginByCellphoneResponse{Session: sess}, nil
	},
}

// LoginQrcode 获取二维码 key（weapi,参考 chaunsin/netease-cloud-music CLI 验证过）。
var LoginQrcode = &engine.Endpoint[*mmpb.LoginQrcodeRequest, *mmpb.LoginQrcodeResponse]{
	Meta:    weapiMeta("/weapi/login/qrcode/unikey"),
	NewResp: func() *mmpb.LoginQrcodeResponse { return &mmpb.LoginQrcodeResponse{} },
	MapRequest: func(_ *mmpb.LoginQrcodeRequest) (map[string]any, error) {
		// type=1 是 weapi 二维码登录的值(参考 chaunsin Go 版 CLI 验证)。
		return map[string]any{"type": 1}, nil
	},
	MapResponse: func(_ *mmpb.LoginQrcodeRequest, raw json.RawMessage) (*mmpb.LoginQrcodeResponse, error) {
		key, err := model.DecodeQrcodeKey(raw)
		if err != nil {
			return nil, err
		}
		return &mmpb.LoginQrcodeResponse{Qrcode: &mmpb.Qrcode{Key: key, Url: QrcodeURL(key)}}, nil
	},
}

// CheckQrcode 轮询二维码状态（weapi）。code=803(已确认)时响应体含账号信息,
// 一并解出 Session;Set-Cookie 由执行入口回传,cookie 由调用方回填。
var CheckQrcode = &engine.Endpoint[*mmpb.CheckQrcodeRequest, *mmpb.CheckQrcodeResponse]{
	Meta:    weapiMeta("/weapi/login/qrcode/client/login"),
	NewResp: func() *mmpb.CheckQrcodeResponse { return &mmpb.CheckQrcodeResponse{} },
	MapRequest: func(req *mmpb.CheckQrcodeRequest) (map[string]any, error) {
		return map[string]any{"key": req.GetKey(), "type": 1}, nil
	},
	MapResponse: func(_ *mmpb.CheckQrcodeRequest, raw json.RawMessage) (*mmpb.CheckQrcodeResponse, error) {
		code, message, err := model.DecodeQrcodeStatus(raw)
		if err != nil {
			return nil, err
		}
		resp := &mmpb.CheckQrcodeResponse{Code: code, Message: message}
		// 803 的响应体也含 account 信息;与 DecodeLoginResponse 的失败分支一致,
		// 提不出来就不带 Session(由调用方按有无 Set-Cookie 决定后续)。
		if code == mmpb.QrcodeCode_QRCODE_CODE_CONFIRMED {
			if sess, err := model.DecodeLoginResponse(raw); err == nil && sess.UserId != 0 {
				resp.Session = sess
			}
		}
		return resp, nil
	},
}

// LoginStatus 查询登录态。未登录(响应里没有账号信息)不算错误,返回空 Session,
// 由调用方决定如何呈现;cookie 从 ctx 回填。
var LoginStatus = &engine.Endpoint[*mmpb.LoginStatusRequest, *mmpb.LoginStatusResponse]{
	Meta:    weapiMeta("/weapi/w/nuser/account/get"),
	NewResp: func() *mmpb.LoginStatusResponse { return &mmpb.LoginStatusResponse{} },
	MapRequest: func(_ *mmpb.LoginStatusRequest) (map[string]any, error) {
		return map[string]any{}, nil
	},
	MapResponse: func(_ *mmpb.LoginStatusRequest, raw json.RawMessage) (*mmpb.LoginStatusResponse, error) {
		sess, err := model.DecodeLoginResponse(raw)
		if err != nil {
			return &mmpb.LoginStatusResponse{}, nil //nolint:nilerr // 未登录不算错误,session 为空即表示未登录
		}
		return &mmpb.LoginStatusResponse{Session: sess}, nil
	},
}

// Logout 登出。
var Logout = &engine.Endpoint[*mmpb.LogoutRequest, *mmpb.LogoutResponse]{
	Meta:    weapiMeta("/weapi/logout"),
	NewResp: func() *mmpb.LogoutResponse { return &mmpb.LogoutResponse{} },
	MapRequest: func(_ *mmpb.LogoutRequest) (map[string]any, error) {
		return map[string]any{}, nil
	},
	MapResponse: func(_ *mmpb.LogoutRequest, _ json.RawMessage) (*mmpb.LogoutResponse, error) {
		return &mmpb.LogoutResponse{}, nil
	},
}

// RefreshLogin 刷新登录态:新的 MUSIC_U 等经 Set-Cookie 下发(只有 weapi 回传 Set-Cookie);
// cookie 失效时上游返回 code=301(映射为未授权)。响应体无用户信息。
var RefreshLogin = &engine.Endpoint[*mmpb.RefreshLoginRequest, *mmpb.RefreshLoginResponse]{
	Meta:    weapiMeta("/weapi/login/token/refresh"),
	NewResp: func() *mmpb.RefreshLoginResponse { return &mmpb.RefreshLoginResponse{} },
	MapRequest: func(_ *mmpb.RefreshLoginRequest) (map[string]any, error) {
		return map[string]any{}, nil
	},
	MapResponse: func(_ *mmpb.RefreshLoginRequest, _ json.RawMessage) (*mmpb.RefreshLoginResponse, error) {
		return &mmpb.RefreshLoginResponse{}, nil
	},
}

// Refresh 刷新 ctx 里 cookie(engine.WithCookie 注入)对应的登录态,返回合并了上游新下发值的完整 cookie;
// renewed 表示上游是否下发了新值。cookie 失效时返回未授权(上游 code=301)。
func Refresh(ctx context.Context, eng *engine.Engine) (cookie string, renewed bool, err error) {
	_, setCookie, err := engine.ExecuteWithSetCookie(eng, ctx, RefreshLogin, nil)
	if err != nil {
		return "", false, err
	}
	return engine.MergeCookies(engine.CookieFromContext(ctx), setCookie), setCookie != "", nil
}

// QrcodeURL 从 key 构造二维码扫描 URL。
func QrcodeURL(key string) string {
	return fmt.Sprintf("https://music.163.com/login?codekey=%s", key)
}
