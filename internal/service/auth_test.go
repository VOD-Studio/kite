package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	merrors "github.com/VOD-Studio/kite/errors"
	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/cache"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// RefreshLogin 带着调用方的 cookie 请求上游,把上游经 Set-Cookie 下发的新值合并进原 cookie 返回;
// cookie 失效(上游 code=301)时返回未授权,调用方据此引导重新登录。
func TestRefreshLogin(t *testing.T) {
	var lastCookie atomic.Value
	var expired atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/weapi/login/token/refresh", r.URL.Path)
		lastCookie.Store(r.Header.Get("Cookie"))
		w.Header().Set("Content-Type", "application/json")
		if expired.Load() {
			_, _ = w.Write([]byte(`{"code":301}`))
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "MUSIC_U", Value: "renewed", Path: "/"})
		_, _ = w.Write([]byte(`{"code":200}`))
	}))
	t.Cleanup(srv.Close)

	eng := engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(srv.URL))
	auth := NewAuthServer(eng, nil)
	ctx := engine.WithCookie(context.Background(), "MUSIC_U=old; __csrf=c")

	resp, err := auth.RefreshLogin(ctx, &mmpb.RefreshLoginRequest{})
	require.NoError(t, err)
	require.Contains(t, lastCookie.Load(), "MUSIC_U=old", "应带着调用方的 cookie 请求上游")
	require.Equal(t, "MUSIC_U=renewed; __csrf=c", resp.GetCookie(), "新值替换旧值,其余项保留")

	expired.Store(true)
	_, err = auth.RefreshLogin(ctx, &mmpb.RefreshLoginRequest{})
	require.ErrorIs(t, err, merrors.ErrUnauthorized)
}

func TestCheckQrcodeCompletesSessionFromFreshCookie(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/weapi/login/qrcode/client/login":
			http.SetCookie(w, &http.Cookie{Name: "MUSIC_U", Value: "fresh", Path: "/"})
			http.SetCookie(w, &http.Cookie{Name: "__csrf", Value: "fresh-csrf", Path: "/"})
			_, _ = w.Write([]byte(`{"code":803,"message":"授权登录成功"}`))
		case "/weapi/w/nuser/account/get":
			login, loginErr := r.Cookie("MUSIC_U")
			csrf, csrfErr := r.Cookie("__csrf")
			if loginErr != nil || csrfErr != nil || login.Value != "fresh" || csrf.Value != "fresh-csrf" {
				_, _ = w.Write([]byte(`{"code":301}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":200,"account":{"id":42},"profile":{"nickname":"扫码账号","avatarUrl":"https://p1.music.126.net/avatar.jpg"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)
	auth := NewAuthServer(engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(upstream.URL)), nil)
	ctx := engine.WithCookie(context.Background(), "MUSIC_U=old; __csrf=old-csrf")

	resp, err := auth.CheckQrcode(ctx, &mmpb.CheckQrcodeRequest{Key: "confirmed"})
	require.NoError(t, err)
	require.Equal(t, mmpb.QrcodeCode_QRCODE_CODE_CONFIRMED, resp.GetCode())
	require.NotNil(t, resp.GetSession())
	require.Equal(t, int64(42), resp.GetSession().GetUserId())
	require.Equal(t, "扫码账号", resp.GetSession().GetNickname())
	require.Equal(t, "https://p1.music.126.net/avatar.jpg", resp.GetSession().GetAvatarUrl())

	status, err := auth.LoginStatus(engine.WithCookie(ctx, resp.GetSession().GetCookie()), &mmpb.LoginStatusRequest{})
	require.NoError(t, err)
	require.Equal(t, int64(42), status.GetSession().GetUserId(), "返回的完整凭据必须能读取新登录账号")
}

func TestCheckQrcodePendingStates(t *testing.T) {
	for _, code := range []mmpb.QrcodeCode{
		mmpb.QrcodeCode_QRCODE_CODE_EXPIRED,
		mmpb.QrcodeCode_QRCODE_CODE_WAITING,
		mmpb.QrcodeCode_QRCODE_CODE_SCANNED,
	} {
		t.Run(code.String(), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/weapi/login/qrcode/client/login" {
					http.Error(w, "账号查询不可用", http.StatusServiceUnavailable)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				http.SetCookie(w, &http.Cookie{Name: "tracking", Value: "anonymous", Path: "/"})
				_, _ = fmt.Fprintf(w, `{"code":%d,"message":"等待授权"}`, code)
			}))
			t.Cleanup(upstream.Close)
			auth := NewAuthServer(engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(upstream.URL)), nil)

			resp, err := auth.CheckQrcode(context.Background(), &mmpb.CheckQrcodeRequest{Key: "pending"})
			require.NoError(t, err)
			require.Equal(t, code, resp.GetCode())
			require.Equal(t, "等待授权", resp.GetMessage())
			require.Nil(t, resp.GetSession(), "未确认状态下的跟踪 Cookie 不是登录凭据")
		})
	}
}

func TestCheckQrcodeUsesInlineAccount(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/weapi/login/qrcode/client/login" {
			http.Error(w, "账号查询不可用", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		http.SetCookie(w, &http.Cookie{Name: "MUSIC_U", Value: "inline", Path: "/"})
		_, _ = w.Write([]byte(`{"code":803,"account":{"id":43},"profile":{"nickname":"完整响应"}}`))
	}))
	t.Cleanup(upstream.Close)
	auth := NewAuthServer(engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(upstream.URL)), nil)

	resp, err := auth.CheckQrcode(context.Background(), &mmpb.CheckQrcodeRequest{Key: "inline"})
	require.NoError(t, err)
	require.Equal(t, mmpb.QrcodeCode_QRCODE_CODE_CONFIRMED, resp.GetCode())
	require.Equal(t, int64(43), resp.GetSession().GetUserId())
	require.Equal(t, "完整响应", resp.GetSession().GetNickname())
	require.Equal(t, "MUSIC_U=inline", resp.GetSession().GetCookie())
}

func TestCheckQrcodeRejectsIncompleteSession(t *testing.T) {
	for _, tt := range []struct {
		name          string
		missingCookie bool
		accountStatus int
		accountBody   string
		wantErr       error
	}{
		{
			name:          "missing_fresh_cookie",
			missingCookie: true,
			accountBody:   `{"code":200,"account":{"id":99}}`,
			wantErr:       merrors.ErrInvalidResponse,
		},
		{
			name:        "rejected_login",
			accountBody: `{"code":301}`,
			wantErr:     merrors.ErrUnauthorized,
		},
		{
			name:          "account_unavailable",
			accountStatus: http.StatusServiceUnavailable,
			accountBody:   `{"code":503}`,
			wantErr:       merrors.ErrUpstreamUnavailable,
		},
		{
			name:        "malformed_account",
			accountBody: `{"code":200,"account":{"id":"invalid"}}`,
			wantErr:     merrors.ErrInvalidResponse,
		},
		{
			name:        "missing_account",
			accountBody: `{"code":200,"account":null}`,
			wantErr:     merrors.ErrInvalidResponse,
		},
		{
			name:        "invalid_account_id",
			accountBody: `{"code":200,"account":{"id":-1}}`,
			wantErr:     merrors.ErrInvalidResponse,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/weapi/login/qrcode/client/login":
					if !tt.missingCookie {
						http.SetCookie(w, &http.Cookie{Name: "MUSIC_U", Value: "fresh", Path: "/"})
					}
					_, _ = w.Write([]byte(`{"code":803}`))
				case "/weapi/w/nuser/account/get":
					if tt.accountStatus != 0 {
						w.WriteHeader(tt.accountStatus)
					}
					_, _ = w.Write([]byte(tt.accountBody))
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(upstream.Close)
			auth := NewAuthServer(engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(upstream.URL)), nil)
			ctx := engine.WithCookie(context.Background(), "MUSIC_U=old-account")

			resp, err := auth.CheckQrcode(ctx, &mmpb.CheckQrcodeRequest{Key: "incomplete"})
			require.ErrorIs(t, err, tt.wantErr)
			require.Nil(t, resp, "不完整凭据不能作为已确认登录返回")
		})
	}
}

// LoginByCellphone 的 gRPC 契约:上游 HTTP 200 + code=200 + Set-Cookie 但响应体
// 解不出账号信息时,service 仍按未登录报错(endpoint 的 MapResponse 是宽容解码,
// CLI 据此可落盘仅 cookie 的会话;严格性由 service 补回)。
func TestLoginByCellphone_NoAccountIsUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		http.SetCookie(w, &http.Cookie{Name: "MUSIC_U", Value: "leaky", Path: "/"})
		_, _ = w.Write([]byte(`{"code":200}`)) // 有 cookie、无 account
	}))
	t.Cleanup(srv.Close)

	eng := engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(srv.URL))
	auth := NewAuthServer(eng, nil)

	_, err := auth.LoginByCellphone(context.Background(), &mmpb.LoginByCellphoneRequest{Phone: "8613800138000", Captcha: "1234"})
	require.ErrorIs(t, err, merrors.ErrUnauthorized, "无账号信息的登录响应按未登录报错")
}
