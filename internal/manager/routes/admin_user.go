package routes

import (
	"net/http"

	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errgroupx"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/utils"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

// randomPasswordLength 定义管理员重置用户密码时随机生成密码的长度，
// 该长度满足 AdminUserPasswordResetIn 的 min=8 约束。
const randomPasswordLength = 10

// UserGroupIn 定义用户加入/离开用户组的输入参数结构体
// 用于处理用户与用户组关系的操作，包含命名空间和组名信息
type UserGroupIn struct {
	// Namespace 表示命名空间名称，用于资源隔离
	// 必须提供，长度限制为1-32个字符
	Namespace string `json:"namespace" binding:"required,min=1,max=32"`

	// GroupName 表示用户组名称，标识用户要加入或离开的组
	// 必须提供，长度限制为3-32个字符
	GroupName string `json:"groupName" binding:"required,min=3,max=32"`
}

// UserCreateIn 定义管理员创建用户的输入结构体
// 继承个人资料与登录凭据结构，包含用户名、密码、昵称和邮箱
type UserCreateIn struct {
	UserProfileChangeIn
	UserPasswordLoginIn
}

// AdminUserPasswordResetIn 定义管理员重置用户密码的输入结构体
//
// 该结构体用于接收管理员强制重置用户密码的请求参数，包含以下字段：
//   - Password: 新密码，可选。若提供则必须满足强密码要求（8-64个ASCII字符）；
//     若为空或省略，则由系统自动生成符合强密码要求的随机密码
type AdminUserPasswordResetIn struct {
	Password string `json:"password" binding:"omitempty,min=8,max=64,ascii,strongPassword"`
}

// AdminUserPasswordResetOut 定义管理员重置用户密码的输出结构体
//
// 该结构体用于返回重置后的明文密码（仅一次性展示），
// 管理员须将新密码安全地告知目标用户。
type AdminUserPasswordResetOut struct {
	Password string `json:"password"`
}

// UserBaseDetailOut 定义用户基础详情输出结构体
// 包含用户的基本信息和系统字段，用于用户详情和列表展示
type UserBaseDetailOut struct {
	utils.BaseSoftDeleteItemOut // 嵌入基础软删除输出结构，包含ID、创建时间、更新时间、删除时间等系统字段

	// UnionID 表示用户的全局唯一标识符
	// 使用HexUUID类型，确保在整个系统中的唯一性
	UnionID serializer.HexUUID `json:"unionID"`

	// NickleName 表示用户的昵称
	// 用于显示和个性化标识
	NickleName string `json:"nickleName"`

	// Username 表示用户的登录用户名
	// 用于用户登录认证，具有唯一性
	Username string `json:"username"`

	// Email 表示用户的电子邮件地址
	// 用于用户通信和密码重置，具有唯一性
	Email string `json:"email"`

	// Active 表示用户是否处于激活状态
	// 当用户被禁用时，该字段为false，用户将无法登录系统
	Active bool `json:"active"`

	// FrontendConfigSyncEnabled 表示用户是否开启了前端配置同步
	// 供管理员在列表中识别已启用配置同步的用户
	FrontendConfigSyncEnabled bool `json:"frontendConfigSyncEnabled"`
}

// UserDetailOut 定义用户完整详情输出结构体
// 包含用户的基本信息及其关联的用户组列表，用于用户详情查询
type UserDetailOut struct {
	UserBaseDetailOut // 嵌入基础详情结构

	// Groups 表示用户所属的所有用户组
	// 每个组条目包含组名和命名空间信息
	Groups []UserRbacPolicyDetailOut `json:"groups"`
}

// UserListOut 定义用户列表输出结构体
// 用于分页查询用户列表时的返回结构，包含分页统计信息
type UserListOut struct {
	UserBaseDetailOut // 嵌入基础详情结构

	utils.BaseTotalOut // 嵌入分页统计信息，包含总记录数等
}

// userListGet 处理获取用户列表请求
// 支持分页、排序和过滤查询，返回符合条件的用户列表
//
// 参数:
//   - c *gin.Context: Gin框架的上下文对象，包含HTTP请求和响应信息
//
// 请求体:
//   - 使用UserListGetSearchIn结构体，支持分页参数、排序条件和过滤条件
//
// 响应:
//   - 成功: 返回包含用户列表和分页信息的JSON响应
//   - 失败: 返回相应的错误信息
func userListGet(c *gin.Context) {
	// 初始化搜索输入参数，设置默认值
	var in UserListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 绑定并验证JSON请求体
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}

	// 准备查询结果容器
	var items []*UserListOut

	// 获取数据库连接并指定查询模型
	db := manager.GetCoreDatabase(c).Model(&core.User{})

	// 构建搜索查询，应用分页、排序和过滤条件
	if query, err := utils.BuildSearchQuery(db, &in); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&items).Error; err != nil {
		// 执行查询并处理结果
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 构建搜索输出结果，包含数据和分页信息
	out := utils.NewSearchOut(items)

	// 返回成功的JSON响应
	manager.SimpleOK.AbortWithGinW(c, out)
}

// userCreate 处理管理员创建用户请求
// 路由：POST /api/v1/admin/user/create
// 功能：创建新用户账户，新用户默认激活并加入默认用户组
//
// 参数:
//   - c *gin.Context: Gin框架的上下文对象，包含HTTP请求和响应信息
//
// 请求体:
//   - 使用UserCreateIn结构体，包含用户名、密码、昵称和邮箱
//
// 响应:
//   - 成功: 返回操作成功的响应
//   - 失败: 返回相应的错误信息（如用户名或邮箱已存在、验证失败）
func userCreate(c *gin.Context) {
	// 初始化创建输入参数，设置默认值
	var in UserCreateIn
	if err := defaults.Set(&in); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 绑定并验证JSON请求体
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

	// 保存用户到数据库，并分配默认用户组策略
	db := manager.GetCoreDatabase(c)
	if err = user.Create(db, false); err != nil {
		ErrUsernameOrEmailAlreadyExist.AbortWithGinH(c)
		return
	}

	// 返回成功响应
	manager.SimpleOK.AbortWithGinH(c)
}

// userDetailGet 处理获取用户详情请求
// 根据用户unionID查询用户基本信息和所属用户组
//
// 参数:
//   - c *gin.Context: Gin框架的上下文对象，包含HTTP请求和响应信息
//
// URL参数:
//   - unionID: 用户的全局唯一标识符，必须为有效的UUID格式
//
// 响应:
//   - 成功: 返回包含用户基本信息和关联用户组的JSON响应
//   - 失败: 返回相应的错误信息
func userDetailGet(c *gin.Context) {
	// 从URL参数中获取用户unionID并验证
	unionID, ok := getUUIDParam(c, "unionID")
	if !ok {
		return
	}

	// 获取数据库连接
	db := manager.GetCoreDatabase(c)

	// 查询用户基本信息
	var baseOut UserBaseDetailOut
	var groupsOut []UserRbacPolicyDetailOut
	if err := db.Model(&core.User{}).Find(&baseOut, "union_id = ?", unionID).Error; err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := db.Model(&core.RbacPolicy{}).Find(&groupsOut, "ptype = ? AND v0 = ?", base.GroupLabel, baseOut.UnionID.String()).Error; err != nil {
		// 查询用户关联的用户组策略
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 返回用户详情信息，包含基本信息和用户组列表
	manager.SimpleOK.AbortWithGinW(c, UserDetailOut{
		UserBaseDetailOut: baseOut,
		Groups:            groupsOut,
	})
}

// userUpdate 处理更新用户信息请求
// 根据用户unionID更新用户的昵称和邮箱信息
//
// 参数:
//   - c *gin.Context: Gin框架的上下文对象，包含HTTP请求和响应信息
//
// URL参数:
//   - unionID: 用户的全局唯一标识符，必须为有效的UUID格式
//
// 请求体:
//   - 使用UserProfileChangeIn结构体，包含要更新的昵称和邮箱信息
//
// 响应:
//   - 成功: 返回操作成功的响应
//   - 失败: 返回相应的错误信息
func userUpdate(c *gin.Context) {
	// 从URL参数中获取用户unionID并验证
	unionID, ok := getUUIDParam(c, "unionID")
	if !ok {
		return
	}

	// 获取数据库连接
	db := manager.GetCoreDatabase(c)

	// 绑定并验证请求体
	var in UserProfileChangeIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 更新用户信息，只更新昵称和邮箱字段
	if err := db.Model(&core.User{}).Where("union_id = ?", unionID).Updates(core.User{NickleName: in.NickleName, Email: in.Email}).Error; err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 返回操作成功的响应
	manager.SimpleOK.AbortWithGinH(c)
}

// UserBatchActiveIn 批量设置用户激活状态的请求输入结构。
// UnionIds 接受标量（单个 unionID 字符串）或数组（多个 unionID 字符串）。
type UserBatchActiveIn struct {
	UnionIds FlexList[serializer.HexUUID] `json:"unionIds"`
}

// userBatchSetActive 批量设置用户激活/禁用状态的处理函数工厂。
// 逐条幂等更新：目标用户不存在（0 行更新）视为成功。
//
// 参数:
//   - active bool: 要设置的用户状态，true表示激活，false表示禁用
//
// 返回值:
//   - gin.HandlerFunc: 批量处理用户状态设置的Gin处理函数
func userBatchSetActive(active bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in UserBatchActiveIn
		if err := c.ShouldBindBodyWithJSON(&in); err != nil {
			manager.CreateBadBodyError(err).AbortWithGinH(c)
			return
		}
		unionIds := []serializer.HexUUID(in.UnionIds)
		if !validFlexLen(c, unionIds) {
			return
		}
		db := manager.GetCoreDatabase(c)
		batchExecute(c, unionIds,
			func(u serializer.HexUUID) string { return u.String() },
			func(u serializer.HexUUID) error {
				// 不存在即 0 行更新，视为成功幂等
				return db.Model(&core.User{}).Where("union_id = ?", u).Update("active", active).Error
			},
		)
	}
}

// userPasswordReset 处理管理员强制重置用户密码请求
// 路由：POST /api/v1/admin/user/:unionID/password/reset
// 功能：管理员为指定用户重置登录密码，可自行指定新密码或由系统随机生成
//
// 参数:
//   - c *gin.Context: Gin框架的上下文对象，包含HTTP请求和响应信息
//
// URL参数:
//   - unionID: 用户的全局唯一标识符，必须为有效的UUID格式
//
// 请求体:
//   - 使用AdminUserPasswordResetIn结构体，Password字段可选：
//   - 提供时使用该密码（需满足强密码要求）
//   - 为空时系统自动生成随机密码
//
// 响应:
//   - 成功: 返回重置后的明文密码（一次性展示），管理员须安全告知目标用户
//   - 失败: 返回相应的错误信息（如用户不存在、密码强度不足）
func userPasswordReset(c *gin.Context) {
	// 从URL参数中获取用户unionID并验证
	unionID, ok := getUUIDParam(c, "unionID")
	if !ok {
		return
	}

	// 绑定并验证请求体
	var in AdminUserPasswordResetIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}

	// 查询目标用户
	db := manager.GetCoreDatabase(c)
	var user core.User
	if err := db.First(&user, "union_id = ?", unionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			manager.CreateCodeError(http.StatusNotFound, manager.CodeUserNotExist, nil).AbortWithGinH(c)
		} else {
			manager.CreateAccidentError(err).AbortWithGinH(c)
		}
		return
	}

	// 确定新密码：优先使用管理员指定的密码，否则随机生成
	password := in.Password
	if password == "" {
		var err error
		if password, err = validator.GenerateStrongPassword(randomPasswordLength); err != nil {
			logger := manager.GetLogger(c)
			logger.Errorf(c, nil, "cannot generate random password for user %s, cause: %v", unionID.String(), err)
			manager.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
	}

	// 更新用户密码
	if err := user.ResetPassword(db, password); err != nil {
		logger := manager.GetLogger(c)
		logger.Errorf(c, nil, "cannot reset user %s's password, cause: %v", unionID.String(), err)
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 返回重置后的明文密码（仅一次性展示）
	manager.SimpleOK.AbortWithGinW(c, AdminUserPasswordResetOut{Password: password})
}

// userGroupJoin 处理用户加入用户组请求
// 将用户添加到指定的用户组中，支持跨命名空间的用户组关联
//
// 参数:
//   - c *gin.Context: Gin框架的上下文对象，包含HTTP请求和响应信息
//
// URL参数:
//   - unionID: 用户的全局唯一标识符，必须为有效的UUID格式
//
// 请求体:
//   - 使用UserGroupIn结构体，包含要加入的用户组名和命名空间
//
// 响应:
//   - 成功: 返回创建的用户组策略详情
//   - 失败: 返回相应的错误信息
//
// 事务处理:
//   - 验证用户存在且处于激活状态
//   - 验证用户组存在
//   - 验证命名空间存在（除非使用通配命名空间）
//   - 创建用户与用户组的关联策略
func userGroupJoin(c *gin.Context) {
	// 从URL参数中获取用户unionID并验证
	unionID, ok := getUUIDParam(c, "unionID")
	if !ok {
		return
	}

	// 绑定并验证请求体
	var in UserGroupIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}

	// 获取数据库连接
	db := manager.GetCoreDatabase(c)

	// 创建用户策略对象
	policy := core.NewUserPolicy(unionID, in.GroupName, in.Namespace)

	// 在事务中执行所有验证和创建操作
	err := db.Transaction(func(tx *database.DB) error {
		// 定义要验证的实体
		var user core.User
		var group core.Group
		var namespace core.Namespace

		// 使用errgroup并发执行验证操作
		wg, innerCtx := errgroupx.WithContext(c)

		// 验证用户存在且处于激活状态
		wg.GoNamed("check-active", func() error {
			return tx.WithContext(innerCtx).First(&user, "union_id = ? AND active = true", unionID).Error
		})

		// 验证用户组存在
		wg.GoNamed("check-name", func() error {
			return tx.WithContext(innerCtx).First(&group, "name = ?", in.GroupName).Error
		})

		// 验证命名空间存在（除非使用通配命名空间）
		wg.GoNamed("check-namespace", func() error {
			if in.Namespace == base.WideMatchLabel {
				return nil
			} else {
				return tx.WithContext(innerCtx).First(&namespace, "name = ?", in.Namespace).Error
			}
		})

		// 创建用户与用户组的关联策略
		wg.Go(func() error {
			return tx.WithContext(innerCtx).Create(&policy).Error
		})

		// 等待所有并发操作完成
		return wg.Wait()
	})

	// 处理事务执行结果
	if err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 返回创建的用户组策略详情
	manager.SimpleOK.AbortWithGinW(c, UserRbacPolicyDetailOut{
		BaseItemOut: utils.BaseItemOut{
			ID:        policy.ID,
			CreatedAt: policy.CreatedAt,
			UpdatedAt: policy.UpdatedAt,
		},
		V1: policy.V1,
		V2: policy.V2,
	})
}

// userGroupLeave 处理用户离开用户组请求
// 将用户从指定的用户组中移除，解除用户与用户组的关联
//
// 参数:
//   - c *gin.Context: Gin框架的上下文对象，包含HTTP请求和响应信息
//
// URL参数:
//   - unionID: 用户的全局唯一标识符，必须为有效的UUID格式
//
// 请求体:
//   - 使用UserGroupIn结构体，包含要离开的用户组名和命名空间
//
// 响应:
//   - 成功: 返回操作成功的响应
//   - 失败: 返回相应的错误信息
func userGroupLeave(c *gin.Context) {
	// 从URL参数中获取用户unionID并验证
	unionID, ok := getUUIDParam(c, "unionID")
	if !ok {
		return
	}

	// 绑定并验证请求体
	var in UserGroupIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}

	// 获取数据库连接
	db := manager.GetCoreDatabase(c)

	// 删除用户与用户组的关联策略
	// 使用精确匹配：策略类型为GroupLabel，用户ID、组名和命名空间完全匹配
	if err := db.Delete(
		&core.RbacPolicy{},
		"ptype = ? AND v0 = ? AND v1 = ? AND v2 = ?",
		base.GroupLabel,
		unionID.String(),
		in.GroupName,
		in.Namespace,
	).Error; err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 返回操作成功的响应
	manager.SimpleOK.AbortWithGinH(c)
}

// userFrontendConfigReset 管理员重置指定用户的前端页面配置
// 路由：POST /api/v1/admin/user/:unionID/frontend-config/reset
// 功能：清空用户的配置内容与哈希，并将版本号重置为 0；
// 同步开关状态保留不变，由用户自行决定是否继续同步
//
// 参数:
//   - c *gin.Context: Gin框架的上下文对象，包含HTTP请求和响应信息
//
// URL参数:
//   - unionID: 用户的全局唯一标识符，必须为有效的UUID格式
//
// 响应:
//   - 成功: 返回操作成功的响应
//   - 失败: 返回相应的错误信息
func userFrontendConfigReset(c *gin.Context) {
	// 从URL参数中获取用户unionID并验证
	unionID, ok := getUUIDParam(c, "unionID")
	if !ok {
		return
	}

	// 获取数据库连接
	db := manager.GetCoreDatabase(c)

	// 清空配置内容与哈希并重置版本号
	if err := db.Model(&core.User{}).Where("union_id = ?", unionID).
		Updates(map[string]any{
			"frontend_config":         "",
			"frontend_config_hash":    "",
			"frontend_config_version": 0,
		}).Error; err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 返回操作成功的响应
	manager.SimpleOK.AbortWithGinH(c)
}
