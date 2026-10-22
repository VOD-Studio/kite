// Package server 的请求凭据解析与托管账号生命周期。
package server

import (
	"context"
	"errors"
	"strings"

	merrors "github.com/VOD-Studio/kite/errors"
	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/account"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

const (
	cookieMetadataKey  = "x-netease-cookie"
	accountMetadataKey = "x-kite-account-id"
	logoutMethod       = "/netease.music.v1.AuthService/Logout"
)

// accountInterceptor 只解析请求显式提供的凭据,不从托管账号中自动选号。
// 续期/失效/退出只提交本次请求所用版本,旧请求不能覆盖新登录。
func accountInterceptor(accounts *account.Store) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if strings.HasPrefix(info.FullMethod, healthMethodPrefix) {
			return handler(ctx, req)
		}
		md, _ := metadata.FromIncomingContext(ctx)
		cookies, ids := md.Get(cookieMetadataKey), md.Get(accountMetadataKey)
		explicit := len(ids) > 0
		cookie := ""
		if len(cookies) > 0 {
			cookie = cookies[len(cookies)-1]
		}
		if explicit && (len(ids) != 1 || strings.TrimSpace(ids[0]) == "" || len(cookies) > 0 || loginRequest(req)) {
			return nil, statusError(codes.InvalidArgument, ReasonInvalidCredentials,
				"账号 ID 必须唯一且非空,不能与 Cookie 同传或用于发起登录", nil)
		}

		var record account.Record
		tracked := false
		if explicit {
			if accounts == nil {
				return nil, statusError(codes.FailedPrecondition, ReasonAccountsDisabled, "服务未启用账号托管", nil)
			}
			var err error
			record, err = accounts.Get(accountOwner(ctx), strings.TrimSpace(ids[0]))
			if err != nil {
				return nil, accountStatusError(err)
			}
			if record.Expired {
				if info.FullMethod == logoutMethod {
					if err := accounts.Delete(record); err != nil {
						return nil, accountStatusError(err)
					}
					return &mmpb.LogoutResponse{}, nil
				}
				return nil, expiredAccountError()
			}
			cookie, tracked = record.Cookie, true
		} else if accounts != nil && cookie != "" && !loginRequest(req) {
			// 旧客户端仍可传 Cookie;其刷新与退出也同步自己拥有的托管记录。
			record, tracked = accounts.FindCookie(accountOwner(ctx), cookie)
		}

		resp, err := handler(engine.WithCookie(ctx, cookie), req)
		if tracked && info.FullMethod == logoutMethod && (err == nil || errors.Is(err, merrors.ErrUnauthorized)) {
			if err := accounts.Delete(record); err != nil {
				return nil, accountStatusError(err)
			}
			return &mmpb.LogoutResponse{}, nil
		}
		if err != nil {
			if tracked && errors.Is(err, merrors.ErrUnauthorized) {
				if persistErr := accounts.Expire(record); persistErr != nil {
					return nil, accountStatusError(persistErr)
				}
				if explicit {
					return nil, expiredAccountError()
				}
			}
			return nil, err
		}

		if accounts == nil {
			return resp, nil
		}
		if sess := loginSession(resp); sess != nil {
			if sess.GetUserId() <= 0 || sess.GetCookie() == "" {
				return nil, merrors.ErrInvalidResponse
			}
			saved, err := accounts.Save(accountOwner(ctx), sess.GetUserId(), sess.GetCookie())
			if err != nil {
				return nil, accountStatusError(err)
			}
			sess.AccountId, sess.Cookie = saved.ID, ""
		}
		if tracked {
			switch out := resp.(type) {
			case *mmpb.RefreshLoginResponse:
				if _, err := accounts.Update(record, out.GetCookie()); err != nil {
					return nil, accountStatusError(err)
				}
				if explicit {
					out.AccountId, out.Cookie = record.ID, ""
				}
			case *mmpb.LoginStatusResponse:
				if out.GetSession() == nil {
					if err := accounts.Expire(record); err != nil {
						return nil, accountStatusError(err)
					}
					if explicit {
						return nil, expiredAccountError()
					}
				} else if explicit {
					out.Session.AccountId, out.Session.Cookie = record.ID, ""
				}
			}
		}
		return resp, nil
	}
}

func loginRequest(req any) bool {
	switch req.(type) {
	case *mmpb.SendCaptchaRequest, *mmpb.LoginByCellphoneRequest, *mmpb.LoginQrcodeRequest, *mmpb.CheckQrcodeRequest:
		return true
	default:
		return false
	}
}

func loginSession(resp any) *mmpb.Session {
	switch out := resp.(type) {
	case *mmpb.LoginByCellphoneResponse:
		return out.GetSession()
	case *mmpb.CheckQrcodeResponse:
		return out.GetSession()
	default:
		return nil
	}
}

func expiredAccountError() error {
	return statusError(codes.Unauthenticated, ReasonAccountExpired, "托管账号登录态已失效,请重新登录", nil)
}

func accountStatusError(err error) error {
	switch {
	case errors.Is(err, account.ErrNotFound):
		return statusError(codes.NotFound, ReasonAccountNotFound, "托管账号不存在或已退出", nil)
	case errors.Is(err, account.ErrForbidden):
		return statusError(codes.PermissionDenied, ReasonAccountForbidden, "无权使用这个托管账号", nil)
	case errors.Is(err, account.ErrChanged):
		return statusError(codes.Aborted, ReasonAccountChanged, "账号登录态已被另一请求更新,请重新发起请求", nil)
	default:
		return err
	}
}
