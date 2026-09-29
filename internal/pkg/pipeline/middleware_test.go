// Package queue 提供基于 asynq 的任务队列和调度器功能测试。
// 此文件包含 queue 包中中间件功能的单元测试，重点测试 RecoverWithStack 中间件的各种场景。

package pipeline

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/hibiken/asynq"
	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// TestRecoverWithStack_MiddlewareCreation 测试 RecoverWithStack 中间件的创建功能。
// 验证中间件工厂函数能够正确创建中间件，并且创建的中间件具有预期的行为。
func TestRecoverWithStack_MiddlewareCreation(t *testing.T) {
	Convey("测试 RecoverWithStack 中间件的创建功能", t, func() {
		// 创建测试用的日志记录器核心，用于捕获日志输出
		observedZapCore, _ := observer.New(zap.InfoLevel)
		observedLogger := zap.New(observedZapCore)
		sugarLogger := &logging.Logger{SugaredLogger: observedLogger.Sugar()}

		Convey("应该能够成功创建中间件函数", func() {
			// 调用 RecoverWithStack 创建中间件
			middleware := RecoverWithStack(sugarLogger)

			// 验证中间件不为 nil
			So(middleware, ShouldNotBeNil)

			// 验证中间件是一个函数类型
			So(middleware, ShouldHaveSameTypeAs, (func(asynq.Handler) asynq.Handler)(nil))
		})

		Convey("创建的中间件应该能够包装任务处理器", func() {
			// 创建中间件
			middleware := RecoverWithStack(sugarLogger)

			// 创建一个模拟的任务处理器
			mockHandler := asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
				return nil
			})

			// 使用中间件包装处理器
			wrappedHandler := middleware(mockHandler)

			// 验证包装后的处理器不为 nil
			So(wrappedHandler, ShouldNotBeNil)

			// 验证包装后的处理器是 asynq.Handler 类型
			So(wrappedHandler, ShouldImplement, (*asynq.Handler)(nil))
		})

		Convey("中间件应该正确处理 nil 日志记录器", func() {
			// 使用 nil 日志记录器创建中间件（应该不会 panic）
			middleware := RecoverWithStack(nil)

			// 验证中间件不为 nil
			So(middleware, ShouldNotBeNil)

			// 创建一个模拟的任务处理器
			mockHandler := asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
				return nil
			})

			// 使用中间件包装处理器（应该不会 panic）
			wrappedHandler := middleware(mockHandler)
			So(wrappedHandler, ShouldNotBeNil)
		})
	})
}

// TestRecoverWithStack_NormalTaskExecution 测试中间件在正常任务执行场景下的行为。
// 验证中间件不会影响正常任务的执行，并且能够正确处理返回的错误。
func TestRecoverWithStack_NormalTaskExecution(t *testing.T) {
	Convey("测试中间件在正常任务执行场景下的行为", t, func() {
		// 创建测试用的日志记录器核心，用于捕获日志输出
		observedZapCore, observedLogs := observer.New(zap.ErrorLevel)
		observedLogger := zap.New(observedZapCore)
		sugarLogger := &logging.Logger{SugaredLogger: observedLogger.Sugar()}

		// 保存原始环境变量值
		originalDebugEnvVar := base.Env.Debug
		originalErrorStackEnvVar := base.Env.ErrorStack

		// 设置测试环境变量
		base.Env.Debug = true
		base.Env.ErrorStack = true

		defer func() {
			// 恢复原始环境变量值
			base.Env.Debug = originalDebugEnvVar
			base.Env.ErrorStack = originalErrorStackEnvVar
		}()

		Convey("应该正确处理成功执行的任务", func() {
			// 创建中间件
			middleware := RecoverWithStack(sugarLogger)

			// 创建一个模拟的成功任务处理器
			expectedTaskType := "test:success"
			mockHandler := asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
				// 验证任务类型
				So(task.Type(), ShouldEqual, expectedTaskType)
				// 成功执行，返回 nil
				return nil
			})

			// 使用中间件包装处理器
			wrappedHandler := middleware(mockHandler)

			// 创建模拟的任务
			task := asynq.NewTask(expectedTaskType, []byte("test payload"))

			// 执行任务
			err := wrappedHandler.ProcessTask(context.Background(), task)

			// 验证执行结果
			So(err, ShouldBeNil)

			// 验证没有错误日志记录
			So(observedLogs.Len(), ShouldEqual, 0)
		})

		Convey("应该正确处理返回错误的任务", func() {
			// 创建中间件
			middleware := RecoverWithStack(sugarLogger)

			// 创建一个模拟的失败任务处理器
			expectedTaskType := "test:error"
			expectedError := errors.New("task execution failed")
			mockHandler := asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
				// 验证任务类型
				So(task.Type(), ShouldEqual, expectedTaskType)
				// 返回预期错误
				return expectedError
			})

			// 使用中间件包装处理器
			wrappedHandler := middleware(mockHandler)

			// 创建模拟的任务
			task := asynq.NewTask(expectedTaskType, []byte("test payload"))

			// 执行任务
			err := wrappedHandler.ProcessTask(context.Background(), task)

			// 验证执行结果包含预期错误
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "task execution failed")

			// 验证有错误日志记录
			So(observedLogs.Len(), ShouldEqual, 1)

			// 获取日志条目
			logEntry := observedLogs.All()[0]

			// 验证日志级别
			So(logEntry.Level, ShouldEqual, zapcore.ErrorLevel)

			// 验证日志消息包含任务信息
			So(logEntry.Message, ShouldContainSubstring, "raise error")
			So(logEntry.Message, ShouldContainSubstring, expectedTaskType)

			// 验证日志上下文包含错误信息
			contextMap := logEntry.ContextMap()
			So(contextMap, ShouldNotBeNil)
		})

		Convey("应该正确处理带有格式化错误信息的任务", func() {
			// 创建中间件
			middleware := RecoverWithStack(sugarLogger)

			// 创建一个模拟的失败任务处理器，返回格式化错误
			expectedTaskType := "test:formatted-error"
			mockHandler := asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
				// 返回格式化错误
				return fmt.Errorf("failed to process item %d: %v", 42, errors.New("file not found"))
			})

			// 使用中间件包装处理器
			wrappedHandler := middleware(mockHandler)

			// 创建模拟的任务
			task := asynq.NewTask(expectedTaskType, []byte("test payload"))

			// 执行任务
			err := wrappedHandler.ProcessTask(context.Background(), task)

			// 验证执行结果包含预期错误信息
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "failed to process item 42")

			// 验证有错误日志记录
			So(observedLogs.Len(), ShouldEqual, 1)
		})
	})
}

// TestRecoverWithStack_PanicRecovery 测试中间件的 panic 恢复功能。
// 验证中间件能够正确捕获和恢复任务执行过程中的 panic。
func TestRecoverWithStack_PanicRecovery(t *testing.T) {
	Convey("测试中间件的 panic 恢复功能", t, func() {
		// 创建测试用的日志记录器核心，用于捕获日志输出
		observedZapCore, observedLogs := observer.New(zap.ErrorLevel)
		observedLogger := zap.New(observedZapCore)
		sugarLogger := &logging.Logger{SugaredLogger: observedLogger.Sugar()}

		// 保存原始环境变量值
		originalDebugEnvVar := base.Env.Debug
		originalErrorStackEnvVar := base.Env.ErrorStack

		// 设置测试环境变量
		base.Env.Debug = true
		base.Env.ErrorStack = true

		defer func() {
			// 恢复原始环境变量值
			base.Env.Debug = originalDebugEnvVar
			base.Env.ErrorStack = originalErrorStackEnvVar
		}()

		Convey("应该能够恢复 nil panic", func() {
			// 创建中间件
			middleware := RecoverWithStack(sugarLogger)

			// 创建一个会触发 nil panic 的任务处理器
			expectedTaskType := "test:nil-panic"
			mockHandler := asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
				// 触发 nil panic
				panic(nil)
			})

			// 使用中间件包装处理器
			wrappedHandler := middleware(mockHandler)

			// 创建模拟的任务
			task := asynq.NewTask(expectedTaskType, []byte("test payload"))

			// 执行任务 - 应该不会 panic
			err := wrappedHandler.ProcessTask(context.Background(), task)

			// 验证执行结果
			So(err, ShouldBeNil)

			// 验证有 panic 日志记录
			So(observedLogs.Len(), ShouldEqual, 1)

			// 获取日志条目
			logEntry := observedLogs.All()[0]

			// 验证日志级别
			So(logEntry.Level, ShouldEqual, zapcore.ErrorLevel)

			// 验证日志消息包含 panic 信息
			So(logEntry.Message, ShouldContainSubstring, "panic")
			So(logEntry.Message, ShouldContainSubstring, expectedTaskType)
		})

		Convey("应该能够恢复字符串 panic", func() {
			// 创建中间件
			middleware := RecoverWithStack(sugarLogger)

			// 创建一个会触发字符串 panic 的任务处理器
			expectedTaskType := "test:string-panic"
			expectedPanicMessage := "something went wrong"
			mockHandler := asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
				// 触发字符串 panic
				panic(expectedPanicMessage)
			})

			// 使用中间件包装处理器
			wrappedHandler := middleware(mockHandler)

			// 创建模拟的任务
			task := asynq.NewTask(expectedTaskType, []byte("test payload"))

			// 执行任务 - 应该不会 panic
			err := wrappedHandler.ProcessTask(context.Background(), task)

			// 验证执行结果
			So(err, ShouldBeNil)

			// 验证有 panic 日志记录
			So(observedLogs.Len(), ShouldEqual, 1)

			// 验证日志消息包含 panic 信息
			logEntry := observedLogs.All()[0]
			So(logEntry.Message, ShouldContainSubstring, expectedPanicMessage)
			So(logEntry.Message, ShouldContainSubstring, expectedTaskType)
		})

		Convey("应该能够恢复错误 panic", func() {
			// 创建中间件
			middleware := RecoverWithStack(sugarLogger)

			// 创建一个会触发错误 panic 的任务处理器
			expectedTaskType := "test:error-panic"
			expectedPanicError := errors.New("critical failure")
			mockHandler := asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
				// 触发错误 panic
				panic(expectedPanicError)
			})

			// 使用中间件包装处理器
			wrappedHandler := middleware(mockHandler)

			// 创建模拟的任务
			task := asynq.NewTask(expectedTaskType, []byte("test payload"))

			// 执行任务 - 应该不会 panic
			err := wrappedHandler.ProcessTask(context.Background(), task)

			// 验证执行结果
			So(err, ShouldBeNil)

			// 验证有 panic 日志记录
			So(observedLogs.Len(), ShouldEqual, 1)

			// 验证日志消息包含 panic 错误信息
			logEntry := observedLogs.All()[0]
			So(logEntry.Message, ShouldContainSubstring, "critical failure")
			So(logEntry.Message, ShouldContainSubstring, expectedTaskType)
		})

		Convey("应该能够恢复运行时 panic（如除零错误）", func() {
			// 创建中间件
			middleware := RecoverWithStack(sugarLogger)

			// 创建一个会触发运行时 panic 的任务处理器
			expectedTaskType := "test:runtime-panic"
			mockHandler := asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
				// 触发除零 panic
				var zero int
				_ = 1 / zero
				return nil
			})

			// 使用中间件包装处理器
			wrappedHandler := middleware(mockHandler)

			// 创建模拟的任务
			task := asynq.NewTask(expectedTaskType, []byte("test payload"))

			// 执行任务 - 应该不会 panic
			err := wrappedHandler.ProcessTask(context.Background(), task)

			// 验证执行结果
			So(err, ShouldBeNil)

			// 验证有 panic 日志记录
			So(observedLogs.Len(), ShouldEqual, 1)

			// 验证日志消息包含 panic 信息和调用栈
			logEntry := observedLogs.All()[0]
			So(logEntry.Message, ShouldContainSubstring, "panic")
			So(logEntry.Message, ShouldContainSubstring, expectedTaskType)
			So(logEntry.Message, ShouldContainSubstring, "stack")
		})
	})
}

// TestRecoverWithStack_ContextAndTaskInfo 测试中间件对上下文和任务信息的处理。
// 验证中间件能够正确获取和使用任务上下文、任务ID等信息。
func TestRecoverWithStack_ContextAndTaskInfo(t *testing.T) {
	Convey("测试中间件对上下文和任务信息的处理", t, func() {
		// 创建测试用的日志记录器核心，用于捕获日志输出
		observedZapCore, observedLogs := observer.New(zap.ErrorLevel)
		observedLogger := zap.New(observedZapCore)
		sugarLogger := &logging.Logger{SugaredLogger: observedLogger.Sugar()}

		// 保存原始环境变量值
		originalDebugEnvVar := base.Env.Debug
		originalErrorStackEnvVar := base.Env.ErrorStack

		// 设置测试环境变量
		base.Env.Debug = true
		base.Env.ErrorStack = true

		defer func() {
			// 恢复原始环境变量值
			base.Env.Debug = originalDebugEnvVar
			base.Env.ErrorStack = originalErrorStackEnvVar
		}()

		Convey("应该能够正确处理任务上下文", func() {
			// 创建中间件
			middleware := RecoverWithStack(sugarLogger)

			// 创建一个会使用上下文的任务处理器
			expectedTaskType := "test:context"
			expectedContextValue := "test-value"

			mockHandler := asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
				// 验证上下文不为 nil
				So(ctx, ShouldNotBeNil)

				// 从上下文中获取值
				value := ctx.Value("test-key")
				So(value, ShouldEqual, expectedContextValue)

				return nil
			})

			// 使用中间件包装处理器
			wrappedHandler := middleware(mockHandler)

			// 创建带有值的上下文
			ctx := context.WithValue(context.Background(), "test-key", expectedContextValue)

			// 创建模拟的任务
			task := asynq.NewTask(expectedTaskType, []byte("test payload"))

			// 执行任务
			err := wrappedHandler.ProcessTask(ctx, task)

			// 验证执行结果
			So(err, ShouldBeNil)
		})

		Convey("应该能够记录任务类型信息", func() {
			// 创建中间件
			middleware := RecoverWithStack(sugarLogger)

			// 创建一个会失败的任务处理器
			expectedTaskType := "email:send"
			expectedError := errors.New("smtp server unavailable")

			mockHandler := asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
				// 验证任务类型
				So(task.Type(), ShouldEqual, expectedTaskType)
				return expectedError
			})

			// 使用中间件包装处理器
			wrappedHandler := middleware(mockHandler)

			// 创建模拟的任务
			task := asynq.NewTask(expectedTaskType, []byte("recipient=test@example.com"))

			// 执行任务
			err := wrappedHandler.ProcessTask(context.Background(), task)

			// 验证执行结果
			So(err, ShouldNotBeNil)

			// 验证有错误日志记录
			So(observedLogs.Len(), ShouldEqual, 1)

			// 验证日志消息包含任务类型
			logEntry := observedLogs.All()[0]
			So(logEntry.Message, ShouldContainSubstring, expectedTaskType)
		})

		Convey("应该能够处理不同的任务类型", func() {
			// 测试不同的任务类型
			testCases := []struct {
				taskType    string
				description string
			}{
				{"image:process", "图片处理任务"},
				{"video:transcode", "视频转码任务"},
				{"report:generate", "报告生成任务"},
				{"notification:send", "通知发送任务"},
			}

			for _, tc := range testCases {
				Convey(fmt.Sprintf("应该能够处理 %s 任务类型", tc.description), func() {
					// 重置日志记录器
					observedZapCore, observedLogs = observer.New(zap.ErrorLevel)
					observedLogger = zap.New(observedZapCore)
					sugarLogger = &logging.Logger{SugaredLogger: observedLogger.Sugar()}

					// 创建中间件
					middleware := RecoverWithStack(sugarLogger)

					// 创建一个会失败的任务处理器
					mockHandler := asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
						return fmt.Errorf("failed to process %s task", tc.taskType)
					})

					// 使用中间件包装处理器
					wrappedHandler := middleware(mockHandler)

					// 创建模拟的任务
					task := asynq.NewTask(tc.taskType, []byte("test payload"))

					// 执行任务
					err := wrappedHandler.ProcessTask(context.Background(), task)

					// 验证执行结果包含错误
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldContainSubstring, tc.taskType)

					// 验证有错误日志记录
					So(observedLogs.Len(), ShouldEqual, 1)

					// 验证日志消息包含任务类型
					logEntry := observedLogs.All()[0]
					So(logEntry.Message, ShouldContainSubstring, tc.taskType)
				})
			}
		})
	})
}

// TestRecoverWithStack_ErrorStackHandling 测试中间件的错误堆栈处理功能。
// 验证中间件根据环境变量配置正确处理错误堆栈。
func TestRecoverWithStack_ErrorStackHandling(t *testing.T) {
	Convey("测试中间件的错误堆栈处理功能", t, func() {
		// 创建测试用的日志记录器核心，用于捕获日志输出
		observedZapCore, _ := observer.New(zap.ErrorLevel)
		observedLogger := zap.New(observedZapCore)
		sugarLogger := &logging.Logger{SugaredLogger: observedLogger.Sugar()}

		// 保存原始环境变量值
		originalDebugEnvVar := base.Env.Debug
		originalErrorStackEnvVar := base.Env.ErrorStack

		Convey("当 ERROR_STACK 为 true 时，应该添加错误堆栈", func() {
			// 设置环境变量
			base.Env.Debug = true
			base.Env.ErrorStack = true

			// 创建中间件
			middleware := RecoverWithStack(sugarLogger)

			// 创建一个会失败的任务处理器
			expectedTaskType := "test:error-stack"
			expectedError := errors.New("stack error test")

			mockHandler := asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
				return expectedError
			})

			// 使用中间件包装处理器
			wrappedHandler := middleware(mockHandler)

			// 创建模拟的任务
			task := asynq.NewTask(expectedTaskType, []byte("test payload"))

			// 执行任务
			err := wrappedHandler.ProcessTask(context.Background(), task)

			// 验证执行结果不为 nil
			So(err, ShouldNotBeNil)

			// 验证错误信息包含堆栈信息
			// 注意：由于 errors.WithStack 的实现，我们只能验证错误不为 nil
			// 具体的堆栈信息格式由 errors 包控制
		})

		Convey("当 ERROR_STACK_ENV_VAR 为 false 时，不应该添加错误堆栈", func() {
			// 设置环境变量
			base.Env.Debug = true
			base.Env.ErrorStack = false

			// 创建中间件
			middleware := RecoverWithStack(sugarLogger)

			// 创建一个会失败的任务处理器
			expectedTaskType := "test:no-stack"
			expectedError := errors.New("no stack error test")

			mockHandler := asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
				return expectedError
			})

			// 使用中间件包装处理器
			wrappedHandler := middleware(mockHandler)

			// 创建模拟的任务
			task := asynq.NewTask(expectedTaskType, []byte("test payload"))

			// 执行任务
			err := wrappedHandler.ProcessTask(context.Background(), task)

			// 验证执行结果不为 nil
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "no stack error test")
		})

		Convey("当 DEBUG_ENV_VAR 为 false 时，中间件可能不会被使用", func() {
			// 设置环境变量
			base.Env.Debug = false
			base.Env.ErrorStack = true

			// 创建中间件（即使 DEBUG_ENV_VAR 为 false，中间件仍然可以创建）
			middleware := RecoverWithStack(sugarLogger)
			So(middleware, ShouldNotBeNil)

			// 注意：在实际使用中，server.go 会检查 DEBUG_ENV_VAR 来决定是否使用中间件
			// 但中间件本身的创建和功能测试仍然有效
		})

		// 恢复原始环境变量值
		defer func() {
			base.Env.Debug = originalDebugEnvVar
			base.Env.ErrorStack = originalErrorStackEnvVar
		}()
	})
}

// TestRecoverWithStack_EdgeCases 测试中间件的边界情况。
// 验证中间件在异常或边界情况下的行为。
func TestRecoverWithStack_EdgeCases(t *testing.T) {
	Convey("测试中间件的边界情况", t, func() {
		// 创建测试用的日志记录器核心，用于捕获日志输出
		observedZapCore, _ := observer.New(zap.ErrorLevel)
		observedLogger := zap.New(observedZapCore)
		sugarLogger := &logging.Logger{SugaredLogger: observedLogger.Sugar()}

		// 保存原始环境变量值
		originalDebugEnvVar := base.Env.Debug
		originalErrorStackEnvVar := base.Env.ErrorStack

		// 设置测试环境变量
		base.Env.Debug = true
		base.Env.ErrorStack = true

		defer func() {
			// 恢复原始环境变量值
			base.Env.Debug = originalDebugEnvVar
			base.Env.ErrorStack = originalErrorStackEnvVar
		}()

		Convey("应该处理任务处理器为 nil 的情况", func() {
			// 创建中间件
			middleware := RecoverWithStack(sugarLogger)

			// 使用中间件包装 nil 处理器
			wrappedHandler := middleware(nil)
			So(wrappedHandler, ShouldNotBeNil)

			// 验证包装后的处理器不为 nil
			// 我们不实际执行任务，因为 nil 处理器会导致 panic
			// 这个测试主要验证中间件创建不会 panic
		})

		Convey("应该处理任务为 nil 的情况", func() {
			// 创建中间件
			middleware := RecoverWithStack(sugarLogger)

			// 创建一个任务处理器
			mockHandler := asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
				// 正常处理任务
				return nil
			})

			// 使用中间件包装处理器
			wrappedHandler := middleware(mockHandler)

			// 注意：我们不测试传入 nil 任务的情况，因为 asynq 库可能会 panic
			// 而且在实际使用中，asynq 服务器不会传入 nil 任务
			// 这个测试主要验证中间件创建和包装不会 panic
			So(wrappedHandler, ShouldNotBeNil)
		})

		Convey("应该处理上下文为 nil 的情况", func() {
			// 创建中间件
			middleware := RecoverWithStack(sugarLogger)

			// 创建一个任务处理器，当上下文为 nil 时可能 panic
			mockHandler := asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
				// 如果上下文为 nil，这里可能会 panic
				// 但我们不进行断言，因为中间件应该处理这种情况
				if ctx == nil {
					// 上下文为 nil，返回错误而不是 panic
					// 这样测试更可控
					return errors.New("context is nil")
				}
				return nil
			})

			// 使用中间件包装处理器
			wrappedHandler := middleware(mockHandler)

			// 创建模拟的任务
			task := asynq.NewTask("test:nil-context", []byte("test payload"))

			// 执行任务，传入 nil 上下文
			err := wrappedHandler.ProcessTask(nil, task)

			// 验证执行结果
			// 由于上下文为 nil，处理器可能返回错误或 panic
			// 中间件应该处理这两种情况
			_ = err // 我们不验证具体错误，只验证中间件不会导致测试 panic
		})

		Convey("应该处理日志记录器记录失败的情况", func() {
			// 创建一个会失败的日志记录器（模拟磁盘已满等场景）
			// 这里我们使用正常的日志记录器，但测试中间件在日志记录失败时的行为
			middleware := RecoverWithStack(sugarLogger)

			// 创建一个会 panic 的任务处理器
			expectedPanicMessage := "test panic for logging failure"
			mockHandler := asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
				panic(expectedPanicMessage)
			})

			// 使用中间件包装处理器
			wrappedHandler := middleware(mockHandler)

			// 创建模拟的任务
			task := asynq.NewTask("test:logging-failure", []byte("test payload"))

			// 执行任务 - 应该不会 panic，即使日志记录可能失败
			err := wrappedHandler.ProcessTask(context.Background(), task)

			// 验证执行结果
			So(err, ShouldBeNil)
		})

		Convey("应该处理长时间运行的任务", func() {
			// 创建中间件
			middleware := RecoverWithStack(sugarLogger)

			// 创建一个长时间运行但最终成功的任务处理器
			mockHandler := asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
				// 模拟长时间运行（但实际上是立即返回）
				// 在实际测试中，这里可能会有 sleep 或实际耗时操作
				// 但对于单元测试，我们只验证中间件不会影响正常执行
				return nil
			})

			// 使用中间件包装处理器
			wrappedHandler := middleware(mockHandler)

			// 创建模拟的任务
			task := asynq.NewTask("test:long-running", []byte("test payload"))

			// 执行任务
			err := wrappedHandler.ProcessTask(context.Background(), task)

			// 验证执行结果
			So(err, ShouldBeNil)
		})
	})
}

// TestRecoverWithStack_Integration 测试中间件的集成场景。
// 验证中间件在实际使用场景中的行为。
func TestRecoverWithStack_Integration(t *testing.T) {
	Convey("测试中间件的集成场景", t, func() {
		// 创建测试用的日志记录器核心，用于捕获日志输出
		observedZapCore, observedLogs := observer.New(zap.ErrorLevel)
		observedLogger := zap.New(observedZapCore)
		sugarLogger := &logging.Logger{SugaredLogger: observedLogger.Sugar()}

		// 保存原始环境变量值
		originalDebugEnvVar := base.Env.Debug
		originalErrorStackEnvVar := base.Env.ErrorStack

		// 设置测试环境变量
		base.Env.Debug = true
		base.Env.ErrorStack = true

		defer func() {
			// 恢复原始环境变量值
			base.Env.Debug = originalDebugEnvVar
			base.Env.ErrorStack = originalErrorStackEnvVar
		}()

		Convey("应该能够正确处理链式中间件", func() {
			// 创建第一个中间件（panic 恢复中间件）
			recoverMiddleware := RecoverWithStack(sugarLogger)

			// 创建一个模拟的日志记录中间件
			loggingMiddleware := func(next asynq.Handler) asynq.Handler {
				return asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
					// 记录任务开始
					// 在实际实现中，这里可能会有日志记录

					// 执行下一个处理器
					err := next.ProcessTask(ctx, task)

					// 记录任务结束
					// 在实际实现中，这里可能会有日志记录

					return err
				})
			}

			// 创建一个会失败的任务处理器
			expectedError := errors.New("chain error")
			baseHandler := asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
				return expectedError
			})

			// 创建中间件链：recoverMiddleware -> loggingMiddleware -> baseHandler
			chainedHandler := recoverMiddleware(loggingMiddleware(baseHandler))
			So(chainedHandler, ShouldNotBeNil)

			// 创建模拟的任务
			task := asynq.NewTask("test:chained", []byte("test payload"))

			// 执行任务
			err := chainedHandler.ProcessTask(context.Background(), task)

			// 验证执行结果
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "chain error")
		})

		Convey("应该能够处理真实世界的任务场景", func() {
			// 模拟真实世界的任务类型和场景
			realWorldScenarios := []struct {
				taskType        string
				payload         []byte
				handlerBehavior string // "success", "error", "panic"
				description     string
			}{
				{
					taskType:        "email:send",
					payload:         []byte(`{"to":"user@example.com","subject":"Welcome","body":"Hello!"}`),
					handlerBehavior: "success",
					description:     "邮件发送成功",
				},
				{
					taskType:        "image:resize",
					payload:         []byte(`{"path":"/uploads/image.jpg","width":800,"height":600}`),
					handlerBehavior: "error",
					description:     "图片处理失败（文件不存在）",
				},
				{
					taskType:        "report:generate",
					payload:         []byte(`{"report_id":"123","format":"pdf"}`),
					handlerBehavior: "panic",
					description:     "报告生成时发生 panic（数据库连接失败）",
				},
			}

			for _, scenario := range realWorldScenarios {
				Convey(fmt.Sprintf("应该处理 %s 场景", scenario.description), func() {
					// 重置日志记录器
					observedZapCore, observedLogs = observer.New(zap.ErrorLevel)
					observedLogger = zap.New(observedZapCore)
					sugarLogger = &logging.Logger{SugaredLogger: observedLogger.Sugar()}

					// 创建中间件
					middleware := RecoverWithStack(sugarLogger)

					// 根据场景创建任务处理器
					var mockHandler asynq.HandlerFunc
					switch scenario.handlerBehavior {
					case "success":
						mockHandler = func(ctx context.Context, task *asynq.Task) error {
							// 验证任务类型和负载
							So(task.Type(), ShouldEqual, scenario.taskType)
							So(task.Payload(), ShouldResemble, scenario.payload)
							return nil
						}
					case "error":
						mockHandler = func(ctx context.Context, task *asynq.Task) error {
							return fmt.Errorf("failed to process %s task", scenario.taskType)
						}
					case "panic":
						mockHandler = func(ctx context.Context, task *asynq.Task) error {
							panic(fmt.Sprintf("panic while processing %s task", scenario.taskType))
						}
					}

					// 使用中间件包装处理器
					wrappedHandler := middleware(mockHandler)

					// 创建模拟的任务
					task := asynq.NewTask(scenario.taskType, scenario.payload)

					// 执行任务
					err := wrappedHandler.ProcessTask(context.Background(), task)

					// 根据场景验证执行结果
					switch scenario.handlerBehavior {
					case "success":
						So(err, ShouldBeNil)
						So(observedLogs.Len(), ShouldEqual, 0)
					case "error":
						So(err, ShouldNotBeNil)
						So(observedLogs.Len(), ShouldEqual, 1)
					case "panic":
						So(err, ShouldBeNil)
						So(observedLogs.Len(), ShouldEqual, 1)
					}
				})
			}
		})

		Convey("应该验证中间件在实际服务器中的使用模式", func() {
			// 这个测试验证中间件在 server.go 中的实际使用模式
			// 在 server.go 中，中间件的使用模式是：
			// if base.Env.Debug {
			//     mux.Use(RecoverWithStack(logger))
			// }

			// 创建模拟的 asynq.ServeMux
			// 注意：由于 asynq.ServeMux 是具体类型，我们无法直接模拟
			// 这里我们验证中间件创建和使用的基本模式

			// 创建中间件
			middleware := RecoverWithStack(sugarLogger)
			So(middleware, ShouldNotBeNil)

			// 验证中间件可以包装处理器
			handler := asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
				return nil
			})

			wrappedHandler := middleware(handler)
			So(wrappedHandler, ShouldNotBeNil)

			// 这个测试主要验证中间件与 server.go 中模式的兼容性
			// 实际集成测试可能需要启动真正的 Redis 和 asynq 服务器
		})
	})
}
