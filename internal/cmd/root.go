package cmd

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"github.com/pterm/pterm"
	"github.com/samber/do/v2"
	"github.com/spf13/cobra"
	"zhongkui.com/zhongkui/internal/pkg/base"
	_ "zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/translate"
	_ "zhongkui.com/zhongkui/internal/pkg/validator"
	"zhongkui.com/zhongkui/internal/provider"
)

const (
	InjectorCtxName = "inj"
	SignalCtxName   = "sig"
)

func cmdToolBox(cmd *cobra.Command) (context.Context, do.Injector) {
	ctx := cmd.Context()
	return ctx, ctx.Value(InjectorCtxName).(do.Injector)
}

// getSignalCancel 从 context 中获取信号监听停止函数
func getSignalCancel(ctx context.Context) context.CancelFunc {
	if cancel, ok := ctx.Value(SignalCtxName).(context.CancelFunc); ok {
		return cancel
	}
	return nil
}

var RootCmd = &cobra.Command{
	Use:   "zhongkui",
	Short: "zhongkui 根命令",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var i = do.New()
		ctx := cmd.Context()

		// 统一信号处理：监听 SIGINT/SIGTERM，收到信号时取消 context
		// 所有服务通过监听 context 取消来实现优雅关闭
		ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
		ctx = context.WithValue(ctx, SignalCtxName, stop)
		cmd.SetContext(ctx)

		provider.InjectOtelShutdowner(ctx, i)

		provider.InjectCommand(i, cmd)
		provider.InjectViper(i)
		provider.InjectKeyDerivation(i)
		provider.InjectUniversalTranslator(ctx, i)
		provider.InjectS3(i)
		provider.InjectGin(i)
		provider.InjectRbac(i)
		provider.InjectDefaultLogger(i)
		provider.InjectApiLogger(i)
		provider.InjectDatabaseLogger(i)
		provider.InjectCaptureLogger(i)
		provider.InjectEtcdLogger(i)
		provider.InjectGrpcLogger(i)
		provider.InjectPipelineLogger(i)
		provider.InjectCoreDatabase(i)
		provider.InjectRawDatabase(i)
		provider.InjectCoreSchemaManager(i)
		provider.InjectRawSchemaManager(i)
		provider.InjectTagEngine(i)
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

		// 初始化翻译器
		_ = provider.MustInvokeUniversalTranslator(i)
		// 必须在所有服务启动完成后调用，确保所有资源都被正确初始化
		_ = provider.MustInvokeOtelShutdowner(ctx, i)

		ctx = context.WithValue(ctx, InjectorCtxName, i)
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
		pterm.Debug.Println(cliT("cli.root.process-exit", "zhongkui 服务进程已退出"))
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func Execute() {
	// 在命令执行前将命令描述与标志说明本地化为 CLI 语言
	// 因为 --help 等路径不会触发 PersistentPreRunE，这里必须提前完成翻译
	localizeCmdTree()
	if err := RootCmd.Execute(); err != nil {
		pterm.Error.Println(err)
	}
}

var describeCmd = &cobra.Command{
	Use:   "describe",
	Short: "显示配置详情",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "运行 zhongkui 服务进程",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var clearCmd = &cobra.Command{
	Use:   "clear",
	Short: "清除 zhongkui 数据",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "数据库迁移管理",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "创建资源（用户、TLS 证书、访问密钥）",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	if base.Env.Debug {
		pterm.EnableDebugMessages()
		pterm.Debug.Println(translate.T("cli.base.pterm-debug", nil))
	}

	// 使用中文 help 命令替换 cobra 默认的英文版本
	RootCmd.SetHelpCommand(helpCmd)
	RootCmd.PersistentFlags().StringP("config", "c", "", "配置文件路径（如 /etc/zhongkui/config.toml）")
	// 手动注册 help 标志以覆盖 cobra 自动生成的英文描述 "help for zhongkui"
	RootCmd.PersistentFlags().BoolP("help", "h", false, "显示帮助信息")
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
	clearCmd.AddCommand(clearEtcdCmd)
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
	createCmd.AddCommand(generateTestcaseCmd)
	RootCmd.AddCommand(createCmd)

	RootCmd.AddCommand(forceResetPasswordCmd)
}
