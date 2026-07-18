package etcd

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// mockElectionHandler 实现 ElectionHandler 接口，用于测试。
type mockElectionHandler struct {
	name         string
	candidateID  string
	startFunc    func(ctx context.Context) error
	shutdownFunc func(ctx context.Context) error
}

func (m *mockElectionHandler) Name() string {
	return m.name
}

func (m *mockElectionHandler) CandidateID() string {
	return m.candidateID
}

func (m *mockElectionHandler) Start(ctx context.Context) error {
	if m.startFunc != nil {
		return m.startFunc(ctx)
	}
	return nil
}

func (m *mockElectionHandler) Shutdown(ctx context.Context) error {
	if m.shutdownFunc != nil {
		return m.shutdownFunc(ctx)
	}
	return nil
}

// TestNewElection 测试 NewElection 函数。
func TestNewElection(t *testing.T) {
	logger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	Convey("测试 NewElection", t, func() {
		Convey("当参数有效时应返回非空的 Election 实例", func() {
			ctx := context.Background()
			handler := &mockElectionHandler{
				name:        "test-election",
				candidateID: "candidate-1",
			}
			election := NewElection(ctx, nil, logger, handler)
			So(election, ShouldNotBeNil)
			So(election.ctx, ShouldEqual, ctx)
			So(election.client, ShouldBeNil)
			So(election.logger, ShouldEqual, logger)
			So(election.handler, ShouldEqual, handler)
		})

		Convey("当 handler 为 nil 时应返回实例但 handler 为 nil", func() {
			ctx := context.Background()
			election := NewElection(ctx, nil, logger, nil)
			So(election, ShouldNotBeNil)
			So(election.handler, ShouldBeNil)
		})

		Convey("当 logger 为 nil 时应返回实例但 logger 为 nil", func() {
			ctx := context.Background()
			handler := &mockElectionHandler{
				name:        "test-election",
				candidateID: "candidate-1",
			}
			election := NewElection(ctx, nil, nil, handler)
			So(election, ShouldNotBeNil)
			So(election.logger, ShouldBeNil)
		})
	})
}

// TestElectionHandler_Interface 测试 ElectionHandler 接口的实现。
func TestElectionHandler_Interface(t *testing.T) {
	Convey("测试 ElectionHandler 接口", t, func() {
		Convey("mockElectionHandler 应实现 ElectionHandler 接口", func() {
			handler := &mockElectionHandler{
				name:        "test",
				candidateID: "id-1",
			}
			var _ ElectionHandler = handler
			So(handler.Name(), ShouldEqual, "test")
			So(handler.CandidateID(), ShouldEqual, "id-1")
		})

		Convey("Start 方法应调用自定义函数", func() {
			called := false
			handler := &mockElectionHandler{
				startFunc: func(ctx context.Context) error {
					called = true
					return nil
				},
			}
			err := handler.Start(context.Background())
			So(err, ShouldBeNil)
			So(called, ShouldBeTrue)
		})

		Convey("Shutdown 方法应调用自定义函数", func() {
			called := false
			handler := &mockElectionHandler{
				shutdownFunc: func(ctx context.Context) error {
					called = true
					return nil
				},
			}
			err := handler.Shutdown(context.Background())
			So(err, ShouldBeNil)
			So(called, ShouldBeTrue)
		})

		Convey("当 startFunc 为 nil 时 Start 应返回 nil", func() {
			handler := &mockElectionHandler{}
			err := handler.Start(context.Background())
			So(err, ShouldBeNil)
		})

		Convey("当 shutdownFunc 为 nil 时 Shutdown 应返回 nil", func() {
			handler := &mockElectionHandler{}
			err := handler.Shutdown(context.Background())
			So(err, ShouldBeNil)
		})
	})
}

// TestElection_Shutdown 测试 Election.Shutdown 方法。
func TestElection_Shutdown(t *testing.T) {
	logger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	Convey("测试 Election.Shutdown", t, func() {
		Convey("Shutdown 应调用 handler.Shutdown", func() {
			called := false
			ctx := context.Background()
			handler := &mockElectionHandler{
				name:        "test",
				candidateID: "id-1",
				shutdownFunc: func(ctx context.Context) error {
					called = true
					return nil
				},
			}
			election := NewElection(ctx, nil, logger, handler)
			election.Shutdown()
			So(called, ShouldBeTrue)
		})

		Convey("当 handler 为 nil 时 Shutdown 应 panic", func() {
			ctx := context.Background()
			election := NewElection(ctx, nil, logger, nil)
			So(func() {
				election.Shutdown()
			}, ShouldPanic)
		})
	})
}
