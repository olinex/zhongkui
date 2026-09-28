// Package auth（本文件）定义访问密钥（Access Key）及其存储池的抽象接口，
// 供上层认证逻辑依赖具体实现。
package auth

import (
	"context"
	"time"
)

// AccessKey 表示单个访问密钥的只读视图。
type AccessKey interface {
	// UserID 返回密钥所属用户标识。
	UserID() string
	// IsEnabled 返回密钥是否处于启用状态。
	IsEnabled() bool
	// Secret 返回用于签名校验的密钥明文。
	Secret() string
	// ExpiredAt 返回密钥的过期时间，未设置过期时间时返回 nil。
	ExpiredAt() *time.Time
}

// AccessKeyPool 定义访问密钥的查询与状态维护能力。
type AccessKeyPool interface {
	// GetKey 根据密钥 ID 查询对应的访问密钥，不存在时返回 nil。
	GetKey(ctx context.Context, id string) (key AccessKey, err error)
	// IsDuplicated 判断给定密钥 ID 与时间戳、随机串组合是否已使用过。
	IsDuplicated(ctx context.Context, id string, timestamp time.Time, nonce string) (bool, error)
	// UpdateLastAccess 更新密钥的最后访问时间。
	UpdateLastAccess(ctx context.Context, id string) error
}
