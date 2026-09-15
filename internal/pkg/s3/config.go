package s3

import (
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
)

// PREFIX 是S3配置在配置文件中的前缀
// 用于在Viper配置管理器中标识S3相关的配置项
const (
	Prefix          = "s3"
	CryptoKeyOffset = 43036
	CryptoKeyLength = 32
)

// Config 是S3兼容对象存储的配置结构体
// 包含连接到S3兼容服务所需的所有参数
// 这些参数通过Viper从配置文件中读取，并用于创建minio.Client实例
type Config struct {
	// Endpoint 是S3兼容对象存储服务的端点URL
	// 格式通常为: http://localhost:9000 或 https://s3.amazonaws.com
	// 默认值: "-" (表示必须显式配置)
	// 参考: https://pkg.go.dev/github.com/minio/minio-go/v7#New
	Endpoint string `mapstructure:"endpoint" default:"-" comment:"S3 兼容对象存储服务端点 URL (ref: https://pkg.go.dev/github.com/minio/minio-go/v7#New)"`

	// AccessKey 是S3身份验证的访问密钥
	// 用于访问S3兼容对象存储服务的身份验证凭证
	// 默认值: "-" (表示必须显式配置)
	// 参考: https://pkg.go.dev/github.com/minio/minio-go/v7/pkg/credentials#Static
	AccessKey string `mapstructure:"accessKey" default:"-" comment:"S3 认证访问密钥 (ref: https://pkg.go.dev/github.com/minio/minio-go/v7/pkg/credentials#Static)"`

	// SecretKey 是S3身份验证的秘密密钥
	// 用于访问S3兼容对象存储服务的身份验证凭证
	// 默认值: "-" (表示必须显式配置)
	// 参考: https://pkg.go.dev/github.com/minio/minio-go/v7/pkg/credentials#Static
	SecretKey string `mapstructure:"secretKey" default:"-" comment:"S3 认证秘密密钥 (ref: https://pkg.go.dev/github.com/minio/minio-go/v7/pkg/credentials#Static)"`

	// Bucket 是默认的存储桶名称
	// 用于S3操作的默认存储桶，如上传、下载、列出对象等
	// 默认值: "-" (表示必须显式配置)
	// 参考: https://pkg.go.dev/github.com/minio/minio-go/v7#Client-BucketExists
	Bucket string `mapstructure:"bucket" default:"-" comment:"默认存储桶名称，用于上传、下载、列出对象等操作 (ref: https://pkg.go.dev/github.com/minio/minio-go/v7#Client-BucketExists)"`

	// Region 是S3区域标识符
	// 指定S3服务的区域，如"us-east-1"、"cn-north-1"等
	// 如果为空字符串，minio-go会自动检测区域
	// 默认值: "" (空字符串，表示自动检测)
	// 参考: https://pkg.go.dev/github.com/minio/minio-go/v7#New
	Region string `mapstructure:"region" default:"" comment:"S3 区域标识符，为空时自动检测 (ref: https://pkg.go.dev/github.com/minio/minio-go/v7#New)"`

	// UseSSL 指示是否使用HTTPS进行S3连接
	// 如果为true，使用HTTPS协议；如果为false，使用HTTP协议
	// 默认值: true (使用HTTPS)
	// 参考: https://pkg.go.dev/github.com/minio/minio-go/v7#New
	UseSSL bool `mapstructure:"useSSL" default:"true" comment:"是否使用 HTTPS 进行 S3 连接 (ref: https://pkg.go.dev/github.com/minio/minio-go/v7#New)"`

	// StreamingUpload 控制加密上传策略。
	//   - true（默认）: 使用 io.Pipe 流式管道 + multipart/chunked encoding 直传，
	//     适用于成熟的 S3 实现（AWS S3、MinIO 等）
	//   - false: 加密到临时文件后以已知大小简单 PUT 上传，
	//     适用于 multipart upload 支持不完整的 S3 实现（如 RustFS Alpha）
	StreamingUpload bool `mapstructure:"streamingUpload" default:"false" comment:"加密上传是否使用流式管道 + multipart；对 multipart 支持不完整的 S3 服务端（如 RustFS alpha）应设置为 false"`

	// ObjectCacheDir 是数据源上传文件在读取时的内存缓存目录（ObjectCache 通用机制）。
	// Linux 默认 /dev/shm/zhongkui-objcache（tmpfs 即内存，不占本地磁盘），其余平台回退
	// 系统临时目录；为空时按平台默认。不可写时自动回退磁盘缓存目录。
	ObjectCacheDir string `mapstructure:"objectCacheDir" default:"" comment:"数据源上传文件内存缓存目录（Linux 默认 /dev/shm/zhongkui-objcache，tmpfs 即内存；为空按平台默认，不可写时回退磁盘缓存目录）"`

	// ObjectCacheMemoryThresholdMB 是数据源上传文件「内存优先」阈值：文件解密后小于等于
	// 该大小落入内存缓存目录，否则临时落盘（TTL 回收）。
	ObjectCacheMemoryThresholdMB int `mapstructure:"objectCacheMemoryThresholdMB" default:"8" comment:"数据源上传文件内存优先阈值（MB）：小于等于该值落入内存缓存目录，否则临时落盘"`

	// ObjectCacheTTL 是数据源上传文件本地缓存的存活时间，到期后失效重拉 S3。
	ObjectCacheTTL time.Duration `mapstructure:"objectCacheTTL" default:"1h" comment:"数据源上传文件本地缓存存活时间，到期后失效并重新拉取 S3"`
}

// ExtractCryptoKey 使用密钥派生函数从配置中提取加密密钥。
//
// 通过 KeyDerivation 以固定偏移量（CryptoKeyOffset = 43036）和固定长度（CryptoKeyLength = 32）
// 从密码本中派生一个 AES-256 子密钥。该密钥用于构造 ZstdAESGCMPipeline 进行对象的加密解密。
//
// 参数：
//   - kd: 密钥派生器实例，包含主密钥和密码本
//
// 返回值：
//   - []byte: 派生的 32 字节 AES-256 密钥
//   - error: 派生失败时返回的错误，包括主密钥获取失败或 HKDF 派生错误
//
// 参考：internal/pkg/crypto/key/keyderivation.go
func (c *Config) ExtractCryptoKey(kd *key.KeyDerivation) ([]byte, error) {
	return kd.DeriveChildKey(CryptoKeyOffset, CryptoKeyLength)
}

// DescribeConfig 返回 S3 配置的结构化描述信息。
//
// 使用泛型函数 config.DescribeConfig[Config] 自动提取 Config 结构体的字段描述，
// 包括字段名、类型、默认值、注释等信息。这些描述通常用于生成配置文档或 CLI 帮助文本。
//
// 返回值：
//   - config.ConfigDescriptions: 配置字段的描述列表，用于文档生成和用户提示
func DescribeConfig() config.ConfigDescriptions {
	return config.DescribeConfig[Config](Prefix)
}

// ExtractConfig 从 Viper 配置管理器中提取 S3 配置。
//
// 使用泛型函数 config.ExtractConfig[Config] 从 Viper 中自动反序列化以 PREFIX（"s3"）为前缀的配置项。
// 内部使用 mapstructure 标签进行字段映射，并使用 defaults 标签填充默认值。
//
// 参数：
//   - v: Viper 配置管理器实例，需包含以 "s3" 为前缀的配置项
//
// 返回值：
//   - *Config: 填充了配置值的 S3 配置结构体指针
//   - error: 提取或反序列化失败时返回的错误
//
// 示例配置（YAML）：
//
//	s3:
//	  endpoint: "http://localhost:9000"
//	  accessKey: "minioadmin"
//	  secretKey: "minioadmin"
//	  bucket: "my-bucket"
//	  region: ""
//	  useSSL: false
func ExtractConfig(viper *viper.Viper, validate *validator.Validate) (*Config, error) {
	return config.ExtractConfig[Config](viper, validate, Prefix)
}
