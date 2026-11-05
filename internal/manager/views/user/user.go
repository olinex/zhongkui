package user

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
	"zhongkuisec.com/zhongkui/internal/manager/config"
	"zhongkuisec.com/zhongkui/internal/manager/features"
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/manager/views"
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/message"
	"zhongkuisec.com/zhongkui/internal/pkg/utils"
	mailhandler "zhongkuisec.com/zhongkui/internal/worker/handlers/mail"
)

var (
	// ErrUsernamePasswordCheckFailed 表示用户名或密码校验失败
	ErrUsernamePasswordCheckFailed = response.CreateCodeError(http.StatusBadRequest, response.CodeUsernamePasswordCheckFailed, nil)
	// ErrOldPasswordCheckFailed 表示旧密码校验失败
	ErrOldPasswordCheckFailed = response.CreateCodeError(http.StatusBadRequest, response.CodeOldPasswordCheckFailed, nil)
	// ErrUsernameOrEmailAlreadyExist 表示用户名或邮箱已存在
	ErrUsernameOrEmailAlreadyExist = response.CreateCodeError(http.StatusBadRequest, response.CodeUsernameOrEmailAlreadyExist, nil)
	// ErrTooManyAccessKey 表示用户创建的访问密钥数量已达上限
	ErrTooManyAccessKey = response.CreateCodeError(http.StatusBadRequest, response.CodeTooManyAccessKey, nil)
	// ErrFrontendConfigConflict 表示前端配置更新冲突（上传的哈希或版本号与服务端不一致）
	// 多浏览器端登录时，其他浏览器已抢先更新则触发该错误
	ErrFrontendConfigConflict = response.CreateCodeError(http.StatusConflict, response.CodeFrontendConfigConflict, nil)
	// ErrFrontendConfigSyncDisabled 表示前端配置同步未开启
	// 用户关闭同步开关后，后端拒绝接收配置上传
	ErrFrontendConfigSyncDisabled = response.CreateCodeError(http.StatusBadRequest, response.CodeFrontendConfigSyncDisabled, nil)
	// ErrPasswordResetUnavailable 表示密码重置功能不可用
	// 系统未启用或未配置 SMTP 时自动禁用，避免开放无意义的密码重置功能
	ErrPasswordResetUnavailable = response.CreateCodeError(http.StatusBadRequest, response.CodePasswordResetUnavailable, nil)
	// ErrPasswordResetTokenInvalid 表示密码重置令牌无效或已过期
	// 令牌不存在、已使用或超过有效期时触发
	ErrPasswordResetTokenInvalid = response.CreateCodeError(http.StatusBadRequest, response.CodePasswordResetTokenInvalid, nil)
	// ErrTooManyRequests 表示申请过于频繁（命中忘记密码申请的来源地址维度限流）
	// 同一来源地址在限流窗口内提交次数超出上限时触发
	ErrTooManyRequests = response.CreateCodeError(http.StatusTooManyRequests, response.CodeTooManyRequests, nil)
)

// UserProfileChangeIn 定义用户个人资料修改的输入结构体
// 包含用户昵称和邮箱信息
type UserProfileChangeIn struct {
	NickleName string `json:"nickleName" viper:"nicklename" prompt:"请输入昵称" binding:"required,min=6,max=32,alphaunicode"` // 用户昵称，必填，6-32个字符，支持Unicode字母
	Email      string `json:"email" viper:"email" prompt:"请输入用户邮箱" binding:"required,min=10,max=128,email"`              // 用户邮箱，必填，10-128个字符，需符合邮箱格式
}

// UserTokenOut 定义 JWT 令牌的输出结构体
// 用于登录、令牌刷新等接口在响应体中返回 JWT 令牌，
// 供前端在 Cookie 不可跨主机读取的场景（如 localhost 与 127.0.0.1）下持久化使用
type UserTokenOut struct {
	Token string `json:"token"` // JWT 令牌（含 Bearer 前缀）
}

// UserPasswordLoginIn 定义密码登录的输入结构体
// 包含用户名和密码信息
type UserPasswordLoginIn struct {
	Username string `json:"username" viper:"username" prompt:"请输入用户名" binding:"required,min=6,max=32,alphaunicode"`                                   // 用户名，必填，6-32个字符，支持Unicode字母
	Password string `json:"password" viper:"password" prompt:"请输入密码" mask:"true" confirm:"true" binding:"required,min=8,max=64,ascii,strongPassword"` // 密码，必填，8-64个ASCII字符，需满足强密码要求
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

// UserPasswordForgotIn 定义忘记密码申请的输入结构体
// 用户在未登录态提交账号名与邮箱地址，后端校验两者与库内一致后才发送重置邮件
type UserPasswordForgotIn struct {
	Username string `json:"username" viper:"username" prompt:"请输入账号名" binding:"required,min=6,max=32,alphaunicode"` // 账号名，必填，6-32个字符，支持Unicode字母
	Email    string `json:"email" viper:"email" prompt:"请输入邮箱地址" binding:"required,min=10,max=128,email"`           // 邮箱地址，必填，10-128个字符，需符合邮箱格式
}

// UserPasswordResetByTokenIn 定义通过邮件链接令牌重置密码的输入结构体
// 令牌由 forgot 接口生成并随邮件链接发送，过期或使用后失效
type UserPasswordResetByTokenIn struct {
	Token       string `json:"token" binding:"required,min=16" prompt:"请输入重置令牌"`                           // 重置令牌，必填
	NewPassword string `json:"newPassword" binding:"required,min=8,max=64,strongPassword" prompt:"请输入新密码"` // 新密码，必填，8-64个ASCII字符，需满足强密码要求
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

// UserFrontendConfigOut 定义用户前端页面配置的输出结构体
// 用于查询/同步接口返回用户当前的配置内容与并发校验元数据
type UserFrontendConfigOut struct {
	SyncEnabled bool   `json:"syncEnabled"` // 是否开启前端配置同步
	Hash        string `json:"hash"`        // 配置内容的 SHA-256 十六进制摘要
	Version     uint64 `json:"version"`     // 配置版本号，每次成功同步递增
	Config      string `json:"config"`      // 配置内容（JSON 文本）
}

// UserFrontendConfigSyncIn 定义前端配置同步上传的输入结构体
// 携带上一次同步的哈希与版本号，用于乐观锁并发校验：
// 任一不一致说明其他浏览器已抢先更新，后端拒绝本次写入
type UserFrontendConfigSyncIn struct {
	PrevHash    string `json:"prevHash"`                            // 上一次同步的配置哈希，首次同步为空字符串
	PrevVersion uint64 `json:"prevVersion"`                         // 上一次同步的配置版本号，首次同步为 0
	Config      string `json:"config" binding:"required,max=65536"` // 新的配置内容（JSON 文本），必填且不超过 64KB
}

// UserFrontendConfigSyncEnabledIn 定义前端配置同步开关的输入结构体
type UserFrontendConfigSyncEnabledIn struct {
	Enabled bool `json:"enabled"` // 是否开启前端配置同步
}

// frontendConfigHash 计算前端配置内容的 SHA-256 十六进制摘要
// 配置内容相同时摘要相同，用于乐观锁并发校验
func frontendConfigHash(config string) string {
	sum := sha256.Sum256([]byte(config))
	return hex.EncodeToString(sum[:])
}

// UserPasswordLogin 处理用户密码登录请求
// 路由：POST /api/v1/user/password/login
// 功能：验证用户名密码并颁发JWT令牌
func UserPasswordLogin(c *gin.Context) {
	var in UserPasswordLoginIn
	// 绑定并验证请求体
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	var user core.User
	// 从数据库查询用户（必须执行SQL查询以确保安全）
	err := middlewares.GetCoreDatabase(c).Take(&user, &core.User{Username: in.Username, Active: true}).Error
	// 验证密码
	passwordValid := user.Password != nil && crypto.CheckPassword(user.Password.Value(), in.Password)
	auth := middlewares.GetJWT(c)
	// 检查登录是否成功
	if err != nil || user.ID == 0 || !passwordValid {
		ErrUsernamePasswordCheckFailed.AbortWithGinH(c)
		return
	} else if token, err := auth.Sign(&user); err != nil {
		err.AbortWithGinH(c)
		return
	} else {
		// 将JWT令牌注入Cookie，并在响应体中返回令牌，供跨源前端持久化使用
		auth.TokenCookieInject(c, token)
		response.SimpleOK.AbortWithGinW(c, UserTokenOut{Token: token})
		return
	}
}

// passwordResetLimitKey 计算忘记密码申请的来源维度限流键。
// 维度取请求来源地址：字典攻击会不断更换「账号名 + 邮箱」组合（每猜一个组合都是新键），
// 按组合计数永不触发，只有来源地址才是攻击者难以逐个更换的请求侧常量。
// 以 SHA-256 十六进制摘要收敛键长，并避免来源地址明文落入 Redis 键空间。
func passwordResetLimitKey(ip string) string {
	sum := sha256.Sum256([]byte(ip))
	return "password-reset:" + hex.EncodeToString(sum[:])
}

// UserPasswordForgot 处理忘记密码申请请求
// 路由：POST /api/v1/user/password/forgot
// 功能：系统未启用或未配置 SMTP 时直接禁止（避免开放无意义功能）；校验账号名与
// 邮箱与库内一致后，生成带有效期的重置令牌并异步发送一封含重置链接的邮件至用户邮箱。
//
// 防枚举与限流：为避免攻击者据响应差异批量验证「账号名↔邮箱」配对（撞库前的标准情报收集），
// 无论账号是否存在、是否与邮箱匹配，一律返回同一成功响应，差异（未命中 / 落库失败 / 入队失败）
// 只写入服务端日志；同时对请求来源地址按配置限额做窗口限流（ErrTooManyRequests）。
func UserPasswordForgot(c *gin.Context) {
	var in UserPasswordForgotIn
	// 绑定并验证请求体
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	logger := middlewares.GetLogger(c)
	// 通过全局功能开关注册表判断密码重置功能是否可用：系统邮件未启用或配置不完整时自动禁用
	if !middlewares.GetFeatureRegistry(c).IsEnabled(features.PasswordResetKey) {
		ErrPasswordResetUnavailable.AbortWithGinH(c)
		return
	}
	// 按请求来源地址维度限流：账号枚举会不断更换「账号名 + 邮箱」组合（每次都是新键），
	// 按组合计数永不触发；来源地址才是能对枚举总量形成约束的请求侧常量。
	// Redis 故障时限流不可用，此处选择放行而非拒绝——限流属纵深防御，不应因缓存故障阻断正常的
	// 密码找回；即使放行，防枚举的统一响应与日志留痕仍然生效。
	if allowed, err := middlewares.GetPasswordResetLimiter(c).Allow(c, passwordResetLimitKey(c.ClientIP())); err != nil {
		logger.Warnf(c, nil, "password reset rate limiter unavailable, allow the request, cause: %v", err)
	} else if !allowed {
		ErrTooManyRequests.AbortWithGinH(c)
		return
	}
	db := middlewares.GetCoreDatabase(c)
	// 校验账号名与邮箱与库内一致；不一致或账号不存在时不发信，但响应与命中情形保持一致，
	// 差异只写日志，避免攻击者据响应差异批量验证「账号名↔邮箱」配对关系。
	var user core.User
	if err := db.Take(&user, &core.User{Username: in.Username, Email: in.Email, Active: true}).Error; err != nil || user.ID == 0 {
		logger.Warnf(c, nil, "password reset request was not matched by an active account, username=%s email=%s", in.Username, in.Email)
		response.SimpleOK.AbortWithGinH(c)
		return
	}
	// 生成高熵随机令牌，仅将 SHA-256 摘要入库，防止数据库泄露后令牌被直接使用
	token, err := crypto.GenerateRandomBase64String(48)
	if err != nil {
		logger.Errorf(c, nil, "cannot generate password reset token for user %s, cause: %v", user.UnionID.String(), err)
		response.SimpleOK.AbortWithGinH(c)
		return
	}
	sum := sha256.Sum256([]byte(token))
	cfg := config.GetConfig(c)
	reset := core.UserPasswordReset{
		UserID:    user.ID,
		Email:     user.Email,
		TokenHash: hex.EncodeToString(sum[:]),
		ExpiredAt: time.Now().Add(cfg.PasswordReset.TokenTTL),
	}
	if err := db.Create(&reset).Error; err != nil {
		logger.Errorf(c, nil, "cannot create password reset record for user %s, cause: %v", user.UnionID.String(), err)
		response.SimpleOK.AbortWithGinH(c)
		return
	}
	// 组装前端重置链接（含语言段与令牌参数），供用户点击或在浏览器中复制访问
	lang := base.DefaultLanguage.String()
	resetURL := fmt.Sprintf(
		"%s/%s/auth/reset-password?token=%s",
		strings.TrimRight(cfg.FrontendBaseURL, "/"),
		lang,
		token,
	)
	// 入队 mail:send 异步任务，经 locale 模板渲染标题/正文并投递 HTML 邮件
	task, err := mailhandler.NewMailSendTask(mailhandler.MailSendInput{
		To: user.Email,
		Message: message.Message{
			Template:    "password.reset",
			ContentType: "text/html", // 重置密码邮件在消息定义层声明为 HTML 正文
			Params: map[string]any{
				"Username": user.Username,
				"ResetURL": resetURL,
				"TokenTTL": cfg.PasswordReset.TokenTTL.String(),
			},
		},
		Language: lang,
	})
	if err != nil {
		logger.Errorf(c, nil, "cannot build password reset mail task for user %s, cause: %v", user.UnionID.String(), err)
		response.SimpleOK.AbortWithGinH(c)
		return
	}
	if _, err = middlewares.GetPipelineClient(c).EnqueueContext(c, task, asynq.MaxRetry(3)); err != nil {
		logger.Errorf(c, nil, "cannot enqueue password reset mail for user %s, cause: %v", user.UnionID.String(), err)
		response.SimpleOK.AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// UserPasswordResetByToken 处理通过邮件链接令牌重置密码请求
// 路由：POST /api/v1/user/password/reset
// 功能：校验令牌存在、未使用且未过期，在同一事务中标记令牌已使用并更新用户密码。
func UserPasswordResetByToken(c *gin.Context) {
	var in UserPasswordResetByTokenIn
	// 绑定并验证请求体
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	logger := middlewares.GetLogger(c)
	db := middlewares.GetCoreDatabase(c)
	// 计算令牌摘要并查询有效的重置记录（未使用且未过期）
	sum := sha256.Sum256([]byte(in.Token))
	var reset core.UserPasswordReset
	err := db.Where("token_hash = ? AND used = ? AND expired_at > ?", hex.EncodeToString(sum[:]), false, time.Now()).First(&reset).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || reset.ID == 0 {
		ErrPasswordResetTokenInvalid.AbortWithGinH(c)
		return
	} else if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 加载目标用户（用户已在上次申请后删除时视为令牌无效）
	var user core.User
	if err := db.Take(&user, "id = ?", reset.UserID).Error; err != nil {
		ErrPasswordResetTokenInvalid.AbortWithGinH(c)
		return
	}
	// 同一事务内作废该用户全部可用的重置令牌并更新密码，保证单次有效与数据一致。
	// 一个用户可能已发送多封有效的重置邮件，任一个链接使用后其余链接应自动失效：
	// 因此将同一用户所有尚未使用的令牌统一标记为已使用，而非仅当前令牌。
	if err := db.Transaction(func(tx *database.DB) error {
		if err := tx.Model(&core.UserPasswordReset{}).
			Where("user_id = ? AND used = ?", user.ID, false).
			Update("used", true).Error; err != nil {
			return errors.WithStack(err)
		}
		return user.ResetPassword(tx, in.NewPassword)
	}); err != nil {
		logger.Errorf(c, nil, "cannot reset password of user %s, cause: %v", user.UnionID.String(), err)
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// UserSelfPasswordReset 处理用户密码重置请求
// 路由：POST /api/v1/user/self/password/reset
// 功能：验证旧密码后更新为新密码
func UserSelfPasswordReset(c *gin.Context) {
	var in UserPasswordResetIn
	// 绑定并验证请求体
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	// 从上下文中获取当前用户
	user, codeErr := middlewares.GetUser(c)
	if codeErr != nil {
		codeErr.AbortWithGinH(c)
		return
	}
	db := middlewares.GetCoreDatabase(c)
	// 从数据库重新加载用户信息（必须执行SQL查询以确保安全）
	err := db.Take(&user, &core.User{UnionID: user.UnionID, Active: true}).Error
	// 验证旧密码
	passwordValid := user.Password != nil && crypto.CheckPassword(user.Password.Value(), in.OldPassword)
	if err != nil || user.ID == 0 || !passwordValid {
		ErrOldPasswordCheckFailed.AbortWithGinH(c)
		return
	} else if err := user.ResetPassword(db, in.NewPassword); err != nil {
		// 密码重置失败
		logger := middlewares.GetLogger(c)
		logger.Errorf(c, nil, "cannot reset password, cause: %v", err)
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else {
		// 密码重置成功
		response.SimpleOK.AbortWithGinH(c)
		return
	}
}

// UserSelfProfileChange 处理用户个人资料修改请求
// 路由：POST /api/v1/user/self/profile/change
// 功能：更新用户昵称和邮箱，并刷新JWT令牌
func UserSelfProfileChange(c *gin.Context) {
	var in UserProfileChangeIn
	// 绑定并验证请求体
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 从上下文中获取当前用户
	user, codeErr := middlewares.GetUser(c)
	if codeErr != nil {
		codeErr.AbortWithGinH(c)
		return
	}
	// 更新用户个人资料
	db := middlewares.GetCoreDatabase(c)
	if err := db.Model(&user).Updates(core.User{NickleName: in.NickleName, Email: in.Email}).Error; err != nil {
		logger := middlewares.GetLogger(c)
		logger.Errorf(c, nil, "cannot change user %s's profile, cause: %v", user.UnionID.String(), err)
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 更新用户信息并刷新JWT令牌
	auth := middlewares.GetJWT(c)
	user.NickleName = in.NickleName
	user.Email = in.Email
	if token, err := auth.Sign(user); err != nil {
		err.AbortWithGinH(c)
		return
	} else {
		// 将新令牌注入Cookie
		auth.TokenCookieInject(c, token)
		response.SimpleOK.AbortWithGinH(c)
		return
	}
}

// UserSelfJwtRefresh 处理JWT令牌刷新请求
// 路由：POST /api/v1/user/jwt/refresh（豁免强制认证与授权，用过期 access token 换发）
// 功能：刷新当前用户的JWT令牌
func UserSelfJwtRefresh(c *gin.Context) {
	auth := middlewares.GetJWT(c)
	// 刷新令牌
	newToken, codeErr := auth.Refresh(c)
	if codeErr != nil {
		codeErr.AbortWithGinH(c)
		return
	}
	// 将新令牌注入Cookie，并在响应体中返回令牌，供跨源前端持久化使用
	auth.TokenCookieInject(c, newToken)
	response.SimpleOK.AbortWithGinW(c, UserTokenOut{Token: newToken})
}

// visibleNamespaces 过滤内置系统命名空间：它承载代码声明的共享数据（如原生接口目录），
// 不作为可切换的租户暴露给前端命名空间选择器。
func visibleNamespaces(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		if name == base.SystemNamespace {
			continue
		}
		out = append(out, name)
	}
	return out
}

// UserSelfAuthorizationInfoGet 获取用户授权信息
// 路由：POST /api/v1/user/self/authorization-info/get
// 功能：获取当前用户的RBAC策略和可访问的命名空间
func UserSelfAuthorizationInfoGet(c *gin.Context) {
	// 从上下文中获取当前用户
	user, codeErr := middlewares.GetUser(c)
	if codeErr != nil {
		codeErr.AbortWithGinH(c)
		return
	}
	logger := middlewares.GetLogger(c)
	db := middlewares.GetCoreDatabase(c)

	// 查询用户所属的用户组
	var groups []*core.RbacPolicy
	if err := db.Find(&groups, "ptype = ? AND v0 = ?", base.GroupLabel, user.UnionID.String()).Error; err != nil {
		logger.Errorf(c, nil, "bad groups, cause: %v", err)
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 提取用户组名称
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		names = append(names, group.V1)
	}

	// 查询用户组对应的策略
	var policies []*core.RbacPolicy
	if err := db.Find(&policies, "ptype = ? AND v0 IN ?", base.PolicyLabel, names).Error; err != nil {
		logger.Errorf(c, nil, "bad policies, cause: %v", err)
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 构建返回结果
	out := AuthorizationInfoOut{
		Model:      base.RbacModel,
		Policies:   make([][]string, 0, len(groups)+len(policies)),
		Namespaces: visibleNamespaces(core.Cache.All(c)),
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
	response.SimpleOK.AbortWithGinW(c, out)
}

// UserSelfAccessKeyListGet 获取用户访问密钥列表
// 路由：POST /api/v1/user/self/access-key/list
// 功能：获取当前用户的所有访问密钥
func UserSelfAccessKeyListGet(c *gin.Context) {
	// 从上下文中获取当前用户
	user, codeErr := middlewares.GetUser(c)
	if codeErr != nil {
		codeErr.AbortWithGinH(c)
		return
	}
	logger := middlewares.GetLogger(c)
	db := middlewares.GetCoreDatabase(c)

	// 查询用户的所有访问密钥
	var keys []*core.AccessKey
	if err := db.Find(&keys, "user_id = ?", user.ID).Error; err != nil {
		logger.Errorf(c, nil, "bad access key, cause: %v", err)
		response.CreateAccidentError(err).AbortWithGinH(c)
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
	response.SimpleOK.AbortWithGinW(c, out)
}

// UserSelfAccessKeyCreate 创建访问密钥
// 路由：POST /api/v1/user/self/access-key/create
// 功能：为用户创建新的访问密钥
func UserSelfAccessKeyCreate(c *gin.Context) {
	// 从上下文中获取当前用户
	user, codeErr := middlewares.GetUser(c)
	if codeErr != nil {
		codeErr.AbortWithGinH(c)
		return
	}

	var in UserAccessKeyCreateIn
	// 设置默认值
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 绑定并验证请求体
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}

	db := middlewares.GetCoreDatabase(c)
	// 创建新的访问密钥对象
	key, err := core.NewAccessKey(user, in.Enabled, in.Description, in.ExpiredAt)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 获取配置信息
	cfg := config.GetConfig(c)

	// 使用复用方法在事务中创建访问密钥
	if err := key.CreateWithLimit(db, int(cfg.Auth.AccessKeyCount)); err != nil {
		if err == core.ErrTooManyAccessKey {
			ErrTooManyAccessKey.AbortWithGinH(c)
		} else {
			response.CreateAccidentError(err).AbortWithGinH(c)
		}
		return
	}

	// 创建成功，返回创建的访问密钥信息
	response.SimpleOK.AbortWithGinW(c, UserAccessKeyOut{
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

// UserSelfAccessKeyRemove 删除访问密钥
// 路由：POST /api/v1/user/self/access-key/:id/remove
// 功能：删除指定ID的访问密钥
func UserSelfAccessKeyRemove(c *gin.Context) {
	// 从上下文中获取当前用户
	user, codeErr := middlewares.GetUser(c)
	if codeErr != nil {
		codeErr.AbortWithGinH(c)
		return
	}

	// 获取URL路径参数
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}

	// 删除访问密钥
	db := middlewares.GetCoreDatabase(c)
	if err := db.Delete(&core.AccessKey{}, "id = ? AND user_id = ?", id, user.ID).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 返回成功响应
	response.SimpleOK.AbortWithGinH(c)
}

// UserSelfAccessKeyEnabled 启用或禁用访问密钥
// 路由：
//
//	POST /api/v1/user/self/access-key/:id/activate   - 启用
//	POST /api/v1/user/self/access-key/:id/deactivate - 禁用
//
// 功能：更新访问密钥的启用状态
func UserSelfAccessKeyEnabled(enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从上下文中获取当前用户
		user, codeErr := middlewares.GetUser(c)
		if codeErr != nil {
			codeErr.AbortWithGinH(c)
			return
		}

		// 获取URL路径参数
		id, ok := views.GetUint64Param(c, "id")
		if !ok {
			return
		}

		// 更新访问密钥的启用状态
		db := middlewares.GetCoreDatabase(c)
		var key core.AccessKey
		if err := db.Model(&key).Where("id = ? AND user_id = ?", id, user.ID).Update("enabled", enabled).Scan(&key).Error; err != nil {
			response.CreateAccidentError(err).AbortWithGinH(c)
			return
		}

		// 返回更新后的访问密钥信息
		response.SimpleOK.AbortWithGinW(c, UserAccessKeyOut{
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

// UserSelfFrontendConfigGet 获取当前用户的前端页面配置
// 路由：POST /api/v1/user/self/frontend-config/get
// 功能：返回用户保存的配置内容、哈希、版本号与同步开关状态，
// 前端登录后据此恢复跨主机的页面配置
func UserSelfFrontendConfigGet(c *gin.Context) {
	// 从上下文中获取当前用户
	user, codeErr := middlewares.GetUser(c)
	if codeErr != nil {
		codeErr.AbortWithGinH(c)
		return
	}
	logger := middlewares.GetLogger(c)
	db := middlewares.GetCoreDatabase(c)
	// 从数据库重新加载用户配置（必须执行 SQL 查询以确保安全）
	var target core.User
	if err := db.Take(&target, &core.User{UnionID: user.UnionID}).Error; err != nil {
		logger.Errorf(c, nil, "cannot load frontend config of user %s, cause: %v", user.UnionID.String(), err)
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 返回配置状态
	response.SimpleOK.AbortWithGinW(c, UserFrontendConfigOut{
		SyncEnabled: target.FrontendConfigSyncEnabled,
		Hash:        target.FrontendConfigHash,
		Version:     target.FrontendConfigVersion,
		Config:      target.FrontendConfig,
	})
}

// UserSelfFrontendConfigSync 同步上传当前用户的前端页面配置
// 路由：POST /api/v1/user/self/frontend-config/sync
// 功能：使用乐观锁条件更新保存配置，并发安全：
//   - 必须携带上一次同步的哈希与版本号，与数据库任一不一致则拒绝写入（其他浏览器已抢先更新）
//   - 同步开关未开启时拒绝接收配置
//
// 响应：返回同步后的最新配置状态（哈希、版本号、配置内容）
func UserSelfFrontendConfigSync(c *gin.Context) {
	var in UserFrontendConfigSyncIn
	// 绑定并验证请求体
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	// 从上下文中获取当前用户
	user, codeErr := middlewares.GetUser(c)
	if codeErr != nil {
		codeErr.AbortWithGinH(c)
		return
	}
	logger := middlewares.GetLogger(c)
	db := middlewares.GetCoreDatabase(c)
	// 从数据库重新加载用户配置（必须执行 SQL 查询以确保安全）
	var target core.User
	if err := db.Take(&target, &core.User{UnionID: user.UnionID}).Error; err != nil {
		logger.Errorf(c, nil, "cannot load frontend config of user %s, cause: %v", user.UnionID.String(), err)
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 校验同步开关，未开启时拒绝接收配置上传
	if !target.FrontendConfigSyncEnabled {
		ErrFrontendConfigSyncDisabled.AbortWithGinH(c)
		return
	}
	// 计算新配置的哈希与版本号
	newHash := frontendConfigHash(in.Config)
	newVersion := target.FrontendConfigVersion + 1
	// 乐观锁条件更新：哈希与版本号任一不一致（其他浏览器已更新）则 RowsAffected 为 0
	result := db.Model(&core.User{}).
		Where("union_id = ? AND frontend_config_hash = ? AND frontend_config_version = ?",
			user.UnionID.String(), in.PrevHash, in.PrevVersion).
		Updates(map[string]any{
			"frontend_config":         in.Config,
			"frontend_config_hash":    newHash,
			"frontend_config_version": newVersion,
		})
	if result.Error != nil {
		logger.Errorf(c, nil, "cannot sync frontend config of user %s, cause: %v", user.UnionID.String(), result.Error)
		response.CreateAccidentError(result.Error).AbortWithGinH(c)
		return
	} else if result.RowsAffected == 0 {
		// 并发竞态：其他浏览器已抢先更新，拒绝本次写入
		ErrFrontendConfigConflict.AbortWithGinH(c)
		return
	}
	// 返回同步后的最新配置状态
	response.SimpleOK.AbortWithGinW(c, UserFrontendConfigOut{
		SyncEnabled: true,
		Hash:        newHash,
		Version:     newVersion,
		Config:      in.Config,
	})
}

// UserSelfFrontendConfigSyncEnabledSet 设置当前用户的前端配置同步开关
// 路由：POST /api/v1/user/self/frontend-config/sync-enabled/set
// 功能：开启后允许浏览器上传配置，关闭后后端拒绝接收配置上传
func UserSelfFrontendConfigSyncEnabledSet(c *gin.Context) {
	var in UserFrontendConfigSyncEnabledIn
	// 绑定并验证请求体
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	// 从上下文中获取当前用户
	user, codeErr := middlewares.GetUser(c)
	if codeErr != nil {
		codeErr.AbortWithGinH(c)
		return
	}
	logger := middlewares.GetLogger(c)
	db := middlewares.GetCoreDatabase(c)
	if err := db.Model(&core.User{}).Where("union_id = ?", user.UnionID.String()).
		Update("frontend_config_sync_enabled", in.Enabled).Error; err != nil {
		logger.Errorf(c, nil, "cannot set frontend config sync enabled of user %s, cause: %v", user.UnionID.String(), err)
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 返回成功响应
	response.SimpleOK.AbortWithGinH(c)
}
