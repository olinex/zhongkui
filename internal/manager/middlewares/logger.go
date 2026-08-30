package middlewares

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// LoggerCtxName 是 ApiLogger 在 gin.Context 中存储的键名
// 通过此键名从请求上下文中获取 ApiLogger 实例
const LoggerCtxName = "logger"

// ApiLogger 是对 logging.Logger（zap.SugaredLogger）的封装
// 在记录日志时自动附加 HTTP 请求上下文信息（请求方法、路径、客户端 IP），
// 专为 Gin 框架的 HTTP 请求处理场景设计，使业务代码无需手动传递这些通用字段
type ApiLogger struct {
	logger *logging.Logger
}

// logf 是核心日志记录方法，自动将 HTTP 请求上下文信息附加到日志字段中
//
// 该方法在用户传入的自定义字段基础上，追加以下结构化字段：
//   - method: HTTP 请求方法（GET、POST 等）
//   - path:   HTTP 请求路径
//   - ip:     客户端 IP 地址
//
// 同时设置 zap.AddCallerSkip(2)，使得日志中记录的调用者位置
// 指向实际调用 Debugf/Infof/Warnf/Errorf 的业务代码行，而非这些方法本身
//
// 参数:
//   - lvl:     日志级别（Debug/Info/Warn/Error）
//   - c:       gin 上下文，用于提取 HTTP 请求信息
//   - fields:  用户自定义的结构化日志字段（zap.Field 或任意类型）
//   - template: 日志消息模板，支持 fmt.Sprintf 风格的格式化占位符
//   - args:    模板格式化参数
func (a *ApiLogger) logf(lvl zapcore.Level, c *gin.Context, fields []any, template string, args ...any) {
	fields = append(
		fields,
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path),
		zap.String("ip", c.ClientIP()),
	)
	// Ctx 仅在 OTEL 启用时注入 context 字段供 otelzap trace 关联，
	// 且会被 contextStrippingCore 从输出中剥离；未启用时为零开销 no-op
	a.logger.Ctx(c.Request.Context()).With(fields...).WithOptions(
		zap.AddCallerSkip(2),
	).Logf(
		lvl,
		template,
		args...,
	)
}

// Debugf 记录调试级别的日志
// 适用于开发调试阶段输出详细的程序运行信息
//
// 参数:
//   - c:       gin 上下文，用于提取 HTTP 请求信息
//   - fields:  自定义结构化日志字段
//   - template: 日志消息模板
//   - args:    模板格式化参数
func (a *ApiLogger) Debugf(c *gin.Context, fields []any, template string, args ...any) {
	a.logf(zapcore.DebugLevel, c, fields, template, args...)
}

// Infof 记录信息级别的日志
// 适用于记录程序正常运行的关键流程信息
//
// 参数:
//   - c:       gin 上下文，用于提取 HTTP 请求信息
//   - fields:  自定义结构化日志字段
//   - template: 日志消息模板
//   - args:    模板格式化参数
func (a *ApiLogger) Infof(c *gin.Context, fields []any, template string, args ...any) {
	a.logf(zapcore.InfoLevel, c, fields, template, args...)
}

// Warnf 记录警告级别的日志
// 适用于记录不影响系统运行但需要关注的情况
//
// 参数:
//   - c:       gin 上下文，用于提取 HTTP 请求信息
//   - fields:  自定义结构化日志字段
//   - template: 日志消息模板
//   - args:    模板格式化参数
func (a *ApiLogger) Warnf(c *gin.Context, fields []any, template string, args ...any) {
	a.logf(zapcore.WarnLevel, c, fields, template, args...)
}

// Errorf 记录错误级别的日志
// 适用于记录系统错误，表示操作失败但系统仍可继续运行
//
// 参数:
//   - c:       gin 上下文，用于提取 HTTP 请求信息
//   - fields:  自定义结构化日志字段
//   - template: 日志消息模板
//   - args:    模板格式化参数
func (a *ApiLogger) Errorf(c *gin.Context, fields []any, template string, args ...any) {
	a.logf(zapcore.ErrorLevel, c, fields, template, args...)
}

// ZapLoggerMiddleware 创建一个 Gin 中间件，为每个 HTTP 请求注入 ApiLogger 实例
//
// 该中间件执行以下操作：
//  1. 将 ApiLogger 实例存入 gin.Context，键名为 LoggerCtxName
//     业务处理函数可通过 GetLogger(c) 获取并使用
//  2. 记录请求开始时间
//  3. 调用 c.Next() 继续执行后续处理链
//  4. 请求处理完成后，自动记录一条信息级别日志，包含：
//     - SS (Status): HTTP 响应状态码
//     - LY (Latency): 请求处理耗时
//     - 以及 logf 自动附加的 method、path、ip 字段
//
// 参数:
//   - logger: 底层日志记录器实例，通常由依赖注入框架提供
//
// 返回值:
//   - gin.HandlerFunc: 可直接注册到 Gin 路由的中间件函数
//
// 使用示例:
//
//	r := gin.New()
//	r.Use(ZapLoggerMiddleware(logger))
func ZapLoggerMiddleware(logger *logging.Logger) gin.HandlerFunc {
	apiLogger := ApiLogger{logger: logger}
	return func(c *gin.Context) {
		c.Set(LoggerCtxName, &apiLogger)
		start := time.Now()
		c.Next()
		apiLogger.Infof(
			c,
			[]any{
				zap.Int("SS", c.Writer.Status()),
				zap.Duration("LY", time.Since(start)),
			},
			"Incoming request",
		)
	}
}

// GetLogger 从 gin.Context 中获取 ApiLogger 实例
//
// 该函数应在 ZapLoggerMiddleware 之后执行的请求处理函数中调用，
// 用于获取当前请求的日志记录器，以便记录带有请求上下文的日志
//
// 参数:
//   - c: gin 上下文
//
// 返回值:
//   - *ApiLogger: 上下文中的 ApiLogger 实例
//
// 注意:
//
//	如果上下文中不存在 logger（通常是因为未注册 ZapLoggerMiddleware），
//	该函数会 panic，这是故意的快速失败设计，确保配置错误在开发阶段即被发现
func GetLogger(c *gin.Context) *ApiLogger {
	if logger, ok := c.Get(LoggerCtxName); ok {
		return logger.(*ApiLogger)
	}
	panic("logger does not exists")
}
