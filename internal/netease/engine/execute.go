// Package engine 的 Execute 泛型函数与 Endpoint 声明类型。
//
// Execute 串起缓存检查 → MapRequest → RawDo → MapResponse → 缓存回填。
// 缓存命中时跳过 RawDo，但 gRPC 链上的 auth/rate/trace/recovery 照常执行
// （它们在 interceptor，Execute 在 service 方法里被调用，时序在拦截器之后）。
//
// 类型参数设计：Req/Resp 都用 proto 生成的指针类型（如 *mmpb.GetSongDetailRequest）。
// 这是 gRPC 的天然签名，也避免 proto message 值拷贝 mutex 的问题（protoimpl.MessageState 含锁）。
// 缓存序列化/反序列化用 proto.Message 接口（指针天然实现），零 reflection。
package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	merrors "github.com/VOD-Studio/kite/errors"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// Endpoint 是一个网易云接口的完整声明。
//
// 数据（Meta + Cache + NewResp）+ 两个映射函数（MapRequest + MapResponse）。
// 不是活跃服务，是声明。每接口一个包级 var。
//
// Req/Resp 用 proto 生成的指针类型（如 *mmpb.GetSongDetailRequest）。
type Endpoint[Req, Resp any] struct {
	// Meta 是网易云 endpoint 的执行元数据。
	Meta Meta
	// PathFunc 可选:按请求动态生成 path(覆盖 Meta.Path)。
	// 部分网易云接口把 id 拼在 path 里(/weapi/artist/albums/{id}),
	// body 传 id 的形式上游 400——此类接口声明 PathFunc。
	PathFunc func(Req) string
	// Cache 是缓存策略。nil 表示不缓存。
	Cache *CachePolicy[Req]
	// NewResp 构造响应实例，用于缓存命中时反序列化。
	// 避免用 reflect 构造（ADR §4.5 零 reflection）。声明形如 func() *mmpb.X { return &mmpb.X{} }。
	NewResp func() Resp
	// MapRequest 把 proto 请求转成网易云加密前的 params map。
	MapRequest func(req Req) (map[string]any, error)
	// MapResponse 把网易云原始 JSON 转成 proto 响应（指针类型）。
	// 接收原始请求，使映射可按请求字段分支（如搜索按 type 分发、歌单按 filter 过滤）。
	MapResponse func(req Req, raw json.RawMessage) (Resp, error)
}

// CachePolicy 声明缓存策略。endpoint 只声明，不执行（执行在 Execute）。
type CachePolicy[Req any] struct {
	// Key 从请求算 cache key。
	Key func(req Req) string
	// TTL 是缓存存活时间。
	TTL time.Duration
}

// Execute 串起缓存检查 → MapRequest → RawDo → MapResponse → 缓存回填。
//
// 缓存命中时跳过 RawDo。cache 在此处而非 interceptor 的理由：cache 是唯一的
// per-endpoint policy 驱动 + 类型相关序列化横切关注点，policy 和具体 Resp 类型都在手边，
// 无需注册表查 FullMethod→CachePolicy、无需 reflection 反序列化，
// 也消灭了「缓存命中漏鉴权」的拦截器顺序 footgun。
func Execute[Req, Resp any](e *Engine, ctx context.Context, ep *Endpoint[Req, Resp], req Req) (Resp, error) { //nolint:revive // Go 不支持泛型方法,Engine 只能作首参
	var zero Resp

	// 缓存 key 与可缓存性在调用前一次算定:需要登录态的接口按凭据隔离,池路径不缓存。
	var cacheKey string
	useCache := false
	if ep.Cache != nil && e.cache != nil {
		cacheKey, useCache = scopedCacheKey(ctx, ep.Meta.Auth, ep.Cache.Key(req))
	}

	// 1. 缓存命中直接返回。
	if useCache {
		key := cacheKey
		if hit, ok, err := e.cache.Get(ctx, key); err == nil && ok {
			if r, ok := unmarshalCached(ep.NewResp, hit); ok {
				return r, nil
			}
		}
	}

	// 2. 真实调用：MapRequest → do → MapResponse（与 ExecuteWithCookie 共用 callEndpoint）。
	resp, _, err := callEndpoint(e, ctx, ep, req, credFromPool)
	if err != nil {
		return zero, err
	}

	// 3. 回填缓存。
	if useCache {
		if pm, ok := any(resp).(proto.Message); ok {
			if data, err := proto.Marshal(pm); err == nil {
				_ = e.cache.Set(ctx, cacheKey, data, ep.Cache.TTL)
			}
		}
	}

	return resp, nil
}

// unmarshalCached 用 NewResp 工厂构造实例，把缓存字节反序列化成 Resp。
//
// 零 reflection：NewResp 是 endpoint 声明的构造函数（如 func() *mmpb.X { return &mmpb.X{} }），
// 返回值天然实现 proto.Message，直接 proto.Unmarshal 填充。
func unmarshalCached[Resp any](newResp func() Resp, data []byte) (Resp, bool) {
	var zero Resp
	if newResp == nil {
		return zero, false
	}
	r := newResp()
	pm, ok := any(r).(proto.Message)
	if !ok {
		return zero, false
	}
	if err := proto.Unmarshal(data, pm); err != nil {
		return zero, false
	}
	return r, true
}

// ExecuteWithCookie 走 cookie override 路径执行 endpoint:cookie 从 ctx 取
// (engine.WithCookie 注入),不经 session 池,不读写缓存。
//
// 写操作 / 即时数据 / 本地持有 cookie 的调用方(CLI、gRPC cookie 拦截器路径)共用
// 本函数,与 Execute 同样应用 PathFunc——执行语义只在 engine 一处定义,调用方不得再拷贝。
func ExecuteWithCookie[Req, Resp any](e *Engine, ctx context.Context, ep *Endpoint[Req, Resp], req Req) (Resp, error) { //nolint:revive // 与 Execute 保持同一参数顺序
	resp, _, err := ExecuteWithSetCookie(e, ctx, ep, req)
	return resp, err
}

// ExecuteWithSetCookie 同 ExecuteWithCookie,但额外返回响应的 Set-Cookie。
//
// 登录类接口(auth 域)靠它拿 session;只有 weapi 回传,与 RawDoWithCookieAndInput 一致。
func ExecuteWithSetCookie[Req, Resp any](e *Engine, ctx context.Context, ep *Endpoint[Req, Resp], req Req) (Resp, string, error) { //nolint:revive // 与 Execute 保持同一参数顺序
	return callEndpoint(e, ctx, ep, req, credFromContext)
}

// callEndpoint 是 Execute / ExecuteWithCookie / ExecuteWithSetCookie 共享的核心:
// MapRequest → PathFunc → 引擎管线 → MapResponse。各入口的区别只有凭据策略
// (及 Execute 外层的缓存),这里只写一份。
func callEndpoint[Req, Resp any](e *Engine, ctx context.Context, ep *Endpoint[Req, Resp], req Req, policy credentialPolicy) (Resp, string, error) { //nolint:revive // 与 Execute 保持同一参数顺序
	var zero Resp

	params, err := ep.MapRequest(req)
	if err != nil {
		return zero, "", fmt.Errorf("map request: %w", err)
	}

	meta := ep.Meta
	if ep.PathFunc != nil {
		meta.Path = ep.PathFunc(req)
	}

	raw, setCookie, err := e.do(ctx, meta, params, policy)
	if err != nil {
		return zero, "", err
	}

	resp, err := ep.MapResponse(req, raw)
	if err != nil {
		return zero, "", wrapMapResponseError(err)
	}
	return resp, setCookie, nil
}

// scopedCacheKey 返回实际使用的缓存 key 及该次调用是否可缓存。
//
//   - 匿名接口：key 原样，各调用方共享。
//   - 需要登录态的接口：响应属于某个用户，key 追加凭据指纹（cookie 的 SHA-256 前 8 字节），
//     不同用户互不命中；指纹而非原始 cookie，避免凭据进入 Redis key / 日志。
//   - 需要登录态但 ctx 没有 cookie（走 session 池）：身份在选 session 之前未知，
//     无法安全地区分用户，直接不缓存。
//   - 可选登录的接口：带 cookie 时同登录态（按凭据指纹隔离），不带时同匿名（共享）。
//
// 这样「接口作者忘了在 key 里带用户」不再可能造成跨用户串数据。
func scopedCacheKey(ctx context.Context, auth session.AuthRequirement, base string) (string, bool) {
	if auth != session.AuthLoggedIn && auth != session.AuthOptional {
		return base, true
	}
	cookie := CookieFromContext(ctx)
	if cookie == "" {
		return base, auth == session.AuthOptional
	}
	sum := sha256.Sum256([]byte(cookie))
	return base + "|u:" + hex.EncodeToString(sum[:8]), true
}

// wrapMapResponseError 包装 MapResponse 返回的错误并保证它带有分类。
//
//   - 已带语义哨兵（不存在 / 未登录 / 限流 / 上游不可用等）的错误保留原类别，
//     不能被误报为「响应格式无效」（例如「歌曲不存在」就是 ErrNotFound）。
//   - 没有明确类别的（JSON 解码失败、缺字段）说明上游响应形状不符预期，归为 ErrInvalidResponse，
//     对外映射为 UPSTREAM_INVALID_RESPONSE，客户端据此知道是上游接口可能变了而不是自己的问题。
func wrapMapResponseError(err error) error {
	for _, sentinel := range []error{
		merrors.ErrUnauthorized, merrors.ErrNotFound, merrors.ErrRateLimited,
		merrors.ErrUpstreamUnavailable, merrors.ErrInvalidResponse, merrors.ErrUnsupportedPlatform,
	} {
		if errors.Is(err, sentinel) {
			return fmt.Errorf("map response: %w", err)
		}
	}
	return fmt.Errorf("%w: map response: %w", merrors.ErrInvalidResponse, err)
}
