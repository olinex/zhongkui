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

// buildPacketEthernet 以链路层头部装配 PacketEthernet
// Ethernet 与 Loopback 复用同一模型，二者仅头部构造不同
func buildPacketEthernet(b *ModelBuilder, headers *EthernetHeaders) any {
	return &PacketEthernet{
		AsPacket: AsPacket{
			BaseMeta: BaseMeta{
				SessionID: b.SessionID,
				Namespace: b.Source.Namespace,
				Capture:   b.Source.Name,
				Version:   b.Source.Version,
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
		},
		// 行详情接口依赖非空的全局唯一 ID，构造时即生成，落库路径无需再补；
		// 作为最底层模型，其 ID 经嵌入传递被 IPv4/IPv6/UDP/TCP 各上层数据包模型继承
		RowMeta: RowMeta{
			ID: serializer.NewHexUUIDv4(),
		},
		EthernetHeaders: *headers,
	}
}

// buildPacketIPv4 以网络层 IPv4 头部装配 PacketIPv4，复用链路层模型
func buildPacketIPv4(b *ModelBuilder, headers *IPv4Headers) any {
	link, ok := b.BuildAsPacketLinkModel().(*PacketEthernet)
	if !ok {
		return nil
	}
	return &PacketIPv4{
		PacketEthernet: *link,
		IPv4Headers:    *headers,
		Payload:        serializer.NewCrypto(b.Payload),
	}
}

// buildPacketIPv6 以网络层 IPv6 头部装配 PacketIPv6，复用链路层模型
func buildPacketIPv6(b *ModelBuilder, headers *IPv6Headers) any {
	link, ok := b.BuildAsPacketLinkModel().(*PacketEthernet)
	if !ok {
		return nil
	}
	return &PacketIPv6{
		PacketEthernet: *link,
		IPv6Headers:    *headers,
		Payload:        serializer.NewCrypto(b.Payload),
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
	RegisterTypedAssembler(LayerLevelNetwork, layers.LayerTypeIPv4, networkIPv4Headers, buildPacketIPv4, ipv4ModelPrototypes)
	RegisterTypedAssembler(LayerLevelNetwork, layers.LayerTypeIPv6, networkIPv6Headers, buildPacketIPv6, ipv6ModelPrototypes)
	RegisterTypedAssembler(LayerLevelTransport, layers.LayerTypeTCP, transportTCPHeaders, buildPacketTCP, tcpModelPrototypes)
	RegisterTypedAssembler(LayerLevelTransport, layers.LayerTypeUDP, transportUDPHeaders, buildPacketUDP, udpModelPrototypes)
}
