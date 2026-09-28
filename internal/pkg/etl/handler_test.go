package etl

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

// TestGetHandlerParam_Success 验证从上下文成功解析处理器配置。
func TestGetHandlerParam_Success(t *testing.T) {
	Convey("TestGetHandlerParam 成功解析配置", t, func() {
		ctx := context.WithValue(context.Background(), ContextHandlerParamName, map[string]any{
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

// TestGetHandlerParam_NotFound 验证上下文缺少配置参数时返回错误。
func TestGetHandlerParam_NotFound(t *testing.T) {
	Convey("TestGetHandlerParam Context中无配置参数", t, func() {
		ctx := context.Background()

		_, err := GetHandlerParam[testHandlerConfig](ctx)

		Convey("返回错误", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

// TestGetHandlerParam_DefaultValues 验证配置缺失字段回落到默认值。
func TestGetHandlerParam_DefaultValues(t *testing.T) {
	Convey("TestGetHandlerParam 验证默认值", t, func() {
		ctx := context.WithValue(context.Background(), ContextHandlerParamName, map[string]any{
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
