package routes

import (
	"net/http"
	"time"

	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

var (
	// ErrUsernamePasswordCheckFailed 表示用户名或密码校验失败
	ErrUsernamePasswordCheckFailed = manager.CreateCodeError(http.StatusBadRequest, manager.CodeUsernamePasswordCheckFailed, nil)
	// ErrOldPasswordCheckFailed 表示旧密码校验失败
	ErrOldPasswordCheckFailed = manager.CreateCodeError(http.StatusBadRequest, manager.CodeOldPasswordCheckFailed, nil)
	// ErrUsernameOrEmailAlreadyExist 表示用户名或邮箱已存在
	ErrUsernameOrEmailAlreadyExist = manager.CreateCodeError(http.StatusBadRequest, manager.CodeUsernameOrEmailAlreadyExist, nil)
	// ErrTooManyAccessKey 表示用户创建的访问密钥数量已达上限
	ErrTooManyAccessKey = manager.CreateCodeError(http.StatusBadRequest, manager.CodeTooManyAccessKey, nil)
)

// UserProfileChangeIn 定义用户个人资料修改的输入结构体
// 包含用户昵称和邮箱信息
type UserProfileChangeIn struct {
	NickleName string `json:"nickleName" viper:"nicklename" prompt:"please enter nicklename" binding:"required,min=6,max=32,alphaunicode"` // 用户昵称，必填，6-32个字符，支持Unicode字母
	Email      string `json:"email" viper:"email" prompt:"please enter user's email" binding:"required,min=10,max=128,email"`              // 用户邮箱，必填，10-128个字符，需符合邮箱格式
}

// UserPasswordLoginIn 定义密码登录的输入结构体
// 包含用户名和密码信息
type UserPasswordLoginIn struct {
	Username string `json:"username" viper:"username" prompt:"please enter username" binding:"required,min=6,max=32,alphaunicode"`                                    // 用户名，必填，6-32个字符，支持Unicode字母
	Password string `json:"password" viper:"password" prompt:"please enter password" mask:"true" confirm:"true" binding:"required,min=8,max=64,ascii,strongPassword"` // 密码，必填，8-64个ASCII字符，需满足强密码要求
}

// UserRegisterIn 定义用户注册的输入结构体
// 继承UserProfileChangeIn和UserPasswordLoginIn，包含注册所需的所有信息
type UserRegisterIn struct {
	UserProfileChangeIn
	UserPasswordLoginIn
}

// UserPasswordResetIn 定义用户密码重置的输入结构体
// 包含旧密码和新密码信息
type UserPasswordResetIn struct {
	OldPassword string `json:"oldPassword" binding:"required,min=8,max=64,ascii"`          // 旧密码，必填，8-64个ASCII字符
	NewPassword string `json:"newPassword" binding:"required,min=8,max=64,strongPassword"` // 新密码，必填，8-64个ASCII字符，需满足强密码要求
}

// UserAccessKeyCreateIn 定义创建访问密钥的输入结构体
// 包含访问密钥的启用状态、描述和过期时间
type UserAccessKeyCreateIn struct {
	Enabled     bool       `json:"enabled" default:"true"`                                  // 是否启用，默认为true
	Description string     `json:"description" binding:"required,min=6,max=256"`            // 描述，必填，6-256个字符
	ExpiredAt   *time.Time `json:"expiredAt" default:"-" binding:"omitempty,gtCurrentTime"` // 过期时间，可选，需晚于当前时间
}

// AuthorizationInfoOut 定义用户授权信息的输出结构体
// 包含RBAC模型、策略列表和命名空间列表
type AuthorizationInfoOut struct {
	Model      string     `json:"model"`      // RBAC模型名称
	Policies   [][]string `json:"policies"`   // 策略列表，每个策略包含ptype, v0, v1, v2, v3
	Namespaces []string   `json:"namespaces"` // 用户可访问的命名空间列表
}

// UserAccessKeyOut 定义访问密钥的输出结构体
// 继承BaseItemOut，包含访问密钥的详细信息
type UserAccessKeyOut struct {
	utils.BaseItemOut
	Enabled         bool       `json:"enabled"`         // 是否启用
	AccessKeyID     string     `json:"accessKeyID"`     // 访问密钥ID
	AccessKeySecret string     `json:"accessKeySecret"` // 访问密钥Secret
	Description     string     `json:"description"`     // 描述信息
	ExpiredAt       *time.Time `json:"expiredAt"`       // 过期时间
	LastAccessAt    *time.Time `json:"lastAccessAt"`    // 最后访问时间
}

// userRegister 处理用户注册请求
// 路由：POST /api/v1/user/register
// 功能：创建新用户账户
func userRegister(c *gin.Context) {
	var in UserRegisterIn
	// 设置默认值
	if err := defaults.Set(&in); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 绑定并验证请求体
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	// 对密码进行哈希处理
	hashPassword, err := crypto.HashPassword(in.Password)
	if err != nil {
		logger := manager.GetLogger(c)
		logger.Errorf(c, nil, "cannot hash password, cause: %v", err)
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 创建用户对象
	user := core.User{
		UnionID:    serializer.NewHexUUIDv4(), // 生成唯一标识符
		Username:   in.Username,
		NickleName: in.NickleName,
		Password:   serializer.NewCrypto(hashPassword),
		Email:      in.Email,
		Active:     true, // 新用户默认激活
	}
	// 保存用户到数据库
	db := manager.GetDatabase(c)
	if err = user.Create(db, false); err != nil {
		ErrUsernameOrEmailAlreadyExist.AbortWithGinH(c)
		return
	}
	// 返回成功响应
	manager.SimpleOK.AbortWithGinH(c)
}

// userPasswordLogin 处理用户密码登录请求
// 路由：POST /api/v1/user/password/login
// 功能：验证用户名密码并颁发JWT令牌
func userPasswordLogin(c *gin.Context) {
	var in UserPasswordLoginIn
	// 绑定并验证请求体
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	var user core.User
	// 从数据库查询用户（必须执行SQL查询以确保安全）
	err := manager.GetDatabase(c).Take(&user, &core.User{Username: in.Username, Active: true}).Error
	// 验证密码
	passwordValid := user.Password != nil && crypto.CheckPassword(user.Password.Value(), in.Password)
	auth := manager.GetAuth(c)
	// 检查登录是否成功
	if err != nil || user.ID == 0 || !passwordValid {
		ErrUsernamePasswordCheckFailed.AbortWithGinH(c)
		return
	} else if token, err := auth.Sign(&user); err != nil {
		err.AbortWithGinH(c)
		return
	} else {
		// 将JWT令牌注入Cookie
		auth.TokenCookieInject(c, token)
		manager.SimpleOK.AbortWithGinH(c)
		return
	}
}

// userSelfPasswordReset 处理用户密码重置请求
// 路由：POST /api/v1/user/self/password/reset
// 功能：验证旧密码后更新为新密码
func userSelfPasswordReset(c *gin.Context) {
	var in UserPasswordResetIn
	// 绑定并验证请求体
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	// 从上下文中获取当前用户
	user, codeErr := manager.GetUser(c)
	if codeErr != nil {
		codeErr.AbortWithGinH(c)
		return
	}
	db := manager.GetDatabase(c)
	// 从数据库重新加载用户信息（必须执行SQL查询以确保安全）
	err := db.Take(&user, &core.User{UnionID: user.UnionID, Active: true}).Error
	// 验证旧密码
	passwordValid := user.Password != nil && crypto.CheckPassword(user.Password.Value(), in.OldPassword)
	if err != nil || user.ID == 0 || !passwordValid {
		ErrOldPasswordCheckFailed.AbortWithGinH(c)
		return
	} else if err := user.ResetPassword(db, in.NewPassword); err != nil {
		// 密码重置失败
		logger := manager.GetLogger(c)
		logger.Errorf(c, nil, "cannot reset password, cause: %v", err)
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else {
		// 密码重置成功
		manager.SimpleOK.AbortWithGinH(c)
		return
	}
}

// userSelfProfileChange 处理用户个人资料修改请求
// 路由：POST /api/v1/user/self/profile/change
// 功能：更新用户昵称和邮箱，并刷新JWT令牌
func userSelfProfileChange(c *gin.Context) {
	var in UserProfileChangeIn
	// 绑定并验证请求体
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 从上下文中获取当前用户
	user, codeErr := manager.GetUser(c)
	if codeErr != nil {
		codeErr.AbortWithGinH(c)
		return
	}
	// 更新用户个人资料
	db := manager.GetDatabase(c)
	if err := db.Model(&user).Updates(core.User{NickleName: in.NickleName, Email: in.Email}).Error; err != nil {
		logger := manager.GetLogger(c)
		logger.Errorf(c, nil, "cannot change user %s's profile, cause: %v", user.UnionID.String(), err)
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 更新用户信息并刷新JWT令牌
	auth := manager.GetAuth(c)
	user.NickleName = in.NickleName
	user.Email = in.Email
	if token, err := auth.Sign(user); err != nil {
		err.AbortWithGinH(c)
		return
	} else {
		// 将新令牌注入Cookie
		auth.TokenCookieInject(c, token)
		manager.SimpleOK.AbortWithGinH(c)
		return
	}
}

// userSelfJwtRefresh 处理JWT令牌刷新请求
// 路由：POST /api/v1/user/self/jwt/refresh
// 功能：刷新当前用户的JWT令牌
func userSelfJwtRefresh(c *gin.Context) {
	auth := manager.GetAuth(c)
	// 刷新令牌
	newToken, codeErr := auth.Refresh(c)
	if codeErr != nil {
		codeErr.AbortWithGinH(c)
		return
	}
	// 将新令牌注入Cookie
	auth.TokenCookieInject(c, newToken)
	manager.SimpleOK.AbortWithGinH(c)
}

// userSelfAuthorizationInfoGet 获取用户授权信息
// 路由：POST /api/v1/user/self/authorization-info/get
// 功能：获取当前用户的RBAC策略和可访问的命名空间
func userSelfAuthorizationInfoGet(c *gin.Context) {
	// 从上下文中获取当前用户
	user, codeErr := manager.GetUser(c)
	if codeErr != nil {
		codeErr.AbortWithGinH(c)
		return
	}
	logger := manager.GetLogger(c)
	db := manager.GetDatabase(c)

	// 查询用户所属的用户组
	var groups []*core.RbacPolicy
	if err := db.Find(&groups, "ptype = ? AND v0 = ?", base.GROUP_LABEL, user.UnionID.String()).Error; err != nil {
		logger.Errorf(c, nil, "bad groups, cause: %v", err)
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 提取用户组名称
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		names = append(names, group.V1)
	}

	// 查询用户组对应的策略
	var policies []*core.RbacPolicy
	if err := db.Find(&policies, "ptype = ? AND v0 IN ?", base.POLICY_LABEL, names).Error; err != nil {
		logger.Errorf(c, nil, "bad policies, cause: %v", err)
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 构建返回结果
	out := AuthorizationInfoOut{
		Model:      base.RBAC_MODEL,
		Policies:   make([][]string, 0, len(groups)+len(policies)),
		Namespaces: core.Cache.All(c),
	}

	// 将策略和用户组信息合并
	for _, p := range append(policies, groups...) {
		out.Policies = append(out.Policies, []string{
			p.Ptype,
			p.V0,
			p.V1,
			p.V2,
			p.V3,
		})
	}

	// 返回授权信息
	manager.SimpleOK.AbortWithGinW(c, out)
}

// userSelfAccessKeyListGet 获取用户访问密钥列表
// 路由：POST /api/v1/user/self/access-key/list
// 功能：获取当前用户的所有访问密钥
func userSelfAccessKeyListGet(c *gin.Context) {
	// 从上下文中获取当前用户
	user, codeErr := manager.GetUser(c)
	if codeErr != nil {
		codeErr.AbortWithGinH(c)
		return
	}
	logger := manager.GetLogger(c)
	db := manager.GetDatabase(c)

	// 查询用户的所有访问密钥
	var keys []*core.AccessKey
	if err := db.Find(&keys, "user_id = ?", user.ID).Error; err != nil {
		logger.Errorf(c, nil, "bad access key, cause: %v", err)
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 构建返回结果
	out := make([]*UserAccessKeyOut, 0, len(keys))
	for _, key := range keys {
		out = append(out, &UserAccessKeyOut{
			BaseItemOut: utils.BaseItemOut{
				ID:        key.ID,
				CreatedAt: key.CreatedAt,
				UpdatedAt: key.UpdatedAt,
			},
			Enabled:         key.Enabled,
			AccessKeyID:     key.AccessKeyID,
			AccessKeySecret: key.Secret(),
			Description:     key.Description,
			ExpiredAt:       key.RawExpiredAt,
			LastAccessAt:    key.LastAccessAt,
		})
	}

	// 返回访问密钥列表
	manager.SimpleOK.AbortWithGinW(c, out)
}

// userSelfAccessKeyCreate 创建访问密钥
// 路由：POST /api/v1/user/self/access-key/create
// 功能：为用户创建新的访问密钥
func userSelfAccessKeyCreate(c *gin.Context) {
	// 从上下文中获取当前用户
	user, codeErr := manager.GetUser(c)
	if codeErr != nil {
		codeErr.AbortWithGinH(c)
		return
	}

	var in UserAccessKeyCreateIn
	// 设置默认值
	if err := defaults.Set(&in); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 绑定并验证请求体
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}

	db := manager.GetDatabase(c)
	// 创建新的访问密钥对象
	key, err := core.NewAccessKey(user, in.Enabled, in.Description, in.ExpiredAt)
	if err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 获取配置信息
	cfg := manager.GetConfig(c)

	// 使用复用方法在事务中创建访问密钥
	if err := key.CreateWithLimit(db, int(cfg.Auth.AccessKeyCount)); err != nil {
		if err == core.ErrTooManyAccessKey {
			ErrTooManyAccessKey.AbortWithGinH(c)
		} else {
			manager.CreateAccidentError(err).AbortWithGinH(c)
		}
		return
	}

	// 创建成功，返回创建的访问密钥信息
	manager.SimpleOK.AbortWithGinW(c, UserAccessKeyOut{
		BaseItemOut: utils.BaseItemOut{
			ID:        key.ID,
			CreatedAt: key.CreatedAt,
			UpdatedAt: key.UpdatedAt,
		},
		Enabled:         key.Enabled,
		AccessKeyID:     key.AccessKeyID,
		AccessKeySecret: key.Secret(),
		Description:     key.Description,
		ExpiredAt:       key.RawExpiredAt,
		LastAccessAt:    key.LastAccessAt,
	})
}

// userSelfAccessKeyRemove 删除访问密钥
// 路由：POST /api/v1/user/self/access-key/:id/remove
// 功能：删除指定ID的访问密钥
func userSelfAccessKeyRemove(c *gin.Context) {
	// 从上下文中获取当前用户
	user, codeErr := manager.GetUser(c)
	if codeErr != nil {
		codeErr.AbortWithGinH(c)
		return
	}

	// 获取URL路径参数
	id, ok := getUint64Param(c, "id")
	if !ok {
		return
	}

	// 删除访问密钥
	db := manager.GetDatabase(c)
	if err := db.Delete(&core.AccessKey{}, "id = ? AND user_id = ?", id, user.ID).Error; err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 返回成功响应
	manager.SimpleOK.AbortWithGinH(c)
}

// userSelfAccessKeyEnabled 启用或禁用访问密钥
// 路由：
//
//	POST /api/v1/user/self/access-key/:id/activate   - 启用
//	POST /api/v1/user/self/access-key/:id/deactivate - 禁用
//
// 功能：更新访问密钥的启用状态
func userSelfAccessKeyEnabled(enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从上下文中获取当前用户
		user, codeErr := manager.GetUser(c)
		if codeErr != nil {
			codeErr.AbortWithGinH(c)
			return
		}

		// 获取URL路径参数
		id, ok := getUint64Param(c, "id")
		if !ok {
			return
		}

		// 更新访问密钥的启用状态
		db := manager.GetDatabase(c)
		var key core.AccessKey
		if err := db.Model(&key).Where("id = ? AND user_id = ?", id, user.ID).Update("enabled", enabled).Scan(&key).Error; err != nil {
			manager.CreateAccidentError(err).AbortWithGinH(c)
			return
		}

		// 返回更新后的访问密钥信息
		manager.SimpleOK.AbortWithGinW(c, UserAccessKeyOut{
			BaseItemOut: utils.BaseItemOut{
				ID:        key.ID,
				CreatedAt: key.CreatedAt,
				UpdatedAt: key.UpdatedAt,
			},
			Enabled:         key.Enabled,
			AccessKeyID:     key.AccessKeyID,
			AccessKeySecret: key.Secret(),
			Description:     key.Description,
			ExpiredAt:       key.RawExpiredAt,
			LastAccessAt:    key.LastAccessAt,
		})
	}
}
