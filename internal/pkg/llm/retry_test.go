package llm

import (
	// 标准库
	"context"
	"testing"
	"time"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// TestRetryOnTimeout_DoublesAndStopsAtLimit 校验超时重试次数与每次超时按倍数放大：
// 全部尝试超时后应执行「首次 + MaxRetries」次，且后续 deadline 明显大于前一次。
func TestRetryOnTimeout_DoublesAndStopsAtLimit(t *testing.T) {
	const base = 20 * time.Millisecond
	observed := make([]time.Duration, 0, 4)
	calls := 0
	result, err := retryOnTimeout(context.Background(), base, TimeoutRetryPolicy{
		Enabled:    true,
		MaxRetries: 3,
		Multiplier: 2,
	}, func(ctx context.Context) (int, error) {
		calls++
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatalf("attempt %d has no deadline", calls)
		}
		observed = append(observed, time.Until(deadline))
		return 0, context.DeadlineExceeded
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("retryOnTimeout() error = %v, want context.DeadlineExceeded", err)
	}
	if result != 0 {
		t.Errorf("retryOnTimeout() result = %d, want zero", result)
	}
	if calls != 4 {
		t.Errorf("retryOnTimeout() calls = %d, want 4 (first + 3 retries)", calls)
	}
	for i := 1; i < len(observed); i++ {
		if observed[i] < observed[i-1]*3/2 {
			t.Errorf("attempt %d timeout = %v, want at least 1.5x previous %v", i+1, observed[i], observed[i-1])
		}
	}
}

// TestRetryOnTimeout_SucceedsAfterRetry 校验超时后重试可成功，并返回成功结果。
func TestRetryOnTimeout_SucceedsAfterRetry(t *testing.T) {
	calls := 0
	result, err := retryOnTimeout(context.Background(), 5*time.Millisecond, TimeoutRetryPolicy{
		Enabled:    true,
		MaxRetries: 3,
		Multiplier: 2,
	}, func(context.Context) (string, error) {
		calls++
		if calls < 3 {
			return "", context.DeadlineExceeded
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("retryOnTimeout() error = %v, want nil", err)
	}
	if result != "ok" {
		t.Errorf("retryOnTimeout() result = %q, want ok", result)
	}
	if calls != 3 {
		t.Errorf("retryOnTimeout() calls = %d, want 3", calls)
	}
}

// TestRetryOnTimeout_NoRetryForNonTimeout 校验非超时错误不触发重试。
func TestRetryOnTimeout_NoRetryForNonTimeout(t *testing.T) {
	calls := 0
	boom := errors.New("boom")
	_, err := retryOnTimeout(context.Background(), 5*time.Millisecond, TimeoutRetryPolicy{
		Enabled:    true,
		MaxRetries: 3,
		Multiplier: 2,
	}, func(context.Context) (int, error) {
		calls++
		return 0, boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("retryOnTimeout() error = %v, want boom", err)
	}
	if calls != 1 {
		t.Errorf("retryOnTimeout() calls = %d, want 1", calls)
	}
}

// TestRetryOnTimeout_DisabledOrNoBase 校验开关关闭或没有基础超时时只执行一次。
func TestRetryOnTimeout_DisabledOrNoBase(t *testing.T) {
	cases := []struct {
		name   string
		base   time.Duration
		policy TimeoutRetryPolicy
	}{
		{"disabled", 5 * time.Millisecond, TimeoutRetryPolicy{Enabled: false, MaxRetries: 3}},
		{"zero retries", 5 * time.Millisecond, TimeoutRetryPolicy{Enabled: true, MaxRetries: 0}},
		{"no base timeout", 0, TimeoutRetryPolicy{Enabled: true, MaxRetries: 3}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			_, err := retryOnTimeout(context.Background(), tc.base, tc.policy, func(context.Context) (int, error) {
				calls++
				return 0, context.DeadlineExceeded
			})
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("retryOnTimeout() error = %v, want context.DeadlineExceeded", err)
			}
			if calls != 1 {
				t.Errorf("retryOnTimeout() calls = %d, want 1", calls)
			}
		})
	}
}

// TestRetryOnTimeout_ParentCancelled 校验父 context 结束后不再重试，避免无意义等待。
func TestRetryOnTimeout_ParentCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	_, err := retryOnTimeout(ctx, 5*time.Millisecond, TimeoutRetryPolicy{
		Enabled:    true,
		MaxRetries: 3,
		Multiplier: 2,
	}, func(context.Context) (int, error) {
		calls++
		return 0, context.DeadlineExceeded
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("retryOnTimeout() error = %v, want context.DeadlineExceeded", err)
	}
	if calls != 1 {
		t.Errorf("retryOnTimeout() calls = %d, want 1", calls)
	}
}
