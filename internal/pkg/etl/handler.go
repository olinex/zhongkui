package etl

import (
	"context"

	"zhongkui.com/zhongkui/internal/pkg/config"
)

const (
	// ContextHandlerParamName 上下文中的任务配置键名
	// 用于存储任务上下文中的任务配置
	ContextHandlerParamName = "ctx_handler_param"
)

// Task 任务抽象接口，提供任务类型与载荷访问。
// 由队列实现（如 asynq.Task）满足该契约，使 etl 包不依赖具体队列实现。
// 这是事件驱动任务（Handler）与分布式常驻任务（Processor）之上的统一任务契约。
type Task interface {
	// Type 返回任务类型，用于匹配对应的处理函数。
	Type() string
	// Payload 返回任务载荷的原始字节。
	Payload() []byte
}

// Handler 任务处理器的接口定义。
// 所有 worker 实现必须实现 Do 方法来执行任务。
// 与 Processor 的区别：
//   - Handler：处理单个任务的短期逻辑，任务执行完毕后立即返回。适用于 ETL 流水线中的 Extract/Transform/Load 阶段。
//   - Processor：管理长期运行的后台工作协程，调用 Start 启动后台循环，调用 Shutdown 优雅停止。
type Handler interface {
	// Do 处理单个任务，任务执行完毕后返回。
	Do(ctx context.Context, task Task) error
}

// LiveFunc 数据处理函数的类型定义。
// 用于在数据处理过程执行前后的自定义逻辑。
type LiveFunc = func(ctx context.Context, task Task) error

// TaskExtractFunc 任务数据提取函数类型。
// 从任务中提取数据并通过 out 通道输出（golang 原生类型，替代 rxgo.Item 桥接）。
// 与通用流水线 ExtractFunc[T] 的区别：TaskExtractFunc 绑定任务上下文，由 Chain 驱动。
type TaskExtractFunc[T any] = func(ctx context.Context, task Task, out chan<- T) error

// TaskTransformFunc 任务数据转换函数类型。
// 对输入数据项进行转换并返回转换后的结果。
type TaskTransformFunc[T any] = func(ctx context.Context, data T) (T, error)

// TaskLoadFunc 任务数据加载函数类型。
// 将一批数据项批量加载到目标存储（如数据库）。
type TaskLoadFunc[T any] = func(ctx context.Context, items []T) error

// HandlerFunc 任务处理函数类型，Handler.Do 的函数形态。
// 与 Handler 接口一一对应，用于在注册表中按任务类型映射处理函数。
type HandlerFunc = func(ctx context.Context, task Task) error

// HandlerFuncs 是任务处理函数的映射表类型别名。
// 键为任务类型（模式），用于匹配队列中的任务类型；值为对应的处理函数。
type HandlerFuncs = map[string]HandlerFunc

// handlerRegistry 全局任务处理函数注册表，通过 HandlerRegister 写入。
var handlerRegistry = newRegistry[HandlerFunc]()

// HandlerRegister 将任务处理器注册到全局 handler 注册表中
// 通过任务类型名称关联对应的处理函数。通常在 init() 函数中调用以完成自动注册，
// 也可在 HandlerRegistry 调用前手动注册。
//
// 使用示例（在 init() 中）：
//
//	func init() {
//	    etl.HandlerRegister("my_task", &MyHandler{})
//	}
func HandlerRegister(name string, handler Handler) {
	handlerRegistry.Register(name, handler.Do)
}

// GetHandlerParam 从上下文中提取指定类型的任务处理器配置
// 泛型参数 T 为目标配置结构体的类型。函数从 ctx 中按键 ContextHandlerParamName
// 查找由 HandlerRegistry 注入的配置参数，并反序列化为 *T。
// 返回 (*T, error)：成功时返回配置指针，上下文无此键或类型不匹配时返回错误。
func GetHandlerParam[T any](ctx context.Context) (*T, error) {
	return config.GetContextParam[T](ctx, ContextHandlerParamName)
}

// HandlerRegistry 获取带参数注入的全局 handler 注册表副本
// 通过 Registry.Snapshot 生成副本：params 中命名的 handler 被包装，
// 在调用前将配置参数注入上下文（键为 ContextHandlerParamName），
// 使 handler 内部可通过 GetHandlerParam 获取配置。
// 返回的是全新创建的映射副本，不会影响全局注册表。
func HandlerRegistry(params map[string]map[string]any) HandlerFuncs {
	return handlerRegistry.Snapshot(params, func(_ string, hFunc HandlerFunc, param map[string]any) HandlerFunc {
		return func(ctx context.Context, task Task) error {
			ctx = withParam(ctx, ContextHandlerParamName, param)
			return hFunc(ctx, task)
		}
	})
}
