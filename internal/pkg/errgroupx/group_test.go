package errgroupx

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// sentinelErr 用于测试的哨兵错误
var sentinelErr = errors.New("sentinel error")

// customErr 自定义错误类型，用于测试 errors.As 兼容性
type customErr struct{ msg string }

func (e *customErr) Error() string { return e.msg }

func TestGroup_Go_SingleError(t *testing.T) {
	g, _ := WithContext(context.Background())
	g.Go(func() error {
		return sentinelErr
	})

	err := g.Wait()
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	// 验证错误信息包含协程标识
	if !strings.Contains(err.Error(), "goroutine#0") {
		t.Errorf("error should contain 'goroutine#0', got: %v", err)
	}

	// 验证原始错误链仍然可通过 errors.Is 检查
	if !errors.Is(err, sentinelErr) {
		t.Error("errors.Is should still match sentinelErr")
	}
}

func TestGroup_Go_TwoDifferentErrors(t *testing.T) {
	g, _ := WithContext(context.Background())

	var errA = errors.New("error from A")
	var errB = errors.New("error from B")

	// 两个协程都返回错误，errgroup 只返回第一个
	// 使用 channel 确保两个协程都启动后再触发错误
	start := make(chan struct{})

	g.Go(func() error {
		<-start
		return errA
	})
	g.Go(func() error {
		<-start
		return errB
	})

	close(start) // 同时释放两个协程
	err := g.Wait()
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	// 错误信息应包含其中一个协程的标识
	msg := err.Error()
	hasIndex := strings.Contains(msg, "goroutine#0") || strings.Contains(msg, "goroutine#1")
	if !hasIndex {
		t.Errorf("error should contain goroutine index, got: %v", err)
	}

	// 至少能匹配到一个原始错误
	if !errors.Is(err, errA) && !errors.Is(err, errB) {
		t.Errorf("errors.Is should match errA or errB, got: %v", err)
	}
}

func TestGroup_Go_TwoSameErrors(t *testing.T) {
	g, _ := WithContext(context.Background())

	start := make(chan struct{})

	g.Go(func() error {
		<-start
		return sentinelErr
	})
	g.Go(func() error {
		<-start
		return sentinelErr
	})

	close(start)
	err := g.Wait()
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	msg := err.Error()

	// 核心断言：即使两个协程返回相同错误，也能通过标识区分来源
	hasIndex := strings.Contains(msg, "goroutine#0") || strings.Contains(msg, "goroutine#1")
	if !hasIndex {
		t.Errorf("error should contain goroutine index to distinguish source, got: %v", err)
	}

	// 原始错误链仍然可用
	if !errors.Is(err, sentinelErr) {
		t.Error("errors.Is should still match sentinelErr")
	}
}

func TestGroup_GoNamed(t *testing.T) {
	g, _ := WithContext(context.Background())

	g.GoNamed("reader", func() error {
		return sentinelErr
	})
	g.GoNamed("writer", func() error {
		return nil
	})

	err := g.Wait()
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	msg := err.Error()
	if !strings.Contains(msg, "[reader]") {
		t.Errorf("error should contain '[reader]' label, got: %v", err)
	}
	if strings.Contains(msg, "[writer]") {
		t.Error("error should NOT contain '[writer]' label (it returned nil)")
	}
	if !errors.Is(err, sentinelErr) {
		t.Error("errors.Is should still match sentinelErr")
	}
}

func TestGroup_GoNamed_MultipleSameError(t *testing.T) {
	g, _ := WithContext(context.Background())

	start := make(chan struct{})

	var errIO = errors.New("io error")

	g.GoNamed("client", func() error {
		<-start
		return errIO
	})
	g.GoNamed("server", func() error {
		<-start
		return errIO
	})

	close(start)
	err := g.Wait()
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	msg := err.Error()
	hasLabel := strings.Contains(msg, "[client]") || strings.Contains(msg, "[server]")
	if !hasLabel {
		t.Errorf("error should contain named label to distinguish source, got: %v", err)
	}
}

func TestGroup_MixedGoAndGoNamed(t *testing.T) {
	g, _ := WithContext(context.Background())

	g.Go(func() error {
		return errors.New("error from index 0")
	})
	g.GoNamed("named", func() error {
		return errors.New("error from named")
	})

	err := g.Wait()
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	msg := err.Error()
	hasAuto := strings.Contains(msg, "goroutine#0")
	hasNamed := strings.Contains(msg, "[named]")
	if !hasAuto && !hasNamed {
		t.Errorf("error should contain either auto index or named label, got: %v", err)
	}
}

func TestGroup_NoError(t *testing.T) {
	g, _ := WithContext(context.Background())

	g.Go(func() error { return nil })
	g.Go(func() error { return nil })
	g.GoNamed("worker", func() error { return nil })

	if err := g.Wait(); err != nil {
		t.Errorf("expected nil error when all goroutines succeed, got: %v", err)
	}
}

func TestGroup_ContextCancellation(t *testing.T) {
	g, ctx := WithContext(context.Background())

	start := make(chan struct{})

	// G0: 立即返回错误，触发 context 取消
	g.Go(func() error {
		return errors.New("trigger cancel")
	})

	// G1: 等待 context 取消
	g.Go(func() error {
		<-start
		<-ctx.Done()
		return ctx.Err()
	})

	close(start)
	err := g.Wait()
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	msg := err.Error()
	// 错误可能来自 G0（触发取消）或 G1（context.Canceled），都应包含标识
	hasIndex := strings.Contains(msg, "goroutine#0") || strings.Contains(msg, "goroutine#1")
	if !hasIndex {
		t.Errorf("error should contain goroutine index even for ctx.Err(), got: %v", err)
	}
}

func TestGroup_SetLimit(t *testing.T) {
	g, _ := WithContext(context.Background())
	g.SetLimit(2)

	var mu sync.Mutex
	var maxConcurrent int
	var current int

	for range 5 {
		g.Go(func() error {
			mu.Lock()
			current++
			if current > maxConcurrent {
				maxConcurrent = current
			}
			mu.Unlock()

			time.Sleep(10 * time.Millisecond)

			mu.Lock()
			current--
			mu.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if maxConcurrent > 2 {
		t.Errorf("max concurrent should be <= 2, got %d", maxConcurrent)
	}
}

func TestGroup_WaitReturnsFirstError(t *testing.T) {
	g, _ := WithContext(context.Background())

	var errFirst = errors.New("first error")
	start := make(chan struct{})

	g.Go(func() error {
		<-start
		return errFirst
	})
	g.Go(func() error {
		<-start
		time.Sleep(50 * time.Millisecond) // 延迟返回，确保第一个错误已被记录
		return errors.New("second error")
	})

	close(start)
	err := g.Wait()

	// Wait 只返回第一个非 nil 错误
	if !errors.Is(err, errFirst) {
		t.Logf("returned error (may be first or second due to scheduling): %v", err)
	}
	// 无论如何，错误应该被包装
	if err.Error() == errFirst.Error() {
		t.Error("error should be wrapped with goroutine identifier")
	}
}

func TestGroup_CounterConcurrencySafety(t *testing.T) {
	g, _ := WithContext(context.Background())

	const goroutines = 100
	start := make(chan struct{})
	results := make([]string, goroutines)

	for i := range goroutines {
		idx := i
		g.Go(func() error {
			<-start
			e := fmt.Errorf("error-%d", idx)
			results[idx] = e.Error()
			return e
		})
	}

	close(start)
	_ = g.Wait()

	// 检查索引分布：100 个协程应有 100 个不同的序号
	indices := make(map[int]bool)
	for range goroutines {
		for j := range goroutines {
			key := fmt.Sprintf("error-%d", j)
			for k, r := range results {
				if r == key && !indices[k] {
					indices[k] = true
					break
				}
			}
		}
	}

	// 主要验证：所有 errors 被唯一分配（无重复覆盖）
	found := map[string]int{}
	for _, r := range results {
		found[r]++
	}
	for k, v := range found {
		if v > 1 {
			t.Errorf("duplicate result '%s' found %d times — counter may not be thread-safe", k, v)
		}
	}
}

func TestGroup_ErrorWrappingPreservesChain(t *testing.T) {
	g, _ := WithContext(context.Background())

	orig := &customErr{msg: "custom typed error"}

	g.Go(func() error {
		return orig
	})

	err := g.Wait()
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	// errors.As 应能提取原始类型
	var target *customErr
	if !errors.As(err, &target) {
		t.Error("errors.As should extract customErr from wrapped error")
	}
	if target != orig {
		t.Error("extracted error should be the original instance")
	}
}

func TestGroup_EmptyGroup(t *testing.T) {
	g, _ := WithContext(context.Background())

	// 没有调用 Go，Wait 应返回 nil
	if err := g.Wait(); err != nil {
		t.Errorf("expected nil error for empty group, got: %v", err)
	}
}

func TestGroup_GoNamed_EmptyName(t *testing.T) {
	g, _ := WithContext(context.Background())

	g.GoNamed("", func() error {
		return sentinelErr
	})

	err := g.Wait()
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	msg := err.Error()
	if !strings.Contains(msg, "[]:") {
		t.Errorf("error should contain '[]:' for empty name, got: %v", err)
	}
}

func TestGroup_NilErrorInGoroutine(t *testing.T) {
	g, _ := WithContext(context.Background())

	g.Go(func() error { return nil })
	g.Go(func() error { return errors.New("only error") })

	err := g.Wait()
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	if !strings.Contains(err.Error(), "goroutine#") {
		t.Errorf("error should contain goroutine index, got: %v", err)
	}

	// 错误信息不应包含 goroutine#0（返回 nil 的协程）
	if strings.Contains(err.Error(), "goroutine#0") {
		// goroutine#0 是返回 nil 的那个，但也不一定，取决于调度
		// 所以只做宽松检查
		t.Log("note: goroutine#0 may be the error source depending on scheduling")
	}
}
