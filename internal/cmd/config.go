package cmd

import (
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/provider"
)

var describeConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "显示默认配置项说明",
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
		// 将配置项说明本地化为 CLI 语言，无对应翻译时回退为 comment 标签中的中文
		for idx := range descs {
			descs[idx].Comment = cliT("config."+descs[idx].Path, descs[idx].Comment)
		}
		if err := descs.Describe(func(key, fallback string) string { return cliT(key, fallback) }); err != nil {
			pterm.Error.Println(err.Error())
			return
		}
	},
}
