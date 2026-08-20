package provider

import (
	"context"
	"fmt"

	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/network/detectors"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/etl"
	"zhongkui.com/zhongkui/internal/pkg/otel"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

const (
	// ContextScopeName 上下文中的注入器键名
	// 用于存储任务上下文中的注入器实例
	ContextScopeName = "ctx_scope"
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
		return context.WithValue(ctx, ContextScopeName, scope)
	}
}

// GetScope 从上下文中获取注入器实例。
func GetScope(ctx context.Context) *do.Scope {
	return ctx.Value(ContextScopeName).(*do.Scope)
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

func NewPipelineInspector() do.Provider[*pipeline.Inspector] {
	return newPipelineInspectorProvider()
}

func NewPipelineScheduler(ctx context.Context) do.Provider[*etcd.Election] {
	return newPipelineSchedulerProvider(ctx)
}

func NewPipelineTaskRegistry() do.Provider[etl.HandlerFuncs] {
	return newPipelineTaskRegistryProvider()
}

func NewPipelineProcessorFactoriesRegistry() do.Provider[etl.ProcessorFactories] {
	return newPipelineProcessorFactoriesRegistryProvider()
}

func NewPipelineClient() do.Provider[*pipeline.Client] {
	return newPipelineClientProvider()
}

func NewPipelineServer(ctx context.Context) do.Provider[*pipeline.Server] {
	return newPipelineServerProvider(ctx)
}

func InjectPipelineInspector(i do.Injector) {
	do.Provide(i, NewPipelineInspector())
}

func InjectPipelineScheduler(ctx context.Context, i do.Injector) {
	do.Provide(i, NewPipelineScheduler(ctx))
}

func InjectPipelineRegistry(i do.Injector) {
	do.Provide(i, NewPipelineTaskRegistry())
}

func InjectPipelineProcessorFactoriesRegistry(i do.Injector) {
	do.Provide(i, NewPipelineProcessorFactoriesRegistry())
}

func InjectPipelineClient(i do.Injector) {
	do.ProvideTransient(i, NewPipelineClient())
}

func InjectPipelineServer(ctx context.Context, i do.Injector) {
	do.Provide(i, NewPipelineServer(ctx))
}

func InvokePipelineInspector(i do.Injector) (*pipeline.Inspector, error) {
	return do.Invoke[*pipeline.Inspector](i)
}

func InvokePipelineScheduler(i do.Injector) (*etcd.Election, error) {
	return do.Invoke[*etcd.Election](i)
}

func InvokePipelineTaskRegistry(i do.Injector) (etl.HandlerFuncs, error) {
	return do.Invoke[etl.HandlerFuncs](i)
}

func InvokePipelineProcessorFactoriesRegistry(i do.Injector) (etl.ProcessorFactories, error) {
	return do.Invoke[etl.ProcessorFactories](i)
}

func InvokePipelineClient(i do.Injector) (*pipeline.Client, error) {
	return do.Invoke[*pipeline.Client](i)
}

func MustInvokePipelineInspector(i do.Injector) *pipeline.Inspector {
	return do.MustInvoke[*pipeline.Inspector](i)
}

func MustInvokePipelineScheduler(i do.Injector) *etcd.Election {
	return do.MustInvoke[*etcd.Election](i)
}

func MustInvokePipelineTaskRegistry(i do.Injector) etl.HandlerFuncs {
	return do.MustInvoke[etl.HandlerFuncs](i)
}

func MustInvokePipelineProcessorFactoriesRegistry(i do.Injector) etl.ProcessorFactories {
	return do.MustInvoke[etl.ProcessorFactories](i)
}

func MustInvokePipelineClient(i do.Injector) *pipeline.Client {
	return do.MustInvoke[*pipeline.Client](i)
}

func MustInvokePipelineServer(i do.Injector) *pipeline.Server {
	return do.MustInvoke[*pipeline.Server](i)
}

func InvokePipelineServer(i do.Injector) (*pipeline.Server, error) {
	return do.Invoke[*pipeline.Server](i)
}

func DescribePipelineConfig() config.ConfigDescriptions {
	return pipeline.DescribeConfig()
}
