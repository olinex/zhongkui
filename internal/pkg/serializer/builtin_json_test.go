package serializer

import (
	"encoding/json"
	"net"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBuiltinNetIP_JSON(t *testing.T) {
	Convey("Test net.IP JSON serialization", t, func() {
		Convey("net.IP MarshalJSON", func() {
			ipStr := "192.168.1.1"
			ip := net.ParseIP(ipStr)
			So(ip, ShouldNotBeNil)

			jsonData, err := json.Marshal(ip)
			So(err, ShouldBeNil)
			t.Logf("net.IP (%s) serialized to: %s", ipStr, string(jsonData))
			So(string(jsonData), ShouldEqual, "\"192.168.1.1\"")
		})

		Convey("net.IP IPv6 MarshalJSON", func() {
			ipStr := "2001:db8::1"
			ip := net.ParseIP(ipStr)
			So(ip, ShouldNotBeNil)

			jsonData, err := json.Marshal(ip)
			So(err, ShouldBeNil)
			t.Logf("net.IP (%s) serialized to: %s", ipStr, string(jsonData))
			So(string(jsonData), ShouldEqual, "\"2001:db8::1\"")
		})

		Convey("net.IP UnmarshalJSON", func() {
			var ip net.IP
			err := json.Unmarshal([]byte("\"192.168.1.1\""), &ip)
			So(err, ShouldBeNil)
			t.Logf("net.IP deserialized from \"192.168.1.1\" to: %v", ip)
			So(ip.String(), ShouldEqual, "192.168.1.1")
		})

		Convey("net.IP in struct", func() {
			type TestStruct struct {
				IP net.IP `json:"ip"`
			}

			testData := TestStruct{
				IP: net.ParseIP("192.168.1.1"),
			}

			jsonData, err := json.Marshal(testData)
			So(err, ShouldBeNil)
			t.Logf("Struct with net.IP serialized to: %s", string(jsonData))
			So(string(jsonData), ShouldEqual, `{"ip":"192.168.1.1"}`)

			var decoded TestStruct
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)
			So(decoded.IP.String(), ShouldEqual, "192.168.1.1")
		})
	})
}

func TestBuiltinHardwareAddr_JSON(t *testing.T) {
	Convey("Test net.HardwareAddr JSON serialization", t, func() {
		Convey("net.HardwareAddr MarshalJSON", func() {
			macStr := "00:11:22:33:44:55"
			mac, err := net.ParseMAC(macStr)
			So(err, ShouldBeNil)

			jsonData, err := json.Marshal(mac)
			So(err, ShouldBeNil)
			t.Logf("net.HardwareAddr (%s) serialized to: %s", macStr, string(jsonData))
			So(string(jsonData), ShouldEqual, "\"ABEiM0RV\"")
		})

		Convey("net.HardwareAddr UnmarshalJSON from base64", func() {
			var mac net.HardwareAddr
			err := json.Unmarshal([]byte("\"ABEiM0RV\""), &mac)
			So(err, ShouldBeNil)
			t.Logf("net.HardwareAddr deserialized from \"ABEiM0RV\" to: %v", mac)
			So(mac.String(), ShouldEqual, "00:11:22:33:44:55")
		})

		Convey("net.HardwareAddr UnmarshalJSON from bytes", func() {
			var mac net.HardwareAddr
			err := json.Unmarshal([]byte("[0,17,34,51,68,85]"), &mac)
			So(err, ShouldBeNil)
			t.Logf("net.HardwareAddr deserialized from [0,17,34,51,68,85] to: %v", mac)
			So(mac.String(), ShouldEqual, "00:11:22:33:44:55")
		})

		Convey("net.HardwareAddr in struct", func() {
			type TestStruct struct {
				MAC net.HardwareAddr `json:"mac"`
			}

			mac, _ := net.ParseMAC("00:11:22:33:44:55")
			testData := TestStruct{
				MAC: mac,
			}

			jsonData, err := json.Marshal(testData)
			So(err, ShouldBeNil)
			t.Logf("Struct with net.HardwareAddr serialized to: %s", string(jsonData))
			So(string(jsonData), ShouldEqual, `{"mac":"ABEiM0RV"}`)

			var decoded TestStruct
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)
			So(decoded.MAC.String(), ShouldEqual, "00:11:22:33:44:55")
		})
	})
}

func TestComparison_BuiltinVsCustom(t *testing.T) {
	Convey("Test builtin vs custom types comparison", t, func() {
		Convey("net.IP vs IPAddr", func() {
			ipStr := "192.168.1.1"
			builtinIP := net.ParseIP(ipStr)
			customIP := IPAddr(builtinIP)

			builtinJSON, err := json.Marshal(builtinIP)
			So(err, ShouldBeNil)
			customJSON, err := json.Marshal(customIP)
			So(err, ShouldBeNil)

			t.Logf("net.IP JSON: %s", string(builtinJSON))
			t.Logf("IPAddr JSON: %s", string(customJSON))

			So(string(builtinJSON), ShouldEqual, "\"192.168.1.1\"")
			So(string(customJSON), ShouldEqual, "\"192.168.1.1\"")
		})

		Convey("net.HardwareAddr vs MacAddr", func() {
			macStr := "00:11:22:33:44:55"
			builtinMAC, _ := net.ParseMAC(macStr)
			customMAC := MacAddr(builtinMAC)

			builtinJSON, err := json.Marshal(builtinMAC)
			So(err, ShouldBeNil)
			customJSON, err := json.Marshal(customMAC)
			So(err, ShouldBeNil)

			t.Logf("net.HardwareAddr JSON: %s", string(builtinJSON))
			t.Logf("MacAddr JSON: %s", string(customJSON))

			So(string(builtinJSON), ShouldEqual, "\"ABEiM0RV\"")
			So(string(customJSON), ShouldEqual, "\"00:11:22:33:44:55\"")
		})
	})
}
