// 本文件定义标签识别引擎的配置项。配置项由 Viper 以 "tagengine" 前缀加载，
// 通过 provider 注入 Engine；默认值与说明由 default / comment 标签统一推导，
// 供 `zki describe config`、默认配置生成与文档参考复用。

package tagengine

import (
	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"

	"zhongkuisec.com/zhongkui/internal/pkg/config"
)

// Prefix 是标签引擎配置在配置文件中的前缀。
const Prefix = "tagengine"

// Config 标签识别引擎（Rego）配置。
type Config struct {
	// S3CryptoMaxBytes crypto.reveal 读取 S3Crypto 字段正文的最大字节数，
	// 超出时内置函数报错（生产识别中该规则体不满足，dry-run 中返回错误）。
	S3CryptoMaxBytes int64 `mapstructure:"s3cryptoMaxBytes" default:"8388608" validate:"gt=0" comment:"rego 规则解密 S3 加密正文（crypto.reveal 的 s3crypto 字段）的最大字节数，超出时报错"`
}

// ExtractConfig 从 Viper 配置管理器中提取标签引擎配置。
func ExtractConfig(viper *viper.Viper, validate *validator.Validate) (*Config, error) {
	return config.ExtractConfig[Config](viper, validate, Prefix)
}

// DescribeConfig 返回标签引擎配置的结构化描述信息。
func DescribeConfig() config.ConfigDescriptions {
	return config.DescribeConfig[Config](Prefix)
}
