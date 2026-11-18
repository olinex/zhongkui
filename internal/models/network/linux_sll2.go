package network

import (
	"strconv"

	"github.com/gopacket/gopacket/layers"
)

// LinuxSLL2Headers 表示 Linux SLL2（Linux cooked capture v2）链路层头部信息
// Linux SLL2 是 SLL 的第二版封装，包含协议类型、接口索引、ARP 硬件类型、
// 数据包类型、地址长度与链路地址等字段。
//
// 数据库映射:
//   - 使用GORM标签显式声明列名与列类型，避免 GORM 推导歧义
//   - 链路地址以字符串存储，不使用 FixedString(6) 的 MAC 序列化器
//   - 在ClickHouse中使用ZSTD(3)压缩算法优化存储
type LinuxSLL2Headers struct {
	// SLL2ProtocolType 内层协议类型（以太网类型语义）
	SLL2ProtocolType layers.EthernetType `gorm:"column:sll2_protocol_type;type:UInt16 CODEC(ZSTD(3))" json:"sll2ProtocolType"`
	// SLL2InterfaceIndex 抓包接口索引
	SLL2InterfaceIndex uint32 `gorm:"column:sll2_interface_index;type:UInt32 CODEC(ZSTD(3))" json:"sll2InterfaceIndex"`
	// SLL2ARPHardwareType ARP 硬件类型（如以太网、回环、Radiotap 等）
	SLL2ARPHardwareType layers.ARPHardwareType `gorm:"column:sll2_arp_hardware_type;type:UInt16 CODEC(ZSTD(3))" json:"sll2ArpHardwareType"`
	// SLL2PacketType 数据包类型（主机、广播、组播等）
	SLL2PacketType layers.LinuxSLL2PacketType `gorm:"column:sll2_packet_type;type:UInt8 CODEC(ZSTD(3))" json:"sll2PacketType"`
	// SLL2AddrLength 链路地址长度（字节）
	SLL2AddrLength uint8 `gorm:"column:sll2_addr_length;type:UInt8 CODEC(ZSTD(3))" json:"sll2AddrLength"`
	// SLL2Addr 链路地址，使用 net.HardwareAddr.String() 的小写冒号十六进制表示
	SLL2Addr string `gorm:"column:sll2_addr;type:String CODEC(ZSTD(3))" json:"sll2Addr"`
}

// NewLinuxSLL2Headers 从 gopacket Linux SLL2 层创建 LinuxSLL2Headers
func NewLinuxSLL2Headers(sll2 *layers.LinuxSLL2) *LinuxSLL2Headers {
	return &LinuxSLL2Headers{
		SLL2ProtocolType:    sll2.ProtocolType,
		SLL2InterfaceIndex:  sll2.InterfaceIndex,
		SLL2ARPHardwareType: sll2.ARPHardwareType,
		SLL2PacketType:      sll2.PacketType,
		SLL2AddrLength:      sll2.AddrLength,
		SLL2Addr:            sll2.Addr.String(),
	}
}

// PacketLinuxSLL2 表示 Linux SLL2 链路层数据包
// 组合链路无关基座与 Linux SLL2 链路头部，用于存储无法继续解析内层协议的 SLL2 帧
type PacketLinuxSLL2 struct {
	PacketLink
	LinuxSLL2Headers
}

// LinkSlotValues 按 Linux SLL2 槽位约定返回链路槽位
// link_1=ProtocolType、link_2=InterfaceIndex、link_3=ARPHardwareType、
// link_4=PacketType、link_5=AddrLength、link_6=Addr。
func (p *PacketLinuxSLL2) LinkSlotValues() LinkSlots {
	return NewLinkSlots(
		strconv.FormatUint(uint64(p.SLL2ProtocolType), 10),
		strconv.FormatUint(uint64(p.SLL2InterfaceIndex), 10),
		strconv.FormatUint(uint64(p.SLL2ARPHardwareType), 10),
		strconv.FormatUint(uint64(p.SLL2PacketType), 10),
		strconv.FormatUint(uint64(p.SLL2AddrLength), 10),
		p.SLL2Addr,
	)
}
