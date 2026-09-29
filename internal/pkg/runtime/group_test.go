package runtime

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

// fakeService 模拟一个可阻塞的服务，用于验证 Group 的生命周期编排。
// Start 阻塞直到 ctx 取消；Shutdown 记录调用次数并可阻塞。
type fakeService struct {
	name        string
	startErr    error
	ctxErr      error
	startCount  atomic.Int32
	shutdownCtn atomic.Int32
	started     chan struct{}
	shutdown    chan struct{}
}

// Name 返回服务名称。
func (f *fakeService) Name() string { return f.name }

// Start 记录启动并阻塞至上下文取消，返回预设的上下文错误。
func (f *fakeService) Start(ctx context.Context) error {
	f.startCount.Add(1)
	if f.startErr != nil {
		return f.startErr
	}
	select {
	case f.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	// 真实服务（如 etcd 选举）在 ctx 取消时正常返回 nil，
	// 返回非 nil 会被 errgroup 视为服务错误
	return f.ctxErr
}

// Shutdown 记录一次关闭调用并返回 nil。
func (f *fakeService) Shutdown(_ context.Context) error {
	f.shutdownCtn.Add(1)
	select {
	case f.shutdown <- struct{}{}:
	default:
	}
	return nil
}

// TestGroup_ShutdownOnCancel 验证 ctx 取消后所有服务都被优雅关闭。
// 注意：时序类断言必须保持单顶层 Convey + 顺序执行，
// 避免 goconvey 叶子重跑机制重复执行函数级副作用（goroutine/cancel）。
func TestGroup_ShutdownOnCancel(t *testing.T) {
	Convey("ctx 取消后所有服务启动并优雅关闭", t, func() {
		svcA := &fakeService{name: "a", started: make(chan struct{}, 1), shutdown: make(chan struct{}, 1)}
		svcB := &fakeService{name: "b", started: make(chan struct{}, 1), shutdown: make(chan struct{}, 1)}

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			done <- NewGroup(svcA, svcB).Run(ctx)
		}()

		// 等待两个服务均已启动
		<-svcA.started
		<-svcB.started
		So(svcA.startCount.Load(), ShouldEqual, 1)
		So(svcB.startCount.Load(), ShouldEqual, 1)

		cancel()

		// Run 正常返回 nil
		select {
		case err := <-done:
			So(err, ShouldBeNil)
		case <-time.After(2 * time.Second):
			t.Fatal("group run did not return after cancel")
		}

		// 所有服务均被关闭且只关闭一次
		So(svcA.shutdownCtn.Load(), ShouldEqual, 1)
		So(svcB.shutdownCtn.Load(), ShouldEqual, 1)
	})
}

// TestGroup_ErrorCancelsOthers 验证某个服务失败后其余服务被取消并关闭。
func TestGroup_ErrorCancelsOthers(t *testing.T) {
	Convey("某个服务失败后其余服务被取消并关闭", t, func() {
		svcA := &fakeService{
			name:     "a",
			startErr: errors.New("boom"),
			started:  make(chan struct{}, 1),
			shutdown: make(chan struct{}, 1),
		}
		svcB := &fakeService{name: "b", started: make(chan struct{}, 1), shutdown: make(chan struct{}, 1)}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			done <- NewGroup(svcA, svcB).Run(ctx)
		}()

		// 等待失败服务返回错误
		select {
		case err := <-done:
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "boom")
		case <-time.After(2 * time.Second):
			t.Fatal("group run did not return after service error")
		}

		// 失败服务自身也会被关闭
		So(svcA.shutdownCtn.Load(), ShouldEqual, 1)
		// 其余服务被取消并关闭
		So(svcB.startCount.Load(), ShouldEqual, 1)
		So(svcB.shutdownCtn.Load(), ShouldEqual, 1)
	})
}

// TestAdapters_ImplementInterfaces 编译期断言适配构造器返回的类型实现 Service 与 Named。
func TestAdapters_ImplementInterfaces(t *testing.T) {
	Convey("适配构造器返回值满足 Service 与 Named 接口", t, func() {
		// Service：构造器返回值类型即 Service 接口
		var _ Service = NewPipelineService(nil)
		var _ Service = NewElectionService(nil)
		var _ Service = NewProcessorService("p", nil)
		// Named：具体适配类型实现 Name 方法
		var _ Named = (*pipelineService)(nil)
		var _ Named = (*electionService)(nil)
		var _ Named = (*processorService)(nil)
		So(true, ShouldBeTrue)
	})
}
