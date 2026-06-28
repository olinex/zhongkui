package logging

import (
	"context"

	"github.com/creasty/defaults"
	"go.uber.org/zap"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// LoggerCfg 是 zap.Config 的类型别名，用于简化导入和类型引用
// LoggerCfg is a type alias for zap.Config, simplifying imports and type references
type LoggerCfg = zap.Config

// Logger 包装 *zap.SugaredLogger，提供结构化日志记录功能。
// 通过嵌入方式继承 SugaredLogger 的全部方法，同时提供 Ctx 方法用于注入 context。
type Logger struct {
	*zap.SugaredLogger
}

// logCtx 控制 Ctx 方法的行为，默认 no-op 避免不必要的开销。
// OTEL 启用时由 EnableCtx 替换为注入 context field 的实现。
var logCtx = func(l *Logger, _ context.Context) *Logger { return l }

// EnableCtx 启用 context 注入功能。
// OTEL 集成应调用此函数，之后 Ctx(ctx) 会将 context 注入日志字段，
// otelzap bridge 自动提取用于 trace 关联，contextStrippingCore 确保不会输出到 stdout。
func EnableCtx() {
	logCtx = func(l *Logger, ctx context.Context) *Logger {
		return &Logger{l.SugaredLogger.With("context", ctx)}
	}
}

// Ctx 将 context 注入到日志记录器中。
// OTEL 未启用时为零开销 no-op；启用后将 context 作为字段注入以供 otelzap trace 关联。
func (l *Logger) Ctx(ctx context.Context) *Logger {
	return logCtx(l, ctx)
}

// NewProductionLogger 根据提供的配置创建生产环境的日志记录器
//
// 参数:
//
//	cfg - zap 日志配置，包含日志级别、输出路径、编码格式等配置项
//
// 返回值:
//
//	*Logger - 成功时返回配置好的日志记录器
//	error  - 配置无效或构建失败时返回错误信息，错误信息包含堆栈跟踪
//
// 使用示例:
//
//	cfg := &zap.Config{...}
//	logger, err := NewProductionLogger(cfg)
//	if err != nil {
//	    log.Fatal("Failed to create logger:", err)
//	}
//	defer logger.Sync()
//
// # NewProductionLogger creates a production-ready logger based on the provided configuration
//
// Parameters:
//
//	cfg - zap logger configuration containing log level, output paths, encoding format, etc.
//
// Returns:
//
//	*Logger - configured logger instance on success
//	error  - error with stack trace if configuration is invalid or build fails
//
// Example:
//
//	cfg := &zap.Config{...}
//	logger, err := NewProductionLogger(cfg)
//	if err != nil {
//	    log.Fatal("Failed to create logger:", err)
//	}
//	defer logger.Sync()
func NewProductionLogger(cfg *LoggerCfg) (*Logger, error) {
	logger, err := cfg.Build()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &Logger{logger.Sugar()}, nil
}

// NewDefaultLogger 使用默认配置创建日志记录器
//
// 该方法使用 Config 结构的默认值（通过 github.com/creasty/defaults 包设置），
// 然后转换为 zap 配置并创建生产环境的日志记录器。
//
// 默认配置包括:
//   - 日志级别: info
//   - 开发模式: false
//   - 禁用调用者信息: false
//   - 禁用堆栈跟踪: false
//   - 编码格式: json
//   - 输出路径: ["stdout"]
//   - 错误输出路径: ["stderr"]
//
// 返回值:
//
//	*Logger - 成功时返回使用默认配置的日志记录器
//	error  - 配置无效或构建失败时返回错误信息，错误信息包含堆栈跟踪
//
// 使用示例:
//
//	logger, err := NewDefaultLogger()
//	if err != nil {
//	    log.Fatal("Failed to create default logger:", err)
//	}
//	defer logger.Sync()
//
// # NewDefaultLogger creates a logger with default configuration
//
// This method uses default values for the Config structure (set via github.com/creasty/defaults package),
// then converts to zap configuration and creates a production-ready logger.
//
// Default configuration includes:
//   - Level: info
//   - Development: false
//   - DisableCaller: false
//   - DisableStacktrace: false
//   - Encoding: json
//   - OutputPaths: ["stdout"]
//   - ErrorOutputPaths: ["stderr"]
//
// Returns:
//
//	*Logger - logger instance with default configuration on success
//	error  - error with stack trace if configuration is invalid or build fails
//
// Example:
//
//	logger, err := NewDefaultLogger()
//	if err != nil {
//	    log.Fatal("Failed to create default logger:", err)
//	}
//	defer logger.Sync()
func NewDefaultLogger() (*Logger, error) {
	var cfg Config
	if err := defaults.Set(&cfg); err != nil {
		return nil, errors.WithStack(err)
	}
	return NewProductionLogger(cfg.ToZapConfig())
}
