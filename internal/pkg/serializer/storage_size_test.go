package serializer

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// TestBibyteSize_BasicMethods 测试 BibyteSize 的基本方法与单位换算。
func TestBibyteSize_BasicMethods(t *testing.T) {
	Convey("测试 BibyteSize 基本方法", t, func() {
		Convey("String 方法", func() {
			size := BibyteSize64(1048576) // 1 MiB
			So(size.String(), ShouldEqual, "1.0 MiB")
		})

		Convey("String 方法 - 0 字节", func() {
			size := BibyteSize64(0)
			So(size.String(), ShouldEqual, "0 B")
		})

		Convey("String 方法 - 大容量", func() {
			size := BibyteSize64(3221225472) // 3 GiB
			So(size.String(), ShouldEqual, "3.0 GiB")
		})

		Convey("KiB 方法", func() {
			size := BibyteSize64(2048)
			So(size.KiB(), ShouldEqual, 2.0)
		})

		Convey("KiB 方法 - 非整数倍", func() {
			size := BibyteSize64(1536)
			So(size.KiB(), ShouldEqual, 1.5)
		})

		Convey("MiB 方法", func() {
			size := BibyteSize64(2097152) // 2 MiB
			So(size.MiB(), ShouldEqual, 2.0)
		})

		Convey("MiB 方法 - 非整数倍", func() {
			size := BibyteSize64(3145728) // 3 MiB
			So(size.MiB(), ShouldEqual, 3.0)
		})

		Convey("GiB 方法", func() {
			size := BibyteSize64(3221225472) // 3 GiB
			So(size.GiB(), ShouldEqual, 3.0)
		})

		Convey("GiB 方法 - 非整数倍", func() {
			size := BibyteSize64(5368709120) // 5 GiB
			So(size.GiB(), ShouldEqual, 5.0)
		})

		Convey("单位换算一致性", func() {
			size := BibyteSize64(1073741824) // 1 GiB
			So(size.KiB(), ShouldEqual, 1048576.0)
			So(size.MiB(), ShouldEqual, 1024.0)
			So(size.GiB(), ShouldEqual, 1.0)
		})
	})
}

// TestBibyteSize_UnmarshalJSON 测试 BibyteSize 的 JSON 反序列化。
func TestBibyteSize_UnmarshalJSON(t *testing.T) {
	Convey("测试 BibyteSize JSON 反序列化", t, func() {
		Convey("解析有效的容量字符串", func() {
			var size BibyteSize64
			err := json.Unmarshal([]byte(`"10 MB"`), &size)
			So(err, ShouldBeNil)
			So(size, ShouldEqual, BibyteSize64(10000000))
		})

		Convey("解析 GiB 单位", func() {
			var size BibyteSize64
			err := json.Unmarshal([]byte(`"1.5 GiB"`), &size)
			So(err, ShouldBeNil)
			So(size, ShouldEqual, BibyteSize64(1610612736))
		})

		Convey("解析纯数字字符串", func() {
			var size BibyteSize64
			err := json.Unmarshal([]byte(`"1024"`), &size)
			So(err, ShouldBeNil)
			So(size, ShouldEqual, BibyteSize64(1024))
		})

		Convey("解析空字符串", func() {
			var size BibyteSize64
			err := json.Unmarshal([]byte(`""`), &size)
			So(err, ShouldBeNil)
			So(size, ShouldEqual, BibyteSize64(0))
		})

		Convey("解析无效格式", func() {
			var size BibyteSize64
			err := json.Unmarshal([]byte(`"invalid"`), &size)
			So(err, ShouldNotBeNil)
		})

		Convey("JSON 序列化与反序列化往返", func() {
			original := BibyteSize64(5242880) // 5 MiB
			jsonData, err := json.Marshal(original.String())
			So(err, ShouldBeNil)

			var restored BibyteSize64
			err = json.Unmarshal(jsonData, &restored)
			So(err, ShouldBeNil)
			So(restored, ShouldEqual, original)
		})
	})
}

// TestBibyteSize_UnmarshalText 测试 BibyteSize 的文本反序列化。
func TestBibyteSize_UnmarshalText(t *testing.T) {
	Convey("测试 BibyteSize Text 反序列化", t, func() {
		Convey("解析有效的容量字符串", func() {
			var size BibyteSize64
			err := size.UnmarshalText([]byte("10 MB"))
			So(err, ShouldBeNil)
			So(size, ShouldEqual, BibyteSize64(10000000))
		})

		Convey("解析空字符串", func() {
			var size BibyteSize64
			err := size.UnmarshalText([]byte(""))
			So(err, ShouldBeNil)
			So(size, ShouldEqual, BibyteSize64(0))
		})

		Convey("解析无效格式", func() {
			var size BibyteSize64
			err := size.UnmarshalText([]byte("invalid"))
			So(err, ShouldNotBeNil)
		})
	})
}

// TestBibyteSize_UnmarshalBinary 测试 BibyteSize 的二进制反序列化。
func TestBibyteSize_UnmarshalBinary(t *testing.T) {
	Convey("测试 BibyteSize Binary 反序列化", t, func() {
		Convey("解析有效的容量字符串", func() {
			var size BibyteSize64
			err := size.UnmarshalBinary([]byte("10 MB"))
			So(err, ShouldBeNil)
			So(size, ShouldEqual, BibyteSize64(10000000))
		})

		Convey("解析空字符串", func() {
			var size BibyteSize64
			err := size.UnmarshalBinary([]byte(""))
			So(err, ShouldBeNil)
			So(size, ShouldEqual, BibyteSize64(0))
		})

		Convey("解析无效格式", func() {
			var size BibyteSize64
			err := size.UnmarshalBinary([]byte("invalid"))
			So(err, ShouldNotBeNil)
		})
	})
}

// TestBibyteSize_StructSerialization 测试 BibyteSize 在结构体中的序列化与反序列化。
func TestBibyteSize_StructSerialization(t *testing.T) {
	Convey("测试 BibyteSize 在结构体中的序列化", t, func() {
		type Config struct {
			MaxSize BibyteSize64 `json:"max_size"`
		}

		Convey("结构体 JSON 反序列化", func() {
			jsonStr := `{"max_size": "2 GiB"}`
			var config Config
			err := json.Unmarshal([]byte(jsonStr), &config)
			So(err, ShouldBeNil)
			So(config.MaxSize, ShouldEqual, BibyteSize64(2147483648))
		})

		Convey("结构体 JSON 序列化与反序列化往返", func() {
			config := Config{MaxSize: BibyteSize64(524288000)}
			jsonData, err := json.Marshal(config)
			So(err, ShouldBeNil)

			var restored Config
			err = json.Unmarshal(jsonData, &restored)
			So(err, ShouldBeNil)
			So(restored.MaxSize, ShouldEqual, config.MaxSize)
		})
	})
}
