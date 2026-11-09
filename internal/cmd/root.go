package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pterm/pterm"
	"github.com/samber/do/v2"
	"github.com/spf13/cobra"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/translate"
	_ "zhongkuisec.com/zhongkui/internal/pkg/validator"
	"zhongkuisec.com/zhongkui/internal/provider"
)

const (
	// InjectorCtxName 是 samber/do 注入器在 context 中存储的键名。
	InjectorCtxName = "inj"
	// SignalCtxName 是信号监听停止函数在 context 中存储的键名。
	SignalCtxName = "sig"

	// annotationSkipInjectorInit 是 cobra.Command.Annotations 的键，标记该命令（含其父链）
	// 不需要初始化依赖注入器即可运行，例如 create default-config 这类"bootstrap"命令。
	annotationSkipInjectorInit = "zhongkui/skip-injector-init"
)

// shouldSkipInjectorInit 判断当前命令链是否标记了跳过注入器初始化。
// 沿 cmd.Parent() 向上查找，允许在父命令上统一标记（例如未来整个 create 组都不需要注入器）。
func shouldSkipInjectorInit(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if _, ok := c.Annotations[annotationSkipInjectorInit]; ok {
			return true
		}
	}
	return false
}

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

// RootCmd 是 zhongkui 的根命令，负责全局依赖注入启动与优雅关闭流程。
var RootCmd = &cobra.Command{
	Use:   "zhongkui",
	Short: "cmd.short",
	// 错误由 Execute 统一打印一次；不再由 cobra 重复打印错误与用法。
	SilenceErrors: true,
	SilenceUsage:  true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// bootstrap 命令（如 create default-config）无需 config.toml 即可运行，
		// 跳过所有依赖 config 的 provider 初始化，仅保留信号处理与 context 传递。
		if shouldSkipInjectorInit(cmd) {
			ctx := cmd.Context()
			ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
			ctx = context.WithValue(ctx, SignalCtxName, stop)
			cmd.SetContext(ctx)
			return nil
		}

		var i = do.New()
		ctx := cmd.Context()

		// 统一信号处理：监听 SIGINT/SIGTERM，收到信号时取消 context
		// 所有服务通过监听 context 取消来实现优雅关闭
		ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
		ctx = context.WithValue(ctx, SignalCtxName, stop)
		cmd.SetContext(ctx)

		provider.InjectOtelShutdowner(ctx, i)
		provider.InjectHealthServer(ctx, i)
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
		provider.InjectCacheRedis(i)
		provider.InjectPipelineRedis(i)
		provider.InjectAccessKeyPool(i)
		provider.InjectEtcdClient(ctx, i)
		provider.InjectTagEngine(i)
		provider.InjectObjectCache(i)
		provider.InjectSQLiteVTabPools(i)
		provider.InjectDatasourceWatcher(i)
		provider.InjectReporterGrpcServer(i)
		provider.InjectReporterGrpcClient(i)
		provider.InjectPipelineInspector(i)
		provider.InjectPipelineScheduler(ctx, i)
		provider.InjectPipelineRegistry(i)
		provider.InjectPipelineProcessorFactoriesRegistry(i)
		provider.InjectPipelineClient(i)
		provider.InjectPipelineServer(ctx, i)
		provider.InjectCaptureClient(i)
		provider.InjectMailSender(i)
		provider.InjectFeatureRegistry(i)
		provider.InjectMessagePool(i)
		provider.InjectJSEngine(ctx, i)
		provider.InjectAi(ctx, i)

		// 初始化密钥派生器
		_ = provider.MustInvokeKeyDerivation(i)
		// 初始化翻译器
		_ = provider.MustInvokeUniversalTranslator(i)
		// 必须在所有服务启动完成后调用，确保所有资源都被正确初始化
		_ = provider.MustInvokeOtelShutdowner(ctx, i)
		// 预解析消息渠道连接池，确保其在优雅关闭阶段由 samber/do 驱动其 Shutdown 关闭
		_ = provider.MustInvokeMessagePool(i)
		// 预解析 sqlite vtable 连接池（含激活），确保 sqlite.query 内置函数
		// 在启动后立即可用且池在优雅关闭阶段由 samber/do 驱动其 Shutdown 关闭
		_ = provider.MustInvokeSQLiteVTabPools(i)

		ctx = context.WithValue(ctx, InjectorCtxName, i)
		cmd.SetContext(ctx)
		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		// bootstrap 命令未初始化依赖注入器，仅需释放信号监听。
		if shouldSkipInjectorInit(cmd) {
			ctx := cmd.Context()
			if cancel := getSignalCancel(ctx); cancel != nil {
				cancel()
			}
			return nil
		}

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
		// ShutdownWithContext 返回的是执行报告（恒非 nil），不是 error；须以 Succeed 判定失败，
		// 否则正常退出也会被当成错误。
		report := i.ShutdownWithContext(shutdownCtx)
		if !report.Succeed {
			return errors.New(report.Error())
		}
		pterm.Debug.Println(translate.T("cli.root.process-exit", nil))
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

// Execute 本地化命令树后执行根命令，执行失败时打印错误并以非零状态退出。
func Execute() {
	// 在命令执行前将命令描述与标志说明本地化为 CLI 语言
	// 因为 --help 等路径不会触发 PersistentPreRunE，这里必须提前完成翻译
	localizeCmdTree()
	if err := RootCmd.Execute(); err != nil {
		pterm.Error.Println(err)
		os.Exit(1)
	}
}

var describeCmd = &cobra.Command{
	Use:   "describe",
	Short: "cmd.describe.short",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "cmd.run.short",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var clearCmd = &cobra.Command{
	Use:   "clear",
	Short: "cmd.clear.short",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "cmd.migrate.short",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "cmd.create.short",
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
	RootCmd.PersistentFlags().StringP("config", "c", "", "flag.config")
	// 手动注册 help 标志以覆盖 cobra 自动生成的英文描述 "help for zhongkui"
	RootCmd.PersistentFlags().BoolP("help", "h", false, "flag.help")
	RootCmd.AddCommand(versionCmd)
	// describe
	describeCmd.AddCommand(describeConfigCmd)
	describeCmd.AddCommand(describeSchedulerCmd)
	describeCmd.AddCommand(describeDepsCmd)
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
	createCmd.AddCommand(createDocsCmd)
	createCmd.AddCommand(createDefaultConfigCmd)
	RootCmd.AddCommand(createCmd)
	// sync
	syncCmd.AddCommand(syncDocsCmd)
	syncCmd.AddCommand(syncApisCmd)
	syncCmd.AddCommand(syncSchemaCmd)
	syncCmd.AddCommand(syncCapabilitiesCmd)
	syncCmd.AddCommand(syncEmbeddingsCmd)
	syncCmd.AddCommand(syncEmbeddingDimensionsCmd)
	RootCmd.AddCommand(syncCmd)

	// s3
	s3LifecycleCmd.AddCommand(s3LifecycleApplyCmd)
	s3LifecycleCmd.AddCommand(s3LifecycleStatusCmd)
	s3LifecycleCmd.AddCommand(s3LifecycleRemoveCmd)
	s3Cmd.AddCommand(s3LifecycleCmd)
	RootCmd.AddCommand(s3Cmd)

	RootCmd.AddCommand(forceResetPasswordCmd)
}
