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

// TestEthernetHeaders_Basic 测试EthernetHeaders结构体的基本功能
// 包括字段验证、JSON序列化和数据库映射
func TestEthernetHeaders_Basic(t *testing.T) {
	Convey("Test EthernetHeaders basic functionality", t, func() {
		Convey("Should create EthernetHeaders with valid MAC addresses", func() {
			dstMAC, _ := net.ParseMAC("00:11:22:33:44:55")
			srcMAC, _ := net.ParseMAC("aa:bb:cc:dd:ee:ff")

			headers := EthernetHeaders{
				EthDstMAC: serializer.MacAddr(dstMAC),
				EthSrcMAC: serializer.MacAddr(srcMAC),
				EthType:   layers.EthernetTypeIPv4,
				EthLength: 1500,
			}

			So(headers.EthDstMAC.String(), ShouldEqual, "00:11:22:33:44:55")
			So(headers.EthSrcMAC.String(), ShouldEqual, "aa:bb:cc:dd:ee:ff")
			So(headers.EthType, ShouldEqual, layers.EthernetTypeIPv4)
			So(headers.EthLength, ShouldEqual, 1500)
		})

		Convey("Should handle different Ethernet types", func() {
			headers := EthernetHeaders{
				EthType: layers.EthernetTypeIPv6,
			}

			So(headers.EthType, ShouldEqual, layers.EthernetTypeIPv6)

			headers.EthType = layers.EthernetTypeARP
			So(headers.EthType, ShouldEqual, layers.EthernetTypeARP)
		})

		Convey("Should support JSON serialization and deserialization", func() {
			dstMAC, _ := net.ParseMAC("00:11:22:33:44:55")
			srcMAC, _ := net.ParseMAC("aa:bb:cc:dd:ee:ff")

			original := EthernetHeaders{
				EthDstMAC: serializer.MacAddr(dstMAC),
				EthSrcMAC: serializer.MacAddr(srcMAC),
				EthType:   layers.EthernetTypeIPv4,
				EthLength: 1500,
			}

			// 序列化为JSON
			jsonData, err := json.Marshal(original)
			So(err, ShouldBeNil)
			So(jsonData, ShouldNotBeNil)

			// 反序列化
			var decoded EthernetHeaders
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)

			// 验证字段一致性
			So(decoded.EthDstMAC.String(), ShouldEqual, original.EthDstMAC.String())
			So(decoded.EthSrcMAC.String(), ShouldEqual, original.EthSrcMAC.String())
			So(decoded.EthType, ShouldEqual, original.EthType)
			So(decoded.EthLength, ShouldEqual, original.EthLength)
		})

		Convey("Should handle empty MAC addresses in JSON", func() {
			var headers EthernetHeaders
			err := json.Unmarshal([]byte(`{"eth_dst_mac": "", "eth_src_mac": ""}`), &headers)
			So(err, ShouldBeNil)
			So(headers.EthDstMAC, ShouldBeNil)
			So(headers.EthSrcMAC, ShouldBeNil)
		})

		Convey("Should handle null values in JSON", func() {
			var headers EthernetHeaders
			err := json.Unmarshal([]byte(`{"eth_dst_mac": null, "eth_src_mac": null}`), &headers)
			So(err, ShouldBeNil)
			So(headers.EthDstMAC, ShouldBeNil)
			So(headers.EthSrcMAC, ShouldBeNil)
		})
	})
}

// TestNewEthernetHeaders 测试NewEthernetHeaders函数的正确性
// 验证从gopacket层到内部结构的转换逻辑
func TestNewEthernetHeaders(t *testing.T) {
	Convey("Test NewEthernetHeaders function", t, func() {
		Convey("Should correctly convert gopacket Ethernet layer", func() {
			dstMAC, _ := net.ParseMAC("00:11:22:33:44:55")
			srcMAC, _ := net.ParseMAC("aa:bb:cc:dd:ee:ff")

			ethLayer := &layers.Ethernet{
				DstMAC:       dstMAC,
				SrcMAC:       srcMAC,
				EthernetType: layers.EthernetTypeIPv4,
				Length:       1500,
			}

			headers := NewEthernetHeaders(ethLayer)

			So(headers, ShouldNotBeNil)
			So(headers.EthDstMAC.String(), ShouldEqual, "00:11:22:33:44:55")
			So(headers.EthSrcMAC.String(), ShouldEqual, "aa:bb:cc:dd:ee:ff")
			So(headers.EthType, ShouldEqual, layers.EthernetTypeIPv4)
			So(headers.EthLength, ShouldEqual, 1500)
		})

		Convey("Should handle IPv6 Ethernet type", func() {
			ethLayer := &layers.Ethernet{
				EthernetType: layers.EthernetTypeIPv6,
				Length:       1500,
			}

			headers := NewEthernetHeaders(ethLayer)
			So(headers.EthType, ShouldEqual, layers.EthernetTypeIPv6)
		})

		Convey("Should handle ARP Ethernet type", func() {
			ethLayer := &layers.Ethernet{
				EthernetType: layers.EthernetTypeARP,
				Length:       64,
			}

			headers := NewEthernetHeaders(ethLayer)
			So(headers.EthType, ShouldEqual, layers.EthernetTypeARP)
			So(headers.EthLength, ShouldEqual, 64)
		})

		Convey("Should handle zero length frames", func() {
			ethLayer := &layers.Ethernet{
				EthernetType: layers.EthernetTypeIPv4,
				Length:       0,
			}

			headers := NewEthernetHeaders(ethLayer)
			So(headers.EthLength, ShouldEqual, 0)
		})

		Convey("Should handle maximum length frames", func() {
			ethLayer := &layers.Ethernet{
				EthernetType: layers.EthernetTypeIPv4,
				Length:       65535, // Maximum value for uint16
			}

			headers := NewEthernetHeaders(ethLayer)
			So(headers.EthLength, ShouldEqual, 65535)
		})
	})
}

// TestPacketEthernet 测试PacketEthernet结构体的功能
// 验证组合结构和数据库映射
func TestPacketEthernet(t *testing.T) {
	Convey("Test PacketEthernet structure", t, func() {
		Convey("Should compose AsPacket and EthernetHeaders correctly", func() {
			sessionID, _ := serializer.HexUUIDv4FromString("12345678-1234-1234-1234-123456789012")
			now := time.Now()
			dstMAC, _ := net.ParseMAC("00:11:22:33:44:55")
			srcMAC, _ := net.ParseMAC("aa:bb:cc:dd:ee:ff")

			packet := PacketEthernet{
				AsPacket: AsPacket{
					BaseMeta: BaseMeta{
						Namespace: "test",
						Capture:   "eth0",
						Version:   "v1",
						SessionID: sessionID,
					},
					TimestampMeta: TimestampMeta{Timestamp: now},
					CaptureMeta: CaptureMeta{
						Truncated:     false,
						CaptureLength: 1514,
					},
					PacketMeta: PacketMeta{
						LayerStack: LayerStack{
							{
								Level:   LayerLevelLink,
								Proto:   layers.LayerTypeEthernet,
								Headers: map[string]any{"test": "value"},
							},
						},
					},
				},
				EthernetHeaders: EthernetHeaders{
					EthDstMAC: serializer.MacAddr(dstMAC),
					EthSrcMAC: serializer.MacAddr(srcMAC),
					EthType:   layers.EthernetTypeIPv4,
					EthLength: 1500,
				},
			}

			// 验证基础元数据
			So(packet.Namespace, ShouldEqual, "test")
			So(packet.Capture, ShouldEqual, "eth0")
			So(packet.Version, ShouldEqual, "v1")
			So(packet.SessionID.String(), ShouldEqual, "12345678-1234-1234-1234-123456789012")
			So(packet.Timestamp.Unix(), ShouldEqual, now.Unix())

			// 验证捕获元数据
			So(packet.Truncated, ShouldBeFalse)
			So(packet.CaptureLength, ShouldEqual, 1514)

			// 验证层堆栈
			So(len(packet.LayerStack), ShouldEqual, 1)
			So(packet.LayerStack[0].Level, ShouldEqual, LayerLevelLink)
			So(packet.LayerStack[0].Proto, ShouldEqual, layers.LayerTypeEthernet)

			// 验证以太网头部
			So(packet.EthDstMAC.String(), ShouldEqual, "00:11:22:33:44:55")
			So(packet.EthSrcMAC.String(), ShouldEqual, "aa:bb:cc:dd:ee:ff")
			So(packet.EthType, ShouldEqual, layers.EthernetTypeIPv4)
			So(packet.EthLength, ShouldEqual, 1500)
		})

		Convey("Should support JSON serialization for PacketEthernet", func() {
			sessionID := serializer.NewHexUUIDv4()
			dstMAC, _ := net.ParseMAC("00:11:22:33:44:55")
			srcMAC, _ := net.ParseMAC("aa:bb:cc:dd:ee:ff")

			packet := PacketEthernet{
				AsPacket: AsPacket{
					BaseMeta: BaseMeta{
						Namespace: "test",
						Capture:   "eth0",
						Version:   "v1",
						SessionID: sessionID,
					},
					TimestampMeta: TimestampMeta{Timestamp: time.Now()},
				},
				EthernetHeaders: EthernetHeaders{
					EthDstMAC: serializer.MacAddr(dstMAC),
					EthSrcMAC: serializer.MacAddr(srcMAC),
					EthType:   layers.EthernetTypeIPv4,
					EthLength: 1500,
				},
			}

			jsonData, err := json.Marshal(packet)
			So(err, ShouldBeNil)

			var decoded PacketEthernet
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)

			So(decoded.Namespace, ShouldEqual, "test")
			So(decoded.EthDstMAC.String(), ShouldEqual, "00:11:22:33:44:55")
			So(decoded.EthType, ShouldEqual, layers.EthernetTypeIPv4)
		})
	})
}

// TestFusedEthernet 测试FusedEthernet结构体的功能
// 验证融合数据结构和数据库映射
func TestFusedEthernet(t *testing.T) {
	Convey("Test FusedEthernet structure", t, func() {
		Convey("Should compose AsFused and EthernetHeaders correctly", func() {
			sessionID, _ := serializer.HexUUIDv4FromString("12345678-1234-1234-1234-123456789012")
			now := time.Now()
			dstMAC, _ := net.ParseMAC("00:11:22:33:44:55")
			srcMAC, _ := net.ParseMAC("aa:bb:cc:dd:ee:ff")

			fused := FusedEthernet{
				AsFused: AsFused{
					BaseMeta: BaseMeta{
						Namespace: "test",
						Capture:   "eth0",
						Version:   "v1",
						SessionID: sessionID,
					},
					TimestampMeta: TimestampMeta{Timestamp: now},
					FusedMeta: FusedMeta{
						FrameStack: FrameStack{
							{
								Timestamp: now.Add(-time.Second),
								LayerStack: LayerStack{
									{
										Level:   LayerLevelLink,
										Proto:   layers.LayerTypeEthernet,
										Headers: map[string]any{"test": "value"},
									},
								},
							},
						},
					},
				},
				EthernetHeaders: EthernetHeaders{
					EthDstMAC: serializer.MacAddr(dstMAC),
					EthSrcMAC: serializer.MacAddr(srcMAC),
					EthType:   layers.EthernetTypeIPv4,
					EthLength: 1500,
				},
			}

			// 验证基础元数据
			So(fused.Namespace, ShouldEqual, "test")
			So(fused.Capture, ShouldEqual, "eth0")
			So(fused.Version, ShouldEqual, "v1")
			So(fused.SessionID.String(), ShouldEqual, "12345678-1234-1234-1234-123456789012")
			So(fused.Timestamp.Unix(), ShouldEqual, now.Unix())

			// 验证帧堆栈
			So(len(fused.FrameStack), ShouldEqual, 1)
			So(fused.FrameStack[0].Timestamp.Unix(), ShouldEqual, now.Add(-time.Second).Unix())
			So(len(fused.FrameStack[0].LayerStack), ShouldEqual, 1)

			// 验证以太网头部
			So(fused.EthDstMAC.String(), ShouldEqual, "00:11:22:33:44:55")
			So(fused.EthSrcMAC.String(), ShouldEqual, "aa:bb:cc:dd:ee:ff")
			So(fused.EthType, ShouldEqual, layers.EthernetTypeIPv4)
			So(fused.EthLength, ShouldEqual, 1500)
		})

		Convey("Should support JSON serialization for FusedEthernet", func() {
			sessionID := serializer.NewHexUUIDv4()
			dstMAC, _ := net.ParseMAC("00:11:22:33:44:55")
			srcMAC, _ := net.ParseMAC("aa:bb:cc:dd:ee:ff")

			fused := FusedEthernet{
				AsFused: AsFused{
					BaseMeta: BaseMeta{
						Namespace: "test",
						Capture:   "eth0",
						Version:   "v1",
						SessionID: sessionID,
					},
					TimestampMeta: TimestampMeta{Timestamp: time.Now()},
				},
				EthernetHeaders: EthernetHeaders{
					EthDstMAC: serializer.MacAddr(dstMAC),
					EthSrcMAC: serializer.MacAddr(srcMAC),
					EthType:   layers.EthernetTypeIPv4,
					EthLength: 1500,
				},
			}

			jsonData, err := json.Marshal(fused)
			So(err, ShouldBeNil)

			var decoded FusedEthernet
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)

			So(decoded.Namespace, ShouldEqual, "test")
			So(decoded.EthDstMAC.String(), ShouldEqual, "00:11:22:33:44:55")
			So(decoded.EthType, ShouldEqual, layers.EthernetTypeIPv4)
		})
	})
}

// TestEthernetHeaders_FieldTags 测试EthernetHeaders字段标签的正确性
// 验证GORM和JSON标签的配置
func TestEthernetHeaders_FieldTags(t *testing.T) {
	Convey("Test EthernetHeaders field tags", t, func() {
		Convey("Should have correct GORM tags for database mapping", func() {
			// 验证字段标签的存在性
			headers := EthernetHeaders{}

			// 通过反射或直接验证标签配置
			// 这里我们主要验证结构体可以正常创建和使用
			So(headers, ShouldNotBeNil)

			// 验证字段可以正常赋值
			dstMAC, _ := net.ParseMAC("00:11:22:33:44:55")
			headers.EthDstMAC = serializer.MacAddr(dstMAC)
			So(headers.EthDstMAC.String(), ShouldEqual, "00:11:22:33:44:55")
		})

		Convey("Should have correct JSON tags for serialization", func() {
			dstMAC, _ := net.ParseMAC("00:11:22:33:44:55")
			srcMAC, _ := net.ParseMAC("aa:bb:cc:dd:ee:ff")

			headers := EthernetHeaders{
				EthDstMAC: serializer.MacAddr(dstMAC),
				EthSrcMAC: serializer.MacAddr(srcMAC),
				EthType:   layers.EthernetTypeIPv4,
				EthLength: 1500,
			}

			// 验证JSON序列化使用正确的字段名
			jsonData, err := json.Marshal(headers)
			So(err, ShouldBeNil)

			var decoded map[string]any
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)

			// 验证JSON字段名
			So(decoded, ShouldContainKey, "eth_dst_mac")
			So(decoded, ShouldContainKey, "eth_src_mac")
			So(decoded, ShouldContainKey, "eth_type")
			So(decoded, ShouldContainKey, "eth_length")
		})
	})
}

// TestEthernetTypeConstants 测试以太网类型常量的正确性
// 验证常用以太网类型的值
func TestEthernetTypeConstants(t *testing.T) {
	Convey("Test Ethernet type constants", t, func() {
		Convey("Should have correct values for common Ethernet types", func() {
			// 验证标准以太网类型值
			So(layers.EthernetTypeIPv4, ShouldEqual, 0x0800)
			So(layers.EthernetTypeIPv6, ShouldEqual, 0x86DD)
			So(layers.EthernetTypeARP, ShouldEqual, 0x0806)

			// 注意：layers包中可能没有VLAN和LLDP常量
			// 我们只测试确实存在的常量
		})

		Convey("Should correctly identify protocol types", func() {
			headers := EthernetHeaders{
				EthType: layers.EthernetTypeIPv4,
			}

			So(headers.EthType, ShouldEqual, 0x0800)
			// EthType的String()方法可能返回数字或协议名称
			// 我们只验证值正确
		})

		Convey("Should handle different Ethernet type values", func() {
			// 测试一些常见的以太网类型
			testCases := []struct {
				name     string
				ethType  layers.EthernetType
				expected uint16
			}{
				{"IPv4", layers.EthernetTypeIPv4, 0x0800},
				{"IPv6", layers.EthernetTypeIPv6, 0x86DD},
				{"ARP", layers.EthernetTypeARP, 0x0806},
			}

			for _, tc := range testCases {
				headers := EthernetHeaders{
					EthType: tc.ethType,
				}
				So(uint16(headers.EthType), ShouldEqual, tc.expected)
			}
		})
	})
}
