package database

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// TestNewZapGormLogger 测试 NewZapGormLogger 函数的创建行为
func TestNewZapGormLogger(t *testing.T) {
	Convey("当创建新的 ZapGormLogger 时", t, func() {
		// 创建一个简单的 zap 日志记录器
		config := zap.NewProductionConfig()
		config.OutputPaths = []string{}
		config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
		zapLogger, _ := config.Build()
		log := &logging.Logger{SugaredLogger: zapLogger.Sugar()}
		
		gormLogger := NewZapGormLogger(log)
		
		Convey("应该返回非空的日志记录器", func() {
			So(gormLogger, ShouldNotBeNil)
		})
	})
}

// TestZapGormLogger_LogMode 测试 LogMode 方法的行为
func TestZapGormLogger_LogMode(t *testing.T) {
	Convey("当调用 LogMode 方法时", t, func() {
		// 创建一个简单的 zap 日志记录器
		config := zap.NewProductionConfig()
		config.OutputPaths = []string{}
		config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
		zapLogger, _ := config.Build()
		log := &logging.Logger{SugaredLogger: zapLogger.Sugar()}
		
		zapLoggerStruct := ZapGormLogger{loggger: log}
		
		Convey("使用 Info 日志级别", func() {
			result := zapLoggerStruct.LogMode(0) // 使用 0 作为日志级别
			So(result, ShouldNotBeNil)
			
			// 应该返回新的 ZapGormLogger 实例
			newLogger, ok := result.(ZapGormLogger)
			So(ok, ShouldBeTrue)
			So(newLogger.loggger, ShouldEqual, log)
		})
	})
}

// TestZapGormLogger_Info 测试 Info 方法的行为
func TestZapGormLogger_Info(t *testing.T) {
	Convey("当调用 Info 方法时", t, func() {
		// 创建一个简单的 zap 日志记录器
		config := zap.NewProductionConfig()
		config.OutputPaths = []string{}
		config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
		zapLogger, _ := config.Build()
		log := &logging.Logger{SugaredLogger: zapLogger.Sugar()}
		
		zapLoggerStruct := ZapGormLogger{loggger: log}
		ctx := context.Background()
		
		Convey("使用简单消息", func() {
			// 不应该 panic
			So(func() {
				zapLoggerStruct.Info(ctx, "test message")
			}, ShouldNotPanic)
		})
		
		Convey("使用带格式的消息", func() {
			// 不应该 panic
			So(func() {
				zapLoggerStruct.Info(ctx, "user %s logged in", "john")
			}, ShouldNotPanic)
		})
	})
}

// TestZapGormLogger_Warn 测试 Warn 方法的行为
func TestZapGormLogger_Warn(t *testing.T) {
	Convey("当调用 Warn 方法时", t, func() {
		// 创建一个简单的 zap 日志记录器
		config := zap.NewProductionConfig()
		config.OutputPaths = []string{}
		config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
		zapLogger, _ := config.Build()
		log := &logging.Logger{SugaredLogger: zapLogger.Sugar()}
		
		zapLoggerStruct := ZapGormLogger{loggger: log}
		ctx := context.Background()
		
		Convey("使用简单消息", func() {
			// 不应该 panic
			So(func() {
				zapLoggerStruct.Warn(ctx, "warning message")
			}, ShouldNotPanic)
		})
	})
}

// TestZapGormLogger_Error 测试 Error 方法的行为
func TestZapGormLogger_Error(t *testing.T) {
	Convey("当调用 Error 方法时", t, func() {
		// 创建一个简单的 zap 日志记录器
		config := zap.NewProductionConfig()
		config.OutputPaths = []string{}
		config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
		zapLogger, _ := config.Build()
		log := &logging.Logger{SugaredLogger: zapLogger.Sugar()}
		
		zapLoggerStruct := ZapGormLogger{loggger: log}
		ctx := context.Background()
		
		Convey("使用简单消息", func() {
			// 不应该 panic
			So(func() {
				zapLoggerStruct.Error(ctx, "error message")
			}, ShouldNotPanic)
		})
	})
}

// TestZapGormLogger_Trace 测试 Trace 方法的行为
func TestZapGormLogger_Trace(t *testing.T) {
	Convey("当调用 Trace 方法时", t, func() {
		// 创建一个简单的 zap 日志记录器
		config := zap.NewProductionConfig()
		config.OutputPaths = []string{}
		config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
		zapLogger, _ := config.Build()
		log := &logging.Logger{SugaredLogger: zapLogger.Sugar()}
		
		zapLoggerStruct := ZapGormLogger{loggger: log}
		ctx := context.Background()
		
		Convey("当 SQL 执行有错误时", func() {
			begin := time.Now().Add(-100 * time.Millisecond)
			fc := func() (string, int64) {
				return "SELECT * FROM users WHERE id = ?", 1
			}
			err := context.DeadlineExceeded
			
			// 不应该 panic
			So(func() {
				zapLoggerStruct.Trace(ctx, begin, fc, err)
			}, ShouldNotPanic)
		})
		
		Convey("当 SQL 执行时间超过慢查询阈值时", func() {
			begin := time.Now().Add(-300 * time.Millisecond) // 300ms > 200ms 阈值
			fc := func() (string, int64) {
				return "SELECT * FROM orders JOIN users ON orders.user_id = users.id", 100
			}
			
			// 不应该 panic
			So(func() {
				zapLoggerStruct.Trace(ctx, begin, fc, nil)
			}, ShouldNotPanic)
		})
		
		Convey("当 SQL 执行时间在正常范围内时", func() {
			begin := time.Now().Add(-50 * time.Millisecond) // 50ms < 200ms 阈值
			fc := func() (string, int64) {
				return "SELECT id, name FROM users", 10
			}
			
			// 不应该 panic
			So(func() {
				zapLoggerStruct.Trace(ctx, begin, fc, nil)
			}, ShouldNotPanic)
		})
	})
}

// TestZapGormLogger_Trace_EdgeCases 测试 Trace 方法的边界情况
func TestZapGormLogger_Trace_EdgeCases(t *testing.T) {
	Convey("当调用 Trace 方法处理边界情况时", t, func() {
		// 创建一个简单的 zap 日志记录器
		config := zap.NewProductionConfig()
		config.OutputPaths = []string{}
		config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
		zapLogger, _ := config.Build()
		log := &logging.Logger{SugaredLogger: zapLogger.Sugar()}
		
		zapLoggerStruct := ZapGormLogger{loggger: log}
		ctx := context.Background()
		
		Convey("当开始时间为未来时间时", func() {
			begin := time.Now().Add(100 * time.Millisecond) // 未来时间
			fc := func() (string, int64) {
				return "SELECT 1", 1
			}
			
			// 不应该 panic
			So(func() {
				zapLoggerStruct.Trace(ctx, begin, fc, nil)
			}, ShouldNotPanic)
		})
		
		Convey("当回调函数返回负的行数时", func() {
			begin := time.Now().Add(-10 * time.Millisecond)
			fc := func() (string, int64) {
				return "INVALID QUERY", -1
			}
			
			// 不应该 panic
			So(func() {
				zapLoggerStruct.Trace(ctx, begin, fc, nil)
			}, ShouldNotPanic)
		})
	})
}