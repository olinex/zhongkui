// Package database 提供数据库连接创建与管理：依据配置选择驱动（postgres / clickhouse）
// 并构建 GORM 连接，同时提供配置解析与日志记录等能力。
package database

import (
	"gorm.io/driver/clickhouse"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// DB 是 gorm.DB 的别名，用于简化类型引用
type DB = gorm.DB

// NewDatabase 根据配置和日志创建一个新的数据库连接。
// 该函数支持多种数据库驱动：postgres 和 clickhouse。
// 对于 clickhouse 驱动，会自动调整一些配置参数以适应其特性。
//
// 参数:
//   - cfg: 数据库配置信息，包含驱动类型、DSN 连接字符串和元配置
//   - logger: 日志记录器，用于记录数据库操作日志和警告信息
//
// 返回:
//   - DB: 成功时返回数据库连接实例
//   - error: 如果驱动不支持或连接失败，返回错误信息
//
// 示例:
//
//	cfg := &database.Config{
//	    Driver: "postgres",
//	    DSN:    "host=localhost user=postgres dbname=test",
//	}
//	db, err := database.NewDatabase(cfg, logger)
//	if err != nil {
//	    // 处理错误
//	}
func NewDatabase(cfg *Config, logger *logging.Logger) (*DB, error) {
	// 检查配置是否为空
	if cfg == nil {
		return nil, errors.New("database configuration is nil")
	}

	// 将配置转换为 GORM 配置
	gcfg := cfg.Meta.ToGormConfig(logger)
	var dialector gorm.Dialector

	// 根据驱动类型选择对应的数据库连接器
	switch cfg.Driver {
	case "postgres":
		// PostgreSQL 数据库连接
		dialector = postgres.Open(cfg.DSN)
	case "clickhouse":
		// ClickHouse 数据库连接
		dialector = clickhouse.Open(cfg.DSN)

		// ClickHouse 不支持外键约束，需要强制调整相关配置
		// 注意：gcfg 可能为 nil，需要检查
		if gcfg != nil {
			if !gcfg.DisableForeignKeyConstraintWhenMigrating {
				gcfg.DisableForeignKeyConstraintWhenMigrating = true
			}
			if !gcfg.IgnoreRelationshipsWhenMigrating {
				gcfg.IgnoreRelationshipsWhenMigrating = true
			}

			// ClickHouse 需要事务支持，调整相关配置
			if gcfg.SkipDefaultTransaction {
				gcfg.SkipDefaultTransaction = false
			}
			if gcfg.DisableNestedTransaction {
				gcfg.DisableNestedTransaction = false
			}
			if gcfg.DisableAutomaticPing {
				gcfg.DisableAutomaticPing = false
			}
			gcfg.TranslateError = false
		}
	default:
		// 不支持的驱动类型，返回错误
		return nil, errors.Newf("Unsupported driver %s", cfg.Driver)
	}

	// 使用配置的连接器和配置打开数据库连接
	return gorm.Open(dialector, gcfg)
}
