package grpc

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	g "google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// testAuthenticator 是Authenticator接口的测试实现
// 用于测试身份验证路由器的功能
type testAuthenticator struct {
	shouldFail bool   // 是否模拟验证失败
	userID     string // 返回的用户ID
	method     string // 记录调用的方法
	token      string // 记录调用的令牌
}

// Handle 模拟身份验证处理逻辑
func (t *testAuthenticator) Handle(ctx context.Context, method, token string) (string, error) {
	t.method = method
	t.token = token
	if t.shouldFail {
		return "", errors.New("authentication failed")
	}
	return t.userID, nil
}

// testServerStream 是g.ServerStream的测试实现
type testServerStream struct {
	ctx context.Context
}

func (t *testServerStream) SetHeader(md metadata.MD) error {
	return nil
}

func (t *testServerStream) SendHeader(md metadata.MD) error {
	return nil
}

func (t *testServerStream) SetTrailer(md metadata.MD) {
}

func (t *testServerStream) Context() context.Context {
	return t.ctx
}

func (t *testServerStream) SendMsg(msg any) error {
	return nil
}

func (t *testServerStream) RecvMsg(msg any) error {
	return nil
}

// TestExtractUserID 测试从上下文中提取用户ID的功能
func TestExtractUserID(t *testing.T) {
	Convey("TestExtractUserID", t, func() {
		Convey("当上下文包含用户ID时", func() {
			ctx := context.WithValue(context.Background(), ACCESS_KEY_USER_ID, "test-user-123")

			userID := ExtractUserID(ctx)

			Convey("应该能够正确提取用户ID", func() {
				So(userID, ShouldEqual, "test-user-123")
			})
		})

		Convey("当上下文不包含用户ID时", func() {
			ctx := context.Background()

			Convey("提取用户ID应该panic", func() {
				So(func() { ExtractUserID(ctx) }, ShouldPanic)
			})
		})
	})
}

// TestNewAuthRouter 测试创建新的身份验证路由器
func TestNewAuthRouter(t *testing.T) {
	Convey("TestNewAuthRouter", t, func() {
		Convey("使用有效的配置创建路由器", func() {
			config := &AuthServerConfig{
				AccessKeyEnabled: true,
			}

			router := NewAuthRouter(config)

			Convey("应该成功创建路由器", func() {
				So(router, ShouldNotBeNil)
				So(router.config, ShouldEqual, config)
				So(router.authes, ShouldBeNil)
			})
		})

		Convey("使用nil配置创建路由器", func() {
			router := NewAuthRouter(nil)

			Convey("应该成功创建路由器但配置为nil", func() {
				So(router, ShouldNotBeNil)
				So(router.config, ShouldBeNil)
			})
		})
	})
}

// TestAuthRouter_Register 测试注册身份验证器
func TestAuthRouter_Register(t *testing.T) {
	Convey("TestAuthRouter_Register", t, func() {
		router := NewAuthRouter(&AuthServerConfig{})
		testAuth := &testAuthenticator{userID: "test-user"}

		Convey("第一次注册身份验证器", func() {
			authenticators := map[string]Authenticator{
				"test": testAuth,
			}

			err := router.Register(authenticators)

			Convey("应该成功注册", func() {
				So(err, ShouldBeNil)
				So(router.Get("test"), ShouldEqual, testAuth)
			})
		})

		Convey("重复注册身份验证器", func() {
			router := NewAuthRouter(&AuthServerConfig{})
			authenticators1 := map[string]Authenticator{
				"test1": &testAuthenticator{userID: "user1"},
			}
			authenticators2 := map[string]Authenticator{
				"test2": &testAuthenticator{userID: "user2"},
			}

			err1 := router.Register(authenticators1)
			err2 := router.Register(authenticators2)

			Convey("第一次注册应该成功", func() {
				So(err1, ShouldBeNil)
			})

			Convey("第二次注册应该失败", func() {
				So(err2, ShouldNotBeNil)
				So(err2.Error(), ShouldContainSubstring, "auth handlers already registered")
			})
		})
	})
}

// TestAuthRouter_Get 测试获取身份验证器
func TestAuthRouter_Get(t *testing.T) {
	Convey("TestAuthRouter_Get", t, func() {
		router := NewAuthRouter(&AuthServerConfig{})
		testAuth := &testAuthenticator{userID: "test-user"}

		authenticators := map[string]Authenticator{
			"test":  testAuth,
			"other": &testAuthenticator{userID: "other-user"},
		}
		router.Register(authenticators)

		Convey("获取存在的验证器", func() {
			auth := router.Get("test")

			Convey("应该返回正确的验证器", func() {
				So(auth, ShouldEqual, testAuth)
			})
		})

		Convey("获取不存在的验证器", func() {
			auth := router.Get("non-existent")

			Convey("应该返回nil", func() {
				So(auth, ShouldBeNil)
			})
		})

		Convey("获取空名称的验证器", func() {
			auth := router.Get("")

			Convey("应该返回nil", func() {
				So(auth, ShouldBeNil)
			})
		})
	})
}

// TestAuthRouter_auth 测试核心身份验证逻辑
func TestAuthRouter_auth(t *testing.T) {
	Convey("TestAuthRouter_auth", t, func() {
		testAuth := &testAuthenticator{userID: "test-user-456"}
		router := NewAuthRouter(&AuthServerConfig{
			AccessKeyEnabled: true,
		})

		authenticators := map[string]Authenticator{
			ProtocolAccessKeySecret: testAuth,
		}
		router.Register(authenticators)

		Convey("成功的身份验证", func() {
			ctx := metadata.NewIncomingContext(context.Background(), metadata.New(map[string]string{
				":authority":    "example.com:443",
				"authorize":     ProtocolAccessKeySecret,
				"authorization": "test-token-123",
			}))

			userID, err := router.auth(ctx, "/service.Method")

			Convey("应该验证成功", func() {
				So(err, ShouldBeNil)
				So(userID, ShouldEqual, "test-user-456")
				So(testAuth.method, ShouldEqual, "https://example.com:443/service.Method")
				So(testAuth.token, ShouldEqual, "test-token-123")
			})
		})

		Convey("缺少元数据", func() {
			ctx := context.Background()

			userID, err := router.auth(ctx, "/service.Method")

			Convey("应该返回ErrMissingMetadata错误", func() {
				So(err, ShouldEqual, ErrMissingMetadata)
				So(userID, ShouldBeEmpty)
			})
		})

		Convey("缺少:authority字段", func() {
			ctx := metadata.NewIncomingContext(context.Background(), metadata.New(map[string]string{
				"authorize":     ProtocolAccessKeySecret,
				"authorization": "test-token",
			}))

			userID, err := router.auth(ctx, "/service.Method")

			Convey("应该返回ErrBadMetadata错误", func() {
				So(err, ShouldEqual, ErrBadMetadata)
				So(userID, ShouldBeEmpty)
			})
		})

		Convey("缺少authorize字段", func() {
			ctx := metadata.NewIncomingContext(context.Background(), metadata.New(map[string]string{
				":authority":    "example.com",
				"authorization": "test-token",
			}))

			userID, err := router.auth(ctx, "/service.Method")

			Convey("应该返回ErrBadMetadata错误", func() {
				So(err, ShouldEqual, ErrBadMetadata)
				So(userID, ShouldBeEmpty)
			})
		})

		Convey("缺少authorization字段", func() {
			ctx := metadata.NewIncomingContext(context.Background(), metadata.New(map[string]string{
				":authority": "example.com",
				"authorize":  ProtocolAccessKeySecret,
			}))

			userID, err := router.auth(ctx, "/service.Method")

			Convey("应该返回ErrBadMetadata错误", func() {
				So(err, ShouldEqual, ErrBadMetadata)
				So(userID, ShouldBeEmpty)
			})
		})

		Convey("访问密钥认证被禁用", func() {
			router := NewAuthRouter(&AuthServerConfig{
				AccessKeyEnabled: false,
			})
			router.Register(authenticators)

			ctx := metadata.NewIncomingContext(context.Background(), metadata.New(map[string]string{
				":authority":    "example.com",
				"authorize":     ProtocolAccessKeySecret,
				"authorization": "test-token",
			}))

			userID, err := router.auth(ctx, "/service.Method")

			Convey("应该返回空用户ID且无错误", func() {
				So(err, ShouldBeNil)
				So(userID, ShouldBeEmpty)
			})
		})

		Convey("无效的认证类型", func() {
			ctx := metadata.NewIncomingContext(context.Background(), metadata.New(map[string]string{
				":authority":    "example.com",
				"authorize":     "invalid-auth-type",
				"authorization": "test-token",
			}))

			userID, err := router.auth(ctx, "/service.Method")

			Convey("应该返回ErrInvalidAuthType错误", func() {
				So(err, ShouldEqual, ErrInvalidAuthType)
				So(userID, ShouldBeEmpty)
			})
		})

		Convey("验证器处理失败", func() {
			failingAuth := &testAuthenticator{shouldFail: true}
			router := NewAuthRouter(&AuthServerConfig{
				AccessKeyEnabled: true,
			})
			router.Register(map[string]Authenticator{
				ProtocolAccessKeySecret: failingAuth,
			})

			ctx := metadata.NewIncomingContext(context.Background(), metadata.New(map[string]string{
				":authority":    "example.com",
				"authorize":     ProtocolAccessKeySecret,
				"authorization": "test-token",
			}))

			userID, err := router.auth(ctx, "/service.Method")

			Convey("应该返回验证器错误", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "authentication failed")
				So(userID, ShouldBeEmpty)
			})
		})
	})
}

// TestAuthRouter_authUnary 测试单元式RPC身份验证拦截器
func TestAuthRouter_authUnary(t *testing.T) {
	Convey("TestAuthRouter_authUnary", t, func() {
		testAuth := &testAuthenticator{userID: "test-user-789"}
		router := NewAuthRouter(&AuthServerConfig{
			AccessKeyEnabled: true,
		})
		router.Register(map[string]Authenticator{
			ProtocolAccessKeySecret: testAuth,
		})

		Convey("成功的单元式RPC验证", func() {
			ctx := metadata.NewIncomingContext(context.Background(), metadata.New(map[string]string{
				":authority":    "example.com",
				"authorize":     ProtocolAccessKeySecret,
				"authorization": "unary-token",
			}))

			handler := func(ctx context.Context, req any) (any, error) {
				userID := ExtractUserID(ctx)
				return userID, nil
			}

			info := &g.UnaryServerInfo{
				FullMethod: "/test.Service/UnaryMethod",
			}

			result, err := router.authUnary(ctx, "test-request", info, handler)

			Convey("应该验证成功并传递用户ID", func() {
				So(err, ShouldBeNil)
				So(result, ShouldEqual, "test-user-789")
				So(testAuth.method, ShouldEqual, "https://example.com/test.Service/UnaryMethod")
				So(testAuth.token, ShouldEqual, "unary-token")
			})
		})

		Convey("单元式RPC验证失败", func() {
			ctx := context.Background() // 缺少元数据

			handler := func(ctx context.Context, req any) (any, error) {
				return "should-not-be-called", nil
			}

			info := &g.UnaryServerInfo{
				FullMethod: "/test.Service/UnaryMethod",
			}

			result, err := router.authUnary(ctx, "test-request", info, handler)

			Convey("应该返回验证错误", func() {
				So(err, ShouldNotBeNil)
				So(err, ShouldEqual, ErrMissingMetadata)
				So(result, ShouldBeNil)
			})
		})
	})
}

// TestAuthRouter_authStream 测试流式RPC身份验证拦截器
func TestAuthRouter_authStream(t *testing.T) {
	Convey("TestAuthRouter_authStream", t, func() {
		testAuth := &testAuthenticator{userID: "test-user-999"}
		router := NewAuthRouter(&AuthServerConfig{
			AccessKeyEnabled: true,
		})
		router.Register(map[string]Authenticator{
			ProtocolAccessKeySecret: testAuth,
		})

		Convey("成功的流式RPC验证", func() {
			ctx := metadata.NewIncomingContext(context.Background(), metadata.New(map[string]string{
				":authority":    "example.com",
				"authorize":     ProtocolAccessKeySecret,
				"authorization": "stream-token",
			}))

			// 创建测试的ServerStream
			testStream := &testServerStream{ctx: ctx}

			handlerCalled := false
			handler := func(srv any, stream g.ServerStream) error {
				handlerCalled = true
				// 验证流上下文包含用户ID
				wrapper, ok := stream.(*WrapperServerStream)
				So(ok, ShouldBeTrue)
				So(wrapper, ShouldNotBeNil)
				return nil
			}

			info := &g.StreamServerInfo{
				FullMethod:     "/test.Service/StreamMethod",
				IsServerStream: true,
			}

			err := router.authStream(nil, testStream, info, handler)

			Convey("应该验证成功并调用处理器", func() {
				So(err, ShouldBeNil)
				So(handlerCalled, ShouldBeTrue)
				So(testAuth.method, ShouldEqual, "https://example.com/test.Service/StreamMethod")
				So(testAuth.token, ShouldEqual, "stream-token")
			})
		})

		Convey("流式RPC验证失败", func() {
			ctx := context.Background() // 缺少元数据
			testStream := &testServerStream{ctx: ctx}

			handlerCalled := false
			handler := func(srv any, stream g.ServerStream) error {
				handlerCalled = true
				return nil
			}

			info := &g.StreamServerInfo{
				FullMethod:     "/test.Service/StreamMethod",
				IsServerStream: true,
			}

			err := router.authStream(nil, testStream, info, handler)

			Convey("应该返回验证错误且不调用处理器", func() {
				So(err, ShouldNotBeNil)
				So(err, ShouldEqual, ErrMissingMetadata)
				So(handlerCalled, ShouldBeFalse)
			})
		})
	})
}
