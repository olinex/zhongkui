package network

import (
	"strconv"

	"github.com/gopacket/gopacket/layers"
)

// LinuxSLLHeaders 表示 Linux SLL（Linux cooked capture）链路层头部信息
// Linux SLL 是 libpcap 在 `any` 接口抓包时使用的链路封装，包含数据包类型、
// 地址长度、链路地址与内层以太网协议类型等字段。
//
// 数据库映射:
//   - 使用GORM标签显式声明列名与列类型，避免 GORM 推导歧义
//   - 链路地址以字符串存储，不使用 FixedString(6) 的 MAC 序列化器
//   - 在ClickHouse中使用ZSTD(3)压缩算法优化存储
type LinuxSLLHeaders struct {
	// SLLPacketType 数据包类型（主机、广播、组播等）
	SLLPacketType layers.LinuxSLLPacketType `gorm:"column:sll_packet_type;type:UInt16 CODEC(ZSTD(3))" json:"sllPacketType"`
	// SLLAddrLen 链路地址长度（字节）
	SLLAddrLen uint16 `gorm:"column:sll_addr_len;type:UInt16 CODEC(ZSTD(3))" json:"sllAddrLen"`
	// SLLAddr 链路地址，使用 net.HardwareAddr.String() 的小写冒号十六进制表示
	SLLAddr string `gorm:"column:sll_addr;type:String CODEC(ZSTD(3))" json:"sllAddr"`
	// SLLAddrType 链路地址类型
	SLLAddrType uint16 `gorm:"column:sll_addr_type;type:UInt16 CODEC(ZSTD(3))" json:"sllAddrType"`
	// SLLProtocol 内层协议类型（以太网类型语义）
	SLLProtocol layers.EthernetType `gorm:"column:sll_protocol;type:UInt16 CODEC(ZSTD(3))" json:"sllProtocol"`
}

// NewLinuxSLLHeaders 从 gopacket Linux SLL 层创建 LinuxSLLHeaders
func NewLinuxSLLHeaders(sll *layers.LinuxSLL) *LinuxSLLHeaders {
	return &LinuxSLLHeaders{
		SLLPacketType: sll.PacketType,
		SLLAddrLen:    sll.AddrLen,
		SLLAddr:       sll.Addr.String(),
		SLLAddrType:   sll.AddrType,
		SLLProtocol:   sll.EthernetType,
	}
}

// PacketLinuxSLL 表示 Linux SLL 链路层数据包
// 组合链路无关基座与 Linux SLL 链路头部，用于存储无法继续解析内层协议的 SLL 帧
type PacketLinuxSLL struct {
	PacketLink
	LinuxSLLHeaders
}

// LinkSlotValues 按 Linux SLL 槽位约定返回链路槽位
// link_1=PacketType、link_2=AddrLen、link_3=Addr、link_4=AddrType、link_5=EthernetType。
func (p *PacketLinuxSLL) LinkSlotValues() LinkSlots {
	return NewLinkSlots(
		strconv.FormatUint(uint64(p.SLLPacketType), 10),
		strconv.FormatUint(uint64(p.SLLAddrLen), 10),
		p.SLLAddr,
		strconv.FormatUint(uint64(p.SLLAddrType), 10),
		strconv.FormatUint(uint64(p.SLLProtocol), 10),
	)
}
