package provider

import (
	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/otel"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

const (
	DefaultLoggerPrefix  = "default"
	ApiLoggerPrefix      = "api"
	DatabaseLoggerPrefix = "database"
	CaptureLoggerPrefix  = "capture"
	EtcdLoggerPrefix     = "etcd"
	GrpcLoggerPrefix     = "grpc"
	PipelineLoggerPrefix = "pipeline"
	DefaultLoggerLabel   = "logger.default"
	ApiLoggerLabel       = "logger.api"
	DatabaseLoggerLabel  = "logger.database"
	CaptureLoggerLabel   = "logger.capture"
	EtcdLoggerLabel      = "logger.etcd"
	GrpcLoggerLabel      = "logger.grpc"
	PipelineLoggerLabel  = "logger.pipeline"
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

func NewDefaultLogger() do.Provider[*logging.Logger] {
	return loggerProvider(DefaultLoggerPrefix)
}

func NewApiLogger() do.Provider[*logging.Logger] {
	return loggerProvider(ApiLoggerPrefix)
}

func NewDatabaseLogger() do.Provider[*logging.Logger] {
	return loggerProvider(DatabaseLoggerPrefix)
}

func NewCaptureLogger() do.Provider[*logging.Logger] {
	return loggerProvider(CaptureLoggerPrefix)
}

func NewEtcdLogger() do.Provider[*logging.Logger] {
	return loggerProvider(EtcdLoggerPrefix)
}

func NewGrpcLogger() do.Provider[*logging.Logger] {
	return loggerProvider(GrpcLoggerPrefix)
}

func NewPipelineLogger() do.Provider[*logging.Logger] {
	return loggerProvider(PipelineLoggerPrefix)
}

func InjectDefaultLogger(i do.Injector) {
	do.ProvideNamed(i, DefaultLoggerLabel, NewDefaultLogger())
}

func InjectApiLogger(i do.Injector) {
	do.ProvideNamed(i, ApiLoggerLabel, NewApiLogger())
}

func InjectDatabaseLogger(i do.Injector) {
	do.ProvideNamed(i, DatabaseLoggerLabel, NewDatabaseLogger())
}

func InjectCaptureLogger(i do.Injector) {
	do.ProvideNamed(i, CaptureLoggerLabel, NewCaptureLogger())
}

func InjectEtcdLogger(i do.Injector) {
	do.ProvideNamed(i, EtcdLoggerLabel, NewEtcdLogger())
}

func InjectGrpcLogger(i do.Injector) {
	do.ProvideNamed(i, GrpcLoggerLabel, NewGrpcLogger())
}

func InjectPipelineLogger(i do.Injector) {
	do.ProvideNamed(i, PipelineLoggerLabel, NewPipelineLogger())
}

func InvokeDefaultLogger(i do.Injector) (*logging.Logger, error) {
	return do.InvokeNamed[*logging.Logger](i, DefaultLoggerLabel)
}

func InvokeApiLogger(i do.Injector) (*logging.Logger, error) {
	return do.InvokeNamed[*logging.Logger](i, ApiLoggerLabel)
}

func InvokeDatabaseLogger(i do.Injector) (*logging.Logger, error) {
	return do.InvokeNamed[*logging.Logger](i, DatabaseLoggerLabel)
}

func InvokeCaptureLogger(i do.Injector) (*logging.Logger, error) {
	return do.InvokeNamed[*logging.Logger](i, CaptureLoggerLabel)
}

func InvokeEtcdLogger(i do.Injector) (*logging.Logger, error) {
	return do.InvokeNamed[*logging.Logger](i, EtcdLoggerLabel)
}

func InvokeGrpcLogger(i do.Injector) (*logging.Logger, error) {
	return do.InvokeNamed[*logging.Logger](i, GrpcLoggerLabel)
}

func InvokePipelineLogger(i do.Injector) (*logging.Logger, error) {
	return do.InvokeNamed[*logging.Logger](i, PipelineLoggerLabel)
}

func MustInvokeDefaultLogger(i do.Injector) *logging.Logger {
	return do.MustInvokeNamed[*logging.Logger](i, DefaultLoggerLabel)
}

func MustInvokeApiLogger(i do.Injector) *logging.Logger {
	return do.MustInvokeNamed[*logging.Logger](i, ApiLoggerLabel)
}

func MustInvokeDatabaseLogger(i do.Injector) *logging.Logger {
	return do.MustInvokeNamed[*logging.Logger](i, DatabaseLoggerLabel)
}

func MustInvokeCaptureLogger(i do.Injector) *logging.Logger {
	return do.MustInvokeNamed[*logging.Logger](i, CaptureLoggerLabel)
}

func MustInvokeEtcdLogger(i do.Injector) *logging.Logger {
	return do.MustInvokeNamed[*logging.Logger](i, EtcdLoggerLabel)
}

func MustInvokeGrpcLogger(i do.Injector) *logging.Logger {
	return do.MustInvokeNamed[*logging.Logger](i, GrpcLoggerLabel)
}

func MustInvokePipelineLogger(i do.Injector) *logging.Logger {
	return do.MustInvokeNamed[*logging.Logger](i, PipelineLoggerLabel)
}

func DescribeLoggerConfig() config.ConfigDescriptions {
	return logging.DescribeConfig(ApiLoggerPrefix, DatabaseLoggerPrefix, EtcdLoggerPrefix, GrpcLoggerPrefix, PipelineLoggerPrefix)
}
