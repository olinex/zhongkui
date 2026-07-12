package s3

import (
	"github.com/spf13/viper"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
)

// PREFIX 是S3配置在配置文件中的前缀
// 用于在Viper配置管理器中标识S3相关的配置项
const (
	PREFIX            = "s3"
	CRYPTO_KEY_OFFSET = 43036
	CRYPTO_KEY_LENGTH = 64
)

// Config 是S3兼容对象存储的配置结构体
// 包含连接到S3兼容服务所需的所有参数
// 这些参数通过Viper从配置文件中读取，并用于创建minio.Client实例
type Config struct {
	// Endpoint 是S3兼容对象存储服务的端点URL
	// 格式通常为: http://localhost:9000 或 https://s3.amazonaws.com
	// 默认值: "-" (表示必须显式配置)
	// 参考: https://pkg.go.dev/github.com/minio/minio-go/v7#New
	Endpoint string `mapstructure:"endpoint" default:"-" comment:"S3 compatible object storage endpoint URL (ref: https://pkg.go.dev/github.com/minio/minio-go/v7#New)"`

	// AccessKey 是S3身份验证的访问密钥
	// 用于访问S3兼容对象存储服务的身份验证凭证
	// 默认值: "-" (表示必须显式配置)
	// 参考: https://pkg.go.dev/github.com/minio/minio-go/v7/pkg/credentials#Static
	AccessKey string `mapstructure:"accessKey" default:"-" comment:"access key for S3 authentication (ref: https://pkg.go.dev/github.com/minio/minio-go/v7/pkg/credentials#Static)"`

	// SecretKey 是S3身份验证的秘密密钥
	// 用于访问S3兼容对象存储服务的身份验证凭证
	// 默认值: "-" (表示必须显式配置)
	// 参考: https://pkg.go.dev/github.com/minio/minio-go/v7/pkg/credentials#Static
	SecretKey string `mapstructure:"secretKey" default:"-" comment:"secret key for S3 authentication (ref: https://pkg.go.dev/github.com/minio/minio-go/v7/pkg/credentials#Static)"`

	// Bucket 是默认的存储桶名称
	// 用于S3操作的默认存储桶，如上传、下载、列出对象等
	// 默认值: "-" (表示必须显式配置)
	// 参考: https://pkg.go.dev/github.com/minio/minio-go/v7#Client-BucketExists
	Bucket string `mapstructure:"bucket" default:"-" comment:"default bucket name for operations (ref: https://pkg.go.dev/github.com/minio/minio-go/v7#Client-BucketExists)"`

	// Region 是S3区域标识符
	// 指定S3服务的区域，如"us-east-1"、"cn-north-1"等
	// 如果为空字符串，minio-go会自动检测区域
	// 默认值: "" (空字符串，表示自动检测)
	// 参考: https://pkg.go.dev/github.com/minio/minio-go/v7#New
	Region string `mapstructure:"region" default:"" comment:"S3 region, auto-detected if empty (ref: https://pkg.go.dev/github.com/minio/minio-go/v7#New)"`

	// UseSSL 指示是否使用HTTPS进行S3连接
	// 如果为true，使用HTTPS协议；如果为false，使用HTTP协议
	// 默认值: true (使用HTTPS)
	// 参考: https://pkg.go.dev/github.com/minio/minio-go/v7#New
	UseSSL bool `mapstructure:"useSSL" default:"true" comment:"use HTTPS for S3 connections (ref: https://pkg.go.dev/github.com/minio/minio-go/v7#New)"`
}

func (c *Config) ExtractCryptoKey(kd *key.KeyDerivation) ([]byte, error) {
	return kd.DeriveChildKey(CRYPTO_KEY_OFFSET, CRYPTO_KEY_LENGTH)
}

func DescribeConfig() config.ConfigDescriptions {
	return config.DescribeConfig[Config](PREFIX)
}

func ExtractConfig(v *viper.Viper) (*Config, error) {
	return config.ExtractConfig[Config](v, PREFIX)
}
