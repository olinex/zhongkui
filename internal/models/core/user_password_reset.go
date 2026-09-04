package core

import (
	"time"

	"gorm.io/gorm"
)

// UserPasswordReset 表示用户强制重置密码的记录实体
// 该结构体映射到数据库中的 user_password_resets 表，
// 存储用户"忘记密码"申请的令牌摘要、过期时间与使用状态。
type UserPasswordReset struct {
	gorm.Model // 内嵌 gorm.Model，包含 ID、CreatedAt、UpdatedAt、DeletedAt 字段

	// UserID 是发起密码重置的用户 ID，外键级联删除
	UserID uint `gorm:"type:BIGINT;not null;index"`
	// Email 是申请密码重置时使用的邮箱
	Email string `gorm:"type:VARCHAR(128);not null"`
	// TokenHash 是重置令牌的 SHA-256 十六进制摘要，唯一，防止数据库泄露后令牌被直接使用
	TokenHash string `gorm:"type:CHAR(64);uniqueIndex;not null"`
	// ExpiredAt 是令牌的过期时间，过期后需要重新发起申请
	ExpiredAt time.Time `gorm:"type:TIMESTAMPTZ;not null"`
	// Used 表示该令牌是否已被使用（单次有效）
	Used bool `gorm:"type:BOOLEAN;default:false"`
}