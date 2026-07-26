package cmd

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"github.com/pterm/pterm"
	"github.com/samber/do/v2"
	"github.com/spf13/cobra"
	_ "zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/validator"
	"zhongkui.com/zhongkui/internal/provider"
)

const (
	INJECTOR_CTX_NAME = "inj"
	SIGNAL_CTX_NAME   = "sig"
)

func cmdToolBox(cmd *cobra.Command) (context.Context, do.Injector) {
	ctx := cmd.Context()
	return ctx, ctx.Value(INJECTOR_CTX_NAME).(do.Injector)
}

// getSignalCancel 从 context 中获取信号监听停止函数
func getSignalCancel(ctx context.Context) context.CancelFunc {
	if cancel, ok := ctx.Value(SIGNAL_CTX_NAME).(context.CancelFunc); ok {
		return cancel
	}
	return nil
}

var RootCmd = &cobra.Command{
	Use:   "help",
	Short: "The zhongkui root command lines",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var i = do.New()
		ctx := cmd.Context()

		// 统一信号处理：监听 SIGINT/SIGTERM，收到信号时取消 context
		// 所有服务通过监听 context 取消来实现优雅关闭
		ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
		ctx = context.WithValue(ctx, SIGNAL_CTX_NAME, stop)
		cmd.SetContext(ctx)

		provider.InjectOtelShutdowner(ctx, i)
		_ = provider.MustInvokeOtelShutdowner(ctx, i)

		provider.InjectCommand(i, cmd)
		provider.InjectViper(i)
		provider.InjectKeyDerivation(i)
		provider.InjectS3(i)
		provider.InjectGin(i)
		validator.Init()
		provider.InjectI18n(ctx, i)
		provider.InjectRbac(i)
		provider.InjectApiLogger(i)
		provider.InjectDatabaseLogger(i)
		provider.InjectCaptureLogger(i)
		provider.InjectEtcdLogger(i)
		provider.InjectGrpcLogger(i)
		provider.InjectPipelineLogger(i)
		provider.InjectCoreDatabase(i)
		provider.InjectRawDatabase(i)
		provider.InjectCacheRedis(i)
		provider.InjectPipelineRedis(i)
		provider.InjectEtcdClient(ctx, i)
		provider.InjectReporterGrpcServer(i)
		provider.InjectReporterGrpcClient(i)
		provider.InjectPipelineInspector(i)
		provider.InjectPipelineScheduler(ctx, i)
		provider.InjectPipelineRegistry(i)
		provider.InjectPipelineProcessorFactoriesRegistry(i)
		provider.InjectPipelineClient(i)
		provider.InjectPipelineServer(ctx, i)
		provider.InjectCaptureClient(i)

		ctx = context.WithValue(ctx, INJECTOR_CTX_NAME, i)
		cmd.SetContext(ctx)
		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		ctx, i := cmdToolBox(cmd)

		// 调用信号取消函数，确保信号监听资源被释放
		// 即使服务已经正常退出，这里调用也是安全的
		if cancel := getSignalCancel(ctx); cancel != nil {
			cancel()
		}

		// 使用独立的 context 进行 shutdown，避免因信号 context 已取消
		// 导致 samber/do 跳过 ShutdownerWithError 实现的实际清理逻辑
		// io.Closer / Shutdowner / ShutdownerWithError / ShutdownerWithContext / ShutdownerWithContextAndError will be called
		// 带 10 秒超时兜底，防止 shutdown 阶段无限阻塞
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		i.ShutdownWithContext(shutdownCtx)
		pterm.Debug.Println("zhongkui server process exit")
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		pterm.Error.Println(err)
	}
}

var describeCmd = &cobra.Command{
	Use:   "describe",
	Short: "Show configuration details",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the zhongkui server process",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var clearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear zhongkui data",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Database migration management",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create resources (user, tls, access-key)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	RootCmd.PersistentFlags().StringP("config", "c", "", "config file path (e.g. /etc/zhongkui/config.toml)")
	RootCmd.AddCommand(versionCmd)
	// describe
	describeCmd.AddCommand(describeConfigCmd)
	describeCmd.AddCommand(describeSchedulerCmd)
	RootCmd.AddCommand(describeCmd)
	// run
	runCmd.AddCommand(webCmd)
	runCmd.AddCommand(grpcCmd)
	runCmd.AddCommand(workerCmd)
	runCmd.AddCommand(captureCmd)
	RootCmd.AddCommand(runCmd)
	// clear
	clearCmd.AddCommand(clearDatabaseCmd)
	RootCmd.AddCommand(clearCmd)
	// migrate
	migrateCmd.AddCommand(migrateUpCmd)
	migrateCmd.AddCommand(migrateDownCmd)
	migrateCmd.AddCommand(migrateUpToCmd)
	migrateCmd.AddCommand(migrateDownToCmd)
	migrateCmd.AddCommand(migrateStatusCmd)
	migrateCmd.AddCommand(migrateDryRunCmd)
	migrateCmd.AddCommand(migrateDiffCmd)
	migrateCmd.AddCommand(migrateCheckCmd)
	migrateCmd.AddCommand(migrateCreateCmd)
	RootCmd.AddCommand(migrateCmd)
	// create
	createCmd.AddCommand(createUserCmd)
	createCmd.AddCommand(generateTLSCmd)
	createCmd.AddCommand(createAccessKeyCmd)
	createCmd.AddCommand(createCapturerCmd)
	createCmd.AddCommand(createPasswordBookCmd)
	RootCmd.AddCommand(createCmd)

	RootCmd.AddCommand(forceResetPasswordCmd)
}
