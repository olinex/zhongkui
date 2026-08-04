package logging

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"zhongkui.com/zhongkui/internal/pkg/config"
)

// PREFIX 是日志配置在配置文件中的前缀
const PREFIX = "zap"

// Config 定义了日志系统的配置结构
// 该结构体通过 mapstructure 标签支持从配置文件自动解析
// 通过 default 标签设置默认值，comment 标签提供配置说明
type Config struct {
	// Level 指定日志级别，控制哪些级别的日志会被记录
	// 有效值: debug, info, warn, error, dpanic, panic, fatal
	// 默认值: info
	// 参考: https://github.com/uber-go/zap/blob/master/level.go
	Level string `mapstructure:"level" default:"info" comment:"日志级别：debug, info, warn, error, dpanic, panic, fatal (ref: https://github.com/uber-go/zap/blob/master/level.go)"`

	// Development 启用开发模式，使用 NewDevelopmentEncoderConfig
	// 在开发模式下，使用控制台编码器并为警告和错误日志添加堆栈跟踪
	// 默认值: false
	// 参考: https://pkg.go.dev/go.uber.org/zap#NewDevelopmentEncoderConfig
	Development bool `mapstructure:"development" default:"false" comment:"启用开发模式，使用控制台编码器并为警告和错误日志添加堆栈跟踪 (ref: https://pkg.go.dev/go.uber.org/zap#NewDevelopmentEncoderConfig)"`

	// DisableCaller 禁用日志输出中的调用者信息
	// 当设置为 false 时，会在日志输出中添加文件名和行号
	// 默认值: false
	// 参考: https://pkg.go.dev/go.uber.org/zap#Config-DisableCaller
	DisableCaller bool `mapstructure:"disableCaller" default:"false" comment:"禁用日志输出中的调用者信息，为 false 时输出文件名和行号 (ref: https://pkg.go.dev/go.uber.org/zap#Config-DisableCaller)"`

	// DisableStacktrace 禁用 ErrorLevel 和 DPanicLevel 日志的堆栈跟踪
	// 当设置为 false 时，会为错误日志添加堆栈跟踪
	// 默认值: false
	// 参考: https://pkg.go.dev/go.uber.org/zap#Config-DisableStacktrace
	DisableStacktrace bool `mapstructure:"disableStacktrace" default:"false" comment:"禁用 ErrorLevel 和 DPanicLevel 日志的堆栈跟踪，为 false 时为错误日志添加堆栈 (ref: https://pkg.go.dev/go.uber.org/zap#Config-DisableStacktrace)"`

	// Encoding 指定日志编码格式
	// 有效值: json 或 console
	// json 格式适合生产环境，console 格式适合开发环境
	// 默认值: json
	// 参考: https://pkg.go.dev/go.uber.org/zap#Config-Encoding
	Encoding string `mapstructure:"encoding" default:"json" comment:"日志编码格式：json 或 console (ref: https://pkg.go.dev/go.uber.org/zap#Config-Encoding)"`

	// OutputPaths 指定日志输出的目标路径列表
	// 支持 stdout, stderr 和文件路径
	// 可以同时指定多个输出路径，例如 ["stdout", "/var/log/app.log"]
	// 默认值: ["stdout"]
	// 参考: https://pkg.go.dev/go.uber.org/zap#Config-OutputPaths
	OutputPaths []string `mapstructure:"outputPaths" default:"[\"stdout\"]" comment:"日志输出目标列表，支持 stdout、stderr 和文件路径 (ref: https://pkg.go.dev/go.uber.org/zap#Config-OutputPaths)"`

	// ErrorOutputPaths 指定内部日志错误输出的目标路径列表
	// 用于记录日志系统自身的错误，如配置错误、写入失败等
	// 默认值: ["stderr"]
	// 参考: https://pkg.go.dev/go.uber.org/zap#Config-ErrorOutputPaths
	ErrorOutputPaths []string `mapstructure:"errorOutputPaths" default:"[\"stderr\"]" comment:"内部日志错误输出目标列表 (ref: https://pkg.go.dev/go.uber.org/zap#Config-ErrorOutputPaths)"`
}

// ToZapConfig 将 Config 结构体转换为 zap 配置
// 该方法根据配置生成对应的 zap.Config，用于创建实际的日志记录器
// 根据 Development 字段决定使用开发环境还是生产环境的编码器配置
//
// 返回值:
//
//	*LoggerCfg - 转换后的 zap 配置，可以直接用于构建日志记录器
//
// 转换规则:
//  1. 如果 Development 为 true，使用 zap.NewDevelopmentEncoderConfig()
//  2. 如果 Development 为 false，使用 zap.NewProductionEncoderConfig()
//  3. 使用 ParseLevel 将字符串日志级别转换为 zapcore.Level
//  4. 将所有配置字段映射到对应的 zap.Config 字段
//
// 使用示例:
//
//	cfg := &Config{Level: "info", Development: false}
//	zapCfg := cfg.ToZapConfig()
//	logger, err := zapCfg.Build()
func (cfg *Config) ToZapConfig() *LoggerCfg {
	var encoderConfig zapcore.EncoderConfig
	if cfg.Development {
		encoderConfig = zap.NewDevelopmentEncoderConfig()
	} else {
		encoderConfig = zap.NewProductionEncoderConfig()
	}
	zcfg := zap.Config{
		Level:             zap.NewAtomicLevelAt(ParseLevel(cfg.Level)),
		Development:       cfg.Development,
		DisableCaller:     cfg.DisableCaller,
		DisableStacktrace: cfg.DisableStacktrace,
		Encoding:          cfg.Encoding,
		EncoderConfig:     encoderConfig,
		OutputPaths:       cfg.OutputPaths,
		ErrorOutputPaths:  cfg.ErrorOutputPaths,
	}
	return &zcfg
}

// ParseLevel 将字符串形式的日志级别转换为 zapcore.Level
// 该函数用于将配置文件中的字符串级别转换为 zap 库能识别的枚举值
// 如果传入的级别字符串无效，函数会 panic
//
// 参数:
//
//	level - 字符串形式的日志级别，不区分大小写（但建议使用小写）
//
// 返回值:
//
//	zapcore.Level - 对应的 zap 日志级别枚举值
//
// 支持的级别:
//   - "debug": 调试级别，最详细的日志信息
//   - "info": 信息级别，常规信息
//   - "warn": 警告级别，需要注意但不影响系统运行的情况
//   - "error": 错误级别，系统错误但可以恢复
//   - "dpanic": 开发环境 panic 级别，生产环境记录错误
//   - "panic": panic 级别，记录日志后调用 panic
//   - "fatal": 致命级别，记录日志后调用 os.Exit(1)
//
// 使用示例:
//
//	level := ParseLevel("info") // 返回 zapcore.InfoLevel
func ParseLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	case "dpanic":
		return zapcore.DPanicLevel
	case "panic":
		return zapcore.PanicLevel
	case "fatal":
		return zapcore.FatalLevel
	default:
		panic(fmt.Sprintf("unknown log level: %s", level))
	}
}

// DescribeConfig 生成日志配置的描述信息
// 该函数用于生成配置文件的文档描述，帮助用户理解配置项的含义和用法
// 返回的配置描述可以用于生成配置文件模板或文档
//
// 参数:
//
//	names - 可变参数，指定要描述的配置名称
//	        如果不提供参数，则生成所有配置的描述
//	        如果提供参数，则只生成指定配置的描述
//
// 返回值:
//
//	config.ConfigDescriptions - 配置描述信息，包含字段名、类型、默认值和说明
//
// 使用示例:
//
//	// 生成所有日志配置的描述
//	descriptions := DescribeConfig()
//
//	// 只生成特定配置的描述
//	descriptions := DescribeConfig("level", "encoding")
func DescribeConfig(names ...string) config.ConfigDescriptions {
	return config.DescribeConfig[Config](fmt.Sprintf(`%s.{%s}`, PREFIX, strings.Join(names, ",")))
}

// ExtractConfig 从 Viper 配置中提取日志配置
// 该函数从 Viper 配置实例中提取指定名称的日志配置，并转换为 Config 结构体
// 使用 mapstructure 标签自动映射配置字段，支持默认值和类型转换
//
// 参数:
//
//	v    - *viper.Viper 实例，包含已加载的配置数据
//	name - 配置名称，对应配置文件中 zap 前缀下的子配置名
//
// 返回值:
//
//	*Config - 成功时返回提取的配置结构体指针
//	error   - 提取失败时返回错误信息
//
// 配置路径格式:
//
//	配置文件中的日志配置应位于 "zap.{name}" 路径下
//	例如，名为 "default" 的配置对应配置文件中的 "zap.default" 部分
//
// 使用示例:
//
//	v := viper.New()
//	v.SetConfigFile("config.yaml")
//	v.ReadInConfig()
//
//	cfg, err := ExtractConfig(v, "default")
//	if err != nil {
//	    log.Fatal("Failed to extract config:", err)
//	}
func ExtractConfig(v *viper.Viper, name string) (*Config, error) {
	return config.ExtractConfig[Config](v, fmt.Sprintf(`%s.%s`, PREFIX, name))
}
