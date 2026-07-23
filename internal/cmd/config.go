package cmd

import (
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/provider"
)

var describeConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Show default config descriptions",
	Run: func(cmd *cobra.Command, args []string) {
		descs := provider.DescribeLoggerConfig()
		descs.Join(manager.DescribeConfig())
		descs.Join(provider.DescribeCryptoConfig())
		descs.Join(provider.DescribeDatabaseConfig())
		descs.Join(provider.DescribeEtcdConfig())
		descs.Join(provider.DescribeRedisConfig())
		descs.Join(provider.DescribeS3Config())
		descs.Join(provider.DescribeGrpcClientConfig())
		descs.Join(provider.DescribeGrpcServerConfig())
		descs.Join(provider.DescribePipelineConfig())
		descs.Join(provider.DescribeTranslateConfig())
		if err := descs.Describe(); err != nil {
			pterm.Error.Println(err.Error())
			return
		}
	},
}
