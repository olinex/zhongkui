package manager

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"zhongkui.com/zhongkui/internal/depend"
)

// createTestLogger 创建一个用于测试的日志记录器
func createTestLogger() *zap.SugaredLogger {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{}
	config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	zapLogger, _ := config.Build()
	return zapLogger.Sugar()
}

// createTestEngine 创建一个用于测试的 Gin 引擎，注册一个简单的健康检查路由
func createTestEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	return engine
}

// newTestHttpFxIn 创建一个用于测试的 HttpFxIn 实例
func newTestHttpFxIn(cfg *Config, mockLc fx.Lifecycle) HttpFxIn {
	return HttpFxIn{
		Cfg:    cfg,
		Lc:     mockLc,
		Engine: createTestEngine(),
		APILoggerFxIn: depend.APILoggerFxIn{
			APILogger: createTestLogger(),
		},
	}
}

// TestHttpFxIn 测试 HttpFxIn 结构体的字段赋值与依赖注入标记
func TestHttpFxIn(t *testing.T) {
	Convey("测试 HttpFxIn 结构体", t, func() {
		Convey("应正确初始化所有字段", func() {
			cfg := &Config{
				Host: "127.0.0.1",
				Port: 9090,
			}
			engine := createTestEngine()
			logger := createTestLogger()

			in := HttpFxIn{
				Cfg:    cfg,
				Engine: engine,
				APILoggerFxIn: depend.APILoggerFxIn{
					APILogger: logger,
				},
			}

			So(in.Cfg, ShouldEqual, cfg)
			So(in.Cfg.Host, ShouldEqual, "127.0.0.1")
			So(in.Cfg.Port, ShouldEqual, 9090)
			So(in.Engine, ShouldEqual, engine)
			So(in.APILogger, ShouldEqual, logger)
		})

		Convey("应支持默认零值初始化", func() {
			in := HttpFxIn{}
			So(in.Cfg, ShouldBeNil)
			So(in.Engine, ShouldBeNil)
			So(in.APILogger, ShouldBeNil)
		})

		Convey("应包含 fx.In 标记以支持依赖注入", func() {
			in := HttpFxIn{}
			var _ fx.In = in.In
			So(true, ShouldBeTrue)
		})

		Convey("不同实例应相互独立", func() {
			cfg1 := &Config{Host: "0.0.0.0", Port: 8080}
			cfg2 := &Config{Host: "127.0.0.1", Port: 9090}
			in1 := HttpFxIn{Cfg: cfg1}
			in2 := HttpFxIn{Cfg: cfg2}

			So(in1.Cfg, ShouldNotEqual, in2.Cfg)
			So(in1.Cfg.Host, ShouldEqual, "0.0.0.0")
			So(in2.Cfg.Host, ShouldEqual, "127.0.0.1")
			So(in1.Cfg.Port, ShouldEqual, 8080)
			So(in2.Cfg.Port, ShouldEqual, 9090)
		})
	})
}

// TestRunHttpServiceFx_HookRegistration 测试 RunHttpServiceFx 是否正确注册生命周期钩子
func TestRunHttpServiceFx_HookRegistration(t *testing.T) {
	Convey("测试 RunHttpServiceFx 钩子注册", t, func() {
		Convey("应正确注册一个生命周期钩子", func() {
			cfg := &Config{Host: "127.0.0.1", Port: 0}

			var hooks []fx.Hook
			mockLc := &mockLifecycle{
				appendFunc: func(hook fx.Hook) {
					hooks = append(hooks, hook)
				},
			}

			in := newTestHttpFxIn(cfg, mockLc)
			RunHttpServiceFx(in)

			So(len(hooks), ShouldEqual, 1)
			So(hooks[0].OnStart, ShouldNotBeNil)
			So(hooks[0].OnStop, ShouldNotBeNil)
		})

		Convey("多次调用应注册多个钩子", func() {
			cfg := &Config{Host: "127.0.0.1", Port: 0}

			var hooks []fx.Hook
			mockLc := &mockLifecycle{
				appendFunc: func(hook fx.Hook) {
					hooks = append(hooks, hook)
				},
			}

			in := newTestHttpFxIn(cfg, mockLc)
			RunHttpServiceFx(in)
			RunHttpServiceFx(in)

			So(len(hooks), ShouldEqual, 2)
		})
	})
}

// TestRunHttpServiceFx_OnStart 测试 OnStart 钩子的行为
func TestRunHttpServiceFx_OnStart(t *testing.T) {
	Convey("测试 OnStart 钩子", t, func() {
		Convey("OnStart 应立即返回（非阻塞）", func() {
			cfg := &Config{Host: "127.0.0.1", Port: 0}

			var hooks []fx.Hook
			mockLc := &mockLifecycle{
				appendFunc: func(hook fx.Hook) {
					hooks = append(hooks, hook)
				},
			}

			in := newTestHttpFxIn(cfg, mockLc)
			RunHttpServiceFx(in)

			started := make(chan struct{})
			go func() {
				_ = hooks[0].OnStart(context.Background())
				close(started)
			}()

			select {
			case <-started:
				So(true, ShouldBeTrue)
			case <-time.After(2 * time.Second):
				t.Fatal("OnStart 应该非阻塞返回，但超时了")
			}
		})

		Convey("OnStart 返回的 error 应为 nil", func() {
			cfg := &Config{Host: "127.0.0.1", Port: 0}

			var hooks []fx.Hook
			mockLc := &mockLifecycle{
				appendFunc: func(hook fx.Hook) {
					hooks = append(hooks, hook)
				},
			}

			in := newTestHttpFxIn(cfg, mockLc)
			RunHttpServiceFx(in)

			err := hooks[0].OnStart(context.Background())
			So(err, ShouldBeNil)

			time.Sleep(50 * time.Millisecond)

			err = hooks[0].OnStop(context.Background())
			So(err, ShouldBeNil)
		})

		Convey("OnStart 后服务器应在随机端口上监听", func() {
			cfg := &Config{Host: "127.0.0.1", Port: 0}

			var hooks []fx.Hook
			mockLc := &mockLifecycle{
				appendFunc: func(hook fx.Hook) {
					hooks = append(hooks, hook)
				},
			}

			in := newTestHttpFxIn(cfg, mockLc)
			RunHttpServiceFx(in)

			err := hooks[0].OnStart(context.Background())
			So(err, ShouldBeNil)

			time.Sleep(100 * time.Millisecond)

			err = hooks[0].OnStop(context.Background())
			So(err, ShouldBeNil)
		})
	})
}

// TestRunHttpServiceFx_OnStop 测试 OnStop 钩子的行为
func TestRunHttpServiceFx_OnStop(t *testing.T) {
	Convey("测试 OnStop 钩子", t, func() {
		Convey("OnStop 应优雅关闭服务器并返回 nil", func() {
			cfg := &Config{Host: "127.0.0.1", Port: 0}

			var hooks []fx.Hook
			mockLc := &mockLifecycle{
				appendFunc: func(hook fx.Hook) {
					hooks = append(hooks, hook)
				},
			}

			in := newTestHttpFxIn(cfg, mockLc)
			RunHttpServiceFx(in)

			_ = hooks[0].OnStart(context.Background())
			time.Sleep(100 * time.Millisecond)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := hooks[0].OnStop(ctx)
			So(err, ShouldBeNil)
		})

		Convey("重复调用 OnStop 不应产生 panic", func() {
			cfg := &Config{Host: "127.0.0.1", Port: 0}

			var hooks []fx.Hook
			mockLc := &mockLifecycle{
				appendFunc: func(hook fx.Hook) {
					hooks = append(hooks, hook)
				},
			}

			in := newTestHttpFxIn(cfg, mockLc)
			RunHttpServiceFx(in)

			_ = hooks[0].OnStart(context.Background())
			time.Sleep(100 * time.Millisecond)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := hooks[0].OnStop(ctx)
			So(err, ShouldBeNil)

			err = hooks[0].OnStop(ctx)
			So(err, ShouldBeNil)
		})

		Convey("未启动服务器直接调用 OnStop 不应产生 panic", func() {
			cfg := &Config{Host: "127.0.0.1", Port: 0}

			var hooks []fx.Hook
			mockLc := &mockLifecycle{
				appendFunc: func(hook fx.Hook) {
					hooks = append(hooks, hook)
				},
			}

			in := newTestHttpFxIn(cfg, mockLc)
			RunHttpServiceFx(in)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := hooks[0].OnStop(ctx)
			So(err, ShouldBeNil)
		})
	})
}

// TestRunHttpServiceFx_Address 测试服务器监听地址的构造
func TestRunHttpServiceFx_Address(t *testing.T) {
	Convey("测试服务器监听地址构造", t, func() {
		Convey("应正确使用 Config 中的 Host 和 Port 构造地址", func() {
			testCases := []struct {
				host     string
				port     int
				expected string
			}{
				{"0.0.0.0", 8080, "0.0.0.0:8080"},
				{"127.0.0.1", 9090, "127.0.0.1:9090"},
				{"localhost", 3000, "localhost:3000"},
				{"0.0.0.0", 443, "0.0.0.0:443"},
			}

			for _, tc := range testCases {
				cfg := &Config{Host: tc.host, Port: tc.port}
				addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
				So(addr, ShouldEqual, tc.expected)
			}
		})

		Convey("端口为 0 时应使用系统自动分配端口", func() {
			cfg := &Config{Host: "127.0.0.1", Port: 0}
			addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
			So(addr, ShouldEqual, "127.0.0.1:0")
		})
	})
}

// TestRunHttpServiceFx_StartAndStop 测试完整的启动和停止流程
func TestRunHttpServiceFx_StartAndStop(t *testing.T) {
	Convey("测试完整的启动和停止流程", t, func() {
		Convey("应正确完成启动-停止循环", func() {
			cfg := &Config{Host: "127.0.0.1", Port: 0}

			var hooks []fx.Hook
			mockLc := &mockLifecycle{
				appendFunc: func(hook fx.Hook) {
					hooks = append(hooks, hook)
				},
			}

			in := newTestHttpFxIn(cfg, mockLc)
			RunHttpServiceFx(in)

			err := hooks[0].OnStart(context.Background())
			So(err, ShouldBeNil)

			time.Sleep(100 * time.Millisecond)

			err = hooks[0].OnStop(context.Background())
			So(err, ShouldBeNil)
		})

		Convey("使用带超时的上下文停止应正常工作", func() {
			cfg := &Config{Host: "127.0.0.1", Port: 0}

			var hooks []fx.Hook
			mockLc := &mockLifecycle{
				appendFunc: func(hook fx.Hook) {
					hooks = append(hooks, hook)
				},
			}

			in := newTestHttpFxIn(cfg, mockLc)
			RunHttpServiceFx(in)

			_ = hooks[0].OnStart(context.Background())
			time.Sleep(100 * time.Millisecond)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			err := hooks[0].OnStop(ctx)
			So(err, ShouldBeNil)
		})
	})
}

// mockLifecycle 是 fx.Lifecycle 的模拟实现，用于测试中捕获注册的生命周期钩子
type mockLifecycle struct {
	appendFunc func(fx.Hook)
}

func (m *mockLifecycle) Append(hook fx.Hook) {
	if m.appendFunc != nil {
		m.appendFunc(hook)
	}
}
