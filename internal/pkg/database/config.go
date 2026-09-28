package database

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
	"gorm.io/gorm"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// Prefix 是数据库配置在 Viper 中的键前缀，所有数据库配置项均位于 "database" 前缀之下。
const (
	Prefix = "database"
)

// MetaConfig 包含 GORM 数据库连接的元配置选项
// 这些配置对应 gorm.Config 结构体的字段，用于精细控制数据库行为
type MetaConfig struct {
	// SkipDefaultTransaction 跳过每个数据库操作的默认事务
	SkipDefaultTransaction bool `mapstructure:"skipDefaultTransaction" default:"false" comment:"跳过每个数据库操作的默认事务 (ref: https://pkg.go.dev/gorm.io/gorm#Config-SkipDefaultTransaction)"`
	// FullSaveAssociations 创建或更新时保存关联
	FullSaveAssociations bool `mapstructure:"fullSaveAssociations" default:"false" comment:"创建或更新时保存关联 (ref: https://pkg.go.dev/gorm.io/gorm#Config-FullSaveAssociations)"`
	// DryRun 仅生成 SQL 不执行，用于预编译语句
	DryRun bool `mapstructure:"dryRun" default:"false" comment:"仅生成 SQL 不执行，用于预编译语句 (ref: https://pkg.go.dev/gorm.io/gorm#Config-DryRun)"`
	// PrepareStmt 缓存预编译语句以供后续调用
	PrepareStmt bool `mapstructure:"prepareStmt" default:"false" comment:"缓存预编译语句以供后续调用 (ref: https://pkg.go.dev/gorm.io/gorm#Config-PrepareStmt)"`
	// DisableAutomaticPing 禁用自动 ping 检查数据库存活
	DisableAutomaticPing bool `mapstructure:"disableAutomaticPing" default:"false" comment:"禁用自动 ping 检查数据库存活 (ref: https://pkg.go.dev/gorm.io/gorm#Config-DisableAutomaticPing)"`
	// DisableForeignKeyConstraintWhenMigrating 迁移时禁用外键约束
	DisableForeignKeyConstraintWhenMigrating bool `mapstructure:"disableForeignKeyConstraintWhenMigrating" default:"false" comment:"迁移时禁用外键约束 (ref: https://pkg.go.dev/gorm.io/gorm#Config-DisableForeignKeyConstraintWhenMigrating)"`
	// IgnoreRelationshipsWhenMigrating 迁移时忽略关联关系
	IgnoreRelationshipsWhenMigrating bool `mapstructure:"ignoreRelationshipsWhenMigrating" default:"false" comment:"迁移时忽略关联关系 (ref: https://pkg.go.dev/gorm.io/gorm#Config-IgnoreRelationshipsWhenMigrating)"`
	// DisableNestedTransaction 禁用嵌套事务，已在事务中时回滚到保存点
	DisableNestedTransaction bool `mapstructure:"disableNestedTransaction" default:"false" comment:"禁用嵌套事务，已在事务中时回滚到保存点 (ref: https://pkg.go.dev/gorm.io/gorm#Config-DisableNestedTransaction)"`
	// AllowGlobalUpdate 允许不带 where 条件的全局更新
	AllowGlobalUpdate bool `mapstructure:"allowGlobalUpdate" default:"false" comment:"允许不带 where 条件的全局更新 (ref: https://pkg.go.dev/gorm.io/gorm#Config-AllowGlobalUpdate)"`
	// QueryFields 查询时选择所有字段
	QueryFields bool `mapstructure:"queryFields" default:"false" comment:"查询时选择所有字段 (ref: https://pkg.go.dev/gorm.io/gorm#Config-QueryFields)"`
	// CreateBatchSize 创建操作的批量大小，0 表示不批量
	CreateBatchSize int `mapstructure:"createBatchSize" default:"0" comment:"创建操作的批量大小，0 表示不批量 (ref: https://pkg.go.dev/gorm.io/gorm#Config-CreateBatchSize)"`
	// TranslateError 将 gorm 错误转换为可读的错误信息
	TranslateError bool `mapstructure:"translateError" default:"true" comment:"将 gorm 错误转换为可读的错误信息 (ref: https://pkg.go.dev/gorm.io/gorm#Config-TranslateError)"`
	// PropagateUnscoped 将 unscoped 传播到嵌套查询
	PropagateUnscoped bool `mapstructure:"propagateUnscoped" default:"false" comment:"将 unscoped 传播到嵌套查询 (ref: https://pkg.go.dev/gorm.io/gorm#Config-PropagateUnscoped)"`
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
	// Driver 数据库驱动名称，支持：postgres、clickhouse
	Driver string `mapstructure:"driver" default:"postgres" comment:"数据库驱动名称，支持：postgres、clickhouse (ref: https://gorm.io/docs/connect.html)"`
	// DSN 数据库数据源名称 / 连接字符串
	DSN string `mapstructure:"dsn" default:"test.db" comment:"数据库数据源名称 / 连接字符串 (ref: https://gorm.io/docs/connect.html#Connection-String)"`
	// Meta 数据库 GORM 配置选项
	Meta MetaConfig `mapstructure:"meta" comment:"数据库 GORM 配置选项 (ref: https://pkg.go.dev/gorm.io/gorm#Config)"`
}

// DescribeConfig 返回数据库配置项的元描述列表，names 为需描述的配置分组名。
func DescribeConfig(names ...string) config.ConfigDescriptions {
	return config.DescribeConfig[Config](fmt.Sprintf(`%s.{%s}`, Prefix, strings.Join(names, ",")))
}

// ExtractConfig 从 Viper 实例中提取并校验指定名称的数据库配置。
func ExtractConfig(viper *viper.Viper, validate *validator.Validate, name string) (*Config, error) {
	return config.ExtractConfig[Config](viper, validate, fmt.Sprintf("%s.%s", Prefix, name))
}
