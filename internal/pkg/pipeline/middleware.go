// Package pipeline 提供基于 asynq 的任务队列和调度器功能。
// 该包包含了任务队列客户端、服务器、调度器、配置管理和中间件等功能模块，
// 用于构建分布式任务处理系统。
package pipeline

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/hibiken/asynq"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// RecoverWithStack 创建一个 panic 恢复中间件，用于捕获任务执行过程中的 panic 并记录调用栈。
// 该中间件主要功能：
// 1. 捕获任务执行过程中的 panic
// 2. 记录 panic 信息、任务元数据（ID 和类型）以及完整的调用栈
// 3. 记录任务执行过程中的错误信息（含完整堆栈）
// 4. 将包含堆栈的错误返回给 asynq，使其 LastErr 字段可查看到堆栈信息
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
// 4. 错误回溯：通过 asynq inspector 接口的 LastErr 字段查看完整堆栈
//
// 注意事项：
// 1. 该中间件应该在任务处理器链的最外层使用
// 2. 始终启用，确保生产环境也能捕获错误和 panic
// 3. panic 被捕获后，任务会正常结束，不会影响其他任务的执行
// 4. 返回给 asynq 的错误会包含完整堆栈，可通过 ETL inspector 接口查看
func RecoverWithStack(logger *logging.Logger) func(next asynq.Handler) asynq.Handler {
	return func(next asynq.Handler) asynq.Handler {
		return asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
			writer := task.ResultWriter()

			taskID := "unknown"
			if writer != nil {
				taskID = writer.TaskID()
			}
			taskType := task.Type()

			// panic 恢复
			defer func() {
				if r := recover(); r != nil {
					stack := string(debug.Stack())
					logger.Ctx(ctx).Errorf(
						"Task [id:%s, type:%s] panic with: %v and stack: %s",
						taskID, taskType, r, stack,
					)
				}
			}()

			err := next.ProcessTask(ctx, task)

			if err != nil {
				// 日志记录完整堆栈信息
				logger.Ctx(ctx).Errorf(
					"Task [id:%s, type:%s] raise error: %+v",
					taskID, taskType, err,
				)
				// 将包含堆栈的错误返回给 asynq，
				// asynq 会通过 err.Error() 将其存入 LastErr 字段，
				// 从而在 ETL inspector 接口中可直接查看完整堆栈
				return fmt.Errorf("%+v", errors.WithStack(err))
			}

			return nil
		})
	}
}
