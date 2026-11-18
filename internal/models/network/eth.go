package network

import (
	"net"
	"strconv"

	"github.com/gopacket/gopacket/layers"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// EthernetHeaders 表示以太网帧的头部信息
// 该结构体封装了以太网帧的关键头部字段，包括源MAC地址、目的MAC地址、
// 以太网类型和帧长度。这些字段用于网络数据包的识别、路由和分析。
//
// 数据库映射:
//   - 使用GORM标签定义数据库约束和列类型
//   - 使用JSON标签支持序列化和反序列化
//   - 在ClickHouse中使用ZSTD(3)压缩算法优化存储
type EthernetHeaders struct {
	// EthDstMAC 目的 MAC 地址
	EthDstMAC serializer.MacAddr `gorm:"type:FixedString(6)" json:"ethDstMac"`
	// EthSrcMAC 源 MAC 地址
	EthSrcMAC serializer.MacAddr `gorm:"type:FixedString(6)" json:"ethSrcMac"`
	// EthType 以太网类型，标识上层协议类型（如 IPv4、IPv6、ARP 等）
	EthType layers.EthernetType `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"ethType"`
	// EthLength 以太网帧数据部分长度（字节）
	EthLength uint16 `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"ethLength"`
}

// NewEthernetHeaders 从gopacket以太网层创建EthernetHeaders实例
// 该函数将gopacket库中的Ethernet层数据转换为应用内部的EthernetHeaders结构，
// 便于后续的数据存储、查询和分析。
//
// 参数:
//   - eth: gopacket的Ethernet层指针，包含原始的以太网帧数据
//
// 返回值:
//   - *EthernetHeaders: 新创建的EthernetHeaders实例指针
//
// 转换过程:
//   - 将原始MAC地址转换为serializer.MacAddr类型
//   - 直接复制EthernetType和Length字段
//   - 保持数据的原始性和准确性，不进行修改
func NewEthernetHeaders(eth *layers.Ethernet) *EthernetHeaders {
	return &EthernetHeaders{
		EthDstMAC: serializer.MacAddr(eth.DstMAC),
		EthSrcMAC: serializer.MacAddr(eth.SrcMAC),
		EthType:   eth.EthernetType,
		EthLength: eth.Length,
	}
}

// NewLoopbackHeaders 构造回环（loopback）数据包使用的空以太网头部实例。
func NewLoopbackHeaders() *EthernetHeaders {
	return &EthernetHeaders{
		EthDstMAC: serializer.MacAddr(net.HardwareAddr{}),
		EthSrcMAC: serializer.MacAddr(net.HardwareAddr{}),
		EthType:   layers.EthernetTypeLLC,
		EthLength: 0,
	}
}

// PacketEthernet 表示完整的以太网数据包
// 该结构体组合了链路无关基座 PacketLink 与 EthernetHeaders 以太网头部信息，
// 用于存储和分析原始的网络数据包。它包含了数据包的所有元数据和载荷。
//
// 结构说明:
//   - PacketLink: 提供数据包的基础元数据（含链路类型）与行级全局唯一 ID
//   - EthernetHeaders: 提供以太网特定的头部信息
//
// 使用场景:
//   - 网络数据包捕获和存储
//   - 网络流量分析和监控
//   - 安全审计和入侵检测
//   - 网络故障排查
type PacketEthernet struct {
	PacketLink
	EthernetHeaders
}

// LinkSlotValues 按以太网槽位约定返回链路槽位
// link_1=目的 MAC、link_2=源 MAC、link_3=以太网类型（十进制）、link_4=帧长度（十进制）。
// 回环链路与以太网共用本模型，其 MAC 为空，此时槽位全部保持空串。
func (p *PacketEthernet) LinkSlotValues() LinkSlots {
	dstMAC, srcMAC := p.EthDstMAC.String(), p.EthSrcMAC.String()
	if dstMAC == "" && srcMAC == "" {
		return LinkSlots{}
	}
	return NewLinkSlots(
		dstMAC,
		srcMAC,
		strconv.FormatUint(uint64(p.EthType), 10),
		strconv.FormatUint(uint64(p.EthLength), 10),
	)
}
