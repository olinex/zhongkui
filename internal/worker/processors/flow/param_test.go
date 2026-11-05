package flow

import (
	"testing"
	"time"

	"github.com/creasty/defaults"
	"github.com/go-playground/validator/v10"
	"github.com/mitchellh/mapstructure"
	. "github.com/smartystreets/goconvey/convey"
)

// TestParam_Defaults 测试默认值填充
// 创建零值 Param 结构体，调用 defaults.Set 后验证每个字段获得预期的默认值
func TestParam_Defaults(t *testing.T) {
	Convey("Param 结构体默认值测试", t, func() {
		var param Param
		err := defaults.Set(&param)
		So(err, ShouldBeNil)

		Convey("MaxReassemblerSize 默认值应为 1024", func() {
			So(param.MaxReassemblerSize, ShouldEqual, 1024)
		})

		Convey("MaxOutOrderPacketSize 默认值应为 256", func() {
			So(param.MaxOutOrderPacketSize, ShouldEqual, 256)
		})

		Convey("NoticeTimeout 默认值应为 30s", func() {
			So(param.NoticeTimeout, ShouldEqual, 30*time.Second)
		})

		Convey("PollInterval 默认值应为 5s", func() {
			So(param.PollInterval, ShouldEqual, 5*time.Second)
		})

		Convey("PollOffset 默认值应为 5m", func() {
			So(param.PollOffset, ShouldEqual, 5*time.Minute)
		})

		Convey("VNodeCount 默认值应为 128", func() {
			So(param.VNodeCount, ShouldEqual, 128)
		})

		Convey("BatchSize 默认值应为 1000", func() {
			So(param.BatchSize, ShouldEqual, 1000)
		})

		Convey("FlushInterval 默认值应为 1m", func() {
			So(param.FlushInterval, ShouldEqual, 1*time.Minute)
		})

		Convey("SaveInterval 默认值应为 30s", func() {
			So(param.SaveInterval, ShouldEqual, 30*time.Second)
		})

		Convey("ReplayWindow 默认值应为 5m", func() {
			So(param.ReplayWindow, ShouldEqual, 5*time.Minute)
		})
	})
}

// TestParam_DecodeFromMap 测试 mapstructure 解码
// 验证自定义值能正确解码，未提供的字段回退为默认值
func TestParam_DecodeFromMap(t *testing.T) {
	Convey("Param mapstructure 解码测试", t, func() {
		input := map[string]any{
			"maxReassemblerSize": 2048,
			"noticeTimeout":      "1m",
			"vNodeCount":         256,
			"batchSize":          5000,
			"flushInterval":      "2m",
			"replayWindow":       "10m",
		}

		var param Param
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			Result:     &param,
			TagName:    "mapstructure",
			DecodeHook: mapstructure.StringToTimeDurationHookFunc(),
		})
		So(err, ShouldBeNil)

		err = decoder.Decode(input)
		So(err, ShouldBeNil)

		// 填充未提供的字段的默认值
		err = defaults.Set(&param)
		So(err, ShouldBeNil)

		Convey("自定义值应被保留", func() {
			So(param.MaxReassemblerSize, ShouldEqual, 2048)
			So(param.NoticeTimeout, ShouldEqual, 1*time.Minute)
			So(param.VNodeCount, ShouldEqual, 256)
			So(param.BatchSize, ShouldEqual, 5000)
			So(param.FlushInterval, ShouldEqual, 2*time.Minute)
			So(param.ReplayWindow, ShouldEqual, 10*time.Minute)
		})

		Convey("未提供的字段应使用默认值", func() {
			So(param.MaxOutOrderPacketSize, ShouldEqual, 256)
			So(param.PollInterval, ShouldEqual, 5*time.Second)
			So(param.PollOffset, ShouldEqual, 5*time.Minute)
			So(param.SaveInterval, ShouldEqual, 30*time.Second)
		})
	})
}

// TestParam_BindingValidation 测试 binding 校验规则
// 验证合法值通过校验，非法值被拒绝
func TestParam_BindingValidation(t *testing.T) {
	validate := validator.New()
	validate.SetTagName("binding")

	Convey("Param binding 校验测试", t, func() {
		Convey("默认值应通过校验", func() {
			var param Param
			err := defaults.Set(&param)
			So(err, ShouldBeNil)

			err = validate.Struct(param)
			So(err, ShouldBeNil)
		})

		Convey("合法边界值应通过校验", func() {
			param := Param{
				MaxReassemblerSize:    1,
				MaxOutOrderPacketSize: 1,
				NoticeTimeout:         1 * time.Second,
				PollInterval:          1 * time.Second,
				PollOffset:            1 * time.Minute,
				VNodeCount:            0,
				BatchSize:             100,
				FlushInterval:         5 * time.Second,
				SaveInterval:          10 * time.Second,
				ReplayWindow:          1 * time.Minute,
			}
			err := validate.Struct(param)
			So(err, ShouldBeNil)
		})

		Convey("超出最大值的 MaxReassemblerSize 应校验失败", func() {
			param := Param{
				MaxReassemblerSize:    100000,
				MaxOutOrderPacketSize: 256,
				NoticeTimeout:         30 * time.Second,
				PollInterval:          5 * time.Second,
				PollOffset:            5 * time.Minute,
				VNodeCount:            128,
				BatchSize:             1000,
				FlushInterval:         1 * time.Minute,
				SaveInterval:          30 * time.Second,
				ReplayWindow:          5 * time.Minute,
			}
			err := validate.Struct(param)
			So(err, ShouldNotBeNil)
		})

		Convey("小于最小值的 MaxOutOrderPacketSize 应校验失败", func() {
			param := Param{
				MaxReassemblerSize:    1024,
				MaxOutOrderPacketSize: 0,
				NoticeTimeout:         30 * time.Second,
				PollInterval:          5 * time.Second,
				PollOffset:            5 * time.Minute,
				VNodeCount:            128,
				BatchSize:             1000,
				FlushInterval:         1 * time.Minute,
				SaveInterval:          30 * time.Second,
				ReplayWindow:          5 * time.Minute,
			}
			err := validate.Struct(param)
			So(err, ShouldNotBeNil)
		})

		Convey("超出最大值的 VNodeCount 应校验失败", func() {
			param := Param{
				MaxReassemblerSize:    1024,
				MaxOutOrderPacketSize: 256,
				NoticeTimeout:         30 * time.Second,
				PollInterval:          5 * time.Second,
				PollOffset:            5 * time.Minute,
				VNodeCount:            2000,
				BatchSize:             1000,
				FlushInterval:         1 * time.Minute,
				SaveInterval:          30 * time.Second,
				ReplayWindow:          5 * time.Minute,
			}
			err := validate.Struct(param)
			So(err, ShouldNotBeNil)
		})

		Convey("NoticeTimeout 超出最大值应校验失败", func() {
			param := Param{
				MaxReassemblerSize:    1024,
				MaxOutOrderPacketSize: 256,
				NoticeTimeout:         10 * time.Minute,
				PollInterval:          5 * time.Second,
				PollOffset:            5 * time.Minute,
				VNodeCount:            128,
				BatchSize:             1000,
				FlushInterval:         1 * time.Minute,
				SaveInterval:          30 * time.Second,
				ReplayWindow:          5 * time.Minute,
			}
			err := validate.Struct(param)
			So(err, ShouldNotBeNil)
		})

		Convey("BatchSize 超出最大值应校验失败", func() {
			param := Param{
				MaxReassemblerSize:    1024,
				MaxOutOrderPacketSize: 256,
				NoticeTimeout:         30 * time.Second,
				PollInterval:          5 * time.Second,
				PollOffset:            5 * time.Minute,
				VNodeCount:            128,
				BatchSize:             20000,
				FlushInterval:         1 * time.Minute,
				SaveInterval:          30 * time.Second,
				ReplayWindow:          5 * time.Minute,
			}
			err := validate.Struct(param)
			So(err, ShouldNotBeNil)
		})

		Convey("PollInterval 小于最小值应校验失败", func() {
			param := Param{
				MaxReassemblerSize:    1024,
				MaxOutOrderPacketSize: 256,
				NoticeTimeout:         30 * time.Second,
				PollInterval:          500 * time.Millisecond,
				PollOffset:            5 * time.Minute,
				VNodeCount:            128,
				BatchSize:             1000,
				FlushInterval:         1 * time.Minute,
				SaveInterval:          30 * time.Second,
				ReplayWindow:          5 * time.Minute,
			}
			err := validate.Struct(param)
			So(err, ShouldNotBeNil)
		})
	})
}
