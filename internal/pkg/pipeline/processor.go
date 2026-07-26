package pipeline

import (
	"context"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

const (
	// CONTEXT_PROC_PARAM_NAME 是处理器参数在上下文中的键名。
	// 编排器将处理器参数作为上下文值传递给处理器，键名为该常量。
	CONTEXT_PROC_PARAM_NAME = "ctx_proc_param"
)

type Processor interface {
	Start(ctx context.Context, name string) error
	Shutdown()
}

type ProcessorFactory func(ctx context.Context) (Processor, error)

type ProcessorWrapper struct {
	Processor
	params map[string]any
}

func (p *ProcessorWrapper) Start(ctx context.Context, name string) error {
	ctx = context.WithValue(ctx, CONTEXT_PROC_PARAM_NAME, p.params)
	return p.Processor.Start(ctx, name)
}

func (p *ProcessorWrapper) Shutdown() {
	p.Processor.Shutdown()
}

type ProcessorFactories = map[string]ProcessorFactory

// registeredProcessorFactories 存储所有已注册的处理器工厂。
var registeredProcessorFactories = make(ProcessorFactories, 0)

// RegisterProcessorFactory 注册一个分布式处理器工厂。
// 在 init() 中调用，将处理器名称与工厂函数关联。
func RegisterProcessorFactory(name string, factory ProcessorFactory) {
	registeredProcessorFactories[name] = factory
}

func GetProcParam[T any](ctx context.Context) (*T, error) {
	return getParam[T](ctx, CONTEXT_PROC_PARAM_NAME)
}

func ProcessorFactoryRegistry(params map[string]map[string]any) ProcessorFactories {
	wrapped := make(ProcessorFactories, len(registeredProcessorFactories))
	for name, factory := range registeredProcessorFactories {
		if param, ok := params[name]; !ok {
			wrapped[name] = factory
		} else {
			wrapped[name] = func(ctx context.Context) (Processor, error) {
				// 注入参数到上下文，使工厂函数在创建阶段即可通过 GetProcParam 获取配置
				ctx = context.WithValue(ctx, CONTEXT_PROC_PARAM_NAME, param)
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
