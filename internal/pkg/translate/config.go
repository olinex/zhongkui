package translate

import (
	"github.com/spf13/viper"
	"zhongkui.com/zhongkui/internal/pkg/config"
)

// PREFIX 是配置前缀，用于 Viper 配置管理
const PREFIX = "i18n"

// Config 结构体定义了国际化（i18n）的配置参数
// 这些参数通过 Viper 从配置文件中读取
type Config struct {
	// RootPath 是本地化文件存储的根路径
	// 默认值: "locales"
	// 参考: https://pkg.go.dev/github.com/nicksnyder/go-i18n/v2/i18n#Bundle-ParseGlob
	RootPath string `mapstructure:"rootPath" default:"locales" comment:"root path for i18n locale files (ref: https://pkg.go.dev/github.com/nicksnyder/go-i18n/v2/i18n#Bundle-ParseGlob)"`
	
	// DefaultLocale 是默认的语言/区域设置
	// 默认值: "en" (英语)
	// 参考: https://pkg.go.dev/github.com/nicksnyder/go-i18n/v2/i18n#Bundle-Locale
	DefaultLocale Language `mapstructure:"defaultLocale" default:"en" comment:"default locale/language for i18n (ref: https://pkg.go.dev/github.com/nicksnyder/go-i18n/v2/i18n#Bundle-Locale)"`
	
	// SupportLocales 是支持的语言/区域设置列表
	// 默认值: ["en", "zh-Hans"] (英语和简体中文)
	// 参考: https://pkg.go.dev/github.com/nicksnyder/go-i18n/v2/i18n#Bundle-Locale
	SupportLocales []Language `mapstructure:"supportLocales" default:"[\"en\", \"zh-Hans\"]" comment:"list of supported locales/languages (ref: https://pkg.go.dev/github.com/nicksnyder/go-i18n/v2/i18n#Bundle-Locale)"`
}

// DescribeConfig 返回配置的描述信息，用于配置文档生成
// 返回值:
//   - config.ConfigDescriptions: 配置描述信息
func DescribeConfig() config.ConfigDescriptions {
	return config.DescribeConfig[Config](PREFIX)
}

// ExtractConfig 从 Viper 实例中提取国际化配置
// 参数:
//   - v: Viper 配置实例
// 返回值:
//   - *Config: 提取的配置实例
//   - error: 提取过程中的错误
func ExtractConfig(v *viper.Viper) (*Config, error) {
	return config.ExtractConfig[Config](v, PREFIX)
}
