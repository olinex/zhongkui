package pipeline

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

type testHandlerConfig struct {
	Name     string        `mapstructure:"name" default:"default-name"`
	Timeout  time.Duration `mapstructure:"timeout" default:"5s"`
	MaxRetry int           `mapstructure:"maxRetry" default:"3"`
}

func TestGetHandlerParam_Success(t *testing.T) {
	Convey("TestGetHandlerParam 成功解析配置", t, func() {
		ctx := context.WithValue(context.Background(), CONTEXT_HANDLER_PARAM_NAME, map[string]any{
			"name":     "test-task",
			"timeout":  "10s",
			"maxRetry": 5,
		})

		cfg, err := GetHandlerParam[testHandlerConfig](ctx)

		Convey("不返回错误", func() {
			So(err, ShouldBeNil)
		})
		Convey("配置结构体正确解析", func() {
			So(cfg.Name, ShouldEqual, "test-task")
			So(cfg.Timeout, ShouldEqual, 10*time.Second)
			So(cfg.MaxRetry, ShouldEqual, 5)
		})
	})
}

func TestGetHandlerParam_NotFound(t *testing.T) {
	Convey("TestGetHandlerParam Context中无配置参数", t, func() {
		ctx := context.Background()

		_, err := GetHandlerParam[testHandlerConfig](ctx)

		Convey("返回错误", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestGetHandlerParam_DefaultValues(t *testing.T) {
	Convey("TestGetHandlerParam 验证默认值", t, func() {
		ctx := context.WithValue(context.Background(), CONTEXT_HANDLER_PARAM_NAME, map[string]any{
			"name": "partial-task",
		})

		cfg, err := GetHandlerParam[testHandlerConfig](ctx)

		Convey("不返回错误", func() {
			So(err, ShouldBeNil)
		})
		Convey("缺失字段使用默认值", func() {
			So(cfg.Name, ShouldEqual, "partial-task")
			So(cfg.Timeout, ShouldEqual, 5*time.Second)
			So(cfg.MaxRetry, ShouldEqual, 3)
		})
	})
}

func TestGetProcParam_Success(t *testing.T) {
	Convey("TestGetProcParam 成功解析配置", t, func() {
		ctx := context.WithValue(context.Background(), CONTEXT_PROC_PARAM_NAME, map[string]any{
			"name":     "proc-task",
			"timeout":  "30s",
			"maxRetry": 10,
		})

		cfg, err := GetProcParam[testHandlerConfig](ctx)

		Convey("不返回错误", func() {
			So(err, ShouldBeNil)
		})
		Convey("配置结构体正确解析", func() {
			So(cfg.Name, ShouldEqual, "proc-task")
			So(cfg.Timeout, ShouldEqual, 30*time.Second)
			So(cfg.MaxRetry, ShouldEqual, 10)
		})
	})
}

func TestGetProcParam_NotFound(t *testing.T) {
	Convey("TestGetProcParam Context中无处理器参数", t, func() {
		ctx := context.Background()

		_, err := GetProcParam[testHandlerConfig](ctx)

		Convey("返回错误", func() {
			So(err, ShouldNotBeNil)
		})
	})
}
