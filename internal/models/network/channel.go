package network

import (
	"database/sql/driver"
	"encoding/json"

	"github.com/gopacket/gopacket/layers"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// VXLANChannel 表示VXLAN隧道通道信息
// VXLAN（Virtual Extensible LAN）是一种网络虚拟化技术，用于在现有IP网络上创建虚拟二层网络
// 该结构体存储VXLAN头部的主要字段，便于后续分析和存储
type VXLANChannel struct {
	ValidIDFlag      bool   `json:"valid_id_flag"`       // VNI有效标志位，表示VNI字段是否包含有效数据
	VNI              uint32 `json:"vni"`                 // 虚拟网络标识符（VXLAN Network Identifier），24位，用于标识虚拟网络
	GBPExtension     bool   `json:"gbp_extension"`       // 组策略扩展标志，表示是否包含组策略扩展头
	GBPDontLearn     bool   `json:"gbp_dont_learn"`      // 组策略不学习标志，控制MAC地址学习行为
	GBPApplied       bool   `json:"gbp_applied"`         // 组策略应用标志，表示组策略是否已应用
	GBPGroupPolicyID uint16 `json:"gbp_group_policy_id"` // 组策略ID，用于分组策略控制
}

// NewVXLANChannel 从gopacket VXLAN层创建VXLANChannel
// 将gopacket库中的VXLAN层数据转换为内部使用的VXLANChannel结构体
//
// 参数:
//   - vxlan: *layers.VXLAN类型，gopacket解析的VXLAN层数据
//
// 返回值:
//   - *VXLANChannel: 转换后的VXLAN通道信息结构体
//
// 功能说明:
//  1. 提取VXLAN头部所有关键字段
//  2. 保持与原始数据的一致性
//  3. 便于后续序列化和存储
func NewVXLANChannel(vxlan *layers.VXLAN) *VXLANChannel {
	return &VXLANChannel{
		ValidIDFlag:      vxlan.ValidIDFlag,
		VNI:              vxlan.VNI,
		GBPExtension:     vxlan.GBPExtension,
		GBPDontLearn:     vxlan.GBPDontLearn,
		GBPApplied:       vxlan.GBPApplied,
		GBPGroupPolicyID: vxlan.GBPGroupPolicyID,
	}
}

// GRERouting 表示GRE路由信息
// GRE（Generic Routing Encapsulation）协议支持源路由扩展（Source Route Extension）
// 该结构体存储GRE源路由信息，支持链式结构表示多个路由条目
type GRERouting struct {
	AddressFamily      uint16      `json:"address_family"`      // 地址族类型，标识路由信息的地址格式（如IPv4、IPv6）
	SRELength          uint8       `json:"sre_length"`          // 源路由扩展长度，以4字节为单位
	RoutingInformation []byte      `json:"routing_information"` // 路由信息数据，具体格式由AddressFamily决定
	Next               *GRERouting `json:"next"`                // 下一个路由条目，支持多级源路由
}

// NewGRERouting 从gopacket GRE路由创建GRERouting
// 递归转换gopacket中的GRE源路由信息为内部结构体
//
// 参数:
//   - raw: *layers.GRERouting类型，gopacket解析的GRE路由数据
//
// 返回值:
//   - *GRERouting: 转换后的GRE路由信息结构体，如果输入为nil则返回nil
//
// 功能说明:
//  1. 递归处理链式路由结构
//  2. 保持原始数据的完整性和一致性
//  3. 便于后续的序列化和反序列化操作
func NewGRERouting(raw *layers.GRERouting) *GRERouting {
	if raw == nil {
		return nil
	}
	return &GRERouting{
		AddressFamily:      raw.AddressFamily,
		SRELength:          raw.SRELength,
		RoutingInformation: raw.RoutingInformation,
		Next:               NewGRERouting(raw.Next),
	}
}

// Value 实现数据库序列化接口
// 将GRERouting结构体序列化为JSON格式，便于存储到数据库
//
// 返回值:
//   - driver.Value: 序列化后的JSON字符串
//   - error: 序列化过程中出现的错误
//
// 功能说明:
//  1. 使用JSON格式序列化结构体
//  2. 支持嵌套结构（Next字段）的递归序列化
//  3. 符合database/sql/driver.Value接口要求
func (g GRERouting) Value() (driver.Value, error) {
	return json.Marshal(g)
}

// Scan 实现数据库反序列化接口
// 从数据库读取JSON数据并反序列化为GRERouting结构体
//
// 参数:
//   - value: any类型，数据库中的原始数据（可能是[]byte或string）
//
// 返回值:
//   - error: 反序列化过程中出现的错误
//
// 异常:
//   - 如果数据类型不支持（既不是[]byte也不是string），返回错误
//
// 功能说明:
//  1. 处理nil值输入
//  2. 支持[]byte和string两种输入格式
//  3. 使用JSON格式反序列化数据
//  4. 支持嵌套结构（Next字段）的递归反序列化
func (g *GRERouting) Scan(value any) error {
	if value == nil {
		g = nil
		return nil
	}
	var bytes []byte
	switch raw := value.(type) {
	case []byte:
		bytes = raw
	case string:
		bytes = []byte(raw)
	default:
		return errors.Newf("Routing scan failed, except []byte, got %T", value)
	}
	return json.Unmarshal(bytes, g)
}

// GREChannel 表示GRE隧道通道信息
// GRE（Generic Routing Encapsulation）是一种通用的隧道封装协议，用于在IP网络中封装各种网络层协议
// 该结构体存储GRE头部的主要字段，包括标志位、协议类型和可选字段
type GREChannel struct {
	ChecksumPresent   bool                `json:"checksum_present"`    // 校验和存在标志，表示头部包含校验和字段
	RoutingPresent    bool                `json:"routing_present"`     // 路由存在标志，表示头部包含源路由扩展
	KeyPresent        bool                `json:"key_present"`         // 密钥存在标志，表示头部包含密钥字段
	SeqPresent        bool                `json:"seq_present"`         // 序列号存在标志，表示头部包含序列号字段
	StrictSourceRoute bool                `json:"strict_source_route"` // 严格源路由标志，控制源路由的处理方式
	AckPresent        bool                `json:"ack_present"`         // 确认号存在标志，表示头部包含确认号字段
	CRecursionControl uint8               `json:"recursion_control"`   // 递归控制字段，限制隧道的嵌套深度
	Flags             uint8               `json:"flags"`               // 保留标志位，用于未来扩展
	Version           uint8               `json:"version"`             // GRE协议版本，目前通常为0
	Protocol          layers.EthernetType `json:"protocol"`            // 封装协议类型，标识内层协议（如IPv4、IPv6等）
	Checksum          uint16              `json:"checksum"`            // 可选校验和，用于验证头部完整性
	Offset            uint16              `json:"offset"`              // 可选偏移量，与校验和相关
	Key               uint32              `json:"key"`                 // 可选密钥，用于隧道标识和安全
	Seq               uint32              `json:"seq"`                 // 可选序列号，用于数据包排序
	Ack               uint32              `json:"ack"`                 // 可选确认号，用于可靠传输
	Routing           *GRERouting         `json:"routing"`             // 可选源路由信息，支持多级路由
}

// NewGREChannel 从gopacket GRE层创建GREChannel
// 将gopacket库中的GRE层数据转换为内部使用的GREChannel结构体
//
// 参数:
//   - gre: *layers.GRE类型，gopacket解析的GRE层数据
//
// 返回值:
//   - *GREChannel: 转换后的GRE通道信息结构体
//
// 功能说明:
//  1. 提取GRE头部所有字段，包括标志位和可选字段
//  2. 递归处理源路由信息（如果存在）
//  3. 保持与原始数据的一致性
//  4. 便于后续序列化和存储
func NewGREChannel(gre *layers.GRE) *GREChannel {
	return &GREChannel{
		ChecksumPresent:   gre.ChecksumPresent,
		RoutingPresent:    gre.RoutingPresent,
		KeyPresent:        gre.KeyPresent,
		SeqPresent:        gre.SeqPresent,
		StrictSourceRoute: gre.StrictSourceRoute,
		AckPresent:        gre.AckPresent,
		CRecursionControl: gre.RecursionControl,
		Flags:             gre.Flags,
		Version:           gre.Version,
		Protocol:          gre.Protocol,
		Checksum:          gre.Checksum,
		Offset:            gre.Offset,
		Key:               gre.Key,
		Seq:               gre.Seq,
		Ack:               gre.Ack,
		Routing:           NewGRERouting(gre.GRERouting),
	}
}

// GeneveOption 表示Geneve选项信息
// Geneve（Generic Network Virtualization Encapsulation）协议支持可扩展的选项字段
// 该结构体存储单个Geneve选项的详细信息，包括类别、类型、标志和数据
type GeneveOption struct {
	Class  uint16 `json:"class"`  // 选项类别，标识选项的用途域
	Type   uint8  `json:"type"`   // 选项类型，在类别内唯一标识选项
	Flags  uint8  `json:"flags"`  // 选项标志位，控制选项的处理行为
	Length uint8  `json:"length"` // 选项数据长度，以4字节为单位（不包括头部）
	Data   []byte `json:"data"`   // 选项数据内容，具体格式由类别和类型决定
}

// NewGeneveOption 从gopacket Geneve选项创建GeneveOption
// 将gopacket库中的Geneve选项数据转换为内部使用的GeneveOption结构体
//
// 参数:
//   - geneve: *layers.GeneveOption类型，gopacket解析的Geneve选项数据
//
// 返回值:
//   - *GeneveOption: 转换后的Geneve选项信息结构体
//
// 功能说明:
//  1. 提取Geneve选项的所有字段
//  2. 保持与原始数据的一致性
//  3. 便于后续序列化和存储
func NewGeneveOption(geneve *layers.GeneveOption) *GeneveOption {
	return &GeneveOption{
		Class:  geneve.Class,
		Type:   geneve.Type,
		Flags:  geneve.Flags,
		Length: geneve.Length,
		Data:   geneve.Data,
	}
}

// GeneveChannel 表示Geneve隧道通道信息
// Geneve是一种通用的网络虚拟化封装协议，支持灵活的选项扩展
// 该结构体存储Geneve头部的主要字段，包括版本、协议类型、VNI和选项列表
type GeneveChannel struct {
	Version        uint8               `json:"version"`         // Geneve协议版本，目前通常为0
	OptionsLength  uint8               `json:"options_length"`  // 选项部分总长度，以4字节为单位
	OAMPacket      bool                `json:"oam_packet"`      // OAM数据包标志，表示这是操作维护管理数据包
	CriticalOption bool                `json:"critical_option"` // 关键选项标志，表示包含必须处理的选项
	Protocol       layers.EthernetType `json:"protocol"`        // 封装协议类型，标识内层协议（如IPv4、IPv6等）
	VNI            uint32              `json:"vni"`             // 虚拟网络标识符，24位，用于标识虚拟网络
	Options        []*GeneveOption     `json:"options"`         // Geneve选项列表，支持多个可扩展选项
}

// NewGeneveChannel 从gopacket Geneve层创建GeneveChannel
// 将gopacket库中的Geneve层数据转换为内部使用的GeneveChannel结构体
//
// 参数:
//   - geneve: *layers.Geneve类型，gopacket解析的Geneve层数据
//
// 返回值:
//   - *GeneveChannel: 转换后的Geneve通道信息结构体
//
// 功能说明:
//  1. 提取Geneve头部所有字段
//  2. 转换所有Geneve选项为内部结构体
//  3. 保持与原始数据的一致性
//  4. 便于后续序列化和存储
func NewGeneveChannel(geneve *layers.Geneve) *GeneveChannel {
	options := make([]*GeneveOption, 0, len(geneve.Options))
	for _, opt := range geneve.Options {
		options = append(options, NewGeneveOption(opt))
	}
	return &GeneveChannel{
		Version:        geneve.Version,
		OptionsLength:  geneve.OptionsLength,
		OAMPacket:      geneve.OAMPacket,
		CriticalOption: geneve.CriticalOption,
		Protocol:       geneve.Protocol,
		VNI:            geneve.VNI,
		Options:        options,
	}
}

// MPLSChannel 表示MPLS标签通道信息
// MPLS（Multiprotocol Label Switching）是一种数据包转发技术，使用标签进行快速交换
// 该结构体存储MPLS标签栈中的单个标签信息，包括标签值、流量类别、栈底标志和TTL
type MPLSChannel struct {
	Label        uint32 `json:"label"`         // MPLS标签值，20位，用于数据包转发
	TrafficClass uint8  `json:"traffic_class"` // 流量类别，3位，用于服务质量（QoS）区分
	StackBottom  bool   `json:"stack_bottom"`  // 栈底标志，1位，表示这是标签栈的最后一个标签
	TTL          uint8  `json:"ttl"`           // 生存时间，8位，防止数据包在网络中无限循环
}

// NewMPLSChannel 从gopacket MPLS层创建MPLSChannel
// 将gopacket库中的MPLS层数据转换为内部使用的MPLSChannel结构体
//
// 参数:
//   - mpls: *layers.MPLS类型，gopacket解析的MPLS层数据
//
// 返回值:
//   - *MPLSChannel: 转换后的MPLS通道信息结构体
//
// 功能说明:
//  1. 提取MPLS标签的所有字段
//  2. 保持与原始数据的一致性
//  3. 便于后续序列化和存储
//  4. 支持MPLS标签栈的递归解析（通过builder.go中的ExtractLayerInfos函数）
func NewMPLSChannel(mpls *layers.MPLS) *MPLSChannel {
	return &MPLSChannel{
		Label:        mpls.Label,
		TrafficClass: mpls.TrafficClass,
		StackBottom:  mpls.StackBottom,
		TTL:          mpls.TTL,
	}
}
