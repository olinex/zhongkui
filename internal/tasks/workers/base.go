package workers

import (
	"context"

	"github.com/reactivex/rxgo/v2"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
)

// LiveFunc 数据处理函数的类型定义。
// 用于在数据处理过程执行前后的自定义逻辑。
type LiveFunc = func(ctx context.Context, task *pipeline.Task) (callback func(), err error)

// ExtractFunc 数据提取函数的类型定义。
// 从任务中提取数据并通过 channel 输出。
type ExtractFunc = func(ctx context.Context, task *pipeline.Task, channel chan<- rxgo.Item) error

// TransformFunc 数据转换函数的类型定义。
// 对输入数据进行转换并返回转换后的结果。
type TransformFunc = func(ctx context.Context, data any) (any, error)

// LoadFunc 数据加载函数的类型定义。
// 将数据项批量加载到目标存储（如数据库）。
type LoadFunc = func(ctx context.Context, items []any) error

// TaskWorker 任务 worker 的接口定义。
// 所有 worker 实现必须实现 Do 方法来执行任务。
type TaskWorker interface {
	Do(ctx context.Context, task *pipeline.Task) error
}

var handlerFuncs pipeline.HandlerFuncs = make(pipeline.HandlerFuncs, 0)

// WorkerRegister 将任务 worker 注册到全局 handler 注册表中。
// 通过任务类型名称关联对应的处理函数。
func WorkerRegister(name string, worker TaskWorker) {
	handlerFuncs[name] = worker.Do
}

// WorkerRegistry 获取全局 handler 注册表。
// 返回所有已注册的任务处理函数映射。
func WorkerRegistry() pipeline.HandlerFuncs {
	return handlerFuncs
}
