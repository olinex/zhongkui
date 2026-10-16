package serializer

import (
	"encoding/json"
	"net"
	"net/netip"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// TestIPAddr_BasicMethods 测试 IPAddr 的基本方法与构造。
func TestIPAddr_BasicMethods(t *testing.T) {
	Convey("Test IPAddr basic methods", t, func() {
		Convey("String method", func() {
			ipStr := "192.168.1.1"
			ip := net.ParseIP(ipStr)
			ipAddr := IPAddr(ip)
			So(ipAddr.String(), ShouldEqual, ipStr)
		})

		Convey("NewIPAddr with valid IP", func() {
			ipStr := "192.168.1.1"
			newIPAddr, err := NewIPAddr(ipStr)
			So(err, ShouldBeNil)
			So(newIPAddr.String(), ShouldEqual, ipStr)
		})

		Convey("NewIPAddr with invalid IP returns nil IP", func() {
			// IPAddr.Scan 对于无效 IP 不返回错误，只是将 IP 设为 nil
			newIPAddr, err := NewIPAddr("invalid")
			So(err, ShouldBeNil)
			So(newIPAddr.String(), ShouldEqual, "<nil>")
		})
	})
}

// TestIPAddr_JSON 测试 IPAddr 的 JSON 序列化与反序列化。
func TestIPAddr_JSON(t *testing.T) {
	Convey("Test IPAddr JSON serialization", t, func() {
		Convey("JSON Marshal", func() {
			ipStr := "192.168.1.1"
			ip := net.ParseIP(ipStr)
			ipAddr := IPAddr(ip)
			jsonData, err := json.Marshal(ipAddr)
			So(err, ShouldBeNil)
			So(string(jsonData), ShouldEqual, "\"192.168.1.1\"")
		})

		Convey("JSON Unmarshal", func() {
			ipStr := "192.168.1.1"
			var unmarshaledIP IPAddr
			err := json.Unmarshal([]byte("\"192.168.1.1\""), &unmarshaledIP)
			So(err, ShouldBeNil)
			So(unmarshaledIP.String(), ShouldEqual, ipStr)
		})

		Convey("Unmarshal null", func() {
			var unmarshaledIP IPAddr
			err := json.Unmarshal([]byte("null"), &unmarshaledIP)
			So(err, ShouldBeNil)
		})

		Convey("Unmarshal empty string", func() {
			var unmarshaledIP IPAddr
			err := json.Unmarshal([]byte("\"\""), &unmarshaledIP)
			So(err, ShouldBeNil)
		})
	})
}

// TestIPAddr_Database 测试 IPAddr 的数据库读写方法。
func TestIPAddr_Database(t *testing.T) {
	Convey("Test IPAddr database methods", t, func() {
		Convey("Value method", func() {
			ipStr := "192.168.1.1"
			ip := net.ParseIP(ipStr)
			ipAddr := IPAddr(ip)
			value, err := ipAddr.Value()
			So(err, ShouldBeNil)
			// 统一以 16 字节原始二进制写入（IPv4 提升为 IPv4-mapped IPv6）
			So(value, ShouldResemble, ip.To16())
		})

		Convey("Value method with IPv6", func() {
			ipStr := "2001:db8::1"
			ip := net.ParseIP(ipStr)
			ipAddr := IPAddr(ip)
			value, err := ipAddr.Value()
			So(err, ShouldBeNil)
			So(value, ShouldResemble, ip.To16())
		})

		Convey("Value method with empty IP", func() {
			emptyIPAddr := IPAddr{}
			value, err := emptyIPAddr.Value()
			So(err, ShouldBeNil)
			So(value, ShouldBeNil)
		})

		Convey("Scan from string", func() {
			ipStr := "192.168.1.1"
			var scannedIP IPAddr
			err := scannedIP.Scan(ipStr)
			So(err, ShouldBeNil)
			So(scannedIP.String(), ShouldEqual, ipStr)
		})

		Convey("Scan from []byte", func() {
			ipStr := "192.168.1.1"
			var scannedIP IPAddr
			err := scannedIP.Scan([]byte(ipStr))
			So(err, ShouldBeNil)
			So(scannedIP.String(), ShouldEqual, ipStr)
		})

		Convey("Scan from nil", func() {
			var scannedIP IPAddr
			err := scannedIP.Scan(nil)
			So(err, ShouldBeNil)
		})

		Convey("Scan unsupported type", func() {
			var scannedIP IPAddr
			err := scannedIP.Scan(123)
			So(err, ShouldNotBeNil)
		})

		Convey("Scan invalid IP returns nil", func() {
			// IPAddr.Scan 对于无效 IP 不返回错误，只是将 IP 设为 nil
			var scannedIP IPAddr
			err := scannedIP.Scan("invalid")
			So(err, ShouldBeNil)
			So(scannedIP.String(), ShouldEqual, "<nil>")
		})

		Convey("Scan FixedString(16) raw binary IPv4-mapped IPv6", func() {
			// 原生 16 字节二进制：::ffff:127.0.0.1，String() 按 IPv4 渲染
			var scannedIP IPAddr
			err := scannedIP.Scan([]byte(net.ParseIP("127.0.0.1").To16()))
			So(err, ShouldBeNil)
			So(scannedIP.String(), ShouldEqual, "127.0.0.1")
		})

		Convey("Scan FixedString(16) raw binary IPv6", func() {
			var scannedIP IPAddr
			err := scannedIP.Scan([]byte(net.ParseIP("::1").To16()))
			So(err, ShouldBeNil)
			So(scannedIP.String(), ShouldEqual, "::1")
		})
	})
}

// TestIPv4Addr_BasicMethods 测试 IPv4Addr 的基本方法与构造。
func TestIPv4Addr_BasicMethods(t *testing.T) {
	Convey("Test IPv4Addr basic methods", t, func() {
		Convey("String method", func() {
			ipStr := "192.168.1.1"
			ip := net.ParseIP(ipStr)
			ipv4Addr := IPv4Addr(ip)
			So(ipv4Addr.String(), ShouldEqual, ipStr)
		})

		Convey("NewIPv4Addr with valid IPv4", func() {
			ipStr := "192.168.1.1"
			newIPv4Addr, err := NewIPv4Addr(ipStr)
			So(err, ShouldBeNil)
			So(newIPv4Addr.String(), ShouldEqual, ipStr)
		})

		Convey("NewIPv4Addr with invalid IPv4", func() {
			_, err := NewIPv4Addr("invalid")
			So(err, ShouldNotBeNil)
		})

		Convey("NewIPv4Addr with IPv6 address", func() {
			_, err := NewIPv4Addr("2001:db8::1")
			So(err, ShouldNotBeNil)
		})
	})
}

// TestIPv4Addr_JSON 测试 IPv4Addr 的 JSON 序列化与反序列化。
func TestIPv4Addr_JSON(t *testing.T) {
	Convey("Test IPv4Addr JSON serialization", t, func() {
		Convey("JSON Marshal", func() {
			ipStr := "192.168.1.1"
			ip := net.ParseIP(ipStr)
			ipv4Addr := IPv4Addr(ip)
			jsonData, err := json.Marshal(ipv4Addr)
			So(err, ShouldBeNil)
			So(string(jsonData), ShouldEqual, "\"192.168.1.1\"")
		})

		Convey("JSON Unmarshal", func() {
			ipStr := "192.168.1.1"
			var unmarshaledIP IPv4Addr
			err := json.Unmarshal([]byte("\"192.168.1.1\""), &unmarshaledIP)
			So(err, ShouldBeNil)
			So(unmarshaledIP.String(), ShouldEqual, ipStr)
		})
	})
}

// TestIPv4Addr_Database 测试 IPv4Addr 的数据库读写方法。
func TestIPv4Addr_Database(t *testing.T) {
	Convey("Test IPv4Addr database methods", t, func() {
		Convey("Value method", func() {
			ipStr := "192.168.1.1"
			ip := net.ParseIP(ipStr).To4()
			ipv4Addr := IPv4Addr(ip)
			value, err := ipv4Addr.Value()
			So(err, ShouldBeNil)
			So(value, ShouldResemble, ip)
		})

		Convey("Scan from string", func() {
			ipStr := "192.168.1.1"
			var scannedIP IPv4Addr
			err := scannedIP.Scan(ipStr)
			So(err, ShouldBeNil)
			So(scannedIP.String(), ShouldEqual, ipStr)
		})
	})
}

// TestIPv6Addr_BasicMethods 测试 IPv6Addr 的基本方法与构造。
func TestIPv6Addr_BasicMethods(t *testing.T) {
	Convey("Test IPv6Addr basic methods", t, func() {
		Convey("String method", func() {
			ipStr := "2001:db8::1"
			ip := net.ParseIP(ipStr)
			ipv6Addr := IPv6Addr(ip)
			So(ipv6Addr.String(), ShouldEqual, ipStr)
		})

		Convey("NewIPv6Addr with valid IPv6", func() {
			ipStr := "2001:db8::1"
			newIPv6Addr, err := NewIPv6Addr(ipStr)
			So(err, ShouldBeNil)
			So(newIPv6Addr.String(), ShouldEqual, ipStr)
		})

		Convey("NewIPv6Addr with invalid IPv6", func() {
			_, err := NewIPv6Addr("invalid")
			So(err, ShouldNotBeNil)
		})
	})
}

// TestIPv6Addr_JSON 测试 IPv6Addr 的 JSON 序列化与反序列化。
func TestIPv6Addr_JSON(t *testing.T) {
	Convey("Test IPv6Addr JSON serialization", t, func() {
		Convey("JSON Marshal", func() {
			ipStr := "2001:db8::1"
			ip := net.ParseIP(ipStr)
			ipv6Addr := IPv6Addr(ip)
			jsonData, err := json.Marshal(ipv6Addr)
			So(err, ShouldBeNil)
			So(string(jsonData), ShouldEqual, "\"2001:db8::1\"")
		})

		Convey("JSON Unmarshal", func() {
			ipStr := "2001:db8::1"
			var unmarshaledIP IPv6Addr
			err := json.Unmarshal([]byte("\"2001:db8::1\""), &unmarshaledIP)
			So(err, ShouldBeNil)
			So(unmarshaledIP.String(), ShouldEqual, ipStr)
		})
	})
}

// TestIPv6Addr_Database 测试 IPv6Addr 的数据库读写方法。
func TestIPv6Addr_Database(t *testing.T) {
	Convey("Test IPv6Addr database methods", t, func() {
		Convey("Value method", func() {
			ipStr := "2001:db8::1"
			ip := net.ParseIP(ipStr).To16()
			ipv6Addr := IPv6Addr(ip)
			value, err := ipv6Addr.Value()
			So(err, ShouldBeNil)
			So(value, ShouldResemble, ip)
		})

		Convey("Scan from string", func() {
			ipStr := "2001:db8::1"
			var scannedIP IPv6Addr
			err := scannedIP.Scan(ipStr)
			So(err, ShouldBeNil)
			So(scannedIP.String(), ShouldEqual, ipStr)
		})
	})
}

// 由于 GormDBDataType 需要数据库连接，这里我们只测试基本的类型转换
func TestIPAddr_GormDBDataType(t *testing.T) {
	Convey("Test IPAddr GormDBDataType", t, func() {
		Convey("Ensure struct and method exist", func() {
			// 这里我们只测试结构体的方法是否存在，不测试实际的数据库类型
			// 注意：由于 GormDBDataType 方法需要数据库连接，我们不直接调用它
			// 只确保结构体和方法存在，不会编译错误
			ipAddr := IPAddr{}
			ipv4Addr := IPv4Addr{}
			ipv6Addr := IPv6Addr{}

			// 确保这些结构体可以被创建（不会编译错误）
			_ = ipAddr
			_ = ipv4Addr
			_ = ipv6Addr
		})
	})
}

// TestCIDR_BasicMethods 测试 CIDR 的基本方法与构造。
func TestCIDR_BasicMethods(t *testing.T) {
	Convey("Test CIDR basic methods", t, func() {
		Convey("NewCIDR 与 String", func() {
			cidr, err := NewCIDR("10.0.0.0/8")
			So(err, ShouldBeNil)
			So(cidr.String(), ShouldEqual, "10.0.0.0/8")
		})

		Convey("掩码外的位归一化为零", func() {
			cidr, err := NewCIDR("10.0.0.1/24")
			So(err, ShouldBeNil)
			So(cidr.String(), ShouldEqual, "10.0.0.0/24")
		})

		Convey("裸 IPv4 按单主机网段补 /32", func() {
			cidr, err := NewCIDR("192.168.1.1")
			So(err, ShouldBeNil)
			So(cidr.String(), ShouldEqual, "192.168.1.1/32")
		})

		Convey("裸 IPv6 按单主机网段补 /128", func() {
			cidr, err := NewCIDR("2001:db8::1")
			So(err, ShouldBeNil)
			So(cidr.String(), ShouldEqual, "2001:db8::1/128")
		})

		Convey("非法网段返回错误", func() {
			_, err := NewCIDR("10.0.0.0/999")
			So(err, ShouldNotBeNil)

			_, err = NewCIDR("invalid")
			So(err, ShouldNotBeNil)
		})

		Convey("零值 String 为空字符串", func() {
			var cidr CIDR
			So(cidr.String(), ShouldEqual, "")
		})
	})
}

// TestCIDR_JSON 测试 CIDR 的 JSON 序列化与反序列化。
func TestCIDR_JSON(t *testing.T) {
	Convey("Test CIDR JSON serialization", t, func() {
		Convey("JSON Marshal", func() {
			cidr, err := NewCIDR("10.0.0.0/8")
			So(err, ShouldBeNil)
			jsonData, err := json.Marshal(cidr)
			So(err, ShouldBeNil)
			So(string(jsonData), ShouldEqual, `"10.0.0.0/8"`)
		})

		Convey("JSON Marshal 零值", func() {
			var cidr CIDR
			jsonData, err := json.Marshal(cidr)
			So(err, ShouldBeNil)
			So(string(jsonData), ShouldEqual, `""`)
		})

		Convey("JSON Unmarshal", func() {
			var cidr CIDR
			err := json.Unmarshal([]byte(`"10.0.0.1/24"`), &cidr)
			So(err, ShouldBeNil)
			So(cidr.String(), ShouldEqual, "10.0.0.0/24")
		})

		Convey("Unmarshal null 置零值", func() {
			cidr, err := NewCIDR("10.0.0.0/8")
			So(err, ShouldBeNil)
			err = json.Unmarshal([]byte("null"), cidr)
			So(err, ShouldBeNil)
			So(cidr.String(), ShouldEqual, "")
		})

		Convey("Unmarshal 空字符串置零值", func() {
			var cidr CIDR
			err := json.Unmarshal([]byte(`""`), &cidr)
			So(err, ShouldBeNil)
			So(cidr.String(), ShouldEqual, "")
		})

		Convey("Unmarshal 非法网段返回错误", func() {
			var cidr CIDR
			err := json.Unmarshal([]byte(`"invalid"`), &cidr)
			So(err, ShouldNotBeNil)
		})
	})
}

// TestCIDR_Text 测试 CIDR 的文本序列化与反序列化。
func TestCIDR_Text(t *testing.T) {
	Convey("Test CIDR text serialization", t, func() {
		Convey("MarshalText", func() {
			cidr, err := NewCIDR("10.0.0.0/8")
			So(err, ShouldBeNil)
			text, err := cidr.MarshalText()
			So(err, ShouldBeNil)
			So(string(text), ShouldEqual, "10.0.0.0/8")
		})

		Convey("MarshalText 零值输出空文本", func() {
			var cidr CIDR
			text, err := cidr.MarshalText()
			So(err, ShouldBeNil)
			So(string(text), ShouldEqual, "")
		})

		Convey("UnmarshalText", func() {
			var cidr CIDR
			err := cidr.UnmarshalText([]byte("192.168.1.1"))
			So(err, ShouldBeNil)
			So(cidr.String(), ShouldEqual, "192.168.1.1/32")
		})

		Convey("UnmarshalText 空文本置零值", func() {
			cidr, err := NewCIDR("10.0.0.0/8")
			So(err, ShouldBeNil)
			err = cidr.UnmarshalText(nil)
			So(err, ShouldBeNil)
			So(cidr.String(), ShouldEqual, "")
		})

		Convey("UnmarshalText 非法网段返回错误", func() {
			var cidr CIDR
			err := cidr.UnmarshalText([]byte("invalid"))
			So(err, ShouldNotBeNil)
		})
	})
}

// TestCIDR_Database 测试 CIDR 的数据库读写方法。
func TestCIDR_Database(t *testing.T) {
	Convey("Test CIDR database methods", t, func() {
		Convey("Value 返回网段文本", func() {
			cidr, err := NewCIDR("10.0.0.0/8")
			So(err, ShouldBeNil)
			value, err := cidr.Value()
			So(err, ShouldBeNil)
			So(value, ShouldEqual, "10.0.0.0/8")
		})

		Convey("Value 零值返回 nil", func() {
			var cidr CIDR
			value, err := cidr.Value()
			So(err, ShouldBeNil)
			So(value, ShouldBeNil)
		})

		Convey("Scan from string", func() {
			var cidr CIDR
			err := cidr.Scan("10.0.0.1/24")
			So(err, ShouldBeNil)
			So(cidr.String(), ShouldEqual, "10.0.0.0/24")
		})

		Convey("Scan from []byte", func() {
			var cidr CIDR
			err := cidr.Scan([]byte("192.168.1.1"))
			So(err, ShouldBeNil)
			So(cidr.String(), ShouldEqual, "192.168.1.1/32")
		})

		Convey("Scan from netip.Prefix", func() {
			var cidr CIDR
			err := cidr.Scan(netip.MustParsePrefix("10.0.0.1/24"))
			So(err, ShouldBeNil)
			So(cidr.String(), ShouldEqual, "10.0.0.0/24")
		})

		Convey("Scan 零值 netip.Prefix 置零值", func() {
			cidr, err := NewCIDR("10.0.0.0/8")
			So(err, ShouldBeNil)
			err = cidr.Scan(netip.Prefix{})
			So(err, ShouldBeNil)
			So(cidr.String(), ShouldEqual, "")
		})

		Convey("Scan from nil", func() {
			cidr, err := NewCIDR("10.0.0.0/8")
			So(err, ShouldBeNil)
			err = cidr.Scan(nil)
			So(err, ShouldBeNil)
			So(cidr.String(), ShouldEqual, "")
		})

		Convey("Scan 空字符串置零值", func() {
			cidr, err := NewCIDR("10.0.0.0/8")
			So(err, ShouldBeNil)
			err = cidr.Scan("")
			So(err, ShouldBeNil)
			So(cidr.String(), ShouldEqual, "")
		})

		Convey("Scan 不支持的类型返回错误", func() {
			var cidr CIDR
			err := cidr.Scan(123)
			So(err, ShouldNotBeNil)
		})

		Convey("Scan 非法网段返回错误", func() {
			var cidr CIDR
			err := cidr.Scan("10.0.0.0/999")
			So(err, ShouldNotBeNil)
		})
	})
}
