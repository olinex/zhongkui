package etcd

import (
	"context"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// mockElectionHandler 是 ElectionHandler 接口的模拟实现，用于单元测试
type mockElectionHandler struct {
	name        string
	candidateID string
	startErr    error
	shutdownCtx context.Context
	mu          sync.Mutex
	started     bool
	stopped     bool
}

func (h *mockElectionHandler) Name() string {
	return h.name
}

func (h *mockElectionHandler) CandidateID() string {
	return h.candidateID
}

func (h *mockElectionHandler) Start(ctx context.Context) error {
	h.mu.Lock()
	h.started = true
	h.mu.Unlock()
	if h.startErr != nil {
		return h.startErr
	}
	<-ctx.Done()
	return nil
}

func (h *mockElectionHandler) Shutdown(ctx context.Context) {
	h.mu.Lock()
	h.stopped = true
	h.mu.Unlock()
	h.shutdownCtx = ctx
}

func (h *mockElectionHandler) isStarted() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.started
}

func (h *mockElectionHandler) isStopped() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.stopped
}

// TestElectionPrefix 测试选举前缀常量
func TestElectionPrefix(t *testing.T) {
	Convey("测试选举前缀常量", t, func() {
		Convey("electionPrefix 应包含 leader-election/ 后缀", func() {
			So(electionPrefix, ShouldContainSubstring, "leader-election/")
		})

		Convey("electionPrefix 应以斜杠结尾", func() {
			So(electionPrefix[len(electionPrefix)-1], ShouldEqual, '/')
		})
	})
}

// TestNewElection 测试 NewElection 构造函数
func TestNewElection(t *testing.T) {
	Convey("测试 NewElection 构造函数", t, func() {
		logger, err := logging.NewDefaultLogger()
		So(err, ShouldBeNil)

		handler := &mockElectionHandler{
			name:        "test-election",
			candidateID: "node-1",
		}

		Convey("应正确初始化所有字段", func() {
			ctx := context.Background()
			election := NewElection(ctx, nil, logger, handler)

			So(election, ShouldNotBeNil)
			So(election.ctx, ShouldEqual, ctx)
			So(election.client, ShouldBeNil)
			So(election.logger, ShouldEqual, logger)
			So(election.handler, ShouldEqual, handler)
		})

		Convey("应支持可取消的上下文", func() {
			ctx, cancel := context.WithCancel(context.Background())
			election := NewElection(ctx, nil, logger, handler)

			So(election, ShouldNotBeNil)
			So(election.ctx, ShouldEqual, ctx)

			cancel()
			So(ctx.Err(), ShouldNotBeNil)
		})

		Convey("应支持带超时的上下文", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			election := NewElection(ctx, nil, logger, handler)
			So(election, ShouldNotBeNil)
			So(election.ctx, ShouldEqual, ctx)
		})

		Convey("不同的 handler 应创建不同的选举实例", func() {
			handler2 := &mockElectionHandler{
				name:        "another-election",
				candidateID: "node-2",
			}

			ctx := context.Background()
			e1 := NewElection(ctx, nil, logger, handler)
			e2 := NewElection(ctx, nil, logger, handler2)

			So(e1.handler.Name(), ShouldEqual, "test-election")
			So(e2.handler.Name(), ShouldEqual, "another-election")
			So(e1.handler.CandidateID(), ShouldEqual, "node-1")
			So(e2.handler.CandidateID(), ShouldEqual, "node-2")
		})
	})
}

// TestMockElectionHandler 测试 mockElectionHandler 的行为
func TestMockElectionHandler(t *testing.T) {
	Convey("测试 mockElectionHandler 的行为", t, func() {
		Convey("应正确返回 Name", func() {
			h := &mockElectionHandler{name: "scheduler"}
			So(h.Name(), ShouldEqual, "scheduler")
		})

		Convey("应正确返回 CandidateID", func() {
			h := &mockElectionHandler{candidateID: "host-1:8080"}
			So(h.CandidateID(), ShouldEqual, "host-1:8080")
		})

		Convey("Start 应在上下文取消前阻塞", func() {
			h := &mockElectionHandler{name: "test"}
			ctx, cancel := context.WithCancel(context.Background())

			done := make(chan struct{})
			go func() {
				_ = h.Start(ctx)
				close(done)
			}()

			time.Sleep(50 * time.Millisecond)
			So(h.isStarted(), ShouldBeTrue)

			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("Start 应在上下文取消后返回")
			}
		})

		Convey("Start 应返回配置的错误", func() {
			h := &mockElectionHandler{
				name:     "test",
				startErr: context.DeadlineExceeded,
			}
			err := h.Start(context.Background())
			So(err, ShouldEqual, context.DeadlineExceeded)
		})

		Convey("Shutdown 应记录调用", func() {
			h := &mockElectionHandler{name: "test"}
			ctx := context.Background()
			h.Shutdown(ctx)

			So(h.isStopped(), ShouldBeTrue)
			So(h.shutdownCtx, ShouldEqual, ctx)
		})
	})
}

// TestElectionHandlerInterface 测试 ElectionHandler 接口的实现
func TestElectionHandlerInterface(t *testing.T) {
	Convey("测试 ElectionHandler 接口实现", t, func() {
		Convey("mockElectionHandler 应实现 ElectionHandler 接口", func() {
			var _ ElectionHandler = &mockElectionHandler{}
		})

		Convey("应支持不同类型的 handler 配置", func() {
			cases := []struct {
				name        string
				candidateID string
			}{
				{"scheduler", "node-1"},
				{"task-dispatcher", "192.168.1.1:8080"},
				{"data-sync", "uuid-abc-123"},
				{"a", "b"},
			}

			for _, tc := range cases {
				h := &mockElectionHandler{
					name:        tc.name,
					candidateID: tc.candidateID,
				}
				So(h.Name(), ShouldEqual, tc.name)
				So(h.CandidateID(), ShouldEqual, tc.candidateID)
			}
		})
	})
}

// TestElection_Shutdown 测试 Election.Shutdown 方法
func TestElection_Shutdown(t *testing.T) {
	Convey("测试 Election.Shutdown 方法", t, func() {
		logger, err := logging.NewDefaultLogger()
		So(err, ShouldBeNil)

		handler := &mockElectionHandler{
			name:        "test-election",
			candidateID: "node-1",
		}

		Convey("调用 Shutdown 应触发 handler 的 Shutdown", func() {
			ctx := context.Background()
			election := NewElection(ctx, nil, logger, handler)

			election.Shutdown()

			So(handler.isStopped(), ShouldBeTrue)
			So(handler.shutdownCtx, ShouldEqual, ctx)
		})

		Convey("使用可取消上下文时 Shutdown 应传递原始上下文", func() {
			ctx, cancel := context.WithCancel(context.Background())
			election := NewElection(ctx, nil, logger, handler)

			election.Shutdown()

			So(handler.isStopped(), ShouldBeTrue)
			So(handler.shutdownCtx, ShouldEqual, ctx)

			cancel()
		})

		Convey("多次调用 Shutdown 不应 panic", func() {
			ctx := context.Background()
			election := NewElection(ctx, nil, logger, handler)

			So(func() {
				election.Shutdown()
				election.Shutdown()
				election.Shutdown()
			}, ShouldNotPanic)
		})
	})
}

// TestElection_Start_NilClient 测试 Start 方法在 nil 客户端时的行为
func TestElection_Start_NilClient(t *testing.T) {
	Convey("测试 Start 方法在 nil 客户端时的行为", t, func() {
		logger, err := logging.NewDefaultLogger()
		So(err, ShouldBeNil)

		handler := &mockElectionHandler{
			name:        "test-election",
			candidateID: "node-1",
		}

		Convey("当 client 为 nil 时 Start 应 panic", func() {
			ctx := context.Background()
			election := NewElection(ctx, nil, logger, handler)

			So(func() {
				_ = election.Start()
			}, ShouldPanic)
		})
	})
}

// TestElection_Start_InvalidClient 测试 Start 方法使用无效 etcd 客户端时的行为
func TestElection_Start_InvalidClient(t *testing.T) {
	Convey("测试 Start 方法使用无效 etcd 客户端时的行为", t, func() {
		logger, err := logging.NewDefaultLogger()
		So(err, ShouldBeNil)

		handler := &mockElectionHandler{
			name:        "test-election",
			candidateID: "node-1",
		}

		Convey("当 etcd 端点不可达时 Start 应在上下文超时后返回错误", func() {
			cfg := &Config{
				Endpoints:   []string{"localhost:9999"},
				DialTimeout: 200 * time.Millisecond,
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			client, clientErr := NewEtcdClient(ctx, cfg, logger)
			So(clientErr, ShouldBeNil)
			defer client.Close()

			election := NewElection(ctx, client, logger, handler)
			startErr := election.Start()
			So(startErr, ShouldNotBeNil)
		})
	})
}

// TestElection_Struct 测试 Election 结构体的字段赋值和读取
func TestElection_Struct(t *testing.T) {
	Convey("测试 Election 结构体", t, func() {
		logger, err := logging.NewDefaultLogger()
		So(err, ShouldBeNil)

		Convey("应正确保存所有字段", func() {
			handler := &mockElectionHandler{
				name:        "my-election",
				candidateID: "my-node",
			}
			ctx := context.Background()
			election := NewElection(ctx, nil, logger, handler)

			So(election.ctx, ShouldEqual, ctx)
			So(election.client, ShouldBeNil)
			So(election.logger, ShouldEqual, logger)
			So(election.handler, ShouldNotBeNil)
			So(election.handler.Name(), ShouldEqual, "my-election")
			So(election.handler.CandidateID(), ShouldEqual, "my-node")
		})

		Convey("不同实例应互相独立", func() {
			handler1 := &mockElectionHandler{name: "election-1", candidateID: "node-1"}
			handler2 := &mockElectionHandler{name: "election-2", candidateID: "node-2"}

			ctx1 := context.Background()
			ctx2 := context.Background()

			e1 := NewElection(ctx1, nil, logger, handler1)
			e2 := NewElection(ctx2, nil, logger, handler2)

			So(e1.handler.Name(), ShouldEqual, "election-1")
			So(e2.handler.Name(), ShouldEqual, "election-2")
			So(e1.handler.Name(), ShouldNotEqual, e2.handler.Name())
		})
	})
}
