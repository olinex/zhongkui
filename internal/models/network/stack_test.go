package network

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// TestNetworkStack 测试 NetworkStack 结构体
// 验证 NetworkStack 各字段的正确性
func TestNetworkStack(t *testing.T) {
	Convey("Test NetworkStack structure", t, func() {
		Convey("Should initialize with all zero values", func() {
			stack := NetworkStack{}
			So(stack.Seq, ShouldEqual, uint32(0))
			So(stack.Level, ShouldEqual, LayerLevelDecodeFailure)
			So(stack.TopProto, ShouldEqual, gopacket.LayerTypeZero)
			So(stack.Proto, ShouldEqual, gopacket.LayerTypeZero)
			So(stack.Headers, ShouldEqual, "")
			So(stack.TraceID, ShouldResemble, serializer.HexUUID{})
		})

		Convey("Should set all fields correctly", func() {
			traceID := serializer.NewHexUUIDv4()
			stack := NetworkStack{
				TopProto: layers.LayerTypeTCP,
				Proto:    layers.LayerTypeIPv4,
				Seq:      1,
				Level:    LayerLevelNetwork,
				Headers:  `{"src":"192.168.1.1","dst":"10.0.0.1"}`,
			}
			stack.TraceID = traceID

			So(stack.TopProto, ShouldEqual, layers.LayerTypeTCP)
			So(stack.Proto, ShouldEqual, layers.LayerTypeIPv4)
			So(stack.Seq, ShouldEqual, uint32(1))
			So(stack.Level, ShouldEqual, LayerLevelNetwork)
			So(stack.Headers, ShouldEqual, `{"src":"192.168.1.1","dst":"10.0.0.1"}`)
		})

		Convey("Should embed BaseMeta correctly", func() {
			stack := NetworkStack{
				BaseMeta: BaseMeta{
					Namespace: "test-ns",
					Capture:   "test-cap",
					Version:   "v1",
				},
			}
			So(stack.Namespace, ShouldEqual, "test-ns")
			So(stack.Capture, ShouldEqual, "test-cap")
			So(stack.Version, ShouldEqual, "v1")
		})

		Convey("Should embed DirectionMeta correctly", func() {
			stack := NetworkStack{
				DirectionMeta: DirectionMeta{
					Cli2Srv: true,
				},
			}
			So(stack.Cli2Srv, ShouldBeTrue)
		})
	})
}

// TestNetworkFrame 测试 NetworkFrame 类型
func TestNetworkFrame(t *testing.T) {
	Convey("Test NetworkFrame type", t, func() {
		Convey("Should allow appending NetworkStack entries", func() {
			frame := make(NetworkFrame, 0, 2)
			So(len(frame), ShouldEqual, 0)

			frame = append(frame, &NetworkStack{Seq: 0, Level: LayerLevelLink})
			frame = append(frame, &NetworkStack{Seq: 1, Level: LayerLevelNetwork})

			So(len(frame), ShouldEqual, 2)
			So(frame[0].Level, ShouldEqual, LayerLevelLink)
			So(frame[1].Level, ShouldEqual, LayerLevelNetwork)
		})
	})
}

// TestNewNetworkFrame 测试 NewNetworkFrame 展平逻辑
// 验证一个数据包的 LayerStack 被平铺为多条 NetworkStack 记录：
// 所有记录共享相同的 TopProto，但 Proto/Level/Headers 各不相同。
func TestNewNetworkFrame(t *testing.T) {
	Convey("Test NewNetworkFrame flatten logic", t, func() {
		Convey("Should flatten Ethernet/IPv4/TCP stack into three records", func() {
			ethHeaders := &EthernetHeaders{
				EthDstMAC: serializer.MacAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF},
				EthSrcMAC: serializer.MacAddr{0x11, 0x22, 0x33, 0x44, 0x55, 0x66},
				EthType:   layers.EthernetTypeIPv4,
			}
			ipv4Headers := &IPv4Headers{
				IPv4Version:    4,
				IPv4TTL:        64,
				IPv4Src:        serializer.IPv4Addr(net.ParseIP("192.168.1.1").To4()),
				IPv4Dst:        serializer.IPv4Addr(net.ParseIP("10.0.0.1").To4()),
				IPv4IPProtocol: layers.IPProtocolTCP,
			}
			tcpHeaders := &TCPHeaders{
				TCPSrcPort: 50000,
				TCPDstPort: 80,
				TCPSeq:     1000,
			}
			stack := LayerStack{
				{Level: LayerLevelLink, Proto: layers.LayerTypeEthernet, Headers: ethHeaders},
				{Level: LayerLevelNetwork, Proto: layers.LayerTypeIPv4, Headers: ipv4Headers},
				{Level: LayerLevelTransport, Proto: layers.LayerTypeTCP, Headers: tcpHeaders},
			}
			meta := BaseMeta{
				Namespace: "test-ns",
				Capture:   "test-cap",
				Version:   "1.0",
			}
			timestamp := time.Now()

			frame := NewNetworkFrame(stack, meta, timestamp)

			So(len(frame), ShouldEqual, 3)

			// 所有记录共享相同的 TopProto（协议栈最高层）
			So(frame[0].TopProto, ShouldEqual, layers.LayerTypeTCP)
			So(frame[1].TopProto, ShouldEqual, layers.LayerTypeTCP)
			So(frame[2].TopProto, ShouldEqual, layers.LayerTypeTCP)

			// 每条记录 Proto/Level 各不相同，Headers 平铺存储
			So(frame[0].Proto, ShouldEqual, layers.LayerTypeEthernet)
			So(frame[0].Level, ShouldEqual, LayerLevelLink)
			ethJSON, err := json.Marshal(ethHeaders)
			So(err, ShouldBeNil)
			So(frame[0].Headers, ShouldEqual, string(ethJSON))

			So(frame[1].Proto, ShouldEqual, layers.LayerTypeIPv4)
			So(frame[1].Level, ShouldEqual, LayerLevelNetwork)
			ipv4JSON, err := json.Marshal(ipv4Headers)
			So(err, ShouldBeNil)
			So(frame[1].Headers, ShouldEqual, string(ipv4JSON))

			So(frame[2].Proto, ShouldEqual, layers.LayerTypeTCP)
			So(frame[2].Level, ShouldEqual, LayerLevelTransport)
			tcpJSON, err := json.Marshal(tcpHeaders)
			So(err, ShouldBeNil)
			So(frame[2].Headers, ShouldEqual, string(tcpJSON))

			// 元数据与时间戳正确复制
			So(frame[0].Namespace, ShouldEqual, "test-ns")
			So(frame[1].Capture, ShouldEqual, "test-cap")
			So(frame[2].Version, ShouldEqual, "1.0")
			So(frame[0].Timestamp, ShouldResemble, timestamp)
			So(frame[0].CreatedAt.IsZero(), ShouldBeFalse)

			// Seq/TraceID/Cli2Srv 由调用方补充设置，此处保持零值
			So(frame[0].Seq, ShouldEqual, uint32(0))
			So(frame[0].TraceID, ShouldResemble, serializer.HexUUID{})
			So(frame[0].Cli2Srv, ShouldBeFalse)
		})

		Convey("Should return empty frame for empty stack", func() {
			frame := NewNetworkFrame(nil, BaseMeta{}, time.Now())
			So(len(frame), ShouldEqual, 0)
		})

		Convey("Should tolerate nil Headers", func() {
			stack := LayerStack{
				{Level: LayerLevelNetwork, Proto: layers.LayerTypeIPv4, Headers: nil},
			}
			frame := NewNetworkFrame(stack, BaseMeta{}, time.Now())
			So(len(frame), ShouldEqual, 1)
			So(frame[0].Headers, ShouldEqual, "")
			So(frame[0].TopProto, ShouldEqual, layers.LayerTypeIPv4)
		})
	})
}
