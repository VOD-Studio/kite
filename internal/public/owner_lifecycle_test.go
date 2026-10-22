package public

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	merrors "github.com/VOD-Studio/kite/errors"
	"github.com/VOD-Studio/kite/internal/cache"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

func lifecycleAccount(t *testing.T, id string, updatedAt time.Time) ServiceAccount {
	t.Helper()
	path := filepath.Join(t.TempDir(), id+".json")
	require.NoError(t, WriteSessionFile(path, SessionFile{
		Cookie: "MUSIC_U=" + id + "; __csrf=" + id,
		UserID: 1, Nickname: id, UpdatedAt: updatedAt,
	}))
	return ServiceAccount{ID: id, SessionFile: path}
}

func awaitOwnerValue[T any](t *testing.T, values <-chan T) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for owner lifecycle operation")
		var zero T
		return zero
	}
}

func TestOwnerExpiredReloadAndExternalRecovery(t *testing.T) {
	now := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	account := lifecycleAccount(t, "expired", now)
	o := newOwnerSession(account.SessionFile, func() time.Time { return now })
	original, err := os.Stat(account.SessionFile)
	require.NoError(t, err)
	o.snapshot().expire()
	for range 2 {
		o.reload(true)
		require.Equal(t, loginExpired, o.state(), "强制检查不能复活同一个失效文件")
		require.Empty(t, o.cookie())
	}

	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.SetCookie(w, &http.Cookie{Name: "MUSIC_U", Value: "unexpected"})
		_, _ = w.Write([]byte(`{"code":200}`))
	}))
	t.Cleanup(upstream.Close)
	eng := engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(upstream.URL))
	require.NoError(t, o.refresh(context.Background(), eng))
	require.Zero(t, calls.Load(), "未重新登录前不再续期已失效会话")
	require.Equal(t, loginExpired, o.state())

	relogin, err := ReadSessionFile(account.SessionFile)
	require.NoError(t, err)
	relogin.Cookie = "MUSIC_U=relogin; __csrf=relogin"
	require.NoError(t, WriteSessionFile(account.SessionFile, relogin))
	require.NoError(t, os.Chtimes(account.SessionFile, original.ModTime(), original.ModTime()))
	replaced, err := os.Stat(account.SessionFile)
	require.NoError(t, err)
	require.Equal(t, original.Size(), replaced.Size(), "相同大小与 mtime 仍需识别原子替换")
	o.reload(true)
	require.Equal(t, loginActive, o.state())
	require.Equal(t, relogin.Cookie, o.cookie(), "相同 mtime 的原子替换仍是新登录")
}

func TestOwnerDeletedAndMalformedFilesRecover(t *testing.T) {
	now := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	account := lifecycleAccount(t, "initial", now)
	o := newOwnerSession(account.SessionFile, func() time.Time { return now })
	require.NoError(t, os.Remove(account.SessionFile))
	o.reload(true)
	require.Equal(t, loginNone, o.state())
	require.Empty(t, o.cookie())

	require.NoError(t, os.WriteFile(account.SessionFile, []byte("{broken"), 0o600))
	o.reload(true)
	require.Equal(t, loginNone, o.state())
	require.Empty(t, o.cookie())
	malformed, err := os.Stat(account.SessionFile)
	require.NoError(t, err)
	require.NoError(t, WriteSessionFile(account.SessionFile, SessionFile{Cookie: "MUSIC_U=recovered", UpdatedAt: now}))
	require.NoError(t, os.Chtimes(account.SessionFile, malformed.ModTime(), malformed.ModTime()))
	o.reload(true)
	require.Equal(t, loginActive, o.state())
	require.Equal(t, "MUSIC_U=recovered", o.cookie())
}

func TestOwnerDelayedRefreshPreservesExternalLogin(t *testing.T) {
	for _, unauthorized := range []bool{false, true} {
		name := "success"
		if unauthorized {
			name = "unauthorized"
		}
		t.Run(name, func(t *testing.T) {
			now := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
			account := lifecycleAccount(t, "old", now)
			o := newOwnerSession(account.SessionFile, func() time.Time { return now })
			original, err := os.Stat(account.SessionFile)
			require.NoError(t, err)
			started, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				close(started)
				<-release
				if unauthorized {
					_, _ = w.Write([]byte(`{"code":301}`))
					return
				}
				http.SetCookie(w, &http.Cookie{Name: "MUSIC_U", Value: "stale-refresh"})
				_, _ = w.Write([]byte(`{"code":200}`))
			}))
			t.Cleanup(upstream.Close)
			t.Cleanup(unblock)
			eng := engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(upstream.URL))
			done := make(chan error, 1)
			go func() { done <- o.refresh(context.Background(), eng) }()
			awaitOwnerValue(t, started)

			relogin := SessionFile{Cookie: "MUSIC_U=relogin", UserID: 2, Nickname: "new login", UpdatedAt: now}
			require.NoError(t, WriteSessionFile(account.SessionFile, relogin))
			require.NoError(t, os.Chtimes(account.SessionFile, original.ModTime(), original.ModTime()))
			unblock()
			err = awaitOwnerValue(t, done)
			if unauthorized {
				require.ErrorIs(t, err, merrors.ErrUnauthorized)
			} else {
				require.NoError(t, err)
			}
			file, err := ReadSessionFile(account.SessionFile)
			require.NoError(t, err)
			require.Equal(t, relogin, file, "旧响应不能覆盖磁盘上的新登录")
			require.Equal(t, loginActive, o.state())
			require.Equal(t, relogin.Cookie, o.cookie())
		})
	}
}

func TestOwnerDelayedRefreshPreservesNewerRefresh(t *testing.T) {
	for _, unauthorized := range []bool{false, true} {
		name := "success"
		if unauthorized {
			name = "unauthorized"
		}
		t.Run(name, func(t *testing.T) {
			now := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
			account := lifecycleAccount(t, "old", now)
			o := newOwnerSession(account.SessionFile, func() time.Time { return now })
			started, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if calls.Add(1) == 1 {
					close(started)
					<-release
					if unauthorized {
						_, _ = w.Write([]byte(`{"code":301}`))
						return
					}
					http.SetCookie(w, &http.Cookie{Name: "MUSIC_U", Value: "stale-refresh"})
				} else {
					http.SetCookie(w, &http.Cookie{Name: "MUSIC_U", Value: "new-refresh"})
				}
				_, _ = w.Write([]byte(`{"code":200}`))
			}))
			t.Cleanup(upstream.Close)
			t.Cleanup(unblock)
			eng := engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(upstream.URL))
			done := make(chan error, 1)
			go func() { done <- o.refresh(context.Background(), eng) }()
			awaitOwnerValue(t, started)
			require.NoError(t, o.refresh(context.Background(), eng))
			newer, err := ReadSessionFile(account.SessionFile)
			require.NoError(t, err)
			require.Contains(t, newer.Cookie, "MUSIC_U=new-refresh")
			unblock()
			err = awaitOwnerValue(t, done)
			if unauthorized {
				require.ErrorIs(t, err, merrors.ErrUnauthorized)
			} else {
				require.NoError(t, err)
			}
			file, err := ReadSessionFile(account.SessionFile)
			require.NoError(t, err)
			require.Equal(t, newer, file)
			require.Equal(t, loginActive, o.state())
			require.Equal(t, newer.Cookie, o.cookie())
		})
	}
}

func TestOwnerCredentialExpirationProtectsNewLogin(t *testing.T) {
	now := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	account := lifecycleAccount(t, "old", now)
	o := newOwnerSession(account.SessionFile, func() time.Time { return now })
	stale := o.snapshot()
	original, err := os.Stat(account.SessionFile)
	require.NoError(t, err)
	require.NoError(t, WriteSessionFile(account.SessionFile, SessionFile{Cookie: "MUSIC_U=relogin", UpdatedAt: now}))
	require.NoError(t, os.Chtimes(account.SessionFile, original.ModTime(), original.ModTime()))
	stale.expire()
	require.Equal(t, loginActive, o.state())
	require.Equal(t, "MUSIC_U=relogin", o.cookie())
	o.snapshot().expire()
	require.Equal(t, loginExpired, o.state(), "当前版本仍可正常标记失效")
}

func TestOwnerPoolIndependentRefresh(t *testing.T) {
	now := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	accounts := []ServiceAccount{lifecycleAccount(t, "primary", now), lifecycleAccount(t, "secondary", now)}
	pool := newOwnerPool(accounts, func() time.Time { return now })
	started, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("MUSIC_U")
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if cookie.Value == "primary" {
			close(started)
			<-release
		}
		http.SetCookie(w, &http.Cookie{Name: "MUSIC_U", Value: cookie.Value + "-renewed"})
		_, _ = w.Write([]byte(`{"code":200}`))
	}))
	t.Cleanup(upstream.Close)
	t.Cleanup(unblock)
	eng := engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(upstream.URL))
	staleSecondary := pool.sessions[1].snapshot()
	done := make(chan error, 1)
	go func() { done <- pool.sessions[0].refresh(context.Background(), eng) }()
	awaitOwnerValue(t, started)
	require.NoError(t, pool.sessions[1].refresh(context.Background(), eng))
	require.Contains(t, pool.sessions[0].cookie(), "MUSIC_U=primary;")
	require.Contains(t, pool.sessions[1].cookie(), "MUSIC_U=secondary-renewed")
	staleSecondary.expire()
	require.Equal(t, loginActive, pool.sessions[1].state(), "旧播放请求不能失效已续期版本")
	unblock()
	require.NoError(t, awaitOwnerValue(t, done))
	for i, account := range accounts {
		file, err := ReadSessionFile(account.SessionFile)
		require.NoError(t, err)
		require.Contains(t, file.Cookie, "MUSIC_U="+account.ID+"-renewed")
		require.Contains(t, file.Cookie, "__csrf="+account.ID)
		require.Equal(t, account.ID, file.Nickname)
		require.Equal(t, now, file.UpdatedAt)
		info, err := os.Stat(account.SessionFile)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
		require.Equal(t, file.Cookie, pool.sessions[i].cookie())
	}
}

func TestOwnerPoolRefreshLoopsAreIndependentAndCancel(t *testing.T) {
	now := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	stale := now.Add(-2 * refreshInterval)
	accounts := []ServiceAccount{lifecycleAccount(t, "primary", stale), lifecycleAccount(t, "secondary", stale)}
	pool := newOwnerPool(accounts, func() time.Time { return now })
	primaryStarted, secondaryStarted := make(chan struct{}), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// net/http 只有消费完 POST body 后才开始监听连接关闭,否则测试服务无法观察客户端取消。
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
			return
		}
		cookie, err := r.Cookie("MUSIC_U")
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if cookie.Value == "primary" {
			close(primaryStarted)
		} else {
			close(secondaryStarted)
		}
		<-r.Context().Done()
	}))
	t.Cleanup(upstream.Close)
	eng := engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(upstream.URL))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan struct{})
	go func() {
		pool.runRefresh(ctx, eng)
		close(done)
	}()
	awaitOwnerValue(t, primaryStarted)
	awaitOwnerValue(t, secondaryStarted)
	cancel()
	awaitOwnerValue(t, done)
}

func TestOwnerPoolRoundRobinAndAggregateState(t *testing.T) {
	now := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	primary, secondary := lifecycleAccount(t, "primary", now), lifecycleAccount(t, "secondary", now)
	missing := ServiceAccount{ID: "missing", SessionFile: filepath.Join(t.TempDir(), "missing.json")}
	pool := newOwnerPool([]ServiceAccount{primary, missing, secondary}, func() time.Time { return now })
	require.Equal(t, loginActive, pool.state())
	for _, want := range []string{"primary", "secondary", "primary", "secondary"} {
		require.Equal(t, "MUSIC_U="+want+"; __csrf="+want, pool.next().cookie)
	}
	pool.next().expire()
	require.Equal(t, loginActive, pool.state())
	for range 3 {
		require.Equal(t, "MUSIC_U=secondary; __csrf=secondary", pool.next().cookie)
	}
	pool.next().expire()
	require.Equal(t, loginExpired, pool.state())
	anonymous := pool.next()
	require.Empty(t, anonymous.cookie)
	anonymous.expire()
	require.Equal(t, loginExpired, pool.state())

	empty := newOwnerPool(nil, func() time.Time { return now })
	require.Equal(t, loginNone, empty.state())
	require.Empty(t, empty.next().cookie)
}

func TestOwnerPoolConcurrentRoundRobin(t *testing.T) {
	now := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	pool := newOwnerPool([]ServiceAccount{lifecycleAccount(t, "primary", now), lifecycleAccount(t, "secondary", now)},
		func() time.Time { return now })
	const requests = 128
	selected := make(chan string, requests)
	var wg sync.WaitGroup
	wg.Add(requests)
	for range requests {
		go func() {
			defer wg.Done()
			selected <- pool.next().cookie
		}()
	}
	wg.Wait()
	close(selected)
	counts := make(map[string]int)
	for cookie := range selected {
		counts[cookie]++
	}
	require.Equal(t, map[string]int{
		"MUSIC_U=primary; __csrf=primary":     requests / 2,
		"MUSIC_U=secondary; __csrf=secondary": requests / 2,
	}, counts)
}
