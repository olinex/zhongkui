package cmd

import (
	"context"
	"os"

	"github.com/spf13/cobra"
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
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Run the zhongkui http web api process",
	Run: func(cmd *cobra.Command, args []string) {
		_, i := cmdToolBox(cmd)
		if err := routes.RouteRegister(i); err != nil {
			panic(err)
		}
		if err := manager.RunHttpService(cmd.Context(), i); err != nil {
			panic(err)
		}
	},
}

var grpcCmd = &cobra.Command{
	Use:   "grpc",
	Short: "Run the zhongkui http grpc api process",
	Run: func(cmd *cobra.Command, args []string) {
		_, i := cmdToolBox(cmd)
		srv := reporter.NewReportService(i)
		if err := srv.Run(); err != nil {
			panic(err)
		}
	},
}

var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Run the zhongkui worker Process",
	Run: func(cmd *cobra.Command, args []string) {
		_, i := cmdToolBox(cmd)
		server, err := provider.InvokePipelineServer(i)
		if err != nil {
			panic(err)
		}
		if err := server.Start(); err != nil {
			panic(err)
		}
	},
}

var captureCmd = &cobra.Command{
	Use:   "capture",
	Short: "Run the zhongkui capture Process",
	Run: func(cmd *cobra.Command, args []string) {
		_, i := cmdToolBox(cmd)
		log := provider.MustInvokeCaptureLogger(i)
		client := provider.MustInvokeCaptureClient(i)

		sessionID := serializer.NewHexUUIDv4()
		log.Infof("starting zhongkui capture %s", sessionID.String())

		if err := client.Run(context.Background(), sessionID); err != nil {
			log.DPanic(err)
			os.Exit(1)
		}

		log.Infof("shutting down zhongkui capture %s", sessionID.String())
		os.Exit(0)
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
	runCmd.AddCommand(webCmd)
	runCmd.AddCommand(grpcCmd)
	runCmd.AddCommand(workerCmd)
	runCmd.AddCommand(captureCmd)
	runCmd.AddCommand(singletonCmd)
}
