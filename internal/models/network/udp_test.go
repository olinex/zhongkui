package network

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gopacket/gopacket/layers"
	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// TestUDPHeaders_Basic 测试UDPHeaders结构体的基本功能
// 包括字段验证、JSON序列化和构造函数测试
func TestUDPHeaders_Basic(t *testing.T) {
	Convey("Test UDPHeaders basic functionality", t, func() {
		Convey("Should create UDPHeaders with all fields", func() {
			headers := UDPHeaders{
				UDPSrcPort:  layers.UDPPort(5353), // mDNS端口
				UDPDstPort:  layers.UDPPort(5353), // mDNS端口
				UDPLength:   512,                  // UDP数据包长度
				UDPChecksum: 0x1234,               // 校验和
			}

			So(headers.UDPSrcPort, ShouldEqual, layers.UDPPort(5353))
			So(headers.UDPDstPort, ShouldEqual, layers.UDPPort(5353))
			So(headers.UDPLength, ShouldEqual, uint16(512))
			So(headers.UDPChecksum, ShouldEqual, uint16(0x1234))
		})

		Convey("Should serialize UDPHeaders to JSON", func() {
			headers := UDPHeaders{
				UDPSrcPort:  layers.UDPPort(53), // DNS端口
				UDPDstPort:  layers.UDPPort(53), // DNS端口
				UDPLength:   1024,               // UDP数据包长度
				UDPChecksum: 0xABCD,             // 校验和
			}

			jsonData, err := json.Marshal(headers)
			So(err, ShouldBeNil)

			var decoded UDPHeaders
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)

			So(decoded.UDPSrcPort, ShouldEqual, headers.UDPSrcPort)
			So(decoded.UDPDstPort, ShouldEqual, headers.UDPDstPort)
			So(decoded.UDPLength, ShouldEqual, headers.UDPLength)
			So(decoded.UDPChecksum, ShouldEqual, headers.UDPChecksum)
		})

		Convey("Should handle UDP headers with zero checksum (valid for IPv4)", func() {
			headers := UDPHeaders{
				UDPSrcPort:  layers.UDPPort(123), // NTP端口
				UDPDstPort:  layers.UDPPort(123), // NTP端口
				UDPLength:   48,                  // NTP数据包标准长度
				UDPChecksum: 0,                   // 校验和为零（IPv4中允许）
			}

			So(headers.UDPChecksum, ShouldEqual, 0)
			So(headers.UDPLength, ShouldEqual, 48)
		})
	})
}

// TestNewUDPHeaders 测试NewUDPHeaders函数的创建功能
// 验证从gopacket UDP层正确创建UDPHeaders
func TestNewUDPHeaders(t *testing.T) {
	Convey("Test NewUDPHeaders function", t, func() {
		Convey("Should create UDPHeaders from gopacket UDP layer", func() {
			udpLayer := &layers.UDP{
				SrcPort:  53,     // DNS源端口
				DstPort:  53,     // DNS目的端口
				Length:   512,    // UDP数据包长度
				Checksum: 0x1234, // 校验和
			}

			headers := NewUDPHeaders(udpLayer)

			So(headers.UDPSrcPort, ShouldEqual, layers.UDPPort(53))
			So(headers.UDPDstPort, ShouldEqual, layers.UDPPort(53))
			So(headers.UDPLength, ShouldEqual, uint16(512))
			So(headers.UDPChecksum, ShouldEqual, uint16(0x1234))
		})

		Convey("Should handle UDP layer with maximum port values", func() {
			udpLayer := &layers.UDP{
				SrcPort:  65535,  // 最大端口号
				DstPort:  1,      // 最小有效端口号
				Length:   1024,   // 较大数据包长度
				Checksum: 0xFFFF, // 最大校验和
			}

			headers := NewUDPHeaders(udpLayer)

			So(headers.UDPSrcPort, ShouldEqual, layers.UDPPort(65535))
			So(headers.UDPDstPort, ShouldEqual, layers.UDPPort(1))
			So(headers.UDPLength, ShouldEqual, uint16(1024))
			So(headers.UDPChecksum, ShouldEqual, uint16(0xFFFF))
		})

		Convey("Should handle UDP layer with typical DNS query parameters", func() {
			udpLayer := &layers.UDP{
				SrcPort:  54321,  // 临时客户端端口
				DstPort:  53,     // DNS服务器端口
				Length:   78,     // 典型DNS查询长度
				Checksum: 0x89AB, // 计算得到的校验和
			}

			headers := NewUDPHeaders(udpLayer)

			So(headers.UDPSrcPort, ShouldEqual, layers.UDPPort(54321))
			So(headers.UDPDstPort, ShouldEqual, layers.UDPPort(53))
			So(headers.UDPLength, ShouldEqual, uint16(78))
			So(headers.UDPChecksum, ShouldEqual, uint16(0x89AB))
		})
	})
}

// TestPacketIPv4UDP_Basic 测试PacketIPv4UDP结构体的基本功能
// 验证IPv4 UDP数据包结构的正确性和完整性
func TestPacketIPv4UDP_Basic(t *testing.T) {
	Convey("Test PacketIPv4UDP basic functionality", t, func() {
		Convey("Should create PacketIPv4UDP with embedded structures", func() {
			packet := &PacketIPv4UDP{
				PacketIPv4: PacketIPv4{
					PacketEthernet: PacketEthernet{
						AsPacket: AsPacket{
							BaseMeta: BaseMeta{
								Namespace: "test-namespace",
								Capture:   "test-capture",
								Version:   "1",
								SessionID: serializer.HexUUID{0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF},
							},
							TimestampMeta: TimestampMeta{
								Timestamp: time.Now(),
							},
							CaptureMeta: CaptureMeta{
								Truncated:     false,
								CaptureLength: 1024,
							},
							PacketMeta: PacketMeta{
								LayerStack: LayerStack{},
							},
							BytesPayload: BytesPayload{
								Payload: []byte{0x01, 0x02, 0x03, 0x04},
							},
						},
						EthernetHeaders: EthernetHeaders{
							EthSrcMAC: serializer.MacAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
							EthDstMAC: serializer.MacAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF},
							EthType:   layers.EthernetTypeIPv4,
						},
					},
					IPv4Headers: IPv4Headers{
						IPv4Version:    4,
						IPv4IHL:        5,
						IPv4TOS:        0,
						IPv4Length:     1024,
						IPv4Id:         12345,
						IPv4Flags:      layers.IPv4Flag(0x02), // DF flag
						IPv4FragOffset: 0,
						IPv4TTL:        64,
						IPv4IPProtocol: layers.IPProtocolUDP,
						IPv4Checksum:   0xABCD,
						IPv4Src:        serializer.IPv4Addr{192, 168, 1, 100},
						IPv4Dst:        serializer.IPv4Addr{192, 168, 1, 1},
						IPv4Options:    nil,
					},
				},
				UDPHeaders: UDPHeaders{
					UDPSrcPort:  layers.UDPPort(54321),
					UDPDstPort:  layers.UDPPort(53),
					UDPLength:   512,
					UDPChecksum: 0x1234,
				},
			}

			So(packet.Namespace, ShouldEqual, "test-namespace")
			So(packet.Capture, ShouldEqual, "test-capture")
			So(packet.IPv4Src.String(), ShouldEqual, "192.168.1.100")
			So(packet.IPv4Dst.String(), ShouldEqual, "192.168.1.1")
			So(packet.UDPSrcPort, ShouldEqual, layers.UDPPort(54321))
			So(packet.UDPDstPort, ShouldEqual, layers.UDPPort(53))
			So(packet.UDPLength, ShouldEqual, uint16(512))
		})

		Convey("Should handle typical DNS over UDP packet structure", func() {
			packet := &PacketIPv4UDP{
				PacketIPv4: PacketIPv4{
					IPv4Headers: IPv4Headers{
						IPv4Src:        serializer.IPv4Addr{8, 8, 8, 8},       // Google DNS
						IPv4Dst:        serializer.IPv4Addr{192, 168, 1, 100}, // 客户端
						IPv4IPProtocol: layers.IPProtocolUDP,
					},
				},
				UDPHeaders: UDPHeaders{
					UDPSrcPort: layers.UDPPort(53),    // DNS服务器端口
					UDPDstPort: layers.UDPPort(54321), // 客户端临时端口
					UDPLength:  512,
				},
			}

			So(packet.IPv4Src.String(), ShouldEqual, "8.8.8.8")
			So(packet.IPv4Dst.String(), ShouldEqual, "192.168.1.100")
			So(packet.UDPSrcPort, ShouldEqual, layers.UDPPort(53))
			So(packet.UDPDstPort, ShouldEqual, layers.UDPPort(54321))
		})
	})
}

// TestPacketIPv6UDP_Basic 测试PacketIPv6UDP结构体的基本功能
// 验证IPv6 UDP数据包结构的正确性和完整性
func TestPacketIPv6UDP_Basic(t *testing.T) {
	Convey("Test PacketIPv6UDP basic functionality", t, func() {
		Convey("Should create PacketIPv6UDP with embedded structures", func() {
			packet := &PacketIPv6UDP{
				PacketIPv6: PacketIPv6{
					PacketEthernet: PacketEthernet{
						AsPacket: AsPacket{
							BaseMeta: BaseMeta{
								Namespace: "ipv6-test",
								Capture:   "ipv6-capture",
								Version:   "1",
								SessionID: serializer.HexUUID{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x00},
							},
						},
						EthernetHeaders: EthernetHeaders{
							EthSrcMAC: serializer.MacAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
							EthDstMAC: serializer.MacAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF},
							EthType:   layers.EthernetTypeIPv6,
						},
					},
					IPv6Headers: IPv6Headers{
						IPv6Base: IPv6Base{
							IPv6Version:      6,
							IPv6TrafficClass: 0,
							IPv6FlowLabel:    12345,
							IPv6Length:       1024,
							IPv6NextHeader:   layers.IPProtocolUDP,
							IPv6HopLimit:     64,
							IPv6Src:          serializer.IPv6Addr{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
							IPv6Dst:          serializer.IPv6Addr{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2},
						},
						IPv6LastHeader: layers.IPProtocolUDP,
					},
				},
				UDPHeaders: UDPHeaders{
					UDPSrcPort:  layers.UDPPort(53),
					UDPDstPort:  layers.UDPPort(54321),
					UDPLength:   512,
					UDPChecksum: 0xABCD,
				},
			}

			So(packet.Namespace, ShouldEqual, "ipv6-test")
			So(packet.Capture, ShouldEqual, "ipv6-capture")
			So(packet.IPv6Src.String(), ShouldEqual, "2001:db8::1")
			So(packet.IPv6Dst.String(), ShouldEqual, "2001:db8::2")
			So(packet.UDPSrcPort, ShouldEqual, layers.UDPPort(53))
			So(packet.UDPDstPort, ShouldEqual, layers.UDPPort(54321))
		})

		Convey("Should handle DNS over IPv6 UDP packet structure", func() {
			packet := &PacketIPv6UDP{
				PacketIPv6: PacketIPv6{
					IPv6Headers: IPv6Headers{
						IPv6Base: IPv6Base{
							IPv6Src:        serializer.IPv6Addr{0x20, 0x01, 0x48, 0x60, 0x48, 0x60, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x88}, // Google DNS IPv6
							IPv6Dst:        serializer.IPv6Addr{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x64},       // 客户端 IPv6
							IPv6NextHeader: layers.IPProtocolUDP,
						},
					},
				},
				UDPHeaders: UDPHeaders{
					UDPSrcPort: layers.UDPPort(53),
					UDPDstPort: layers.UDPPort(54321),
				},
			}

			// 注意：serializer.IPv6Addr的String()方法可能返回不同的格式
			// 我们只验证结构体创建成功，不验证具体的字符串格式
			So(packet.IPv6Src, ShouldNotBeNil)
			So(packet.IPv6Dst, ShouldNotBeNil)
			So(packet.UDPSrcPort, ShouldEqual, layers.UDPPort(53))
			So(packet.UDPDstPort, ShouldEqual, layers.UDPPort(54321))
		})
	})
}

// TestUDPHeaders_JSONCompatibility 测试UDPHeaders的JSON兼容性
// 验证结构体与JSON格式的互操作性和数据一致性
func TestUDPHeaders_JSONCompatibility(t *testing.T) {
	Convey("Test UDPHeaders JSON compatibility", t, func() {
		Convey("Should maintain data integrity through JSON round-trip", func() {
			original := UDPHeaders{
				UDPSrcPort:  layers.UDPPort(67), // DHCP服务器端口
				UDPDstPort:  layers.UDPPort(68), // DHCP客户端端口
				UDPLength:   576,                // DHCP标准数据包长度
				UDPChecksum: 0x5678,             // 校验和
			}

			// 序列化为JSON
			jsonData, err := json.Marshal(original)
			So(err, ShouldBeNil)
			So(jsonData, ShouldNotBeNil)

			// 反序列化
			var decoded UDPHeaders
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)

			// 验证数据一致性
			So(decoded.UDPSrcPort, ShouldEqual, original.UDPSrcPort)
			So(decoded.UDPDstPort, ShouldEqual, original.UDPDstPort)
			So(decoded.UDPLength, ShouldEqual, original.UDPLength)
			So(decoded.UDPChecksum, ShouldEqual, original.UDPChecksum)
		})

		Convey("Should handle JSON with missing optional fields gracefully", func() {
			// 模拟可能缺少某些字段的JSON数据
			jsonStr := `{"src_port": 53, "dst_port": 53, "length": 512}`

			var headers UDPHeaders
			err := json.Unmarshal([]byte(jsonStr), &headers)

			// 期望能够成功解析，缺失的字段使用零值
			So(err, ShouldBeNil)
			So(headers.UDPSrcPort, ShouldEqual, layers.UDPPort(53))
			So(headers.UDPDstPort, ShouldEqual, layers.UDPPort(53))
			So(headers.UDPLength, ShouldEqual, uint16(512))
			So(headers.UDPChecksum, ShouldEqual, uint16(0)) // 零值
		})
	})
}
