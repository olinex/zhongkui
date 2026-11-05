package status

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func okProbe() Probe {
	return func(ctx context.Context) Status {
		return Status{Configured: true, Reachable: true}
	}
}

// TestRunConcurrentOrder 校验并发收集结果顺序与注册顺序一致。
func TestRunConcurrentOrder(t *testing.T) {
	reg := New()
	reg.Register(Spec{Name: "a", Category: "cache", Probe: okProbe()})
	reg.Register(Spec{Name: "b", Category: "db", Probe: okProbe()})
	reg.Register(Spec{Name: "c", Category: "s3", Probe: okProbe()})

	results := reg.Run(context.Background())
	if len(results) != 3 {
		t.Fatalf("unexpected result count: %d", len(results))
	}
	want := []string{"a", "b", "c"}
	for i, r := range results {
		if r.Name != want[i] {
			t.Errorf("order mismatch at %d: got %q want %q", i, r.Name, want[i])
		}
		if r.State() != "ok" {
			t.Errorf("state mismatch at %d: got %q want ok", i, r.State())
		}
	}
}

// TestPanicDoesNotInterruptOthers 校验单探测 panic 被 recover 且不中断其他探测。
func TestPanicDoesNotInterruptOthers(t *testing.T) {
	reg := New()

	var panicRan atomic.Bool
	reg.Register(Spec{Name: "panic", Category: "slow", Probe: func(ctx context.Context) Status {
		panicRan.Store(true)
		panic("boom")
	}})
	reg.Register(Spec{Name: "ok", Category: "db", Probe: okProbe()})

	results := reg.Run(context.Background())
	if !panicRan.Load() {
		t.Fatal("panic probe did not run")
	}
	if len(results) != 2 {
		t.Fatalf("unexpected result count: %d", len(results))
	}
	// panic 探测被标记为不可达，且不影响其他探测。
	if results[0].Reachable {
		t.Errorf("panic probe should be unreachable")
	}
	if results[0].State() != "unreachable" {
		t.Errorf("panic probe state %q want unreachable", results[0].State())
	}
	if results[0].Err == "" {
		t.Errorf("panic probe should record a panic reason")
	}
	if !results[1].Reachable || results[1].Err != "" {
		t.Errorf("other probe should remain reachable, got %+v", results[1])
	}
}

// TestDisabledState 校验未配置（Configured=false）依赖进入 disabled 态。
func TestDisabledState(t *testing.T) {
	reg := New()
	reg.Register(Spec{Name: "redis", Category: "cache", Probe: func(ctx context.Context) Status {
		return Status{Configured: false, Reachable: false}
	}})

	results := reg.Run(context.Background())
	if len(results) != 1 {
		t.Fatalf("unexpected result count: %d", len(results))
	}
	if results[0].Configured {
		t.Errorf("expected not configured")
	}
	if results[0].State() != "disabled" {
		t.Errorf("state %q want disabled", results[0].State())
	}
}

// TestTimeoutState 校验探测在独立超时后进入不可达态且耗时受限。
func TestTimeoutState(t *testing.T) {
	reg := New()
	reg.SetPerProbeTimeout(50 * time.Millisecond)

	var entered atomic.Bool
	reg.Register(Spec{Name: "slow", Category: "slow", Probe: func(ctx context.Context) Status {
		entered.Store(true)
		<-ctx.Done() // 等待超时 ctx 取消
		return Status{Configured: true, Reachable: false, Err: ctx.Err().Error()}
	}})

	results := reg.Run(context.Background())
	if len(results) != 1 {
		t.Fatalf("unexpected result count: %d", len(results))
	}
	if !entered.Load() {
		t.Fatal("slow probe did not run")
	}
	if results[0].Reachable {
		t.Errorf("timed-out probe should be unreachable")
	}
	if results[0].State() != "unreachable" {
		t.Errorf("state %q want unreachable", results[0].State())
	}
	if results[0].Err == "" {
		t.Errorf("expected a deadline error message")
	}
	if results[0].Latency <= 0 || results[0].Latency > 200*time.Millisecond {
		t.Errorf("latency out of expected range: %v", results[0].Latency)
	}
}

// TestRenderNoError 校验 Render 在带 localize 与 nil 两种情况下均可正常执行。
func TestRenderNoError(t *testing.T) {
	reg := New()
	reg.Register(Spec{Name: "redis", Category: "cache", Probe: okProbe()})
	reg.Run(context.Background())

	if err := reg.Render(func(key string) string { return key }); err != nil {
		t.Fatalf("render with localize failed: %v", err)
	}
	if err := reg.Render(nil); err != nil {
		t.Fatalf("render with nil localize failed: %v", err)
	}
}
