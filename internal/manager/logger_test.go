package manager

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// newObservedLogger 创建一个基于 observer 的测试日志记录器
// 返回日志记录器和 observer 实例，后者可用于断言日志内容
func newObservedLogger() (*logging.Logger, *observer.ObservedLogs) {
	core, recorded := observer.New(zapcore.DebugLevel)
	logger := zap.New(core).Sugar()
	return logger, recorded
}

// newLoggerGinContext 创建用于测试的 gin.Context
// 可选地设置请求方法和路径
func newLoggerGinContext(method, path string) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, nil)
	c.Request.Header.Set("X-Real-Ip", "192.168.1.1")
	return c
}

// TestApiLogger_logf 测试 ApiLogger.logf 方法是否正确附加请求上下文字段
func TestApiLogger_logf(t *testing.T) {
	Convey("TestApiLogger_logf", t, func() {
		logger, recorded := newObservedLogger()
		apiLogger := &ApiLogger{logger: logger}
		c := newLoggerGinContext(http.MethodPost, "/api/test")

		Convey("应该将请求方法、路径和 IP 附加到日志字段中", func() {
			apiLogger.logf(zapcore.InfoLevel, c, nil, "test message")

			So(recorded.Len(), ShouldEqual, 1)
			entry := recorded.All()[0]
			So(entry.Message, ShouldEqual, "test message")
			So(entry.Context[0].Key, ShouldEqual, "method")
			So(entry.Context[0].String, ShouldEqual, "POST")
			So(entry.Context[1].Key, ShouldEqual, "path")
			So(entry.Context[1].String, ShouldEqual, "/api/test")
			So(entry.Context[2].Key, ShouldEqual, "ip")
		})

		Convey("应该将用户自定义字段与上下文字段合并", func() {
			fields := []any{zap.String("custom_key", "custom_value")}
			apiLogger.logf(zapcore.InfoLevel, c, fields, "with custom fields")

			So(recorded.Len(), ShouldEqual, 1)
			entry := recorded.All()[0]
			So(entry.Context[0].Key, ShouldEqual, "custom_key")
			So(entry.Context[0].String, ShouldEqual, "custom_value")
			So(entry.Context[1].Key, ShouldEqual, "method")
			So(entry.Context[3].Key, ShouldEqual, "ip")
		})

		Convey("应该支持格式化模板参数", func() {
			apiLogger.logf(zapcore.InfoLevel, c, nil, "user %s performed action %d", "alice", 42)

			So(recorded.Len(), ShouldEqual, 1)
			So(recorded.All()[0].Message, ShouldEqual, "user alice performed action 42")
		})

		Convey("当 fields 为空切片时仍应正确记录日志", func() {
			apiLogger.logf(zapcore.InfoLevel, c, []any{}, "empty fields")

			So(recorded.Len(), ShouldEqual, 1)
			So(recorded.All()[0].Message, ShouldEqual, "empty fields")
		})
	})
}

// TestApiLogger_Debugf 测试 Debugf 方法
func TestApiLogger_Debugf(t *testing.T) {
	Convey("TestApiLogger_Debugf", t, func() {
		logger, recorded := newObservedLogger()
		apiLogger := &ApiLogger{logger: logger}
		c := newLoggerGinContext(http.MethodGet, "/debug")

		Convey("应该记录 Debug 级别的日志", func() {
			apiLogger.Debugf(c, nil, "debug message")

			So(recorded.Len(), ShouldEqual, 1)
			So(recorded.All()[0].Level, ShouldEqual, zapcore.DebugLevel)
			So(recorded.All()[0].Message, ShouldEqual, "debug message")
		})

		Convey("应该附加请求上下文字段", func() {
			apiLogger.Debugf(c, nil, "debug with context")

			entry := recorded.All()[0]
			So(entry.Context[0].Key, ShouldEqual, "method")
			So(entry.Context[0].String, ShouldEqual, "GET")
		})
	})
}

// TestApiLogger_Infof 测试 Infof 方法
func TestApiLogger_Infof(t *testing.T) {
	Convey("TestApiLogger_Infof", t, func() {
		logger, recorded := newObservedLogger()
		apiLogger := &ApiLogger{logger: logger}
		c := newLoggerGinContext(http.MethodGet, "/info")

		Convey("应该记录 Info 级别的日志", func() {
			apiLogger.Infof(c, nil, "info message")

			So(recorded.Len(), ShouldEqual, 1)
			So(recorded.All()[0].Level, ShouldEqual, zapcore.InfoLevel)
			So(recorded.All()[0].Message, ShouldEqual, "info message")
		})

		Convey("应该支持带自定义字段的 Info 日志", func() {
			apiLogger.Infof(c, []any{zap.Int("code", 200)}, "info with fields")

			So(recorded.Len(), ShouldEqual, 1)
			entry := recorded.All()[0]
			So(entry.Context[0].Key, ShouldEqual, "code")
			So(entry.Context[0].Integer, ShouldEqual, 200)
		})
	})
}

// TestApiLogger_Warnf 测试 Warnf 方法
func TestApiLogger_Warnf(t *testing.T) {
	Convey("TestApiLogger_Warnf", t, func() {
		logger, recorded := newObservedLogger()
		apiLogger := &ApiLogger{logger: logger}
		c := newLoggerGinContext(http.MethodPut, "/warn")

		Convey("应该记录 Warn 级别的日志", func() {
			apiLogger.Warnf(c, nil, "warn message")

			So(recorded.Len(), ShouldEqual, 1)
			So(recorded.All()[0].Level, ShouldEqual, zapcore.WarnLevel)
			So(recorded.All()[0].Message, ShouldEqual, "warn message")
		})

		Convey("应该附加请求上下文字段", func() {
			apiLogger.Warnf(c, nil, "warn with context")

			entry := recorded.All()[0]
			So(entry.Context[0].Key, ShouldEqual, "method")
			So(entry.Context[0].String, ShouldEqual, "PUT")
			So(entry.Context[1].Key, ShouldEqual, "path")
			So(entry.Context[1].String, ShouldEqual, "/warn")
		})
	})
}

// TestApiLogger_Errorf 测试 Errorf 方法
func TestApiLogger_Errorf(t *testing.T) {
	Convey("TestApiLogger_Errorf", t, func() {
		logger, recorded := newObservedLogger()
		apiLogger := &ApiLogger{logger: logger}
		c := newLoggerGinContext(http.MethodDelete, "/error")

		Convey("应该记录 Error 级别的日志", func() {
			apiLogger.Errorf(c, nil, "error message")

			So(recorded.Len(), ShouldEqual, 1)
			So(recorded.All()[0].Level, ShouldEqual, zapcore.ErrorLevel)
			So(recorded.All()[0].Message, ShouldEqual, "error message")
		})

		Convey("应该附加请求上下文字段", func() {
			apiLogger.Errorf(c, nil, "error with context")

			entry := recorded.All()[0]
			So(entry.Context[0].Key, ShouldEqual, "method")
			So(entry.Context[0].String, ShouldEqual, "DELETE")
			So(entry.Context[1].Key, ShouldEqual, "path")
			So(entry.Context[1].String, ShouldEqual, "/error")
		})
	})
}

// TestZapLoggerMiddleware 测试 ZapLoggerMiddleware 中间件
func TestZapLoggerMiddleware(t *testing.T) {
	Convey("TestZapLoggerMiddleware", t, func() {
		logger, recorded := newObservedLogger()
		middleware := ZapLoggerMiddleware(logger)

		Convey("应该将 ApiLogger 存入 gin.Context", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(middleware)
			r.GET("/test", func(c *gin.Context) {
				apiLogger, exists := c.Get(LOGGER_CTX_NAME)
				So(exists, ShouldBeTrue)
				So(apiLogger, ShouldHaveSameTypeAs, &ApiLogger{})
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("请求完成后应该记录请求日志", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(middleware)
			r.GET("/api/log", func(c *gin.Context) {
				c.Status(http.StatusCreated)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/log", nil)
			r.ServeHTTP(w, req)

			So(recorded.Len(), ShouldBeGreaterThan, 0)
			entry := recorded.All()[0]
			So(entry.Message, ShouldEqual, "Incoming request")
			So(entry.Level, ShouldEqual, zapcore.InfoLevel)

			fieldKeys := make(map[string]bool)
			for _, f := range entry.Context {
				fieldKeys[f.Key] = true
			}
			So(fieldKeys["SS"], ShouldBeTrue)
			So(fieldKeys["LY"], ShouldBeTrue)
			So(fieldKeys["method"], ShouldBeTrue)
			So(fieldKeys["path"], ShouldBeTrue)
			So(fieldKeys["ip"], ShouldBeTrue)
		})

		Convey("日志中应包含正确的响应状态码", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(middleware)
			r.GET("/api/status", func(c *gin.Context) {
				c.Status(http.StatusNotFound)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
			r.ServeHTTP(w, req)

			So(recorded.Len(), ShouldEqual, 1)
			entry := recorded.All()[0]
			var statusCode int
			for _, f := range entry.Context {
				if f.Key == "SS" {
					statusCode = int(f.Integer)
					break
				}
			}
			So(statusCode, ShouldEqual, http.StatusNotFound)
		})
	})
}

// TestGetLogger 测试 GetLogger 函数
func TestGetLogger(t *testing.T) {
	Convey("TestGetLogger", t, func() {
		Convey("当上下文中存在 logger 时应正确返回", func() {
			c := &gin.Context{}
			c.Keys = make(map[string]any)
			apiLogger := &ApiLogger{}
			c.Keys[LOGGER_CTX_NAME] = apiLogger

			result := GetLogger(c)
			So(result, ShouldEqual, apiLogger)
		})

		Convey("当上下文中不存在 logger 时应该 panic", func() {
			c := &gin.Context{}
			c.Keys = make(map[string]any)

			So(func() { GetLogger(c) }, ShouldPanicWith, "logger does not exists")
		})

		Convey("当上下文中存储了错误类型时应该 panic", func() {
			c := &gin.Context{}
			c.Keys = make(map[string]any)
			c.Keys[LOGGER_CTX_NAME] = "not an ApiLogger"

			So(func() { GetLogger(c) }, ShouldPanic)
		})
	})
}

// TestLOGGER_CTX_NAME 测试常量值
func TestLOGGER_CTX_NAME(t *testing.T) {
	Convey("TestLOGGER_CTX_NAME", t, func() {
		Convey("LOGGER_CTX_NAME 应等于 logger", func() {
			So(LOGGER_CTX_NAME, ShouldEqual, "logger")
		})
	})
}
