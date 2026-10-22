package public

import (
	"context"
	"sync"
	"time"

	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// ownerCredential 是一次播放请求选中的登录态快照,不会随续期或重新登录而改变。
type ownerCredential struct {
	cookie   string
	session  *ownerSession
	revision uint64
}

// expire 只失效本次请求用到的版本,不能影响后来续期或重新登录的会话。
func (c ownerCredential) expire() {
	if c.session != nil {
		c.session.expire(c.revision)
	}
}

// ownerPool 只包含部署者显式配置的账号,按配置顺序轮询可用登录态。
type ownerPool struct {
	mu       sync.Mutex
	sessions []*ownerSession
	cursor   int
}

func newOwnerPool(accounts []ServiceAccount, now func() time.Time) *ownerPool {
	p := &ownerPool{sessions: make([]*ownerSession, len(accounts))}
	for i, account := range accounts {
		p.sessions[i] = newOwnerSession(account.SessionFile, now)
	}
	return p
}

func (p *ownerPool) next() ownerCredential {
	p.mu.Lock()
	defer p.mu.Unlock()
	for offset := range len(p.sessions) {
		i := (p.cursor + offset) % len(p.sessions)
		credential := p.sessions[i].snapshot()
		if credential.cookie != "" {
			p.cursor = (i + 1) % len(p.sessions)
			return credential
		}
	}
	return ownerCredential{}
}

func (p *ownerPool) state() string {
	status := loginNone
	for _, session := range p.sessions {
		switch session.state() {
		case loginActive:
			return loginActive
		case loginExpired:
			status = loginExpired
		}
	}
	return status
}

// 每个账号独立续期,慢请求不会阻塞其他账号;退出前等待所有续期循环响应取消。
func (p *ownerPool) runRefresh(ctx context.Context, eng *engine.Engine) {
	var wg sync.WaitGroup
	wg.Add(len(p.sessions))
	for _, session := range p.sessions {
		go func() {
			defer wg.Done()
			session.runRefresh(ctx, eng)
		}()
	}
	wg.Wait()
}
