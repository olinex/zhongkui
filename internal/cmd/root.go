package cmd

import (
	"context"

	"github.com/pterm/pterm"
	"github.com/samber/do/v2"
	"github.com/spf13/cobra"
	"zhongkui.com/zhongkui/internal/pkg/validator"
	"zhongkui.com/zhongkui/internal/provider"
)

const (
	INJECTOR_CTX_NAME = "inj"
)

func cmdToolBox(cmd *cobra.Command) (context.Context, do.Injector) {
	ctx := cmd.Context()
	return ctx, ctx.Value(INJECTOR_CTX_NAME).(do.Injector)
}

var RootCmd = &cobra.Command{
	Use:   "help",
	Short: "The zhongkui root command lines",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var i = do.New()
		ctx := cmd.Context()
		provider.InjectOtelShutdowner(ctx, i)
		_ = provider.MustInvokeOtelShutdowner(ctx, i)

		provider.InjectCommand(i, cmd)
		provider.InjectViper(i)
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
		provider.InjectPipelineClient(i)
		provider.InjectPipelineServer(ctx, i)
		provider.InjectCaptureClient(i)

		ctx = context.WithValue(ctx, INJECTOR_CTX_NAME, i)
		cmd.SetContext(ctx)
		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		ctx, i := cmdToolBox(cmd)
		i.ShutdownWithContext(ctx)
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
	RootCmd.AddCommand(describeConfigCmd)
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
	RootCmd.AddCommand(createCmd)

	RootCmd.AddCommand(forceResetPasswordCmd)
}
