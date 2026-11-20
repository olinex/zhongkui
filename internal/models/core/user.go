package core

import (
	"gorm.io/gorm"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// User 表示系统用户实体
// 该结构体映射到数据库中的用户表，包含用户的基本信息和认证凭据
type User struct {
	gorm.Model // 内嵌 gorm.Model，包含 ID、CreatedAt、UpdatedAt、DeletedAt 字段

	// UnionID 是用户的全局唯一标识符
	// 使用 HexUUID 类型存储，确保在整个系统中的唯一性
	UnionID serializer.HexUUID `gorm:"type:UUID"`

	// NickleName 是用户的昵称
	// 用于显示和个性化标识，长度限制为 64 个字符
	NickleName string `gorm:"type:VARCHAR(64)"`

	// Username 是用户的登录用户名
	// 必须唯一，用于用户登录认证，长度限制为 64 个字符
	Username string `gorm:"type:VARCHAR(64);uniqueIndex"`

	// Password 是用户的加密密码
	// 使用 bcrypt 算法加密后，再通过 AES-GCM 加密存储
	Password *serializer.Crypto `gorm:"type:BYTEA;serializer:crypto" crypto:"mode=\"aes-gcm\",offset=3985,length=32"`

	// Email 是用户的电子邮件地址
	// 必须唯一，用于用户通信和密码重置，长度限制为 128 个字符
	Email string `gorm:"type:VARCHAR(128);uniqueIndex"`

	// Active 表示用户是否处于激活状态
	// 当用户被禁用时，该字段为 false，用户将无法登录系统
	Active bool `gorm:"type:BOOLEAN"`

	// FrontendConfig 是用户的前端页面配置
	// 以 JSON 文本形式存储，由浏览器端在配置变化时自动上传，
	// 便于用户跨主机登录时保持相同的前端页面配置
	FrontendConfig string `gorm:"type:TEXT"`

	// FrontendConfigHash 是前端配置内容的 SHA-256 十六进制摘要
	// 配合 FrontendConfigVersion 实现跨浏览器端更新时的乐观锁并发校验
	FrontendConfigHash string `gorm:"type:CHAR(64)"`

	// FrontendConfigVersion 是前端配置的版本号
	// 每次成功同步递增 1，用于检测多浏览器端登录时的更新竞态
	FrontendConfigVersion uint64 `gorm:"type:BIGINT"`

	// FrontendConfigSyncEnabled 表示是否开启前端配置同步
	// 由用户个人资料页的开关控制，关闭时后端拒绝接收配置上传
	FrontendConfigSyncEnabled bool `gorm:"type:BOOLEAN"`
}

// ResetPassword 重置用户密码
//
// 参数:
//   - db: 数据库连接实例，用于执行密码更新操作
//   - password: 新的明文密码，将在方法内部进行加密处理
//
// 返回值:
//   - error: 如果密码加密失败或数据库更新失败，返回错误信息
//
// 功能说明:
//  1. 使用 crypto.HashPassword 函数对新密码进行加密
//  2. 使用数据库事务更新用户的密码字段
//  3. 使用 UpdateColumn 方法直接更新密码字段，避免触发模型钩子
//  4. 错误处理使用 errors.WithStack 包装错误信息，便于调试
func (u *User) ResetPassword(db *database.DB, password string) error {
	if newHashedPassword, err := crypto.HashPassword(password); err != nil {
		return errors.WithStack(err)
	} else {
		u.Password = serializer.NewCrypto(newHashedPassword)
		return errors.WithStack(db.Select("password").Updates(u).Error)
	}
}

// Create 创建新用户并分配相应的 RBAC 策略
//
// 参数:
//   - db: 数据库连接实例，用于执行创建操作
//   - superuser: 布尔值，指示是否创建超级用户
//
// 返回值:
//   - error: 如果创建用户或分配策略失败，返回错误信息
//
// 功能说明:
//  1. 在数据库事务中执行用户创建操作，确保数据一致性
//  2. 根据 superuser 参数决定用户的权限级别:
//     - 如果 superuser 为 true: 分配超级用户权限 (SuperuserGroup 和 WideMatchLabel)
//     - 如果 superuser 为 false: 分配默认用户权限 (DefaultGroup 和 DefaultNamespace)
//  3. 使用 NewUserPolicy 函数创建相应的 RBAC 策略
//  4. 将用户和策略一起保存到数据库，确保原子性操作
//  5. 错误处理使用 errors.WithStack 包装错误信息，便于调试
func (u *User) Create(db *database.DB, superuser bool) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(u).Error; err != nil {
			return errors.WithStack(err)
		}
		var policy *RbacPolicy
		if superuser {
			policy = NewUserPolicy(
				u.UnionID,
				base.SuperuserGroup,
				base.WideMatchLabel,
			)
		} else {
			policy = NewUserPolicy(
				u.UnionID,
				base.DefaultGroup,
				base.DefaultNamespace,
			)
		}
		if err := tx.Create(policy).Error; err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
}
