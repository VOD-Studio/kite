package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/account"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// managedUpstream models account-dependent responses, refresh rotation and upstream expiration.
// All traffic still traverses the real TCP gateway, gRPC interceptors, services and engine.
type managedUpstream struct {
	mu         sync.Mutex
	login      string
	generation int
	expired    map[string]bool
	lastCookie map[string]string
}

func (u *managedUpstream) serve(w http.ResponseWriter, r *http.Request) {
	u.mu.Lock()
	defer u.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	who := whoAmI(r)
	u.lastCookie[r.URL.Path] = r.Header.Get("Cookie")
	switch r.URL.Path {
	case "/weapi/login/cellphone", "/weapi/login/qrcode/client/login":
		id := 101
		if u.login == "B" {
			id = 202
		}
		http.SetCookie(w, &http.Cookie{Name: "MUSIC_U", Value: fmt.Sprintf("ACCOUNT_%s_%d", u.login, u.generation), Path: "/"})
		code := 200
		if strings.Contains(r.URL.Path, "qrcode") {
			code = 803
		}
		_, _ = fmt.Fprintf(w, `{"code":%d,"account":{"id":%d},"profile":{"nickname":%q}}`, code, id, u.login)
	case "/weapi/v6/playlist/detail":
		if u.expired[who] {
			_, _ = io.WriteString(w, `{"code":301}`)
			return
		}
		_, _ = fmt.Fprintf(w, `{"code":200,"playlist":{"id":2,"name":%q,"trackCount":0,"tracks":[]}}`, who)
	case "/weapi/w/nuser/account/get":
		if who == "anon" || u.expired[who] {
			_, _ = io.WriteString(w, `{"code":301}`)
			return
		}
		id := 101
		if who == "B" {
			id = 202
		}
		_, _ = fmt.Fprintf(w, `{"code":200,"account":{"id":%d},"profile":{"nickname":%q}}`, id, who)
	case "/weapi/login/token/refresh":
		if u.expired[who] {
			_, _ = io.WriteString(w, `{"code":301}`)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "MUSIC_U", Value: "ACCOUNT_" + who + "_renewed", Path: "/"})
		_, _ = io.WriteString(w, `{"code":200}`)
	case "/weapi/logout":
		_, _ = io.WriteString(w, `{"code":200}`)
	default:
		http.NotFound(w, r)
	}
}

func (u *managedUpstream) loginAs(who string, generation int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.login, u.generation = who, generation
}

func managedContext(t *testing.T, token, id string) context.Context {
	t.Helper()
	ctx := metadata.AppendToOutgoingContext(bg(t), "authorization", "Bearer "+token)
	if id != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, accountMetadataKey, id)
	}
	return ctx
}

func startManagedApp(t *testing.T, upstream string, store *account.Store) (*App, *grpc.ClientConn) {
	t.Helper()
	eng := engine.New(engine.WithBaseURL(upstream), engine.WithCache(&memCache{m: map[string][]byte{}}))
	app, err := NewApp(Options{
		GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0",
		AuthTokens: []AuthToken{{Name: "blog", Value: "t-blog"}, {Name: "bot", Value: "t-bot"}},
	}, eng, store)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		require.NoError(t, app.Shutdown(ctx))
	})
	conn, err := grpc.NewClient(app.GRPCAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return app, conn
}

func managedHTTP(t *testing.T, app *App, method, path, body, token, id string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequestWithContext(bg(t), method, "http://"+app.HTTPAddr()+path, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	if id != "" {
		req.Header.Set("Grpc-Metadata-X-Kite-Account-Id", id)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return resp.StatusCode, out
}

func requireAccountReason(t *testing.T, err error, code codes.Code, reason string) {
	t.Helper()
	require.Equal(t, code, status.Code(err), "%v", err)
	require.Equal(t, reason, errorInfoOf(t, err).GetReason())
}

func TestManagedAccounts_IdentityPermissionsPersistenceAndLifecycle(t *testing.T) {
	u := &managedUpstream{login: "A", generation: 1, expired: map[string]bool{}, lastCookie: map[string]string{}}
	up := httptest.NewServer(http.HandlerFunc(u.serve))
	t.Cleanup(up.Close)
	path := filepath.Join(t.TempDir(), "accounts.json")
	store, err := account.Open(path)
	require.NoError(t, err)
	app, conn := startManagedApp(t, up.URL, store)
	auth := mmpb.NewAuthServiceClient(conn)

	// REST login A and gRPC QR confirmation B both return opaque owned IDs, not cookies.
	code, out := managedHTTP(t, app, http.MethodPost, "/api/v1/auth/login/cellphone", `{"phone":"8613800138000","captcha":"1234"}`, "t-blog", "")
	require.Equal(t, http.StatusOK, code, "%v", out)
	sess := out["session"].(map[string]any)
	a := sess["accountId"].(string)
	require.NotEqual(t, "101", a)
	require.Empty(t, sess["cookie"])
	u.loginAs("B", 1)
	qr, err := auth.CheckQrcode(managedContext(t, "t-blog", ""), &mmpb.CheckQrcodeRequest{Key: "confirmed"})
	require.NoError(t, err)
	b := qr.GetSession().GetAccountId()
	require.NotEmpty(t, b)
	require.NotEqual(t, a, b)
	require.Empty(t, qr.GetSession().GetCookie())

	// Repeated and concurrent A/B/anonymous reads must return the selected identity,
	// including cache hits. Anonymous cannot borrow either stored account.
	playlists := mmpb.NewPlaylistServiceClient(conn)
	for _, row := range []struct{ id, want string }{{a, "A"}, {b, "B"}, {"", "anon"}, {a, "A"}} {
		pl, err := playlists.GetPlaylist(managedContext(t, "t-blog", row.id), &mmpb.GetPlaylistRequest{PlaylistId: 2})
		require.NoError(t, err)
		require.Equal(t, row.want, pl.GetPlaylist().GetName())
	}
	var wg sync.WaitGroup
	for i := range 24 {
		id, want := a, "A"
		if i%2 == 1 {
			id, want = b, "B"
		}
		wg.Go(func() {
			pl, err := playlists.GetPlaylist(managedContext(t, "t-blog", id), &mmpb.GetPlaylistRequest{PlaylistId: 2})
			if err != nil {
				t.Error(err)
				return
			}
			if pl.GetPlaylist().GetName() != want {
				t.Errorf("account identity: got %q want %q", pl.GetPlaylist().GetName(), want)
			}
		})
	}
	wg.Wait()
	code, out = managedHTTP(t, app, http.MethodGet, "/api/v1/playlists/2", "", "t-blog", b)
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "B", out["playlist"].(map[string]any)["name"])
	_, err = auth.LoginStatus(managedContext(t, "t-bot", a), &mmpb.LoginStatusRequest{})
	requireAccountReason(t, err, codes.PermissionDenied, ReasonAccountForbidden)
	code, out = managedHTTP(t, app, http.MethodGet, "/api/v1/auth/status", "", "t-bot", a)
	require.Equal(t, http.StatusForbidden, code)
	require.Equal(t, ReasonAccountForbidden, out["details"].([]any)[0].(map[string]any)["reason"])
	_, err = auth.LoginStatus(managedContext(t, "t-blog", "unknown"), &mmpb.LoginStatusRequest{})
	requireAccountReason(t, err, codes.NotFound, ReasonAccountNotFound)

	refresh, err := auth.RefreshLogin(managedContext(t, "t-blog", a), &mmpb.RefreshLoginRequest{})
	require.NoError(t, err)
	require.Equal(t, a, refresh.GetAccountId())
	require.Empty(t, refresh.GetCookie())

	// Shut down the original app, reopen only persisted state, and use the same IDs.
	require.NoError(t, conn.Close())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	require.NoError(t, app.Shutdown(ctx))
	cancel()
	reopened, err := account.Open(path)
	require.NoError(t, err)
	_, conn2 := startManagedApp(t, up.URL, reopened)
	auth2 := mmpb.NewAuthServiceClient(conn2)
	for _, row := range []struct {
		id   string
		user int64
	}{{a, 101}, {b, 202}} {
		st, err := auth2.LoginStatus(managedContext(t, "t-blog", row.id), &mmpb.LoginStatusRequest{})
		require.NoError(t, err)
		require.Equal(t, row.user, st.GetSession().GetUserId())
		require.Equal(t, row.id, st.GetSession().GetAccountId())
		require.Empty(t, st.GetSession().GetCookie())
		if row.id == a {
			u.mu.Lock()
			require.Contains(t, u.lastCookie["/weapi/w/nuser/account/get"], "ACCOUNT_A_renewed")
			u.mu.Unlock()
		}
	}

	// A expires independently; B remains usable. Persisted expiration fails before upstream.
	u.mu.Lock()
	u.expired["A"] = true
	u.mu.Unlock()
	_, err = auth2.LoginStatus(managedContext(t, "t-blog", a), &mmpb.LoginStatusRequest{})
	requireAccountReason(t, err, codes.Unauthenticated, ReasonAccountExpired)
	_, err = auth2.RefreshLogin(managedContext(t, "t-blog", a), &mmpb.RefreshLoginRequest{})
	requireAccountReason(t, err, codes.Unauthenticated, ReasonAccountExpired)
	_, err = auth2.LoginStatus(managedContext(t, "t-blog", b), &mmpb.LoginStatusRequest{})
	require.NoError(t, err)

	// Expired logout is a local revocation; active logout removes the upstream session too.
	for _, id := range []string{a, b} {
		_, err = auth2.Logout(managedContext(t, "t-blog", id), &mmpb.LogoutRequest{})
		require.NoError(t, err)
		_, err = auth2.LoginStatus(managedContext(t, "t-blog", id), &mmpb.LoginStatusRequest{})
		requireAccountReason(t, err, codes.NotFound, ReasonAccountNotFound)
	}
	final, err := account.Open(path)
	require.NoError(t, err)
	_, err = final.Get("caller:blog", b)
	require.ErrorIs(t, err, account.ErrNotFound)
}

func TestManagedAccounts_CredentialConflictsAndDisabledStore(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("invalid credential request reached upstream")
		http.Error(w, "unexpected", 500)
	}))
	t.Cleanup(up.Close)
	_, conn := startManagedApp(t, up.URL, nil)
	auth := mmpb.NewAuthServiceClient(conn)
	_, err := auth.LoginStatus(managedContext(t, "t-blog", "an-account"), &mmpb.LoginStatusRequest{})
	requireAccountReason(t, err, codes.FailedPrecondition, ReasonAccountsDisabled)
	for _, pairs := range [][]string{
		{accountMetadataKey, ""},
		{accountMetadataKey, "A", accountMetadataKey, "B"},
		{accountMetadataKey, "A", cookieMetadataKey, "MUSIC_U=B"},
		{accountMetadataKey, "A", cookieMetadataKey, ""},
	} {
		ctx := metadata.AppendToOutgoingContext(managedContext(t, "t-blog", ""), pairs...)
		_, err = auth.LoginStatus(ctx, &mmpb.LoginStatusRequest{})
		requireAccountReason(t, err, codes.InvalidArgument, ReasonInvalidCredentials)
	}

	// A caller may attach its default account selector to every RPC. Health remains
	// unauthenticated and must neither resolve nor disclose managed account state.
	healthCtx := metadata.AppendToOutgoingContext(bg(t), accountMetadataKey, "unknown")
	health, err := healthpb.NewHealthClient(conn).Check(healthCtx, &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_SERVING, health.GetStatus())
}

func TestManagedAccounts_StaleLifecycleCannotOverwriteNewLogin(t *testing.T) {
	for _, operation := range []string{"refresh", "expire", "logout"} {
		t.Run(operation, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/weapi/login/cellphone" {
					http.SetCookie(w, &http.Cookie{Name: "MUSIC_U", Value: "ACCOUNT_A_new", Path: "/"})
					_, _ = io.WriteString(w, `{"code":200,"account":{"id":101}}`)
					return
				}
				if strings.Contains(r.Header.Get("Cookie"), "ACCOUNT_A_old") {
					close(started)
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
					if operation == "expire" {
						_, _ = io.WriteString(w, `{"code":301}`)
						return
					}
					http.SetCookie(w, &http.Cookie{Name: "MUSIC_U", Value: "ACCOUNT_A_stale", Path: "/"})
					_, _ = io.WriteString(w, `{"code":200}`)
					return
				}
				_, _ = io.WriteString(w, `{"code":200,"account":{"id":101}}`)
			}))
			t.Cleanup(up.Close)
			store, err := account.Open(filepath.Join(t.TempDir(), "accounts.json"))
			require.NoError(t, err)
			record, err := store.Save("caller:blog", 101, "MUSIC_U=ACCOUNT_A_old")
			require.NoError(t, err)
			_, conn := startManagedApp(t, up.URL, store)
			auth := mmpb.NewAuthServiceClient(conn)
			result := make(chan error, 1)
			go func() {
				ctx := managedContext(t, "t-blog", record.ID)
				var err error
				switch operation {
				case "refresh":
					_, err = auth.RefreshLogin(ctx, &mmpb.RefreshLoginRequest{})
				case "expire":
					_, err = auth.LoginStatus(ctx, &mmpb.LoginStatusRequest{})
				case "logout":
					_, err = auth.Logout(ctx, &mmpb.LogoutRequest{})
				}
				result <- err
			}()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("lifecycle request did not reach upstream")
			}
			login, err := auth.LoginByCellphone(managedContext(t, "t-blog", ""), &mmpb.LoginByCellphoneRequest{Phone: "8613800138000", Captcha: "1234"})
			close(release)
			require.NoError(t, err)
			require.Equal(t, record.ID, login.GetSession().GetAccountId())
			requireAccountReason(t, <-result, codes.Aborted, ReasonAccountChanged)
			st, err := auth.LoginStatus(managedContext(t, "t-blog", record.ID), &mmpb.LoginStatusRequest{})
			require.NoError(t, err)
			require.Equal(t, int64(101), st.GetSession().GetUserId())
			saved, err := store.Get("caller:blog", record.ID)
			require.NoError(t, err)
			require.Equal(t, "MUSIC_U=ACCOUNT_A_new", saved.Cookie)
		})
	}
}
