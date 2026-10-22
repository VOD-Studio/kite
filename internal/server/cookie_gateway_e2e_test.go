package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// memCache 是测试用的进程内缓存(cmd/server 线上用 cache.Noop,这里要验证按身份隔离)。
type memCache struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (c *memCache) Get(_ context.Context, k string) ([]byte, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[k]
	return v, ok, nil
}

func (c *memCache) Set(_ context.Context, k string, v []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[k] = v
	return nil
}

func (c *memCache) Delete(_ context.Context, k string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, k)
	return nil
}

// whoAmI 从上游请求的 Cookie 头认出调用方:A / B / 匿名。
func whoAmI(r *http.Request) string {
	switch c := r.Header.Get("Cookie"); {
	case strings.Contains(c, "MUSIC_U=ACCOUNT_A"):
		return "A"
	case strings.Contains(c, "MUSIC_U=ACCOUNT_B"):
		return "B"
	default:
		return "anon"
	}
}

// issue #90:REST 调用方经 Grpc-Metadata-X-Netease-Cookie 带的 cookie 要一路到达上游;
// 不带时匿名。歌单详情会缓存,带身份的结果按身份隔离。
func TestGateway_CallerCookieReachesAccountReads(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	seen := map[string][]string{}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		who := whoAmI(r)
		mu.Lock()
		seen[r.URL.Path] = append(seen[r.URL.Path], who)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/weapi/user/playlist":
			// 私有歌单只有本人带 cookie 才看得到。
			list := `{"id":1,"name":"公开","creator":{"userId":7}}`
			if who == "A" {
				list += `,{"id":2,"name":"A 的私有歌单","creator":{"userId":7}}`
			}
			_, _ = w.Write([]byte(`{"code":200,"more":false,"playlist":[` + list + `]}`))
		case "/weapi/v6/playlist/detail":
			_, _ = w.Write([]byte(`{"code":200,"playlist":{"id":2,"name":"` + who + ` 看到的歌单","trackCount":1,"tracks":[],"trackIds":[{"id":11}]}}`))
		case "/weapi/v3/song/detail":
			_, _ = w.Write([]byte(`{"code":200,"songs":[{"id":11,"name":"歌","ar":[],"al":{},"dt":1000}]}`))
		default:
			t.Errorf("unexpected upstream path %s", r.URL.Path)
		}
	}))
	t.Cleanup(up.Close)

	eng := engine.New(engine.WithCache(&memCache{m: map[string][]byte{}}), engine.WithBaseURL(up.URL))
	app, err := NewApp(Options{GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0", AuthTokens: single(testToken)}, eng, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = app.Shutdown(ctx)
	})

	get := func(path, cookie string) map[string]any {
		t.Helper()
		req, err := http.NewRequestWithContext(bg(t), http.MethodGet, "http://"+app.HTTPAddr()+path, nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+testToken)
		if cookie != "" {
			req.Header.Set("Grpc-Metadata-X-Netease-Cookie", cookie)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		var out map[string]any
		require.NoError(t, json.Unmarshal(body, &out))
		return out
	}

	a := get("/api/v1/users/7/playlists", "MUSIC_U=ACCOUNT_A")
	require.EqualValues(t, 2, a["total"], "带本人 cookie 时看得到私有歌单")
	anon := get("/api/v1/users/7/playlists", "")
	require.EqualValues(t, 1, anon["total"], "匿名只看到公开歌单")

	tracks := get("/api/v1/playlists/2/tracks", "MUSIC_U=ACCOUNT_A")
	require.EqualValues(t, 1, tracks["total"])

	// 歌单详情交错读取:A、B、匿名、再 A(第二次 A 命中 A 自己的缓存)。
	name := func(cookie string) string {
		pl, _ := get("/api/v1/playlists/2", cookie)["playlist"].(map[string]any)
		return pl["name"].(string)
	}
	require.Equal(t, "A 看到的歌单", name("MUSIC_U=ACCOUNT_A"))
	require.Equal(t, "B 看到的歌单", name("MUSIC_U=ACCOUNT_B"))
	require.Equal(t, "anon 看到的歌单", name(""))
	require.Equal(t, "A 看到的歌单", name("MUSIC_U=ACCOUNT_A"))

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"A", "anon"}, seen["/weapi/user/playlist"])
	require.Equal(t, []string{"A"}, seen["/weapi/v3/song/detail"])
	// 详情:AllTracks 的 A 一次,随后 A / B / 匿名各一次;最后一次 A 命中缓存,不再请求上游。
	require.Equal(t, []string{"A", "A", "B", "anon"}, seen["/weapi/v6/playlist/detail"])
}
