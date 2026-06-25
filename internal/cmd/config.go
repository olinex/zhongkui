package cmd

import (
	"github.com/spf13/cobra"
	"zhongkui.com/zhongkui/internal/provider"
)

var describeConfigCmd = &cobra.Command{
	Use:   "describe-config",
	Short: "Show default config descriptions",
	RunE: func(cmd *cobra.Command, args []string) error {
		descs := provider.DescribeLoggerConfig()
		descs.Join(provider.DescribeDatabaseConfig())
		descs.Join(provider.DescribeEtcdConfig())
		descs.Join(provider.DescribeRedisConfig())
		descs.Join(provider.DescribeS3Config())
		descs.Join(provider.DescribeGrpcClientConfig())
		descs.Join(provider.DescribeGrpcServerConfig())
		descs.Join(provider.DescribePipelineConfig())
		descs.Join(provider.DescribeTranslateConfig())
		return descs.Describe()
	},
}
