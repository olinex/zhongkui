package network

import (
	"time"

	"github.com/gopacket/gopacket/layers"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// linkEthernetHeaders 提取链路层以太网头部（Ethernet 与 Loopback 共用）
func linkEthernetHeaders(stack LayerStack) (*EthernetHeaders, bool) {
	return firstHeaderOf[*EthernetHeaders](stack, LayerLevelLink)
}

// linkLinuxSLLHeaders 提取链路层 Linux SLL 头部
func linkLinuxSLLHeaders(stack LayerStack) (*LinuxSLLHeaders, bool) {
	return firstHeaderOf[*LinuxSLLHeaders](stack, LayerLevelLink)
}

// linkLinuxSLL2Headers 提取链路层 Linux SLL2 头部
func linkLinuxSLL2Headers(stack LayerStack) (*LinuxSLL2Headers, bool) {
	return firstHeaderOf[*LinuxSLL2Headers](stack, LayerLevelLink)
}

// linkPPPHeaders 提取链路层 PPP 头部
func linkPPPHeaders(stack LayerStack) (*PPPHeaders, bool) {
	return firstHeaderOf[*PPPHeaders](stack, LayerLevelLink)
}

// linkDot11Headers 提取链路层 Dot11 头部
func linkDot11Headers(stack LayerStack) (*Dot11Headers, bool) {
	return firstHeaderOf[*Dot11Headers](stack, LayerLevelLink)
}

// linkWirelessDot11Headers 为 RadioTap / Prism 首层提取 Dot11 头部
// 无线抓包元数据层本身没有独立模型，链路模型以随后的 Dot11 头部为准；
// 截断导致 Dot11 缺失时返回零值头部，避免整包无法建模
func linkWirelessDot11Headers(stack LayerStack) (*Dot11Headers, bool) {
	if headers, ok := linkDot11Headers(stack); ok {
		return headers, true
	}
	return &Dot11Headers{}, true
}

// networkIPv4Headers 提取网络层 IPv4 头部
func networkIPv4Headers(stack LayerStack) (*IPv4Headers, bool) {
	return firstHeaderOf[*IPv4Headers](stack, LayerLevelNetwork)
}

// networkIPv6Headers 提取网络层 IPv6 头部
func networkIPv6Headers(stack LayerStack) (*IPv6Headers, bool) {
	return firstHeaderOf[*IPv6Headers](stack, LayerLevelNetwork)
}

// transportTCPHeaders 提取传输层 TCP 头部
// 传输层头部以协议栈最后一层为准，与构建入口的层级校验口径一致
func transportTCPHeaders(stack LayerStack) (*TCPHeaders, bool) {
	return lastHeaderOf[*TCPHeaders](stack, LayerLevelTransport)
}

// transportUDPHeaders 提取传输层 UDP 头部
// 传输层头部以协议栈最后一层为准，与构建入口的层级校验口径一致
func transportUDPHeaders(stack LayerStack) (*UDPHeaders, bool) {
	return lastHeaderOf[*UDPHeaders](stack, LayerLevelTransport)
}

// newAsPacket 构造数据包公共元数据
// 集中填充基础元数据、链路类型、时间与捕获信息，供各链路与上层模型复用
func (b *ModelBuilder) newAsPacket() AsPacket {
	return AsPacket{
		BaseMeta: BaseMeta{
			SessionID: b.SessionID,
			Namespace: b.Source.Namespace,
			Capture:   b.Source.Name,
			Version:   b.Source.Version,
		},
		LinkMeta: LinkMeta{
			LinkType: b.LinkType,
		},
		TimestampMeta: TimestampMeta{
			Timestamp: b.Timestamp,
			CreatedAt: time.Now(),
		},
		CaptureMeta: CaptureMeta{
			Truncated:     b.Truncated,
			CaptureLength: uint32(b.CaptureLength),
		},
		PacketMeta: PacketMeta{
			LayerStack: b.Stack,
		},
	}
}

// newPacketLink 构造链路无关基座并生成行唯一 ID
// 行详情接口依赖非空的全局唯一 ID，构造时即生成，落库路径无需再补
func (b *ModelBuilder) newPacketLink() PacketLink {
	return PacketLink{
		AsPacket: b.newAsPacket(),
		RowMeta: RowMeta{
			ID: serializer.NewHexUUIDv4(),
		},
	}
}

// buildPacketEthernet 以链路层以太网头部装配 PacketEthernet
// Ethernet 与 Loopback 复用同一模型，二者仅头部构造不同
func buildPacketEthernet(b *ModelBuilder, headers *EthernetHeaders) any {
	return &PacketEthernet{
		PacketLink:      b.newPacketLink(),
		EthernetHeaders: *headers,
	}
}

// buildPacketLinuxSLL 以 Linux SLL 链路头部装配 PacketLinuxSLL
func buildPacketLinuxSLL(b *ModelBuilder, headers *LinuxSLLHeaders) any {
	return &PacketLinuxSLL{
		PacketLink:      b.newPacketLink(),
		LinuxSLLHeaders: *headers,
	}
}

// buildPacketLinuxSLL2 以 Linux SLL2 链路头部装配 PacketLinuxSLL2
func buildPacketLinuxSLL2(b *ModelBuilder, headers *LinuxSLL2Headers) any {
	return &PacketLinuxSLL2{
		PacketLink:       b.newPacketLink(),
		LinuxSLL2Headers: *headers,
	}
}

// buildPacketPPP 以 PPP 链路头部装配 PacketPPP
func buildPacketPPP(b *ModelBuilder, headers *PPPHeaders) any {
	return &PacketPPP{
		PacketLink: b.newPacketLink(),
		PPPHeaders: *headers,
	}
}

// buildPacketDot11 以 802.11 链路头部装配 PacketDot11
// RadioTap / Prism 首层的无线抓包同样装配为本模型
func buildPacketDot11(b *ModelBuilder, headers *Dot11Headers) any {
	return &PacketDot11{
		PacketLink:   b.newPacketLink(),
		Dot11Headers: *headers,
	}
}

// buildPacketIPv4 以网络层 IPv4 头部装配 PacketIPv4，链路信息取自通用槽位
func buildPacketIPv4(b *ModelBuilder, headers *IPv4Headers) any {
	return &PacketIPv4{
		PacketLink:  b.newPacketLink(),
		LinkSlots:   b.linkSlots(),
		IPv4Headers: *headers,
		Payload:     serializer.NewCrypto(b.Payload),
	}
}

// buildPacketIPv6 以网络层 IPv6 头部装配 PacketIPv6，链路信息取自通用槽位
func buildPacketIPv6(b *ModelBuilder, headers *IPv6Headers) any {
	return &PacketIPv6{
		PacketLink:  b.newPacketLink(),
		LinkSlots:   b.linkSlots(),
		IPv6Headers: *headers,
		Payload:     serializer.NewCrypto(b.Payload),
	}
}

// buildPacketTCP 以传输层 TCP 头部装配 TCP 数据包模型，复用网络层模型选择 IPv4/IPv6 变体
func buildPacketTCP(b *ModelBuilder, headers *TCPHeaders) any {
	switch network := b.BuildAsPacketNetworkModel().(type) {
	case *PacketIPv4:
		pkt := &PacketIPv4TCP{
			PacketIPv4: *network,
			TCPHeaders: *headers,
			Payload:    serializer.NewCrypto(b.Payload),
		}
		pkt.TCPStreamID = pkt.StreamID()
		return pkt
	case *PacketIPv6:
		pkt := &PacketIPv6TCP{
			PacketIPv6: *network,
			TCPHeaders: *headers,
			Payload:    serializer.NewCrypto(b.Payload),
		}
		pkt.TCPStreamID = pkt.StreamID()
		return pkt
	default:
		return nil
	}
}

// buildPacketUDP 以传输层 UDP 头部装配 UDP 数据包模型，复用网络层模型选择 IPv4/IPv6 变体
func buildPacketUDP(b *ModelBuilder, headers *UDPHeaders) any {
	switch network := b.BuildAsPacketNetworkModel().(type) {
	case *PacketIPv4:
		pkt := &PacketIPv4UDP{
			PacketIPv4: *network,
			UDPHeaders: *headers,
			Payload:    serializer.NewCrypto(b.Payload),
		}
		pkt.UDPStreamID = pkt.StreamID()
		return pkt
	case *PacketIPv6:
		pkt := &PacketIPv6UDP{
			PacketIPv6: *network,
			UDPHeaders: *headers,
			Payload:    serializer.NewCrypto(b.Payload),
		}
		pkt.UDPStreamID = pkt.StreamID()
		return pkt
	default:
		return nil
	}
}

// ethernetModelPrototypes 返回链路层模型原型，Ethernet 与 Loopback 共用 PacketEthernet
func ethernetModelPrototypes() []any {
	return []any{&PacketEthernet{}}
}

// linuxSLLModelPrototypes 返回 Linux SLL 链路层模型原型
func linuxSLLModelPrototypes() []any {
	return []any{&PacketLinuxSLL{}}
}

// linuxSLL2ModelPrototypes 返回 Linux SLL2 链路层模型原型
func linuxSLL2ModelPrototypes() []any {
	return []any{&PacketLinuxSLL2{}}
}

// pppModelPrototypes 返回 PPP 链路层模型原型
func pppModelPrototypes() []any {
	return []any{&PacketPPP{}}
}

// dot11ModelPrototypes 返回 802.11 链路层模型原型
// RadioTap、Prism 与 Dot11 三个首层共用 PacketDot11，由注册表按运行时类型去重
func dot11ModelPrototypes() []any {
	return []any{&PacketDot11{}}
}

// ipv4ModelPrototypes 返回 IPv4 网络层模型原型
func ipv4ModelPrototypes() []any {
	return []any{&PacketIPv4{}}
}

// ipv6ModelPrototypes 返回 IPv6 网络层模型原型
func ipv6ModelPrototypes() []any {
	return []any{&PacketIPv6{}}
}

// tcpModelPrototypes 返回 TCP 传输层模型原型，覆盖 IPv4 与 IPv6 两种网络层组合
func tcpModelPrototypes() []any {
	return []any{&PacketIPv4TCP{}, &PacketIPv6TCP{}}
}

// udpModelPrototypes 返回 UDP 传输层模型原型，覆盖 IPv4 与 IPv6 两种网络层组合
func udpModelPrototypes() []any {
	return []any{&PacketIPv4UDP{}, &PacketIPv6UDP{}}
}

// init 注册内置协议层的模型装配器
// 注册顺序决定 PacketModelPrototypes 的输出顺序
func init() {
	RegisterTypedAssembler(LayerLevelLink, layers.LayerTypeEthernet, linkEthernetHeaders, buildPacketEthernet, ethernetModelPrototypes)
	RegisterTypedAssembler(LayerLevelLink, layers.LayerTypeLoopback, linkEthernetHeaders, buildPacketEthernet, ethernetModelPrototypes)
	RegisterTypedAssembler(LayerLevelLink, layers.LayerTypeLinuxSLL, linkLinuxSLLHeaders, buildPacketLinuxSLL, linuxSLLModelPrototypes)
	RegisterTypedAssembler(LayerLevelLink, layers.LayerTypeLinuxSLL2, linkLinuxSLL2Headers, buildPacketLinuxSLL2, linuxSLL2ModelPrototypes)
	RegisterTypedAssembler(LayerLevelLink, layers.LayerTypePPP, linkPPPHeaders, buildPacketPPP, pppModelPrototypes)
	RegisterTypedAssembler(LayerLevelLink, layers.LayerTypeRadioTap, linkWirelessDot11Headers, buildPacketDot11, dot11ModelPrototypes)
	RegisterTypedAssembler(LayerLevelLink, layers.LayerTypePrismHeader, linkWirelessDot11Headers, buildPacketDot11, dot11ModelPrototypes)
	RegisterTypedAssembler(LayerLevelLink, layers.LayerTypeDot11, linkDot11Headers, buildPacketDot11, dot11ModelPrototypes)
	RegisterTypedAssembler(LayerLevelNetwork, layers.LayerTypeIPv4, networkIPv4Headers, buildPacketIPv4, ipv4ModelPrototypes)
	RegisterTypedAssembler(LayerLevelNetwork, layers.LayerTypeIPv6, networkIPv6Headers, buildPacketIPv6, ipv6ModelPrototypes)
	RegisterTypedAssembler(LayerLevelTransport, layers.LayerTypeTCP, transportTCPHeaders, buildPacketTCP, tcpModelPrototypes)
	RegisterTypedAssembler(LayerLevelTransport, layers.LayerTypeUDP, transportUDPHeaders, buildPacketUDP, udpModelPrototypes)
}
