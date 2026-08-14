// Package key 提供密钥管理和密钥派生相关的功能。
//
// 本包实现了基于 HKDF（HMAC-based Key Derivation Function）的密钥派生机制，
// 支持从主密钥（master key）派生出多个子密钥。主要组件包括：
//
//   - PasswordBook: 密码本，提供用于密钥派生的盐值数据
//   - SecretKeyProvider: 主密钥提供者接口，支持从配置文件或 KMS 获取主密钥
//   - KeyDerivation: 密钥派生器，使用 HKDF-SHA256 从主密钥和密码本派生子密钥
//
// 密钥派生流程：
//  1. 从 SecretKeyProvider 获取主密钥（master key）
//  2. 从 PasswordBook 的指定偏移位置读取盐值（salt）
//  3. 使用 HKDF-SHA256 算法派生指定长度的子密钥
//
// 安全注意事项：
//   - 主密钥应妥善保管，生产环境建议使用 KMS 服务
//   - 密码本应定期更换，避免使用默认密码本
//   - 不同的子密钥应使用不同的偏移位置，确保密钥独立性
package key

import (
	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
	"zhongkui.com/zhongkui/internal/pkg/config"
)

// PREFIX 定义配置项的前缀，所有密钥相关配置项都以 "crypto." 开头
// 例如：crypto.source, crypto.parent_secret_key 等
const Prefix = "crypto"

// Config 定义密钥管理相关的配置项。
//
// 配置项说明：
//   - Source: 主密钥来源，支持 "config"（配置文件）或 "kms"（密钥管理服务）
//   - ParentSecretKey: 当 Source 为 "config" 时，主密钥的明文值
//   - PasswordBookPath: 密码本文件路径，为空时使用内置默认密码本
//
// 安全建议：
//   - 生产环境应将 Source 设置为 "kms" 并使用专业的密钥管理服务
//   - 避免在配置文件中明文存储 ParentSecretKey
//   - 密码本文件应设置严格的文件权限（如 0600）
type Config struct {
	// Source 主密钥来源
	// 可选值：
	//   - "config": 从配置文件的 ParentSecretKey 字段读取（不推荐用于生产）
	//   - "kms": 从密钥管理服务获取（推荐用于生产环境）
	// 默认值："config"
	Source string `mapstructure:"source" default:"config" binding:"oneof=config kms" comment:"主密钥来源：config（配置文件）或 kms（密钥管理服务）"`

	// ParentSecretKey 主密钥（仅当 Source="config" 时有效）
	// 这是用于派生子密钥的根密钥，应妥善保管
	// 生产环境强烈建议使用 KMS 而非此字段
	ParentSecretKey string `mapstructure:"parent_secret_key" comment:"主密钥明文值（仅当 source 为 config 时使用）"`

	// PasswordBookPath 密码本文件路径
	// 密码本是一个固定大小（65536 字节）的随机数据文件，用于提供密钥派生的盐值
	// 为空时使用内置的默认密码本（不推荐用于生产）
	// 可通过 'zki create password-book' 命令生成新的密码本
	PasswordBookPath string `mapstructure:"password_book_path" comment:"密码本文件路径，用于提供密钥派生的盐值"`
}

// DescribeConfig 返回配置项的描述信息，用于生成配置文档
func DescribeConfig() config.ConfigDescriptions {
	return config.DescribeConfig[Config](Prefix)
}

// ExtractConfig 从 viper 实例中提取密钥配置
//
// 参数：
//   - v: viper 实例，通常包含从配置文件和命令行参数加载的配置
//
// 返回值：
//   - *Config: 提取的配置对象
//   - error: 配置提取或验证失败时返回错误
func ExtractConfig(viper *viper.Viper, validate *validator.Validate) (*Config, error) {
	return config.ExtractConfig[Config](viper, validate, Prefix)
}
