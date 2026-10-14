package etcd

import (
	// 标准库
	"context"
	"sync"
	"testing"
	"time"

	// 第三方
	. "github.com/smartystreets/goconvey/convey"
	mvccpb "go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
)

// noopHandler 返回 nil 的空变更处理器。
func noopHandler() ChangeHandler {
	return ChangeHandlerFunc(func(_ context.Context, _ []byte) error { return nil })
}

// TestNewWatcher 测试 NewWatcher 的默认值与 WithWatchRetryInterval 选项覆盖。
func TestNewWatcher(t *testing.T) {
	logger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	handler := noopHandler()

	Convey("Test NewWatcher", t, func() {
		Convey("默认重连间隔为 watchRetryInterval", func() {
			w := NewWatcher(&EtcdClient{}, "test/change", handler, logger)
			So(w.client, ShouldNotBeNil)
			So(w.key, ShouldEqual, "test/change")
			So(w.handler, ShouldNotBeNil)
			So(w.logger, ShouldEqual, logger)
			So(w.opts.retryInterval, ShouldEqual, watchRetryInterval)
			So(w.watch, ShouldNotBeNil)
		})

		Convey("WithWatchRetryInterval 覆盖重连间隔", func() {
			w := NewWatcher(&EtcdClient{}, "test/change", handler, logger, WithWatchRetryInterval(time.Second))
			So(w.opts.retryInterval, ShouldEqual, time.Second)
		})
	})
}

// TestWatcherStartNoEtcd 未绑定 etcd 客户端时 Start 直接返回（无监听能力）。
func TestWatcherStartNoEtcd(t *testing.T) {
	logger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	Convey("Test Watcher Start without etcd client", t, func() {
		w := NewWatcher(nil, "test/change", noopHandler(), logger)
		So(w.Start(context.Background()), ShouldBeNil)
	})
}

// TestWatcherHandleResponse 测试 handleResponse 对 watch 响应的处理。
func TestWatcherHandleResponse(t *testing.T) {
	logger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	Convey("Test Watcher HandleResponse", t, func() {
		ctx := context.Background()

		Convey("响应携带错误时返回非 nil", func() {
			w := NewWatcher(&EtcdClient{}, "test/change", noopHandler(), logger)
			err := w.handleResponse(ctx, clientv3.WatchResponse{Canceled: true})
			So(err, ShouldNotBeNil)
		})

		Convey("空事件直接返回 nil 且不调用 handler", func() {
			called := false
			w := NewWatcher(&EtcdClient{}, "test/change", ChangeHandlerFunc(func(_ context.Context, _ []byte) error {
				called = true
				return nil
			}), logger)
			So(w.handleResponse(ctx, clientv3.WatchResponse{}), ShouldBeNil)
			So(called, ShouldBeFalse)
		})

		Convey("单个 PUT 事件 handler 收到该 value", func() {
			var got []byte
			w := NewWatcher(&EtcdClient{}, "test/change", ChangeHandlerFunc(func(_ context.Context, value []byte) error {
				got = value
				return nil
			}), logger)
			resp := clientv3.WatchResponse{Events: []*clientv3.Event{{
				Type: mvccpb.PUT,
				Kv:   &mvccpb.KeyValue{Value: []byte("payload-1")},
			}}}
			So(w.handleResponse(ctx, resp), ShouldBeNil)
			So(got, ShouldResemble, []byte("payload-1"))
		})

		Convey("单事件回调失败仅告警，后续事件仍被处理，最终返回 nil", func() {
			calls := 0
			var got []byte
			w := NewWatcher(&EtcdClient{}, "test/change", ChangeHandlerFunc(func(_ context.Context, value []byte) error {
				calls++
				if calls == 1 {
					return errors.New("handler boom")
				}
				got = value
				return nil
			}), logger)
			resp := clientv3.WatchResponse{Events: []*clientv3.Event{
				{Type: mvccpb.PUT, Kv: &mvccpb.KeyValue{Value: []byte("first")}},
				{Type: mvccpb.PUT, Kv: &mvccpb.KeyValue{Value: []byte("second")}},
			}}
			So(w.handleResponse(ctx, resp), ShouldBeNil)
			So(calls, ShouldEqual, 2)
			So(got, ShouldResemble, []byte("second"))
		})
	})
}

// TestWatcherStartReconnect 测试 watch 中断后自动重连，ctx 取消后 Start 返回 nil。
func TestWatcherStartReconnect(t *testing.T) {
	logger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	Convey("Test Watcher Start reconnect", t, func() {
		// 第一次 watch 返回的通道：先发一个事件再关闭，模拟 watch 中断
		first := make(chan clientv3.WatchResponse, 1)
		first <- clientv3.WatchResponse{Events: []*clientv3.Event{{
			Type: mvccpb.PUT,
			Kv:   &mvccpb.KeyValue{Value: []byte("v1")},
		}}}
		close(first)
		// 第二次 watch 返回的通道：阻塞直到 ctx 取消
		blocking := make(chan clientv3.WatchResponse)
		var mu sync.Mutex
		watchCalls := 0
		fakeWatch := func(_ context.Context, _ string) clientv3.WatchChan {
			mu.Lock()
			watchCalls++
			calls := watchCalls
			mu.Unlock()
			if calls == 1 {
				return first
			}
			return blocking
		}
		handled := make(chan []byte, 1)
		handler := ChangeHandlerFunc(func(_ context.Context, value []byte) error {
			handled <- value
			return nil
		})
		w := NewWatcher(&EtcdClient{}, "test/change", handler, logger, WithWatchRetryInterval(time.Millisecond))
		w.watch = fakeWatch

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		errCh := make(chan error, 1)
		go func() { errCh <- w.Start(ctx) }()

		// 第一次 watch 的事件应被 handler 收到
		select {
		case value := <-handled:
			So(value, ShouldResemble, []byte("v1"))
		case <-time.After(3 * time.Second):
			t.Fatal("handler was not called within timeout")
		}

		// 第一次 watch 通道关闭后应自动重连（第二次调用 watch 函数）
		So(waitFor(func() bool {
			mu.Lock()
			defer mu.Unlock()
			return watchCalls >= 2
		}, 3*time.Second), ShouldBeTrue)

		// ctx 取消后 Start 应返回 nil
		cancel()
		select {
		case err := <-errCh:
			So(err, ShouldBeNil)
		case <-time.After(3 * time.Second):
			t.Fatal("Start did not return after ctx cancel")
		}
	})
}

// waitFor 轮询条件直到满足或超时。
func waitFor(cond func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
}
