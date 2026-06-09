package grpc

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// mockServerStream 是 g.ServerStream 接口的模拟实现
// 用于测试 WrapperServerStream 的包装功能
type mockServerStream struct {
	contextCalled    bool
	sendHeaderCalled bool
	setHeaderCalled  bool
	setTrailerCalled bool
	sendMsgCalled    bool
	recvMsgCalled    bool
	headerMD         metadata.MD
	trailerMD        metadata.MD
	contextValue     context.Context
	sendMsgError     error
	recvMsgError     error
}

// SetHeader 模拟设置响应头
func (m *mockServerStream) SetHeader(md metadata.MD) error {
	m.setHeaderCalled = true
	m.headerMD = md
	return nil
}

// SendHeader 模拟发送响应头
func (m *mockServerStream) SendHeader(md metadata.MD) error {
	m.sendHeaderCalled = true
	m.headerMD = md
	return nil
}

// SetTrailer 模拟设置响应尾
func (m *mockServerStream) SetTrailer(md metadata.MD) {
	m.setTrailerCalled = true
	m.trailerMD = md
}

// Context 模拟返回上下文
func (m *mockServerStream) Context() context.Context {
	m.contextCalled = true
	if m.contextValue != nil {
		return m.contextValue
	}
	return context.Background()
}

// SendMsg 模拟发送消息
func (m *mockServerStream) SendMsg(msg any) error {
	m.sendMsgCalled = true
	return m.sendMsgError
}

// RecvMsg 模拟接收消息
func (m *mockServerStream) RecvMsg(msg any) error {
	m.recvMsgCalled = true
	return m.recvMsgError
}

// TestNewWrapperServerStream 测试创建包装器流
func TestNewWrapperServerStream(t *testing.T) {
	Convey("TestNewWrapperServerStream", t, func() {
		Convey("当创建包装器流时", func() {
			userID := "test-user-123"
			mockStream := &mockServerStream{}
			wrapper := NewWrapperServerStream(userID, mockStream)

			Convey("应该正确设置用户ID", func() {
				// 由于 userID 字段是私有的，我们通过 Context 方法来验证
				ctx := wrapper.Context()
				extractedUserID := ctx.Value(ACCESS_KEY_USER_ID)
				So(extractedUserID, ShouldEqual, userID)
			})

			Convey("应该正确包装底层流", func() {
				So(wrapper, ShouldNotBeNil)
			})
		})
	})
}

// TestWrapperServerStream_Context 测试包装器流的上下文功能
func TestWrapperServerStream_Context(t *testing.T) {
	Convey("TestWrapperServerStream_Context", t, func() {
		Convey("当获取包装器流的上下文时", func() {
			userID := "test-user-456"
			mockStream := &mockServerStream{}
			wrapper := NewWrapperServerStream(userID, mockStream)

			Convey("应该在上下文中包含用户ID", func() {
				ctx := wrapper.Context()
				extractedUserID := ctx.Value(ACCESS_KEY_USER_ID)
				So(extractedUserID, ShouldEqual, userID)
			})

			Convey("应该调用底层流的Context方法", func() {
				wrapper.Context()
				So(mockStream.contextCalled, ShouldBeTrue)
			})

			Convey("当底层流有自定义上下文时", func() {
				originalCtx := context.WithValue(context.Background(), "custom-key", "custom-value")
				mockStream.contextValue = originalCtx
				wrapper := NewWrapperServerStream(userID, mockStream)

				Convey("应该保留原始上下文的值", func() {
					ctx := wrapper.Context()
					customValue := ctx.Value("custom-key")
					So(customValue, ShouldEqual, "custom-value")
				})

				Convey("应该同时包含用户ID", func() {
					ctx := wrapper.Context()
					extractedUserID := ctx.Value(ACCESS_KEY_USER_ID)
					So(extractedUserID, ShouldEqual, userID)
				})
			})
		})
	})
}

// TestWrapperServerStream_SetHeader 测试设置响应头
func TestWrapperServerStream_SetHeader(t *testing.T) {
	Convey("TestWrapperServerStream_SetHeader", t, func() {
		Convey("当设置响应头时", func() {
			userID := "test-user-789"
			mockStream := &mockServerStream{}
			wrapper := NewWrapperServerStream(userID, mockStream)
			md := metadata.Pairs("key1", "value1", "key2", "value2")

			err := wrapper.SetHeader(md)

			Convey("应该成功", func() {
				So(err, ShouldBeNil)
			})

			Convey("应该调用底层流的SendHeader方法", func() {
				So(mockStream.sendHeaderCalled, ShouldBeTrue)
			})

			Convey("应该传递正确的元数据", func() {
				So(mockStream.headerMD.Get("key1"), ShouldResemble, []string{"value1"})
				So(mockStream.headerMD.Get("key2"), ShouldResemble, []string{"value2"})
			})
		})
	})
}

// TestWrapperServerStream_SendHeader 测试发送响应头
func TestWrapperServerStream_SendHeader(t *testing.T) {
	Convey("TestWrapperServerStream_SendHeader", t, func() {
		Convey("当发送响应头时", func() {
			userID := "test-user-999"
			mockStream := &mockServerStream{}
			wrapper := NewWrapperServerStream(userID, mockStream)
			md := metadata.Pairs("content-type", "application/grpc")

			err := wrapper.SendHeader(md)

			Convey("应该成功", func() {
				So(err, ShouldBeNil)
			})

			Convey("应该调用底层流的SendHeader方法", func() {
				So(mockStream.sendHeaderCalled, ShouldBeTrue)
			})

			Convey("应该传递正确的元数据", func() {
				So(mockStream.headerMD.Get("content-type"), ShouldResemble, []string{"application/grpc"})
			})
		})
	})
}

// TestWrapperServerStream_SetTrailer 测试设置响应尾
func TestWrapperServerStream_SetTrailer(t *testing.T) {
	Convey("TestWrapperServerStream_SetTrailer", t, func() {
		Convey("当设置响应尾时", func() {
			userID := "test-user-111"
			mockStream := &mockServerStream{}
			wrapper := NewWrapperServerStream(userID, mockStream)
			md := metadata.Pairs("status", "ok", "code", "200")

			wrapper.SetTrailer(md)

			Convey("应该调用底层流的SetTrailer方法", func() {
				So(mockStream.setTrailerCalled, ShouldBeTrue)
			})

			Convey("应该传递正确的元数据", func() {
				So(mockStream.trailerMD.Get("status"), ShouldResemble, []string{"ok"})
				So(mockStream.trailerMD.Get("code"), ShouldResemble, []string{"200"})
			})
		})
	})
}

// TestWrapperServerStream_SendMsg 测试发送消息
func TestWrapperServerStream_SendMsg(t *testing.T) {
	Convey("TestWrapperServerStream_SendMsg", t, func() {
		Convey("当发送消息时", func() {
			userID := "test-user-222"
			mockStream := &mockServerStream{}
			wrapper := NewWrapperServerStream(userID, mockStream)
			msg := "test message"

			err := wrapper.SendMsg(msg)

			Convey("应该成功", func() {
				So(err, ShouldBeNil)
			})

			Convey("应该调用底层流的SendMsg方法", func() {
				So(mockStream.sendMsgCalled, ShouldBeTrue)
			})

			Convey("当底层流发送消息失败时", func() {
				mockStream.sendMsgError = status.Errorf(codes.Internal, "internal error")
				err := wrapper.SendMsg(msg)

				Convey("应该返回错误", func() {
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldContainSubstring, "internal error")
				})
			})
		})
	})
}

// TestWrapperServerStream_RecvMsg 测试接收消息
func TestWrapperServerStream_RecvMsg(t *testing.T) {
	Convey("TestWrapperServerStream_RecvMsg", t, func() {
		Convey("当接收消息时", func() {
			userID := "test-user-333"
			mockStream := &mockServerStream{}
			wrapper := NewWrapperServerStream(userID, mockStream)
			var msg string

			err := wrapper.RecvMsg(&msg)

			Convey("应该成功", func() {
				So(err, ShouldBeNil)
			})

			Convey("应该调用底层流的RecvMsg方法", func() {
				So(mockStream.recvMsgCalled, ShouldBeTrue)
			})

			Convey("当底层流接收消息失败时", func() {
				mockStream.recvMsgError = status.Errorf(codes.Unavailable, "unavailable")
				err := wrapper.RecvMsg(&msg)

				Convey("应该返回错误", func() {
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldContainSubstring, "unavailable")
				})
			})
		})
	})
}

// TestWrapperServerStream_Integration 测试包装器流的集成功能
func TestWrapperServerStream_Integration(t *testing.T) {
	Convey("TestWrapperServerStream_Integration", t, func() {
		Convey("当使用包装器流进行完整操作时", func() {
			userID := "integration-user"
			mockStream := &mockServerStream{}
			wrapper := NewWrapperServerStream(userID, mockStream)

			Convey("应该能够获取包含用户ID的上下文", func() {
				ctx := wrapper.Context()
				extractedUserID := ExtractUserID(ctx)
				So(extractedUserID, ShouldEqual, userID)
			})

			Convey("应该能够设置和发送头部", func() {
				headerMD := metadata.Pairs("authorization", "Bearer token123")
				err := wrapper.SetHeader(headerMD)
				So(err, ShouldBeNil)
				So(mockStream.sendHeaderCalled, ShouldBeTrue)
			})

			Convey("应该能够发送消息", func() {
				err := wrapper.SendMsg("test data")
				So(err, ShouldBeNil)
				So(mockStream.sendMsgCalled, ShouldBeTrue)
			})

			Convey("应该能够接收消息", func() {
				var msg string
				err := wrapper.RecvMsg(&msg)
				So(err, ShouldBeNil)
				So(mockStream.recvMsgCalled, ShouldBeTrue)
			})

			Convey("应该能够设置尾部", func() {
				trailerMD := metadata.Pairs("grpc-status", "0")
				wrapper.SetTrailer(trailerMD)
				So(mockStream.setTrailerCalled, ShouldBeTrue)
			})
		})
	})
}
