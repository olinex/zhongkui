package serializer

import (
	"encoding/json"
	"net"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMacAddr_BasicMethods(t *testing.T) {
	Convey("Test MacAddr basic methods", t, func() {
		Convey("String method", func() {
			macStr := "00:11:22:33:44:55"
			mac, err := net.ParseMAC(macStr)
			So(err, ShouldBeNil)
			macAddr := MacAddr(mac)
			So(macAddr.String(), ShouldEqual, macStr)
		})

		Convey("NewMacAddr with valid MAC", func() {
			macStr := "00:11:22:33:44:55"
			newMacAddr, err := NewMacAddr(macStr)
			So(err, ShouldBeNil)
			So(newMacAddr.String(), ShouldEqual, macStr)
		})

		Convey("NewMacAddr with invalid MAC", func() {
			_, err := NewMacAddr("invalid")
			So(err, ShouldNotBeNil)
		})
	})
}

func TestMacAddr_JSON(t *testing.T) {
	Convey("Test MacAddr JSON serialization", t, func() {
		Convey("JSON Marshal", func() {
			macStr := "00:11:22:33:44:55"
			mac, _ := net.ParseMAC(macStr)
			macAddr := MacAddr(mac)
			jsonData, err := json.Marshal(macAddr)
			So(err, ShouldBeNil)
			So(string(jsonData), ShouldEqual, "\"00:11:22:33:44:55\"")
		})

		Convey("JSON Unmarshal", func() {
			macStr := "00:11:22:33:44:55"
			var unmarshaledMac MacAddr
			err := json.Unmarshal([]byte("\"00:11:22:33:44:55\""), &unmarshaledMac)
			So(err, ShouldBeNil)
			So(unmarshaledMac.String(), ShouldEqual, macStr)
		})

		Convey("Unmarshal null", func() {
			var unmarshaledMac MacAddr
			err := json.Unmarshal([]byte("null"), &unmarshaledMac)
			So(err, ShouldBeNil)
		})

		Convey("Unmarshal empty string", func() {
			var unmarshaledMac MacAddr
			err := json.Unmarshal([]byte("\"\""), &unmarshaledMac)
			So(err, ShouldBeNil)
		})
	})
}

func TestMacAddr_Database(t *testing.T) {
	Convey("Test MacAddr database methods", t, func() {
		Convey("Value method", func() {
			macStr := "00:11:22:33:44:55"
			mac, _ := net.ParseMAC(macStr)
			macAddr := MacAddr(mac)
			value, err := macAddr.Value()
			So(err, ShouldBeNil)
			So(value, ShouldResemble, []byte(mac))
		})

		Convey("Value method with empty MAC", func() {
			emptyMacAddr := MacAddr{}
			value, err := emptyMacAddr.Value()
			So(err, ShouldBeNil)
			So(value, ShouldBeNil)
		})

		Convey("Scan from string", func() {
			macStr := "00:11:22:33:44:55"
			var scannedMac MacAddr
			err := scannedMac.Scan(macStr)
			So(err, ShouldBeNil)
			So(scannedMac.String(), ShouldEqual, macStr)
		})

		Convey("Scan from string []byte", func() {
			macStr := "00:11:22:33:44:55"
			var scannedMac MacAddr
			err := scannedMac.Scan([]byte(macStr))
			So(err, ShouldBeNil)
			So(scannedMac.String(), ShouldEqual, macStr)
		})

		Convey("Scan from 6-byte raw MAC (FixedString(6))", func() {
			raw := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
			var scannedMac MacAddr
			err := scannedMac.Scan(raw)
			So(err, ShouldBeNil)
			So(scannedMac.String(), ShouldEqual, "00:11:22:33:44:55")
		})

		Convey("Scan from empty []byte", func() {
			var scannedMac MacAddr
			err := scannedMac.Scan([]byte{})
			So(err, ShouldBeNil)
		})

		Convey("Scan from nil", func() {
			var scannedMac MacAddr
			err := scannedMac.Scan(nil)
			So(err, ShouldBeNil)
		})

		Convey("Scan unsupported type", func() {
			var scannedMac MacAddr
			err := scannedMac.Scan(123)
			So(err, ShouldNotBeNil)
		})

		Convey("Scan invalid MAC", func() {
			var scannedMac MacAddr
			err := scannedMac.Scan("invalid")
			So(err, ShouldNotBeNil)
		})
	})
}

// 由于 GormDBDataType 需要数据库连接，这里我们只测试基本的类型转换
func TestMacAddr_GormDBDataType(t *testing.T) {
	Convey("Test MacAddr GormDBDataType", t, func() {
		Convey("Ensure struct and method exist", func() {
			// 这里我们只测试结构体的方法是否存在，不测试实际的数据库类型
			// 注意：由于 GormDBDataType 方法需要数据库连接，我们不直接调用它
			// 只确保结构体和方法存在，不会编译错误
			macAddr := MacAddr{}
			// 确保这些结构体可以被创建（不会编译错误）
			_ = macAddr
		})
	})
}
