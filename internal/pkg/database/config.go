package database

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
	"gorm.io/gorm"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// PREFIX 是数据库配置在配置文件中的前缀
const PREFIX = "database"

// MetaConfig 包含 GORM 数据库连接的元配置选项
// 这些配置对应 gorm.Config 结构体的字段，用于精细控制数据库行为
type MetaConfig struct {
	SkipDefaultTransaction                   bool `mapstructure:"skipDefaultTransaction" default:"false" comment:"skip default transaction for every DB operation (ref: https://pkg.go.dev/gorm.io/gorm#Config-SkipDefaultTransaction)"`
	FullSaveAssociations                     bool `mapstructure:"fullSaveAssociations" default:"false" comment:"save associations when creating/updating (ref: https://pkg.go.dev/gorm.io/gorm#Config-FullSaveAssociations)"`
	DryRun                                   bool `mapstructure:"dryRun" default:"false" comment:"generate SQL without executing, used for prepare statements (ref: https://pkg.go.dev/gorm.io/gorm#Config-DryRun)"`
	PrepareStmt                              bool `mapstructure:"prepareStmt" default:"false" comment:"cache prepared statements for future calls (ref: https://pkg.go.dev/gorm.io/gorm#Config-PrepareStmt)"`
	DisableAutomaticPing                     bool `mapstructure:"disableAutomaticPing" default:"false" comment:"disable auto ping to check if DB is alive (ref: https://pkg.go.dev/gorm.io/gorm#Config-DisableAutomaticPing)"`
	DisableForeignKeyConstraintWhenMigrating bool `mapstructure:"disableForeignKeyConstraintWhenMigrating" default:"false" comment:"disable foreign key constraint when migrating (ref: https://pkg.go.dev/gorm.io/gorm#Config-DisableForeignKeyConstraintWhenMigrating)"`
	IgnoreRelationshipsWhenMigrating         bool `mapstructure:"ignoreRelationshipsWhenMigrating" default:"false" comment:"ignore relationships when migrating (ref: https://pkg.go.dev/gorm.io/gorm#Config-IgnoreRelationshipsWhenMigrating)"`
	DisableNestedTransaction                 bool `mapstructure:"disableNestedTransaction" default:"false" comment:"disable nested transaction, roll back to savepoint if already in transaction (ref: https://pkg.go.dev/gorm.io/gorm#Config-DisableNestedTransaction)"`
	AllowGlobalUpdate                        bool `mapstructure:"allowGlobalUpdate" default:"false" comment:"allow global update without where clause (ref: https://pkg.go.dev/gorm.io/gorm#Config-AllowGlobalUpdate)"`
	QueryFields                              bool `mapstructure:"queryFields" default:"false" comment:"select all fields when querying (ref: https://pkg.go.dev/gorm.io/gorm#Config-QueryFields)"`
	CreateBatchSize                          int  `mapstructure:"createBatchSize" default:"0" comment:"batch size for create operations, 0 means no batching (ref: https://pkg.go.dev/gorm.io/gorm#Config-CreateBatchSize)"`
	TranslateError                           bool `mapstructure:"translateError" default:"true" comment:"translate gorm errors to human-readable error messages (ref: https://pkg.go.dev/gorm.io/gorm#Config-TranslateError)"`
	PropagateUnscoped                        bool `mapstructure:"propagateUnscoped" default:"false" comment:"propagate unscoped to nested queries (ref: https://pkg.go.dev/gorm.io/gorm#Config-PropagateUnscoped)"`
}

// ToGormConfig 将 MetaConfig 转换为 GORM 配置
// 如果 MetaConfig 为 nil，返回 nil
//
// 参数:
//   - logger: 日志记录器，用于创建 GORM 日志适配器
//
// 返回:
//   - *gorm.Config: 转换后的 GORM 配置，包含日志适配器
func (m *MetaConfig) ToGormConfig(logger *logging.Logger) *gorm.Config {
	if m == nil {
		return nil
	}
	return &gorm.Config{
		SkipDefaultTransaction:                   m.SkipDefaultTransaction,
		FullSaveAssociations:                     m.FullSaveAssociations,
		DryRun:                                   m.DryRun,
		PrepareStmt:                              m.PrepareStmt,
		DisableAutomaticPing:                     m.DisableAutomaticPing,
		DisableForeignKeyConstraintWhenMigrating: m.DisableForeignKeyConstraintWhenMigrating,
		IgnoreRelationshipsWhenMigrating:         m.IgnoreRelationshipsWhenMigrating,
		DisableNestedTransaction:                 m.DisableNestedTransaction,
		AllowGlobalUpdate:                        m.AllowGlobalUpdate,
		QueryFields:                              m.QueryFields,
		CreateBatchSize:                          m.CreateBatchSize,
		TranslateError:                           m.TranslateError,
		PropagateUnscoped:                        m.PropagateUnscoped,
		Logger:                                   NewZapGormLogger(logger),
	}
}

// Config 是数据库连接的主配置结构
// 包含驱动类型、连接字符串和元配置
type Config struct {
	Driver string     `mapstructure:"driver" default:"sqlite" comment:"database driver name, supports: 'sqlite', 'postgres', 'clickhouse' (ref: https://gorm.io/docs/connect.html)"`
	DSN    string     `mapstructure:"dsn" default:"test.db" comment:"database source name / connection string (ref: https://gorm.io/docs/connect.html#Connection-String)"`
	Meta   MetaConfig `mapstructure:"meta" comment:"database GORM config options (ref: https://pkg.go.dev/gorm.io/gorm#Config)"`
}

// DescribeConfig 返回数据库配置的描述信息
// 用于生成配置文档或验证配置结构
//
// 参数:
//   - names: 可选的配置名称列表，用于生成特定配置的描述
//
// 返回:
//   - config.ConfigDescriptions: 配置描述信息
func DescribeConfig(names ...string) config.ConfigDescriptions {
	return config.DescribeConfig[Config](fmt.Sprintf(`%s.{%s}`, PREFIX, strings.Join(names, ",")))
}

// ExtractConfig 从 Viper 配置中提取数据库配置
// 使用指定的名称从配置文件中读取配置并解析为 Config 结构
//
// 参数:
//   - v: Viper 配置实例
//   - name: 配置名称，对应配置文件中的 database.{name} 路径
//
// 返回:
//   - *Config: 成功时返回配置实例
//   - error: 如果配置解析失败，返回错误信息
func ExtractConfig(v *viper.Viper, name string) (*Config, error) {
	return config.ExtractConfig[Config](v, fmt.Sprintf(`%s.%s`, PREFIX, name))
}
