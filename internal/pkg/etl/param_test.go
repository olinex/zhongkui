package etl

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

type testProcConfig struct {
	Name     string        `mapstructure:"name" default:"default-name"`
	Timeout  time.Duration `mapstructure:"timeout" default:"5s"`
	MaxRetry int           `mapstructure:"maxRetry" default:"3"`
}

func TestGetProcParam_Success(t *testing.T) {
	Convey("TestGetProcParam 成功解析配置", t, func() {
		ctx := context.WithValue(context.Background(), ContextProcParamName, map[string]any{
			"name":     "proc-task",
			"timeout":  "30s",
			"maxRetry": 10,
		})

		cfg, err := GetProcParam[testProcConfig](ctx)

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

		_, err := GetProcParam[testProcConfig](ctx)

		Convey("返回错误", func() {
			So(err, ShouldNotBeNil)
		})
	})
}
