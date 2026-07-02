package cmd

import (
	"errors"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/manager/routes"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/provider"
	"zhongkui.com/zhongkui/internal/reporter"
	_ "zhongkui.com/zhongkui/internal/tasks"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the zhongkui server process",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Run the zhongkui http web api process",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, i := cmdToolBox(cmd)
		if err := routes.RouteRegister(ctx, i); err != nil {
			return err
		}
		return manager.RunHttpService(ctx, i)
	},
}

var grpcCmd = &cobra.Command{
	Use:   "grpc",
	Short: "Run the zhongkui http grpc api process",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, i := cmdToolBox(cmd)
		srv := reporter.NewReportService(i)
		return srv.Run(ctx)
	},
}

var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Run the zhongkui worker Process",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, i := cmdToolBox(cmd)
		server, err := provider.InvokePipelineServer(i)
		if err != nil {
			return err
		}
		scheduler, err := provider.InvokePipelineScheduler(i)
		if err != nil {
			return err
		}
		wg, ctx := errgroup.WithContext(ctx)
		wg.Go(server.Start)
		wg.Go(scheduler.Start)
		return wg.Wait()
	},
}

var captureCmd = &cobra.Command{
	Use:   "capture",
	Short: "Run the zhongkui capture Process",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, i := cmdToolBox(cmd)
		log := provider.MustInvokeCaptureLogger(i)
		client := provider.MustInvokeCaptureClient(i)

		sessionID := serializer.NewHexUUIDv4()
		log.Ctx(ctx).Infof("starting zhongkui capture %s", sessionID.String())

		if err := client.Run(ctx, sessionID); err != nil {
			log.DPanic(err)
			return errors.New("capture client run failed")
		}

		log.Ctx(ctx).Infof("shutting down zhongkui capture %s", sessionID.String())
		return nil
	},
}

func init() {
	runCmd.AddCommand(webCmd)
	runCmd.AddCommand(grpcCmd)
	runCmd.AddCommand(workerCmd)
	runCmd.AddCommand(captureCmd)
}
