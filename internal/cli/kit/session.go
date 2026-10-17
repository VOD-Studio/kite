package kit

import "github.com/VOD-Studio/kite/internal/localstate"

// 会话文件的读写在 localstate(与 CLI 无关,TUI 复用);这里只做 Kit 的薄委托,
// 把一次性迁移提示导向 stderr。

// CurrentCookie 返回当前生效的 cookie: 环境变量 NETEASE_COOKIE 优先,其次本地会话文件。
func (k *Kit) CurrentCookie() string { return localstate.CurrentCookie(k.err()) }

// SaveSession 把会话写盘。
func (k *Kit) SaveSession(sess localstate.Session) error { return localstate.SaveSession(sess) }

// LoadSession 读本地会话;首次读取时可能触发旧路径迁移(提示写 stderr)。
func (k *Kit) LoadSession() (localstate.Session, error) { return localstate.LoadSession(k.err()) }

// ClearSession 删除本地会话文件,不存在时不算错误。
func (k *Kit) ClearSession() error { return localstate.ClearSession() }
