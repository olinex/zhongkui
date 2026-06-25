package network

import (
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	. "github.com/smartystreets/goconvey/convey"
	reporter_v1 "zhongkui.com/zhongkui/internal/pkg/protobuf/reporter/v1"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// TestExtractLayerInfos 测试ExtractLayerInfos函数
func TestExtractLayerInfos(t *testing.T) {
	Convey("Test ExtractLayerInfos function", t, func() {
		Convey("Should extract Ethernet layer information", func() {
			// 创建以太网数据包
			buf := gopacket.NewSerializeBuffer()
			opts := gopacket.SerializeOptions{
				FixLengths:       true,
				ComputeChecksums: true,
			}
			eth := &layers.Ethernet{
				SrcMAC:       []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
				DstMAC:       []byte{0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB},
				EthernetType: layers.EthernetTypeIPv4,
			}
			ip := &layers.IPv4{
				Version:  4,
				TTL:      64,
				Protocol: layers.IPProtocolTCP,
				SrcIP:    []byte{192, 168, 1, 1},
				DstIP:    []byte{192, 168, 1, 2},
			}
			tcp := &layers.TCP{
				SrcPort: 8080,
				DstPort: 80,
				SYN:     true,
			}
			tcp.SetNetworkLayerForChecksum(ip)
			payload := []byte("test payload")

			err := gopacket.SerializeLayers(buf, opts,
				eth,
				ip,
				tcp,
				gopacket.Payload(payload),
			)
			So(err, ShouldBeNil)

			packet := gopacket.NewPacket(buf.Bytes(), layers.LayerTypeEthernet, gopacket.Default)

			// 提取层信息
			level, stack, extractedPayload := ExtractLayerInfos(packet, LayerLevelLink)

			// 验证结果
			So(level, ShouldEqual, LayerLevelTransport)
			So(len(stack), ShouldEqual, 3)
			So(stack[0].Level, ShouldEqual, LayerLevelLink)
			So(stack[0].Proto, ShouldEqual, layers.LayerTypeEthernet)
			So(stack[1].Level, ShouldEqual, LayerLevelNetwork)
			So(stack[1].Proto, ShouldEqual, layers.LayerTypeIPv4)
			So(stack[2].Level, ShouldEqual, LayerLevelTransport)
			So(stack[2].Proto, ShouldEqual, layers.LayerTypeTCP)
			So(string(extractedPayload), ShouldEqual, "test payload")
		})

		Convey("Should handle UDP packets", func() {
			// 创建UDP数据包
			buf := gopacket.NewSerializeBuffer()
			opts := gopacket.SerializeOptions{
				FixLengths:       true,
				ComputeChecksums: true,
			}
			eth := &layers.Ethernet{
				SrcMAC:       []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
				DstMAC:       []byte{0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB},
				EthernetType: layers.EthernetTypeIPv4,
			}
			ip := &layers.IPv4{
				Version:  4,
				TTL:      64,
				Protocol: layers.IPProtocolUDP,
				SrcIP:    []byte{10, 0, 0, 1},
				DstIP:    []byte{10, 0, 0, 2},
			}
			udp := &layers.UDP{
				SrcPort: 12345,
				DstPort: 1234,
			}
			udp.SetNetworkLayerForChecksum(ip)
			payload := []byte("test payload")

			err := gopacket.SerializeLayers(buf, opts,
				eth,
				ip,
				udp,
				gopacket.Payload(payload),
			)
			So(err, ShouldBeNil)

			packet := gopacket.NewPacket(buf.Bytes(), layers.LayerTypeEthernet, gopacket.Default)

			// 提取层信息
			level, stack, extractedPayload := ExtractLayerInfos(packet, LayerLevelLink)

			// 验证结果
			So(level, ShouldEqual, LayerLevelTransport)
			So(len(stack), ShouldEqual, 3)
			So(stack[2].Proto, ShouldEqual, layers.LayerTypeUDP)
			So(string(extractedPayload), ShouldEqual, "test payload")
		})

		Convey("Should handle IPv6 packets", func() {
			// 创建IPv6数据包
			buf := gopacket.NewSerializeBuffer()
			opts := gopacket.SerializeOptions{
				FixLengths:       true,
				ComputeChecksums: true,
			}
			eth := &layers.Ethernet{
				SrcMAC:       []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
				DstMAC:       []byte{0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB},
				EthernetType: layers.EthernetTypeIPv6,
			}
			ip := &layers.IPv6{
				Version:    6,
				NextHeader: layers.IPProtocolTCP,
				SrcIP:      []byte{0x20, 0x01, 0x0d, 0xb8, 0x85, 0xa3, 0x00, 0x00, 0x00, 0x00, 0x8a, 0x2e, 0x03, 0x70, 0x73, 0x34},
				DstIP:      []byte{0x20, 0x01, 0x0d, 0xb8, 0x85, 0xa3, 0x00, 0x00, 0x00, 0x00, 0x8a, 0x2e, 0x03, 0x70, 0x73, 0x35},
			}
			tcp := &layers.TCP{
				SrcPort: 443,
				DstPort: 443,
				SYN:     true,
			}
			tcp.SetNetworkLayerForChecksum(ip)
			payload := []byte("tls handshake")

			gopacket.SerializeLayers(buf, opts,
				eth,
				ip,
				tcp,
				gopacket.Payload(payload),
			)

			packet := gopacket.NewPacket(buf.Bytes(), layers.LayerTypeEthernet, gopacket.Default)

			// 提取层信息
			level, stack, extractedPayload := ExtractLayerInfos(packet, LayerLevelLink)

			// 验证结果
			So(level, ShouldEqual, LayerLevelTransport)
			So(len(stack), ShouldEqual, 3)
			So(stack[1].Proto, ShouldEqual, layers.LayerTypeIPv6)
			So(string(extractedPayload), ShouldEqual, "tls handshake")
		})

		Convey("Should handle payload-only packets", func() {
			// 创建只有载荷的数据包
			payload := []byte("raw payload data")
			packet := gopacket.NewPacket(payload, gopacket.LayerTypePayload, gopacket.Default)

			// 提取层信息
			level, stack, extractedPayload := ExtractLayerInfos(packet, LayerLevelLink)

			// 验证结果
			So(level, ShouldEqual, LayerLevelLink)
			So(len(stack), ShouldEqual, 0)
			So(string(extractedPayload), ShouldEqual, "raw payload data")
		})
	})
}

// TestModelBuilder 测试ModelBuilder相关功能
func TestModelBuilder(t *testing.T) {
	Convey("Test ModelBuilder functions", t, func() {
		// 创建测试数据包
		buf := gopacket.NewSerializeBuffer()
		opts := gopacket.SerializeOptions{
			FixLengths:       true,
			ComputeChecksums: true,
		}
		eth := &layers.Ethernet{
			SrcMAC:       []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
			DstMAC:       []byte{0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB},
			EthernetType: layers.EthernetTypeIPv4,
		}
		ip := &layers.IPv4{
			Version:  4,
			TTL:      64,
			Protocol: layers.IPProtocolTCP,
			SrcIP:    []byte{192, 168, 1, 100},
			DstIP:    []byte{192, 168, 1, 200},
			Id:       12345,
			Flags:    layers.IPv4DontFragment,
		}
		tcp := &layers.TCP{
			SrcPort: 54321,
			DstPort: 80,
			SYN:     true,
			ACK:     false,
			Seq:     1000,
			Window:  65535,
		}
		tcp.SetNetworkLayerForChecksum(ip)
		payload := []byte("HTTP/1.1 GET /test")

		err := gopacket.SerializeLayers(buf, opts,
			eth,
			ip,
			tcp,
			gopacket.Payload(payload),
		)
		So(err, ShouldBeNil)

		packet := gopacket.NewPacket(buf.Bytes(), layers.LayerTypeEthernet, gopacket.Default)
		packet.Metadata().CaptureInfo = gopacket.CaptureInfo{
			Timestamp:      time.Now(),
			CaptureLength:  len(buf.Bytes()),
			Length:         len(buf.Bytes()),
			InterfaceIndex: 0,
		}

		// 创建ModelBuilder
		sessionID := serializer.HexUUID{0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF}
		source := reporter_v1.ResourceRaw{
			Namespace: "test-namespace",
			Name:      "test-capturer",
			Version:   "v1.0.0",
		}

		builder := NewPacketModelBuilder(packet, sessionID, source)

		Convey("NewPacketModelBuilder should create valid builder", func() {
			So(builder, ShouldNotBeNil)
			So(builder.SessionID, ShouldResemble, sessionID)
			So(builder.Source.Namespace, ShouldEqual, "test-namespace")
			So(builder.Source.Name, ShouldEqual, "test-capturer")
			So(builder.Source.Version, ShouldEqual, "v1.0.0")
			So(builder.Level, ShouldEqual, LayerLevelTransport)
			So(len(builder.Stack), ShouldEqual, 3)
			So(len(builder.Payload), ShouldBeGreaterThan, 0)
		})

		Convey("BuildAsPacketLinkModel should create Ethernet packet model", func() {
			model := builder.BuildAsPacketLinkModel()
			So(model, ShouldNotBeNil)

			ethModel, ok := model.(*PacketEthernet)
			So(ok, ShouldBeTrue)
			So(ethModel.Namespace, ShouldEqual, "test-namespace")
			So(ethModel.Capture, ShouldEqual, "test-capturer")
			So(ethModel.Version, ShouldEqual, "v1.0.0")
			So(ethModel.SessionID, ShouldResemble, sessionID)
			So(ethModel.EthSrcMAC.String(), ShouldEqual, "00:11:22:33:44:55")
			So(ethModel.EthDstMAC.String(), ShouldEqual, "66:77:88:99:aa:bb")
			So(ethModel.EthType, ShouldEqual, layers.EthernetTypeIPv4)
		})

		Convey("BuildAsPacketNetworkModel should create IPv4 packet model", func() {
			model := builder.BuildAsPacketNetworkModel()
			So(model, ShouldNotBeNil)

			ipv4Model, ok := model.(*PacketIPv4)
			So(ok, ShouldBeTrue)
			So(ipv4Model.Namespace, ShouldEqual, "test-namespace")
			So(ipv4Model.IPv4Src.String(), ShouldEqual, "192.168.1.100")
			So(ipv4Model.IPv4Dst.String(), ShouldEqual, "192.168.1.200")
			So(ipv4Model.IPv4IPProtocol, ShouldEqual, layers.IPProtocolTCP)
			So(ipv4Model.IPv4TTL, ShouldEqual, 64)
		})

		Convey("BuildAsPacketTransportModel should create TCP packet model", func() {
			model := builder.BuildAsPacketTransportModel()
			So(model, ShouldNotBeNil)

			tcpModel, ok := model.(*PacketIPv4TCP)
			So(ok, ShouldBeTrue)
			So(tcpModel.Namespace, ShouldEqual, "test-namespace")
			So(tcpModel.TCPSrcPort, ShouldEqual, layers.TCPPort(54321))
			So(tcpModel.TCPDstPort, ShouldEqual, layers.TCPPort(80))
			So(tcpModel.TCPBitSYN, ShouldBeTrue)
			So(tcpModel.TCPBitACK, ShouldBeFalse)
			So(tcpModel.TCPSeq, ShouldEqual, 1000)
		})

		Convey("BuildAsPacketModel should auto-select correct model type", func() {
			model := builder.BuildAsPacketModel()
			So(model, ShouldNotBeNil)

			// 根据层级应该返回TCP模型
			tcpModel, ok := model.(*PacketIPv4TCP)
			So(ok, ShouldBeTrue)
			So(tcpModel.TCPDstPort, ShouldEqual, layers.TCPPort(80))
		})

		Convey("BuildAsPacketApplicationModel should return nil (not implemented)", func() {
			model := builder.BuildAsPacketApplicationModel()
			So(model, ShouldBeNil)
		})
	})
}

// TestModelBuilderErrorCases 测试ModelBuilder的错误情况
func TestModelBuilderErrorCases(t *testing.T) {
	Convey("Test ModelBuilder error cases", t, func() {

		Convey("BuildAsPacketLinkModel should return nil with empty stack", func() {
			builder := &ModelBuilder{
				Stack: LayerStack{},
			}

			result := builder.BuildAsPacketLinkModel()
			So(result, ShouldBeNil)
		})

		Convey("BuildAsPacketLinkModel should return nil with wrong layer level", func() {
			builder := &ModelBuilder{
				Stack: LayerStack{
					{
						Level: LayerLevelNetwork, // 错误的层级
						Proto: layers.LayerTypeEthernet,
					},
				},
			}

			result := builder.BuildAsPacketLinkModel()
			So(result, ShouldBeNil)
		})

		Convey("BuildAsPacketLinkModel should return nil with nil headers", func() {
			builder := &ModelBuilder{
				Stack: LayerStack{
					{
						Level:   LayerLevelLink,
						Proto:   layers.LayerTypeEthernet,
						Headers: nil,
					},
				},
			}

			result := builder.BuildAsPacketLinkModel()
			So(result, ShouldBeNil)
		})

		Convey("BuildAsPacketNetworkModel should panic with insufficient layers", func() {
			builder := &ModelBuilder{
				Stack: LayerStack{
					{
						Level: LayerLevelLink,
						Proto: layers.LayerTypeEthernet,
					},
				},
			}

			So(func() {
				builder.BuildAsPacketNetworkModel()
			}, ShouldPanicWith, "network layer stack have not enough layers")
		})
	})
}

// TestTunnelProtocols 测试隧道协议解析
func TestTunnelProtocols(t *testing.T) {
	Convey("Test tunnel protocol extraction", t, func() {
		// 注意：由于gopacket库的限制，我们无法直接创建完整的VXLAN/GRE/Geneve/MPLS数据包进行测试
		// 这里主要测试函数的基本逻辑和错误处理

		Convey("ExtractLayerInfos should handle unknown protocol layers", func() {
			// 创建包含未知协议的数据包
			packet := gopacket.NewPacket([]byte("unknown protocol data"), gopacket.LayerType(999), gopacket.Default)

			level, stack, payload := ExtractLayerInfos(packet, LayerLevelLink)

			So(level, ShouldEqual, LayerLevelLink)
			So(len(stack), ShouldEqual, 1)
			So(stack[0].Level, ShouldEqual, LayerLevelLink)
			So(stack[0].Proto, ShouldEqual, gopacket.LayerTypeDecodeFailure)
			So(string(payload), ShouldEqual, "unknown protocol data")
		})
	})
}

// BenchmarkExtractLayerInfos 性能测试
func BenchmarkExtractLayerInfos(b *testing.B) {
	// 准备测试数据包
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}
	eth := &layers.Ethernet{
		SrcMAC:       []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		DstMAC:       []byte{0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{
		Version:  4,
		TTL:      64,
		Protocol: layers.IPProtocolTCP,
		SrcIP:    []byte{192, 168, 1, 1},
		DstIP:    []byte{192, 168, 1, 2},
	}
	tcp := &layers.TCP{
		SrcPort: 8080,
		DstPort: 80,
		SYN:     true,
	}
	tcp.SetNetworkLayerForChecksum(ip)
	payload := []byte("benchmark payload")

	gopacket.SerializeLayers(buf, opts,
		eth,
		ip,
		tcp,
		gopacket.Payload(payload),
	)

	packetData := buf.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		packet := gopacket.NewPacket(packetData, layers.LayerTypeEthernet, gopacket.Default)
		ExtractLayerInfos(packet, LayerLevelLink)
	}
}

// BenchmarkModelBuilder 性能测试
func BenchmarkModelBuilder(b *testing.B) {
	// 准备测试数据包
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}
	eth := &layers.Ethernet{
		SrcMAC:       []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		DstMAC:       []byte{0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{
		Version:  4,
		TTL:      64,
		Protocol: layers.IPProtocolTCP,
		SrcIP:    []byte{192, 168, 1, 1},
		DstIP:    []byte{192, 168, 1, 2},
	}
	tcp := &layers.TCP{
		SrcPort: 8080,
		DstPort: 80,
		SYN:     true,
	}
	tcp.SetNetworkLayerForChecksum(ip)
	payload := []byte("benchmark payload")

	gopacket.SerializeLayers(buf, opts,
		eth,
		ip,
		tcp,
		gopacket.Payload(payload),
	)

	packet := gopacket.NewPacket(buf.Bytes(), layers.LayerTypeEthernet, gopacket.Default)
	packet.Metadata().CaptureInfo = gopacket.CaptureInfo{
		Timestamp:      time.Now(),
		CaptureLength:  len(buf.Bytes()),
		Length:         len(buf.Bytes()),
		InterfaceIndex: 0,
	}

	sessionID := serializer.HexUUID{0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF}
	source := reporter_v1.ResourceRaw{
		Namespace: "benchmark-namespace",
		Name:      "benchmark-capturer",
		Version:   "v1.0.0",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder := NewPacketModelBuilder(packet, sessionID, source)
		builder.BuildAsPacketModel()
	}
}
