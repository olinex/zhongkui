package network

import (
	"strconv"

	"github.com/gopacket/gopacket/layers"
)

// RadioTapHeaders 表示 RadioTap 无线抓包元数据头部
// RadioTap 的详细字段位于 gopacket 的命名空间结构中，这里仅固化稳定的版本与长度，
// 其余链路信息仍完整保留在数据包的 layer_stack 中。
type RadioTapHeaders struct {
	// Version RadioTap 版本号
	Version uint8 `json:"version"`
	// Length RadioTap 头部总长度
	Length uint16 `json:"length"`
}

// PrismHeaders 表示 Prism 无线抓包元数据头部
type PrismHeaders struct {
	// Code Prism 消息码
	Code uint16 `json:"code"`
	// Length Prism 头部长度
	Length uint16 `json:"length"`
	// DeviceName 抓包设备名称
	DeviceName string `json:"deviceName"`
}

// Dot11Headers 表示 IEEE 802.11 链路层头部信息
//
// 数据库映射:
//   - 使用GORM标签显式声明列名与列类型，避免 GORM 推导歧义
//   - 地址字段以字符串存储，不使用 FixedString(6) 的 MAC 序列化器
//   - 在ClickHouse中使用ZSTD(3)压缩算法优化存储
type Dot11Headers struct {
	// Dot11Type 帧类型与子类型
	Dot11Type layers.Dot11Type `gorm:"column:dot11_type;type:UInt8 CODEC(ZSTD(3))" json:"dot11Type"`
	// Dot11Proto 协议版本
	Dot11Proto uint8 `gorm:"column:dot11_proto;type:UInt8 CODEC(ZSTD(3))" json:"dot11Proto"`
	// Dot11Flags 帧控制标志位
	Dot11Flags layers.Dot11Flags `gorm:"column:dot11_flags;type:UInt8 CODEC(ZSTD(3))" json:"dot11Flags"`
	// Dot11DurationID 持续时间或关联标识
	Dot11DurationID uint16 `gorm:"column:dot11_duration_id;type:UInt16 CODEC(ZSTD(3))" json:"dot11DurationID"`
	// Dot11Address1 接收方地址
	Dot11Address1 string `gorm:"column:dot11_address_1;type:String CODEC(ZSTD(3))" json:"dot11Address1"`
	// Dot11Address2 发送方地址
	Dot11Address2 string `gorm:"column:dot11_address_2;type:String CODEC(ZSTD(3))" json:"dot11Address2"`
	// Dot11Address3 第三个地址
	Dot11Address3 string `gorm:"column:dot11_address_3;type:String CODEC(ZSTD(3))" json:"dot11Address3"`
	// Dot11Address4 第四个地址
	Dot11Address4 string `gorm:"column:dot11_address_4;type:String CODEC(ZSTD(3))" json:"dot11Address4"`
	// Dot11SequenceNumber 序列号
	Dot11SequenceNumber uint16 `gorm:"column:dot11_sequence_number;type:UInt16 CODEC(ZSTD(3))" json:"dot11SequenceNumber"`
	// Dot11FragmentNumber 分片号
	Dot11FragmentNumber uint16 `gorm:"column:dot11_fragment_number;type:UInt16 CODEC(ZSTD(3))" json:"dot11FragmentNumber"`
	// Dot11HasQOS 是否带有 QoS 控制字段
	Dot11HasQOS bool `gorm:"column:dot11_has_qos;type:Bool" json:"dot11HasQOS"`
	// Dot11QOSTID 流量标识（TID）
	Dot11QOSTID uint8 `gorm:"column:dot11_qos_tid;type:UInt8 CODEC(ZSTD(3))" json:"dot11QOSTID"`
	// Dot11QOSEOSP 服务期结束标志
	Dot11QOSEOSP bool `gorm:"column:dot11_qos_eosp;type:Bool" json:"dot11QOSEOSP"`
	// Dot11QOSAckPolicy 确认策略
	Dot11QOSAckPolicy layers.Dot11AckPolicy `gorm:"column:dot11_qos_ack_policy;type:UInt8 CODEC(ZSTD(3))" json:"dot11QOSAckPolicy"`
	// Dot11QOSTXOP 发送机会限制
	Dot11QOSTXOP uint8 `gorm:"column:dot11_qos_txop;type:UInt8 CODEC(ZSTD(3))" json:"dot11QOSTXOP"`
}

// NewRadioTapHeaders 从 gopacket RadioTap 层创建 RadioTapHeaders
func NewRadioTapHeaders(radioTap *layers.RadioTap) *RadioTapHeaders {
	return &RadioTapHeaders{
		Version: radioTap.Version,
		Length:  radioTap.Length,
	}
}

// NewPrismHeaders 从 gopacket PrismHeader 层创建 PrismHeaders
func NewPrismHeaders(prism *layers.PrismHeader) *PrismHeaders {
	return &PrismHeaders{
		Code:       prism.Code,
		Length:     prism.Length,
		DeviceName: prism.DeviceName,
	}
}

// NewDot11Headers 从 gopacket Dot11 层创建 Dot11Headers
func NewDot11Headers(dot11 *layers.Dot11) *Dot11Headers {
	headers := &Dot11Headers{
		Dot11Type:           dot11.Type,
		Dot11Proto:          dot11.Proto,
		Dot11Flags:          dot11.Flags,
		Dot11DurationID:     dot11.DurationID,
		Dot11Address1:       dot11.Address1.String(),
		Dot11Address2:       dot11.Address2.String(),
		Dot11Address3:       dot11.Address3.String(),
		Dot11Address4:       dot11.Address4.String(),
		Dot11SequenceNumber: dot11.SequenceNumber,
		Dot11FragmentNumber: dot11.FragmentNumber,
	}
	if dot11.QOS != nil {
		headers.Dot11HasQOS = true
		headers.Dot11QOSTID = dot11.QOS.TID
		headers.Dot11QOSEOSP = dot11.QOS.EOSP
		headers.Dot11QOSAckPolicy = dot11.QOS.AckPolicy
		headers.Dot11QOSTXOP = dot11.QOS.TXOP
	}
	return headers
}

// PacketDot11 表示 IEEE 802.11 链路层数据包
// 组合链路无关基座与 802.11 链路头部，用于存储无法继续解析内层协议的无线帧；
// RadioTap / Prism 首层的抓包同样装配为本模型，其抓包头保留在 layer_stack 中。
type PacketDot11 struct {
	PacketLink
	Dot11Headers
}

// LinkSlotValues 按 802.11 槽位约定返回链路槽位
// link_1=接收方地址、link_2=发送方地址、link_3=地址3、link_4=地址4、
// link_5=帧类型、link_6=帧标志、link_7=序列号、link_8=分片号。
func (p *PacketDot11) LinkSlotValues() LinkSlots {
	return NewLinkSlots(
		p.Dot11Address1,
		p.Dot11Address2,
		p.Dot11Address3,
		p.Dot11Address4,
		strconv.FormatUint(uint64(p.Dot11Type), 10),
		strconv.FormatUint(uint64(p.Dot11Flags), 10),
		strconv.FormatUint(uint64(p.Dot11SequenceNumber), 10),
		strconv.FormatUint(uint64(p.Dot11FragmentNumber), 10),
	)
}
