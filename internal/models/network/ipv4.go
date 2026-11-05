package network

import (
	"database/sql/driver"
	"encoding/json"
	"net"

	"github.com/gopacket/gopacket/layers"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

var (
	_ driver.Valuer = (*IPv4Options)(nil)
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
	if i == nil {
		return nil, nil
	}
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
		return errors.Newf("Ipv4_options scan failed, except []byte, got %T", value)
	}
	return json.Unmarshal(bytes, i)
}

// IPv4Headers 表示IPv4协议头部信息
// IPv4（Internet Protocol version 4）是互联网协议的第4版，是当前互联网最主要的网络层协议
// 该结构体封装了IPv4协议头部的所有字段，用于存储、查询和分析网络数据包
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
	// IPv4Version 协议版本号，固定为 4（4 位）
	IPv4Version uint8 `gorm:"type:UInt8 CODEC(ZSTD(3))" json:"ipv4Version"`
	// IPv4IHL 头部长度（IHL），以 32 位字为单位（4 位）
	IPv4IHL uint8 `gorm:"type:UInt8 CODEC(ZSTD(3))" json:"ipv4Ihl"`
	// IPv4TOS 服务类型（TOS），用于 QoS 优先级控制（8 位）
	IPv4TOS uint8 `gorm:"type:UInt8 CODEC(ZSTD(3))" json:"ipv4Tos"`
	// IPv4Length 总长度，含头部与数据，单位字节（16 位）
	IPv4Length uint16 `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"ipv4Length"`
	// IPv4ID 标识符，用于分片重组（16 位）
	IPv4ID uint16 `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"ipv4ID"`
	// IPv4Flags 标志位，包含 DF、MF（3 位）
	IPv4Flags layers.IPv4Flag `gorm:"type:UInt8 CODEC(ZSTD(3))" json:"ipv4Flags"`
	// IPv4FragOffset 分片偏移，以 8 字节为单位（13 位）
	IPv4FragOffset uint16 `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"ipv4FragOffset"`
	// IPv4TTL 生存时间，防止数据包无限循环（8 位）
	IPv4TTL uint8 `gorm:"type:UInt8 CODEC(ZSTD(3))" json:"ipv4Ttl"`
	// IPv4IPProtocol 上层协议类型，如 TCP=6、UDP=17、ICMP=1（8 位）
	IPv4IPProtocol layers.IPProtocol `gorm:"type:UInt8 CODEC(ZSTD(3))" json:"ipv4IpProtocol"`
	// IPv4Checksum 头部校验和，用于错误检测（16 位）
	IPv4Checksum uint16 `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"ipv4Checksum"`
	// IPv4Src 源 IP 地址（32 位）
	IPv4Src serializer.IPv4Addr `gorm:"type:IPv4" json:"ipv4Src"`
	// IPv4Dst 目的 IP 地址（32 位）
	IPv4Dst serializer.IPv4Addr `gorm:"type:IPv4" json:"ipv4Dst"`
	// IPv4Options IPv4 选项列表，可选字段
	IPv4Options IPv4Options `gorm:"type:String CODEC(ZSTD(3))" json:"ipv4Options"`
	// IPv4Padding 填充字节，确保头部长度为 32 位的整数倍
	IPv4Padding string `gorm:"type:String CODEC(ZSTD(3))" json:"ipv4Padding"`
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
		IPv4ID:         ipv4.Id,
		IPv4Flags:      ipv4.Flags,
		IPv4FragOffset: ipv4.FragOffset,
		IPv4TTL:        ipv4.TTL,
		IPv4IPProtocol: ipv4.Protocol,
		IPv4Checksum:   ipv4.Checksum,
		IPv4Src:        serializer.IPv4Addr(ipv4.SrcIP),
		IPv4Dst:        serializer.IPv4Addr(ipv4.DstIP),
		IPv4Options:    IPv4Options(ipv4.Options),
		IPv4Padding:    string(ipv4.Padding),
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
	// 加密存储的 IP 载荷
	Payload *serializer.Crypto `gorm:"type:String CODEC(ZSTD(3));serializer:crypto" json:"payload" secure:"mode=\"aes-gcm\",offset=16429,length=32"`
}
