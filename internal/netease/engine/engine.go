// Package engine 是网易云共享执行引擎，是 kite 的核心深模块。
//
// Engine 持有 transport（HTTP）、SessionStore（cookie 池）、Cache（缓存）、
// retryPolicy、circuitBreaker。所有入口(Execute / ExecuteWithCookie /
// ExecuteWithSetCookie / RawDoWithCookieAndInput)背后是同一条管线 do：
// 熔断检查 → 重试 { 凭据解析 → 加密 → HTTP 请求 → 错误映射 }。
// 入口之间只在「凭据从哪来」上不同(credentialPolicy),其余只写一份。
//
// deletion test：删掉这条管线，这些复杂度会散落到 357 个调用点。
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	merrors "github.com/VOD-Studio/kite/errors"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// CryptoMethod 标识网易云请求的加密方式。
type CryptoMethod int

const (
	// CryptoNone 不加密（GET 请求用）。
	CryptoNone CryptoMethod = iota
	// CryptoWeAPI 用 weapi 协议加密（大部分 POST 接口）。
	CryptoWeAPI
	// CryptoEAPI 用 eapi 协议加密。
	CryptoEAPI
	// CryptoLinuxAPI 用 linuxapi 协议加密。
	CryptoLinuxAPI
)

// Meta 是网易云 endpoint 的执行元数据。
type Meta struct {
	// Path 是网易云端点路径（如 /weapi/song/enhance/player/url/v1）。
	Path string
	// Method 是 HTTP 方法。
	Method string
	// Crypto 是加密方式。
	Crypto CryptoMethod
	// Auth 是登录态需求，驱动 cookie 池选取。
	Auth session.AuthRequirement
	// UserAgent 覆盖请求 UA（空 = 默认桌面客户端 UA）。
	// 网易云按 UA 分配 CDN 节点：song url 接口桌面 UA 拿到的 x04 节点链接
	// 回源鉴权 403，移动端 UA 的 x01 节点正常——需移动 UA 的 endpoint 用
	// engine.MobileUserAgent。
	UserAgent string
}

// Engine 是网易云共享执行引擎。
type Engine struct {
	transport *transport
	sessions  session.SessionStore
	cache     cacheClient
	retry     retryPolicy
	breaker   *circuitBreaker
}

// cacheClient 是 Engine 对缓存的依赖接口（internal/cache.Cache 的别名契约）。
type cacheClient interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

// Option 是 Engine 的配置项。
type Option func(*Engine)

// WithTimeout 设置 HTTP 超时。
func WithTimeout(d time.Duration) Option {
	return func(e *Engine) { e.transport = newTransport(d) }
}

// WithBaseURL 覆盖网易云 API 基础地址（测试用，指向 httptest server）。
func WithBaseURL(base string) Option {
	return func(e *Engine) { e.transport = e.transport.withBaseURL(base) }
}

// WithProxyURL 让全部 API 请求经指定代理:显式设置整体替换环境变量层
// (ProxyFromEnvironment 不再参与,无混合态);u 为 nil 时不注入,保留默认
// (环境变量 HTTPS_PROXY/HTTP_PROXY 照常生效)。flag 与 config 解析出同一
// URL 后注入——决策单一来源(PRD-0018)。
func WithProxyURL(u *url.URL) Option {
	return func(e *Engine) { e.transport = e.transport.withProxy(u) }
}

// Apply 装配期应用/覆写 option(root 在 flag 解析后才拿到 --proxy 值,
// 经此注入;Option 语义与 New 一致)。
func (e *Engine) Apply(opts ...Option) {
	for _, opt := range opts {
		opt(e)
	}
}

// WithSessions 注入 SessionStore。
func WithSessions(s session.SessionStore) Option {
	return func(e *Engine) { e.sessions = s }
}

// WithCache 注入 Cache。
func WithCache(c cacheClient) Option {
	return func(e *Engine) { e.cache = c }
}

// New 创建 Engine。
func New(opts ...Option) *Engine {
	e := &Engine{
		transport: newTransport(10 * time.Second),
		retry:     defaultRetryPolicy,
		breaker:   newCircuitBreaker(5, 30*time.Second),
	}
	e.Apply(opts...)
	return e
}

// credentialPolicy 决定一次调用的 cookie 从哪来——两条执行路径唯一刻意不同的地方。
// 加密分发、熔断、重试、错误映射对两者完全相同,只在 do 里写一份。
type credentialPolicy int

const (
	// credFromPool 是会话池路径(Execute):
	// AuthLoggedIn 的接口 ctx 里的 cookie 优先,否则取会话池;AuthOptional 只带 ctx 里的 cookie,
	// 没有就匿名;匿名接口不携带任何用户 cookie。池里取出的 session 失效时上报。
	credFromPool credentialPolicy = iota
	// credFromContext 是 ctx cookie 路径(RawDoWithCookieAndInput / ExecuteWithCookie / ExecuteWithSetCookie):
	// 无条件使用 ctx 里的 cookie(哪怕是匿名接口),从不碰会话池,也不上报。
	// 写操作 / 登录类接口 / 本地持有 cookie 的调用方(CLI、gRPC cookie 拦截器路径)用它。
	credFromContext
)

// RawDoWithCookieAndInput 走 ctx cookie 路径执行一次原始调用(Meta + 网易云参数),
// 返回原始 JSON 与响应的 Set-Cookie,不经 endpoint 声明(MapRequest / MapResponse)。
//
// 登录类接口已改为声明式 endpoint(engine.ExecuteWithSetCookie);本入口只作为
// engine 特征化测试的原始管线 seam 保留,生产代码不得新增调用(由 archtest 守护)。
func (e *Engine) RawDoWithCookieAndInput(ctx context.Context, meta Meta, params map[string]any) (json.RawMessage, string, error) {
	return e.do(ctx, meta, params, credFromContext)
}

// do 是 engine 唯一的执行管线:熔断检查 → 重试 { 凭据解析 → 加密分发 → 请求 → 错误映射 }。
// 所有 RawDo* / Execute* 入口都经过它,策略只决定凭据来源。
func (e *Engine) do(ctx context.Context, meta Meta, params map[string]any, policy credentialPolicy) (json.RawMessage, string, error) {
	// 熔断检查。
	if !e.breaker.allow() {
		return nil, "", ErrCircuitOpen
	}

	// 用闭包局部变量把 doOnce 的结果传出来(withRetry 回调签名是 func() error)。
	var (
		raw       json.RawMessage
		setCookie string
	)
	err := withRetry(ctx, e.retry, func() error {
		r, sc, callErr := e.doOnce(ctx, meta, params, policy)
		e.breaker.recordResult(ctx, callErr)
		if callErr != nil {
			return callErr
		}
		raw = r
		setCookie = sc
		return nil
	})

	if err != nil {
		return nil, "", err
	}
	return raw, setCookie, nil
}

// doOnce 执行一次完整调用(不含重试 / 熔断):凭据解析 → 序列化 → 加密分发 → 错误映射。
// 返回 raw JSON + Set-Cookie。
func (e *Engine) doOnce(ctx context.Context, meta Meta, params map[string]any, policy credentialPolicy) (json.RawMessage, string, error) {
	cookie, pooled, err := e.resolveCredential(ctx, meta, policy)
	if err != nil {
		return nil, "", err
	}

	// 参数序列化为 JSON。
	payload, err := json.Marshal(params)
	if err != nil {
		return nil, "", fmt.Errorf("序列化参数失败: %w", err)
	}

	body, setCookie, callErr := e.send(ctx, meta, params, string(payload), cookie)
	if callErr != nil {
		// 池里取出的 cookie 失效时上报(只上报本次实际用的那个 session)。
		if pooled != nil && isUnauthorized(callErr) {
			e.sessions.ReportFailure(pooled.UserID, callErr)
		}
		return nil, "", callErr
	}
	return json.RawMessage(body), setCookie, nil
}

// resolveCredential 按策略解析本次调用的 cookie。pooled 非 nil 表示 cookie 来自会话池,
// 失效时需要上报。没有可用凭据时返回 notSent 错误(请求根本没发出,不计入熔断)。
func (e *Engine) resolveCredential(ctx context.Context, meta Meta, policy credentialPolicy) (cookie string, pooled *session.Session, err error) {
	if policy == credFromContext {
		return CookieFromContext(ctx), nil, nil
	}

	// 会话池路径:匿名接口不携带任何用户 cookie;可选登录的接口只认 ctx cookie,没有就匿名。
	switch meta.Auth {
	case session.AuthLoggedIn:
		// 往下走:ctx cookie 优先,否则取会话池。
	case session.AuthOptional:
		return CookieFromContext(ctx), nil, nil
	default:
		return "", nil, nil
	}
	// 需要登录态的接口:调用方经 ctx 自带的 cookie(gRPC 拦截器注入)优先,否则从 session 池选取。
	if c := CookieFromContext(ctx); c != "" {
		return c, nil, nil
	}
	if e.sessions == nil {
		return "", nil, notSent(merrors.ErrUnauthorized)
	}
	sess, err := e.sessions.GetAvailable(ctx, meta.Auth)
	if err != nil {
		return "", nil, notSent(err)
	}
	return sess.Cookie, sess, nil
}

// send 按 meta.Crypto 分发到对应的加密 + 传输实现,返回响应体与 Set-Cookie。
// 只有 weapi 回传 Set-Cookie(登录类接口靠它拿 session);eapi 与无加密请求不回传。
func (e *Engine) send(ctx context.Context, meta Meta, params map[string]any, payload, cookie string) (body []byte, setCookie string, err error) {
	switch meta.Crypto {
	case CryptoEAPI:
		encrypted, encErr := EAPIEncrypt(meta.Path, payload)
		if encErr != nil {
			return nil, "", fmt.Errorf("eapi 加密失败: %w", encErr)
		}
		body, _, err = e.transport.eapiPost(ctx, meta.Path, encrypted.Params, cookie, meta.UserAgent)
	case CryptoNone:
		if meta.Method == "GET" {
			body, err = e.transport.apiGet(ctx, meta.Path, toQueryValues(params), cookie, meta.UserAgent)
		} else {
			body, _, err = e.transport.postJSON(ctx, e.transport.baseURL+meta.Path, payload, cookie, meta.UserAgent)
		}
	default: // CryptoWeAPI,以及未知加密方式
		body, setCookie, err = e.transport.weapiPost(ctx, meta.Path, payload, cookie, meta.UserAgent)
	}
	return body, setCookie, err
}

// isUnauthorized 判断错误是否为登录态失效。
func isUnauthorized(err error) bool {
	return errors.Is(err, merrors.ErrUnauthorized)
}

// toQueryValues 把 params map 转成 url.Values（GET 请求用）。
func toQueryValues(params map[string]any) url.Values {
	q := url.Values{}
	for k, v := range params {
		q.Set(k, fmt.Sprintf("%v", v))
	}
	return q
}
