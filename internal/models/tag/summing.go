// Package risk 提供风险分析相关的数据模型和聚合逻辑。
// 本文件定义了基于 ClickHouse SummingMergeTree 引擎的标签聚合模型，
// 通过物化视图自动从网络传输数据中聚合风险标签统计信息。
package tag

import (
	"fmt"
	"time"

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
//   - api:    API 接口标识
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
	// Minute 聚合时间窗口的起始时间（分钟对齐），使用 DateTime64(6) 精度
	// DoubleDelta 编码对时间序列数据有极高的压缩比
	Minute time.Time `gorm:"type:DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3))" json:"minute"`
	// Api API 接口标识，标识触发风险标签的 API 端点
	Api string `gorm:"type:String CODEC(ZSTD(3))" json:"api"`
	// Tags 风险标签组合，存储去重排序后的标签数组
	// ClickHouse 中使用 Array(String) 类型存储
	Tags []string `gorm:"type:Array(String)" json:"tags"`
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
//   - ORDER BY (minute, api, tags): 按 minute、api、tags 排序，作为主键和去重依据
//   - PARTITION BY toStartOfHour(minute): 按小时分区，优化查询和过期管理
//   - TTL minute + INTERVAL 72 HOUR DELETE: 数据保留 72 小时后自动删除
func (t *TagSumming) TableOptions() map[string]any {
	return map[string]any{
		"gorm:table_options": fmt.Sprintf(`
			Engine = SummingMergeTree()
			ORDER BY (
				minute,
				api,
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

// MaterializedViewName 根据源表和目标表名生成物化视图名称
// 物化视图命名规则为：{源表名}_2_{目标表名}_mv
// 其中 _mv 后缀标识该对象为物化视图（Materialized View）
//
// 参数:
//   - db:     数据库连接实例，用于解析源表和目标表的表名
//   - source: 源数据模型，物化视图的数据来源表
//
// 返回值:
//   - string: 物化视图名称，格式为 {srcTable}_2_{dstTable}_mv
//   - error:  如果表名解析失败，返回带堆栈信息的错误
func (t *TagSumming) MaterializedViewName(db *database.DB, source any) (string, error) {
	dstTable, err := t.TableName(db)
	if err != nil {
		return "", errors.WithStack(err)
	}
	srcStmt := db.Model(source).Statement
	if err := srcStmt.Parse(t); err != nil {
		return "", errors.WithStack(err)
	}
	srcTable := srcStmt.Table
	return fmt.Sprintf(`%s_2_%s_mv`, srcTable, dstTable), nil
}

// EnrollMaterializedView 创建从源表到目标表的物化视图
// 物化视图会在数据写入源表时自动触发，将聚合结果写入 TagSumming 目标表。
//
// 聚合逻辑:
//   - toStartOfMinute(reqtime): 将请求时间对齐到分钟粒度
//   - arraySort(arrayDistinct(labels)): 对标签数组去重并排序，确保标签组合的一致性
//   - count(): 统计同一分钟、同一 API、同一标签组合的命中次数
//   - GROUP BY minute, api, tags: 按这三个维度进行分组聚合
//
// 参数:
//   - db:     数据库连接实例，用于执行 DDL 语句
//   - source: 源数据模型，物化视图的数据来源表（如 HttpTransport）
//
// 返回值:
//   - error: 如果物化视图创建失败，返回错误信息
func (t *TagSumming) EnrollMaterializedView(db *database.DB, source any) error {
	mvTable, err := t.MaterializedViewName(db, source)
	if err != nil {
		return errors.WithStack(err)
	}
	dstTable, err := t.TableName(db)
	if err != nil {
		return errors.WithStack(err)
	}
	srcStmt := db.Model(source).Statement
	if err := srcStmt.Parse(t); err != nil {
		return errors.WithStack(err)
	}
	srcTable := srcStmt.Table
	if err := db.Exec(`
		CREATE MATERIALIZED VIEW %s
		TO %s
		AS
		SELECT
			toStartOfMinute(reqtime) AS minute,
			api,
			arraySort(arrayDistinct(labels)) AS tags,
			count() AS cnt
		FROM %s
		GROUP BY minute, api, tags;
	`, mvTable, dstTable, srcTable).Error; err != nil {
		return err
	}
	return nil
}

// ClearMaterialView 删除从源表到目标表的物化视图
// 在表清理流程中调用，确保物化视图在目标表删除前被正确移除
//
// 参数:
//   - db:     数据库连接实例，用于执行 DDL 语句
//   - source: 源数据模型，用于确定物化视图名称
//
// 返回值:
//   - error: 如果物化视图删除失败，返回错误信息
func (t *TagSumming) ClearMaterialView(db *database.DB, source any) error {
	mvTable, err := t.MaterializedViewName(db, source)
	if err != nil {
		return errors.WithStack(err)
	}
	if err := db.Exec(`
		DROP MATERIALIZED VIEW IF EXISTS %s
	`, mvTable).Error; err != nil {
		return err
	}
	return nil
}
