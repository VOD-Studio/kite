// Package service 的 AuthServer:需要 Set-Cookie 或组合调用的登录 RPC 手写,
// 纯转发由 auth_gen.go 生成。托管账号的持久化与调用方权限由 server 适配器维护。
package service

import (
	"context"
	"fmt"

	merrors "github.com/VOD-Studio/kite/errors"
	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/compose"
	authendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/auth"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// AuthServer 实现 AuthServiceServer。
type AuthServer struct {
	mmpb.UnimplementedAuthServiceServer
	eng *engine.Engine
}

// NewAuthServer 创建 AuthServer。
func NewAuthServer(eng *engine.Engine) *AuthServer {
	return &AuthServer{eng: eng}
}

// LoginByCellphone 手机号登录,返回完整的登录会话。
func (s *AuthServer) LoginByCellphone(ctx context.Context, req *mmpb.LoginByCellphoneRequest) (*mmpb.LoginByCellphoneResponse, error) {
	resp, setCookie, err := engine.ExecuteWithSetCookie(s.eng, ctx, authendpoint.LoginCellphone, req)
	if err != nil {
		return nil, err
	}
	// gRPC 契约与旧实现一致:登录响应里没有账号信息按未登录报错。
	// MapResponse 是宽容解码(CLI 据此可落盘仅 cookie 的会话),严格性在这里补回。
	if resp.GetSession().GetUserId() == 0 {
		return nil, fmt.Errorf("未登录: %w", merrors.ErrUnauthorized)
	}

	resp.Session.Cookie = setCookie

	return resp, nil
}

// CheckQrcode 轮询二维码登录状态。
func (s *AuthServer) CheckQrcode(ctx context.Context, req *mmpb.CheckQrcodeRequest) (*mmpb.CheckQrcodeResponse, error) {
	return compose.CheckQrcode(ctx, s.eng, req)
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

// RefreshLogin 刷新登录态,返回合并了上游新下发值的完整 cookie。
func (s *AuthServer) RefreshLogin(ctx context.Context, _ *mmpb.RefreshLoginRequest) (*mmpb.RefreshLoginResponse, error) {
	cookie, _, err := authendpoint.Refresh(ctx, s.eng)
	if err != nil {
		return nil, err
	}
	return &mmpb.RefreshLoginResponse{Cookie: cookie}, nil
}
