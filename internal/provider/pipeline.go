package provider

import (
	"context"
	"fmt"

	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/network/detectors"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/etl"
	"zhongkui.com/zhongkui/internal/pkg/otel"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

// TaskContext 创建任务上下文的函数。
// 返回一个函数，用于构建包含注入器实例的上下文。
func TaskContext(ctx context.Context, i do.Injector) pipeline.ContextFunc {
	return func() context.Context {
		suffix, err := crypto.GenerateRandomString(16)
		if err != nil {
			panic(err)
		}
		scope := i.Scope(fmt.Sprintf("task:%s", suffix))
		return context.WithValue(ctx, base.ContextScopeName, scope)
	}
}

// GetScope 从上下文中获取注入器实例。
func GetScope(ctx context.Context) *do.Scope {
	return ctx.Value(base.ContextScopeName).(*do.Scope)
}

func initDetectors(i do.Injector) error {
	if kd, err := InvokeKeyDerivation(i); err != nil {
		return errors.WithStack(err)
	} else if client, err := InvokeS3(i); err != nil {
		return errors.WithStack(err)
	} else {
		detectors.RegisterAppProtocolDetector(
			detectors.AppHttp,
			detectors.NewHttpDetector(client, kd),
		)
		return nil
	}
}

func newPipelineInspectorProvider() do.Provider[*pipeline.Inspector] {
	return func(i do.Injector) (*pipeline.Inspector, error) {
		if redis, err := InvokePipelineRedis(i); err != nil {
			return nil, errors.WithStack(err)
		} else {
			return pipeline.NewInspector(redis), nil
		}
	}
}

func newPipelineSchedulerProvider(ctx context.Context) do.Provider[*etcd.Election] {
	return func(i do.Injector) (*etcd.Election, error) {
		if viper, err := InvokeViper(i); err != nil {
			return nil, errors.WithStack(err)
		} else if cfg, err := pipeline.ExtractConfig(viper, validator.Validate); err != nil {
			return nil, errors.WithStack(err)
		} else if redis, err := InvokePipelineRedis(i); err != nil {
			return nil, errors.WithStack(err)
		} else if client, err := InvokeEtcdClient(i); err != nil {
			return nil, errors.WithStack(err)
		} else if logger, err := InvokePipelineLogger(i); err != nil {
			return nil, errors.WithStack(err)
		} else {
			return pipeline.NewElectionScheduler(
				ctx,
				client,
				redis,
				&cfg.Scheduler,
				logger,
			), nil
		}
	}
}

func newPipelineTaskRegistryProvider() do.Provider[etl.HandlerFuncs] {
	return func(i do.Injector) (etl.HandlerFuncs, error) {
		if viper, err := InvokeViper(i); err != nil {
			return nil, errors.WithStack(err)
		} else if cfg, err := pipeline.ExtractConfig(viper, validator.Validate); err != nil {
			return nil, errors.WithStack(err)
		} else if err := initDetectors(i); err != nil {
			return nil, errors.WithStack(err)
		} else {
			return etl.HandlerRegistry(cfg.Custom.Tasks), nil
		}
	}
}

func newPipelineProcessorFactoriesRegistryProvider() do.Provider[etl.ProcessorFactories] {
	return func(i do.Injector) (etl.ProcessorFactories, error) {
		if viper, err := InvokeViper(i); err != nil {
			return nil, errors.WithStack(err)
		} else if cfg, err := pipeline.ExtractConfig(viper, validator.Validate); err != nil {
			return nil, errors.WithStack(err)
		} else if err := initDetectors(i); err != nil {
			return nil, errors.WithStack(err)
		} else {
			return etl.ProcessorFactoryRegistry(cfg.Custom.Processors), nil
		}
	}
}

func newPipelineClientProvider() do.Provider[*pipeline.Client] {
	return func(i do.Injector) (*pipeline.Client, error) {
		redis, err := InvokePipelineRedis(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return pipeline.NewClient(redis), nil
	}
}

func newPipelineServerProvider(ctx context.Context) do.Provider[*pipeline.Server] {
	return func(i do.Injector) (*pipeline.Server, error) {
		if viper, err := InvokeViper(i); err != nil {
			return nil, errors.WithStack(err)
		} else if cfg, err := pipeline.ExtractConfig(viper, validator.Validate); err != nil {
			return nil, errors.WithStack(err)
		} else if redisClient, err := InvokePipelineRedis(i); err != nil {
			return nil, errors.WithStack(err)
		} else if logger, err := InvokePipelineLogger(i); err != nil {
			return nil, errors.WithStack(err)
		} else if handlers, err := InvokePipelineTaskRegistry(i); err != nil {
			return nil, errors.WithStack(err)
		} else {
			ctxFunc := TaskContext(ctx, i)
			rawCfg := cfg.ToAsynqConfig(ctxFunc, logger)

			return pipeline.NewServer(
				ctx,
				redisClient,
				rawCfg,
				logger,
				handlers,
				otel.AsynqServerMiddleware(),
			), nil
		}
	}
}

// NewPipelineInspector 返回构建流水线巡检器的 provider 工厂。
func NewPipelineInspector() do.Provider[*pipeline.Inspector] {
	return newPipelineInspectorProvider()
}

// NewPipelineScheduler 返回构建流水线调度器的 provider 工厂。
func NewPipelineScheduler(ctx context.Context) do.Provider[*etcd.Election] {
	return newPipelineSchedulerProvider(ctx)
}

// NewPipelineTaskRegistry 返回构建流水线任务注册表的 provider 工厂。
func NewPipelineTaskRegistry() do.Provider[etl.HandlerFuncs] {
	return newPipelineTaskRegistryProvider()
}

// NewPipelineProcessorFactoriesRegistry 返回构建流水线处理器工厂注册表的 provider 工厂。
func NewPipelineProcessorFactoriesRegistry() do.Provider[etl.ProcessorFactories] {
	return newPipelineProcessorFactoriesRegistryProvider()
}

// NewPipelineClient 返回构建流水线客户端的 provider 工厂。
func NewPipelineClient() do.Provider[*pipeline.Client] {
	return newPipelineClientProvider()
}

// NewPipelineServer 返回构建流水线服务端的 provider 工厂。
func NewPipelineServer(ctx context.Context) do.Provider[*pipeline.Server] {
	return newPipelineServerProvider(ctx)
}

// InjectPipelineInspector 向依赖注入器注册流水线巡检器 provider。
func InjectPipelineInspector(i do.Injector) {
	do.Provide(i, NewPipelineInspector())
}

// InjectPipelineScheduler 向依赖注入器注册流水线调度器 provider。
func InjectPipelineScheduler(ctx context.Context, i do.Injector) {
	do.Provide(i, NewPipelineScheduler(ctx))
}

// InjectPipelineRegistry 向依赖注入器注册流水线任务注册表 provider。
func InjectPipelineRegistry(i do.Injector) {
	do.Provide(i, NewPipelineTaskRegistry())
}

// InjectPipelineProcessorFactoriesRegistry 向依赖注入器注册流水线处理器工厂注册表 provider。
func InjectPipelineProcessorFactoriesRegistry(i do.Injector) {
	do.Provide(i, NewPipelineProcessorFactoriesRegistry())
}

// InjectPipelineClient 向依赖注入器注册流水线客户端 provider（按需创建）。
func InjectPipelineClient(i do.Injector) {
	do.ProvideTransient(i, NewPipelineClient())
}

// InjectPipelineServer 向依赖注入器注册流水线服务端 provider。
func InjectPipelineServer(ctx context.Context, i do.Injector) {
	do.Provide(i, NewPipelineServer(ctx))
}

// InvokePipelineInspector 从依赖注入器解析流水线巡检器，错误交由调用方处理。
func InvokePipelineInspector(i do.Injector) (*pipeline.Inspector, error) {
	return do.Invoke[*pipeline.Inspector](i)
}

// InvokePipelineScheduler 从依赖注入器解析流水线调度器，错误交由调用方处理。
func InvokePipelineScheduler(i do.Injector) (*etcd.Election, error) {
	return do.Invoke[*etcd.Election](i)
}

// InvokePipelineTaskRegistry 从依赖注入器解析流水线任务注册表，错误交由调用方处理。
func InvokePipelineTaskRegistry(i do.Injector) (etl.HandlerFuncs, error) {
	return do.Invoke[etl.HandlerFuncs](i)
}

// InvokePipelineProcessorFactoriesRegistry 从依赖注入器解析流水线处理器工厂注册表，错误交由调用方处理。
func InvokePipelineProcessorFactoriesRegistry(i do.Injector) (etl.ProcessorFactories, error) {
	return do.Invoke[etl.ProcessorFactories](i)
}

// InvokePipelineClient 从依赖注入器解析流水线客户端，错误交由调用方处理。
func InvokePipelineClient(i do.Injector) (*pipeline.Client, error) {
	return do.Invoke[*pipeline.Client](i)
}

// MustInvokePipelineInspector 从依赖注入器解析流水线巡检器，失败时 panic。
func MustInvokePipelineInspector(i do.Injector) *pipeline.Inspector {
	return do.MustInvoke[*pipeline.Inspector](i)
}

// MustInvokePipelineScheduler 从依赖注入器解析流水线调度器，失败时 panic。
func MustInvokePipelineScheduler(i do.Injector) *etcd.Election {
	return do.MustInvoke[*etcd.Election](i)
}

// MustInvokePipelineTaskRegistry 从依赖注入器解析流水线任务注册表，失败时 panic。
func MustInvokePipelineTaskRegistry(i do.Injector) etl.HandlerFuncs {
	return do.MustInvoke[etl.HandlerFuncs](i)
}

// MustInvokePipelineProcessorFactoriesRegistry 从依赖注入器解析流水线处理器工厂注册表，失败时 panic。
func MustInvokePipelineProcessorFactoriesRegistry(i do.Injector) etl.ProcessorFactories {
	return do.MustInvoke[etl.ProcessorFactories](i)
}

// MustInvokePipelineClient 从依赖注入器解析流水线客户端，失败时 panic。
func MustInvokePipelineClient(i do.Injector) *pipeline.Client {
	return do.MustInvoke[*pipeline.Client](i)
}

// MustInvokePipelineServer 从依赖注入器解析流水线服务端，失败时 panic。
func MustInvokePipelineServer(i do.Injector) *pipeline.Server {
	return do.MustInvoke[*pipeline.Server](i)
}

// InvokePipelineServer 从依赖注入器解析流水线服务端，错误交由调用方处理。
func InvokePipelineServer(i do.Injector) (*pipeline.Server, error) {
	return do.Invoke[*pipeline.Server](i)
}

// DescribePipelineConfig 返回流水线的配置项描述。
func DescribePipelineConfig() config.ConfigDescriptions {
	return pipeline.DescribeConfig()
}
