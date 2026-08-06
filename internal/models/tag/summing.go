// Package risk 提供风险分析相关的数据模型和聚合逻辑。
// 本文件定义了基于 ClickHouse SummingMergeTree 引擎的标签聚合模型，
// 通过物化视图自动从网络传输数据中聚合风险标签统计信息。
package tag

import (
	"fmt"
	"time"

	"zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// SummingTTLHours 定义聚合数据在 ClickHouse 中的保留时间（小时）
// 默认保留 3 天（72 小时），超期数据由 ClickHouse TTL 机制自动删除
const (
	SummingTTLHours = 3 * 24
)

// TagSumming 表示标签聚合统计模型，对应 ClickHouse 中的 SummingMergeTree 表
// 该模型用于按分钟粒度聚合风险标签的命中次数，支持风险趋势分析和实时监控。
//
// 聚合维度:
//   - minute: 按分钟对齐的时间窗口
//   - source: 标签来源的数据源（如 "HttpTransport"）
//   - type:   标签维度类型（如 "fp_tcp_client_sig"、"fp_tcp_server_sig"）
//   - item:   标签维度类型对应的具体值（如指纹签名串）
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
	// Minute 聚合时间窗口的起始时间（分钟对齐），使用 DateTime64(6) 精度
	Minute time.Time `gorm:"type:DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3))" json:"minute"`
	// Source 标签来源的数据源，标识标签由哪个数据源聚合而来（如 "HttpTransport"）
	Source string `gorm:"type:String CODEC(ZSTD(3))" json:"source"`
	// Type 标签维度类型，标识标签命中的指纹维度（如 "fp_tcp_client_sig"、"fp_tcp_server_sig"）
	Type string `gorm:"type:String CODEC(ZSTD(3))" json:"type"`
	// Item 标签维度类型对应的具体值（如具体的指纹签名串）
	Item string `gorm:"type:String CODEC(ZSTD(3))" json:"item"`
	// Cnt 命中次数，SummingMergeTree 引擎会自动对相同排序键的 cnt 值求和
	Cnt uint `gorm:"type:UInt32" json:"cnt"`
}

// TableOptions 返回 ClickHouse 表的创建选项
// 配置 SummingMergeTree 引擎的排序键、分区策略和数据过期时间
//
// 返回值:
//   - map[string]any: 包含 gorm:table_options 键的选项映射
//
// 引擎配置说明:
//   - Engine = SummingMergeTree(): 使用求和合并树引擎，自动合并并累加数值列
//   - ORDER BY (minute, source, type, item, tags): 按 minute、source、type、item、tags 排序，作为主键和去重依据
//   - PARTITION BY toStartOfHour(minute): 按小时分区，优化查询和过期管理
//   - TTL minute + INTERVAL 72 HOUR DELETE: 数据保留 72 小时后自动删除
func (t *TagSumming) TableOptions() map[string]any {
	return map[string]any{
		"gorm:table_options": fmt.Sprintf(`
			Engine = SummingMergeTree()
			ORDER BY (
				minute,
				source,
				type,
				item,
				tags
			)
			PARTITION BY toStartOfHour(minute)
			TTL minute + INTERVAL %d HOUR DELETE`,
			SummingTTLHours,
		),
	}
}

// TableName 通过 GORM Statement 解析获取目标表名
// 利用 GORM 的模型解析机制自动推导表名，避免硬编码
//
// 参数:
//   - db: 数据库连接实例，用于解析模型元数据
//
// 返回值:
//   - string: 解析得到的表名
//   - error: 如果模型解析失败，返回带堆栈信息的错误
func (t *TagSumming) TableName(db *database.DB) (string, error) {
	dstStmt := db.Model(t).Statement
	if err := dstStmt.Parse(t); err != nil {
		return "", errors.WithStack(err)
	}
	return dstStmt.Table, nil
}
