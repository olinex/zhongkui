package cmd

import (
	"context"
	"time"

	"github.com/pterm/pterm"
	"github.com/samber/do/v2"
	"github.com/spf13/cobra"
	"zhongkui.com/zhongkui/internal/datasource"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/pkg/runtime"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/translate"
	"zhongkui.com/zhongkui/internal/provider"
	"zhongkui.com/zhongkui/internal/reporter"
	"zhongkui.com/zhongkui/internal/worker"
)

// startHealth 在独立 goroutine 中启动健康探针服务，与主服务主体并发运行。
// 服务名用于 etcd 服务目录标识；构建失败时仅记录日志，不影响主服务启动。
func startHealth(ctx context.Context, i do.Injector, serviceName string) {
	hs, err := provider.NewHealthService(ctx, i, serviceName)
	if err != nil {
		pterm.Error.Println(err.Error())
		return
	}
	go func() {
		if err := hs.Start(ctx); err != nil {
			pterm.Error.Println(err.Error())
		}
	}()
}

// startAiWatch 在独立 goroutine 中启动 AI 运行时配置变更监听（etcd 广播触发热重载），
// 与主服务主体并发运行。运行时解析失败或无 etcd 客户端时仅记录日志，不影响主服务启动。
func startAiWatch(ctx context.Context, i do.Injector) {
	rt, err := provider.InvokeAi(ctx, i)
	if err != nil {
		pterm.Error.Println(err.Error())
		return
	}
	go func() {
		if err := rt.Start(ctx); err != nil {
			pterm.Error.Println(err.Error())
		}
	}()
}

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "cmd.run.web.short",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, i := cmdToolBox(cmd)
		if err := manager.RouteRegister(ctx, i); err != nil {
			pterm.Error.Println(err.Error())
			return
		}
		startHealth(ctx, i, provider.HealthServiceNameWeb)
		startAiWatch(ctx, i)
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
		startHealth(ctx, i, provider.HealthServiceNameGrpc)
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
		worker.Registry(ctx, i)
		// 与 web 进程一致，启动时重载第三方数据源：扫描已启用数据源注册为各命名空间的
		// SQLite vtable，保证 worker 进程内 sqlite.query 可按裸名查询。缺失该步骤时
		// 规则中的第三方数据源查询全部失败（no such table），标签识别报错后透传、
		// 真实运行无法命中标签（试运行走 web 进程故正常）。
		sqliteVTabPools := provider.MustInvokeSQLiteVTabPools(i)
		coreDB := provider.MustInvokeCoreDatabase(i)
		if err := datasource.Reload(ctx, coreDB, sqliteVTabPools); err != nil {
			pterm.Error.Println(err.Error())
			return
		}
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
		datasourceWatcher, err := provider.InvokeDatasourceWatcher(i)
		if err != nil {
			pterm.Error.Println(err.Error())
			return
		}
		// 统一服务生命周期：任务队列 Server、etcd 选举 Scheduler、
		// 分布式常驻 Processor、标签识别引擎（监听规则变更）与数据源变更
		// Watcher（同步 web 进程数据源增删改/启停到本地 vtable 池）均通过
		// runtime.Group 编排启动与关闭，Redis 队列故障不再连带影响流式处理器。
		// 单个处理器创建失败仅记录并跳过。
		healthService, err := provider.NewHealthService(ctx, i, provider.HealthServiceNameWorker)
		if err != nil {
			pterm.Error.Println(err.Error())
			return
		}
		services := []runtime.Service{
			healthService,
			tagEngine,
			datasourceWatcher,
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
