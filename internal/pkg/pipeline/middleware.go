// Package pipeline 提供基于 asynq 的任务队列和调度器功能。
// 该包包含了任务队列客户端、服务器、调度器、配置管理和中间件等功能模块，
// 用于构建分布式任务处理系统。
package pipeline

import (
	"context"
	"runtime/debug"

	"github.com/hibiken/asynq"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// RecoverWithStack 创建一个 panic 恢复中间件，用于捕获任务执行过程中的 panic 并记录调用栈。
// 该中间件主要功能：
// 1. 捕获任务执行过程中的 panic
// 2. 记录 panic 信息、任务元数据（ID 和类型）以及完整的调用栈
// 3. 记录任务执行过程中的错误信息
// 4. 为所有错误添加堆栈跟踪信息（根据环境变量配置）
//
// 参数：
//
//	logger: *logging.Logger - 日志记录器实例，用于记录 panic 和错误信息
//
// 返回值：
//
//	func(next asynq.Handler) asynq.Handler - 中间件函数，接受下一个处理器并返回包装后的处理器
//
// 使用场景：
// 1. 生产环境：捕获未预期的 panic，防止任务队列服务器崩溃
// 2. 开发环境：提供详细的调试信息，便于定位问题
// 3. 监控告警：通过日志系统监控任务执行异常
//
// 注意事项：
// 1. 该中间件应该在任务处理器链的最外层使用
// 2. 在 server.go 中可以看到，该中间件仅在 DEBUG_ENV_VAR 为 true 时启用
// 3. panic 被捕获后，任务会正常结束，不会影响其他任务的执行
// 4. 错误信息会通过 errors.WithStack 添加堆栈跟踪（根据 ERROR_STACK_ENV_VAR 配置）
//
// 示例用法：
//
//	// 在任务服务器初始化时使用中间件
//	mux := asynq.NewServeMux()
//	if base.DEBUG_ENV_VAR {
//	    mux.Use(RecoverWithStack(logger))
//	}
//	mux.HandleFunc("email:send", emailHandler)
//
// 日志输出示例：
// 1. Panic 日志：Task [id:abc123, type:email:send] panic with: runtime error: invalid memory address and stack: goroutine 1 [running]: ...
// 2. Error 日志：Task [id:def456, type:image:process] raise error: failed to process image: file not found
func RecoverWithStack(logger *logging.Logger) func(next asynq.Handler) asynq.Handler {
	return func(next asynq.Handler) asynq.Handler {
		// 返回一个新的 asynq.Handler，包装了 panic 恢复和错误记录逻辑
		return asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
			// 获取任务结果写入器，用于获取任务 ID
			writer := task.ResultWriter()

			// 使用 defer 和 recover 机制捕获 panic
			defer func() {
				// 捕获 panic，防止任务执行过程中的异常导致服务器崩溃
				if r := recover(); r != nil {
					// 记录 panic 信息，包括任务元数据和完整的调用栈
					// 任务 ID 和类型有助于定位具体是哪个任务出现了问题
					// debug.Stack() 提供完整的 goroutine 调用栈信息
					taskID := "unknown"
					if writer != nil {
						taskID = writer.TaskID()
					}
					logger.Errorf(
						"Task [id:%s, type:%s] panic with: %v and stack: %s",
						taskID,        // 任务唯一标识符，如果 writer 为 nil 则使用 "unknown"
						task.Type(),   // 任务类型（处理器模式）
						r,             // panic 的值
						debug.Stack(), // 完整的调用栈信息
					)
					// 注意：这里没有重新 panic，任务会正常结束
					// 这是为了防止单个任务的异常影响整个任务队列系统
				}
			}()

			// 执行原始任务处理器
			// 这是实际业务逻辑执行的地方
			err := next.ProcessTask(ctx, task)

			// 如果任务执行过程中出现错误（非 panic），记录错误信息
			// 与 panic 不同，这里的错误是任务处理器显式返回的错误
			if err != nil {
				// 记录任务执行错误，使用 %+v 格式输出完整的错误信息
				// 包括可能的堆栈跟踪（如果 errors.WithStack 被调用）
				taskID := "unknown"
				if writer != nil {
					taskID = writer.TaskID()
				}
				logger.Errorf(
					"Task [id:%s, type:%s] raise error: %+v",
					taskID,      // 任务唯一标识符，如果 writer 为 nil 则使用 "unknown"
					task.Type(), // 任务类型
					err,         // 错误对象
				)
			}

			// 返回错误，使用 errors.WithStack 包装以添加堆栈跟踪
			// 根据 base.ERROR_STACK_ENV_VAR 配置决定是否真正添加堆栈
			// 这样可以统一错误处理，便于调试和问题追踪
			return errors.WithStack(err)
		})
	}
}
