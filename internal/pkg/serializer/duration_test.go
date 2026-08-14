package serializer

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/creasty/defaults"
	. "github.com/smartystreets/goconvey/convey"
)

// TestDuration_JSON 验证 Duration 的 JSON 序列化与反序列化对称。
// 核心契约：序列化输出可读字符串（如 "5s"），反序列化同样接受该字符串格式。
func TestDuration_JSON(t *testing.T) {
	Convey("Test Duration JSON serialization symmetry", t, func() {
		Convey("Marshal 输出可读字符串而非纳秒整数", func() {
			d := DurationNS(5 * time.Second)
			data, err := json.Marshal(d)
			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, `"5s"`)
		})

		Convey("Marshal 复合时长", func() {
			d := DurationNS(90 * time.Second)
			data, err := json.Marshal(d)
			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, `"1m30s"`)
		})

		Convey("Unmarshal 字符串时长与 Marshal 对称", func() {
			var d DurationNS
			err := json.Unmarshal([]byte(`"5s"`), &d)
			So(err, ShouldBeNil)
			So(d, ShouldEqual, DurationNS(5*time.Second))

			roundTrip, err := json.Marshal(d)
			So(err, ShouldBeNil)
			So(string(roundTrip), ShouldEqual, `"5s"`)
		})

		Convey("Unmarshal 兼容整数纳秒", func() {
			var d DurationNS
			err := json.Unmarshal([]byte("5000000000"), &d)
			So(err, ShouldBeNil)
			So(d, ShouldEqual, DurationNS(5*time.Second))
		})

		Convey("Unmarshal null 与空串按 0 处理", func() {
			var d DurationNS
			So(json.Unmarshal([]byte("null"), &d), ShouldBeNil)
			So(d, ShouldEqual, 0)

			So(json.Unmarshal([]byte(`""`), &d), ShouldBeNil)
			So(d, ShouldEqual, 0)
		})

		Convey("Unmarshal 非法格式返回错误", func() {
			var d DurationNS
			So(json.Unmarshal([]byte(`"invalid"`), &d), ShouldNotBeNil)
		})

		Convey("嵌套结构体 JSON 往返", func() {
			type wrapper struct {
				Window DurationNS `json:"window"`
			}
			raw := `{"window":"1m30s"}`
			var w wrapper
			So(json.Unmarshal([]byte(raw), &w), ShouldBeNil)
			So(w.Window, ShouldEqual, DurationNS(90*time.Second))

			out, err := json.Marshal(w)
			So(err, ShouldBeNil)
			So(string(out), ShouldEqual, raw)
		})
	})
}

// TestDuration_Text 验证 Duration 的文本编解码（Viper/mapstructure/defaults 依赖此接口）。
func TestDuration_Text(t *testing.T) {
	Convey("Test Duration text marshal/unmarshal", t, func() {
		Convey("MarshalText 输出可读字符串", func() {
			data, err := DurationNS(5 * time.Second).MarshalText()
			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, "5s")
		})

		Convey("UnmarshalText 解析字符串时长", func() {
			var d DurationNS
			So(d.UnmarshalText([]byte("1m30s")), ShouldBeNil)
			So(d, ShouldEqual, DurationNS(90*time.Second))
		})

		Convey("UnmarshalText 空文本按 0 处理", func() {
			var d DurationNS
			So(d.UnmarshalText([]byte("")), ShouldBeNil)
			So(d, ShouldEqual, 0)
		})

		Convey("UnmarshalText 非法格式返回错误", func() {
			var d DurationNS
			So(d.UnmarshalText([]byte("abc")), ShouldNotBeNil)
		})
	})
}

// TestDuration_Database 验证 Duration 的数据库读写对称（Valuer/Scanner）。
func TestDuration_Database(t *testing.T) {
	Convey("Test Duration database value/scan", t, func() {
		Convey("Value 输出整数纳秒", func() {
			v, err := DurationNS(5 * time.Second).Value()
			So(err, ShouldBeNil)
			So(v, ShouldEqual, int64(5*time.Second))
		})

		Convey("Scan 兼容 int64 纳秒", func() {
			var d DurationNS
			So(d.Scan(int64(5*time.Second)), ShouldBeNil)
			So(d, ShouldEqual, DurationNS(5*time.Second))
		})

		Convey("Scan 兼容文本时长", func() {
			var d DurationNS
			So(d.Scan("1m30s"), ShouldBeNil)
			So(d, ShouldEqual, DurationNS(90*time.Second))
		})

		Convey("Scan nil 按 0 处理", func() {
			var d DurationNS
			So(d.Scan(nil), ShouldBeNil)
			So(d, ShouldEqual, 0)
		})

		Convey("Scan 不支持的类型返回错误", func() {
			var d DurationNS
			So(d.Scan(123), ShouldNotBeNil)
		})
	})
}

// TestDuration_Defaults 验证 Duration 支持 default 标签（defaults.Set 优先调用 TextUnmarshaler）。
func TestDuration_Defaults(t *testing.T) {
	Convey("Test Duration defaults.Set", t, func() {
		Convey("default 标签解析字符串时长", func() {
			type cfg struct {
				Interval DurationNS `default:"1m"`
			}
			var c cfg
			So(defaults.Set(&c), ShouldBeNil)
			So(c.Interval, ShouldEqual, DurationNS(time.Minute))
		})

		Convey("default 标签解析零值", func() {
			type cfg struct {
				Timeout DurationNS `default:"0"`
			}
			var c cfg
			So(defaults.Set(&c), ShouldBeNil)
			So(c.Timeout, ShouldEqual, 0)
		})
	})
}

// TestDurationMS_JSON 验证 DurationMS 的 JSON 编解码。
// 核心契约：序列化输出整数毫秒（与 gRPC 协议一致），反序列化同时接受可读时长字符串与整数毫秒。
func TestDurationMS_JSON(t *testing.T) {
	Convey("Test DurationMS JSON serialization", t, func() {
		Convey("Marshal 输出整数毫秒", func() {
			d := DurationMS(864000)
			data, err := json.Marshal(d)
			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, "864000")
		})

		Convey("Unmarshal 可读时长字符串", func() {
			var d DurationMS
			So(json.Unmarshal([]byte(`"5s"`), &d), ShouldBeNil)
			So(d, ShouldEqual, DurationMS(5000))
		})

		Convey("Unmarshal 复合时长字符串", func() {
			var d DurationMS
			So(json.Unmarshal([]byte(`"1m30s"`), &d), ShouldBeNil)
			So(d, ShouldEqual, DurationMS(90000))
		})

		Convey("Unmarshal 整数毫秒", func() {
			var d DurationMS
			So(json.Unmarshal([]byte("864000"), &d), ShouldBeNil)
			So(d, ShouldEqual, DurationMS(864000))
		})

		Convey("Unmarshal null 与空串按 0 处理", func() {
			var d DurationMS
			So(json.Unmarshal([]byte("null"), &d), ShouldBeNil)
			So(d, ShouldEqual, 0)
			So(json.Unmarshal([]byte(`""`), &d), ShouldBeNil)
			So(d, ShouldEqual, 0)
		})

		Convey("Unmarshal 非法格式返回错误", func() {
			var d DurationMS
			So(json.Unmarshal([]byte(`"invalid"`), &d), ShouldNotBeNil)
		})

		Convey("嵌套结构体 JSON 往返", func() {
			type wrapper struct {
				Timeout DurationMS `json:"timeout"`
			}
			raw := `{"timeout":"1m30s"}`
			var w wrapper
			So(json.Unmarshal([]byte(raw), &w), ShouldBeNil)
			So(w.Timeout, ShouldEqual, DurationMS(90000))

			out, err := json.Marshal(w)
			So(err, ShouldBeNil)
			// 序列化输出整数毫秒
			So(string(out), ShouldEqual, `{"timeout":90000}`)
		})
	})
}

// TestDurationMS_Text 验证 DurationMS 的文本编解码（cliinput 交互输入依赖此接口）。
func TestDurationMS_Text(t *testing.T) {
	Convey("Test DurationMS text marshal/unmarshal", t, func() {
		Convey("String 输出可读时长", func() {
			So(DurationMS(864000).String(), ShouldEqual, "14m24s")
			So(DurationMS(5000).String(), ShouldEqual, "5s")
			So(DurationMS(0).String(), ShouldEqual, "0s")
		})

		Convey("MarshalText 输出可读字符串", func() {
			data, err := DurationMS(5000).MarshalText()
			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, "5s")
		})

		Convey("UnmarshalText 解析可读时长字符串", func() {
			var d DurationMS
			So(d.UnmarshalText([]byte("1m30s")), ShouldBeNil)
			So(d, ShouldEqual, DurationMS(90000))
		})

		Convey("UnmarshalText 解析整数毫秒", func() {
			var d DurationMS
			So(d.UnmarshalText([]byte("864000")), ShouldBeNil)
			So(d, ShouldEqual, DurationMS(864000))
		})

		Convey("UnmarshalText 空文本按 0 处理", func() {
			var d DurationMS
			So(d.UnmarshalText([]byte("")), ShouldBeNil)
			So(d, ShouldEqual, 0)
		})

		Convey("UnmarshalText 非法格式返回错误", func() {
			var d DurationMS
			So(d.UnmarshalText([]byte("abc")), ShouldNotBeNil)
		})
	})
}

// TestDurationMS_Database 验证 DurationMS 的数据库读写对称（Valuer/Scanner）。
func TestDurationMS_Database(t *testing.T) {
	Convey("Test DurationMS database value/scan", t, func() {
		Convey("Value 输出整数毫秒", func() {
			v, err := DurationMS(864000).Value()
			So(err, ShouldBeNil)
			So(v, ShouldEqual, int64(864000))
		})

		Convey("Scan 兼容 int64 毫秒", func() {
			var d DurationMS
			So(d.Scan(int64(864000)), ShouldBeNil)
			So(d, ShouldEqual, DurationMS(864000))
		})

		Convey("Scan 兼容文本时长", func() {
			var d DurationMS
			So(d.Scan("1m30s"), ShouldBeNil)
			So(d, ShouldEqual, DurationMS(90000))
		})

		Convey("Scan nil 按 0 处理", func() {
			var d DurationMS
			So(d.Scan(nil), ShouldBeNil)
			So(d, ShouldEqual, 0)
		})
	})
}
