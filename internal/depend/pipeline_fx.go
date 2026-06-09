package depend

import (
	"context"
	"fmt"

	"github.com/spf13/viper"
	"go.uber.org/fx"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
)

const (
	WORKER_PIPELINE = "worker"
)

type PipelineInspectorFxIn struct {
	fx.In
	PipelineRedisFxIn
}

type PipelineSchedulerFxIn struct {
	fx.In
	ManageEtcdFxIn
	PipelineRedisFxIn
	PipelineLoggerFxIn
	Ctx context.Context
	V   *viper.Viper
}

type PipelineClientFxIn struct {
	fx.In
	PipelineRedisFxIn
	Lc fx.Lifecycle
}

type PipelineServerFxIn struct {
	fx.In
	PipelineLoggerFxIn
	PipelineRedisFxIn
	V        *viper.Viper
	CtxFunc  pipeline.ContextFunc
	Handlers pipeline.HandlerFuncs
}

type WorkerPipelineInspectorFxIn struct {
	fx.In
	WorkerPipelineInspector *pipeline.Inspector `name:"worker"`
}

type WorkerPipelineSchedulerFxIn struct {
	fx.In
	WorkerPipelineScheduler *etcd.Election `name:"worker"`
}

type WorkerPipelineClientFxIn struct {
	fx.In
	WorkerPipelineClient *pipeline.Client `name:"worker"`
}

type WorkerPipelineServerFxIn struct {
	fx.In
	WorkerPipelineServer *pipeline.Server `name:"worker"`
}

type PipelineFxIn struct {
	fx.In
	PipelineLoggerFxIn
	WorkerPipelineSchedulerFxIn
	WorkerPipelineServerFxIn
	Lc fx.Lifecycle
}

func newPipelineInspector(in PipelineInspectorFxIn) *pipeline.Inspector {
	return pipeline.NewInspector(in.PipelineRedis)
}

func newPipelineScheduler(in PipelineSchedulerFxIn) *etcd.Election {
	cfg, err := pipeline.ExtractConfig(in.V, WORKER_PIPELINE)
	if err != nil {
		panic(err)
	}
	return pipeline.NewElectionScheduler(
		in.Ctx,
		in.ManageEtcd,
		in.PipelineRedis,
		&cfg.Scheduler,
		in.PipelineLogger,
	)
}

func newPipelineClient(in PipelineClientFxIn) *pipeline.Client {
	cli := pipeline.NewClient(in.PipelineRedis)
	in.Lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return cli.Close()
		},
	})
	return cli
}

func newPipelineServer(in PipelineServerFxIn) *pipeline.Server {
	cfg, err := pipeline.ExtractConfig(in.V, WORKER_PIPELINE)
	if err != nil {
		panic(err)
	}
	rawCfg := cfg.ToAsynqConfig(in.CtxFunc, in.PipelineLogger)
	return pipeline.NewServer(
		in.PipelineRedis,
		rawCfg,
		in.PipelineLogger,
		in.Handlers,
	)
}

func PipelineInspectorAnnotate() any {
	return fx.Annotate(newPipelineInspector, fx.ResultTags(fmt.Sprintf(`name:"%s"`, WORKER_PIPELINE)))
}

func PipelineSchedulerAnnotate() any {
	return fx.Annotate(newPipelineScheduler, fx.ResultTags(fmt.Sprintf(`name:"%s"`, WORKER_PIPELINE)))
}

func PipelineClientAnnotate() any {
	return fx.Annotate(newPipelineClient, fx.ResultTags(fmt.Sprintf(`name:"%s"`, WORKER_PIPELINE)))
}

func PipelineServerAnnotate() any {
	return fx.Annotate(newPipelineServer, fx.ResultTags(fmt.Sprintf(`name:"%s"`, WORKER_PIPELINE)))
}

func DescribePipelineConfig() config.ConfigDescriptions {
	return pipeline.DescribeConfig(WORKER_PIPELINE)
}

func PipelineWorkerFx(in PipelineFxIn) {
	in.Lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := in.WorkerPipelineServer.Start(); err != nil {
				return errors.WithStack(err)
			}
			in.PipelineLogger.Info("pipeline server is running...")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			in.WorkerPipelineServer.Shutdown()
			in.PipelineLogger.Info("pipeline server shutdown")
			return nil
		},
	})
	in.Lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				in.PipelineLogger.Info("pipeline election scheduler is running...")
				if err := in.WorkerPipelineScheduler.Start(); err != nil {
					in.PipelineLogger.Fatalf(`Pipeline election scheduler failed cause %v`, err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			in.WorkerPipelineScheduler.Shutdown()
			in.PipelineLogger.Info("pipeline election scheduler shutdown")
			return nil
		},
	})
}
