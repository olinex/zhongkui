package logging

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"go.uber.org/zap"
)

// 测试基于配置创建生产日志记录器。
func TestNewProductionLogger(t *testing.T) {
	convey.Convey("Test NewProductionLogger", t, func() {
		convey.Convey("should create logger with valid config", func() {
			cfg := &LoggerCfg{
				Level:            zap.NewAtomicLevelAt(zap.InfoLevel),
				Development:      false,
				Encoding:         "json",
				EncoderConfig:    zap.NewProductionEncoderConfig(),
				OutputPaths:      []string{"stdout"},
				ErrorOutputPaths: []string{"stderr"},
			}

			logger, err := NewProductionLogger(cfg)
			convey.So(err, convey.ShouldBeNil)
			convey.So(logger, convey.ShouldNotBeNil)
			logger.Info("Test message")
		})

		convey.Convey("should return error with invalid encoding", func() {
			cfg := &LoggerCfg{
				Level:            zap.NewAtomicLevelAt(zap.InfoLevel),
				Development:      false,
				Encoding:         "invalid",
				EncoderConfig:    zap.NewProductionEncoderConfig(),
				OutputPaths:      []string{"stdout"},
				ErrorOutputPaths: []string{"stderr"},
			}

			logger, err := NewProductionLogger(cfg)
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(logger, convey.ShouldBeNil)
		})

		convey.Convey("should work with console encoding", func() {
			cfg := &LoggerCfg{
				Level:            zap.NewAtomicLevelAt(zap.DebugLevel),
				Development:      true,
				Encoding:         "console",
				EncoderConfig:    zap.NewDevelopmentEncoderConfig(),
				OutputPaths:      []string{"stdout"},
				ErrorOutputPaths: []string{"stderr"},
			}

			logger, err := NewProductionLogger(cfg)
			convey.So(err, convey.ShouldBeNil)
			convey.So(logger, convey.ShouldNotBeNil)
			logger.Debug("Console debug message")
		})
	})
}

// 测试创建默认日志记录器。
func TestNewDefaultLogger(t *testing.T) {
	convey.Convey("Test NewDefaultLogger", t, func() {
		convey.Convey("should create logger with default config", func() {
			logger, err := NewDefaultLogger()
			convey.So(err, convey.ShouldBeNil)
			convey.So(logger, convey.ShouldNotBeNil)

			logger.Info("Default logger test")
			logger.Warn("Warning message")
			logger.Error("Error message")
		})

		convey.Convey("should create multiple independent loggers", func() {
			logger1, err1 := NewDefaultLogger()
			logger2, err2 := NewDefaultLogger()

			convey.So(err1, convey.ShouldBeNil)
			convey.So(err2, convey.ShouldBeNil)
			convey.So(logger1, convey.ShouldNotBeNil)
			convey.So(logger2, convey.ShouldNotBeNil)
		})
	})
}

// 测试日志类型别名与嵌入关系。
func TestLoggerAliases(t *testing.T) {
	convey.Convey("Test type aliases", t, func() {
		convey.Convey("LoggerCfg should be alias of zap.Config", func() {
			var cfg LoggerCfg
			var zapCfg zap.Config
			cfg = zapCfg
			convey.So(cfg, convey.ShouldResemble, zapCfg)
		})

		convey.Convey("Logger should embed *zap.SugaredLogger", func() {
			cfg := &LoggerCfg{
				Level:            zap.NewAtomicLevelAt(zap.InfoLevel),
				Development:      false,
				Encoding:         "json",
				EncoderConfig:    zap.NewProductionEncoderConfig(),
				OutputPaths:      []string{"stdout"},
				ErrorOutputPaths: []string{"stderr"},
			}

			logger, err := NewProductionLogger(cfg)
			convey.So(err, convey.ShouldBeNil)
			convey.So(logger.SugaredLogger, convey.ShouldNotBeNil)
		})
	})
}

// 测试日志记录器的各级别与结构化输出。
func TestLoggerFunctionality(t *testing.T) {
	convey.Convey("Test logger functionality", t, func() {
		logger, err := NewDefaultLogger()
		convey.So(err, convey.ShouldBeNil)

		convey.Convey("should support different log levels", func() {
			logger.Debug("Debug")
			logger.Info("Info")
			logger.Warn("Warn")
			logger.Error("Error")
		})

		convey.Convey("should support structured logging", func() {
			logger.Infow("Structured log",
				"id", 123,
				"name", "test",
				"enabled", true,
			)
		})

		convey.Convey("should support formatted logging", func() {
			logger.Infof("Formatted: %s %d", "test", 42)
			logger.Errorf("Error: %v", "something went wrong")
		})
	})
}
