package capture

import (
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/klauspost/compress/zstd"
	"github.com/spf13/viper"
	"zhongkui.com/zhongkui/internal/pkg/config"
	reporter_v1 "zhongkui.com/zhongkui/internal/pkg/protobuf/reporter/v1"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

const (
	// PREFIX 配置前缀，用于在配置文件中标识抓包相关的配置项
	// 例如：capture.watch.intervalTimeout
	Prefix = "capture"
)

// WatchConfig 监控配置，用于配置与远程服务器的监控连接参数
// 该配置定义了心跳检测、重试机制和容差计算等监控相关参数
type WatchConfig struct {
	// IntervalTimeout 心跳间隔超时时间，定义了向服务器发送心跳的频率
	// 默认值：60s，建议根据网络状况和服务器负载进行调整
	IntervalTimeout time.Duration `mapstructure:"intervalTimeout" default:"60s"`

	// TolerateRatio 容差比率，用于计算允许的超时范围
	// 公式：允许超时 = IntervalTimeout * (1 + TolerateRatio)
	// 默认值：3.0，表示允许3倍的超时时间
	TolerateRatio float32 `mapstructure:"tolerateRatio" default:"3"`

	// MaxRetries 最大重试次数，当心跳失败时的重试次数
	// 达到最大重试次数后，监控连接将被标记为失败
	MaxRetries uint `mapstructure:"maxRetries" default:"5"`
}

// TolerateIntervalTimeout 计算允许的超时间隔，基于基础间隔和容差比率
// 返回值：允许的最大超时时间，计算公式：IntervalTimeout * (1 + TolerateRatio)
// 示例：如果IntervalTimeout=60s，TolerateRatio=3，则返回240s
func (c *WatchConfig) TolerateIntervalTimeout() time.Duration {
	return time.Duration(float64(c.IntervalTimeout.Nanoseconds()) * float64(1+c.TolerateRatio))
}

// FeedbackConfig 反馈配置，用于配置向服务器发送状态反馈的参数
// 该配置定义了状态反馈的发送频率、容差和重试机制
type FeedbackConfig struct {
	// IntervalTimeout 反馈发送间隔时间，定义了向服务器发送状态反馈的频率
	// 默认值：5s，建议根据服务器处理能力和网络状况进行调整
	IntervalTimeout time.Duration `mapstructure:"intervalTimeout" default:"5s"`

	// TolerateRatio 容差比率，用于计算允许的超时范围
	// 公式：允许超时 = IntervalTimeout * (1 + TolerateRatio)
	// 默认值：3.0，表示允许3倍的超时时间
	TolerateRatio float32 `mapstructure:"tolerateRatio" default:"3"`

	// MaxRetries 最大重试次数，当反馈发送失败时的重试次数
	// 达到最大重试次数后，反馈将被丢弃或延迟处理
	MaxRetries uint `mapstructure:"maxRetries" default:"5"`
}

// TolerateIntervalTimeout 计算允许的超时间隔，基于基础间隔和容差比率
// 返回值：允许的最大超时时间，计算公式：IntervalTimeout * (1 + TolerateRatio)
// 示例：如果IntervalTimeout=5s，TolerateRatio=3，则返回20s
func (c *FeedbackConfig) TolerateIntervalTimeout() time.Duration {
	return time.Duration(float64(c.IntervalTimeout.Nanoseconds()) * float64(1+c.TolerateRatio))
}

// UploadConfig 上传配置，用于配置数据包捕获和上传的相关参数
// 该配置定义了数据包捕获、压缩、存储和上传的完整流程参数
type UploadConfig struct {
	// DumpDir 临时文件目录，用于存储pcap原始文件和压缩后的文件
	// 默认值："./tmp"，建议使用绝对路径以确保权限正确
	DumpDir string `mapstructure:"dumpDir" default:"./tmp"`

	// EncoderLevel 压缩级别，使用zstd库的EncoderLevel类型
	// 有效值范围：1-4，对应不同的压缩策略
	// 1: SpeedFastest (最快压缩)
	// 2: SpeedDefault (默认压缩)
	// 3: SpeedBetterCompression (更好压缩)
	// 4: SpeedBestCompression (最佳压缩)
	// 注意：EncoderLevelFromZstd函数可以将0-22的整数映射到上述级别
	// 默认值：2，对应SpeedDefault级别
	EncoderLevel zstd.EncoderLevel `mapstructure:"encoderLevel" binding:"gte=1,lte=4" default:"2"`

	// IntervalTimeout 上传间隔超时时间，定义了数据包上传的频率
	// 默认值：1m，建议根据数据量和网络带宽进行调整
	IntervalTimeout time.Duration `mapstructure:"intervalTimeout" default:"1m"`

	// PacketBufferSize 数据包缓冲区大小，定义了内存中缓存的数据包大小
	// 默认值：100MB，根据可用内存和网络流量进行调整
	PacketBufferSize serializer.BibyteSize `mapstructure:"packetBufferSize" default:"100MB"`

	// PacketCount 数据包缓冲区大小，定义了内存中缓存的数据包数量
	// 默认值：1000000，根据可用内存和网络流量进行调整
	PacketCount uint `mapstructure:"packetCount" default:"1000000"`

	// ExpireAge S3对象过期时间，定义了上传到S3的文件在多长时间后自动删除
	// 默认值：1h，根据数据保留策略进行调整
	ExpireAge time.Duration `mapstructure:"expireAge" default:"1h"`

	// MaxRetries 最大重试次数，当上传失败时的重试次数
	// 达到最大重试次数后，数据包可能被丢弃或延迟上传
	MaxRetries uint `mapstructure:"maxRetries" default:"5"`
}

// Config 抓包功能的完整配置，包含资源定义和各个模块的配置
// 该结构体是整个抓包功能的配置入口，通过Viper配置框架进行解析和加载
type Config struct {
	// Resource 资源标识，包含namespace、name、version等信息
	// 用于标识当前抓包实例的归属和版本信息
	Resource reporter_v1.ResourceRaw `mapstructure:"resource"`

	// Watch 监控配置，定义了与远程服务器的监控连接参数
	Watch WatchConfig `mapstructure:"watch"`

	// Feedback 反馈配置，定义了向服务器发送状态反馈的参数
	Feedback FeedbackConfig `mapstructure:"feedback"`

	// Upload 上传配置，定义了数据包捕获和上传的相关参数
	Upload UploadConfig `mapstructure:"upload"`
}

// DescribeConfig 返回配置描述信息，用于配置文档生成
// 该方法生成配置的结构化描述，包括字段名、类型、默认值和说明
// 返回值：config.ConfigDescriptions 配置描述集合
func DescribeConfig() config.ConfigDescriptions {
	return config.DescribeConfig[Config](Prefix)
}

// ExtractConfig 从Viper实例中提取抓包配置
// 参数：
//
//	v: *viper.Viper - Viper配置实例，包含已加载的配置数据
//
// 返回值：
//
//	*Config: 解析后的配置对象
//	error: 解析过程中出现的错误，如果解析成功则为nil
//
// 使用示例：
//
//	v := viper.New()
//	v.SetConfigFile("config.yaml")
//	v.ReadInConfig()
//	cfg, err := ExtractConfig(v)
func ExtractConfig(v *viper.Viper, validate *validator.Validate) (*Config, error) {
	return config.ExtractConfig[Config](v, validate, Prefix)
}
