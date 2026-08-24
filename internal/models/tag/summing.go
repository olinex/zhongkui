// Package risk 提供风险分析相关的数据模型和聚合逻辑。
// 本文件定义了基于 ClickHouse SummingMergeTree 引擎的标签聚合模型，
// 通过物化视图自动从网络传输数据中聚合风险标签统计信息。
package tag

import (
	"time"

	"zhongkui.com/zhongkui/internal/models/network"
)

// TagSumming 表示标签聚合统计模型，对应 ClickHouse 中的 SummingMergeTree 表
// 该模型用于按分钟粒度聚合风险标签的命中次数，支持风险趋势分析和实时监控。
//
// 聚合维度:
//   - namespace: 命名空间，多租户隔离的首要分组维度
//   - minute: 按分钟对齐的时间窗口
//   - model:  目标模型类型键（如 zhongkui.com/zhongkui/internal/models/network.HttpTransport），
//     标识标签由哪个模型的识别规则产出，与 TagRule.Model 取值一致
//   - dimension: 标签维度类型（如 "fpTcpClientSig"、"fpTcpServerSig"）
//   - item:      标签维度类型对应的具体值（如指纹签名串）
//   - tags:   风险标签组合（去重排序后的标签数组）
//
// 聚合指标:
//   - cnt: 命中次数，SummingMergeTree 引擎会自动对相同主键的记录求和
//
// 表引擎特性:
//   - 使用 SummingMergeTree 引擎，自动合并相同排序键的行并累加数值列
//   - 按小时分区，便于数据过期管理和查询优化
//   - 设置 72 小时 TTL，超期数据自动清理
//   - 使用 DoubleDelta 编码压缩时间列，ZSTD(3) 压缩其他列
type TagSumming struct {
	network.TagsMeta
	// Namespace 命名空间，多租户隔离字段，聚合时按命名空间分组
	Namespace string `gorm:"type:LowCardinality(String) CODEC(ZSTD(3))" json:"namespace"`
	// Minute 聚合时间窗口的起始时间（分钟对齐），使用 DateTime64(6) 精度
	Minute time.Time `gorm:"type:DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3))" json:"minute"`
	// Model 目标模型类型键，标识标签由哪个模型的识别规则产出（如 zhongkui.com/zhongkui/internal/models/network.HttpTransport）
	Model string `gorm:"type:String CODEC(ZSTD(3))" json:"model"`
	// Dimension 标签维度类型，标识标签命中的指纹维度（如 "fpTcpClientSig"、"fpTcpServerSig"）
	Dimension string `gorm:"type:String CODEC(ZSTD(3))" json:"dimension"`
	// Item 标签维度类型对应的具体值（如具体的指纹签名串）
	Item string `gorm:"type:String CODEC(ZSTD(3))" json:"item"`
	// Cnt 命中次数，SummingMergeTree 引擎会自动对相同排序键的 cnt 值求和
	Cnt uint `gorm:"type:UInt32" json:"cnt"`
}
