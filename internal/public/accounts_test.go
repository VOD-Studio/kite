package public

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/cache"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

func TestStreamCacheFollowsCredentials(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := "anon"
		if cookie, err := r.Cookie("MUSIC_U"); err == nil {
			name = cookie.Value
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"code":200,"data":[{"id":1,"url":"https://m801.music.126.net/%s.mp3","code":200}]}`, name)
	}))
	t.Cleanup(upstream.Close)
	cat := newCachedCatalog(engineCatalog{eng: engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(upstream.URL))}, time.Now)
	for _, name := range []string{"a", "b", "a", "a-renewed", "b"} {
		url, err := cat.StreamURL(context.Background(), 1, mmpb.SongLevel_SONG_LEVEL_EXHIGH, "MUSIC_U="+name)
		require.NoError(t, err)
		require.Equal(t, "https://m801.music.126.net/"+name+".mp3", url.GetUrl())
	}
	url, err := cat.StreamURL(context.Background(), 1, mmpb.SongLevel_SONG_LEVEL_EXHIGH, "")
	require.NoError(t, err)
	require.Equal(t, "https://m801.music.126.net/anon.mp3", url.GetUrl())
}

// publicPlaybackUpstream distinguishes account rights, not merely which cookie was forwarded.
type publicPlaybackUpstream struct {
	mu       sync.Mutex
	calls    []string
	expired  map[string]bool
	failCode map[string]int
}

func (u *publicPlaybackUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/weapi/v3/song/detail" {
		_, _ = w.Write([]byte(`{"code":200,"songs":[{"id":1,"name":"VIP","fee":1,"ar":[],"al":{},"dt":1000}]}`))
		return
	}
	if r.URL.Path != "/eapi/song/enhance/player/url/v1" {
		http.Error(w, "unexpected path", http.StatusNotFound)
		return
	}
	name := "anon"
	if cookie, err := r.Cookie("MUSIC_U"); err == nil {
		name = cookie.Value
	}
	u.mu.Lock()
	u.calls = append(u.calls, name)
	expired := u.expired[name]
	code := u.failCode[name]
	u.mu.Unlock()
	if expired {
		_, _ = w.Write([]byte(`{"code":301}`))
		return
	}
	if code != 0 {
		_, _ = fmt.Fprintf(w, `{"code":%d,"data":[{"id":1,"url":null,"code":%d}]}`, code, code)
		return
	}
	trial := ""
	if name == "b" {
		trial = `,"freeTrialInfo":{"start":0,"end":30000}`
	}
	_, _ = fmt.Fprintf(w, `{"code":200,"data":[{"id":1,"url":"https://m801.music.126.net/%s.mp3","code":200%s}]}`, name, trial)
}

func (u *publicPlaybackUpstream) expire(name string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.expired[name] = true
}

func (u *publicPlaybackUpstream) history() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.calls...)
}

func multiAccountServer(t *testing.T) (*Server, *publicPlaybackUpstream, *clock, http.Handler) {
	t.Helper()
	c := &clock{t: time.Now()}
	opts := testOptions()
	opts.Playlists = nil
	opts.Songs = []int64{1}
	for _, id := range []string{"a", "b"} {
		path := filepath.Join(t.TempDir(), id+".json")
		require.NoError(t, WriteSessionFile(path, SessionFile{Cookie: "MUSIC_U=" + id, UpdatedAt: c.now()}))
		opts.Accounts = append(opts.Accounts, ServiceAccount{ID: id, SessionFile: path})
	}
	u := &publicPlaybackUpstream{expired: map[string]bool{}, failCode: map[string]int{}}
	upstream := httptest.NewServer(u)
	t.Cleanup(upstream.Close)
	eng := engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(upstream.URL))
	s := newServer(engineCatalog{eng: eng}, opts, c.now)
	return s, u, c, s.Handler()
}

func TestMultiAccountPlaybackSelectionAndExpiry(t *testing.T) {
	_, u, c, h := multiAccountServer(t)
	path := streamPath(t, h, "1")
	rec := get(t, h, path+"&account=b", "Cookie", "MUSIC_U=attacker", "X-Kite-Account-Id", "b")
	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, "https://m801.music.126.net/a.mp3", rec.Header().Get("Location"), "public input cannot override server selection")
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	rec = get(t, h, path)
	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, "https://m801.music.126.net/b.mp3", rec.Header().Get("Location"))
	require.Equal(t, "start=0; end=30000", rec.Header().Get("X-Kite-Trial"), "no cross-account entitlement cache or trial fallback")
	require.Equal(t, []string{"a", "b"}, u.history())

	c.add(streamTTL + time.Second)
	u.expire("a")
	rec = get(t, h, path)
	require.Equal(t, "UNPLAYABLE", reason(t, rec))
	require.Equal(t, []string{"a", "b", "a"}, u.history(), "unauthorized does not retry another account in the same request")
	require.Equal(t, loginActive, decode[infoDTO](t, get(t, h, "/player/v1/info")).Login)
	rec = get(t, h, path)
	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, "https://m801.music.126.net/b.mp3", rec.Header().Get("Location"))

	c.add(streamTTL + time.Second)
	u.expire("b")
	require.Equal(t, "UNPLAYABLE", reason(t, get(t, h, path)))
	require.Equal(t, loginExpired, decode[infoDTO](t, get(t, h, "/player/v1/info")).Login)
	require.False(t, decode[trackDTO](t, get(t, h, "/player/v1/tracks/1")).Playable)
	rec = get(t, h, path)
	require.Equal(t, http.StatusFound, rec.Code, "anonymous may still play whatever upstream permits")
	require.Equal(t, "https://m801.music.126.net/anon.mp3", rec.Header().Get("Location"))
	require.Equal(t, []string{"a", "b", "a", "b", "b", "anon"}, u.history())
}

func TestMultiAccountReloadAndReadOnlyBoundary(t *testing.T) {
	s, u, c, h := multiAccountServer(t)
	require.Equal(t, "LINK_INVALID", reason(t, get(t, h, "/player/v1/tracks/1/stream")))
	require.Equal(t, "NOT_ALLOWED", reason(t, get(t, h, "/player/v1/tracks/99")))
	req := httptest.NewRequest(http.MethodPost, "/player/v1/tracks/1/stream", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.NotEqual(t, http.StatusFound, rec.Code)
	require.Empty(t, u.history(), "invalid requests cannot consume selection or credentials")
	path := streamPath(t, h, "1")
	require.Equal(t, "https://m801.music.126.net/a.mp3", get(t, h, path).Header().Get("Location"))
	require.Equal(t, "https://m801.music.126.net/b.mp3", get(t, h, path).Header().Get("Location"))
	require.NoError(t, WriteSessionFile(s.opts.Accounts[0].SessionFile, SessionFile{Cookie: "MUSIC_U=a-new", UpdatedAt: c.now()}))
	c.add(statInterval + time.Second)
	require.Equal(t, "https://m801.music.126.net/a-new.mp3", get(t, h, path).Header().Get("Location"), "hot login does not reuse old account cache")

	for _, account := range s.opts.Accounts {
		require.NoError(t, os.Remove(account.SessionFile))
	}
	c.add(statInterval + time.Second)
	rec = get(t, h, "/player/v1/info")
	require.Equal(t, loginNone, decode[infoDTO](t, rec).Login)
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	var info map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &info))
	require.Equal(t, map[string]any{"version": "1", "login": loginNone, "quality": "exhigh"}, info, "no account identities or credentials in public status")
	require.Equal(t, "https://m801.music.126.net/anon.mp3", get(t, h, path).Header().Get("Location"))
}

func TestMultiAccountDoesNotProbeOtherEntitlements(t *testing.T) {
	for _, tc := range []struct {
		name   string
		code   int
		reason string
	}{
		{"no_rights", 200, "UNPLAYABLE"},
		{"unavailable_track", 404, "UNPLAYABLE"},
		{"rate_limited", 460, "UPSTREAM_ERROR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, u, _, h := multiAccountServer(t)
			u.failCode["a"] = tc.code
			path := streamPath(t, h, "1")
			require.Equal(t, tc.reason, reason(t, get(t, h, path)))
			for _, selected := range u.history() {
				require.Equal(t, "a", selected, "engine may retry the same credential, never another account")
			}
			require.Equal(t, "https://m801.music.126.net/b.mp3", get(t, h, path).Header().Get("Location"))
			require.Equal(t, tc.reason, reason(t, get(t, h, path)), "lack of rights or rate limiting does not expire credentials")
			require.Equal(t, loginActive, decode[infoDTO](t, get(t, h, "/player/v1/info")).Login)
		})
	}
}
