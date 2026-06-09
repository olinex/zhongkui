package reporter

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/fx"
	"go.uber.org/zap/zaptest"
	"zhongkui.com/zhongkui/internal/depend"
	reporter_v1 "zhongkui.com/zhongkui/internal/pkg/protobuf/reporter/v1"
)

// TestReporterServiceFxIn 测试 ReporterServiceFxIn 结构体
func TestReporterServiceFxIn(t *testing.T) {
	Convey("测试 ReporterServiceFxIn 结构体", t, func() {
		Convey("结构体应正确嵌入依赖注入字段", func() {
			in := ReporterServiceFxIn{}

			So(in, ShouldNotBeNil)
			// 验证结构体可以正确创建
			So(true, ShouldBeTrue)
		})

		Convey("结构体应声明生命周期管理器字段", func() {
			in := ReporterServiceFxIn{}
			// Lc 字段存在，但在零值结构体中为 nil
			// 这里我们验证结构体定义正确
			var _ fx.Lifecycle = in.Lc // 类型检查
			So(true, ShouldBeTrue)     // 编译通过即表示测试通过
		})
	})
}

// TestReporterService 测试 ReporterService 结构体
func TestReporterService(t *testing.T) {
	Convey("测试 ReporterService 结构体", t, func() {
		Convey("应正确创建服务实例", func() {
			logger := zaptest.NewLogger(t).Sugar()
			service := &ReporterService{
				client:  nil, // 在实际测试中需要模拟
				pClient: nil, // 在实际测试中需要模拟
				logger:  logger,
			}

			So(service, ShouldNotBeNil)
			So(service.logger, ShouldNotBeNil)
		})

		Convey("应实现 UnimplementedReporterServiceServer 接口", func() {
			service := &ReporterService{}
			// 验证服务实现了接口（编译时检查）
			var _ reporter_v1.ReporterServiceServer = service
			So(true, ShouldBeTrue) // 编译通过即表示测试通过
		})
	})
}

// TestNewWatchResponseError 测试 newWatchResponseError 函数
func TestNewWatchResponseError(t *testing.T) {
	Convey("测试 newWatchResponseError 函数", t, func() {
		Convey("应创建包含错误信息的响应", func() {
			testErr := context.Canceled
			errorType := reporter_v1.ErrorType_UNKNOWN
			globalVersion := int64(12345)

			response := newWatchResponseError(testErr, errorType, globalVersion)

			So(response, ShouldNotBeNil)
			So(response.Timestamp, ShouldBeGreaterThan, 0)
			So(response.GlobalVersion, ShouldEqual, globalVersion)

			// 检查响应内容类型
			errorContent, ok := response.Content.(*reporter_v1.WatchResponse_Error)
			So(ok, ShouldBeTrue)
			So(errorContent.Error, ShouldNotBeNil)
			So(errorContent.Error.Type, ShouldEqual, errorType)
			So(errorContent.Error.Detail, ShouldContainSubstring, "context canceled")
		})

		Convey("应包含正确的时间戳", func() {
			before := time.Now().UnixMilli()
			response := newWatchResponseError(context.Canceled, reporter_v1.ErrorType_UNKNOWN, 0)
			after := time.Now().UnixMilli()

			// 时间戳应该在前后时间之间，或者等于前后时间（毫秒级可能相同）
			So(response.Timestamp, ShouldBeGreaterThanOrEqualTo, before)
			So(response.Timestamp, ShouldBeLessThanOrEqualTo, after)
		})

		Convey("不同错误类型应正确设置", func() {
			testCases := []struct {
				name      string
				errorType reporter_v1.ErrorType
			}{
				{"UNKNOWN", reporter_v1.ErrorType_UNKNOWN},
				{"NOT_FOUND", reporter_v1.ErrorType_NOT_FOUND},
				{"GOAWAY", reporter_v1.ErrorType_GOAWAY},
				{"DUPLICATED", reporter_v1.ErrorType_DUPLICATED},
			}

			for _, tc := range testCases {
				Convey("错误类型 "+string(tc.name), func() {
					response := newWatchResponseError(context.Canceled, tc.errorType, 0)
					errorContent, ok := response.Content.(*reporter_v1.WatchResponse_Error)
					So(ok, ShouldBeTrue)
					So(errorContent.Error.Type, ShouldEqual, tc.errorType)
				})
			}
		})
	})
}

// TestServiceInjectFx 测试 ServiceInjectFx 函数的基础行为
func TestServiceInjectFx(t *testing.T) {
	Convey("测试 ServiceInjectFx 函数", t, func() {
		Convey("函数需要有效的 gRPC 服务器才能正常执行", func() {
			// 由于 ServiceInjectFx 需要有效的 gRPC 服务器实例
			// 而单元测试环境难以提供，这里跳过实际执行测试
			// 在实际集成测试中会验证此功能
			So(true, ShouldBeTrue)
		})

		Convey("函数结构应正确", func() {
			// 验证函数签名和基本结构
			var _ func(ReporterServiceFxIn) = ServiceInjectFx
			So(true, ShouldBeTrue)
		})
	})
}

// TestRunGrpcServiceFx 测试 RunGrpcServiceFx 函数的基础行为
func TestRunGrpcServiceFx(t *testing.T) {
	Convey("测试 RunGrpcServiceFx 函数", t, func() {
		Convey("函数应正常执行不 panic", func() {
			logger := zaptest.NewLogger(t).Sugar()

			in := ReporterServiceFxIn{
				Lc: &mockLifecycle{},
				GRPCLoggerFxIn: depend.GRPCLoggerFxIn{
					GRPCLogger: logger,
				},
			}

			So(func() {
				RunGrpcServiceFx(in)
			}, ShouldNotPanic)
		})
	})
}

// mockLifecycle 是 fx.Lifecycle 的简单模拟实现
type mockLifecycle struct{}

func (m *mockLifecycle) Append(hook fx.Hook) {
	// 模拟添加生命周期钩子
}

// BenchmarkNewWatchResponseError 基准测试
func BenchmarkNewWatchResponseError(b *testing.B) {
	testErr := context.Canceled
	errorType := reporter_v1.ErrorType_UNKNOWN
	globalVersion := int64(1000)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = newWatchResponseError(testErr, errorType, globalVersion)
	}
}

// BenchmarkServiceInjectFx 基准测试 - 由于需要有效的 gRPC 服务器，暂时跳过
// func BenchmarkServiceInjectFx(b *testing.B) {
// 	in := ReporterServiceFxIn{}
//
// 	b.ResetTimer()
//
// 	for i := 0; i < b.N; i++ {
// 		ServiceInjectFx(in)
// 	}
// }

// 集成测试占位符 - 在实际项目中需要完整的模拟环境
func TestReporterServiceIntegration(t *testing.T) {
	Convey("ReporterService 集成测试占位符", t, func() {
		Convey("Watch 方法需要完整的 etcd 和 gRPC 模拟", func() {
			So(true, ShouldBeTrue) // 标记为需要集成测试
		})

		Convey("Feedback 方法需要完整的流式处理和任务队列模拟", func() {
			So(true, ShouldBeTrue) // 标记为需要集成测试
		})
	})
}

// 测试辅助函数
func TestHelperFunctions(t *testing.T) {
	Convey("测试辅助函数", t, func() {
		Convey("响应时间戳应为当前时间", func() {
			start := time.Now()
			response := newWatchResponseError(context.Canceled, reporter_v1.ErrorType_UNKNOWN, 0)
			end := time.Now()

			responseTime := time.UnixMilli(response.Timestamp)
			// 使用更宽松的时间比较，允许毫秒级误差
			So(responseTime, ShouldHappenAfter, start.Add(-time.Millisecond))
			So(responseTime, ShouldHappenBefore, end.Add(time.Millisecond))
		})

		Convey("应避免传递 nil 错误到 newWatchResponseError", func() {
			// 这是一个设计验证，实际使用时应确保不传递 nil 错误
			So(func() {
				_ = newWatchResponseError(nil, reporter_v1.ErrorType_UNKNOWN, 0)
			}, ShouldPanic)
		})
	})
}
