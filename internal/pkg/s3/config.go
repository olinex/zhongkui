package s3

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
	"zhongkui.com/zhongkui/internal/pkg/config"
)

// PREFIX 是S3配置在配置文件中的前缀
// 用于在Viper配置管理器中标识S3相关的配置项
const PREFIX = "s3"

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

// DescribeConfig 返回S3配置的描述信息
// 用于生成配置文档或验证配置结构
//
// 参数:
//   - names: 可选的配置名称列表，用于生成特定配置的描述
//     如果提供多个名称，将生成类似"s3.{name1,name2}"的配置前缀
//     如果不提供名称，将使用默认前缀"s3"
//
// 返回值:
//   - config.ConfigDescriptions: 配置描述信息，包含每个配置项的路径、类型、默认值和注释
//
// 示例:
//   // 获取所有S3配置的描述
//   descs := DescribeConfig()
//
//   // 获取特定S3配置的描述
//   descs := DescribeConfig("default", "backup")
func DescribeConfig(names ...string) config.ConfigDescriptions {
	return config.DescribeConfig[Config](fmt.Sprintf(`%s.{%s}`, PREFIX, strings.Join(names, ",")))
}

// ExtractConfig 从Viper实例中提取S3配置
// 根据配置名称从Viper配置中提取对应的S3配置实例
//
// 参数:
//   - v: Viper配置实例，包含已加载的配置数据
//   - name: 配置名称，对应配置文件中的s3配置节名称
//     例如，如果配置文件有"s3.default"节，name应为"default"
//
// 返回值:
//   - *Config: 提取的S3配置实例，包含所有配置字段的值
//   - error: 提取过程中的错误，如配置不存在或解析失败
//
// 示例:
//   // 从Viper实例中提取名为"default"的S3配置
//   cfg, err := ExtractConfig(v, "default")
//   if err != nil {
//       // 处理错误
//   }
//
//   // 使用配置创建S3客户端
//   client, err := NewS3Client(cfg)
func ExtractConfig(v *viper.Viper, name string) (*Config, error) {
	return config.ExtractConfig[Config](v, fmt.Sprintf(`%s.%s`, PREFIX, name))
}
