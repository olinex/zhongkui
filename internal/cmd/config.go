package cmd

import (
	"github.com/spf13/cobra"
	"zhongkui.com/zhongkui/internal/depend"
)

var describeConfigCmd = &cobra.Command{
	Use:   "describe-config",
	Short: "Show default config descriptions",
	Run: func(cmd *cobra.Command, args []string) {
		descs := depend.DescribeLoggerConfig()
		descs.Join(depend.DescribeDatabaseConfig())
		descs.Join(depend.DescribeEtcdConfig())
		descs.Join(depend.DescribeRedisConfig())
		descs.Join(depend.DescribeS3Config())
		descs.Join(depend.DescribeGrpcClientConfig())
		descs.Join(depend.DescribeGrpcServerConfig())
		descs.Join(depend.DescribePipelineConfig())
		descs.Join(depend.DescribeTranslateConfig())
		if err := descs.Describe(); err != nil {
			panic(err)
		}
	},
}
