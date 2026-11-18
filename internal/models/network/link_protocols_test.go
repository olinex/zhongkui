package network

import (
	"encoding/binary"
	"net"
	"strconv"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	. "github.com/smartystreets/goconvey/convey"
	reporter_v1 "zhongkuisec.com/zhongkui/internal/pkg/protobuf/reporter/v1"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// testSLLAddr 测试用 Linux SLL 链路地址
var testSLLAddr = net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}

// serializeIPv4TCP 序列化 IPv4 + TCP + 载荷，供各链路前缀拼接复用
func serializeIPv4TCP(t *testing.T) []byte {
	t.Helper()

	ip := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: layers.IPProtocolTCP,
		SrcIP:    net.IPv4(192, 0, 2, 10).To4(),
		DstIP:    net.IPv4(198, 51, 100, 20).To4(),
	}
	tcp := &layers.TCP{
		SrcPort: 49152,
		DstPort: 8080,
		Seq:     1000,
		SYN:     true,
		Window:  65535,
	}
	if err := tcp.SetNetworkLayerForChecksum(ip); err != nil {
		t.Fatalf("设置 TCP 校验和网络层失败: %v", err)
	}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	if err := gopacket.SerializeLayers(buf, opts, ip, tcp, gopacket.Payload([]byte("payload"))); err != nil {
		t.Fatalf("序列化 IPv4/TCP 失败: %v", err)
	}
	return buf.Bytes()
}

// buildSLLFrame 构造 Linux SLL 头（大端）并拼接内层数据
func buildSLLFrame(packetType layers.LinuxSLLPacketType, addrType uint16, addr net.HardwareAddr, ethernetType layers.EthernetType, inner []byte) []byte {
	header := make([]byte, 16)
	binary.BigEndian.PutUint16(header[0:2], uint16(packetType))
	binary.BigEndian.PutUint16(header[2:4], addrType)
	binary.BigEndian.PutUint16(header[4:6], uint16(len(addr)))
	copy(header[6:14], addr)
	binary.BigEndian.PutUint16(header[14:16], uint16(ethernetType))
	return append(header, inner...)
}

// buildSLL2Frame 构造 Linux SLL2 头（大端）并拼接内层数据
func buildSLL2Frame(protocolType layers.EthernetType, interfaceIndex uint32, arpHardwareType layers.ARPHardwareType, packetType layers.LinuxSLL2PacketType, addr net.HardwareAddr, inner []byte) []byte {
	header := make([]byte, 20)
	binary.BigEndian.PutUint16(header[0:2], uint16(protocolType))
	binary.BigEndian.PutUint32(header[4:8], interfaceIndex)
	binary.BigEndian.PutUint16(header[8:10], uint16(arpHardwareType))
	header[10] = byte(packetType)
	header[11] = byte(len(addr))
	copy(header[12:20], addr)
	return append(header, inner...)
}

// newLinkModelBuilder 按链路类型构建模型构建器
func newLinkModelBuilder(data []byte, decodeType, linkType layers.LinkType) *ModelBuilder {
	packet := gopacket.NewPacket(data, decodeType, gopacket.Default)
	return NewPacketModelBuilderWithLinkType(packet, serializer.NewHexUUIDv4(), reporter_v1.ResourceRaw{
		Namespace: "link-namespace",
		Name:      "link-capturer",
		Version:   "v1.0.0",
	}, linkType)
}

// TestPacketLinuxSLLSingleFrame 验证 SLL 单包在内层协议无法解析时停留在链路层并映射槽位
func TestPacketLinuxSLLSingleFrame(t *testing.T) {
	Convey("SLL 单包应产出 PacketLinuxSLL", t, func() {
		data := buildSLLFrame(layers.LinuxSLLPacketTypeHost, 1, testSLLAddr, layers.EthernetType(0x88b5), []byte("unknown-inner"))
		builder := newLinkModelBuilder(data, layers.LinkTypeLinuxSLL, layers.LinkTypeLinuxSLL)

		So(builder.Level, ShouldEqual, LayerLevelLink)
		model, ok := builder.BuildAsPacketModel().(*PacketLinuxSLL)
		So(ok, ShouldBeTrue)

		So(model.TableName(), ShouldEqual, "packet_linux_slls")
		So(model.LinkType, ShouldEqual, layers.LinkTypeLinuxSLL)
		So(model.SLLPacketType, ShouldEqual, layers.LinuxSLLPacketTypeHost)
		So(model.SLLAddrLen, ShouldEqual, 6)
		So(model.SLLAddr, ShouldEqual, "aa:bb:cc:dd:ee:ff")
		So(model.SLLAddrType, ShouldEqual, 1)
		So(model.SLLProtocol, ShouldEqual, layers.EthernetType(0x88b5))

		slots := model.LinkSlotValues()
		So(slots.Link1, ShouldEqual, "0")
		So(slots.Link2, ShouldEqual, "6")
		So(slots.Link3, ShouldEqual, "aa:bb:cc:dd:ee:ff")
		So(slots.Link4, ShouldEqual, "1")
		So(slots.Link5, ShouldEqual, "34997")
		So(slots.Link6, ShouldBeEmpty)
	})
}

// TestPacketLinuxSLL2SingleFrame 验证 SLL2 单包在内层协议无法解析时停留在链路层并映射槽位
func TestPacketLinuxSLL2SingleFrame(t *testing.T) {
	Convey("SLL2 单包应产出 PacketLinuxSLL2", t, func() {
		data := buildSLL2Frame(layers.EthernetType(0x88b5), 3, layers.ARPHardwareTypeEthernet, layers.LinuxSLL2PacketTypeHost, testSLLAddr, []byte("unknown-inner"))
		builder := newLinkModelBuilder(data, layers.LinkTypeLinuxSLL2, layers.LinkTypeLinuxSLL2)

		So(builder.Level, ShouldEqual, LayerLevelLink)
		model, ok := builder.BuildAsPacketModel().(*PacketLinuxSLL2)
		So(ok, ShouldBeTrue)

		So(model.TableName(), ShouldEqual, "packet_linux_sll2s")
		So(model.LinkType, ShouldEqual, layers.LinkTypeLinuxSLL2)
		So(model.SLL2ProtocolType, ShouldEqual, layers.EthernetType(0x88b5))
		So(model.SLL2InterfaceIndex, ShouldEqual, 3)
		So(model.SLL2ARPHardwareType, ShouldEqual, layers.ARPHardwareTypeEthernet)
		So(model.SLL2PacketType, ShouldEqual, layers.LinuxSLL2PacketTypeHost)
		So(model.SLL2AddrLength, ShouldEqual, 6)
		So(model.SLL2Addr, ShouldEqual, "aa:bb:cc:dd:ee:ff")

		slots := model.LinkSlotValues()
		So(slots.Link1, ShouldEqual, "34997")
		So(slots.Link2, ShouldEqual, "3")
		So(slots.Link3, ShouldEqual, "1")
		So(slots.Link4, ShouldEqual, "0")
		So(slots.Link5, ShouldEqual, "6")
		So(slots.Link6, ShouldEqual, "aa:bb:cc:dd:ee:ff")
		So(slots.Link7, ShouldBeEmpty)
	})
}

// TestPacketPPPSingleFrame 验证 PPP 单包在内层协议无法解析时停留在链路层并映射槽位
func TestPacketPPPSingleFrame(t *testing.T) {
	Convey("PPP 单包应产出 PacketPPP", t, func() {
		data := []byte{0x12, 0x35, 'u', 'n', 'k', 'n', 'o', 'w', 'n'}
		builder := newLinkModelBuilder(data, layers.LinkTypePPP, layers.LinkTypePPP)

		So(builder.Level, ShouldEqual, LayerLevelLink)
		model, ok := builder.BuildAsPacketModel().(*PacketPPP)
		So(ok, ShouldBeTrue)

		So(model.TableName(), ShouldEqual, "packet_ppps")
		So(model.LinkType, ShouldEqual, layers.LinkTypePPP)
		So(model.PPPType, ShouldEqual, layers.PPPType(0x1235))
		So(model.PPPHasPPTPHeader, ShouldBeFalse)

		slots := model.LinkSlotValues()
		So(slots.Link1, ShouldEqual, "4661")
		So(slots.Link2, ShouldEqual, "false")
		So(slots.Link3, ShouldBeEmpty)
	})
}

// TestLinkLayersCarryIPv4TCP 验证 SLL/SLL2/PPP 承载 IPv4 + TCP 时产出上层模型并写入链路槽位
func TestLinkLayersCarryIPv4TCP(t *testing.T) {
	ipTCP := serializeIPv4TCP(t)

	cases := []struct {
		name     string
		data     []byte
		linkType layers.LinkType
		slots    []string
	}{
		{
			name:     "Linux SLL",
			data:     buildSLLFrame(layers.LinuxSLLPacketTypeHost, 1, testSLLAddr, layers.EthernetTypeIPv4, ipTCP),
			linkType: layers.LinkTypeLinuxSLL,
			slots:    []string{"0", "6", "aa:bb:cc:dd:ee:ff", "1", "2048"},
		},
		{
			name:     "Linux SLL2",
			data:     buildSLL2Frame(layers.EthernetTypeIPv4, 3, layers.ARPHardwareTypeEthernet, layers.LinuxSLL2PacketTypeHost, testSLLAddr, ipTCP),
			linkType: layers.LinkTypeLinuxSLL2,
			slots:    []string{"2048", "3", "1", "0", "6", "aa:bb:cc:dd:ee:ff"},
		},
		{
			name:     "PPP",
			data:     pppIPv4TCP(t),
			linkType: layers.LinkTypePPP,
			slots:    []string{"33", "false"},
		},
	}

	for _, tc := range cases {
		Convey(tc.name+" 承载 IPv4/TCP 应产出 PacketIPv4TCP", t, func() {
			builder := newLinkModelBuilder(tc.data, tc.linkType, tc.linkType)
			So(builder.Level, ShouldEqual, LayerLevelTransport)

			model, ok := builder.BuildAsPacketModel().(*PacketIPv4TCP)
			So(ok, ShouldBeTrue)
			So(model.LinkType, ShouldEqual, tc.linkType)
			So(model.IPv4Src.String(), ShouldEqual, "192.0.2.10")
			So(model.IPv4Dst.String(), ShouldEqual, "198.51.100.20")
			So(model.IPv4IPProtocol, ShouldEqual, layers.IPProtocolTCP)
			So(model.TCPSrcPort, ShouldEqual, layers.TCPPort(49152))
			So(model.TCPDstPort, ShouldEqual, layers.TCPPort(8080))
			So(model.TCPBitSYN, ShouldBeTrue)
			So(model.TCPStreamID, ShouldNotBeEmpty)

			slots := []string{
				model.Link1, model.Link2, model.Link3, model.Link4,
				model.Link5, model.Link6,
			}
			expected := append(append([]string{}, tc.slots...), make([]string, 6-len(tc.slots))...)
			So(slots, ShouldResemble, expected)
			So(model.Link7, ShouldBeEmpty)
		})
	}
}

// pppIPv4TCP 序列化 PPP + IPv4 + TCP + 载荷
func pppIPv4TCP(t *testing.T) []byte {
	t.Helper()

	ip := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: layers.IPProtocolTCP,
		SrcIP:    net.IPv4(192, 0, 2, 10).To4(),
		DstIP:    net.IPv4(198, 51, 100, 20).To4(),
	}
	tcp := &layers.TCP{
		SrcPort: 49152,
		DstPort: 8080,
		Seq:     1000,
		SYN:     true,
		Window:  65535,
	}
	if err := tcp.SetNetworkLayerForChecksum(ip); err != nil {
		t.Fatalf("设置 TCP 校验和网络层失败: %v", err)
	}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	if err := gopacket.SerializeLayers(buf, opts, &layers.PPP{PPPType: layers.PPPTypeIPv4}, ip, tcp, gopacket.Payload([]byte("payload"))); err != nil {
		t.Fatalf("序列化 PPP/IPv4/TCP 失败: %v", err)
	}
	return buf.Bytes()
}

// testEthSrc 测试用以太网源 MAC
var testEthSrc = net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}

// testEthDst 测试用以太网目的 MAC
var testEthDst = net.HardwareAddr{0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb}

// testDot11Addr1 测试用 802.11 接收方地址
var testDot11Addr1 = net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x00, 0x01}

// testDot11Addr2 测试用 802.11 发送方地址
var testDot11Addr2 = net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x00, 0x02}

// testDot11Addr3 测试用 802.11 第三个地址
var testDot11Addr3 = net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x00, 0x03}

// buildVLANFrame 构造以太网 + 一层或多层 VLAN 标签 + 内层载荷
// 最后一个标签的内层类型固定为 IPv4，支持构造 QinQ 双层标签
func buildVLANFrame(t *testing.T, tags []*layers.Dot1Q, inner []byte) []byte {
	t.Helper()

	eth := &layers.Ethernet{
		SrcMAC:       testEthSrc,
		DstMAC:       testEthDst,
		EthernetType: layers.EthernetTypeDot1Q,
	}
	serializable := make([]gopacket.SerializableLayer, 0, len(tags)+2)
	serializable = append(serializable, eth)
	for i, tag := range tags {
		if i == len(tags)-1 {
			tag.Type = layers.EthernetTypeIPv4
		}
		serializable = append(serializable, tag)
	}
	serializable = append(serializable, gopacket.Payload(inner))

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	if err := gopacket.SerializeLayers(buf, opts, serializable...); err != nil {
		t.Fatalf("序列化 VLAN 帧失败: %v", err)
	}
	return buf.Bytes()
}

// buildPPPoEFrame 构造以太网 + PPPoE + 可选 PPP + 内层载荷
func buildPPPoEFrame(t *testing.T, code layers.PPPoECode, ppp *layers.PPP, inner []byte) []byte {
	t.Helper()

	etherType := layers.EthernetTypePPPoESession
	if code != layers.PPPoECodeSession {
		etherType = layers.EthernetTypePPPoEDiscovery
	}
	eth := &layers.Ethernet{
		SrcMAC:       testEthSrc,
		DstMAC:       testEthDst,
		EthernetType: etherType,
	}
	pppoe := &layers.PPPoE{
		Version:   1,
		Type:      1,
		Code:      code,
		SessionId: 0x1234,
	}
	serializable := []gopacket.SerializableLayer{eth, pppoe}
	if ppp != nil {
		serializable = append(serializable, ppp)
	}
	serializable = append(serializable, gopacket.Payload(inner))

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	if err := gopacket.SerializeLayers(buf, opts, serializable...); err != nil {
		t.Fatalf("序列化 PPPoE 帧失败: %v", err)
	}
	return buf.Bytes()
}

// buildDot11Frame 构造最小 802.11 帧（24 字节帧头）并拼接载荷
// frameControl 的高 6 位表示类型与子类型，flags 为帧控制第二字节
func buildDot11Frame(frameControl byte, flags byte, addr1, addr2, addr3 net.HardwareAddr, payload []byte) []byte {
	frame := make([]byte, 0, 24+len(payload))
	frame = append(frame, frameControl, flags)
	frame = append(frame, 0x00, 0x00) // Duration
	frame = append(frame, addr1...)
	frame = append(frame, addr2...)
	frame = append(frame, addr3...)
	frame = append(frame, 0x00, 0x00) // Sequence Control
	return append(frame, payload...)
}

// buildLLCSNAP 构造 LLC + SNAP 头部，内层类型固定为 IPv4
func buildLLCSNAP() []byte {
	return []byte{
		0xaa, 0xaa, 0x03, // LLC：DSAP、SSAP、Control
		0x00, 0x00, 0x00, // SNAP：组织代码
		0x08, 0x00, // SNAP：IPv4
	}
}

// buildRadioTapPrefix 构造 8 字节最小 RadioTap 头（version=0、length=8、无 present 字段）
func buildRadioTapPrefix() []byte {
	return []byte{0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00}
}

// buildPrismPrefix 构造 24 字节最小 Prism 头（type1 消息码、无扩展值）
func buildPrismPrefix() []byte {
	header := make([]byte, 24)
	binary.LittleEndian.PutUint16(header[0:2], uint16(layers.PrismType1MessageCode))
	binary.LittleEndian.PutUint16(header[4:6], 24)
	copy(header[8:24], []byte("prism-test-device"))
	return header
}

// findLayerInfo 返回层堆栈中首个指定协议类型的层信息
func findLayerInfo(stack LayerStack, proto gopacket.LayerType) *LayerInfo {
	for _, info := range stack {
		if info != nil && info.Proto == proto {
			return info
		}
	}
	return nil
}

// TestVLANCarriesIPv4TCP 验证单层 802.1Q 标签可继续解析到 TCP，且以太网槽位语义不变
func TestVLANCarriesIPv4TCP(t *testing.T) {
	Convey("单层 VLAN 应产出 PacketIPv4TCP", t, func() {
		tag := &layers.Dot1Q{Priority: 3, DropEligible: true, VLANIdentifier: 100}
		data := buildVLANFrame(t, []*layers.Dot1Q{tag}, serializeIPv4TCP(t))
		builder := newLinkModelBuilder(data, layers.LinkTypeEthernet, layers.LinkTypeEthernet)

		So(builder.Level, ShouldEqual, LayerLevelTransport)
		info := findLayerInfo(builder.Stack, layers.LayerTypeDot1Q)
		So(info, ShouldNotBeNil)
		So(info.Level, ShouldEqual, LayerLevelLink)
		headers, ok := info.Headers.(*Dot1QHeaders)
		So(ok, ShouldBeTrue)
		So(headers.Priority, ShouldEqual, 3)
		So(headers.DropEligible, ShouldBeTrue)
		So(headers.VLANIdentifier, ShouldEqual, 100)
		So(headers.Type, ShouldEqual, layers.EthernetTypeIPv4)

		model, ok := builder.BuildAsPacketModel().(*PacketIPv4TCP)
		So(ok, ShouldBeTrue)
		So(model.Link1, ShouldEqual, testEthDst.String())
		So(model.Link2, ShouldEqual, testEthSrc.String())
		So(model.Link3, ShouldEqual, "33024")
		So(model.Link4, ShouldEqual, "0")
	})
}

// TestQinQCarriesIPv4TCP 验证双层 VLAN 标签（QinQ）可继续解析到 TCP
func TestQinQCarriesIPv4TCP(t *testing.T) {
	Convey("QinQ 应产出 PacketIPv4TCP", t, func() {
		outer := &layers.Dot1Q{VLANIdentifier: 100, Type: layers.EthernetTypeQinQ}
		inner := &layers.Dot1Q{VLANIdentifier: 200}
		data := buildVLANFrame(t, []*layers.Dot1Q{outer, inner}, serializeIPv4TCP(t))
		builder := newLinkModelBuilder(data, layers.LinkTypeEthernet, layers.LinkTypeEthernet)

		So(builder.Level, ShouldEqual, LayerLevelTransport)
		count := 0
		for _, info := range builder.Stack {
			if info.Proto == layers.LayerTypeDot1Q {
				count++
			}
		}
		So(count, ShouldEqual, 2)
		So(findLayerInfo(builder.Stack, layers.LayerTypeDot1Q).Level, ShouldEqual, LayerLevelLink)

		model, ok := builder.BuildAsPacketModel().(*PacketIPv4TCP)
		So(ok, ShouldBeTrue)
		So(model.TCPDstPort, ShouldEqual, layers.TCPPort(8080))
	})
}

// TestPPPoECarriesIPv4TCP 验证 PPPoE 会话封装可继续解析到 TCP
func TestPPPoECarriesIPv4TCP(t *testing.T) {
	Convey("PPPoE 会话应产出 PacketIPv4TCP", t, func() {
		ppp := &layers.PPP{PPPType: layers.PPPTypeIPv4}
		data := buildPPPoEFrame(t, layers.PPPoECodeSession, ppp, serializeIPv4TCP(t))
		builder := newLinkModelBuilder(data, layers.LinkTypeEthernet, layers.LinkTypeEthernet)

		So(builder.Level, ShouldEqual, LayerLevelTransport)
		info := findLayerInfo(builder.Stack, layers.LayerTypePPPoE)
		So(info, ShouldNotBeNil)
		So(info.Level, ShouldEqual, LayerLevelLink)
		headers, ok := info.Headers.(*PPPoEHeaders)
		So(ok, ShouldBeTrue)
		So(headers.SessionID, ShouldEqual, 0x1234)
		So(headers.Code, ShouldEqual, layers.PPPoECodeSession)
		So(findLayerInfo(builder.Stack, layers.LayerTypePPP), ShouldNotBeNil)

		model, ok := builder.BuildAsPacketModel().(*PacketIPv4TCP)
		So(ok, ShouldBeTrue)
		So(model.IPv4Src.String(), ShouldEqual, "192.0.2.10")
	})
}

// TestPPPoEDiscoveryStopsAtLink 验证 PPPoE 发现报文停留在链路层并保留剩余载荷
func TestPPPoEDiscoveryStopsAtLink(t *testing.T) {
	Convey("PPPoE 发现报文应停留在链路层", t, func() {
		payload := []byte("discovery-tags")
		data := buildPPPoEFrame(t, layers.PPPoECodePADI, nil, payload)
		builder := newLinkModelBuilder(data, layers.LinkTypeEthernet, layers.LinkTypeEthernet)

		So(builder.Level, ShouldEqual, LayerLevelLink)
		info := findLayerInfo(builder.Stack, layers.LayerTypePPPoE)
		So(info, ShouldNotBeNil)
		headers, ok := info.Headers.(*PPPoEHeaders)
		So(ok, ShouldBeTrue)
		So(headers.Code, ShouldEqual, layers.PPPoECodePADI)
		So(string(builder.Payload), ShouldEqual, string(payload))

		_, ok = builder.BuildAsPacketModel().(*PacketEthernet)
		So(ok, ShouldBeTrue)
	})
}

// TestRadioTapCarriesIPv4TCP 验证 RadioTap + Dot11 + LLC/SNAP 可继续解析到 TCP
func TestRadioTapCarriesIPv4TCP(t *testing.T) {
	Convey("RadioTap 无线帧应产出 PacketIPv4TCP", t, func() {
		llcSnap := buildLLCSNAP()
		dot11Payload := append(llcSnap, serializeIPv4TCP(t)...)
		dot11 := buildDot11Frame(0x08, 0x00, testDot11Addr1, testDot11Addr2, testDot11Addr3, dot11Payload)
		data := append(buildRadioTapPrefix(), dot11...)

		builder := newLinkModelBuilder(data, layers.LinkTypeIEEE80211Radio, layers.LinkTypeIEEE80211Radio)

		So(builder.Level, ShouldEqual, LayerLevelTransport)
		So(findLayerInfo(builder.Stack, layers.LayerTypeRadioTap).Level, ShouldEqual, LayerLevelLink)
		So(findLayerInfo(builder.Stack, layers.LayerTypeDot11).Level, ShouldEqual, LayerLevelLink)
		So(findLayerInfo(builder.Stack, layers.LayerTypeDot11Data), ShouldNotBeNil)
		So(findLayerInfo(builder.Stack, layers.LayerTypeLLC).Level, ShouldEqual, LayerLevelLink)
		So(findLayerInfo(builder.Stack, layers.LayerTypeSNAP).Level, ShouldEqual, LayerLevelLink)

		model, ok := builder.BuildAsPacketModel().(*PacketIPv4TCP)
		So(ok, ShouldBeTrue)
		So(model.Link1, ShouldEqual, testDot11Addr1.String())
		So(model.Link2, ShouldEqual, testDot11Addr2.String())
		So(model.Link3, ShouldEqual, testDot11Addr3.String())
		So(model.Link5, ShouldEqual, "2")
		So(model.Link6, ShouldEqual, "0")
		So(model.TCPDstPort, ShouldEqual, layers.TCPPort(8080))
	})
}

// TestRawDot11CarriesIPv4TCP 验证无 RadioTap 的 802.11 抓包可继续解析到 TCP
func TestRawDot11CarriesIPv4TCP(t *testing.T) {
	Convey("裸 802.11 帧应产出 PacketIPv4TCP", t, func() {
		dot11Payload := append(buildLLCSNAP(), serializeIPv4TCP(t)...)
		data := buildDot11Frame(0x08, 0x00, testDot11Addr1, testDot11Addr2, testDot11Addr3, dot11Payload)

		builder := newLinkModelBuilder(data, layers.LinkTypeIEEE802_11, layers.LinkTypeIEEE802_11)
		So(builder.Level, ShouldEqual, LayerLevelTransport)

		model, ok := builder.BuildAsPacketModel().(*PacketIPv4TCP)
		So(ok, ShouldBeTrue)
		So(model.Link1, ShouldEqual, testDot11Addr1.String())
		So(model.Link2, ShouldEqual, testDot11Addr2.String())
	})
}

// TestDot11DecryptedPayloadWithProtectedBit 验证 Protected 位保留但载荷已解密时仍可解析到 TCP
func TestDot11DecryptedPayloadWithProtectedBit(t *testing.T) {
	Convey("已解密但保留 Protected 位的 802.11 帧应产出 PacketIPv4TCP", t, func() {
		dot11Payload := append(buildLLCSNAP(), serializeIPv4TCP(t)...)
		// 帧控制第二字节 0x40 为 Protected 位，模拟解密后未清除该位的抓包
		dot11 := buildDot11Frame(0x08, 0x40, testDot11Addr1, testDot11Addr2, testDot11Addr3, dot11Payload)

		cases := []struct {
			name     string
			data     []byte
			linkType layers.LinkType
		}{
			{
				name:     "裸 802.11",
				data:     dot11,
				linkType: layers.LinkTypeIEEE802_11,
			},
			{
				name:     "RadioTap",
				data:     append(buildRadioTapPrefix(), dot11...),
				linkType: layers.LinkTypeIEEE80211Radio,
			},
		}

		for _, tc := range cases {
			Convey(tc.name+" 应绕过 Dot11WEP 继续解析", func() {
				builder := newLinkModelBuilder(tc.data, tc.linkType, tc.linkType)
				So(builder.Level, ShouldEqual, LayerLevelTransport)
				So(findLayerInfo(builder.Stack, layers.LayerTypeDot11WEP), ShouldBeNil)

				model, ok := builder.BuildAsPacketModel().(*PacketIPv4TCP)
				So(ok, ShouldBeTrue)
				So(model.Link1, ShouldEqual, testDot11Addr1.String())
				So(model.TCPDstPort, ShouldEqual, layers.TCPPort(8080))
			})
		}
	})
}

// TestDot11EncryptedPayloadStopsAtLink 验证真正的加密载荷仍停留在链路层并产出 PacketDot11
func TestDot11EncryptedPayloadStopsAtLink(t *testing.T) {
	Convey("加密的 802.11 帧应停留在链路层", t, func() {
		// 非 LLC/SNAP 前缀的随机密文，模拟未解密的载荷
		encrypted := []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x10, 0x32, 0x54, 0x76}
		data := buildDot11Frame(0x08, 0x40, testDot11Addr1, testDot11Addr2, testDot11Addr3, encrypted)

		builder := newLinkModelBuilder(data, layers.LinkTypeIEEE802_11, layers.LinkTypeIEEE802_11)
		So(builder.Level, ShouldEqual, LayerLevelLink)

		model, ok := builder.BuildAsPacketModel().(*PacketDot11)
		So(ok, ShouldBeTrue)
		So(model.Dot11Address1, ShouldEqual, testDot11Addr1.String())
	})
}

// TestDot11BeaconBuildsLinkModel 验证 802.11 管理帧停留在链路层并产出 PacketDot11
func TestDot11BeaconBuildsLinkModel(t *testing.T) {
	Convey("802.11 Beacon 应产出 PacketDot11", t, func() {
		broadcast := net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
		body := make([]byte, 12)
		data := buildDot11Frame(0x80, 0x00, broadcast, testDot11Addr2, testDot11Addr3, body)

		builder := newLinkModelBuilder(data, layers.LinkTypeIEEE802_11, layers.LinkTypeIEEE802_11)
		So(builder.Level, ShouldEqual, LayerLevelLink)

		model, ok := builder.BuildAsPacketModel().(*PacketDot11)
		So(ok, ShouldBeTrue)
		So(model.Dot11Type, ShouldEqual, layers.Dot11TypeMgmtBeacon)
		So(model.Dot11Address1, ShouldEqual, broadcast.String())
		So(model.Dot11Address2, ShouldEqual, testDot11Addr2.String())
		slots := model.LinkSlotValues()
		So(slots.Link1, ShouldEqual, broadcast.String())
		So(slots.Link5, ShouldEqual, strconv.FormatUint(uint64(layers.Dot11TypeMgmtBeacon), 10))
	})
}

// TestPrismCarriesDot11Beacon 验证 Prism 首层抓包可解析出 Dot11 管理帧模型
func TestPrismCarriesDot11Beacon(t *testing.T) {
	Convey("Prism 抓包应产出 PacketDot11", t, func() {
		broadcast := net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
		body := make([]byte, 12)
		dot11 := buildDot11Frame(0x80, 0x00, broadcast, testDot11Addr2, testDot11Addr3, body)
		data := append(buildPrismPrefix(), dot11...)

		builder := newLinkModelBuilder(data, layers.LinkTypePrismHeader, layers.LinkTypePrismHeader)
		So(builder.Level, ShouldEqual, LayerLevelLink)

		model, ok := builder.BuildAsPacketModel().(*PacketDot11)
		So(ok, ShouldBeTrue)
		So(model.Dot11Type, ShouldEqual, layers.Dot11TypeMgmtBeacon)
		So(model.Dot11Address1, ShouldEqual, broadcast.String())
	})
}

// TestRawIPCarriesIPv4TCP 验证 Raw IP 链路从网络层开始建模且链路槽位为空
func TestRawIPCarriesIPv4TCP(t *testing.T) {
	ipTCP := serializeIPv4TCP(t)

	for _, linkType := range []layers.LinkType{layers.LinkTypeRaw, layers.LinkTypeIPv4, layers.LinkTypeIPv6} {
		Convey("Raw IP 链路类型应产出 PacketIPv4TCP", t, func() {
			builder := newLinkModelBuilder(ipTCP, layers.LinkTypeRaw, linkType)
			So(builder.Level, ShouldEqual, LayerLevelTransport)
			So(firstLayerOfLevel(builder.Stack, LayerLevelLink), ShouldBeNil)

			model, ok := builder.BuildAsPacketModel().(*PacketIPv4TCP)
			So(ok, ShouldBeTrue)
			So(model.LinkType, ShouldEqual, linkType)
			So(model.IPv4Src.String(), ShouldEqual, "192.0.2.10")
			So(model.IPv4Dst.String(), ShouldEqual, "198.51.100.20")
			So(model.TCPSrcPort, ShouldEqual, layers.TCPPort(49152))
			So(model.TCPDstPort, ShouldEqual, layers.TCPPort(8080))

			slots := []string{
				model.Link1, model.Link2, model.Link3, model.Link4, model.Link5, model.Link6,
				model.Link7, model.Link8, model.Link9, model.Link10, model.Link11, model.Link12,
				model.Link13, model.Link14, model.Link15, model.Link16,
			}
			for _, slot := range slots {
				So(slot, ShouldBeEmpty)
			}
		})
	}
}
