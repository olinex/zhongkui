package provider

import (
	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/otel"
)

const (
	API_LOGGER_PREFIX      = "api"
	DATABASE_LOGGER_PREFIX = "database"
	CAPTURE_LOGGER_PREFIX  = "capture"
	ETCD_LOGGER_PREFIX     = "etcd"
	GRPC_LOGGER_PREFIX     = "grpc"
	PIPELINE_LOGGER_PREFIX = "pipeline"
	API_LOGGER_LABEL       = "logger.api"
	DATABASE_LOGGER_LABEL  = "logger.database"
	CAPTURE_LOGGER_LABEL   = "logger.capture"
	ETCD_LOGGER_LABEL      = "logger.etcd"
	GRPC_LOGGER_LABEL      = "logger.grpc"
	PIPELINE_LOGGER_LABEL  = "logger.pipeline"
)

func loggerProvider(name string) do.Provider[*logging.Logger] {
	return func(i do.Injector) (*logging.Logger, error) {
		viper, err := InvokeViper(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		cfg, err := logging.ExtractConfig(viper, name)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		logger, err := logging.NewProductionLogger(cfg.ToZapConfig())
		if err != nil {
			return nil, errors.WithStack(err)
		}
		// 如果 OTEL 已启用，将 logger 通过 otelzap bridge 接入 OpenTelemetry
		logger = otel.WrapSugaredLogger(logger, name)
		return logger, nil
	}
}

func NewApiLogger() do.Provider[*logging.Logger] {
	return loggerProvider(API_LOGGER_PREFIX)
}

func NewDatabaseLogger() do.Provider[*logging.Logger] {
	return loggerProvider(DATABASE_LOGGER_PREFIX)
}

func NewCaptureLogger() do.Provider[*logging.Logger] {
	return loggerProvider(CAPTURE_LOGGER_PREFIX)
}

func NewEtcdLogger() do.Provider[*logging.Logger] {
	return loggerProvider(ETCD_LOGGER_PREFIX)
}

func NewGrpcLogger() do.Provider[*logging.Logger] {
	return loggerProvider(GRPC_LOGGER_PREFIX)
}

func NewPipelineLogger() do.Provider[*logging.Logger] {
	return loggerProvider(PIPELINE_LOGGER_PREFIX)
}

func InjectApiLogger(i do.Injector) {
	do.ProvideNamed(i, API_LOGGER_LABEL, NewApiLogger())
}

func InjectDatabaseLogger(i do.Injector) {
	do.ProvideNamed(i, DATABASE_LOGGER_LABEL, NewDatabaseLogger())
}

func InjectCaptureLogger(i do.Injector) {
	do.ProvideNamed(i, CAPTURE_LOGGER_LABEL, NewCaptureLogger())
}

func InjectEtcdLogger(i do.Injector) {
	do.ProvideNamed(i, ETCD_LOGGER_LABEL, NewEtcdLogger())
}

func InjectGrpcLogger(i do.Injector) {
	do.ProvideNamed(i, GRPC_LOGGER_LABEL, NewGrpcLogger())
}

func InjectPipelineLogger(i do.Injector) {
	do.ProvideNamed(i, PIPELINE_LOGGER_LABEL, NewPipelineLogger())
}

func InvokeApiLogger(i do.Injector) (*logging.Logger, error) {
	return do.InvokeNamed[*logging.Logger](i, API_LOGGER_LABEL)
}

func InvokeDatabaseLogger(i do.Injector) (*logging.Logger, error) {
	return do.InvokeNamed[*logging.Logger](i, DATABASE_LOGGER_LABEL)
}

func InvokeCaptureLogger(i do.Injector) (*logging.Logger, error) {
	return do.InvokeNamed[*logging.Logger](i, CAPTURE_LOGGER_LABEL)
}

func InvokeEtcdLogger(i do.Injector) (*logging.Logger, error) {
	return do.InvokeNamed[*logging.Logger](i, ETCD_LOGGER_LABEL)
}

func InvokeGrpcLogger(i do.Injector) (*logging.Logger, error) {
	return do.InvokeNamed[*logging.Logger](i, GRPC_LOGGER_LABEL)
}

func InvokePipelineLogger(i do.Injector) (*logging.Logger, error) {
	return do.InvokeNamed[*logging.Logger](i, PIPELINE_LOGGER_LABEL)
}

func MustInvokeApiLogger(i do.Injector) *logging.Logger {
	return do.MustInvokeNamed[*logging.Logger](i, API_LOGGER_LABEL)
}

func MustInvokeDatabaseLogger(i do.Injector) *logging.Logger {
	return do.MustInvokeNamed[*logging.Logger](i, DATABASE_LOGGER_LABEL)
}

func MustInvokeCaptureLogger(i do.Injector) *logging.Logger {
	return do.MustInvokeNamed[*logging.Logger](i, CAPTURE_LOGGER_LABEL)
}

func MustInvokeEtcdLogger(i do.Injector) *logging.Logger {
	return do.MustInvokeNamed[*logging.Logger](i, ETCD_LOGGER_LABEL)
}

func MustInvokeGrpcLogger(i do.Injector) *logging.Logger {
	return do.MustInvokeNamed[*logging.Logger](i, GRPC_LOGGER_LABEL)
}

func MustInvokePipelineLogger(i do.Injector) *logging.Logger {
	return do.MustInvokeNamed[*logging.Logger](i, PIPELINE_LOGGER_LABEL)
}

func DescribeLoggerConfig() config.ConfigDescriptions {
	return logging.DescribeConfig(API_LOGGER_PREFIX, DATABASE_LOGGER_PREFIX, ETCD_LOGGER_PREFIX, GRPC_LOGGER_PREFIX, PIPELINE_LOGGER_PREFIX)
}
