package cmd

import (
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/manager/routes"
	"zhongkui.com/zhongkui/internal/pkg/errgroupx"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/provider"
	"zhongkui.com/zhongkui/internal/reporter"
	"zhongkui.com/zhongkui/internal/worker"
)

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "运行 zhongkui HTTP Web API 进程",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, i := cmdToolBox(cmd)
		if err := routes.RouteRegister(ctx, i); err != nil {
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
	Short: "运行 zhongkui gRPC API 进程",
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
	Short: "运行 zhongkui 工作节点进程",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, i := cmdToolBox(cmd)
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

		// 启动所有注册的分布式处理器
		wg, ctx := errgroupx.WithContext(ctx)
		wg.GoNamed("server", func() error { return server.Start(ctx) })
		wg.GoNamed("scheduler", func() error { return scheduler.Start(ctx) })

		if err := wg.Wait(); err != nil {
			pterm.Error.Println(err.Error())
			return
		}
	},
}

var captureCmd = &cobra.Command{
	Use:   "capture",
	Short: "运行 zhongkui 抓包进程",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, i := cmdToolBox(cmd)
		v := provider.MustInvokeViper(i)
		client := provider.MustInvokeCaptureClient(i)
		sessionID := serializer.NewHexUUIDv4()

		// 命令行调试统计：检查是否开启定时打印抓包指标到 stdout
		if sPrint := v.GetBool("stats-print"); sPrint {
			sInterval := v.GetDuration("stats-interval")
			client.EnableStats(sInterval)
			pterm.Debug.Printf(
				"capture stats enabled, printing every %s to stdout\n",
				sInterval,
			)
		}

		if err := client.Run(ctx, sessionID); err != nil {
			pterm.Error.Println(err.Error())
			return
		}
	},
}

func init() {
	captureCmd.Flags().Bool("stats-print", false, "启用周期性抓包统计输出到标准输出")
	captureCmd.Flags().Duration("stats-interval", 5*time.Second, "统计输出的时间间隔（如 5s、10s、1m）")
}
