package network

import (
	"encoding/json"
	"net"
	"testing"

	"github.com/gopacket/gopacket/layers"
	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// TestIPv6HeaderTLVOption_Basic 测试IPv6HeaderTLVOption类型的基本功能
func TestIPv6HeaderTLVOption_Basic(t *testing.T) {
	Convey("Test IPv6HeaderTLVOption basic functionality", t, func() {
		Convey("Should create IPv6HeaderTLVOption with valid fields", func() {
			option := IPv6HeaderTLVOption{
				OptionType:       5, // 路由告警（Router Alert）选项
				OptionLength:     4,
				ActualLength:     8,
				OptionData:       []byte{0x00, 0x00, 0x00, 0x01},
				OptionAlignmentX: 2,
				OptionAlignmentY: 0,
			}

			So(option.OptionType, ShouldEqual, 5)
			So(option.OptionLength, ShouldEqual, 4)
			So(option.ActualLength, ShouldEqual, 8)
			So(option.OptionData, ShouldResemble, []byte{0x00, 0x00, 0x00, 0x01})
			So(option.OptionAlignmentX, ShouldEqual, 2)
			So(option.OptionAlignmentY, ShouldEqual, 0)
		})

		Convey("Should support JSON serialization and deserialization", func() {
			original := IPv6HeaderTLVOption{
				OptionType:       194, // 超大载荷（Jumbo Payload）选项
				OptionLength:     4,
				ActualLength:     8,
				OptionData:       []byte{0x00, 0x01, 0x02, 0x03},
				OptionAlignmentX: 4,
				OptionAlignmentY: 0,
			}

			jsonData, err := json.Marshal(original)
			So(err, ShouldBeNil)
			So(jsonData, ShouldNotBeNil)

			var decoded IPv6HeaderTLVOption
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)

			So(decoded.OptionType, ShouldEqual, original.OptionType)
			So(decoded.OptionLength, ShouldEqual, original.OptionLength)
			So(decoded.ActualLength, ShouldEqual, original.ActualLength)
			So(decoded.OptionData, ShouldResemble, original.OptionData)
			So(decoded.OptionAlignmentX, ShouldEqual, original.OptionAlignmentX)
			So(decoded.OptionAlignmentY, ShouldEqual, original.OptionAlignmentY)
		})
	})
}

// TestIPv6HeaderTLVOptions_Database 测试IPv6HeaderTLVOptions类型的数据库序列化功能
func TestIPv6HeaderTLVOptions_Database(t *testing.T) {
	Convey("Test IPv6HeaderTLVOptions database serialization", t, func() {
		Convey("Should serialize IPv6HeaderTLVOptions to JSON for database storage", func() {
			options := IPv6HeaderTLVOptions{
				{
					OptionType:   5, // 路由告警（Router Alert）
					OptionLength: 4,
					ActualLength: 8,
					OptionData:   []byte{0x00, 0x00, 0x00, 0x01},
				},
				{
					OptionType:   1, // PadN 填充
					OptionLength: 3,
					ActualLength: 4,
					OptionData:   []byte{0x00, 0x00, 0x00},
				},
			}

			value, err := options.Value()
			So(err, ShouldBeNil)
			So(value, ShouldNotBeNil)

			var decoded IPv6HeaderTLVOptions
			err = decoded.Scan(value)
			So(err, ShouldBeNil)
			So(len(options), ShouldEqual, 2)
			So(decoded[0].OptionType, ShouldEqual, 5)
			So(decoded[1].OptionType, ShouldEqual, 1)
		})

		Convey("Should handle nil value in Scan method", func() {
			var options IPv6HeaderTLVOptions
			err := options.Scan(nil)
			So(err, ShouldBeNil)
			So(options, ShouldBeNil)
		})

		Convey("Should handle string value in Scan method", func() {
			jsonStr := `[{"optionType":5,"optionLength":4,"actualLength":8,"optionData":"AAEAAQ=="}]`
			var options IPv6HeaderTLVOptions

			err := options.Scan(jsonStr)
			So(err, ShouldBeNil)
			So(len(options), ShouldEqual, 1)
			So(options[0].OptionType, ShouldEqual, 5)
			So(options[0].OptionLength, ShouldEqual, 4)
			So(options[0].ActualLength, ShouldEqual, 8)
		})

		Convey("Should handle []byte value in Scan method", func() {
			jsonData := []byte(`[{"optionType":194,"optionLength":4,"actualLength":8,"optionData":"AAECAw=="}]`)
			var options IPv6HeaderTLVOptions

			err := options.Scan(jsonData)
			So(err, ShouldBeNil)
			So(len(options), ShouldEqual, 1)
			So(options[0].OptionType, ShouldEqual, 194) // 超大载荷（Jumbo Payload）
		})

		Convey("Should return error for unsupported type in Scan method", func() {
			var options IPv6HeaderTLVOptions
			err := options.Scan(123) // 整数类型不支持
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "except []byte")
		})

		Convey("Should handle empty string in Scan method", func() {
			var options IPv6HeaderTLVOptions
			err := options.Scan("")
			So(err, ShouldBeNil)
			So(options, ShouldBeNil)
		})

		Convey("Should handle empty bytes in Scan method", func() {
			var options IPv6HeaderTLVOptions
			err := options.Scan([]byte{})
			So(err, ShouldBeNil)
			So(options, ShouldBeNil)
		})
	})
}

// TestIPv6HopByHop_Basic 测试IPv6HopByHop结构体的基本功能
func TestIPv6HopByHop_Basic(t *testing.T) {
	Convey("Test IPv6HopByHop basic functionality", t, func() {
		Convey("Should create IPv6HopByHop with valid fields", func() {
			options := IPv6HeaderTLVOptions{
				{
					OptionType:   5, // 路由告警（Router Alert）
					OptionLength: 4,
					ActualLength: 8,
					OptionData:   []byte{0x00, 0x00, 0x00, 0x01},
				},
			}

			hbh := IPv6HopByHop{
				NextHeader:   layers.IPProtocolTCP,
				HeaderLength: 1,
				ActualLength: 16,
				Options:      options,
			}

			So(hbh.NextHeader, ShouldEqual, layers.IPProtocolTCP)
			So(hbh.HeaderLength, ShouldEqual, 1)
			So(hbh.ActualLength, ShouldEqual, 16)
			So(len(hbh.Options), ShouldEqual, 1)
			So(hbh.Options[0].OptionType, ShouldEqual, 5)
		})

		Convey("Should support JSON serialization and deserialization", func() {
			options := IPv6HeaderTLVOptions{
				{
					OptionType:       5,
					OptionLength:     4,
					ActualLength:     8,
					OptionData:       []byte{0x00, 0x00, 0x00, 0x01},
					OptionAlignmentX: 2,
					OptionAlignmentY: 0,
				},
			}

			original := IPv6HopByHop{
				NextHeader:   layers.IPProtocolUDP,
				HeaderLength: 1,
				ActualLength: 16,
				Options:      options,
			}

			jsonData, err := json.Marshal(original)
			So(err, ShouldBeNil)
			So(jsonData, ShouldNotBeNil)

			var decoded IPv6HopByHop
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)

			So(decoded.NextHeader, ShouldEqual, original.NextHeader)
			So(decoded.HeaderLength, ShouldEqual, original.HeaderLength)
			So(decoded.ActualLength, ShouldEqual, original.ActualLength)
			So(len(decoded.Options), ShouldEqual, len(original.Options))
			So(decoded.Options[0].OptionType, ShouldEqual, original.Options[0].OptionType)
		})

		Convey("Should handle database serialization with Value method", func() {
			hbh := IPv6HopByHop{
				NextHeader:   layers.IPProtocolICMPv6,
				HeaderLength: 0,
				ActualLength: 8,
				Options:      nil,
			}

			value, err := hbh.Value()
			So(err, ShouldBeNil)
			So(value, ShouldNotBeNil)

			jsonBytes, ok := value.([]byte)
			So(ok, ShouldBeTrue)
			// 检查JSON是否有效
			var decoded IPv6HopByHop
			err = json.Unmarshal(jsonBytes, &decoded)
			So(err, ShouldBeNil)
			// 检查反序列化后的结构是否与原始结构一致
			So(decoded.NextHeader, ShouldEqual, hbh.NextHeader)
			So(decoded.HeaderLength, ShouldEqual, hbh.HeaderLength)
			So(decoded.ActualLength, ShouldEqual, hbh.ActualLength)
		})

		Convey("Should handle database deserialization with Scan method", func() {
			jsonData := `{"nextHeader":6,"headerLength":1,"actualLength":16,"options":[]}`
			var hbh IPv6HopByHop

			err := hbh.Scan(jsonData)
			So(err, ShouldBeNil)
			So(hbh.NextHeader, ShouldEqual, layers.IPProtocolTCP)
			So(hbh.HeaderLength, ShouldEqual, 1)
			So(hbh.ActualLength, ShouldEqual, 16)
			So(len(hbh.Options), ShouldEqual, 0)
		})

		Convey("Should return error for invalid type in Scan method", func() {
			var hbh IPv6HopByHop
			err := hbh.Scan(123)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "except []byte")
		})

		Convey("Should handle nil pointer with Value method", func() {
			var hbh *IPv6HopByHop // nil 指针
			value, err := hbh.Value()
			So(err, ShouldBeNil)
			// nil 指针应为 SQL NULL 返回 nil driver.Value
			So(value, ShouldBeNil)
		})

		Convey("Should handle empty string in Scan method", func() {
			var hbh IPv6HopByHop
			err := hbh.Scan("")
			So(err, ShouldBeNil)
			// 空值扫描后结构体应为零值
			So(hbh.NextHeader, ShouldEqual, layers.IPProtocol(0))
			So(hbh.HeaderLength, ShouldEqual, 0)
			So(hbh.ActualLength, ShouldEqual, 0)
			So(hbh.Options, ShouldBeNil)
		})

		Convey("Should handle empty bytes in Scan method", func() {
			var hbh IPv6HopByHop
			err := hbh.Scan([]byte{})
			So(err, ShouldBeNil)
			// 空值扫描后结构体应为零值
			So(hbh.NextHeader, ShouldEqual, layers.IPProtocol(0))
			So(hbh.HeaderLength, ShouldEqual, 0)
			So(hbh.ActualLength, ShouldEqual, 0)
			So(hbh.Options, ShouldBeNil)
		})
	})
}

// TestIPv6Destination_Basic 测试IPv6Destination结构体的基本功能
func TestIPv6Destination_Basic(t *testing.T) {
	Convey("Test IPv6Destination basic functionality", t, func() {
		Convey("Should create IPv6Destination with valid fields", func() {
			dest := IPv6Destination{
				NextHeader:   layers.IPProtocolTCP,
				HeaderLength: 1,
				ActualLength: 16,
				Options:      nil,
			}

			So(dest.NextHeader, ShouldEqual, layers.IPProtocolTCP)
			So(dest.HeaderLength, ShouldEqual, 1)
			So(dest.ActualLength, ShouldEqual, 16)
			So(dest.Options, ShouldBeNil)
		})

		Convey("Should support JSON serialization", func() {
			dest := IPv6Destination{
				NextHeader:   layers.IPProtocolUDP,
				HeaderLength: 0,
				ActualLength: 8,
				Options:      IPv6HeaderTLVOptions{},
			}

			jsonData, err := json.Marshal(dest)
			So(err, ShouldBeNil)

			var decoded IPv6Destination
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)
			So(decoded.NextHeader, ShouldEqual, layers.IPProtocolUDP)
		})
	})
}

// TestIPv6Fragment_Basic 测试IPv6Fragment结构体的基本功能
func TestIPv6Fragment_Basic(t *testing.T) {
	Convey("Test IPv6Fragment basic functionality", t, func() {
		Convey("Should create IPv6Fragment with valid fields", func() {
			fragment := IPv6Fragment{
				NextHeader:     layers.IPProtocolTCP,
				Reserved1:      0,
				FragmentOffset: 100,
				Reserved2:      0,
				MoreFragments:  true,
				Identification: 123456789,
			}

			So(fragment.NextHeader, ShouldEqual, layers.IPProtocolTCP)
			So(fragment.Reserved1, ShouldEqual, 0)
			So(fragment.FragmentOffset, ShouldEqual, 100)
			So(fragment.Reserved2, ShouldEqual, 0)
			So(fragment.MoreFragments, ShouldBeTrue)
			So(fragment.Identification, ShouldEqual, 123456789)
		})

		Convey("Should support JSON serialization", func() {
			fragment := IPv6Fragment{
				NextHeader:     layers.IPProtocolUDP,
				Reserved1:      0,
				FragmentOffset: 0,
				Reserved2:      0,
				MoreFragments:  false,
				Identification: 987654321,
			}

			jsonData, err := json.Marshal(fragment)
			So(err, ShouldBeNil)

			var decoded IPv6Fragment
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)
			So(decoded.NextHeader, ShouldEqual, layers.IPProtocolUDP)
			So(decoded.MoreFragments, ShouldBeFalse)
			So(decoded.Identification, ShouldEqual, 987654321)
		})
	})
}

// TestIPv6Routing_Basic 测试IPv6Routing结构体的基本功能
func TestIPv6Routing_Basic(t *testing.T) {
	Convey("Test IPv6Routing basic functionality", t, func() {
		Convey("Should create IPv6Routing with valid fields", func() {
			srcIPs := []net.IP{
				net.ParseIP("2001:db8::1"),
				net.ParseIP("2001:db8::2"),
			}

			routing := IPv6Routing{
				NextHeader:       layers.IPProtocolTCP,
				HeaderLength:     2,
				ActualLength:     24,
				RoutingType:      0,
				Reserved:         []byte{0x00, 0x00},
				SourceRoutingIPs: srcIPs,
			}

			So(routing.NextHeader, ShouldEqual, layers.IPProtocolTCP)
			So(routing.HeaderLength, ShouldEqual, 2)
			So(routing.ActualLength, ShouldEqual, 24)
			So(routing.RoutingType, ShouldEqual, 0)
			So(routing.Reserved, ShouldResemble, []byte{0x00, 0x00})
			So(len(routing.SourceRoutingIPs), ShouldEqual, 2)
			So(routing.SourceRoutingIPs[0].String(), ShouldEqual, "2001:db8::1")
			So(routing.SourceRoutingIPs[1].String(), ShouldEqual, "2001:db8::2")
		})

		Convey("Should support JSON serialization", func() {
			srcIPs := []net.IP{
				net.ParseIP("2001:db8::a"),
			}

			routing := IPv6Routing{
				NextHeader:       layers.IPProtocolUDP,
				HeaderLength:     1,
				ActualLength:     16,
				RoutingType:      2,
				Reserved:         []byte{0x00},
				SourceRoutingIPs: srcIPs,
			}

			jsonData, err := json.Marshal(routing)
			So(err, ShouldBeNil)

			var decoded IPv6Routing
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)
			So(decoded.NextHeader, ShouldEqual, layers.IPProtocolUDP)
			So(decoded.RoutingType, ShouldEqual, 2)
			So(len(decoded.SourceRoutingIPs), ShouldEqual, 1)
		})
	})
}

// TestIPv6Base_Basic 测试IPv6Base结构体的基本功能
func TestIPv6Base_Basic(t *testing.T) {
	Convey("Test IPv6Base basic functionality", t, func() {
		Convey("Should create IPv6Base with valid IPv6 addresses", func() {
			srcIP := net.ParseIP("2001:db8::1")
			dstIP := net.ParseIP("2001:db8::2")

			base := IPv6Base{
				IPv6Version:      6,
				IPv6TrafficClass: 0,
				IPv6FlowLabel:    12345,
				IPv6Length:       1500,
				IPv6NextHeader:   layers.IPProtocolTCP,
				IPv6HopLimit:     64,
				IPv6Src:          serializer.IPv6Addr(srcIP),
				IPv6Dst:          serializer.IPv6Addr(dstIP),
				IPv6HopByHop:     nil,
			}

			So(base.IPv6Version, ShouldEqual, 6)
			So(base.IPv6TrafficClass, ShouldEqual, 0)
			So(base.IPv6FlowLabel, ShouldEqual, 12345)
			So(base.IPv6Length, ShouldEqual, 1500)
			So(base.IPv6NextHeader, ShouldEqual, layers.IPProtocolTCP)
			So(base.IPv6HopLimit, ShouldEqual, 64)
			So(net.IP(base.IPv6Src).String(), ShouldEqual, "2001:db8::1")
			So(net.IP(base.IPv6Dst).String(), ShouldEqual, "2001:db8::2")
			So(base.IPv6HopByHop, ShouldBeNil)
		})

		Convey("Should support JSON serialization and deserialization", func() {
			srcIP := net.ParseIP("2001:db8::100")
			dstIP := net.ParseIP("2001:db8::200")

			original := IPv6Base{
				IPv6Version:      6,
				IPv6TrafficClass: 0x20,
				IPv6FlowLabel:    54321,
				IPv6Length:       576,
				IPv6NextHeader:   layers.IPProtocolUDP,
				IPv6HopLimit:     128,
				IPv6Src:          serializer.IPv6Addr(srcIP),
				IPv6Dst:          serializer.IPv6Addr(dstIP),
				IPv6HopByHop:     nil,
			}

			jsonData, err := json.Marshal(original)
			So(err, ShouldBeNil)
			So(jsonData, ShouldNotBeNil)

			var decoded IPv6Base
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)

			So(decoded.IPv6Version, ShouldEqual, original.IPv6Version)
			So(decoded.IPv6TrafficClass, ShouldEqual, original.IPv6TrafficClass)
			So(decoded.IPv6FlowLabel, ShouldEqual, original.IPv6FlowLabel)
			So(decoded.IPv6Length, ShouldEqual, original.IPv6Length)
			So(decoded.IPv6NextHeader, ShouldEqual, original.IPv6NextHeader)
			So(decoded.IPv6HopLimit, ShouldEqual, original.IPv6HopLimit)
			So(net.IP(decoded.IPv6Src).String(), ShouldEqual, net.IP(original.IPv6Src).String())
			So(net.IP(decoded.IPv6Dst).String(), ShouldEqual, net.IP(original.IPv6Dst).String())
		})
	})
}

// TestIPv6Headers_Basic 测试IPv6Headers结构体的基本功能
func TestIPv6Headers_Basic(t *testing.T) {
	Convey("Test IPv6Headers basic functionality", t, func() {
		Convey("Should create IPv6Headers with valid fields", func() {
			srcIP := net.ParseIP("2001:db8::1")
			dstIP := net.ParseIP("2001:db8::2")

			headers := IPv6Headers{
				IPv6Base: IPv6Base{
					IPv6Version:      6,
					IPv6TrafficClass: 0,
					IPv6FlowLabel:    12345,
					IPv6Length:       1500,
					IPv6NextHeader:   layers.IPProtocolTCP,
					IPv6HopLimit:     64,
					IPv6Src:          serializer.IPv6Addr(srcIP),
					IPv6Dst:          serializer.IPv6Addr(dstIP),
					IPv6HopByHop:     nil,
				},
				IPv6LastHeader: layers.IPProtocolTCP,
			}

			So(headers.IPv6Version, ShouldEqual, 6)
			So(headers.IPv6TrafficClass, ShouldEqual, 0)
			So(headers.IPv6FlowLabel, ShouldEqual, 12345)
			So(headers.IPv6Length, ShouldEqual, 1500)
			So(headers.IPv6NextHeader, ShouldEqual, layers.IPProtocolTCP)
			So(headers.IPv6HopLimit, ShouldEqual, 64)
			So(headers.SrcAddr().String(), ShouldEqual, "2001:db8::1")
			So(headers.DstAddr().String(), ShouldEqual, "2001:db8::2")
			So(headers.IPv6LastHeader, ShouldEqual, layers.IPProtocolTCP)
		})

		Convey("Should correctly return source address with SrcAddr method", func() {
			srcIP := net.ParseIP("2001:db8::abc")
			headers := IPv6Headers{
				IPv6Base: IPv6Base{
					IPv6Src: serializer.IPv6Addr(srcIP),
				},
			}

			result := headers.SrcAddr()
			So(result.String(), ShouldEqual, "2001:db8::abc")
			So(result, ShouldHaveSameTypeAs, net.IP{})
		})

		Convey("Should correctly return destination address with DstAddr method", func() {
			dstIP := net.ParseIP("2001:db8::def")
			headers := IPv6Headers{
				IPv6Base: IPv6Base{
					IPv6Dst: serializer.IPv6Addr(dstIP),
				},
			}

			result := headers.DstAddr()
			So(result.String(), ShouldEqual, "2001:db8::def")
			So(result, ShouldHaveSameTypeAs, net.IP{})
		})
	})
}

// TestPacketIPv6_Basic 测试PacketIPv6结构体的基本功能
func TestPacketIPv6_Basic(t *testing.T) {
	Convey("Test PacketIPv6 basic functionality", t, func() {
		Convey("Should create PacketIPv6 with valid fields", func() {
			srcIP := net.ParseIP("2001:db8::1")
			dstIP := net.ParseIP("2001:db8::2")

			packet := PacketIPv6{
				PacketEthernet: PacketEthernet{
					EthernetHeaders: EthernetHeaders{
						EthSrcMAC: serializer.MacAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
						EthDstMAC: serializer.MacAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
						EthType:   layers.EthernetTypeIPv6,
					},
				},
				IPv6Headers: IPv6Headers{
					IPv6Base: IPv6Base{
						IPv6Version:      6,
						IPv6TrafficClass: 0,
						IPv6FlowLabel:    12345,
						IPv6Length:       1500,
						IPv6NextHeader:   layers.IPProtocolTCP,
						IPv6HopLimit:     64,
						IPv6Src:          serializer.IPv6Addr(srcIP),
						IPv6Dst:          serializer.IPv6Addr(dstIP),
						IPv6HopByHop:     nil,
					},
					IPv6LastHeader: layers.IPProtocolTCP,
				},
			}

			So(packet.EthSrcMAC.String(), ShouldEqual, "00:11:22:33:44:55")
			So(packet.EthDstMAC.String(), ShouldEqual, "aa:bb:cc:dd:ee:ff")
			So(packet.EthType, ShouldEqual, layers.EthernetTypeIPv6)
			So(packet.IPv6Version, ShouldEqual, 6)
			So(packet.SrcAddr().String(), ShouldEqual, "2001:db8::1")
			So(packet.DstAddr().String(), ShouldEqual, "2001:db8::2")
			So(packet.IPv6LastHeader, ShouldEqual, layers.IPProtocolTCP)
		})

		Convey("Should support JSON serialization", func() {
			srcIP := net.ParseIP("2001:db8::a")
			dstIP := net.ParseIP("2001:db8::b")

			packet := PacketIPv6{
				PacketEthernet: PacketEthernet{
					EthernetHeaders: EthernetHeaders{
						EthSrcMAC: serializer.MacAddr{0x01, 0x02, 0x03, 0x04, 0x05, 0x06},
						EthDstMAC: serializer.MacAddr{0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c},
						EthType:   layers.EthernetTypeIPv6,
					},
				},
				IPv6Headers: IPv6Headers{
					IPv6Base: IPv6Base{
						IPv6Version:      6,
						IPv6TrafficClass: 0x10,
						IPv6FlowLabel:    9999,
						IPv6Length:       500,
						IPv6NextHeader:   layers.IPProtocolUDP,
						IPv6HopLimit:     32,
						IPv6Src:          serializer.IPv6Addr(srcIP),
						IPv6Dst:          serializer.IPv6Addr(dstIP),
					},
					IPv6LastHeader: layers.IPProtocolUDP,
				},
			}

			jsonData, err := json.Marshal(packet)
			So(err, ShouldBeNil)

			var decoded PacketIPv6
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)
			So(decoded.EthType, ShouldEqual, layers.EthernetTypeIPv6)
			So(decoded.IPv6Version, ShouldEqual, 6)
			So(decoded.IPv6LastHeader, ShouldEqual, layers.IPProtocolUDP)
		})
	})
}
