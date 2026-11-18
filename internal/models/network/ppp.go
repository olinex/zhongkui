package network

import (
	"strconv"

	"github.com/gopacket/gopacket/layers"
)

// PPPHeaders 表示 PPP（Point-to-Point Protocol）链路层头部信息
// PPP 头部包含协议类型以及是否带有 PPPoE/HDLC 风格的 0xff03 前缀。
//
// 数据库映射:
//   - 使用GORM标签显式声明列名与列类型，避免 GORM 推导歧义
//   - 在ClickHouse中使用ZSTD(3)压缩算法优化存储
type PPPHeaders struct {
	// PPPType PPP 协议类型，标识内层协议（如 IPv4、IPv6）
	PPPType layers.PPPType `gorm:"column:ppp_type;type:UInt16 CODEC(ZSTD(3))" json:"pppType"`
	// PPPHasPPTPHeader 是否带有 0xff03 前缀（HDLC 地址/控制字段）
	PPPHasPPTPHeader bool `gorm:"column:ppp_has_pptp_header;type:Bool" json:"pppHasPPTPHeader"`
}

// NewPPPHeaders 从 gopacket PPP 层创建 PPPHeaders
func NewPPPHeaders(ppp *layers.PPP) *PPPHeaders {
	return &PPPHeaders{
		PPPType:          ppp.PPPType,
		PPPHasPPTPHeader: ppp.HasPPTPHeader,
	}
}

// PacketPPP 表示 PPP 链路层数据包
// 组合链路无关基座与 PPP 链路头部，用于存储无法继续解析内层协议的 PPP 帧
type PacketPPP struct {
	PacketLink
	PPPHeaders
}

// LinkSlotValues 按 PPP 槽位约定返回链路槽位
// link_1=PPPType（十进制）、link_2=HasPPTPHeader（true/false）。
func (p *PacketPPP) LinkSlotValues() LinkSlots {
	return NewLinkSlots(
		strconv.FormatUint(uint64(p.PPPType), 10),
		strconv.FormatBool(p.PPPHasPPTPHeader),
	)
}
