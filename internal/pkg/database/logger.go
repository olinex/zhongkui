package database

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// ZapGormLogger 是 GORM 日志接口的 Zap 适配器
// 将 GORM 的日志输出重定向到项目的 Zap 日志系统
type ZapGormLogger struct {
	// loggger 项目结构化日志器，GORM 日志经其输出（字段名沿用历史拼写）。
	loggger *logging.Logger
}

// NewZapGormLogger 创建新的 ZapGormLogger 实例
// 用于将 GORM 日志适配到项目的 Zap 日志系统
//
// 参数:
//   - logger: 项目的 Zap 日志记录器实例
//
// 返回:
//   - logger.Interface: 实现了 GORM 日志接口的适配器
func NewZapGormLogger(logger *logging.Logger) logger.Interface {
	return ZapGormLogger{
		loggger: logger,
	}
}

// LogMode 设置日志级别（当前实现中忽略级别设置）
// 注意：当前实现中日志级别控制由底层的 Zap 日志系统处理
//
// 参数:
//   - level: GORM 日志级别
//
// 返回:
//   - logger.Interface: 返回自身实例（级别设置被忽略）
func (l ZapGormLogger) LogMode(level logger.LogLevel) logger.Interface {
	return ZapGormLogger{
		loggger: l.loggger,
	}
}

// Info 记录信息级别日志
// 对应 GORM 的一般信息日志输出
//
// 参数:
//   - ctx: 上下文
//   - msg: 日志消息
//   - data: 可变参数，用于格式化消息
func (l ZapGormLogger) Info(ctx context.Context, msg string, data ...any) {
	// 过滤掉 GORM 自动替换的回调函数日志
	if strings.Contains(msg, "replacing callback") {
		return
	}
	l.loggger.Ctx(ctx).Infof(msg, data...)
}

// Warn 记录警告级别日志
// 对应 GORM 的警告信息日志输出
//
// 参数:
//   - ctx: 上下文
//   - msg: 日志消息
//   - data: 可变参数，用于格式化消息
func (l ZapGormLogger) Warn(ctx context.Context, msg string, data ...any) {
	l.loggger.Ctx(ctx).Warnf(msg, data...)
}

// Error 记录错误级别日志
// 对应 GORM 的错误信息日志输出
//
// 参数:
//   - ctx: 上下文
//   - msg: 日志消息
//   - data: 可变参数，用于格式化消息
func (l ZapGormLogger) Error(ctx context.Context, msg string, data ...any) {
	l.loggger.Ctx(ctx).Errorf(msg, data...)
}

// Trace 记录 SQL 跟踪日志
// 这是 GORM 日志系统的核心方法，记录所有 SQL 执行详情
// 包括执行时间、SQL 语句、影响行数和错误信息
//
// 参数:
//   - ctx: 上下文
//   - begin: SQL 开始执行的时间
//   - fc: 回调函数，返回 SQL 语句和影响行数
//   - err: SQL 执行错误（如果有）
func (l ZapGormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	elapsed := time.Since(begin)
	sql, rows := fc()

	// 根据执行结果和耗时选择适当的日志级别
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		// 记录不存在是查询未命中的正常结果（如按档位读取可选绑定、按 ID 探测存在性），
		// 调用方普遍将其视为业务状态而非故障，与 GORM 默认日志的
		// IgnoreRecordNotFoundError 语义一致，故不按错误级别输出，仅保留调试可见性
		l.loggger.Ctx(ctx).Debugf("sql record not found, elapsed: %v, sql: %s, rows: %d", elapsed, sql, rows)
	case err != nil:
		// SQL 执行错误：记录错误级别日志
		l.loggger.Ctx(ctx).Errorf("sql execution error: %v, elapsed: %v, sql: %s, rows: %d", err, elapsed, sql, rows)
	case elapsed > 200*time.Millisecond:
		// 慢查询：记录警告级别日志（执行时间超过200毫秒）
		l.loggger.Ctx(ctx).Warnf("slow sql query, elapsed: %v, sql: %s, rows: %d", elapsed, sql, rows)
	default:
		// 正常查询：记录调试级别日志
		l.loggger.Ctx(ctx).Debugf("sql execution, elapsed: %v, sql: %s, rows: %d", elapsed, sql, rows)
	}
}
