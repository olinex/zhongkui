package key

import (
	"github.com/spf13/viper"
	"zhongkui.com/zhongkui/internal/pkg/config"
)

const PREFIX = "crypto"

// Config 包含密码学相关配置。
type Config struct {
	Source           string `mapstructure:"source" default:"config" binding:"oneof=config kms" comment:"crypto key source, config or kms"`
	ParentSecretKey  string `mapstructure:"parent_secret_key" comment:"parent secret key for kms source"`
	PasswordBookPath string `mapstructure:"password_book_path" comment:"password book path for config source"`
}

func DescribeConfig() config.ConfigDescriptions {
	return config.DescribeConfig[Config](PREFIX)
}

func ExtractConfig(v *viper.Viper) (*Config, error) {
	return config.ExtractConfig[Config](v, PREFIX)
}
