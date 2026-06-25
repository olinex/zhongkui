package pipeline

import (
	"context"
	"fmt"

	"github.com/creasty/defaults"
	"github.com/mitchellh/mapstructure"
	"github.com/reactivex/rxgo/v2"
	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

const (
	// CONTEXT_SCOPE_NAME 上下文中的注入器键名
	// 用于存储任务上下文中的注入器实例
	CONTEXT_SCOPE_NAME = "ctx_scope"

	// CONTEXT_TASK_PARAM_NAME 上下文中的任务配置键名
	// 用于存储任务上下文中的任务配置
	CONTEXT_TASK_PARAM_NAME = "ctx_task_param"
)

// TaskContext 创建任务上下文的函数。
// 返回一个函数，用于构建包含注入器实例的上下文。
func TaskContext(ctx context.Context, i do.Injector) ContextFunc {
	return func() context.Context {
		suffix, err := crypto.GenerateRandomString(16)
		if err != nil {
			panic(err)
		}
		scope := i.Scope(fmt.Sprintf("task:%s", suffix))
		return context.WithValue(ctx, CONTEXT_SCOPE_NAME, scope)
	}
}

// GetScope 从上下文中获取注入器实例。
func GetScope(ctx context.Context) *do.Scope {
	return ctx.Value(CONTEXT_SCOPE_NAME).(*do.Scope)
}

// GetTaskParam 从上下文中获取任务配置。
func GetTaskParam[T any](ctx context.Context) (*T, error) {
	raw, ok := ctx.Value(CONTEXT_TASK_PARAM_NAME).(map[string]any)
	if !ok {
		return nil, errors.New("task param not found")
	}
	var target T
	cfg := mapstructure.DecoderConfig{
		Result:           &target,
		WeaklyTypedInput: true,
		DecodeHook:       mapstructure.StringToTimeDurationHookFunc(),
	}
	if err := defaults.Set(&target); err != nil {
		return nil, errors.WithStack(err)
	} else if decoder, err := mapstructure.NewDecoder(&cfg); err != nil {
		return nil, errors.WithStack(err)
	} else if err := decoder.Decode(raw); err != nil {
		return nil, errors.WithStack(err)
	}
	return &target, nil
}

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

// TaskWorker 任务 worker 的接口定义。
// 所有 worker 实现必须实现 Do 方法来执行任务。
type TaskWorker interface {
	Do(ctx context.Context, task *Task) error
}

var handlerFuncs HandlerFuncs = make(HandlerFuncs, 0)

// WorkerRegister 将任务 worker 注册到全局 handler 注册表中。
// 通过任务类型名称关联对应的处理函数。
func WorkerRegister(name string, worker TaskWorker) {
	handlerFuncs[name] = worker.Do
}

// WorkerRegistry 获取全局 handler 注册表。
// 返回所有已注册的任务处理函数映射。
func WorkerRegistry(params map[string]map[string]any) HandlerFuncs {
	wrapped := make(HandlerFuncs, len(handlerFuncs))
	for name, hFunc := range handlerFuncs {
		if param, ok := params[name]; !ok {
			wrapped[name] = hFunc
		} else {
			wrapped[name] = func(ctx context.Context, task *Task) error {
				ctx = context.WithValue(ctx, CONTEXT_TASK_PARAM_NAME, param)
				return hFunc(ctx, task)
			}
		}
	}
	return wrapped
}
