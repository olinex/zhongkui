package pipeline

import (
	"context"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

const (
	// 是处理器参数在上下文中的键名。
	// 编排器将处理器参数作为上下文值传递给处理器，键名为该常量。
	ContextProcParamName = "ctx_proc_param"
)

// Processor 长生命周期后台处理器接口
// Processor 与 Handler 的区别：
//   - Handler：处理单个任务的短期逻辑，任务执行完毕后立即返回。适用于 ETL 流水线中的 Extract/Transform/Load 阶段。
//   - Processor：管理长期运行的后台工作协程（如定期拉取、事件监听）。调用 Start 启动后台循环，调用 Shutdown 优雅停止。
//
// 当业务需要持续运行而非一次性的任务处理时，应使用 Processor 而非 Handler。
type Processor interface {
	Start(ctx context.Context, name string) error
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
	params map[string]any
}

func (p *ProcessorWrapper) Start(ctx context.Context, name string) error {
	ctx = context.WithValue(ctx, ContextProcParamName, p.params)
	return p.Processor.Start(ctx, name)
}

func (p *ProcessorWrapper) Shutdown() {
	p.Processor.Shutdown()
}

// ProcessorFactories 处理器工厂注册表类型别名
// 键为处理器名称，值为对应的 ProcessorFactory 工厂函数。
// 通过 RegisterProcessorFactory 注册，通过 ProcessorFactoryRegistry 获取带参数注入的副本。
type ProcessorFactories = map[string]ProcessorFactory

// 存储所有已注册的处理器工厂。
var registeredProcessorFactories = make(ProcessorFactories, 0)

// RegisterProcessorFactory 注册一个分布式处理器工厂。
// 在 init() 中调用，将处理器名称与工厂函数关联。
func RegisterProcessorFactory(name string, factory ProcessorFactory) {
	registeredProcessorFactories[name] = factory
}

// GetProcParam 从上下文中提取指定类型的处理器配置参数
// 泛型参数 T 为目标配置结构体的类型，函数从 ctx 中按键 ContextProcParamName 取值并反序列化。
// 返回 (*T, error)：成功时返回配置指针，上下文无此键或类型不匹配时返回错误。
func GetProcParam[T any](ctx context.Context) (*T, error) {
	return getParam[T](ctx, ContextProcParamName)
}

// ProcessorFactoryRegistry 获取带参数注入的处理器工厂注册表副本
// 遍历所有已注册的处理器工厂，如果 params 中存在同名的配置参数，则包装工厂函数：
// 在创建 Processor 时先将配置参数注入上下文，再调用原始工厂，并将结果包装为 ProcessorWrapper。
// 返回的是全新创建的映射副本，不会影响全局注册表。
func ProcessorFactoryRegistry(params map[string]map[string]any) ProcessorFactories {
	wrapped := make(ProcessorFactories, len(registeredProcessorFactories))
	for name, factory := range registeredProcessorFactories {
		if param, ok := params[name]; !ok {
			wrapped[name] = factory
		} else {
			wrapped[name] = func(ctx context.Context) (Processor, error) {
				// 注入参数到上下文，使工厂函数在创建阶段即可通过 GetProcParam 获取配置
				ctx = context.WithValue(ctx, ContextProcParamName, param)
				processor, err := factory(ctx)
				if err != nil {
					return nil, errors.WithStack(err)
				}
				return &ProcessorWrapper{
					Processor: processor,
					params:    param,
				}, nil
			}
		}
	}
	return wrapped
}
