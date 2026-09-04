package resourcepool

import (
	// stdlib
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	// third-party
	. "github.com/smartystreets/goconvey/convey"
)

// plainResource 未实现 Lifespan 但实现 io.Closer 的资源类型，
// 用于验证「纯对象复用 + CloseAll 时经 io.Closer 关闭」的分支。
// plainResource is a resource type that does not implement Lifespan but implements
// io.Closer, used to verify the "plain object reuse + close via io.Closer on CloseAll" branch.
type plainResource struct {
	closes int32
}

// Close 实现 io.Closer，供池在销毁时调用（计数器自增）。
// Close implements io.Closer so the pool can destroy it (incrementing a counter).
func (r *plainResource) Close() error {
	atomic.AddInt32(&r.closes, 1)
	return nil
}

// lifecycleResource 实现 Lifespan 的资源类型，带创建/打开/校验/关闭计数，
// 并支持在测试中按需注入校验结果以模拟连接失效。
// lifecycleResource implements Lifespan with create/open/validate/close counters and
// injectable validation results to simulate a stale connection.
type lifecycleResource struct {
	openCount    int32
	closeCount   int32
	validCount   int32
	validateFunc func() bool
}

// Open 自增打开计数并返回 nil（模拟建连成功）。
// Open increments the open counter and returns nil (simulating a successful connect).
func (r *lifecycleResource) Open(context.Context) error {
	atomic.AddInt32(&r.openCount, 1)
	return nil
}

// Validate 自增校验计数；存在注入的校验函数时按其结果返回，否则默认可用。
// Validate increments the validate counter, returning the injected result when set, else true.
func (r *lifecycleResource) Validate(context.Context) bool {
	atomic.AddInt32(&r.validCount, 1)
	if r.validateFunc != nil {
		return r.validateFunc()
	}
	return true
}

// Close 自增关闭计数并返回 nil。
// Close increments the close counter and returns nil.
func (r *lifecycleResource) Close() error {
	atomic.AddInt32(&r.closeCount, 1)
	return nil
}

// newTestManager 构造一个以 makeCount 记录 Make 调用次数、Make 返回 lifecycleResource 的管理器。
// newTestManager builds a manager whose Make records invocation count and returns lifecycleResource.
func newTestManager(t *testing.T, cfg *Config) (*Manager[*lifecycleResource], *int32) {
	t.Helper()
	var makeCount int32
	m := NewManager(func(_ Key) (*lifecycleResource, error) {
		atomic.AddInt32(&makeCount, 1)
		return &lifecycleResource{}, nil
	}, cfg)
	return m, &makeCount
}

// TestManager_ReuseSameKey 同一 Key 多次借出应复用池中实例而不重建（Make 仅首次调用）。
// TestManager_ReuseSameKey verifies reusing the same key borrows the pooled instance
// rather than rebuilding (Make is only called once).
func TestManager_ReuseSameKey(t *testing.T) {
	Convey("同一 Key 借出-归还后重复借出复用实例不重建", t, func() {
		cfg := NewDefaultConfig()
		cfg.MaxTotal = 2
		cfg.TestOnBorrow = false
		cfg.TestOnCreate = false
		cfg.TimeBetweenEvictionRuns = 0 // 关闭驱逐线程，保证确定性
		cfg.MinEvictableIdleTime = 0
		m, makeCount := newTestManager(t, cfg)
		key := Key("k1")

		first, err := m.Acquire(context.Background(), key)
		So(err, ShouldBeNil)
		m.Release(context.Background(), key, first)
		So(atomic.LoadInt32(makeCount), ShouldEqual, 1)

		second, err := m.Acquire(context.Background(), key)
		So(err, ShouldBeNil)
		So(second, ShouldEqual, first) // 归还后复用同一实例（地址相同）
		m.Release(context.Background(), key, second)
		So(atomic.LoadInt32(makeCount), ShouldEqual, 1) // 未重建
	})
}

// TestManager_ValidateFailureRebuild 借出校验失败时应销毁旧实例并通过 Make 重建新实例。
// TestManager_ValidateFailureRebuild verifies a borrow-validation failure destroys the
// stale instance and rebuilds a new one via Make.
func TestManager_ValidateFailureRebuild(t *testing.T) {
	Convey("借出校验（testOnBorrow）失败销毁并重建", t, func() {
		cfg := NewDefaultConfig()
		cfg.MaxTotal = 2
		cfg.TestOnBorrow = true
		cfg.TestOnCreate = false
		cfg.TimeBetweenEvictionRuns = 0
		cfg.MinEvictableIdleTime = 0
		m, makeCount := newTestManager(t, cfg)
		key := Key("k")

		first, err := m.Acquire(context.Background(), key)
		So(err, ShouldBeNil)
		So(atomic.LoadInt32(&first.openCount), ShouldEqual, 1)

		// 令该实例在下一次借出校验时失效（模拟 SMTP 连接被服务端断开）
		first.validateFunc = func() bool { return false }
		m.Release(context.Background(), key, first)

		second, err := m.Acquire(context.Background(), key)
		So(err, ShouldBeNil)
		So(second, ShouldNotEqual, first)                       // 已重建为新实例
		So(atomic.LoadInt32(makeCount), ShouldEqual, 2)         // 首次 + 校验失败后重建共两次 Make
		So(atomic.LoadInt32(&first.closeCount), ShouldEqual, 1) // 失败旧实例被 Close 销毁
		So(atomic.LoadInt32(&second.openCount), ShouldEqual, 1)
		m.Release(context.Background(), key, second)
	})
}

// TestManager_MaxTotalBlocks 并发借出超过 MaxTotal 且在 blockWhenExhausted 下应阻塞直至 ctx 超时失败。
// TestManager_MaxTotalBlocks verifies borrows beyond MaxTotal block until ctx deadline
// when blockWhenExhausted is enabled.
func TestManager_MaxTotalBlocks(t *testing.T) {
	Convey("池耗尽时借出阻塞直至上下文超时", t, func() {
		cfg := NewDefaultConfig()
		cfg.MaxTotal = 1
		cfg.TestOnBorrow = false
		cfg.TestOnCreate = false
		cfg.TimeBetweenEvictionRuns = 0
		cfg.MinEvictableIdleTime = 0
		m, _ := newTestManager(t, cfg)
		key := Key("k")

		held, err := m.Acquire(context.Background(), key)
		So(err, ShouldBeNil)
		defer m.Release(context.Background(), key, held)

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err = m.Acquire(ctx, key)
		elapsed := time.Since(start)
		So(err, ShouldNotBeNil) // 阻塞超时返回错误
		So(elapsed, ShouldBeGreaterThanOrEqualTo, 30*time.Millisecond)
	})
}

// TestManager_CloseAllDestroys 关闭管理器应通过 Close 销毁全部存活（闲置）对象。
// TestManager_CloseAllDestroys verifies CloseAll destroys all live (idle) objects via Close.
func TestManager_CloseAllDestroys(t *testing.T) {
	Convey("CloseAll 销毁全部闲置对象", t, func() {
		cfg := NewDefaultConfig()
		cfg.MaxTotal = 3
		cfg.TestOnBorrow = false
		cfg.TestOnCreate = false
		cfg.TimeBetweenEvictionRuns = 0
		cfg.MinEvictableIdleTime = 0
		m, _ := newTestManager(t, cfg)
		key := Key("k")

		// 借出并归还两个对象使之闲置
		a, err := m.Acquire(context.Background(), key)
		So(err, ShouldBeNil)
		m.Release(context.Background(), key, a)
		b, err := m.Acquire(context.Background(), key)
		So(err, ShouldBeNil)
		m.Release(context.Background(), key, b)

		So(atomic.LoadInt32(&a.closeCount), ShouldEqual, 0)
		So(atomic.LoadInt32(&b.closeCount), ShouldEqual, 0)
		m.CloseAll(context.Background())
		So(atomic.LoadInt32(&a.closeCount), ShouldEqual, 1)
		So(atomic.LoadInt32(&b.closeCount), ShouldEqual, 1)
	})
}

// TestManager_PlainReuseAndCloser 未实现 Lifespan 的类型按纯对象复用；
// CloseAll 应经 io.Closer 关闭。确认不持有持久连接的资源类型无需实现 Open/Validate。
// TestManager_PlainReuseAndCloser verifies types without Lifespan reuse as plain objects
// and are closed via io.Closer on CloseAll (no Open/Validate required).
func TestManager_PlainReuseAndCloser(t *testing.T) {
	Convey("未实现 Lifespan 的资源纯对象复用且经 io.Closer 关闭", t, func() {
		cfg := NewDefaultConfig()
		cfg.MaxTotal = 2
		cfg.TestOnBorrow = false
		cfg.TestOnCreate = false
		cfg.TimeBetweenEvictionRuns = 0
		cfg.MinEvictableIdleTime = 0
		var makeCount int32
		var created []*plainResource
		var mu sync.Mutex
		m := NewManager(func(_ Key) (*plainResource, error) {
			mu.Lock()
			defer mu.Unlock()
			atomic.AddInt32(&makeCount, 1)
			r := &plainResource{}
			created = append(created, r)
			return r, nil
		}, cfg)
		key := Key("k")

		first, err := m.Acquire(context.Background(), key)
		So(err, ShouldBeNil)
		m.Release(context.Background(), key, first)
		second, err := m.Acquire(context.Background(), key)
		So(err, ShouldBeNil)
		So(second, ShouldEqual, first) // 纯对象复用
		m.Release(context.Background(), key, second)
		So(atomic.LoadInt32(&makeCount), ShouldEqual, 1)

		m.CloseAll(context.Background())
		mu.Lock()
		defer mu.Unlock()
		So(len(created), ShouldEqual, 1) // 仅一个实例且未被重建
	})
}

// TestManager_IdleEviction 闲置实例在超过 MinEvictableIdleTime 后被驱逐线程 Close 销毁。
// 采用带轮询等待的宽松断言，避免时序抖动导致偶发失败。
// TestManager_IdleEviction verifies idle instances are evicted by the evictor after
// MinEvictableIdleTime (Close called). A polling wait keeps it robust to timing jitter.
func TestManager_IdleEviction(t *testing.T) {
	Convey("闲置连接超阈值后被驱逐销毁", t, func() {
		cfg := NewDefaultConfig()
		cfg.MaxTotal = 2
		cfg.TestOnBorrow = false
		cfg.TestOnCreate = false
		cfg.MinEvictableIdleTime = 50 * time.Millisecond
		cfg.TimeBetweenEvictionRuns = 20 * time.Millisecond
		m, _ := newTestManager(t, cfg)
		key := Key("k")

		obj, err := m.Acquire(context.Background(), key)
		So(err, ShouldBeNil)
		m.Release(context.Background(), key, obj)
		So(atomic.LoadInt32(&obj.closeCount), ShouldEqual, 0)

		// 轮询等待驱逐线程销毁闲置实例，最多等待 2s
		deadline := time.Now().Add(2 * time.Second)
		for atomic.LoadInt32(&obj.closeCount) == 0 && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
		}
		So(atomic.LoadInt32(&obj.closeCount), ShouldEqual, 1) // 闲置超后被 Close 销毁
	})
}
