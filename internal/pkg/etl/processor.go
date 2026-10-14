package etl

import (
	"context"

	"zhongkuisec.com/zhongkui/internal/pkg/config"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

const (
	// ContextProcParamName 处理器参数在上下文中的键名。
	// 编排器将处理器参数作为上下文值传递给处理器，键名为该常量。
	ContextProcParamName = "ctx_proc_param"
)

// Processor 长生命周期后台处理器接口
// Processor 与队列 Handler 的区别：
//   - Handler：处理单个任务的短期逻辑，任务执行完毕后立即返回。适用于 ETL 流水线中的 Extract/Transform/Load 阶段。
//   - Processor：管理长期运行的后台工作协程（如定期拉取、事件监听）。调用 Start 启动后台循环，调用 Shutdown 优雅停止。
//
// 当业务需要持续运行而非一次性的任务处理时，应使用 Processor 而非 Handler。
// Processor 是分布式/流式驱动的常驻处理契约，其生命周期由运行时统一编排，不隶属于任务队列。
type Processor interface {
	// Start 启动后台循环，阻塞直到 ctx 取消或发生错误；name 为处理器标识。
	Start(ctx context.Context, name string) error
	// Shutdown 优雅停止处理器并释放资源。
	Shutdown()
}

// ProcessorFactory 处理器工厂函数类型别名
// 接收 context.Context 作为上下文，创建并返回一个 Processor 实例。
// 工厂函数在 ProcessorFactoryRegistry 中被调用，调用前上下文已注入配置参数。
type ProcessorFactory func(ctx context.Context) (Processor, error)

// ProcessorWrapper 处理器包装器结构
// 嵌入 Processor 接口，并额外存储配置参数映射 params。
// 在 Start 方法中自动将 params 注入到上下文中，使被包装的 Processor 通过 GetProcParam 获取配置。
type ProcessorWrapper struct {
	Processor
	// params 处理器配置参数映射，Start 时注入上下文供处理器读取。
	params map[string]any
}

// Start 将配置参数注入上下文后启动被包装的处理器。
func (p *ProcessorWrapper) Start(ctx context.Context, name string) error {
	ctx = context.WithValue(ctx, ContextProcParamName, p.params)
	return p.Processor.Start(ctx, name)
}

// Shutdown 停止被包装的处理器。
func (p *ProcessorWrapper) Shutdown() {
	p.Processor.Shutdown()
}

// ProcessorFactories 处理器工厂注册表类型别名
// 键为处理器名称，值为对应的 ProcessorFactory 工厂函数。
// 通过 RegisterProcessorFactory 注册，通过 ProcessorFactoryRegistry 获取带参数注入的副本。
type ProcessorFactories = map[string]ProcessorFactory

// processorRegistry 全局处理器工厂注册表，通过 RegisterProcessorFactory 写入。
var processorRegistry = newRegistry[ProcessorFactory]()

// RegisterProcessorFactory 注册一个分布式处理器工厂。
// 在 init() 中调用，将处理器名称与工厂函数关联。
func RegisterProcessorFactory(name string, factory ProcessorFactory) {
	processorRegistry.Register(name, factory)
}

// GetProcParam 从上下文中提取指定类型的处理器配置参数
// 泛型参数 T 为目标配置结构体的类型，函数从 ctx 中按键 ContextProcParamName 取值并反序列化。
// 返回 (*T, error)：成功时返回配置指针，上下文无此键或类型不匹配时返回错误。
func GetProcParam[T any](ctx context.Context) (*T, error) {
	return config.GetContextParam[T](ctx, ContextProcParamName)
}

// ProcessorFactoryRegistry 获取带参数注入的处理器工厂注册表副本
// 通过 Registry.Snapshot 生成副本：params 中命名的工厂被包装，
// 在创建 Processor 时先将配置参数注入上下文（键为 ContextProcParamName），
// 并将结果包装为 ProcessorWrapper（Start 时再次注入）。
// 返回的是全新创建的映射副本，不会影响全局注册表。
func ProcessorFactoryRegistry(params map[string]map[string]any) ProcessorFactories {
	return processorRegistry.Snapshot(params, func(_ string, factory ProcessorFactory, param map[string]any) ProcessorFactory {
		return func(ctx context.Context) (Processor, error) {
			// 注入参数到上下文，使工厂函数在创建阶段即可通过 GetProcParam 获取配置
			ctx = withParam(ctx, ContextProcParamName, param)
			processor, err := factory(ctx)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			return &ProcessorWrapper{
				Processor: processor,
				params:    param,
			}, nil
		}
	})
}
