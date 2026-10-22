package public

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// ownerSession 独立维护一个服务账号的登录态,只用于给白名单内容生成播放链接。
type ownerSession struct {
	path string
	now  func() time.Time

	mu       sync.Mutex
	file     SessionFile
	status   string
	info     os.FileInfo
	revision uint64
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
	return o.snapshot().cookie
}

func (o *ownerSession) state() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.reloadLocked(false)
	return o.status
}

func (o *ownerSession) snapshot() ownerCredential {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.reloadLocked(false)
	if o.status != loginActive {
		return ownerCredential{}
	}
	return ownerCredential{cookie: o.file.Cookie, session: o, revision: o.revision}
}

func (o *ownerSession) expire(revision uint64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	// 播放或续期的响应可能晚于部署者重新登录,必须先检查磁盘上的新会话。
	o.reloadLocked(true)
	if o.revision != revision || o.status != loginActive {
		return
	}
	o.status = loginExpired
	o.revision++
	slog.Warn("服务账号的网易云登录态已失效,VIP 歌曲暂停播放;请重新执行 server public-login",
		slog.String("path", o.path))
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
		if o.info != nil || o.status != loginNone {
			o.revision++
		}
		o.file, o.status, o.info = SessionFile{}, loginNone, nil
		return
	}
	// force 只绕过检查间隔,不能把同一份已失效文件重新激活。
	// public-login 原子替换文件,即使 mtime 相同也要通过文件身份识别新登录。
	if o.info != nil && os.SameFile(info, o.info) &&
		info.ModTime().Equal(o.info.ModTime()) && info.Size() == o.info.Size() {
		return
	}
	f, info, err := readSessionFile(o.path)
	o.revision++
	o.info = info
	if err != nil || f.Cookie == "" {
		slog.Warn("服务账号登录态文件无效,改为匿名播放", slog.String("path", o.path))
		o.file, o.status = SessionFile{}, loginNone
		return
	}
	o.file, o.status = f, loginActive
}

// refresh 续期一次:成功时把上游下发的新 cookie 写回文件;cookie 失效时标记为 expired。
func (o *ownerSession) refresh(ctx context.Context, eng *engine.Engine) error {
	o.mu.Lock()
	o.reloadLocked(true)
	f, status, revision := o.file, o.status, o.revision
	o.mu.Unlock()
	if status != loginActive {
		return nil
	}

	cookie, renewed, err := authendpoint.Refresh(engine.WithCookie(ctx, f.Cookie), eng)
	if err != nil {
		if errors.Is(err, merrors.ErrUnauthorized) {
			o.expire(revision)
		}
		return err
	}
	if !renewed {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	// 网络请求期间可能发生重新登录、其他续期或失效,旧响应不能覆盖新会话。
	o.reloadLocked(true)
	if o.revision != revision || o.status != loginActive {
		return nil
	}
	f.Cookie, f.UpdatedAt = cookie, o.now()
	if err := WriteSessionFile(o.path, f); err != nil {
		return err
	}
	o.reloadLocked(true)
	return nil
}

// runRefresh 在后台定期续期,直到 ctx 结束。启动时若距上次续期已超过间隔,先续一次。
func (o *ownerSession) runRefresh(ctx context.Context, eng *engine.Engine) {
	if o.path == "" || ctx.Err() != nil {
		return
	}
	o.mu.Lock()
	o.reloadLocked(true)
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
		slog.Warn("服务账号登录态续期失败,稍后重试",
			slog.String("path", o.path), slog.String("error", err.Error()))
	}
}

// ReadSessionFile 读取登录态文件。
func ReadSessionFile(path string) (SessionFile, error) {
	f, _, err := readSessionFile(path)
	return f, err
}

// 从同一个打开的文件读取内容与身份,避免原子替换时把旧内容关联到新文件。
func readSessionFile(path string) (SessionFile, os.FileInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return SessionFile{}, nil, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return SessionFile{}, nil, err
	}
	b, err := io.ReadAll(file)
	if err != nil {
		return SessionFile{}, info, err
	}
	var f SessionFile
	if err := json.Unmarshal(b, &f); err != nil {
		return SessionFile{}, info, fmt.Errorf("解析登录态文件: %w", err)
	}
	return f, info, nil
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
