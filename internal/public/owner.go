package public

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	merrors "github.com/VOD-Studio/kite/errors"
	authendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/auth"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// SessionFile 是服务账号网易云登录态文件的内容(server public-login 写入,运行中续期时改写)。
type SessionFile struct {
	Cookie    string    `json:"cookie"`
	UserID    int64     `json:"userId"`
	Nickname  string    `json:"nickname"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// 服务账号登录态的三种状态,经 /player/v1/info 告诉播放器。
const (
	loginNone    = "none"
	loginActive  = "active"
	loginExpired = "expired"
)

// refreshInterval 是续期间隔。网易云 cookie 有效期以月计,每天续一次足够,也不显得异常频繁。
const refreshInterval = 24 * time.Hour

// statInterval 是检查登录态文件是否被改写的最短间隔(public-login 在另一个进程里写文件)。
const statInterval = 30 * time.Second

// ownerSession 持有可选的服务账号登录态。所有听众的 VIP 播放都用这一个账号,
// 所以只用于给白名单内容生成播放链接,不用于任何别的接口。
type ownerSession struct {
	path string
	now  func() time.Time

	mu       sync.Mutex
	file     SessionFile
	status   string
	modTime  time.Time
	lastStat time.Time
}

func newOwnerSession(path string, now func() time.Time) *ownerSession {
	o := &ownerSession{path: path, now: now, status: loginNone}
	if path != "" {
		o.reload(true)
	}
	return o
}

// cookie 返回当前可用的 cookie;未配置或已失效时返回空串(按匿名播放)。
func (o *ownerSession) cookie() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.reloadLocked(false)
	if o.status != loginActive {
		return ""
	}
	return o.file.Cookie
}

func (o *ownerSession) state() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.reloadLocked(false)
	return o.status
}

func (o *ownerSession) reload(force bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.reloadLocked(force)
}

// reloadLocked 在文件被改写后重新读取(例如部署者重新扫码)。force 为 false 时按 statInterval 节流。
func (o *ownerSession) reloadLocked(force bool) {
	if o.path == "" {
		return
	}
	now := o.now()
	if !force && now.Sub(o.lastStat) < statInterval {
		return
	}
	o.lastStat = now
	info, err := os.Stat(o.path)
	if err != nil {
		if o.status != loginNone {
			slog.Warn("服务账号登录态文件不可读,改为匿名播放", slog.String("path", o.path), slog.String("error", err.Error()))
		}
		o.file, o.status = SessionFile{}, loginNone
		return
	}
	if !force && info.ModTime().Equal(o.modTime) {
		return
	}
	f, err := ReadSessionFile(o.path)
	if err != nil || f.Cookie == "" {
		slog.Warn("服务账号登录态文件无效,改为匿名播放", slog.String("path", o.path))
		o.file, o.status = SessionFile{}, loginNone
		return
	}
	o.file, o.status, o.modTime = f, loginActive, info.ModTime()
}

// refresh 续期一次:成功时把上游下发的新 cookie 写回文件;cookie 失效时标记为 expired。
func (o *ownerSession) refresh(ctx context.Context, eng *engine.Engine) error {
	o.mu.Lock()
	o.reloadLocked(true)
	f, status := o.file, o.status
	o.mu.Unlock()
	if status == loginNone {
		return nil
	}

	cookie, renewed, err := authendpoint.Refresh(engine.WithCookie(ctx, f.Cookie), eng)
	if err != nil {
		if errors.Is(err, merrors.ErrUnauthorized) {
			o.mu.Lock()
			o.status = loginExpired
			o.mu.Unlock()
			slog.Warn("服务账号的网易云登录态已失效,VIP 歌曲暂停播放;请重新执行 server public-login")
		}
		return err
	}
	if !renewed {
		return nil
	}
	f.Cookie, f.UpdatedAt = cookie, o.now()
	if err := WriteSessionFile(o.path, f); err != nil {
		return err
	}
	o.reload(true)
	return nil
}

// runRefresh 在后台定期续期,直到 ctx 结束。启动时若距上次续期已超过间隔,先续一次。
func (o *ownerSession) runRefresh(ctx context.Context, eng *engine.Engine) {
	if o.path == "" {
		return
	}
	o.mu.Lock()
	stale := o.status == loginActive && o.now().Sub(o.file.UpdatedAt) >= refreshInterval
	o.mu.Unlock()
	if stale {
		o.logRefresh(o.refresh(ctx, eng))
	}
	t := time.NewTicker(refreshInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			o.logRefresh(o.refresh(ctx, eng))
		}
	}
}

func (o *ownerSession) logRefresh(err error) {
	if err != nil && !errors.Is(err, merrors.ErrUnauthorized) {
		slog.Warn("服务账号登录态续期失败,稍后重试", slog.String("error", err.Error()))
	}
}

// ReadSessionFile 读取登录态文件。
func ReadSessionFile(path string) (SessionFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return SessionFile{}, err
	}
	var f SessionFile
	if err := json.Unmarshal(b, &f); err != nil {
		return SessionFile{}, fmt.Errorf("解析登录态文件: %w", err)
	}
	return f, nil
}

// WriteSessionFile 原子地写入登录态文件(先写临时文件再改名),权限 0600。
func WriteSessionFile(path string, f SessionFile) error {
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("创建登录态目录: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".kite-session-*")
	if err != nil {
		return fmt.Errorf("写登录态文件: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
