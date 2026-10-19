// Package auth 定义登录认证接口的声明。
//
// auth 接口不走 Execute（需要 Set-Cookie 创建 session），含 Meta + MapRequest。
// auth service 直接调 engine.RawDoWithCookie，拿到 raw + setCookie 后调 model 解码 + Save session。
// 例外:Refresh 把「刷新 + 合并 cookie」收在这里,gRPC service 与 CLI 共用,语义只定义一处。
package auth

import (
	"context"
	"encoding/json"
	"fmt"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// Meta 集合：各 auth 接口的执行元数据。

var (
	// SendCaptcha 发送验证码。
	SendCaptcha = engine.Meta{
		Path: "/weapi/sms/captcha/sent", Method: "POST", Crypto: engine.CryptoWeAPI, Auth: session.AuthAnonymous,
	}
	// LoginByCellphone 手机号登录。
	LoginCellphone = engine.Meta{
		Path: "/weapi/login/cellphone", Method: "POST", Crypto: engine.CryptoWeAPI, Auth: session.AuthAnonymous,
	}
	// LoginQrcode 获取二维码 key（weapi,参考 chaunsin/netease-cloud-music CLI 验证过）。
	LoginQrcode = engine.Meta{
		Path: "/weapi/login/qrcode/unikey", Method: "POST", Crypto: engine.CryptoWeAPI, Auth: session.AuthAnonymous,
	}
	// CheckQrcode 轮询二维码状态（weapi）。
	CheckQrcode = engine.Meta{
		Path: "/weapi/login/qrcode/client/login", Method: "POST", Crypto: engine.CryptoWeAPI, Auth: session.AuthAnonymous,
	}
	// LoginStatus 查询登录态。
	LoginStatus = engine.Meta{
		Path: "/weapi/w/nuser/account/get", Method: "POST", Crypto: engine.CryptoWeAPI, Auth: session.AuthAnonymous,
	}
	// Logout 登出。
	Logout = engine.Meta{
		Path: "/weapi/logout", Method: "POST", Crypto: engine.CryptoWeAPI, Auth: session.AuthAnonymous,
	}
	// RefreshLogin 刷新登录态:新的 MUSIC_U 等经 Set-Cookie 下发(只有 weapi 回传 Set-Cookie);
	// cookie 失效时上游返回 code=301(映射为未授权)。
	RefreshLogin = engine.Meta{
		Path: "/weapi/login/token/refresh", Method: "POST", Crypto: engine.CryptoWeAPI, Auth: session.AuthAnonymous,
	}
)

// MapRequest 集合：各 auth 接口的入参构造。

// SendCaptchaRequest 构造发送验证码入参。
func SendCaptchaRequest(req *mmpb.SendCaptchaRequest) map[string]any {
	return map[string]any{"cellphone": req.GetPhone(), "ctcode": "86"}
}

// LoginCellphoneRequest 构造手机登录入参。
func LoginCellphoneRequest(req *mmpb.LoginByCellphoneRequest) map[string]any {
	return map[string]any{
		"phone":         req.GetPhone(),
		"captcha":       req.GetCaptcha(),
		"countrycode":   "86",
		"rememberLogin": "true",
	}
}

// LoginQrcodeRequest 构造获取二维码 key 入参。
// type=1 是 weapi 二维码登录的值(参考 chaunsin Go 版 CLI 验证)。
func LoginQrcodeRequest(_ *mmpb.LoginQrcodeRequest) map[string]any {
	return map[string]any{"type": 1}
}

// CheckQrcodeRequest 构造轮询二维码入参。
func CheckQrcodeRequest(req *mmpb.CheckQrcodeRequest) map[string]any {
	return map[string]any{"key": req.GetKey(), "type": 1}
}

// LoginStatusRequest 构造查询登录态入参。
func LoginStatusRequest(_ *mmpb.LoginStatusRequest) map[string]any {
	return map[string]any{}
}

// LogoutRequest 构造登出入参。
func LogoutRequest(_ *mmpb.LogoutRequest) map[string]any {
	return map[string]any{}
}

// RefreshLoginRequest 构造刷新登录态入参(无业务参数,凭据只在 cookie 里)。
func RefreshLoginRequest(_ *mmpb.RefreshLoginRequest) map[string]any {
	return map[string]any{}
}

// RawDoer 执行一次 auth 调用并返回 raw + Set-Cookie:engine 的 RawDoWithCookieAndInput,或 CLI 的 kit.RawDo。
type RawDoer func(ctx context.Context, meta engine.Meta, params map[string]any) (json.RawMessage, string, error)

// Refresh 刷新 ctx 里 cookie(engine.WithCookie 注入)对应的登录态,返回合并了上游新下发值的完整 cookie;
// renewed 表示上游是否下发了新值。cookie 失效时返回未授权(上游 code=301)。
func Refresh(ctx context.Context, do RawDoer) (cookie string, renewed bool, err error) {
	_, setCookie, err := do(ctx, RefreshLogin, RefreshLoginRequest(nil))
	if err != nil {
		return "", false, err
	}
	return engine.MergeCookies(engine.CookieFromContext(ctx), setCookie), setCookie != "", nil
}

// QrcodeURL 从 key 构造二维码扫描 URL。
func QrcodeURL(key string) string {
	return fmt.Sprintf("https://music.163.com/login?codekey=%s", key)
}
