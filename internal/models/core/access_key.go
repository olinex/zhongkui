package core

import (
	"context"
	"fmt"
	"time"

	"zhongkui.com/zhongkui/internal/pkg/auth"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/redis"
)

const (
	// ACCESS_KEY_PREFIX 是访问键在 Redis 缓存中的前缀
	// 用于构建唯一的缓存键，防止与其他缓存键冲突
	ACCESS_KEY_PREFIX = "akp"
)

// ErrTooManyAccessKey 表示用户创建的访问密钥数量已达上限
var ErrTooManyAccessKey = errors.New("too many access keys")

// AccessKey 表示一个访问键实体，用于 API 认证和授权
// 每个访问键属于一个用户，包含访问 ID 和密钥对
type AccessKey struct {
	database.BaseModel
	// RawUserID 是用户的数据库 ID
	RawUserID uint `gorm:"column:user_id;type:INT"`
	// User 是关联的用户实体
	User User `gorm:"foreignKey:RawUserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	// Enabled 表示访问键是否启用
	// true: 启用，可以用于认证
	// false: 禁用，不能用于认证
	Enabled bool `gorm:"type:BOOLEAN"`
	// AccessKeyID 是访问键的唯一标识符
	// 长度为 32 字符，用于 API 请求的身份识别
	AccessKeyID string `gorm:"type:VARCHAR(32)"`
	// AccessKeySecret 是访问键的密钥
	// 长度为 64 字符，用于签名验证
	AccessKeySecret string `gorm:"type:VARCHAR(64)"`
	// Description 是访问键的可选描述信息
	// 用于标识访问键的用途或所有者
	Description string `gorm:"type:VARCHAR(256)"`
	// RawExpiredAt 是访问键的过期时间
	// nil 表示永不过期
	// 非 nil 表示在该时间点后访问键将失效
	RawExpiredAt *time.Time `gorm:"column:expired_at;type:TIMESTAMPTZ"`
	// LastAccessAt 记录最后一次使用该访问键的时间
	// 用于监控和审计目的
	LastAccessAt *time.Time `gorm:"type:TIMESTAMPTZ"`
}

// UserID 返回用户的统一标识符
// 实现 auth.AccessKey 接口
func (a *AccessKey) UserID() string {
	return a.User.UnionID.String()
}

// IsEnabled 返回访问键是否启用
// 实现 auth.AccessKey 接口
func (a *AccessKey) IsEnabled() bool {
	return a.Enabled
}

// Secret 返回访问键的密钥
// 实现 auth.AccessKey 接口
func (a *AccessKey) Secret() string {
	return a.AccessKeySecret
}

// ExpiredAt 返回访问键的过期时间
// 实现 auth.AccessKey 接口
func (a *AccessKey) ExpiredAt() *time.Time {
	return a.RawExpiredAt
}

// NewAccessKey 创建一个新的访问键实例
// 参数:
//   - user: 关联的用户对象
//   - enabled: 是否启用访问键
//   - description: 访问键的描述信息
//   - expiredAt: 过期时间，nil 表示永不过期
//
// 返回:
//   - *AccessKey: 新创建的访问键实例
//   - error: 创建过程中发生的错误
func NewAccessKey(user *User, enabled bool, description string, expiredAt *time.Time) (*AccessKey, error) {
	// 生成 20 字符的随机访问键 ID
	id, err := crypto.GenerateRandomString(20)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// 生成 40 字符的随机访问键密钥
	secret, err := crypto.GenerateRandomString(40)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &AccessKey{
		RawUserID:       user.ID,
		User:            *user,
		Enabled:         enabled,
		AccessKeyID:     id,
		AccessKeySecret: secret,
		Description:     description,
		RawExpiredAt:    expiredAt,
		LastAccessAt:    nil,
	}, nil
}

// CreateWithLimit 在事务中创建访问键，并检查用户已有的访问键数量是否超限
// 参数:
//   - db: 数据库连接
//   - maxCount: 用户允许创建的最大访问键数量
//
// 返回:
//   - error: 创建过程中发生的错误，如果超出数量限制返回 ErrTooManyAccessKey
func (a *AccessKey) CreateWithLimit(db *database.DB, maxCount int) error {
	return db.Transaction(func(tx *database.DB) error {
		var count int64
		if err := tx.Model(&AccessKey{}).Where("user_id = ?", a.RawUserID).Count(&count).Error; err != nil {
			return errors.WithStack(err)
		}
		if count >= int64(maxCount) {
			return ErrTooManyAccessKey
		}
		return tx.Create(a).Error
	})
}

// AccessKeyPool 管理访问键的缓存和重复检查
// 用于防止重放攻击和提供访问键的快速查找
type AccessKeyPool struct {
	// db 是数据库连接，用于持久化存储访问键
	db *database.DB
	// cache 是 Redis 客户端，用于缓存访问键和防重放信息
	cache *redis.Client
	// ttl 是缓存条目的生存时间
	// 用于控制防重放检查的有效期
	ttl time.Duration
}

// nonceKey 生成用于防重放检查的 Redis 缓存键
// 格式: akp:{access_key_id}:{nonce}
func nonceKey(id, nonce string) string {
	return fmt.Sprintf("%s:%s:%s", ACCESS_KEY_PREFIX, id, nonce)
}

// NewAccessKeyPool 创建一个新的访问键池实例
// 参数:
//   - db: 数据库连接
//   - cache: Redis 客户端
//   - ttl: 缓存生存时间
//
// 返回:
//   - *AccessKeyPool: 新创建的访问键池实例
func NewAccessKeyPool(db *database.DB, cache *redis.Client, ttl time.Duration) *AccessKeyPool {
	return &AccessKeyPool{
		db,
		cache,
		ttl,
	}
}

// GetKey 根据访问键 ID 获取访问键信息
// 参数:
//   - ctx: 上下文，用于传递请求级数据和控制超时
//   - id: 访问键 ID
//
// 返回:
//   - auth.AccessKey: 访问键接口实现
//   - error: 查询过程中发生的错误
func (a *AccessKeyPool) GetKey(ctx context.Context, id string) (auth.AccessKey, error) {
	key := AccessKey{}
	if err := a.db.WithContext(ctx).First(&key, "access_key_id = ?", id).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return &key, nil
}

// IsDuplicated 检查请求是否重复（防重放攻击）
// 参数:
//   - ctx: 上下文
//   - id: 访问键 ID
//   - timestamp: 请求时间戳
//   - nonce: 请求随机数
//
// 返回:
//   - bool: true 表示请求重复或已过期，false 表示请求有效
//   - error: 检查过程中发生的错误
func (a *AccessKeyPool) IsDuplicated(ctx context.Context, id string, timestamp time.Time, nonce string) (bool, error) {
	// 检查时间戳是否已过期
	if time.Now().After(timestamp.Add(a.ttl)) {
		return true, nil
	}
	// 构建缓存键
	key := nonceKey(id, nonce)
	// 检查随机数是否已使用
	rsp := a.cache.Exists(ctx, key)
	if err := rsp.Err(); err != nil {
		return true, errors.WithStack(err)
	} else if exists := rsp.Val(); exists != 0 {
		return true, nil
	} else if err = a.cache.Set(ctx, key, 1, a.ttl).Err(); err != nil {
		return true, errors.WithStack(err)
	} else {
		return false, nil
	}
}

// UpdateLastAccess 更新访问键的最后访问时间
// 参数:
//   - ctx: 上下文
//   - id: 访问键 ID
//
// 返回:
//   - error: 更新过程中发生的错误
func (a *AccessKeyPool) UpdateLastAccess(ctx context.Context, id string) error {
	return a.db.
		WithContext(ctx).
		Model(&AccessKey{}).
		Where("access_key_id = ?", id).
		UpdateColumn("last_access_at", time.Now()).
		Error
}
