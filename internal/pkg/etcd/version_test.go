package etcd

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// mockVersionHandler 实现 VersionHandler 接口，用于测试。
type mockVersionHandler struct {
	name      string
	handleFunc func(ctx context.Context, logger *logging.Logger, lastVersion string) (string, error)
}

func (m *mockVersionHandler) Name() string {
	return m.name
}

func (m *mockVersionHandler) Handle(ctx context.Context, logger *logging.Logger, lastVersion string) (string, error) {
	if m.handleFunc != nil {
		return m.handleFunc(ctx, logger, lastVersion)
	}
	return "", nil
}

// TestNewVersion 测试 NewVersion 函数。
func TestNewVersion(t *testing.T) {
	logger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	Convey("测试 NewVersion", t, func() {
		Convey("当参数有效时应返回非空的 Version 实例", func() {
			handler := &mockVersionHandler{
				name: "test-version",
			}
			version := NewVersion(nil, logger, handler)
			So(version, ShouldNotBeNil)
			So(version.client, ShouldBeNil)
			So(version.logger, ShouldEqual, logger)
			So(version.handler, ShouldEqual, handler)
		})

		Convey("当 handler 为 nil 时应返回实例但 handler 为 nil", func() {
			version := NewVersion(nil, logger, nil)
			So(version, ShouldNotBeNil)
			So(version.handler, ShouldBeNil)
		})

		Convey("当 logger 为 nil 时应返回实例但 logger 为 nil", func() {
			handler := &mockVersionHandler{
				name: "test-version",
			}
			version := NewVersion(nil, nil, handler)
			So(version, ShouldNotBeNil)
			So(version.logger, ShouldBeNil)
		})
	})
}

// TestVersionHandler_Interface 测试 VersionHandler 接口的实现。
func TestVersionHandler_Interface(t *testing.T) {
	logger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	Convey("测试 VersionHandler 接口", t, func() {
		Convey("mockVersionHandler 应实现 VersionHandler 接口", func() {
			handler := &mockVersionHandler{
				name: "test",
			}
			var _ VersionHandler = handler
			So(handler.Name(), ShouldEqual, "test")
		})

		Convey("Handle 方法应调用自定义函数", func() {
			called := false
			var receivedLastVersion string
			handler := &mockVersionHandler{
				handleFunc: func(ctx context.Context, logger *logging.Logger, lastVersion string) (string, error) {
					called = true
					receivedLastVersion = lastVersion
					return "v2.0.0", nil
				},
			}
			nextVersion, err := handler.Handle(context.Background(), logger, "v1.0.0")
			So(err, ShouldBeNil)
			So(called, ShouldBeTrue)
			So(receivedLastVersion, ShouldEqual, "v1.0.0")
			So(nextVersion, ShouldEqual, "v2.0.0")
		})

		Convey("当 handleFunc 为 nil 时 Handle 应返回空字符串和 nil 错误", func() {
			handler := &mockVersionHandler{
				name: "test",
			}
			nextVersion, err := handler.Handle(context.Background(), logger, "v1.0.0")
			So(err, ShouldBeNil)
			So(nextVersion, ShouldEqual, "")
		})
	})
}
