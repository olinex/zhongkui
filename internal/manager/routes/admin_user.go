package routes

import (
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

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
	db := manager.GetDatabase(c).Model(&core.User{})

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
	db := manager.GetDatabase(c)

	// 查询用户基本信息
	var baseOut UserBaseDetailOut
	var groupsOut []UserRbacPolicyDetailOut
	if err := db.Model(&core.User{}).Find(&baseOut, "union_id = ?", unionID).Error; err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := db.Model(&core.RbacPolicy{}).Find(&groupsOut, "ptype = ? AND v0 = ?", base.GROUP_LABEL, baseOut.UnionID.String()).Error; err != nil {
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
	db := manager.GetDatabase(c)

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

// userSetActive 创建用户激活/禁用状态设置的处理函数
// 这是一个高阶函数，根据传入的active参数返回具体的Gin处理函数
//
// 参数:
//   - active bool: 要设置的用户状态，true表示激活，false表示禁用
//
// 返回值:
//   - gin.HandlerFunc: 处理用户状态设置的Gin处理函数
//
// 处理函数功能:
//   - 根据用户unionID设置用户的激活状态
//   - 支持激活用户（active=true）或禁用用户（active=false）
func userSetActive(active bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从URL参数中获取用户unionID并验证
		unionID, ok := getUUIDParam(c, "unionID")
		if !ok {
			return
		}

		// 获取数据库连接
		db := manager.GetDatabase(c)

		// 更新用户的激活状态
		if err := db.Model(&core.User{}).Where("union_id = ?", unionID).Update("active", active).Error; err != nil {
			manager.CreateAccidentError(err).AbortWithGinH(c)
			return
		}

		// 返回操作成功的响应
		manager.SimpleOK.AbortWithGinH(c)
	}
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
	db := manager.GetDatabase(c)

	// 创建用户策略对象
	policy := core.NewUserPolicy(unionID, in.GroupName, in.Namespace)

	// 在事务中执行所有验证和创建操作
	err := db.Transaction(func(tx *database.DB) error {
		// 定义要验证的实体
		var user core.User
		var group core.Group
		var namespace core.Namespace

		// 使用errgroup并发执行验证操作
		wg, innerCtx := errgroup.WithContext(c)

		// 验证用户存在且处于激活状态
		wg.Go(func() error {
			return tx.WithContext(innerCtx).First(&user, "union_id = ? AND active = true", unionID).Error
		})

		// 验证用户组存在
		wg.Go(func() error {
			return tx.WithContext(innerCtx).First(&group, "name = ?", in.GroupName).Error
		})

		// 验证命名空间存在（除非使用通配命名空间）
		wg.Go(func() error {
			if in.Namespace == base.WIDE_MATCH_LABEL {
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
	db := manager.GetDatabase(c)

	// 删除用户与用户组的关联策略
	// 使用精确匹配：策略类型为GROUP_LABEL，用户ID、组名和命名空间完全匹配
	if err := db.Delete(
		&core.RbacPolicy{},
		"ptype = ? AND v0 = ? AND v1 = ? AND v2 = ?",
		base.GROUP_LABEL,
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
