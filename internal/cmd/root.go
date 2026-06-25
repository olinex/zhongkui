package cmd

import (
	"context"
	"fmt"

	"github.com/samber/do/v2"
	"github.com/spf13/cobra"
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
		provider.InjectCommand(i, cmd)
		provider.InjectValidate(i)
		// 强制性注入 validator.Validate 实例，确保在其他组件中可以使用
		provider.MustInvokeValidate(i)
		provider.InjectS3(i)
		provider.InjectGin(i)
		provider.InjectViper(i)
		provider.InjectI18n(i)
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
		_, i := cmdToolBox(cmd)
		// // 优雅关闭 OTEL，确保未导出的数据被刷新
		// if rt, err := provider.InvokeOtel(i); err == nil && rt != nil {
		// 	rt.Shutdown(ctx)
		// }
		i.Shutdown()
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
	}
}

func init() {
	RootCmd.PersistentFlags().StringP("config", "c", "", "config file path (e.g. /etc/zhongkui/config.toml)")
	RootCmd.AddCommand(runCmd)
	RootCmd.AddCommand(describeConfigCmd)
	RootCmd.AddCommand(migrateCmd)
	RootCmd.AddCommand(clearCmd)
	RootCmd.AddCommand(createUserCmd)
	RootCmd.AddCommand(generateTLSCmd)
	RootCmd.AddCommand(forceResetPasswordCmd)
	RootCmd.AddCommand(versionCmd)
}
