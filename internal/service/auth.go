// Package service 的 AuthService impl。
//
// auth 接口与其它域一样走声明式 endpoint,只是入口用 engine.ExecuteWithSetCookie
// (登录类接口要拿 Set-Cookie 建 session);CheckQrcode 的确认-补查-落盘组合在
// compose 层;session 池的写入是本层的装配职责。
package service

import (
	"context"
	"fmt"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/compose"
	authendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/auth"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// AuthServer 实现 AuthServiceServer。
//
// 持有 *engine.Engine 和 SessionStore（登录成功后存 cookie 池）。
type AuthServer struct {
	mmpb.UnimplementedAuthServiceServer
	eng      *engine.Engine
	sessions session.SessionStore
}

// NewAuthServer 创建 AuthServer。
func NewAuthServer(eng *engine.Engine, sessions session.SessionStore) *AuthServer {
	return &AuthServer{eng: eng, sessions: sessions}
}

// SendCaptcha 发送验证码。
func (s *AuthServer) SendCaptcha(ctx context.Context, req *mmpb.SendCaptchaRequest) (*mmpb.SendCaptchaResponse, error) {
	return engine.ExecuteWithCookie(s.eng, ctx, authendpoint.SendCaptcha, req)
}

// LoginByCellphone 手机号登录，返回 Session 并存入 cookie 池。
func (s *AuthServer) LoginByCellphone(ctx context.Context, req *mmpb.LoginByCellphoneRequest) (*mmpb.LoginByCellphoneResponse, error) {
	resp, setCookie, err := engine.ExecuteWithSetCookie(s.eng, ctx, authendpoint.LoginCellphone, req)
	if err != nil {
		return nil, err
	}

	// 存入 session 池（UserID 用数字 ID 转字符串，Nickname 不唯一不能做 key）。
	s.saveSession(ctx, resp.GetSession().GetUserId(), setCookie)
	resp.Session.Cookie = setCookie

	return resp, nil
}

// LoginQrcode 获取登录二维码。
func (s *AuthServer) LoginQrcode(ctx context.Context, req *mmpb.LoginQrcodeRequest) (*mmpb.LoginQrcodeResponse, error) {
	return engine.ExecuteWithCookie(s.eng, ctx, authendpoint.LoginQrcode, req)
}

// CheckQrcode 轮询二维码登录状态。
func (s *AuthServer) CheckQrcode(ctx context.Context, req *mmpb.CheckQrcodeRequest) (*mmpb.CheckQrcodeResponse, error) {
	resp, err := compose.CheckQrcode(ctx, s.eng, req)
	if err != nil {
		return nil, err
	}

	if sess := resp.GetSession(); sess != nil {
		s.saveSession(ctx, sess.UserId, sess.Cookie)
	}

	return resp, nil
}

// LoginStatus 查询当前登录态。
func (s *AuthServer) LoginStatus(ctx context.Context, req *mmpb.LoginStatusRequest) (*mmpb.LoginStatusResponse, error) {
	resp, err := engine.ExecuteWithCookie(s.eng, ctx, authendpoint.LoginStatus, req)
	if err != nil {
		return nil, err
	}
	if resp.GetSession() != nil {
		resp.Session.Cookie = engine.CookieFromContext(ctx)
	}
	return resp, nil
}

// Logout 登出。
func (s *AuthServer) Logout(ctx context.Context, req *mmpb.LogoutRequest) (*mmpb.LogoutResponse, error) {
	return engine.ExecuteWithCookie(s.eng, ctx, authendpoint.Logout, req)
}

// RefreshLogin 刷新登录态,返回合并了上游新下发值的完整 cookie。
//
// 响应体不含用户信息,拿不到 UserID,所以不写 session 池;cookie 由调用方保存(与登录接口一致)。
func (s *AuthServer) RefreshLogin(ctx context.Context, _ *mmpb.RefreshLoginRequest) (*mmpb.RefreshLoginResponse, error) {
	cookie, _, err := authendpoint.Refresh(ctx, s.eng)
	if err != nil {
		return nil, err
	}
	return &mmpb.RefreshLoginResponse{Cookie: cookie}, nil
}

// saveSession 把登录态 cookie 存入 session 池（UserID 用数字转字符串）。
func (s *AuthServer) saveSession(ctx context.Context, userID int64, cookie string) {
	if s.sessions == nil || userID == 0 || cookie == "" {
		return
	}
	_ = s.sessions.Save(ctx, &session.Session{UserID: fmt.Sprintf("%d", userID), Cookie: cookie})
}
