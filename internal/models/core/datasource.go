// Package core 提供平台核心领域的持久化模型。本文件定义第三方数据源（Datasource）
// 模型：前端动态配置的 Redis / MySQL(MariaDB) 第三方数据源，每个数据源注册为一张
// SQLite vtable，供 rego 规则经 sqlite.query 查询（当前由标签事件消费，属通用能力）。
// 数据源按命名空间隔离：连接会话（连接池 Key）与 vtable 注册身份均以命名空间为维度，
// 即使不同命名空间链接同一个外部实例，也不会复用同一个连接池（隔离连接与凭据，
// 避免跨租户串扰）。
package core

import (
	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// Datasource 表示一个命名空间级的第三方数据源配置，每行对应一张 sqlite vtable。
// Name 为命名空间内唯一（与 NamespaceName 组合唯一）的本地身份；注册时作为所在
// 命名空间池（vtab.Pools）内的裸名 vtable {name}，rego 规则直接以该裸名引用。
// Type 表明数据源类型（redis | mysql）；Config 为连接配置（含密码等凭据），以加密
// 形式落库（crypto serializer，BYTEA 列），明文经 ConfigData() 获取。
type Datasource struct {
	database.BaseModel

	// Namespace 所属的命名空间实体，用于 GORM 关联（foreignKey:NamespaceName;references:Name；级联删除）
	Namespace Namespace `gorm:"foreignKey:NamespaceName;references:Name;onDelete:CASCADE"`
	// NamespaceName 命名空间名称，多租户隔离字段，同一命名空间内数据源名称唯一；
	// 同时作为连接池 Key 的命名空间维度
	NamespaceName string `gorm:"column:namespace_name;type:VARCHAR(32);uniqueIndex:uq_datasources_namespace_name;index:idx_datasources_namespace"`
	// Name 数据源名称（本地身份），与 NamespaceName 组合唯一；注册为该命名空间池内的裸名 vtable
	Name string `gorm:"column:name;type:VARCHAR(128);uniqueIndex:uq_datasources_namespace_name"`
	// Type 数据源类型键：redis | mysql，取值定义在 internal/pkg/sqlitevtab/vtab
	// （DatasourceTypeRedis / DatasourceTypeMySQL），避免与模型层造成常量重复。
	Type string `gorm:"column:type;type:VARCHAR(16)"`
	// Enabled 是否启用，禁用的数据源不注册 vtable
	Enabled bool `gorm:"column:enabled;type:BOOLEAN"`
	// Config 数据源连接配置（JSON 文本），可能含传输凭据（如密码），以加密形式持久化
	// （crypto serializer 密文入库，BYTEA 列）；明文经 ConfigData() 获取
	Config *serializer.Crypto `json:"config" gorm:"column:config;type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=21002,length=32"`
	// Description 数据源描述，说明用途
	Description string `gorm:"column:description;type:VARCHAR(256)"`
}

// ConfigData 返回数据源连接配置的明文字节，nil 安全（Config 为空或 nil 时返回 nil）。
func (d *Datasource) ConfigData() []byte {
	if d == nil || d.Config == nil {
		return nil
	}
	return d.Config.Value()
}
