package network

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/gopacket/gopacket"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

var (
	_ driver.Valuer = (*LayerStack)(nil)
)

const (
	// LayerLevelDecodeFailure 表示解码失败协议层（OSI第0层）
	LayerLevelDecodeFailure LayerLevel = 0

	// LayerLevelLink 表示链路层（OSI第2层）
	LayerLevelLink LayerLevel = 2

	// LayerLevelNetwork 表示网络层（OSI第3层）
	LayerLevelNetwork LayerLevel = 3

	// LayerLevelTransport 表示传输层（OSI第4层）
	LayerLevelTransport LayerLevel = 4

	// LayerLevelApplication 表示应用层（OSI第7层）
	LayerLevelApplication LayerLevel = 7
)

// LayerLevel 表示网络协议栈的层次级别
// 使用OSI模型中的层级定义，但跳过了表示层和会话层（第5-6层）
type LayerLevel uint8

// Next 返回下一个协议层级别
// 根据OSI模型，链路层(2) -> 网络层(3) -> 传输层(4) -> 应用层(7)
// 注意：跳过了表示层(5)和会话层(6)
func (l LayerLevel) Next() LayerLevel {
	if l == LayerLevelApplication {
		panic("application layer next level is not defined")
	}
	if l == LayerLevelTransport {
		return LayerLevel(l + 3)
	}
	return LayerLevel(l + 1)
}

// LayerInfo 表示网络层的详细信息
// 包含协议层级、协议类型和对应的头部信息
type LayerInfo struct {
	Level   LayerLevel         `json:"level"`   // 协议层级（如链路层、网络层等）
	Proto   gopacket.LayerType `json:"proto"`   // 协议类型（如Ethernet、IPv4、TCP等）
	Headers any                `json:"headers"` // 协议头部信息，具体类型由Proto决定
}

// LayerStack 表示网络层的堆栈结构
// 按照从底层到高层的顺序存储网络协议层信息
type LayerStack []*LayerInfo

// Value 实现数据库序列化接口
// 将LayerStack序列化为JSON字符串存储到数据库
func (l LayerStack) Value() (driver.Value, error) {
	if l == nil {
		return "[]", nil
	}
	if bytes, err := json.Marshal(l); err != nil {
		panic(err)
		// return nil, errors.Cause(err)
	} else {
		return string(bytes), nil
	}
}

// Scan 实现数据库反序列化接口
// 从数据库读取JSON字符串并反序列化为LayerStack
func (l *LayerStack) Scan(value any) error {
	if value == nil {
		*l = nil
		return nil
	}
	var bytes []byte
	switch raw := value.(type) {
	case []byte:
		if len(raw) == 0 {
			*l = nil
			return nil
		}
		bytes = raw
	case string:
		if raw == "" {
			*l = nil
			return nil
		}
		bytes = []byte(raw)
	default:
		return errors.Newf("Layer_stack scan failed, except []byte, got %T", value)
	}
	return json.Unmarshal(bytes, l)
}

// FrameInfo 表示数据包的帧信息
// 包含时间戳和对应的网络层堆栈信息
type FrameInfo struct {
	Timestamp  time.Time  `json:"timestamp"`   // 数据包捕获时间戳
	LayerStack LayerStack `json:"layerStack"` // 网络层堆栈信息
}

// BaseMeta 表示基础元数据信息
// 所有网络数据模型的基础字段，用于数据分区和查询优化
type BaseMeta struct {
	// WARN：gorm 的 clickhouse driver 有着严重的 bug
	// 在进行FindInBatches时， 若返回的数据数量超过 batchSize，会根据主键排序
	// 若此时表没有主键， 会导致报错 primary key required，因此 clickhouse 表必须设置一个伪主键，且不能是联合主键
	Namespace string             `gorm:"type:LowCardinality(String) CODEC(ZSTD(3));primaryKey" json:"namespace"`
	Capture   string             `gorm:"type:LowCardinality(String) CODEC(ZSTD(3))" json:"capture"`
	Version   string             `gorm:"type:LowCardinality(String) CODEC(ZSTD(3))" json:"version"`
	SessionID serializer.HexUUID `gorm:"type:UUID CODEC(ZSTD(3))" json:"sessionId"`
}

// RowMeta 行唯一标识元数据
// 为流量数据行提供全局唯一 ID，供行详情接口按 namespace + id 定位单行
type RowMeta struct {
	ID serializer.HexUUID `gorm:"type:UUID CODEC(ZSTD(3))" json:"id"` // 全局唯一 ID
}

// TimestampMeta 表示时间元数据
// 包含采集时间与入库时间，使用高精度时间格式存储
type TimestampMeta struct {
	Timestamp time.Time `gorm:"type:DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3))" json:"timestamp"`  // 采集时间：数据包被捕获的时刻
	CreatedAt time.Time `gorm:"type:DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3))" json:"createdAt"` // 入库时间：数据写入数据库的时刻
}

type TraceMeta struct {
	TraceID serializer.HexUUID `gorm:"type:UUID CODEC(ZSTD(3))" json:"traceId"`
}

// DirectionMeta 表示数据包方向元数据
// Cli2Srv 标记该数据包是否为客户端到服务端方向（true表示客户端到服务端，false表示服务端到客户端）
type DirectionMeta struct {
	Cli2Srv bool `gorm:"column:cli_2_srv;type:Bool" json:"cli2Srv"`
}

// CaptureMeta 表示捕获元数据
// 包含数据包捕获时的状态信息
type CaptureMeta struct {
	Truncated     bool   `gorm:"type:Bool" json:"truncated"`
	CaptureLength uint32 `gorm:"type:UInt32 CODEC(ZSTD(3))" json:"captureLength"`
}

// PacketMeta 表示数据包元数据
// 包含原始数据包的网络层堆栈信息
type PacketMeta struct {
	LayerStack LayerStack `gorm:"type:String CODEC(ZSTD(3))" json:"layerStack"`
}

type TagsMeta struct {
	// Tags 风险标签组合，存储去重排序后的标签数组
	// ClickHouse 中使用 Array(String) 类型存储
	Tags []string `gorm:"type:Array(String)" json:"tags"`
}

// AsPacket 表示数据包的结构
// 用于存储原始网络数据包，包含完整的元数据和载荷
type AsPacket struct {
	BaseMeta
	TimestampMeta
	CaptureMeta
	PacketMeta
}
