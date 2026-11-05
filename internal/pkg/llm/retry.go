package llm

import (
	// 标准库
	"context"
	"strings"
	"time"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// TimeoutRetryPolicy 是超时重试策略：仅用于非流式 Generate，遇到超时错误时按倍数放宽
// 单次请求截止时间后重试。零值（Enabled=false）表示不重试。
type TimeoutRetryPolicy struct {
	// Enabled 是否开启超时重试
	Enabled bool
	// MaxRetries 最大重试次数（不含首次请求）；Enabled 为 true 且该值大于 0 时生效
	MaxRetries int
	// Multiplier 每次重试的截止时间倍数；非正数按 2 处理
	Multiplier int
}

// maxTimeoutRetryDeadline 是单次重试允许的最大截止时间，避免倍数放大导致异常长的等待。
const maxTimeoutRetryDeadline = 24 * time.Hour

// retryOnTimeout 以 base 超时执行 call，并在失败原因为超时时按 policy 放宽截止时间重试：
// 首次使用 base，第 n 次重试使用 base * multiplier^n（受 maxTimeoutRetryDeadline 约束）。
// 非超时错误、父 context 已结束或重试次数耗尽时立即返回最后一次错误。base 非正数时不施加
// 截止时间，也不重试（没有可放大的基础超时）。
func retryOnTimeout[T any](ctx context.Context, base time.Duration, policy TimeoutRetryPolicy, call func(context.Context) (T, error)) (T, error) {
	var zero T
	attempts := 1
	if policy.Enabled && policy.MaxRetries > 0 && base > 0 {
		attempts = policy.MaxRetries + 1
	}
	multiplier := policy.Multiplier
	if multiplier <= 0 {
		multiplier = 2
	}
	timeout := base
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		attemptCtx := ctx
		cancel := context.CancelFunc(func() {})
		if timeout > 0 {
			attemptCtx, cancel = context.WithTimeout(ctx, timeout)
		}
		result, err := call(attemptCtx)
		cancel()
		if err == nil {
			return result, nil
		}
		lastErr = err
		// 父 context 已结束（请求取消或外层截止）时不再重试；非超时错误同样不重试。
		if attempt+1 >= attempts || ctx.Err() != nil || !isTimeoutError(err) {
			break
		}
		timeout = nextRetryTimeout(timeout, multiplier)
	}
	return zero, errors.WithStack(lastErr)
}

// nextRetryTimeout 返回下一次重试的截止时间：current 乘以 multiplier，并限制在
// maxTimeoutRetryDeadline 内；current 非正数时原样返回。
func nextRetryTimeout(current time.Duration, multiplier int) time.Duration {
	if current <= 0 || multiplier <= 1 {
		return current
	}
	if current >= maxTimeoutRetryDeadline/time.Duration(multiplier) {
		return maxTimeoutRetryDeadline
	}
	next := current * time.Duration(multiplier)
	if next > maxTimeoutRetryDeadline {
		return maxTimeoutRetryDeadline
	}
	return next
}

// isTimeoutError 判断错误是否由超时导致：优先使用 errors.Is 识别 context.DeadlineExceeded，
// 并兼容底层客户端以纯文本包装的常见超时文案（如 HTTP 响应头超时、i/o 超时）。
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "context deadline exceeded") ||
		strings.Contains(message, "timeout awaiting response headers") ||
		strings.Contains(message, "client.timeout exceeded") ||
		strings.Contains(message, "i/o timeout")
}
