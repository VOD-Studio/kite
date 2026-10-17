// Package engine 的熔断器。
//
// 连续失败达阈值后短路，避免雪崩。熔断打开时直接返回 ErrCircuitOpen。
// 新架构下熔断器保护 engine.RawDo 这一个方法，不需要装饰每个能力子接口。
package engine

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	merrors "github.com/VOD-Studio/kite/errors"
)

// ErrCircuitOpen 是熔断器打开时的错误。
var ErrCircuitOpen = errors.New("熔断器打开")

// errNotSent 标记「请求在发出前就失败了」的本地错误(凭据缺失、取 session 失败等)。
// 它与其它错误一起包装(%w + %w),调用方仍可用 errors.Is 判断原始错误类别
// (如 ErrUnauthorized),但熔断器据此忽略——没有真实的上游调用,既不说明健康也不说明故障。
var errNotSent = errors.New("请求未发出")

// notSent 把 err 标记为未发出请求的本地错误。
func notSent(err error) error {
	return fmt.Errorf("%w: %w", errNotSent, err)
}

// circuitState 是熔断器状态。
type circuitState int

const (
	stateClosed   circuitState = iota // 关闭（正常放行）
	stateOpen                         // 打开（短路，直接返回错误）
	stateHalfOpen                     // 半开（放行一次探测）
)

// circuitBreaker 是熔断器。
//
// 连续 failures 次失败后进入 Open，经过 openTimeout 后进入 HalfOpen 放行一次探测。
// 探测成功则 Closed，探测失败则重新 Open。线程安全。
type circuitBreaker struct {
	mu          sync.Mutex
	state       circuitState
	failures    int
	threshold   int
	openTimeout time.Duration
	lastFailure time.Time
}

// newCircuitBreaker 创建熔断器。
//
// threshold 是连续失败多少次后打开熔断，openTimeout 是熔断持续时间。
func newCircuitBreaker(threshold int, openTimeout time.Duration) *circuitBreaker {
	return &circuitBreaker{
		state:       stateClosed,
		threshold:   threshold,
		openTimeout: openTimeout,
	}
}

// allow 判断是否放行（调用前检查）。
//
// 返回 true 表示放行（Closed 或 HalfOpen），false 表示短路（Open）。
// HalfOpen 时只放行一次。
func (cb *circuitBreaker) allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case stateClosed:
		return true
	case stateOpen:
		if time.Since(cb.lastFailure) >= cb.openTimeout {
			cb.state = stateHalfOpen
			return true
		}
		return false
	case stateHalfOpen:
		return true
	default:
		return true
	}
}

// recordSuccess 记录一次成功调用。
func (cb *circuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	cb.state = stateClosed
}

// recordFailure 记录一次失败调用。
func (cb *circuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	cb.lastFailure = time.Now()
	if cb.state == stateHalfOpen || cb.failures >= cb.threshold {
		cb.state = stateOpen
	}
}

// state 返回当前熔断器状态（测试 / 指标用）。
func (cb *circuitBreaker) State() circuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// recordResult 把一次调用结果记入熔断器。
//
// 熔断器衡量的是「上游是否健康」,所以只有上游不可用 / 被限流才算失败。未登录、资源不存在、
// 响应格式无效说明上游正常应答了,按成功处理——否则连续几次「未登录」就会让长驻进程(server /
// TUI)里全部接口被短路。调用方主动取消、本地序列化 / 加密失败从未到达上游,既不说明健康也不说明
// 故障,忽略。
//
// 取消通过 ctx.Err() 判断:transport 用 %v 包装底层错误,errors.Is(err, context.Canceled) 已失效。
func (cb *circuitBreaker) recordResult(ctx context.Context, err error) {
	switch {
	case err == nil:
		cb.recordSuccess()
	case errors.Is(err, errNotSent):
		// 请求未发出(本地凭据缺失等):没有真实探测,不计——否则无凭据的请求会清零
		// 已累计的上游失败,或在半开状态下无探测地关闭熔断器。
	case errors.Is(err, context.Canceled) || ctx.Err() != nil:
		// 调用方取消:不计。
	case isRetryable(err):
		cb.recordFailure()
	case errors.Is(err, merrors.ErrUnauthorized),
		errors.Is(err, merrors.ErrNotFound),
		errors.Is(err, merrors.ErrInvalidResponse):
		cb.recordSuccess() // 上游正常应答的业务错误
	default:
		// 本地错误(序列化 / 加密 / 构造请求):不计。
	}
}
