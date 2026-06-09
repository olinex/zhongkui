package database

import (
	"time"
)

// BaseModel 是数据库模型的基础结构
// 所有数据库模型都应该嵌入此结构以获得标准字段
// GORM 会自动管理 CreatedAt 和 UpdatedAt 字段的时间戳
type BaseModel struct {
	// ID 是主键字段，使用 uint 类型
	// gorm:"primarykey" 标签标识此字段为主键
	ID        uint `gorm:"primarykey"`
	
	// CreatedAt 记录创建时间戳
	// GORM 在创建记录时自动设置此字段为当前时间
	CreatedAt time.Time
	
	// UpdatedAt 记录更新时间戳
	// GORM 在更新记录时自动设置此字段为当前时间
	UpdatedAt time.Time
}
