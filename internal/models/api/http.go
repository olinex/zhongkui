// Package api 定义接口访问统计的持久化模型（ClickHouse 表结构映射），
// 涵盖接口访问元信息聚合记录与接口路径节点树的存储结构。
package api

import (
	"time"

	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// ApiNodeTtlHours 接口路径节点记录的默认过期时长（小时）。
const (
	ApiNodeTtlHours = 24
)

// HttpApiMeta 表示接口访问的元信息聚合记录。
type HttpApiMeta struct {
	// Batch 写入批号，标识同一次写入的数据批次
	Batch uint32 `gorm:"type:UInt32 CODEC(ZSTD(3))"`
	// ID 接口的唯一标识（UUID）
	ID serializer.HexUUID `gorm:"type:FixedString(16) CODEC(ZSTD(3))"`
	// Method HTTP 请求方法（如 GET、POST）
	Method string `gorm:"type:LowCardinality(String) CODEC(ZSTD(3))"`
	// Api 接口路径
	Api string `gorm:"type:String CODEC(ZSTD(3))"`
	// Hits 命中次数
	Hits int64 `gorm:"type:UInt64 CODEC(ZSTD(3))"`
	// StatusCodes 响应的状态码列表
	StatusCodes []uint64 `gorm:"type:Array(UInt64) CODEC(ZSTD(3))"`
	// CreateTime 记录首次创建时间
	CreateTime time.Time `gorm:"type:DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3))"`
	// LastHitTime 最近一次命中时间
	LastHitTime time.Time `gorm:"type:DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3))"`
}

// HttpApiNode 表示接口路径按分段构建的节点树中的单个节点记录。
type HttpApiNode struct {
	// Batch 写入批号，标识同一次写入的数据批次
	Batch uint32 `gorm:"type:UInt32 CODEC(ZSTD(3))"`
	// ID 节点的唯一标识（UUID）
	ID serializer.HexUUID `gorm:"type:FixedString(16) CODEC(ZSTD(3))"`
	// Parent 父节点的唯一标识（UUID）
	Parent serializer.HexUUID `gorm:"type:FixedString(16) CODEC(ZSTD(3))"`
	// Sequence 同级节点中的排列顺序
	Sequence uint32 `gorm:"type:UInt32 CODEC(ZSTD(3))"`
	// NType 节点类型
	NType int64 `gorm:"type:UInt64 CODEC(ZSTD(3))"`
	// VType 节点值的类型
	VType int64 `gorm:"type:UInt64 CODEC(ZSTD(3))"`
	// Key 节点键名（路径分段）
	Key string `gorm:"type:String CODEC(ZSTD(3))"`
	// Depth 节点在树中的深度
	Depth int64 `gorm:"type:UInt64 CODEC(ZSTD(3))"`
	// Hits 节点命中次数
	Hits int64 `gorm:"type:UInt64 CODEC(ZSTD(3))"`
	// Variants 该节点的变体信息
	Variants string `gorm:"type:String CODEC(ZSTD(3))"`
	// CreateTime 节点首次创建时间
	CreateTime time.Time `gorm:"type:DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3))"`
	// LastHitTime 节点最近一次命中时间
	LastHitTime time.Time `gorm:"type:DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3))"`
	// ChildsPopularityN 子节点热度统计的样本数
	ChildsPopularityN string `gorm:"type:String CODEC(ZSTD(3))"`
	// ChildsPopularityLineSum 子节点热度统计的线性和
	ChildsPopularityLineSum string `gorm:"type:String CODEC(ZSTD(3))"`
	// ChildsPopularitySquareSum 子节点热度统计的平方和
	ChildsPopularitySquareSum string `gorm:"type:String CODEC(ZSTD(3))"`
	// ChildsDispersionN 子节点离散度统计的样本数
	ChildsDispersionN string `gorm:"type:String CODEC(ZSTD(3))"`
	// ChildsDispersionLineSum 子节点离散度统计的线性和
	ChildsDispersionLineSum string `gorm:"type:String CODEC(ZSTD(3))"`
	// ChildsDispersionSquareSum 子节点离散度统计的平方和
	ChildsDispersionSquareSum string `gorm:"type:String CODEC(ZSTD(3))"`
	// ChildsConcentrationN 子节点集中度统计的样本数
	ChildsConcentrationN string `gorm:"type:String CODEC(ZSTD(3))"`
	// ChildsConcentrationLineSum 子节点集中度统计的线性和
	ChildsConcentrationLineSum string `gorm:"type:String CODEC(ZSTD(3))"`
	// ChildsConcentrationSquareSum 子节点集中度统计的平方和
	ChildsConcentrationSquareSum string `gorm:"type:String CODEC(ZSTD(3))"`
}
