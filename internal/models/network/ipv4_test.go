package network

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket/layers"
	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// TestIPv4Options_Basic 测试IPv4Options类型的基本功能
// 包括数据库序列化、反序列化和JSON处理
func TestIPv4Options_Basic(t *testing.T) {
	Convey("Test IPv4Options basic functionality", t, func() {
		Convey("Should create empty IPv4Options", func() {
			var options IPv4Options
			So(options, ShouldBeNil)
			So(len(options), ShouldEqual, 0)
		})

		Convey("Should serialize IPv4Options to JSON for database storage", func() {
			options := IPv4Options{
				{
					OptionType:   1, // IPv4 NOP option
					OptionLength: 1,
					OptionData:   []byte{},
				},
				{
					OptionType:   0, // IPv4 EOL option
					OptionLength: 1,
					OptionData:   []byte{},
				},
			}

			value, err := options.Value()
			So(err, ShouldBeNil)
			So(value, ShouldNotBeNil)

			// 验证序列化结果可以反序列化
			var decoded IPv4Options
			err = decoded.Scan(value)
			So(err, ShouldBeNil)
			So(len(decoded), ShouldEqual, 2)
			So(decoded[0].OptionType, ShouldEqual, uint8(1)) // NOP
			So(decoded[1].OptionType, ShouldEqual, uint8(0)) // EOL
		})

		Convey("Should handle nil value in Scan method", func() {
			var options IPv4Options
			err := options.Scan(nil)
			So(err, ShouldBeNil)
			So(options, ShouldBeNil)
		})

		Convey("Should handle string value in Scan method", func() {
			var options IPv4Options
			jsonStr := `[{"OptionType":1,"OptionLength":1,"OptionData":""}]`
			err := options.Scan(jsonStr)
			So(err, ShouldBeNil)
			So(len(options), ShouldEqual, 1)
			So(options[0].OptionType, ShouldEqual, uint8(1)) // NOP
		})

		Convey("Should return error for unsupported type in Scan method", func() {
			var options IPv4Options
			err := options.Scan(123) // 整数类型不支持
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "except []byte")
		})

		Convey("Should support JSON serialization and deserialization", func() {
			original := IPv4Options{
				{
					OptionType:   68, // IPv4 Timestamp option
					OptionLength: 4,
					OptionData:   []byte{0x01, 0x02, 0x03, 0x04},
				},
			}

			jsonData, err := json.Marshal(original)
			So(err, ShouldBeNil)
			So(jsonData, ShouldNotBeNil)

			var decoded IPv4Options
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)

			So(len(decoded), ShouldEqual, 1)
			So(decoded[0].OptionType, ShouldEqual, uint8(68)) // Timestamp
			So(decoded[0].OptionLength, ShouldEqual, 4)
			So(decoded[0].OptionData, ShouldResemble, []byte{0x01, 0x02, 0x03, 0x04})
		})

		Convey("Should handle nil slice with Value method", func() {
			var options IPv4Options // nil slice
			value, err := options.Value()
			So(err, ShouldBeNil)
			// nil slice should return nil driver.Value for SQL NULL
			So(value, ShouldBeNil)
		})

		Convey("Should handle empty string in Scan method", func() {
			var options IPv4Options
			err := options.Scan("")
			So(err, ShouldBeNil)
			So(options, ShouldBeNil)
		})

		Convey("Should handle empty bytes in Scan method", func() {
			var options IPv4Options
			err := options.Scan([]byte{})
			So(err, ShouldBeNil)
			So(options, ShouldBeNil)
		})
	})
}

// TestIPv4Headers_Basic 测试IPv4Headers结构体的基本功能
// 包括字段验证、JSON序列化和数据库映射
func TestIPv4Headers_Basic(t *testing.T) {
	Convey("Test IPv4Headers basic functionality", t, func() {
		Convey("Should create IPv4Headers with valid IP addresses", func() {
			srcIP := net.ParseIP("192.168.1.100")
			dstIP := net.ParseIP("10.0.0.1")

			headers := IPv4Headers{
				IPv4Version:    4,
				IPv4IHL:        5,
				IPv4TOS:        0,
				IPv4Length:     1500,
				IPv4Id:         12345,
				IPv4Flags:      layers.IPv4DontFragment,
				IPv4FragOffset: 0,
				IPv4TTL:        64,
				IPv4IPProtocol: layers.IPProtocolTCP,
				IPv4Checksum:   0xABCD,
				IPv4Src:        serializer.IPv4Addr(srcIP),
				IPv4Dst:        serializer.IPv4Addr(dstIP),
				IPv4Options:    IPv4Options{},
				IPv4Padding:    "",
			}

			So(headers.IPv4Version, ShouldEqual, 4)
			So(headers.IPv4IHL, ShouldEqual, 5)
			So(headers.IPv4Length, ShouldEqual, 1500)
			So(headers.IPv4Id, ShouldEqual, 12345)
			So(headers.IPv4Flags, ShouldEqual, layers.IPv4DontFragment)
			So(headers.IPv4TTL, ShouldEqual, 64)
			So(headers.IPv4IPProtocol, ShouldEqual, layers.IPProtocolTCP)
			So(headers.IPv4Checksum, ShouldEqual, 0xABCD)
			So(headers.SrcAddr().String(), ShouldEqual, "192.168.1.100")
			So(headers.DstAddr().String(), ShouldEqual, "10.0.0.1")
		})

		Convey("Should handle different IP protocols", func() {
			headers := IPv4Headers{
				IPv4IPProtocol: layers.IPProtocolUDP,
			}
			So(headers.IPv4IPProtocol, ShouldEqual, layers.IPProtocolUDP)

			headers.IPv4IPProtocol = layers.IPProtocolICMPv4
			So(headers.IPv4IPProtocol, ShouldEqual, layers.IPProtocolICMPv4)

			headers.IPv4IPProtocol = layers.IPProtocolIPv6
			So(headers.IPv4IPProtocol, ShouldEqual, layers.IPProtocolIPv6)
		})

		Convey("Should handle different IPv4 flags", func() {
			headers := IPv4Headers{
				IPv4Flags: layers.IPv4MoreFragments,
			}
			So(headers.IPv4Flags, ShouldEqual, layers.IPv4MoreFragments)

			headers.IPv4Flags = layers.IPv4DontFragment | layers.IPv4MoreFragments
			So(headers.IPv4Flags, ShouldEqual, layers.IPv4DontFragment|layers.IPv4MoreFragments)
		})

		Convey("Should support JSON serialization and deserialization", func() {
			srcIP := net.ParseIP("192.168.1.100")
			dstIP := net.ParseIP("10.0.0.1")

			original := IPv4Headers{
				IPv4Version:    4,
				IPv4IHL:        5,
				IPv4TOS:        0,
				IPv4Length:     1500,
				IPv4Id:         12345,
				IPv4Flags:      layers.IPv4DontFragment,
				IPv4FragOffset: 0,
				IPv4TTL:        64,
				IPv4IPProtocol: layers.IPProtocolTCP,
				IPv4Checksum:   0xABCD,
				IPv4Src:        serializer.IPv4Addr(srcIP),
				IPv4Dst:        serializer.IPv4Addr(dstIP),
				IPv4Options:    IPv4Options{},
				IPv4Padding:    "",
			}

			// 序列化为JSON
			jsonData, err := json.Marshal(original)
			So(err, ShouldBeNil)
			So(jsonData, ShouldNotBeNil)

			// 反序列化
			var decoded IPv4Headers
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)

			// 验证字段一致性
			So(decoded.IPv4Version, ShouldEqual, original.IPv4Version)
			So(decoded.IPv4IHL, ShouldEqual, original.IPv4IHL)
			So(decoded.IPv4Length, ShouldEqual, original.IPv4Length)
			So(decoded.IPv4Id, ShouldEqual, original.IPv4Id)
			So(decoded.IPv4Flags, ShouldEqual, original.IPv4Flags)
			So(decoded.IPv4TTL, ShouldEqual, original.IPv4TTL)
			So(decoded.IPv4IPProtocol, ShouldEqual, original.IPv4IPProtocol)
			So(decoded.IPv4Checksum, ShouldEqual, original.IPv4Checksum)
			So(decoded.SrcAddr().String(), ShouldEqual, original.SrcAddr().String())
			So(decoded.DstAddr().String(), ShouldEqual, original.DstAddr().String())
		})

		Convey("Should handle IPv4 options in JSON", func() {
			srcIP := net.ParseIP("192.168.1.100")
			dstIP := net.ParseIP("10.0.0.1")

			headers := IPv4Headers{
				IPv4Version: 4,
				IPv4IHL:     5,
				IPv4Src:     serializer.IPv4Addr(srcIP),
				IPv4Dst:     serializer.IPv4Addr(dstIP),
				IPv4Options: IPv4Options{
					{
						OptionType:   1, // IPv4 NOP option
						OptionLength: 1,
						OptionData:   []byte{},
					},
				},
			}

			jsonData, err := json.Marshal(headers)
			So(err, ShouldBeNil)

			var decoded IPv4Headers
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)
			So(len(decoded.IPv4Options), ShouldEqual, 1)
			So(decoded.IPv4Options[0].OptionType, ShouldEqual, uint8(1)) // NOP
		})
	})
}

// TestNewIPv4Headers 测试NewIPv4Headers函数的正确性
// 验证从gopacket层到内部结构的转换逻辑
func TestNewIPv4Headers(t *testing.T) {
	Convey("Test NewIPv4Headers function", t, func() {
		Convey("Should correctly convert gopacket IPv4 layer", func() {
			srcIP := net.ParseIP("192.168.1.100")
			dstIP := net.ParseIP("10.0.0.1")

			ipv4Layer := &layers.IPv4{
				Version:    4,
				IHL:        5,
				TOS:        0,
				Length:     1500,
				Id:         12345,
				Flags:      layers.IPv4DontFragment,
				FragOffset: 0,
				TTL:        64,
				Protocol:   layers.IPProtocolTCP,
				Checksum:   0xABCD,
				SrcIP:      srcIP,
				DstIP:      dstIP,
				Options: []layers.IPv4Option{
					{
						OptionType:   1, // IPv4 NOP option
						OptionLength: 1,
						OptionData:   []byte{},
					},
				},
				Padding: []byte{0x00, 0x00},
			}

			headers := NewIPv4Headers(ipv4Layer)

			So(headers, ShouldNotBeNil)
			So(headers.IPv4Version, ShouldEqual, 4)
			So(headers.IPv4IHL, ShouldEqual, 5)
			So(headers.IPv4Length, ShouldEqual, 1500)
			So(headers.IPv4Id, ShouldEqual, 12345)
			So(headers.IPv4Flags, ShouldEqual, layers.IPv4DontFragment)
			So(headers.IPv4TTL, ShouldEqual, 64)
			So(headers.IPv4IPProtocol, ShouldEqual, layers.IPProtocolTCP)
			So(headers.IPv4Checksum, ShouldEqual, 0xABCD)
			So(headers.SrcAddr().String(), ShouldEqual, "192.168.1.100")
			So(headers.DstAddr().String(), ShouldEqual, "10.0.0.1")
			So(len(headers.IPv4Options), ShouldEqual, 1)
			So(headers.IPv4Options[0].OptionType, ShouldEqual, uint8(1)) // NOP
			So(headers.IPv4Padding, ShouldEqual, "\x00\x00")
		})

		Convey("Should handle IPv4 layer without options and padding", func() {
			srcIP := net.ParseIP("10.0.0.2")
			dstIP := net.ParseIP("192.168.1.200")

			ipv4Layer := &layers.IPv4{
				Version:    4,
				IHL:        5,
				TOS:        0x10,
				Length:     576,
				Id:         54321,
				Flags:      layers.IPv4MoreFragments,
				FragOffset: 100,
				TTL:        128,
				Protocol:   layers.IPProtocolUDP,
				Checksum:   0x1234,
				SrcIP:      srcIP,
				DstIP:      dstIP,
				Options:    nil,
				Padding:    nil,
			}

			headers := NewIPv4Headers(ipv4Layer)

			So(headers, ShouldNotBeNil)
			So(headers.IPv4Version, ShouldEqual, 4)
			So(headers.IPv4TOS, ShouldEqual, 0x10)
			So(headers.IPv4Length, ShouldEqual, 576)
			So(headers.IPv4Id, ShouldEqual, 54321)
			So(headers.IPv4Flags, ShouldEqual, layers.IPv4MoreFragments)
			So(headers.IPv4FragOffset, ShouldEqual, 100)
			So(headers.IPv4TTL, ShouldEqual, 128)
			So(headers.IPv4IPProtocol, ShouldEqual, layers.IPProtocolUDP)
			So(headers.IPv4Checksum, ShouldEqual, 0x1234)
			So(headers.SrcAddr().String(), ShouldEqual, "10.0.0.2")
			So(headers.DstAddr().String(), ShouldEqual, "192.168.1.200")
			So(headers.IPv4Options, ShouldBeNil)
			So(headers.IPv4Padding, ShouldBeEmpty)
		})

		Convey("Should handle IPv4 layer with multiple options", func() {
			srcIP := net.ParseIP("172.16.0.1")
			dstIP := net.ParseIP("172.16.0.2")

			ipv4Layer := &layers.IPv4{
				Version:  4,
				IHL:      6,
				Protocol: layers.IPProtocolICMPv4,
				SrcIP:    srcIP,
				DstIP:    dstIP,
				Options: []layers.IPv4Option{
					{
						OptionType:   1, // IPv4 NOP option
						OptionLength: 1,
						OptionData:   []byte{},
					},
					{
						OptionType:   0, // IPv4 EOL option
						OptionLength: 1,
						OptionData:   []byte{},
					},
					{
						OptionType:   68, // IPv4 Timestamp option
						OptionLength: 4,
						OptionData:   []byte{0x01, 0x02, 0x03, 0x04},
					},
				},
			}

			headers := NewIPv4Headers(ipv4Layer)

			So(headers, ShouldNotBeNil)
			So(headers.IPv4IHL, ShouldEqual, 6)
			So(len(headers.IPv4Options), ShouldEqual, 3)
			So(headers.IPv4Options[0].OptionType, ShouldEqual, uint8(1))  // NOP
			So(headers.IPv4Options[1].OptionType, ShouldEqual, uint8(0))  // EOL
			So(headers.IPv4Options[2].OptionType, ShouldEqual, uint8(68)) // Timestamp
			So(headers.IPv4Options[2].OptionData, ShouldResemble, []byte{0x01, 0x02, 0x03, 0x04})
		})
	})
}

// TestIPv4Headers_Methods 测试IPv4Headers结构体的方法
func TestIPv4Headers_Methods(t *testing.T) {
	Convey("Test IPv4Headers methods", t, func() {
		Convey("SrcAddr should return correct source IP address", func() {
			srcIP := net.ParseIP("192.168.1.100")
			dstIP := net.ParseIP("10.0.0.1")

			headers := IPv4Headers{
				IPv4Src: serializer.IPv4Addr(srcIP),
				IPv4Dst: serializer.IPv4Addr(dstIP),
			}

			result := headers.SrcAddr()
			So(result, ShouldNotBeNil)
			So(result.String(), ShouldEqual, "192.168.1.100")
			So(result, ShouldHaveSameTypeAs, net.IP{})
		})

		Convey("DstAddr should return correct destination IP address", func() {
			srcIP := net.ParseIP("192.168.1.100")
			dstIP := net.ParseIP("10.0.0.1")

			headers := IPv4Headers{
				IPv4Src: serializer.IPv4Addr(srcIP),
				IPv4Dst: serializer.IPv4Addr(dstIP),
			}

			result := headers.DstAddr()
			So(result, ShouldNotBeNil)
			So(result.String(), ShouldEqual, "10.0.0.1")
			So(result, ShouldHaveSameTypeAs, net.IP{})
		})

		Convey("SrcAddr and DstAddr should handle IPv4-mapped IPv6 addresses", func() {
			// IPv4-mapped IPv6地址：::ffff:192.168.1.100
			srcIP := net.ParseIP("::ffff:192.168.1.100")
			dstIP := net.ParseIP("::ffff:10.0.0.1")

			headers := IPv4Headers{
				IPv4Src: serializer.IPv4Addr(srcIP),
				IPv4Dst: serializer.IPv4Addr(dstIP),
			}

			srcResult := headers.SrcAddr()
			dstResult := headers.DstAddr()

			So(srcResult, ShouldNotBeNil)
			So(dstResult, ShouldNotBeNil)
			// 注意：IPv4-mapped IPv6地址在转换为字符串时会显示为IPv4格式
			So(srcResult.String(), ShouldEqual, "192.168.1.100")
			So(dstResult.String(), ShouldEqual, "10.0.0.1")
		})

		Convey("SrcAddr should return nil for empty IPv4Src", func() {
			headers := IPv4Headers{
				IPv4Src: nil,
			}

			result := headers.SrcAddr()
			So(result, ShouldBeNil)
		})

		Convey("DstAddr should return nil for empty IPv4Dst", func() {
			headers := IPv4Headers{
				IPv4Dst: nil,
			}

			result := headers.DstAddr()
			So(result, ShouldBeNil)
		})
	})
}

// TestPacketIPv4_Structure 测试PacketIPv4结构体的组合关系
func TestPacketIPv4_Structure(t *testing.T) {
	Convey("Test PacketIPv4 structure", t, func() {
		Convey("Should correctly embed PacketEthernet and IPv4Headers", func() {
			srcMAC, _ := net.ParseMAC("00:11:22:33:44:55")
			dstMAC, _ := net.ParseMAC("aa:bb:cc:dd:ee:ff")
			srcIP := net.ParseIP("192.168.1.100")
			dstIP := net.ParseIP("10.0.0.1")

			sessionID, _ := serializer.HexUUIDv4FromString("123e4567-e89b-12d3-a456-426614174000")
			packet := PacketIPv4{
				PacketEthernet: PacketEthernet{
					AsPacket: AsPacket{
						BaseMeta: BaseMeta{
							SessionID: sessionID,
							Namespace: "test-namespace",
							Capture:   "test-capture",
							Version:   "v1.0",
						},
						TimestampMeta: TimestampMeta{
							Timestamp: time.Now(),
						},
					},
					EthernetHeaders: EthernetHeaders{
						EthDstMAC: serializer.MacAddr(dstMAC),
						EthSrcMAC: serializer.MacAddr(srcMAC),
						EthType:   layers.EthernetTypeIPv4,
						EthLength: 1500,
					},
				},
				IPv4Headers: IPv4Headers{
					IPv4Version:    4,
					IPv4IHL:        5,
					IPv4IPProtocol: layers.IPProtocolTCP,
					IPv4TTL:        64,
					IPv4Src:        serializer.IPv4Addr(srcIP),
					IPv4Dst:        serializer.IPv4Addr(dstIP),
				},
			}

			So(packet, ShouldNotBeNil)
			So(packet.SessionID.String(), ShouldEqual, "123e4567-e89b-12d3-a456-426614174000")
			So(packet.Namespace, ShouldEqual, "test-namespace")
			So(packet.Capture, ShouldEqual, "test-capture")
			So(packet.Version, ShouldEqual, "v1.0")
			So(packet.EthSrcMAC.String(), ShouldEqual, "00:11:22:33:44:55")
			So(packet.EthDstMAC.String(), ShouldEqual, "aa:bb:cc:dd:ee:ff")
			So(packet.EthType, ShouldEqual, layers.EthernetTypeIPv4)
			So(packet.IPv4Version, ShouldEqual, 4)
			So(packet.SrcAddr().String(), ShouldEqual, "192.168.1.100")
			So(packet.DstAddr().String(), ShouldEqual, "10.0.0.1")
			So(packet.IPv4IPProtocol, ShouldEqual, layers.IPProtocolTCP)
			So(packet.IPv4TTL, ShouldEqual, 64)
		})

		Convey("Should inherit methods from embedded types", func() {
			srcIP := net.ParseIP("192.168.1.100")
			dstIP := net.ParseIP("10.0.0.1")

			packet := PacketIPv4{
				IPv4Headers: IPv4Headers{
					IPv4Src: serializer.IPv4Addr(srcIP),
					IPv4Dst: serializer.IPv4Addr(dstIP),
				},
			}

			// 验证可以从PacketIPv4直接调用IPv4Headers的方法
			srcAddr := packet.SrcAddr()
			dstAddr := packet.DstAddr()

			So(srcAddr.String(), ShouldEqual, "192.168.1.100")
			So(dstAddr.String(), ShouldEqual, "10.0.0.1")
		})
	})
}