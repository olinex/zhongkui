package cmd

import (
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"zhongkuisec.com/zhongkui/internal/manager/config"
	"zhongkuisec.com/zhongkui/internal/pkg/translate"
	"zhongkuisec.com/zhongkui/internal/provider"
)

var describeConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "cmd.describe.config.short",
	Run: func(cmd *cobra.Command, args []string) {
		descs := provider.DescribeLoggerConfig()
		descs.Join(config.DescribeConfig())
		descs.Join(provider.DescribeCryptoConfig())
		descs.Join(provider.DescribeDatabaseConfig())
		descs.Join(provider.DescribeEtcdConfig())
		descs.Join(provider.DescribeRedisConfig())
		descs.Join(provider.DescribeS3Config())
		descs.Join(provider.DescribeGrpcClientConfig())
		descs.Join(provider.DescribeGrpcServerConfig())
		descs.Join(provider.DescribePipelineConfig())
		descs.Join(provider.DescribeMailConfig())
		descs.Join(provider.DescribeHealthConfig())
		descs.Join(provider.DescribeTagEngineConfig())
		// 表头经 translate.T 按 key 本地化（cli.config.col-*）；配置项说明以结构体
		// comment 标签为唯一事实来源，英文翻译可追加到 en.toml 的 config.<path>
		if err := descs.Describe(func(key, fallback string) string { return translate.T(key, nil) }); err != nil {
			pterm.Error.Println(err.Error())
			return
		}
	},
}
