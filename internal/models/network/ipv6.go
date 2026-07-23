package network

import (
	"database/sql/driver"
	"encoding/json"
	"net"

	"github.com/gopacket/gopacket/layers"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

var (
	_ driver.Valuer = (*IPv6HeaderTLVOptions)(nil)
	_ driver.Valuer = (*IPv6HopByHop)(nil)
)

// IPv6HeaderTLVOption 表示IPv6扩展头部TLV选项
// IPv6扩展头部中的TLV（类型-长度-值）选项结构，用于存储IPv6扩展头部中的各种选项信息
// 这些选项可以出现在逐跳选项头部（Hop-by-Hop Options Header）或目标选项头部（Destination Options Header）中
type IPv6HeaderTLVOption struct {
	// OptionType 选项类型，标识选项的具体类型
	// 常见类型包括：0（Pad1）、1（PadN）、5（Router Alert）、194（Jumbo Payload）等
	OptionType uint8 `json:"option_type"`

	// OptionLength 选项长度，表示OptionData字段的长度（以字节为单位）
	// 注意：对于Pad1选项（类型0），该字段不存在，长度为0
	OptionLength uint8 `json:"option_length"`

	// ActualLength 实际长度，表示选项在数据包中的实际字节长度
	// 由于对齐要求，实际长度可能与OptionLength不同
	ActualLength int `json:"actual_length"`

	// OptionData 选项数据，存储选项的具体内容
	// 对于不同类型的选项，该字段包含不同的数据
	OptionData []byte `json:"option_data"`

	// OptionAlignmentX 选项对齐要求X分量
	// 表示选项需要对齐的边界，格式为x+n*y，其中x是OptionAlignmentX，y是OptionAlignmentY
	OptionAlignmentX uint8 `json:"option_alignment_x"`

	// OptionAlignmentY 选项对齐要求Y分量
	// 表示选项需要对齐的边界，格式为x+n*y，其中x是OptionAlignmentX，y是OptionAlignmentY
	OptionAlignmentY uint8 `json:"option_alignment_y"`
}

// NewIPv6HeaderTLVOptionByHBH 从逐跳选项创建IPv6HeaderTLVOption
// 将gopacket的IPv6HopByHopOption转换为内部表示的IPv6HeaderTLVOption结构
// 该函数用于处理IPv6逐跳选项头部中的TLV选项
//
// 参数:
//
//	opt - gopacket的IPv6HopByHopOption指针，如果为nil则返回nil
//
// 返回值:
//
//	*IPv6HeaderTLVOption - 转换后的TLV选项结构指针，如果输入为nil则返回nil
func NewIPv6HeaderTLVOptionByHBH(opt *layers.IPv6HopByHopOption) *IPv6HeaderTLVOption {
	if opt == nil {
		return nil
	}
	return &IPv6HeaderTLVOption{
		OptionType:       opt.OptionType,
		OptionLength:     opt.OptionLength,
		ActualLength:     opt.ActualLength,
		OptionData:       opt.OptionData,
		OptionAlignmentX: opt.OptionAlignment[0],
		OptionAlignmentY: opt.OptionAlignment[1],
	}
}

// NewIPv6HeaderTLVOptionByDestination 从目标选项创建IPv6HeaderTLVOption
// 将gopacket的IPv6DestinationOption转换为内部表示的IPv6HeaderTLVOption结构
// 该函数用于处理IPv6目标选项头部中的TLV选项
//
// 参数:
//
//	opt - gopacket的IPv6DestinationOption指针，如果为nil则返回nil
//
// 返回值:
//
//	*IPv6HeaderTLVOption - 转换后的TLV选项结构指针，如果输入为nil则返回nil
func NewIPv6HeaderTLVOptionByDestination(opt *layers.IPv6DestinationOption) *IPv6HeaderTLVOption {
	if opt == nil {
		return nil
	}
	return &IPv6HeaderTLVOption{
		OptionType:       opt.OptionType,
		OptionLength:     opt.OptionLength,
		ActualLength:     opt.ActualLength,
		OptionData:       opt.OptionData,
		OptionAlignmentX: opt.OptionAlignment[0],
		OptionAlignmentY: opt.OptionAlignment[1],
	}
}

// IPv6HeaderTLVOptions 表示IPv6扩展头部TLV选项列表
// IPv6HeaderTLVOption的切片类型，用于存储多个TLV选项
// 该类型实现了数据库序列化接口，可以将选项列表存储到数据库中
type IPv6HeaderTLVOptions []IPv6HeaderTLVOption

// Value 实现数据库序列化接口
// 将IPv6HeaderTLVOptions转换为数据库可存储的值（JSON字符串）
// 该方法实现了driver.Valuer接口，用于GORM数据库操作
//
// 返回值:
//
//	driver.Value - 序列化后的JSON字符串
//	error - 序列化过程中发生的错误，如果成功则为nil
func (i IPv6HeaderTLVOptions) Value() (driver.Value, error) {
	return json.Marshal(i)
}

// Scan 实现数据库反序列化接口
// 从数据库值中解析IPv6HeaderTLVOptions
// 该方法实现了sql.Scanner接口，用于GORM数据库操作
//
// 参数:
//
//	value - 数据库中的原始值，可以是[]byte、string或nil
//
// 返回值:
//
//	error - 反序列化过程中发生的错误，如果成功则为nil
//
// 支持的数据类型:
//   - nil: 将切片设置为nil
//   - []byte: 直接解析JSON字节数组
//   - string: 将字符串转换为字节数组后解析JSON
//   - 其他类型: 返回类型错误
func (i *IPv6HeaderTLVOptions) Scan(value any) error {
	if value == nil {
		*i = nil
		return nil
	}
	var bytes []byte
	switch raw := value.(type) {
	case []byte:
		if len(raw) == 0 {
			*i = nil
			return nil
		}
		bytes = raw
	case string:
		if raw == "" {
			*i = nil
			return nil
		}
		bytes = []byte(raw)
	default:
		return errors.Newf("Ipv6_header_tlv_options scan failed, except []byte, got %T", value)
	}
	return json.Unmarshal(bytes, i)
}

// IPv6HopByHop 表示IPv6逐跳选项扩展头部
// IPv6逐跳选项头部（Hop-by-Hop Options Header）结构
// 该扩展头部包含需要在数据包传输路径上每个路由器处理的选项
// 通常用于路由器告警（Router Alert）、Jumbo Payload等选项
type IPv6HopByHop struct {
	// NextHeader 下一个头部类型，标识紧接在该扩展头部之后的下一个头部
	// 使用layers.IPProtocol类型表示，如TCP、UDP、ICMPv6等
	NextHeader layers.IPProtocol `json:"next_header"`

	// HeaderLength 头部长度，以8字节为单位，不包括前8个字节
	// 实际头部长度 = (HeaderLength + 1) * 8 字节
	HeaderLength uint8 `json:"header_length"`

	// ActualLength 实际长度，表示头部在数据包中的实际字节长度
	// 由于对齐要求，实际长度可能与计算出的长度不同
	ActualLength int `json:"actual_length"`

	// Options TLV选项列表，包含逐跳选项头部中的所有选项
	// 使用IPv6HeaderTLVOptions类型存储多个TLV选项
	Options IPv6HeaderTLVOptions `json:"options"`
}

// NewIPv6HopByHop 从gopacket创建IPv6HopByHop
// 将gopacket的IPv6HopByHop层转换为内部表示的IPv6HopByHop结构
// 该函数用于处理IPv6数据包中的逐跳选项扩展头部
//
// 参数:
//
//	hbh - gopacket的IPv6HopByHop指针，如果为nil则返回nil
//
// 返回值:
//
//	*IPv6HopByHop - 转换后的逐跳选项头部结构指针，如果输入为nil则返回nil
//
// 处理过程:
//  1. 检查输入是否为nil，如果是则返回nil
//  2. 创建选项切片，预分配容量以提高性能
//  3. 遍历所有gopacket选项，使用NewIPv6HeaderTLVOptionByHBH进行转换
//  4. 创建并返回IPv6HopByHop结构
func NewIPv6HopByHop(hbh *layers.IPv6HopByHop) *IPv6HopByHop {
	if hbh == nil {
		return nil
	}
	options := make([]IPv6HeaderTLVOption, 0, len(hbh.Options))
	for _, v := range hbh.Options {
		options = append(options, *NewIPv6HeaderTLVOptionByHBH(v))
	}
	return &IPv6HopByHop{
		NextHeader:   hbh.NextHeader,
		HeaderLength: hbh.HeaderLength,
		ActualLength: hbh.ActualLength,
		Options:      options,
	}
}

// Value 实现数据库序列化接口
// 将IPv6HopByHop结构序列化为JSON字符串，用于数据库存储
// 该方法实现了driver.Valuer接口，支持GORM数据库操作
//
// 返回值:
//
//	driver.Value - 序列化后的JSON字符串
//	error - 序列化过程中发生的错误，如果成功则为nil
func (h *IPv6HopByHop) Value() (driver.Value, error) {
	if h == nil {
		return nil, nil
	}
	return json.Marshal(h)
}

// Scan 实现数据库反序列化接口
// 从数据库值中解析IPv6HopByHop结构
// 该方法实现了sql.Scanner接口，支持GORM数据库操作
//
// 参数:
//
//	value - 数据库中的原始值，可以是[]byte、string或nil
//
// 返回值:
//
//	error - 反序列化过程中发生的错误，如果成功则为nil
//
// 注意:
//
//	当value为nil时，会将接收者指针设置为nil
//	支持[]byte和string类型的JSON数据
//	其他类型会返回类型错误
func (h *IPv6HopByHop) Scan(value any) error {
	if value == nil {
		if h != nil {
			*h = IPv6HopByHop{}
		}
		return nil
	}
	var bytes []byte
	switch raw := value.(type) {
	case []byte:
		if len(raw) == 0 {
			*h = IPv6HopByHop{}
			return nil
		}
		bytes = raw
	case string:
		if raw == "" {
			*h = IPv6HopByHop{}
			return nil
		}
		bytes = []byte(raw)
	default:
		return errors.Newf("Ipv6_hop_by_hop scan failed, except []byte, got %T", value)
	}
	return json.Unmarshal(bytes, h)
}

// IPv6Destination 表示IPv6目标选项扩展头部
// IPv6目标选项头部（Destination Options Header）结构
// 该扩展头部包含只需要在目标节点处理的选项
// 通常用于移动IPv6、家乡地址选项等
type IPv6Destination struct {
	// NextHeader 下一个头部类型，标识紧接在该扩展头部之后的下一个头部
	// 使用layers.IPProtocol类型表示，如TCP、UDP、ICMPv6或其他扩展头部
	NextHeader layers.IPProtocol `json:"next_header"`

	// HeaderLength 头部长度，以8字节为单位，不包括前8个字节
	// 实际头部长度 = (HeaderLength + 1) * 8 字节
	HeaderLength uint8 `json:"header_length"`

	// ActualLength 实际长度，表示头部在数据包中的实际字节长度
	// 由于对齐要求，实际长度可能与计算出的长度不同
	ActualLength int `json:"actual_length"`

	// Options TLV选项列表，包含目标选项头部中的所有选项
	// 使用IPv6HeaderTLVOptions类型存储多个TLV选项
	Options IPv6HeaderTLVOptions `json:"options"`
}

// NewIPv6Destination 从gopacket创建IPv6Destination
// 将gopacket的IPv6Destination层转换为内部表示的IPv6Destination结构
// 该函数用于处理IPv6数据包中的目标选项扩展头部
//
// 参数:
//
//	ipv6 - gopacket的IPv6Destination指针
//
// 返回值:
//
//	*IPv6Destination - 转换后的目标选项头部结构指针
//
// 注意:
//
//	该函数假设输入不为nil，调用者需要确保输入有效性
//	如果输入可能为nil，调用者需要先进行nil检查
func NewIPv6Destination(ipv6 *layers.IPv6Destination) *IPv6Destination {
	options := make([]IPv6HeaderTLVOption, 0, len(ipv6.Options))
	for _, v := range ipv6.Options {
		options = append(options, *NewIPv6HeaderTLVOptionByDestination(v))
	}
	return &IPv6Destination{
		NextHeader:   ipv6.NextHeader,
		HeaderLength: ipv6.HeaderLength,
		ActualLength: ipv6.ActualLength,
		Options:      options,
	}
}

// IPv6Fragment 表示IPv6分片扩展头部
// IPv6分片头部（Fragment Header）结构
// 该扩展头部用于IPv6数据包的分片和重组
// 与IPv4不同，IPv6只在源节点进行分片，中间路由器不进行分片
type IPv6Fragment struct {
	// NextHeader 下一个头部类型，标识紧接在该扩展头部之后的下一个头部
	// 使用layers.IPProtocol类型表示，通常是传输层协议或其他扩展头部
	NextHeader layers.IPProtocol `json:"next_header"`

	// Reserved1 保留字段1，必须设置为0
	// 在RFC 8200中定义，保留供未来使用
	Reserved1 uint8 `json:"reserved1"`

	// FragmentOffset 分片偏移量，以8字节为单位
	// 表示该分片在原始数据包中的位置
	// 范围：0-8191（对应0-65528字节）
	FragmentOffset uint16 `json:"fragment_offset"`

	// Reserved2 保留字段2，必须设置为0
	// 在RFC 8200中定义，保留供未来使用
	Reserved2 uint8 `json:"reserved2"`

	// MoreFragments 更多分片标志
	// true: 表示后面还有更多分片
	// false: 表示这是最后一个分片
	MoreFragments bool `json:"more_fragments"`

	// Identification 标识符，用于关联属于同一原始数据包的所有分片
	// 源节点为每个需要分片的数据包生成唯一的标识符
	Identification uint32 `json:"identification"`
}

// NewIPv6Fragment 从gopacket创建IPv6Fragment
// 将gopacket的IPv6Fragment层转换为内部表示的IPv6Fragment结构
// 该函数用于处理IPv6数据包中的分片扩展头部
//
// 参数:
//
//	ipv6 - gopacket的IPv6Fragment指针
//
// 返回值:
//
//	*IPv6Fragment - 转换后的分片头部结构指针
func NewIPv6Fragment(ipv6 *layers.IPv6Fragment) *IPv6Fragment {
	return &IPv6Fragment{
		NextHeader:     ipv6.NextHeader,
		Reserved1:      ipv6.Reserved1,
		FragmentOffset: ipv6.FragmentOffset,
		Reserved2:      ipv6.Reserved2,
		MoreFragments:  ipv6.MoreFragments,
		Identification: ipv6.Identification,
	}
}

// IPv6Routing 表示IPv6路由扩展头部
// IPv6路由头部（Routing Header）结构
// 该扩展头部用于指定数据包经过的中间节点（源路由）
// 支持多种路由类型，如类型0（已弃用）、类型2（移动IPv6）等
type IPv6Routing struct {
	// NextHeader 下一个头部类型，标识紧接在该扩展头部之后的下一个头部
	// 使用layers.IPProtocol类型表示
	NextHeader layers.IPProtocol

	// HeaderLength 头部长度，以8字节为单位，不包括前8个字节
	// 实际头部长度 = (HeaderLength + 1) * 8 字节
	HeaderLength uint8 `json:"header_length"`

	// ActualLength 实际长度，表示头部在数据包中的实际字节长度
	// 由于对齐要求，实际长度可能与计算出的长度不同
	ActualLength int `json:"actual_length"`

	// RoutingType 路由类型，标识路由头部的具体类型
	// 常见类型：0（已弃用）、2（移动IPv6）、3（RPL源路由）等
	RoutingType uint8 `json:"routing_type"`

	// Reserved 保留字段，具体含义取决于路由类型
	// 对于某些路由类型，该字段可能包含特定信息
	Reserved []byte `json:"reserved"`

	// SourceRoutingIPs 源路由IP地址列表
	// 包含数据包需要经过的中间节点的IPv6地址
	// 列表顺序表示数据包经过这些节点的顺序
	SourceRoutingIPs []net.IP `json:"source_routing_ips"`
}

// NewIPv6Routing 从gopacket创建IPv6Routing
// 将gopacket的IPv6Routing层转换为内部表示的IPv6Routing结构
// 该函数用于处理IPv6数据包中的路由扩展头部
//
// 参数:
//
//	ipv6 - gopacket的IPv6Routing指针
//
// 返回值:
//
//	*IPv6Routing - 转换后的路由头部结构指针
func NewIPv6Routing(ipv6 *layers.IPv6Routing) *IPv6Routing {
	return &IPv6Routing{
		NextHeader:       ipv6.NextHeader,
		HeaderLength:     ipv6.HeaderLength,
		ActualLength:     ipv6.ActualLength,
		RoutingType:      ipv6.RoutingType,
		Reserved:         ipv6.Reserved,
		SourceRoutingIPs: ipv6.SourceRoutingIPs,
	}
}

// IPv6Base 表示IPv6基础头部信息
// IPv6基础头部结构，包含IPv6数据包的基本信息
// 该结构对应RFC 8200中定义的IPv6头部格式
type IPv6Base struct {
	// IPv6Version IPv6版本号，固定为6
	// 用于标识IP协议版本，IPv6的版本号始终为6
	IPv6Version uint8 `gorm:"type:UInt8 CODEC(ZSTD(3))" json:"ipv6_version"`

	// IPv6TrafficClass 流量类别，用于QoS（服务质量）区分
	// 对应IPv4中的TOS字段，用于区分数据包的优先级和服务类型
	IPv6TrafficClass uint8 `gorm:"type:UInt8 CODEC(ZSTD(3))" json:"ipv6_traffic_class"`

	// IPv6FlowLabel 流标签，用于标识属于同一流的数据包
	// 用于支持需要特殊处理的数据流，如实时音视频流
	IPv6FlowLabel uint32 `gorm:"type:UInt32 CODEC(ZSTD(3))" json:"ipv6_flow_label"`

	// IPv6Length 有效载荷长度，表示IPv6头部之后的数据长度（以字节为单位）
	// 不包括IPv6头部本身的40字节，但包括扩展头部
	IPv6Length uint16 `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"ipv6_length"`

	// IPv6NextHeader 下一个头部类型，标识紧接在IPv6头部之后的内容
	// 可以是传输层协议（如TCP、UDP、ICMPv6）或扩展头部
	IPv6NextHeader layers.IPProtocol `gorm:"type:UInt8 CODEC(ZSTD(3))" json:"ipv6_next_header"`

	// IPv6HopLimit 跳数限制，对应IPv4中的TTL字段
	// 每经过一个路由器减1，当减到0时数据包被丢弃
	IPv6HopLimit uint8 `gorm:"type:UInt8 CODEC(ZSTD(3))" json:"ipv6_hop_limit"`

	// IPv6Src 源IPv6地址，数据包的发送方地址
	// 使用serializer.IPv6Addr类型进行序列化和数据库存储
	IPv6Src serializer.IPv6Addr `gorm:"type:FixedString(16)" json:"ipv6_src"`

	// IPv6Dst 目的IPv6地址，数据包的接收方地址
	// 使用serializer.IPv6Addr类型进行序列化和数据库存储
	IPv6Dst serializer.IPv6Addr `gorm:"type:FixedString(16)" json:"ipv6_dst"`

	// IPv6HopByHop IPv6逐跳选项扩展头部
	// 包含需要在数据包传输路径上每个路由器处理的选项
	// 使用JSON序列化存储到数据库中
	IPv6HopByHop *IPv6HopByHop `gorm:"type:String CODEC(ZSTD(3))" json:"ipv6_hop_by_hop"`
}

// NewIPv6Base 从gopacket IPv6层创建IPv6Base
// 将gopacket的IPv6层转换为内部表示的IPv6Base结构
// 该函数用于处理IPv6数据包的基础头部信息
//
// 参数:
//
//	ipv6 - gopacket的IPv6层指针
//
// 返回值:
//
//	*IPv6Base - 转换后的IPv6基础头部结构指针
//
// 转换过程:
//  1. 复制基本字段：版本、流量类别、流标签、长度等
//  2. 转换IP地址：使用serializer.IPv6Addr类型进行序列化
//  3. 处理逐跳选项：使用NewIPv6HopByHop函数转换逐跳选项头部
func NewIPv6Base(ipv6 *layers.IPv6) *IPv6Base {
	return &IPv6Base{
		IPv6Version:      ipv6.Version,
		IPv6TrafficClass: ipv6.TrafficClass,
		IPv6FlowLabel:    ipv6.FlowLabel,
		IPv6Length:       ipv6.Length,
		IPv6NextHeader:   ipv6.NextHeader,
		IPv6HopLimit:     ipv6.HopLimit,
		IPv6Src:          serializer.IPv6Addr(ipv6.SrcIP),
		IPv6Dst:          serializer.IPv6Addr(ipv6.DstIP),
		IPv6HopByHop:     NewIPv6HopByHop(ipv6.HopByHop),
	}
}

// IPv6Headers 表示IPv6协议头部信息
// IPv6头部完整结构，包含基础头部和最后一个头部类型
// 该结构用于存储IPv6数据包的完整头部信息
type IPv6Headers struct {
	// IPv6Base IPv6基础头部，包含版本、地址、长度等基本信息
	IPv6Base

	// IPv6LastHeader 最后一个头部类型，标识数据包最终的协议类型
	// 在IPv6中，数据包可能包含多个扩展头部，该字段表示最后一个头部
	// 通常是传输层协议（TCP、UDP、ICMPv6）或应用层协议
	IPv6LastHeader layers.IPProtocol `gorm:"type:UInt8 CODEC(ZSTD(3))" json:"ipv6_last_header"`
}

// SrcAddr 返回源IP地址
// 获取IPv6Headers结构中的源IP地址
//
// 返回值:
//
//	net.IP - 源IPv6地址
func (i *IPv6Headers) SrcAddr() net.IP {
	return net.IP(i.IPv6Src)
}

// DstAddr 返回目的IP地址
// 获取IPv6Headers结构中的目的IP地址
//
// 返回值:
//
//	net.IP - 目的IPv6地址
func (i *IPv6Headers) DstAddr() net.IP {
	return net.IP(i.IPv6Dst)
}

// NewIPv6Headers 从gopacket IPv6层创建IPv6Headers
// 将gopacket的IPv6层转换为内部表示的IPv6Headers结构
func NewIPv6Headers(ipv6 *layers.IPv6) *IPv6Headers {
	return &IPv6Headers{
		IPv6Base:       *NewIPv6Base(ipv6),
		IPv6LastHeader: ipv6.NextHeader,
	}
}

// PacketIPv6 表示IPv6数据包
// IPv6数据包完整结构，包含以太网头部和IPv6头部
// 该结构用于存储原始IPv6数据包信息，对应数据库中的packet_ipv6表
type PacketIPv6 struct {
	// PacketEthernet 以太网头部信息，包含MAC地址、以太网类型等
	PacketEthernet

	// IPv6Headers IPv6头部信息，包含IPv6基础头部和扩展头部
	IPv6Headers

	// Payload 有效载荷，包含IPv6头部之后的数据
	Payload *serializer.Crypto `gorm:"type:String CODEC(ZSTD(3));serializer:crypto" json:"payload" secure:"mode=\"aes-gcm\",offset=31496,length=32"`
}

// FusedIPv6 表示融合的IPv6数据
// 融合的IPv6数据包结构，包含以太网头部和IPv6头部
// 该结构用于存储融合后的IPv6数据包信息，对应数据库中的fused_ipv6表
// 与PacketIPv6相比，FusedIPv6可能包含额外的处理信息或聚合数据
type FusedIPv6 struct {
	// FusedEthernet 融合的以太网头部信息
	FusedEthernet

	// IPv6Headers IPv6头部信息，包含IPv6基础头部和扩展头部
	IPv6Headers

	// Payload 有效载荷，包含IPv6头部之后的数据
	Payload *serializer.Crypto `gorm:"type:String CODEC(ZSTD(3));serializer:crypto" json:"payload" secure:"mode=\"aes-gcm\",offset=31406,length=32"`
}
