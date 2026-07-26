package pipeline

import (
	"context"

	"github.com/reactivex/rxgo/v2"
)

const (
	// CONTEXT_HANDLER_PARAM_NAME 上下文中的任务配置键名
	// 用于存储任务上下文中的任务配置
	CONTEXT_HANDLER_PARAM_NAME = "ctx_handler_param"
)

// LiveFunc 数据处理函数的类型定义。
// 用于在数据处理过程执行前后的自定义逻辑。
type LiveFunc = func(ctx context.Context, task *Task) error

// ExtractFunc 数据提取函数的类型定义。
// 从任务中提取数据并通过 channel 输出。
type ExtractFunc = func(ctx context.Context, task *Task, channel chan<- rxgo.Item) error

// TransformFunc 数据转换函数的类型定义。
// 对输入数据进行转换并返回转换后的结果。
type TransformFunc = func(ctx context.Context, data any) (any, error)

// LoadFunc 数据加载函数的类型定义。
// 将数据项批量加载到目标存储（如数据库）。
type LoadFunc = func(ctx context.Context, items []any) error

// Handler 任务处理器的接口定义。
// 所有 worker 实现必须实现 Do 方法来执行任务。
type Handler interface {
	Do(ctx context.Context, task *Task) error
}

var handlerFuncs HandlerFuncs = make(HandlerFuncs, 0)

// HandlerRegister 将任务 worker 注册到全局 handler 注册表中。
// 通过任务类型名称关联对应的处理函数。
func HandlerRegister(name string, handler Handler) {
	handlerFuncs[name] = handler.Do
}

// GetHandlerParam 从上下文中获取任务处理器配置。
func GetHandlerParam[T any](ctx context.Context) (*T, error) {
	return getParam[T](ctx, CONTEXT_HANDLER_PARAM_NAME)
}

// HandlerRegistry 获取全局 handler 注册表。
// 返回所有已注册的任务处理函数映射。
func HandlerRegistry(params map[string]map[string]any) HandlerFuncs {
	wrapped := make(HandlerFuncs, len(handlerFuncs))
	for name, hFunc := range handlerFuncs {
		if param, ok := params[name]; !ok {
			wrapped[name] = hFunc
		} else {
			wrapped[name] = func(ctx context.Context, task *Task) error {
				ctx = context.WithValue(ctx, CONTEXT_HANDLER_PARAM_NAME, param)
				return hFunc(ctx, task)
			}
		}
	}
	return wrapped
}
