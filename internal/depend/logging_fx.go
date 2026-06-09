package depend

import (
	"fmt"

	"github.com/spf13/viper"
	"go.uber.org/fx"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

const (
	API_LOGGER      = "api"
	DATABASE_LOGGER = "database"
	CAPTURE_LOGGER  = "capture"
	ETCD_LOGGER     = "etcd"
	GRPC_LOGGER     = "grpc"
	PIPELINE_LOGGER = "pipeline"
)

var (
	DefaultLogger *logging.Logger
)

type LoggerFxIn struct {
	fx.In
	V *viper.Viper
}

type CaptureLoggerFxIn struct {
	fx.In
	CaptureLogger *logging.Logger `name:"capture"`
}

type APILoggerFxIn struct {
	fx.In
	APILogger *logging.Logger `name:"api"`
}

type DatabaseLoggerFxIn struct {
	fx.In
	DatabaseLogger *logging.Logger `name:"database"`
}

type EtcdLoggerFxIn struct {
	fx.In
	ETCDLogger *logging.Logger `name:"etcd"`
}

type GRPCLoggerFxIn struct {
	fx.In
	GRPCLogger *logging.Logger `name:"grpc"`
}

type PipelineLoggerFxIn struct {
	fx.In
	PipelineLogger *logging.Logger `name:"pipeline"`
}

func newLogger(name string) func(in LoggerFxIn) *logging.Logger {
	return func(in LoggerFxIn) *logging.Logger {
		if cfg, err := logging.ExtractConfig(in.V, name); err != nil {
			panic(err)
		} else if logger, err := logging.NewProductionLogger(cfg.ToZapConfig()); err != nil {
			panic(err)
		} else {
			return logger
		}
	}
}

func loggerAnnotate(name string) any {
	return fx.Annotate(newLogger(name), fx.ResultTags(fmt.Sprintf(`name:"%s"`, name)))
}

func CaptureLoggerAnnotate() any {
	return loggerAnnotate(CAPTURE_LOGGER)
}

func ApiLoggerAnnotate() any {
	return loggerAnnotate(API_LOGGER)
}

func DatabaseLoggerAnnotate() any {
	return loggerAnnotate(DATABASE_LOGGER)
}

func ETCDLoggerAnnotate() any {
	return loggerAnnotate(ETCD_LOGGER)
}

func GRPCLoggerAnnotate() any {
	return loggerAnnotate(GRPC_LOGGER)
}

func PipelineLoggerAnnotate() any {
	return loggerAnnotate(PIPELINE_LOGGER)
}

func DescribeLoggerConfig() config.ConfigDescriptions {
	return logging.DescribeConfig(API_LOGGER, DATABASE_LOGGER, ETCD_LOGGER, GRPC_LOGGER, PIPELINE_LOGGER)
}

func init() {
	if tmp, err := logging.NewDefaultLogger(); err != nil {
		panic(err)
	} else {
		DefaultLogger = tmp
	}
}
