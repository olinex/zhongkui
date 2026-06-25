package network

import (
	"net"

	"github.com/gopacket/gopacket/layers"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// EthernetHeaders 表示以太网帧的头部信息
// 该结构体封装了以太网帧的关键头部字段，包括源MAC地址、目的MAC地址、
// 以太网类型和帧长度。这些字段用于网络数据包的识别、路由和分析。
//
// 字段说明:
//   - EthDstMAC: 目的MAC地址，标识数据包的目标网络接口
//   - EthSrcMAC: 源MAC地址，标识数据包的发送网络接口
//   - EthType:   以太网类型，标识上层协议类型（如IPv4、IPv6、ARP等）
//   - EthLength: 帧长度，表示以太网帧的数据部分长度
//
// 数据库映射:
//   - 使用GORM标签定义数据库约束和列类型
//   - 使用JSON标签支持序列化和反序列化
//   - 在ClickHouse中使用ZSTD(3)压缩算法优化存储
type EthernetHeaders struct {
	EthDstMAC serializer.MacAddr  `gorm:"type:FixedString(6)" json:"eth_dst_mac"`
	EthSrcMAC serializer.MacAddr  `gorm:"type:FixedString(6)" json:"eth_src_mac"`
	EthType   layers.EthernetType `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"eth_type"`
	EthLength uint16              `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"eth_length"`
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

func NewLoopbackHeaders() *EthernetHeaders {
	return &EthernetHeaders{
		EthDstMAC: serializer.MacAddr(net.HardwareAddr{}),
		EthSrcMAC: serializer.MacAddr(net.HardwareAddr{}),
		EthType:   layers.EthernetTypeLLC,
		EthLength: 0,
	}
}

// PacketEthernet 表示完整的以太网数据包
// 该结构体组合了AsPacket基础结构和EthernetHeaders以太网头部信息，
// 用于存储和分析原始的网络数据包。它包含了数据包的所有元数据和载荷。
//
// 结构说明:
//   - AsPacket: 提供数据包的基础元数据，包括时间戳、捕获信息、层堆栈等
//   - EthernetHeaders: 提供以太网特定的头部信息
//
// 使用场景:
//   - 网络数据包捕获和存储
//   - 网络流量分析和监控
//   - 安全审计和入侵检测
//   - 网络故障排查
type PacketEthernet struct {
	AsPacket
	EthernetHeaders
}

// FusedEthernet 表示融合后的以太网数据
// 该结构体组合了AsFused基础结构和EthernetHeaders以太网头部信息，
// 用于存储经过融合处理的数据包。融合数据通常包含多个相关数据包的信息，
// 用于高级分析和聚合查询。
//
// 结构说明:
//   - AsFused: 提供融合数据的基础元数据，包括时间戳、帧堆栈等
//   - EthernetHeaders: 提供以太网特定的头部信息
//
// 使用场景:
//   - 网络会话重组和分析
//   - 流量聚合统计
//   - 长期趋势分析
//   - 大数据查询和报表
type FusedEthernet struct {
	AsFused
	EthernetHeaders
}
