package network

import (
	"database/sql/driver"
	"encoding/json"
	"net"

	"github.com/gopacket/gopacket/layers"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// IPv4Options 表示IPv4协议头部中的选项列表
// IPv4选项是IPv4协议的可选扩展字段，用于提供额外的控制功能
// 常见的IPv4选项包括：记录路由（Record Route）、时间戳（Timestamp）、
// 松散源路由（Loose Source Route）、严格源路由（Strict Source Route）等
//
// 类型定义:
//   - 基于gopacket的layers.IPv4Option切片，保持与底层库的兼容性
//   - 实现了数据库序列化接口（driver.Valuer）和反序列化接口（sql.Scanner）
//   - 支持JSON格式的存储和传输
//
// 使用场景:
//   - 存储和查询包含IPv4选项的网络数据包
//   - 分析网络路径、延迟等高级网络特性
//   - 支持特殊路由需求的网络流量分析
type IPv4Options []layers.IPv4Option

// Value 实现driver.Valuer接口，将IPv4Options序列化为数据库存储格式
// 该方法将IPv4选项列表转换为JSON格式的字节数组，便于在数据库中存储
//
// 参数:
//   - 无
//
// 返回值:
//   - driver.Value: 数据库可存储的值类型，实际为[]byte类型
//   - error: 序列化过程中发生的错误，如果成功则为nil
//
// 异常:
//   - 如果JSON序列化失败，返回相应的错误
//
// 示例:
//   - 将IPv4Options序列化为JSON字符串存储在ClickHouse的String类型列中
func (i IPv4Options) Value() (driver.Value, error) {
	return json.Marshal(i)
}

// Scan 实现sql.Scanner接口，从数据库读取值并反序列化为IPv4Options
// 该方法支持从[]byte或string类型的数据库值反序列化为IPv4Options结构
//
// 参数:
//   - value: 数据库读取的原始值，可能为[]byte、string或nil
//
// 返回值:
//   - error: 反序列化过程中发生的错误，如果成功则为nil
//
// 异常:
//   - 如果value类型不是[]byte或string，返回类型错误
//   - 如果JSON反序列化失败，返回相应的错误
//
// 支持的数据类型:
//   - nil: 清空IPv4Options
//   - []byte: 直接进行JSON反序列化
//   - string: 转换为[]byte后进行JSON反序列化
func (i *IPv4Options) Scan(value any) error {
	if value == nil {
		*i = nil
		return nil
	}
	var bytes []byte
	switch raw := value.(type) {
	case []byte:
		bytes = raw
	case string:
		bytes = []byte(raw)
	default:
		return errors.Newf("Ipv4_options scan failed, except []byte, got %T", value)
	}
	return json.Unmarshal(bytes, i)
}

// IPv4Headers 表示IPv4协议头部信息
// IPv4（Internet Protocol version 4）是互联网协议的第4版，是当前互联网最主要的网络层协议
// 该结构体封装了IPv4协议头部的所有字段，用于存储、查询和分析网络数据包
//
// 字段说明:
//   - IPv4Version:    协议版本号，固定为4（4位）
//   - IPv4IHL:        头部长度（Internet Header Length），以32位字为单位（4位）
//   - IPv4TOS:        服务类型（Type of Service），用于QoS优先级控制（8位）
//   - IPv4Length:     总长度（Total Length），包括头部和数据，以字节为单位（16位）
//   - IPv4Id:         标识符（Identification），用于分片重组（16位）
//   - IPv4Flags:      标志位（Flags），包含DF（Don't Fragment）和MF（More Fragments）（3位）
//   - IPv4FragOffset: 分片偏移（Fragment Offset），以8字节为单位（13位）
//   - IPv4TTL:        生存时间（Time To Live），防止数据包无限循环（8位）
//   - IPv4IPProtocol: 上层协议类型（Protocol），如TCP(6)、UDP(17)、ICMP(1)等（8位）
//   - IPv4Checksum:   头部校验和（Header Checksum），用于错误检测（16位）
//   - IPv4Src:        源IP地址（Source Address）（32位）
//   - IPv4Dst:        目的IP地址（Destination Address）（32位）
//   - IPv4Options:    IPv4选项列表，可选字段
//   - IPv4Padding:    填充字节，确保头部长度为32位的整数倍
//
// 数据库映射:
//   - 使用GORM标签定义数据库约束和列类型
//   - 使用JSON标签支持序列化和反序列化
//   - 在ClickHouse中使用ZSTD(3)压缩算法优化存储
//   - 所有字段标记为NOT NULL，确保数据完整性
//
// 网络分析应用:
//   - 流量统计：按源/目的IP、协议类型进行聚合分析
//   - 安全监控：检测异常TTL值、分片攻击等
//   - 路径追踪：通过TTL递减分析网络路径
//   - QoS分析：根据TOS字段分析服务质量
type IPv4Headers struct {
	IPv4Version    uint8               `gorm:"type:UInt8 CODEC(ZSTD(3))" json:"ipv4_version"`
	IPv4IHL        uint8               `gorm:"type:UInt8 CODEC(ZSTD(3))" json:"ipv4_ihl"`
	IPv4TOS        uint8               `gorm:"type:UInt8 CODEC(ZSTD(3))" json:"ipv4_tos"`
	IPv4Length     uint16              `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"ipv4_length"`
	IPv4Id         uint16              `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"ipv4_id"`
	IPv4Flags      layers.IPv4Flag     `gorm:"type:UInt8 CODEC(ZSTD(3))" json:"ipv4_flags"`
	IPv4FragOffset uint16              `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"ipv4_frag_offset"`
	IPv4TTL        uint8               `gorm:"type:UInt8 CODEC(ZSTD(3))" json:"ipv4_ttl"`
	IPv4IPProtocol layers.IPProtocol   `gorm:"type:UInt8 CODEC(ZSTD(3))" json:"ipv4_ip_protocol"`
	IPv4Checksum   uint16              `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"ipv4_checksum"`
	IPv4Src        serializer.IPv4Addr `gorm:"type:FixedString(4)" json:"ipv4_src"`
	IPv4Dst        serializer.IPv4Addr `gorm:"type:FixedString(4)" json:"ipv4_dst"`
	IPv4Options    IPv4Options         `gorm:"type:String CODEC(ZSTD(3))" json:"ipv4_options"`
	IPv4Padding    []byte              `gorm:"type:String CODEC(ZSTD(3))" json:"ipv4_padding"`
}

// NewIPv4Headers 从gopacket IPv4层创建IPv4Headers结构体
// 该函数是gopacket库与内部数据模型之间的桥梁，用于将捕获的原始IPv4数据包
// 转换为内部结构化的数据模型，便于存储、查询和分析
//
// 参数:
//   - ipv4: 指向gopacket layers.IPv4层的指针，包含原始IPv4头部信息
//
// 返回值:
//   - *IPv4Headers: 新创建的IPv4Headers结构体指针，包含所有IPv4头部字段
//
// 转换过程:
//   - 直接映射字段：Version、IHL、TOS、Length等基础字段
//   - 类型转换：将net.IP转换为serializer.IPv4Addr类型
//   - 切片转换：将[]layers.IPv4Option转换为IPv4Options类型
//   - 字节复制：Padding字段直接复制字节切片
//
// 性能考虑:
//   - 使用指针传递避免大结构体拷贝
//   - 直接类型转换而非深拷贝，提高性能
//   - 保持与gopacket库的数据一致性
//
// 使用场景:
//   - 网络数据包捕获和解析流水线
//   - PCAP文件导入和转换
//   - 实时网络流量分析
//
// 示例:
//   - 在PacketModelBuilder中用于构建网络层模型
//   - 在ETL任务中用于数据转换
func NewIPv4Headers(ipv4 *layers.IPv4) *IPv4Headers {
	return &IPv4Headers{
		IPv4Version:    ipv4.Version,
		IPv4IHL:        ipv4.IHL,
		IPv4TOS:        ipv4.TOS,
		IPv4Length:     ipv4.Length,
		IPv4Id:         ipv4.Id,
		IPv4Flags:      ipv4.Flags,
		IPv4FragOffset: ipv4.FragOffset,
		IPv4TTL:        ipv4.TTL,
		IPv4IPProtocol: ipv4.Protocol,
		IPv4Checksum:   ipv4.Checksum,
		IPv4Src:        serializer.IPv4Addr(ipv4.SrcIP),
		IPv4Dst:        serializer.IPv4Addr(ipv4.DstIP),
		IPv4Options:    IPv4Options(ipv4.Options),
		IPv4Padding:    ipv4.Padding,
	}
}

// SrcAddr 返回IPv4Headers中的源IP地址
// 该方法提供类型安全的IP地址访问，将内部存储的serializer.IPv4Addr类型
// 转换为标准库的net.IP类型，便于进行网络操作和比较
//
// 接收者:
//   - i: IPv4Headers结构体指针
//
// 返回值:
//   - net.IP: 标准库的IP地址类型，包含源IP地址
//
// 类型转换:
//   - serializer.IPv4Addr是[]byte的别名，与net.IP类型兼容
//   - 转换过程是零拷贝的，仅进行类型断言
//
// 使用场景:
//   - 网络流量分析和统计
//   - 访问控制列表（ACL）检查
//   - 地理定位和IP归属地查询
//   - 网络拓扑分析
//
// 示例:
//   - 按源IP地址进行流量聚合
//   - 检测特定IP地址的异常行为
//   - 实现基于IP地址的路由策略
func (i *IPv4Headers) SrcAddr() net.IP {
	return net.IP(i.IPv4Src)
}

// DstAddr 返回IPv4Headers中的目的IP地址
// 该方法提供类型安全的IP地址访问，将内部存储的serializer.IPv4Addr类型
// 转换为标准库的net.IP类型，便于进行网络操作和比较
//
// 接收者:
//   - i: IPv4Headers结构体指针
//
// 返回值:
//   - net.IP: 标准库的IP地址类型，包含目的IP地址
//
// 类型转换:
//   - serializer.IPv4Addr是[]byte的别名，与net.IP类型兼容
//   - 转换过程是零拷贝的，仅进行类型断言
//
// 使用场景:
//   - 目标服务器流量分析
//   - 服务依赖关系发现
//   - 网络延迟和性能监控
//   - 分布式追踪和调用链分析
//
// 示例:
//   - 按目的IP地址统计服务访问量
//   - 分析服务间的通信模式
//   - 检测DDoS攻击目标
func (i *IPv4Headers) DstAddr() net.IP {
	return net.IP(i.IPv4Dst)
}

// PacketIPv4 表示完整的IPv4网络数据包
// 该结构体组合了以太网层（链路层）和IPv4层（网络层）的头部信息
// 用于存储和分析从网络捕获的原始IPv4数据包
//
// 结构组成:
//   - PacketEthernet: 以太网层头部信息，包含MAC地址、以太网类型等
//   - IPv4Headers:    IPv4网络层头部信息，包含IP地址、协议类型、TTL等
//
// 数据库特性:
//   - 使用MergeTree引擎存储，支持高性能查询
//   - 按时间分区（每小时），优化时间范围查询性能
//   - 设置TTL（生存时间），自动清理过期数据
//   - 复合排序键，优化常见查询模式
//
// 查询优化:
//   - 排序键包含namespace、capture、version、session_id等维度
//   - 支持按源/目的IP、协议类型、MAC地址等字段快速过滤
//   - 时间戳分区支持高效的时间范围查询
//
// 使用场景:
//   - 原始网络数据包存储和分析
//   - 网络安全事件调查和取证
//   - 网络流量监控和统计
//   - 协议分析和故障诊断
type PacketIPv4 struct {
	PacketEthernet
	IPv4Headers
	Payload *serializer.Crypto `gorm:"type:String CODEC(ZSTD(3));serializer:crypto" json:"payload" secure:"mode=\"aes-gcm\",offset=16429,length=32"`
}

// FusedIPv4 表示融合的IPv4数据
// 该结构体用于存储经过ETL处理后的融合数据，相比原始数据包进行了聚合和优化
// 融合数据通常包含统计信息、聚合结果或经过处理的业务数据
//
// 结构组成:
//   - FusedEthernet: 融合的以太网层数据，可能包含统计信息
//   - IPv4Headers:   IPv4网络层头部信息
//
// 数据库特性:
//   - 使用MergeTree引擎存储，支持高性能查询
//   - 按时间分区（每小时），优化时间范围查询性能
//   - 设置更长的TTL（7天），保留更长时间的历史数据
//   - 复合排序键，优化业务查询模式
//
// 数据特点:
//   - 数据量通常比原始数据包小
//   - 包含业务相关的聚合信息
//   - 适合长期存储和历史分析
//
// 使用场景:
//   - 业务指标监控和报表
//   - 长期趋势分析
//   - 容量规划和性能优化
//   - 合规性审计和报告
type FusedIPv4 struct {
	FusedEthernet
	IPv4Headers
	Payload *serializer.Crypto `gorm:"type:String CODEC(ZSTD(3));serializer:crypto" json:"payload" secure:"mode=\"aes-gcm\",offset=60580,length=32"`
}
