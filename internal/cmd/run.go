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
	_ "zhongkui.com/zhongkui/internal/tasks"
)

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Run the zhongkui http web api process",
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
	Short: "Run the zhongkui http grpc api process",
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
	Short: "Run the zhongkui worker Process",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, i := cmdToolBox(cmd)
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
		wg, ctx := errgroupx.WithContext(ctx)
		wg.GoNamed("server", server.Start)
		wg.GoNamed("scheduler", scheduler.Start)
		if err := wg.Wait(); err != nil {
			pterm.Error.Println(err.Error())
			return
		}
	},
}

var captureCmd = &cobra.Command{
	Use:   "capture",
	Short: "Run the zhongkui capture Process",
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
	captureCmd.Flags().Bool("stats-print", false, "enable periodic capture statistics printing to stdout")
	captureCmd.Flags().Duration("stats-interval", 5*time.Second, "interval between stats print lines (e.g. 5s, 10s, 1m)")
}
