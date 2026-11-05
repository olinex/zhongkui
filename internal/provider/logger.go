package provider

import (
	"github.com/samber/do/v2"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/config"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/otel"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

// 各日志器在依赖注入器中的命名标签。
const (
	DefaultLoggerLabel  = "logger." + base.DefaultLoggerName
	ApiLoggerLabel      = "logger." + base.ApiLoggerName
	DatabaseLoggerLabel = "logger." + base.DatabaseLoggerName
	CaptureLoggerLabel  = "logger." + base.CaptureLoggerName
	EtcdLoggerLabel     = "logger." + base.EtcdLoggerName
	GrpcLoggerLabel     = "logger." + base.GrpcLoggerName
	PipelineLoggerLabel = "logger." + base.PipelineLoggerName
)

func loggerProvider(name string) do.Provider[*logging.Logger] {
	return func(i do.Injector) (*logging.Logger, error) {
		if viper, err := InvokeViper(i); err != nil {
			return nil, errors.WithStack(err)
		} else if cfg, err := logging.ExtractConfig(viper, validator.Validate, name); err != nil {
			return nil, errors.WithStack(err)
		} else if logger, err := logging.NewProductionLogger(cfg.ToZapConfig()); err != nil {
			return nil, errors.WithStack(err)
		} else {
			logger = otel.WrapSugaredLogger(logger, name)
			return logger, nil
		}
	}
}

// NewDefaultLogger 返回构建默认日志器的 provider 工厂。
func NewDefaultLogger() do.Provider[*logging.Logger] {
	return loggerProvider(base.DefaultLoggerName)
}

// NewApiLogger 返回构建 API 日志器的 provider 工厂。
func NewApiLogger() do.Provider[*logging.Logger] {
	return loggerProvider(base.ApiLoggerName)
}

// NewDatabaseLogger 返回构建数据库日志器的 provider 工厂。
func NewDatabaseLogger() do.Provider[*logging.Logger] {
	return loggerProvider(base.DatabaseLoggerName)
}

// NewCaptureLogger 返回构建抓包日志器的 provider 工厂。
func NewCaptureLogger() do.Provider[*logging.Logger] {
	return loggerProvider(base.CaptureLoggerName)
}

// NewEtcdLogger 返回构建 etcd 日志器的 provider 工厂。
func NewEtcdLogger() do.Provider[*logging.Logger] {
	return loggerProvider(base.EtcdLoggerName)
}

// NewGrpcLogger 返回构建 gRPC 日志器的 provider 工厂。
func NewGrpcLogger() do.Provider[*logging.Logger] {
	return loggerProvider(base.GrpcLoggerName)
}

// NewPipelineLogger 返回构建流水线日志器的 provider 工厂。
func NewPipelineLogger() do.Provider[*logging.Logger] {
	return loggerProvider(base.PipelineLoggerName)
}

// InjectDefaultLogger 向依赖注入器注册默认日志器 provider。
func InjectDefaultLogger(i do.Injector) {
	do.ProvideNamed(i, DefaultLoggerLabel, NewDefaultLogger())
}

// InjectApiLogger 向依赖注入器注册 API 日志器 provider。
func InjectApiLogger(i do.Injector) {
	do.ProvideNamed(i, ApiLoggerLabel, NewApiLogger())
}

// InjectDatabaseLogger 向依赖注入器注册数据库日志器 provider。
func InjectDatabaseLogger(i do.Injector) {
	do.ProvideNamed(i, DatabaseLoggerLabel, NewDatabaseLogger())
}

// InjectCaptureLogger 向依赖注入器注册抓包日志器 provider。
func InjectCaptureLogger(i do.Injector) {
	do.ProvideNamed(i, CaptureLoggerLabel, NewCaptureLogger())
}

// InjectEtcdLogger 向依赖注入器注册 etcd 日志器 provider。
func InjectEtcdLogger(i do.Injector) {
	do.ProvideNamed(i, EtcdLoggerLabel, NewEtcdLogger())
}

// InjectGrpcLogger 向依赖注入器注册 gRPC 日志器 provider。
func InjectGrpcLogger(i do.Injector) {
	do.ProvideNamed(i, GrpcLoggerLabel, NewGrpcLogger())
}

// InjectPipelineLogger 向依赖注入器注册流水线日志器 provider。
func InjectPipelineLogger(i do.Injector) {
	do.ProvideNamed(i, PipelineLoggerLabel, NewPipelineLogger())
}

// InvokeDefaultLogger 从依赖注入器解析默认日志器，错误交由调用方处理。
func InvokeDefaultLogger(i do.Injector) (*logging.Logger, error) {
	return do.InvokeNamed[*logging.Logger](i, DefaultLoggerLabel)
}

// InvokeApiLogger 从依赖注入器解析 API 日志器，错误交由调用方处理。
func InvokeApiLogger(i do.Injector) (*logging.Logger, error) {
	return do.InvokeNamed[*logging.Logger](i, ApiLoggerLabel)
}

// InvokeDatabaseLogger 从依赖注入器解析数据库日志器，错误交由调用方处理。
func InvokeDatabaseLogger(i do.Injector) (*logging.Logger, error) {
	return do.InvokeNamed[*logging.Logger](i, DatabaseLoggerLabel)
}

// InvokeCaptureLogger 从依赖注入器解析抓包日志器，错误交由调用方处理。
func InvokeCaptureLogger(i do.Injector) (*logging.Logger, error) {
	return do.InvokeNamed[*logging.Logger](i, CaptureLoggerLabel)
}

// InvokeEtcdLogger 从依赖注入器解析 etcd 日志器，错误交由调用方处理。
func InvokeEtcdLogger(i do.Injector) (*logging.Logger, error) {
	return do.InvokeNamed[*logging.Logger](i, EtcdLoggerLabel)
}

// InvokeGrpcLogger 从依赖注入器解析 gRPC 日志器，错误交由调用方处理。
func InvokeGrpcLogger(i do.Injector) (*logging.Logger, error) {
	return do.InvokeNamed[*logging.Logger](i, GrpcLoggerLabel)
}

// InvokePipelineLogger 从依赖注入器解析流水线日志器，错误交由调用方处理。
func InvokePipelineLogger(i do.Injector) (*logging.Logger, error) {
	return do.InvokeNamed[*logging.Logger](i, PipelineLoggerLabel)
}

// MustInvokeDefaultLogger 从依赖注入器解析默认日志器，失败时 panic。
func MustInvokeDefaultLogger(i do.Injector) *logging.Logger {
	return do.MustInvokeNamed[*logging.Logger](i, DefaultLoggerLabel)
}

// MustInvokeApiLogger 从依赖注入器解析 API 日志器，失败时 panic。
func MustInvokeApiLogger(i do.Injector) *logging.Logger {
	return do.MustInvokeNamed[*logging.Logger](i, ApiLoggerLabel)
}

// MustInvokeDatabaseLogger 从依赖注入器解析数据库日志器，失败时 panic。
func MustInvokeDatabaseLogger(i do.Injector) *logging.Logger {
	return do.MustInvokeNamed[*logging.Logger](i, DatabaseLoggerLabel)
}

// MustInvokeCaptureLogger 从依赖注入器解析抓包日志器，失败时 panic。
func MustInvokeCaptureLogger(i do.Injector) *logging.Logger {
	return do.MustInvokeNamed[*logging.Logger](i, CaptureLoggerLabel)
}

// MustInvokeEtcdLogger 从依赖注入器解析 etcd 日志器，失败时 panic。
func MustInvokeEtcdLogger(i do.Injector) *logging.Logger {
	return do.MustInvokeNamed[*logging.Logger](i, EtcdLoggerLabel)
}

// MustInvokeGrpcLogger 从依赖注入器解析 gRPC 日志器，失败时 panic。
func MustInvokeGrpcLogger(i do.Injector) *logging.Logger {
	return do.MustInvokeNamed[*logging.Logger](i, GrpcLoggerLabel)
}

// MustInvokePipelineLogger 从依赖注入器解析流水线日志器，失败时 panic。
func MustInvokePipelineLogger(i do.Injector) *logging.Logger {
	return do.MustInvokeNamed[*logging.Logger](i, PipelineLoggerLabel)
}

// DescribeLoggerConfig 返回各日志器的配置项描述。
func DescribeLoggerConfig() config.ConfigDescriptions {
	return logging.DescribeConfig(
		base.ApiLoggerName,
		base.DatabaseLoggerName,
		base.EtcdLoggerName,
		base.GrpcLoggerName,
		base.PipelineLoggerName,
	)
}
