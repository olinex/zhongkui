package cmd

import (
	"time"

	"github.com/pterm/pterm"
	"github.com/samber/do/v2"
	"github.com/spf13/cobra"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/manager/router"
	"zhongkui.com/zhongkui/internal/notify"
	"zhongkui.com/zhongkui/internal/pkg/runtime"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/translate"
	"zhongkui.com/zhongkui/internal/provider"
	"zhongkui.com/zhongkui/internal/reporter"
	"zhongkui.com/zhongkui/internal/worker"
)

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "cmd.run.web.short",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, i := cmdToolBox(cmd)
		seedNotificationTemplates(i)
		if err := router.RouteRegister(ctx, i); err != nil {
			pterm.Error.Println(err.Error())
			return
		}
		if err := manager.RunHttpService(ctx, i); err != nil {
			pterm.Error.Println(err.Error())
			return
		}
	},
}

var grpcCmd = &cobra.Command{
	Use:   "grpc",
	Short: "cmd.run.grpc.short",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, i := cmdToolBox(cmd)
		_ = provider.MustInvokeKeyDerivation(i)
		srv := reporter.NewReportService(i)
		if err := srv.Run(ctx); err != nil {
			pterm.Error.Println(err.Error())
			return
		}
	},
}

var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "cmd.run.worker.short",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, i := cmdToolBox(cmd)
		seedNotificationTemplates(i)
		worker.Registry(ctx, i)
		server, err := provider.InvokePipelineServer(i)
		if err != nil {
			pterm.Error.Println(err.Error())
			return
		}
		scheduler, err := provider.InvokePipelineScheduler(i)
		if err != nil {
			pterm.Error.Println(err.Error())
			return
		}
		factories, err := provider.InvokePipelineProcessorFactoriesRegistry(i)
		if err != nil {
			pterm.Error.Println(err.Error())
			return
		}
		tagEngine, err := provider.InvokeTagEngine(i)
		if err != nil {
			pterm.Error.Println(err.Error())
			return
		}
		// 统一服务生命周期：任务队列 Server、etcd 选举 Scheduler、
		// 分布式常驻 Processor 与标签识别引擎（监听规则变更）均通过
		// runtime.Group 编排启动与关闭，Redis 队列故障不再连带影响流式处理器。
		// 单个处理器创建失败仅记录并跳过。
		services := []runtime.Service{
			tagEngine,
			runtime.NewPipelineService(server),
			runtime.NewElectionService(scheduler),
		}
		for name, factory := range factories {
			processor, err := factory(ctx)
			if err != nil {
				pterm.Error.Println(translate.T("cli.run.processor-create-failed", map[string]any{"Name": name, "Err": err}))
				continue
			}
			services = append(services, runtime.NewProcessorService(name, processor))
		}

		if err := runtime.NewGroup(services...).Run(ctx); err != nil {
			pterm.Error.Println(err.Error())
			return
		}
	},
}

var captureCmd = &cobra.Command{
	Use:   "capture",
	Short: "cmd.run.capture.short",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, i := cmdToolBox(cmd)
		v := provider.MustInvokeViper(i)
		client := provider.MustInvokeCaptureClient(i)
		sessionID := serializer.NewHexUUIDv4()

		// 命令行调试统计：检查是否开启定时打印抓包指标到 stdout
		if sPrint := v.GetBool("stats-print"); sPrint {
			sInterval := v.GetDuration("stats-interval")
			client.EnableStats(sInterval)
			pterm.Debug.Println(translate.T("cli.run.capture-stats-enabled", map[string]any{"Interval": sInterval}))
		}

		if err := client.Run(ctx, sessionID); err != nil {
			pterm.Error.Println(err.Error())
			return
		}
	},
}

func init() {
	captureCmd.Flags().Bool("stats-print", false, "flag.run.capture.stats-print")
	captureCmd.Flags().Duration("stats-interval", 5*time.Second, "flag.run.capture.stats-interval")
}

// seedNotificationTemplates 在服务启动前，为既有命名空间幂等回填事件模板注册表中的内置
// 默认通知模板（source=default），保证老命名空间具备可用的默认模板。失败仅记录警告，不
// 阻断启动；回填本身幂等（ON CONFLICT DO NOTHING），可安全重复执行。
// seedNotificationTemplates backfills, before a serving command starts, the built-in
// default notification templates (source=default) for all existing namespaces so older
// namespaces have usable defaults. A failure is only logged as a warning and does not
// block startup; the backfill is idempotent (ON CONFLICT DO NOTHING) and safe to rerun.
func seedNotificationTemplates(i do.Injector) {
	db := provider.MustInvokeCoreDatabase(i)
	logger := provider.MustInvokeDatabaseLogger(i)
	if err := notify.BackfillDefaultTemplates(db, logger); err != nil {
		logger.Warnf("notify: failed to backfill default templates: %v", err)
	}
}
