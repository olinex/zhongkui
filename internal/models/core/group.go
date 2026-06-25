package core

import (
	"zhongkui.com/zhongkui/internal/pkg/database"
)

// Group 表示系统中的用户组实体。
// 组用于对用户进行逻辑分组，便于权限管理和资源分配。
// 每个组具有唯一的名称和可选的描述信息。
type Group struct {
	database.BaseModel
	// Name 是组的唯一名称，最大长度为32个字符，不能为空。
	Name string `gorm:"type:VARCHAR(32);uniqueIndex"`
	// Description 是组的可选描述信息，最大长度为256个字符。
	Description string `gorm:"type:VARCHAR(256)"`
}
