package cmd

import (
	"context"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"zhongkui.com/zhongkui/internal/capture"
	"zhongkui.com/zhongkui/internal/depend"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/manager/routes"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/validator"
	"zhongkui.com/zhongkui/internal/reporter"
	_ "zhongkui.com/zhongkui/internal/tasks"
	"zhongkui.com/zhongkui/internal/tasks/workers"
)

type FxIn struct {
	fx.In
	depend.CaptureLoggerFxIn
	depend.ReporterGrpcClientFxIn
	depend.CaptureS3ClientIn
	Lc     fx.Lifecycle
	Ctx    context.Context
	Client *capture.CaptureClient
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the zhongkui server process",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Run the zhongkui http web api process",
	Run: func(cmd *cobra.Command, args []string) {
		fx.New(
			depend.FxLogger(),
			fx.Provide(
				func() (*cobra.Command, context.Context) {
					return cmd, context.Background()
				},
				config.NewDefaultViperWithCmd,
				manager.ExtractConfig,
				depend.NewGin,
				depend.ApiLoggerAnnotate(),
				depend.DatabaseLoggerAnnotate(),
				depend.ETCDLoggerAnnotate(),
				depend.ManageEtcdAnnotate(),
				depend.CoreDatabaseAnnotate(),
				depend.RawDatabaseAnnotate(),
				depend.PipelineRedisAnnotate(),
				depend.PipelineInspectorAnnotate(),
				depend.NewI18n,
				depend.NewRbac,
			),
			fx.Invoke(
				validator.ValidatorRegister,
				core.NamespaceCacheFx,
				routes.RouteRegisterFx,
				manager.RunHttpServiceFx,
			),
		).Run()
	},
}

var grpcCmd = &cobra.Command{
	Use:   "grpc",
	Short: "Run the zhongkui http grpc api process",
	Run: func(cmd *cobra.Command, args []string) {
		fx.New(
			depend.FxLogger(),
			fx.Provide(
				func() (*cobra.Command, context.Context) {
					return cmd, context.Background()
				},
				gin.New,
				config.NewDefaultViperWithCmd,
			manager.ExtractConfig,
			depend.GRPCLoggerAnnotate(),
				depend.DatabaseLoggerAnnotate(),
				depend.ETCDLoggerAnnotate(),
				depend.ManageEtcdAnnotate(),
				depend.CacheRedisAnnotate(),
				depend.PipelineRedisAnnotate(),
				depend.PipelineClientAnnotate(),
				depend.CoreDatabaseAnnotate(),
				reporter.NewReporterAuthenticators,
				depend.ReporterGrpcServerAnnotate(),
				depend.NewI18n,
				depend.NewRbac,
			),
			fx.Invoke(
				validator.ValidatorRegister,
				reporter.ServiceInjectFx,
				reporter.RunGrpcServiceFx,
			),
		).Run()
	},
}

var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Run the zhongkui worker Process",
	Run: func(cmd *cobra.Command, args []string) {
		fx.New(
			depend.FxLogger(),
			fx.Provide(
				// base depends
				func() (*cobra.Command, context.Context) {
					return cmd, context.Background()
				},
				config.NewDefaultViperWithCmd,
				depend.ETCDLoggerAnnotate(),
				depend.DatabaseLoggerAnnotate(),
				depend.PipelineLoggerAnnotate(),
				depend.ManageEtcdAnnotate(),
				depend.PipelineRedisAnnotate(),
				depend.CaptureS3Annotate(),
				depend.RawDatabaseAnnotate(),
				// pipeline depends
				workers.TaskContext,
				workers.WorkerRegistry,
				depend.PipelineServerAnnotate(),
				depend.PipelineSchedulerAnnotate(),
			),
			fx.Invoke(
				depend.PipelineWorkerFx,
			),
		).Run()
	},
}

var captureCmd = &cobra.Command{
	Use:   "capture",
	Short: "Run the zhongkui capture Process",
	Run: func(cmd *cobra.Command, args []string) {
		fx.New(
			depend.FxLogger(),
			fx.Provide(
				func() (*cobra.Command, context.Context) {
					return cmd, context.Background()
				},
				config.NewDefaultViperWithCmd,
				depend.CaptureLoggerAnnotate(),
				depend.ReporterGrpcClientAnnotate(),
				depend.CaptureS3Annotate(),
				depend.NewCaptureClientFx,
			),
			fx.Invoke(func(in FxIn) {
				sessionID := serializer.NewHexUUIDv4()
				in.CaptureLogger.Infof("starting zhongkui capture %s", sessionID.String())

				if err := in.Client.Run(in.Ctx, sessionID); err != nil {
					in.CaptureLogger.DPanic(err)
					os.Exit(1)
				}

				in.CaptureLogger.Infof("shutting down zhongkui capture %s", sessionID.String())
				os.Exit(0)
			}),
		).Run()
	},
}

var singletonCmd = &cobra.Command{
	Use:   "singleton",
	Short: "Run the zhongkui singleton Process",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	// 其他无状态服务
	runCmd.AddCommand(webCmd)
	runCmd.AddCommand(grpcCmd)
	runCmd.AddCommand(workerCmd)
	runCmd.AddCommand(captureCmd)
	runCmd.AddCommand(singletonCmd)
}
