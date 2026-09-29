package tcp

import (
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket/layers"
	. "github.com/smartystreets/goconvey/convey"
	n "zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

type mockTCPPacket struct {
	meta       *n.BaseMeta
	streamID   string
	frameInfo  *n.FrameInfo
	ingestedAt time.Time
	srcAddr    net.IP
	dstAddr    net.IP
	srcPort    layers.TCPPort
	dstPort    layers.TCPPort
	seqNum     uint32
	ackNum     uint32
	flags      struct {
		syn    bool
		synAck bool
		ack    bool
		fin    bool
		rst    bool
	}
	payload []byte
}

// Meta 返回模拟数据包的基础元数据。
func (m *mockTCPPacket) Meta() *n.BaseMeta {
	return m.meta
}

// StreamID 返回所属 TCP 流标识。
func (m *mockTCPPacket) StreamID() string {
	return m.streamID
}

// CaptureTimestamp 返回帧捕获时间戳。
func (m *mockTCPPacket) CaptureTimestamp() time.Time {
	return m.frameInfo.Timestamp
}

// IngestedAt 返回入库时间。
func (m *mockTCPPacket) IngestedAt() time.Time {
	return m.ingestedAt
}

// FrameInfo 返回帧信息。
func (m *mockTCPPacket) FrameInfo() *n.FrameInfo {
	return m.frameInfo
}

// NetworkFrame 根据帧信息与元数据构造网络帧。
func (m *mockTCPPacket) NetworkFrame() n.NetworkFrame {
	return n.NewNetworkFrame(m.frameInfo.LayerStack, *m.meta, m.frameInfo.Timestamp)
}

// SrcAddr 返回源 IP 地址。
func (m *mockTCPPacket) SrcAddr() net.IP {
	return m.srcAddr
}

// DstAddr 返回目的 IP 地址。
func (m *mockTCPPacket) DstAddr() net.IP {
	return m.dstAddr
}

// SrcPort 返回源端口。
func (m *mockTCPPacket) SrcPort() layers.TCPPort {
	return m.srcPort
}

// DstPort 返回目的端口。
func (m *mockTCPPacket) DstPort() layers.TCPPort {
	return m.dstPort
}

// SeqNum 返回 TCP 序列号。
func (m *mockTCPPacket) SeqNum() uint32 {
	return m.seqNum
}

// AckNum 返回 TCP 确认号。
func (m *mockTCPPacket) AckNum() uint32 {
	return m.ackNum
}

// IsSyn 返回是否带 SYN 标志。
func (m *mockTCPPacket) IsSyn() bool {
	return m.flags.syn
}

// IsSynAck 返回是否带 SYN-ACK 标志。
func (m *mockTCPPacket) IsSynAck() bool {
	return m.flags.synAck
}

// IsAck 返回是否带 ACK 标志。
func (m *mockTCPPacket) IsAck() bool {
	return m.flags.ack
}

// IsFin 返回是否带 FIN 标志。
func (m *mockTCPPacket) IsFin() bool {
	return m.flags.fin
}

// IsRst 返回是否带 RST 标志。
func (m *mockTCPPacket) IsRst() bool {
	return m.flags.rst
}

// GetPayload 返回 TCP 负载数据。
func (m *mockTCPPacket) GetPayload() []byte {
	return m.payload
}

// TcpFingerprint 返回模拟数据包的被动 TCP 指纹，默认无指纹返回 nil
func (m *mockTCPPacket) TcpFingerprint() *n.PacketTCPFingerprint {
	return nil
}

func newMockTCPPacket() *mockTCPPacket {
	return &mockTCPPacket{
		meta: &n.BaseMeta{
			Namespace: "test-namespace",
			Capture:   "test-capture",
			Version:   "1.0.0",
			SessionID: serializer.HexUUID{0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF},
		},
		streamID: "a1b2c3d4e5f678901234567890123456",
		frameInfo: &n.FrameInfo{
			Timestamp: time.Now(),
			LayerStack: n.LayerStack{
				{
					Level:   n.LayerLevelNetwork,
					Proto:   layers.LayerTypeIPv4,
					Headers: nil,
				},
			},
		},
		ingestedAt: time.Now(),
		srcAddr:    net.ParseIP("192.168.1.100"),
		dstAddr:    net.ParseIP("10.0.0.1"),
		srcPort:    8080,
		dstPort:    443,
		seqNum:     1234567890,
		ackNum:     0,
		payload:    []byte("test payload data"),
	}
}

func newMockTCPPacketWithOptions(options ...func(*mockTCPPacket)) *mockTCPPacket {
	packet := newMockTCPPacket()
	for _, option := range options {
		option(packet)
	}
	return packet
}

func withSrcAddr(addr net.IP) func(*mockTCPPacket) {
	return func(m *mockTCPPacket) {
		m.srcAddr = addr
	}
}

func withDstAddr(addr net.IP) func(*mockTCPPacket) {
	return func(m *mockTCPPacket) {
		m.dstAddr = addr
	}
}

func withSrcPort(port layers.TCPPort) func(*mockTCPPacket) {
	return func(m *mockTCPPacket) {
		m.srcPort = port
	}
}

func withDstPort(port layers.TCPPort) func(*mockTCPPacket) {
	return func(m *mockTCPPacket) {
		m.dstPort = port
	}
}

func withSeqNum(seq uint32) func(*mockTCPPacket) {
	return func(m *mockTCPPacket) {
		m.seqNum = seq
	}
}

func withSynFlag(enabled bool) func(*mockTCPPacket) {
	return func(m *mockTCPPacket) {
		m.flags.syn = enabled
	}
}

func withSynAckFlag(enabled bool) func(*mockTCPPacket) {
	return func(m *mockTCPPacket) {
		m.flags.synAck = enabled
	}
}

func withAckFlag(enabled bool) func(*mockTCPPacket) {
	return func(m *mockTCPPacket) {
		m.flags.ack = enabled
	}
}

func withFinFlag(enabled bool) func(*mockTCPPacket) {
	return func(m *mockTCPPacket) {
		m.flags.fin = enabled
	}
}

func withRstFlag(enabled bool) func(*mockTCPPacket) {
	return func(m *mockTCPPacket) {
		m.flags.rst = enabled
	}
}

func withPayload(payload []byte) func(*mockTCPPacket) {
	return func(m *mockTCPPacket) {
		m.payload = payload
	}
}

// TestTCPPacketInterface 验证 mockTCPPacket 的默认值、构造选项与各访问器行为。
func TestTCPPacketInterface(t *testing.T) {
	Convey("Test TCPPacket interface", t, func() {
		Convey("Should create mock TCP packet with default values", func() {
			packet := newMockTCPPacket()

			So(packet, ShouldNotBeNil)
			So(packet.Meta().Namespace, ShouldEqual, "test-namespace")
			So(packet.SrcAddr().String(), ShouldEqual, "192.168.1.100")
			So(packet.DstAddr().String(), ShouldEqual, "10.0.0.1")
			So(packet.SrcPort(), ShouldEqual, 8080)
			So(packet.DstPort(), ShouldEqual, 443)
			So(packet.SeqNum(), ShouldEqual, 1234567890)
			So(string(packet.GetPayload()), ShouldEqual, "test payload data")
		})

		Convey("Should create TCP packet with custom IP addresses", func() {
			packet := newMockTCPPacketWithOptions(
				withSrcAddr(net.ParseIP("10.10.10.10")),
				withDstAddr(net.ParseIP("20.20.20.20")),
			)

			So(packet.SrcAddr().String(), ShouldEqual, "10.10.10.10")
			So(packet.DstAddr().String(), ShouldEqual, "20.20.20.20")
		})

		Convey("Should create TCP packet with custom ports", func() {
			packet := newMockTCPPacketWithOptions(
				withSrcPort(3000),
				withDstPort(80),
			)

			So(packet.SrcPort(), ShouldEqual, 3000)
			So(packet.DstPort(), ShouldEqual, 80)
		})

		Convey("Should create TCP packet with custom sequence number", func() {
			packet := newMockTCPPacketWithOptions(
				withSeqNum(987654321),
			)

			So(packet.SeqNum(), ShouldEqual, 987654321)
		})

		Convey("Should create TCP packet with SYN flag", func() {
			packet := newMockTCPPacketWithOptions(
				withSynFlag(true),
			)

			So(packet.IsSyn(), ShouldBeTrue)
			So(packet.IsSynAck(), ShouldBeFalse)
			So(packet.IsAck(), ShouldBeFalse)
		})

		Convey("Should create TCP packet with SYN-ACK flag", func() {
			packet := newMockTCPPacketWithOptions(
				withSynAckFlag(true),
			)

			So(packet.IsSyn(), ShouldBeFalse)
			So(packet.IsSynAck(), ShouldBeTrue)
			So(packet.IsAck(), ShouldBeFalse)
		})

		Convey("Should create TCP packet with ACK flag", func() {
			packet := newMockTCPPacketWithOptions(
				withAckFlag(true),
			)

			So(packet.IsSyn(), ShouldBeFalse)
			So(packet.IsSynAck(), ShouldBeFalse)
			So(packet.IsAck(), ShouldBeTrue)
		})

		Convey("Should create TCP packet with FIN flag", func() {
			packet := newMockTCPPacketWithOptions(
				withFinFlag(true),
			)

			So(packet.IsFin(), ShouldBeTrue)
			So(packet.IsRst(), ShouldBeFalse)
		})

		Convey("Should create TCP packet with RST flag", func() {
			packet := newMockTCPPacketWithOptions(
				withRstFlag(true),
			)

			So(packet.IsFin(), ShouldBeFalse)
			So(packet.IsRst(), ShouldBeTrue)
		})

		Convey("Should create TCP packet with custom payload", func() {
			customPayload := []byte("custom application data")
			packet := newMockTCPPacketWithOptions(
				withPayload(customPayload),
			)

			So(string(packet.GetPayload()), ShouldEqual, "custom application data")
		})

		Convey("Should create TCP packet with empty payload", func() {
			packet := newMockTCPPacketWithOptions(
				withPayload([]byte{}),
			)

			So(packet.GetPayload(), ShouldNotBeNil)
			So(len(packet.GetPayload()), ShouldEqual, 0)
		})

		Convey("Should handle IPv6 addresses", func() {
			packet := newMockTCPPacketWithOptions(
				withSrcAddr(net.ParseIP("2001:0db8:85a3:0000:0000:8a2e:0370:7334")),
				withDstAddr(net.ParseIP("2001:0db8:85a3:0000:0000:8a2e:0370:7335")),
			)

			So(packet.SrcAddr().String(), ShouldEqual, "2001:db8:85a3::8a2e:370:7334")
			So(packet.DstAddr().String(), ShouldEqual, "2001:db8:85a3::8a2e:370:7335")
		})
	})
}

// TestTCPPacketInterfaceCompliance 验证 mockTCPPacket 满足 TCPPacket 接口及各方法返回类型。
func TestTCPPacketInterfaceCompliance(t *testing.T) {
	Convey("Test TCPPacket interface compliance", t, func() {
		Convey("Should satisfy TCPPacket interface", func() {
			var packet TCPPacket
			mock := newMockTCPPacket()

			// 这行代码会编译失败如果mock没有实现TCPPacket接口
			packet = mock

			So(packet, ShouldNotBeNil)
			So(packet, ShouldEqual, mock)
		})

		Convey("Should return correct data types for all methods", func() {
			packet := newMockTCPPacket()

			// 测试Meta方法返回正确的类型
			meta := packet.Meta()
			So(meta, ShouldHaveSameTypeAs, &n.BaseMeta{})

			// 测试StreamID方法返回字符串
			streamID := packet.StreamID()
			So(streamID, ShouldHaveSameTypeAs, "")

			// 测试FrameInfo方法返回正确的类型
			frameInfo := packet.FrameInfo()
			So(frameInfo, ShouldHaveSameTypeAs, &n.FrameInfo{})

			// 测试IP地址方法返回net.IP类型
			srcAddr := packet.SrcAddr()
			So(srcAddr, ShouldHaveSameTypeAs, net.IP{})

			dstAddr := packet.DstAddr()
			So(dstAddr, ShouldHaveSameTypeAs, net.IP{})

			// 测试端口方法返回layers.TCPPort类型
			srcPort := packet.SrcPort()
			So(srcPort, ShouldHaveSameTypeAs, layers.TCPPort(0))

			dstPort := packet.DstPort()
			So(dstPort, ShouldHaveSameTypeAs, layers.TCPPort(0))

			// 测试序列号和确认号返回uint32类型
			seqNum := packet.SeqNum()
			So(seqNum, ShouldHaveSameTypeAs, uint32(0))

			ackNum := packet.AckNum()
			So(ackNum, ShouldHaveSameTypeAs, uint32(0))

			// 测试标志位方法返回bool类型
			So(packet.IsSyn(), ShouldHaveSameTypeAs, true)
			So(packet.IsSynAck(), ShouldHaveSameTypeAs, true)
			So(packet.IsAck(), ShouldHaveSameTypeAs, true)
			So(packet.IsFin(), ShouldHaveSameTypeAs, true)
			So(packet.IsRst(), ShouldHaveSameTypeAs, true)

			// 测试GetPayload方法返回[]byte类型
			payload := packet.GetPayload()
			So(payload, ShouldHaveSameTypeAs, []byte{})
		})
	})
}

// TestTCPPacketSeqFunction 验证 tcpPacketSeq 对各类地址、端口与序列号的格式化结果。
func TestTCPPacketSeqFunction(t *testing.T) {
	Convey("Test tcpPacketSeq function", t, func() {
		Convey("Should format TCP packet sequence information correctly", func() {
			packet := newMockTCPPacket()

			result := tcpPacketSeq(packet)

			So(result, ShouldEqual, "src: 192.168.1.100:8080, dst: 10.0.0.1:443, seq: 1234567890")
		})

		Convey("Should format TCP packet with different IP addresses and ports", func() {
			packet := newMockTCPPacketWithOptions(
				withSrcAddr(net.ParseIP("172.16.0.1")),
				withDstAddr(net.ParseIP("192.168.0.100")),
				withSrcPort(5000),
				withDstPort(9000),
				withSeqNum(555555555),
			)

			result := tcpPacketSeq(packet)

			So(result, ShouldEqual, "src: 172.16.0.1:5000, dst: 192.168.0.100:9000, seq: 555555555")
		})

		Convey("Should format TCP packet with IPv6 addresses", func() {
			packet := newMockTCPPacketWithOptions(
				withSrcAddr(net.ParseIP("2001:db8::1")),
				withDstAddr(net.ParseIP("2001:db8::2")),
				withSrcPort(22),
				withDstPort(22),
				withSeqNum(1111111111),
			)

			result := tcpPacketSeq(packet)

			So(result, ShouldEqual, "src: 2001:db8::1:22, dst: 2001:db8::2:22, seq: 1111111111")
		})

		Convey("Should handle zero sequence number", func() {
			packet := newMockTCPPacketWithOptions(
				withSeqNum(0),
			)

			result := tcpPacketSeq(packet)

			So(result, ShouldEqual, "src: 192.168.1.100:8080, dst: 10.0.0.1:443, seq: 0")
		})

		Convey("Should handle maximum sequence number", func() {
			packet := newMockTCPPacketWithOptions(
				withSeqNum(4294967295), // 2^32 - 1
			)

			result := tcpPacketSeq(packet)

			So(result, ShouldEqual, "src: 192.168.1.100:8080, dst: 10.0.0.1:443, seq: 4294967295")
		})

		Convey("Should handle loopback addresses", func() {
			packet := newMockTCPPacketWithOptions(
				withSrcAddr(net.ParseIP("127.0.0.1")),
				withDstAddr(net.ParseIP("127.0.0.1")),
				withSrcPort(8080),
				withDstPort(8080),
			)

			result := tcpPacketSeq(packet)

			So(result, ShouldEqual, "src: 127.0.0.1:8080, dst: 127.0.0.1:8080, seq: 1234567890")
		})

		Convey("Should handle well-known ports", func() {
			packet := newMockTCPPacketWithOptions(
				withSrcPort(80),  // HTTP 端口
				withDstPort(443), // HTTPS 端口
			)

			result := tcpPacketSeq(packet)

			So(result, ShouldEqual, "src: 192.168.1.100:80, dst: 10.0.0.1:443, seq: 1234567890")
		})
	})
}
