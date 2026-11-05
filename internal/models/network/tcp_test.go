package network

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gopacket/gopacket/layers"
	. "github.com/smartystreets/goconvey/convey"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// TestTCPOption_Basic 测试TCPOption类型的基本功能
// 包括结构体字段验证和JSON序列化
func TestTCPOption_Basic(t *testing.T) {
	Convey("Test TCPOption basic functionality", t, func() {
		Convey("Should create TCPOption with valid fields", func() {
			option := TCPOption{
				Type:   layers.TCPOptionKindMSS, // MSS选项类型
				Length: 4,                       // 长度字段
				Data:   []byte{0x05, 0xB4},      // MSS值 1460
			}

			So(option.Type, ShouldEqual, layers.TCPOptionKindMSS)
			So(option.Length, ShouldEqual, 4)
			So(option.Data, ShouldResemble, []byte{0x05, 0xB4})
		})

		Convey("Should serialize TCPOption to JSON", func() {
			option := TCPOption{
				Type:   layers.TCPOptionKindTimestamps, // 时间戳选项
				Length: 10,
				Data:   []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
			}

			jsonData, err := json.Marshal(option)
			So(err, ShouldBeNil)

			var decoded TCPOption
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)

			So(decoded.Type, ShouldEqual, option.Type)
			So(decoded.Length, ShouldEqual, option.Length)
			So(decoded.Data, ShouldResemble, option.Data)
		})
	})
}

// TestTCPOptions_Basic 测试TCPOptions类型的基本功能
// 包括数据库序列化、反序列化和JSON处理
func TestTCPOptions_Basic(t *testing.T) {
	Convey("Test TCPOptions basic functionality", t, func() {
		Convey("Should create empty TCPOptions", func() {
			var options TCPOptions
			So(options, ShouldBeNil)
			So(len(options), ShouldEqual, 0)
		})

		Convey("Should create TCPOptions from gopacket options", func() {
			gopacketOptions := []layers.TCPOption{
				{
					OptionType:   layers.TCPOptionKindMSS,
					OptionLength: 4,
					OptionData:   []byte{0x05, 0xB4},
				},
				{
					OptionType:   layers.TCPOptionKindWindowScale,
					OptionLength: 3,
					OptionData:   []byte{0x07},
				},
			}

			options := NewTCPOptions(gopacketOptions)
			So(len(options), ShouldEqual, 2)
			So(options[0].Type, ShouldEqual, layers.TCPOptionKindMSS)
			So(options[0].Length, ShouldEqual, 4)
			So(options[0].Data, ShouldResemble, []byte{0x05, 0xB4})
			So(options[1].Type, ShouldEqual, layers.TCPOptionKindWindowScale)
			So(options[1].Length, ShouldEqual, 3)
			So(options[1].Data, ShouldResemble, []byte{0x07})
		})

		Convey("Should serialize TCPOptions to JSON for database storage", func() {
			options := TCPOptions{
				{
					Type:   layers.TCPOptionKindMSS,
					Length: 4,
					Data:   []byte{0x05, 0xB4},
				},
				{
					Type:   layers.TCPOptionKindNop,
					Length: 1,
					Data:   []byte{},
				},
			}

			value, err := options.Value()
			So(err, ShouldBeNil)
			So(value, ShouldNotBeNil)

			// 验证序列化结果可以反序列化
			var decoded TCPOptions
			err = decoded.Scan(value)
			So(err, ShouldBeNil)
			So(len(decoded), ShouldEqual, 2)
			So(decoded[0].Type, ShouldEqual, layers.TCPOptionKindMSS)
			So(decoded[1].Type, ShouldEqual, layers.TCPOptionKindNop)
		})

		Convey("Should handle nil value in Scan method", func() {
			var options TCPOptions
			err := options.Scan(nil)
			So(err, ShouldBeNil)
			So(options, ShouldBeNil)
		})

		Convey("Should handle string value in Scan method", func() {
			var options TCPOptions
			jsonStr := `[{"type":2,"length":4,"data":"BbQ="}]` // MSS选项的base64编码，0x05B4 = 1460
			err := options.Scan(jsonStr)
			So(err, ShouldBeNil)
			So(len(options), ShouldEqual, 1)
			So(options[0].Type, ShouldEqual, layers.TCPOptionKind(2)) // MSS 选项
			So(options[0].Length, ShouldEqual, 4)
			So(options[0].Data, ShouldResemble, []byte{0x05, 0xB4})
		})

		Convey("Should return error for unsupported type in Scan method", func() {
			var options TCPOptions
			err := options.Scan(123) // 整数类型不支持
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "Tcp_options scan failed")
		})

		Convey("Should support JSON serialization and deserialization", func() {
			original := TCPOptions{
				{
					Type:   layers.TCPOptionKindTimestamps,
					Length: 10,
					Data:   []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
				},
			}

			jsonData, err := json.Marshal(original)
			So(err, ShouldBeNil)
			So(jsonData, ShouldNotBeNil)

			var decoded TCPOptions
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)

			So(len(decoded), ShouldEqual, 1)
			So(decoded[0].Type, ShouldEqual, layers.TCPOptionKindTimestamps)
			So(decoded[0].Length, ShouldEqual, 10)
			So(decoded[0].Data, ShouldResemble, []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08})
		})

		Convey("Should handle nil slice with Value method", func() {
			var options TCPOptions // nil 切片
			value, err := options.Value()
			So(err, ShouldBeNil)
			// nil 切片对应 SQL NULL，应返回 nil driver.Value
			So(value, ShouldBeNil)
		})

		Convey("Should handle empty string in Scan method", func() {
			var options TCPOptions
			err := options.Scan("")
			So(err, ShouldBeNil)
			So(options, ShouldBeNil)
		})

		Convey("Should handle empty bytes in Scan method", func() {
			var options TCPOptions
			err := options.Scan([]byte{})
			So(err, ShouldBeNil)
			So(options, ShouldBeNil)
		})
	})
}

// TestTCPHeaders_Basic 测试TCPHeaders结构体的基本功能
// 包括字段验证、标志位判断和JSON序列化
func TestTCPHeaders_Basic(t *testing.T) {
	Convey("Test TCPHeaders basic functionality", t, func() {
		Convey("Should create TCPHeaders with all fields", func() {
			headers := TCPHeaders{
				TCPSrcPort:    layers.TCPPort(8080),
				TCPDstPort:    layers.TCPPort(443),
				TCPSeq:        1234567890,
				TCPAck:        987654321,
				TCPDataOffset: 5,
				TCPBitFIN:     false,
				TCPBitSYN:     true,
				TCPBitRST:     false,
				TCPBitPSH:     false,
				TCPBitACK:     false,
				TCPBitURG:     false,
				TCPBitECE:     false,
				TCPBitCWR:     false,
				TCPBitNS:      false,
				TCPWindow:     64240,
				TCPChecksum:   0x1234,
				TCPUrgent:     0,
				TCPOptions: TCPOptions{
					{
						Type:   layers.TCPOptionKindMSS,
						Length: 4,
						Data:   []byte{0x05, 0xB4},
					},
				},
			}

			So(headers.SrcPort(), ShouldEqual, layers.TCPPort(8080))
			So(headers.DstPort(), ShouldEqual, layers.TCPPort(443))
			So(headers.SeqNum(), ShouldEqual, uint32(1234567890))
			So(headers.AckNum(), ShouldEqual, uint32(987654321))
			So(headers.TCPDataOffset, ShouldEqual, 5)
			So(headers.TCPWindow, ShouldEqual, 64240)
			So(headers.TCPChecksum, ShouldEqual, 0x1234)
			So(len(headers.TCPOptions), ShouldEqual, 1)
		})

		Convey("Should correctly identify SYN packet", func() {
			headers := TCPHeaders{
				TCPBitSYN: true,
				TCPBitACK: false,
				TCPBitFIN: false,
			}

			So(headers.IsSyn(), ShouldBeTrue)
			So(headers.IsSynAck(), ShouldBeFalse)
			So(headers.IsAck(), ShouldBeFalse)
			So(headers.IsFin(), ShouldBeFalse)
			So(headers.IsRst(), ShouldBeFalse)
		})

		Convey("Should correctly identify SYN-ACK packet", func() {
			headers := TCPHeaders{
				TCPBitSYN: true,
				TCPBitACK: true,
				TCPBitFIN: false,
			}

			So(headers.IsSyn(), ShouldBeFalse) // 不是纯SYN包
			So(headers.IsSynAck(), ShouldBeTrue)
			So(headers.IsAck(), ShouldBeFalse)
			So(headers.IsFin(), ShouldBeFalse)
			So(headers.IsRst(), ShouldBeFalse)
		})

		Convey("Should correctly identify ACK packet", func() {
			headers := TCPHeaders{
				TCPBitSYN: false,
				TCPBitACK: true,
				TCPBitFIN: false,
			}

			So(headers.IsSyn(), ShouldBeFalse)
			So(headers.IsSynAck(), ShouldBeFalse)
			So(headers.IsAck(), ShouldBeTrue)
			So(headers.IsFin(), ShouldBeFalse)
			So(headers.IsRst(), ShouldBeFalse)
		})

		Convey("Should correctly identify FIN packet", func() {
			headers := TCPHeaders{
				TCPBitFIN: true,
			}

			So(headers.IsFin(), ShouldBeTrue)
			So(headers.IsRst(), ShouldBeFalse)
		})

		Convey("Should correctly identify RST packet", func() {
			headers := TCPHeaders{
				TCPBitRST: true,
			}

			So(headers.IsFin(), ShouldBeFalse)
			So(headers.IsRst(), ShouldBeTrue)
		})

		Convey("Should serialize TCPHeaders to JSON", func() {
			headers := TCPHeaders{
				TCPSrcPort:    layers.TCPPort(8080),
				TCPDstPort:    layers.TCPPort(443),
				TCPSeq:        1234567890,
				TCPAck:        987654321,
				TCPDataOffset: 5,
				TCPBitSYN:     true,
				TCPBitACK:     false,
				TCPWindow:     64240,
				TCPChecksum:   0x1234,
			}

			jsonData, err := json.Marshal(headers)
			So(err, ShouldBeNil)

			var decoded TCPHeaders
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)

			So(decoded.SrcPort(), ShouldEqual, headers.SrcPort())
			So(decoded.DstPort(), ShouldEqual, headers.DstPort())
			So(decoded.SeqNum(), ShouldEqual, headers.SeqNum())
			So(decoded.AckNum(), ShouldEqual, headers.AckNum())
			So(decoded.TCPBitSYN, ShouldEqual, headers.TCPBitSYN)
			So(decoded.TCPBitACK, ShouldEqual, headers.TCPBitACK)
		})
	})
}

// TestNewTCPHeaders 测试NewTCPHeaders函数的创建功能
// 验证从gopacket TCP层正确创建TCPHeaders
func TestNewTCPHeaders(t *testing.T) {
	Convey("Test NewTCPHeaders function", t, func() {
		Convey("Should create TCPHeaders from gopacket TCP layer", func() {
			tcpLayer := &layers.TCP{
				SrcPort:    8080,
				DstPort:    443,
				Seq:        1234567890,
				Ack:        987654321,
				DataOffset: 5,
				FIN:        false,
				SYN:        true,
				RST:        false,
				PSH:        false,
				ACK:        false,
				URG:        false,
				ECE:        false,
				CWR:        false,
				NS:         false,
				Window:     64240,
				Checksum:   0x1234,
				Urgent:     0,
				Options: []layers.TCPOption{
					{
						OptionType:   layers.TCPOptionKindMSS,
						OptionLength: 4,
						OptionData:   []byte{0x05, 0xB4},
					},
				},
			}

			headers := NewTCPHeaders(tcpLayer)

			So(headers.SrcPort(), ShouldEqual, layers.TCPPort(8080))
			So(headers.DstPort(), ShouldEqual, layers.TCPPort(443))
			So(headers.SeqNum(), ShouldEqual, uint32(1234567890))
			So(headers.AckNum(), ShouldEqual, uint32(987654321))
			So(headers.TCPDataOffset, ShouldEqual, 5)
			So(headers.TCPBitSYN, ShouldBeTrue)
			So(headers.TCPBitACK, ShouldBeFalse)
			So(headers.TCPWindow, ShouldEqual, 64240)
			So(headers.TCPChecksum, ShouldEqual, 0x1234)
			So(len(headers.TCPOptions), ShouldEqual, 1)
			So(headers.TCPOptions[0].Type, ShouldEqual, layers.TCPOptionKindMSS)
		})

		Convey("Should handle TCP layer with no options", func() {
			tcpLayer := &layers.TCP{
				SrcPort:    80,
				DstPort:    54321,
				Seq:        1000,
				Ack:        2000,
				DataOffset: 5,
				SYN:        true,
				ACK:        true,
				Window:     65535,
			}

			headers := NewTCPHeaders(tcpLayer)

			So(headers.SrcPort(), ShouldEqual, layers.TCPPort(80))
			So(headers.DstPort(), ShouldEqual, layers.TCPPort(54321))
			So(headers.IsSynAck(), ShouldBeTrue)
			So(headers.TCPOptions, ShouldBeEmpty)
		})
	})
}

// TestPacketIPv4TCP_StreamID 测试PacketIPv4TCP的StreamID生成功能
// 验证流ID的一致性、正确性以及TCPStreamID数据库字段的赋值
func TestPacketIPv4TCP_StreamID(t *testing.T) {
	Convey("Test PacketIPv4TCP StreamID generation", t, func() {
		Convey("Should generate consistent StreamID regardless of packet direction", func() {
			// 创建测试数据包结构
			packet1 := &PacketIPv4TCP{
				PacketIPv4: PacketIPv4{
					IPv4Headers: IPv4Headers{
						IPv4Src: serializer.IPv4Addr{192, 168, 1, 100},
						IPv4Dst: serializer.IPv4Addr{192, 168, 1, 1},
					},
				},
				TCPHeaders: TCPHeaders{
					TCPSrcPort: layers.TCPPort(54321),
					TCPDstPort: layers.TCPPort(80),
				},
			}

			packet2 := &PacketIPv4TCP{
				PacketIPv4: PacketIPv4{
					IPv4Headers: IPv4Headers{
						IPv4Src: serializer.IPv4Addr{192, 168, 1, 1},
						IPv4Dst: serializer.IPv4Addr{192, 168, 1, 100},
					},
				},
				TCPHeaders: TCPHeaders{
					TCPSrcPort: layers.TCPPort(80),
					TCPDstPort: layers.TCPPort(54321),
				},
			}

			streamID1 := packet1.StreamID()
			streamID2 := packet2.StreamID()

			So(streamID1, ShouldEqual, streamID2)
			So(len(streamID1), ShouldEqual, 16) // 64 位哈希的十六进制表示

			// 验证TCPStreamID数据库字段可以直接设置为StreamID计算结果
			packet1.TCPStreamID = packet1.StreamID()
			packet2.TCPStreamID = packet2.StreamID()
			So(packet1.TCPStreamID, ShouldEqual, streamID1)
			So(packet2.TCPStreamID, ShouldEqual, streamID2)
		})
	})
}

// TestPacketIPv4TCP_CaptureTimestamp 测试 PacketIPv4TCP.CaptureTimestamp 方法
// 验证 CaptureTimestamp 正确返回 TimestampMeta 中的时间戳
func TestPacketIPv4TCP_CaptureTimestamp(t *testing.T) {
	Convey("Test PacketIPv4TCP CaptureTimestamp", t, func() {
		Convey("Should return Timestamp from TimestampMeta", func() {
			now := time.Now()
			pkt := &PacketIPv4TCP{
				PacketIPv4: PacketIPv4{
					PacketEthernet: PacketEthernet{
						AsPacket: AsPacket{
							TimestampMeta: TimestampMeta{Timestamp: now},
						},
					},
				},
			}
			So(pkt.CaptureTimestamp().Equal(now), ShouldBeTrue)
		})
	})
}

// TestPacketIPv6TCP_CaptureTimestamp 测试 PacketIPv6TCP.CaptureTimestamp 方法
func TestPacketIPv6TCP_CaptureTimestamp(t *testing.T) {
	Convey("Test PacketIPv6TCP CaptureTimestamp", t, func() {
		Convey("Should return Timestamp from TimestampMeta", func() {
			now := time.Now()
			pkt := &PacketIPv6TCP{
				PacketIPv6: PacketIPv6{
					PacketEthernet: PacketEthernet{
						AsPacket: AsPacket{
							TimestampMeta: TimestampMeta{Timestamp: now},
						},
					},
				},
			}
			So(pkt.CaptureTimestamp().Equal(now), ShouldBeTrue)
		})
	})
}

// TestAppTailer 测试 AppTailer 结构体
// 验证 AppTailer 结构体字段的正确性
func TestAppTailer(t *testing.T) {
	Convey("Test AppTailer structure", t, func() {
		Convey("Should initialize with all zero values", func() {
			tailer := AppTailer{}
			So(tailer.ClientConsumed, ShouldEqual, uint64(0))
			So(tailer.ServerConsumed, ShouldEqual, uint64(0))
			So(tailer.ClientStartPayloadOffset, ShouldEqual, uint64(0))
			So(tailer.ClientEndPayloadOffset, ShouldEqual, uint64(0))
			So(tailer.ServerStartPayloadOffset, ShouldEqual, uint64(0))
			So(tailer.ServerEndPayloadOffset, ShouldEqual, uint64(0))
			So(tailer.ClientStartTimestamp.IsZero(), ShouldBeTrue)
			So(tailer.ClientEndTimestamp.IsZero(), ShouldBeTrue)
			So(tailer.ServerStartTimestamp.IsZero(), ShouldBeTrue)
			So(tailer.ServerEndTimestamp.IsZero(), ShouldBeTrue)
		})

		Convey("Should set all fields correctly", func() {
			now := time.Now()
			tailer := AppTailer{
				ClientConsumed:           100,
				ClientStartPayloadOffset: 0,
				ClientEndPayloadOffset:   100,
				ClientStartTimestamp:     now,
				ClientEndTimestamp:       now.Add(time.Second),
				ServerConsumed:           200,
				ServerStartPayloadOffset: 0,
				ServerEndPayloadOffset:   200,
				ServerStartTimestamp:     now,
				ServerEndTimestamp:       now.Add(time.Second),
			}
			So(tailer.ClientConsumed, ShouldEqual, uint64(100))
			So(tailer.ServerConsumed, ShouldEqual, uint64(200))
			So(tailer.ClientStartTimestamp.Equal(now), ShouldBeTrue)
		})
	})
}

// TestPacketIPv6TCP_StreamID 测试PacketIPv6TCP的StreamID生成功能
// 验证IPv6流ID的一致性、正确性以及TCPStreamID数据库字段的赋值
func TestPacketIPv6TCP_StreamID(t *testing.T) {
	Convey("Test PacketIPv6TCP StreamID generation", t, func() {
		Convey("Should generate consistent StreamID for IPv6 regardless of packet direction", func() {
			// 创建测试数据包结构
			packet1 := &PacketIPv6TCP{
				PacketIPv6: PacketIPv6{
					IPv6Headers: IPv6Headers{
						IPv6Base: IPv6Base{
							IPv6Src: serializer.IPv6Addr{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
							IPv6Dst: serializer.IPv6Addr{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2},
						},
					},
				},
				TCPHeaders: TCPHeaders{
					TCPSrcPort: layers.TCPPort(54321),
					TCPDstPort: layers.TCPPort(80),
				},
			}

			packet2 := &PacketIPv6TCP{
				PacketIPv6: PacketIPv6{
					IPv6Headers: IPv6Headers{
						IPv6Base: IPv6Base{
							IPv6Src: serializer.IPv6Addr{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2},
							IPv6Dst: serializer.IPv6Addr{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
						},
					},
				},
				TCPHeaders: TCPHeaders{
					TCPSrcPort: layers.TCPPort(80),
					TCPDstPort: layers.TCPPort(54321),
				},
			}

			streamID1 := packet1.StreamID()
			streamID2 := packet2.StreamID()

			So(streamID1, ShouldEqual, streamID2)
			So(len(streamID1), ShouldEqual, 16) // 64 位哈希的十六进制表示

			// 验证TCPStreamID数据库字段可以直接设置为StreamID计算结果
			packet1.TCPStreamID = packet1.StreamID()
			packet2.TCPStreamID = packet2.StreamID()
			So(packet1.TCPStreamID, ShouldEqual, streamID1)
			So(packet2.TCPStreamID, ShouldEqual, streamID2)
		})
	})
}
