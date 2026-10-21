package public

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"time"

	merrors "github.com/VOD-Studio/kite/errors"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// protocolVersion 是 /player/v1 协议版本,经 /player/v1/info 返回。
const protocolVersion = "1"

// Server 是公开模式的 HTTP 服务。
type Server struct {
	opts   Options
	cat    catalog
	signer signer
	limit  *limiter
	owner  *ownerSession
	auth   func(ctx context.Context) error // 服务账号登录态续期,nil 表示不续期(测试)
	now    func() time.Time

	allowPlaylists map[int64]bool
	allowSongs     map[int64]bool

	http   *http.Server
	lis    net.Listener
	cancel context.CancelFunc
}

// New 创建公开模式服务。eng 与 gRPC 服务共用同一个引擎(熔断、重试状态一致)。
func New(eng *engine.Engine, opts Options) *Server {
	s := newServer(engineCatalog{eng: eng}, opts, time.Now)
	s.auth = func(ctx context.Context) error {
		s.owner.runRefresh(ctx, eng)
		return nil
	}
	return s
}

func newServer(cat catalog, opts Options, now func() time.Time) *Server {
	s := &Server{
		opts:           opts,
		cat:            newCachedCatalog(cat, now),
		signer:         signer{key: opts.SigningKey, ttl: opts.LinkTTL, now: now},
		limit:          newLimiter(opts.RatePerMinute, now),
		owner:          newOwnerSession(opts.SessionFile, now),
		now:            now,
		allowPlaylists: map[int64]bool{},
		allowSongs:     map[int64]bool{},
	}
	for _, id := range opts.Playlists {
		s.allowPlaylists[id] = true
	}
	for _, id := range opts.Songs {
		s.allowSongs[id] = true
	}
	return s
}

// Handler 返回完整的 HTTP 处理链:跨域 → 限流 → 路由。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "SERVING"})
	})
	mux.HandleFunc("GET /player/v1/info", s.handleInfo)
	mux.HandleFunc("GET /player/v1/playlists/{id}", s.handlePlaylist)
	mux.HandleFunc("GET /player/v1/tracks/{id}", s.handleTrack)
	mux.HandleFunc("GET /player/v1/tracks/{id}/lyrics", s.handleLyrics)
	mux.HandleFunc("GET /player/v1/tracks/{id}/stream", s.handleStream)
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "没有这个接口")
	})
	return s.cors(s.rateLimit(mux))
}

// Start 开始监听;登录态续期在后台运行,直到 Shutdown。
func (s *Server) Start() error {
	lis, err := net.Listen("tcp", s.opts.Addr)
	if err != nil {
		return fmt.Errorf("listen public %s: %w", s.opts.Addr, err)
	}
	s.lis = lis
	s.http = &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	if s.auth != nil {
		go func() { _ = s.auth(ctx) }()
	}
	go func() {
		slog.Info("public server starting",
			slog.String("addr", lis.Addr().String()),
			slog.Int("playlists", len(s.opts.Playlists)), slog.Int("songs", len(s.opts.Songs)),
			slog.String("owner_login", s.owner.state()))
		if err := s.http.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("public server failed", slog.String("error", err.Error()))
		}
	}()
	return nil
}

// Addr 返回实际监听地址(Addr 写 :0 时用于测试)。
func (s *Server) Addr() string {
	if s.lis == nil {
		return s.opts.Addr
	}
	return s.lis.Addr().String()
}

// Shutdown 停止续期并排空连接。
func (s *Server) Shutdown(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}
	if s.http == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}

func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	quality := "exhigh"
	for name, lv := range levels {
		if lv == s.opts.Level {
			quality = name
		}
	}
	writeJSON(w, http.StatusOK, infoDTO{Version: protocolVersion, Login: s.owner.state(), Quality: quality})
}

func (s *Server) handlePlaylist(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if !s.allowPlaylists[id] {
		writeError(w, http.StatusNotFound, "NOT_ALLOWED", "这个歌单没有在站点白名单里")
		return
	}
	pl, err := s.cat.Playlist(r.Context(), id)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	active := s.owner.state() == loginActive
	out := playlistDTO{
		ID:         strconv.FormatInt(pl.GetId(), 10),
		Name:       pl.GetName(),
		Cover:      https(pl.GetCoverUrl()),
		TrackCount: pl.GetTrackCount(),
		Tracks:     []trackDTO{},
	}
	for _, song := range pl.GetSongs() {
		out.Tracks = append(out.Tracks, s.track(song, active))
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleTrack(w http.ResponseWriter, r *http.Request) {
	id, ok := s.allowedTrack(w, r)
	if !ok {
		return
	}
	song, err := s.cat.Song(r.Context(), id)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, http.StatusOK, s.track(song, s.owner.state() == loginActive))
}

func (s *Server) handleLyrics(w http.ResponseWriter, r *http.Request) {
	id, ok := s.allowedTrack(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	lyric, err := s.cat.Lyric(ctx, id)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	// 逐字歌词与副歌时间是锦上添花:取不到时退回普通歌词,不让整首歌没有歌词。
	word, err := s.cat.WordLyric(ctx, id)
	if err != nil {
		slog.Debug("逐字歌词不可用", slog.Int64("id", id), slog.String("error", err.Error()))
	}
	chorus, err := s.cat.Chorus(ctx, id)
	if err != nil {
		slog.Debug("副歌时间不可用", slog.Int64("id", id), slog.String("error", err.Error()))
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
	writeJSON(w, http.StatusOK, buildLyrics(lyric, word, chorus))
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	switch err := s.signer.verify(id, q.Get("exp"), q.Get("sig")); {
	case errors.Is(err, errLinkExpired):
		writeError(w, http.StatusForbidden, "LINK_EXPIRED", "播放链接已过期,请重新获取歌单或歌曲")
		return
	case err != nil:
		writeError(w, http.StatusForbidden, "LINK_INVALID", "播放链接签名无效")
		return
	}
	cookie := s.owner.cookie()
	url, err := s.cat.StreamURL(r.Context(), id, s.opts.Level, cookie)
	if err != nil && !errors.Is(err, merrors.ErrNotFound) {
		writeUpstreamError(w, err)
		return
	}
	if url == "" {
		msg := "这首歌暂时无法播放(版权限制或需要 VIP)"
		if cookie == "" {
			msg = "这首是 VIP 歌曲,当前服务没有登录 VIP 账号"
		}
		writeError(w, http.StatusNotFound, "UNPLAYABLE", msg)
		return
	}
	target, ok := streamTarget(url)
	if !ok {
		slog.Warn("上游返回了不在网易云域名下的播放链接,拒绝跳转", slog.Int64("id", id))
		writeError(w, http.StatusBadGateway, "UPSTREAM_ERROR", "上游返回的播放链接无效")
		return
	}
	// 直链几十分钟后过期:只让浏览器短暂复用,过期后播放器会重新请求这里。
	w.Header().Set("Cache-Control", "private, max-age=60")
	http.Redirect(w, r, target, http.StatusFound) //nolint:gosec // 目标来自上游并经 streamTarget 校验为网易云域名的 https 地址
}

// streamHosts 是允许跳转的上游直链域名后缀。
var streamHosts = []string{".126.net", ".163.com", ".netease.com"}

// streamTarget 把上游直链换成 https,并确认它在网易云的域名下;公开接口不做任意地址的跳转。
func streamTarget(raw string) (string, bool) {
	u, err := neturl.Parse(https(raw))
	if err != nil || u.Scheme != "https" || u.User != nil {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	for _, suffix := range streamHosts {
		if strings.HasSuffix(host, suffix) {
			return u.String(), true
		}
	}
	return "", false
}

// allowedTrack 解析路径里的歌曲 ID,并确认它在白名单里(单独列出,或属于某个白名单歌单)。
func (s *Server) allowedTrack(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, ok := pathID(w, r)
	if !ok {
		return 0, false
	}
	if s.allowSongs[id] {
		return id, true
	}
	for plID := range s.allowPlaylists {
		pl, err := s.cat.Playlist(r.Context(), plID)
		if err != nil {
			slog.Warn("读取白名单歌单失败", slog.Int64("playlist", plID), slog.String("error", err.Error()))
			continue
		}
		for _, song := range pl.GetSongs() {
			if song.GetId() == id {
				return id, true
			}
		}
	}
	writeError(w, http.StatusNotFound, "NOT_ALLOWED", "这首歌没有在站点白名单里")
	return 0, false
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "ID 必须是正整数")
		return 0, false
	}
	return id, true
}

// cors 只允许白名单站点跨域读取接口。带 Origin 但不在白名单的请求直接拒绝,
// 防止别的网页盗用本站额度;<audio> 加载播放链接不带 Origin,不受影响。
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if !s.originAllowed(origin) {
				writeError(w, http.StatusForbidden, "ORIGIN_NOT_ALLOWED", "这个站点没有在 KITE_PUBLIC_ORIGINS 里")
				return
			}
			h := w.Header()
			if s.opts.AnyOrigin {
				h.Set("Access-Control-Allow-Origin", "*")
			} else {
				h.Set("Access-Control-Allow-Origin", origin)
				h.Add("Vary", "Origin")
			}
			if r.Method == http.MethodOptions {
				h.Set("Access-Control-Allow-Methods", "GET, OPTIONS")
				h.Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) originAllowed(origin string) bool {
	if s.opts.AnyOrigin {
		return true
	}
	for _, o := range s.opts.Origins {
		if strings.EqualFold(o, origin) {
			return true
		}
	}
	return false
}

func (s *Server) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" && !s.limit.allow(s.clientIP(r)) {
			w.Header().Set("Retry-After", "10")
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "请求太频繁,稍后再试")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) clientIP(r *http.Request) string {
	if s.opts.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, reason, msg string) {
	writeJSON(w, status, errorDTO{Error: errorBody{Reason: reason, Message: msg}})
}

func writeUpstreamError(w http.ResponseWriter, err error) {
	if errors.Is(err, merrors.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "上游没有这个资源")
		return
	}
	slog.Warn("public upstream error", slog.String("error", err.Error()))
	writeError(w, http.StatusBadGateway, "UPSTREAM_ERROR", "上游暂时不可用,稍后再试")
}
