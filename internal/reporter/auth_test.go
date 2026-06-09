package reporter

import (
	"context"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/depend"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/grpc"
)

// TestACCESS_KEY_PREFIX 测试 ACCESS_KEY_PREFIX 常量的值是否正确
func TestACCESS_KEY_PREFIX(t *testing.T) {
	Convey("测试 ACCESS_KEY_PREFIX 常量", t, func() {
		Convey("常量值应为 'akp'", func() {
			So(ACCESS_KEY_PREFIX, ShouldEqual, "akp")
		})

		Convey("常量不应为空字符串", func() {
			So(ACCESS_KEY_PREFIX, ShouldNotBeEmpty)
		})

		Convey("常量长度应为 3", func() {
			So(len(ACCESS_KEY_PREFIX), ShouldEqual, 3)
		})
	})
}

// TestReporterAuthRouterFxIn 测试 ReporterAuthRouterFxIn 结构体
func TestReporterAuthRouterFxIn(t *testing.T) {
	Convey("测试 ReporterAuthRouterFxIn 结构体", t, func() {
		Convey("结构体应正确嵌入依赖注入字段", func() {
			in := ReporterAuthRouterFxIn{}

			// 验证结构体可以正确创建
			So(in, ShouldNotBeNil)

			// 验证字段存在性
			So(in.CoreDatabaseFxIn, ShouldNotBeNil)
			So(in.CacheRedisFxIn, ShouldNotBeNil)
		})

		Convey("结构体字段应具有正确的类型", func() {
			in := ReporterAuthRouterFxIn{}

			// 验证字段类型
			var _ depend.CoreDatabaseFxIn = in.CoreDatabaseFxIn
			var _ depend.CacheRedisFxIn = in.CacheRedisFxIn

			// 如果编译通过，说明类型正确
			So(true, ShouldBeTrue)
		})
	})
}

// TestNewReporterAuthenticators 测试 NewReporterAuthenticators 函数的主要功能
func TestNewReporterAuthenticators(t *testing.T) {
	Convey("测试 NewReporterAuthenticators 函数", t, func() {
		Convey("当提供有效的依赖时", func() {
			// 创建依赖注入输入（使用 nil 值，因为这是单元测试）
			in := ReporterAuthRouterFxIn{
				CoreDatabaseFxIn: depend.CoreDatabaseFxIn{
					CoreDB: &database.DB{},
				},
				CacheRedisFxIn: depend.CacheRedisFxIn{
					CacheRedis: nil, // Redis 客户端可以为 nil
				},
			}

			Convey("应成功创建身份验证器映射", func() {
				authenticators := NewReporterAuthenticators(in)

				So(authenticators, ShouldNotBeNil)
				So(len(authenticators), ShouldEqual, 1)
			})

			Convey("映射应包含 ProtocolAccessKeySecret 键", func() {
				authenticators := NewReporterAuthenticators(in)

				_, exists := authenticators[grpc.ProtocolAccessKeySecret]
				So(exists, ShouldBeTrue)
			})

			Convey("ProtocolAccessKeySecret 对应的值应为非 nil 的身份验证器", func() {
				authenticators := NewReporterAuthenticators(in)

				authenticator := authenticators[grpc.ProtocolAccessKeySecret]
				So(authenticator, ShouldNotBeNil)
			})
		})

		Convey("当依赖为 nil 时", func() {
			in := ReporterAuthRouterFxIn{}

			Convey("函数应正常执行，不 panic", func() {
				So(func() {
					NewReporterAuthenticators(in)
				}, ShouldNotPanic)
			})

			Convey("应返回非空的验证器映射", func() {
				authenticators := NewReporterAuthenticators(in)
				So(authenticators, ShouldNotBeNil)
			})

			Convey("映射应包含一个身份验证器", func() {
				authenticators := NewReporterAuthenticators(in)
				So(len(authenticators), ShouldEqual, 1)
			})
		})

		Convey("当数据库连接为 nil 时", func() {
			in := ReporterAuthRouterFxIn{
				CoreDatabaseFxIn: depend.CoreDatabaseFxIn{
					CoreDB: nil,
				},
				CacheRedisFxIn: depend.CacheRedisFxIn{
					CacheRedis: nil,
				},
			}

			Convey("函数应正常执行", func() {
				authenticators := NewReporterAuthenticators(in)
				So(authenticators, ShouldNotBeNil)
			})
		})
	})
}

// TestAccessKeyPoolCreation 测试访问密钥池的创建
func TestAccessKeyPoolCreation(t *testing.T) {
	Convey("测试访问密钥池的创建逻辑", t, func() {
		in := ReporterAuthRouterFxIn{
			CoreDatabaseFxIn: depend.CoreDatabaseFxIn{
				CoreDB: &database.DB{},
			},
			CacheRedisFxIn: depend.CacheRedisFxIn{
				CacheRedis: nil,
			},
		}

		Convey("应创建有效的身份验证器", func() {
			authenticators := NewReporterAuthenticators(in)
			authenticator := authenticators[grpc.ProtocolAccessKeySecret]

			So(authenticator, ShouldNotBeNil)

			// 测试验证器是否实现了 Authenticator 接口
			_, ok := authenticator.(grpc.Authenticator)
			So(ok, ShouldBeTrue)
		})

		Convey("验证器应能响应身份验证请求", func() {
			authenticators := NewReporterAuthenticators(in)
			authenticator := authenticators[grpc.ProtocolAccessKeySecret]

			// 创建一个测试上下文
			ctx := context.Background()

			// 测试验证器的 Handle 方法
			// 由于使用了 nil 依赖，验证会失败，但我们验证了接口调用
			_, err := authenticator.Handle(ctx, "/test.Service/Method", "test-token")

			// 验证会返回错误，因为依赖是 nil
			So(err, ShouldNotBeNil)
		})
	})
}

// TestAuthenticatorMapStructure 测试验证器映射的结构
func TestAuthenticatorMapStructure(t *testing.T) {
	Convey("测试验证器映射的结构特性", t, func() {
		in := ReporterAuthRouterFxIn{
			CoreDatabaseFxIn: depend.CoreDatabaseFxIn{
				CoreDB: &database.DB{},
			},
			CacheRedisFxIn: depend.CacheRedisFxIn{
				CacheRedis: nil,
			},
		}

		authenticators := NewReporterAuthenticators(in)

		Convey("映射应包含正确的协议键", func() {
			for protocol := range authenticators {
				Convey("协议 '"+protocol+"' 应为非空字符串", func() {
					So(protocol, ShouldNotBeEmpty)
					So(protocol, ShouldEqual, grpc.ProtocolAccessKeySecret)
				})
			}
		})

		Convey("映射值应为有效的身份验证器", func() {
			for protocol, authenticator := range authenticators {
				Convey("协议 '"+protocol+"' 的验证器应非空", func() {
					So(authenticator, ShouldNotBeNil)
				})

				Convey("协议 '"+protocol+"' 的验证器应实现 Authenticator 接口", func() {
					_, ok := authenticator.(grpc.Authenticator)
					So(ok, ShouldBeTrue)
				})
			}
		})

		Convey("映射不应为 nil", func() {
			So(authenticators, ShouldNotBeNil)
		})

		Convey("映射应恰好包含一个条目", func() {
			So(len(authenticators), ShouldEqual, 1)
		})
	})
}

// TestNewReporterAuthenticators_Parallel 测试并发安全性
func TestNewReporterAuthenticators_Parallel(t *testing.T) {
	Convey("测试 NewReporterAuthenticators 的并发安全性", t, func() {
		in := ReporterAuthRouterFxIn{
			CoreDatabaseFxIn: depend.CoreDatabaseFxIn{
				CoreDB: &database.DB{},
			},
			CacheRedisFxIn: depend.CacheRedisFxIn{
				CacheRedis: nil,
			},
		}

		Convey("应支持并发调用", func() {
			done := make(chan bool, 10)
			errors := make(chan error, 10)

			for i := 0; i < 10; i++ {
				go func(id int) {
					defer func() {
						if r := recover(); r != nil {
							errors <- r.(error)
						}
						done <- true
					}()

					// 并发调用函数，不使用 GoConvey 断言
					authenticators := NewReporterAuthenticators(in)
					if authenticators == nil {
						errors <- fmt.Errorf("goroutine %d: authenticators is nil", id)
						return
					}
					if len(authenticators) != 1 {
						errors <- fmt.Errorf("goroutine %d: expected 1 authenticator, got %d", id, len(authenticators))
						return
					}

					authenticator := authenticators[grpc.ProtocolAccessKeySecret]
					if authenticator == nil {
						errors <- fmt.Errorf("goroutine %d: authenticator is nil", id)
						return
					}
				}(i)
			}

			// 等待所有 goroutine 完成
			for i := 0; i < 10; i++ {
				<-done
			}

			// 验证没有发生错误
			select {
			case <-errors:
				// 如果有错误，测试失败
				t.Errorf("并发调用发生错误")
			default:
				// 没有错误，测试通过
			}
		})

		Convey("每次调用应返回有效的映射实例", func() {
			auth1 := NewReporterAuthenticators(in)
			auth2 := NewReporterAuthenticators(in)

			// 验证两个映射都有效
			So(auth1, ShouldNotBeNil)
			So(auth2, ShouldNotBeNil)

			// 验证它们包含相同数量的条目
			So(len(auth1), ShouldEqual, len(auth2))

			// 验证它们都包含相同的协议键
			So(auth1[grpc.ProtocolAccessKeySecret], ShouldNotBeNil)
			So(auth2[grpc.ProtocolAccessKeySecret], ShouldNotBeNil)

			// 注意：Go 中的映射不能直接比较，所以我们验证它们的内容相同
			// 而不是验证它们是不同的实例
		})
	})
}

// BenchmarkNewReporterAuthenticators 基准测试
func BenchmarkNewReporterAuthenticators(b *testing.B) {
	in := ReporterAuthRouterFxIn{
		CoreDatabaseFxIn: depend.CoreDatabaseFxIn{
			CoreDB: &database.DB{},
		},
		CacheRedisFxIn: depend.CacheRedisFxIn{
			CacheRedis: nil,
		},
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = NewReporterAuthenticators(in)
	}
}
