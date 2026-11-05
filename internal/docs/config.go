package docs

import (
	// 标准库
	"sort"
	"strings"

	// 项目内部
	managerconfig "zhongkuisec.com/zhongkui/internal/manager/config"
	"zhongkuisec.com/zhongkui/internal/pkg/config"
	"zhongkuisec.com/zhongkui/internal/provider"
)

// configReferenceProjection 生成配置参考页：复用 `zki describe config` 的数据源与顺序口径。
func configReferenceProjection() Projection {
	descs := provider.DescribeLoggerConfig()
	descs.Join(managerconfig.DescribeConfig())
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
	return Projection{
		Title:       "配置参考",
		Summary:     "列出全部配置项、类型、默认值与说明，数据源与 `zki describe config` 一致。",
		Roles:       []Role{RoleOps, RoleDev},
		Scene:       "需要编写或核对 config.toml、排查配置加载问题时阅读。",
		Output:      GeneratedRoot + "/reference/config.md",
		SourceLabel: "internal/pkg/config",
		Content:     renderConfig(descs),
	}
}

// renderConfig 生成配置参考正文，按配置路径排序。
func renderConfig(descs config.ConfigDescriptions) string {
	sort.Slice(descs, func(i, j int) bool { return descs[i].Path < descs[j].Path })
	var b strings.Builder
	b.WriteString("## 配置项\n\n")
	b.WriteString("配置项说明以结构体 `comment` 标签为唯一事实来源；默认值来自 `default` 标签。\n\n")
	b.WriteString("| 路径 | 类型 | 默认值 | 说明 |\n| --- | --- | --- | --- |\n")
	for _, desc := range descs {
		b.WriteString("| `")
		b.WriteString(mdCell(desc.Path))
		b.WriteString("` | `")
		b.WriteString(mdCell(desc.Kind))
		b.WriteString("` | `")
		b.WriteString(mdCell(desc.Default))
		b.WriteString("` | ")
		b.WriteString(mdCell(desc.Comment))
		b.WriteString(" |\n")
	}
	return b.String()
}
