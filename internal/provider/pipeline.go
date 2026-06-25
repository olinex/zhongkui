package provider

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
)

func newPipelineInspectorProvider() do.Provider[*pipeline.Inspector] {
	return func(i do.Injector) (*pipeline.Inspector, error) {
		if redis, err := InvokePipelineRedis(i); err != nil {
			return nil, errors.WithStack(err)
		} else {
			return pipeline.NewInspector(redis), nil
		}
	}
}

func newPipelineSchdulerProvider(ctx context.Context) do.Provider[*etcd.Election] {
	return func(i do.Injector) (*etcd.Election, error) {
		if viper, err := InvokeViper(i); err != nil {
			return nil, errors.WithStack(err)
		} else if cfg, err := pipeline.ExtractConfig(viper); err != nil {
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

func newPipelineRegistryProvider() do.Provider[pipeline.HandlerFuncs] {
	return func(i do.Injector) (pipeline.HandlerFuncs, error) {
		if viper, err := InvokeViper(i); err != nil {
			return nil, errors.WithStack(err)
		} else if cfg, err := pipeline.ExtractConfig(viper); err != nil {
			return nil, errors.WithStack(err)
		} else {
			return pipeline.WorkerRegistry(cfg.Params), nil
		}
	}
}

func newPipelineClientProvider() do.Provider[*pipeline.Client] {
	return func(i do.Injector) (*pipeline.Client, error) {
		redis, err := InvokePipelineRedis(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		// 如果 OTEL 已启用，创建带链路追踪的客户端
		if rt, err := InvokeOtel(i); err == nil && rt != nil {
			return pipeline.NewTracedClient(redis), nil
		}
		return pipeline.NewClient(redis), nil
	}
}

func newPipelineServerProvider(ctx context.Context) do.Provider[*pipeline.Server] {
	return func(i do.Injector) (*pipeline.Server, error) {
		viper, err := InvokeViper(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		cfg, err := pipeline.ExtractConfig(viper)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		redisClient, err := InvokePipelineRedis(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		logger, err := InvokePipelineLogger(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		handlers, err := InvokePipelineRegistry(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		ctxFunc := pipeline.TaskContext(ctx, i)
		rawCfg := cfg.ToAsynqConfig(ctxFunc, logger)

		// 如果 OTEL 已启用，添加 Asynq trace 中间件
		rt, err := InvokeOtel(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		var mws []asynq.MiddlewareFunc
		if rt != nil {
			if mw := rt.AsynqMiddleware(); mw != nil {
				mws = append(mws, mw)
			}
		}

		return pipeline.NewServer(
			redisClient,
			rawCfg,
			logger,
			handlers,
			mws...,
		), nil
	}
}

func NewPipelineInspector() do.Provider[*pipeline.Inspector] {
	return newPipelineInspectorProvider()
}

func NewPipelineScheduler(ctx context.Context) do.Provider[*etcd.Election] {
	return newPipelineSchdulerProvider(ctx)
}

func NewPipelineRegistry() do.Provider[pipeline.HandlerFuncs] {
	return newPipelineRegistryProvider()
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
	do.Provide(i, NewPipelineRegistry())
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

func InvokePipelineRegistry(i do.Injector) (pipeline.HandlerFuncs, error) {
	return do.Invoke[pipeline.HandlerFuncs](i)
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

func MustInvokePipelineRegistry(i do.Injector) pipeline.HandlerFuncs {
	return do.MustInvoke[pipeline.HandlerFuncs](i)
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
