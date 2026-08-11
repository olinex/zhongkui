// Package routes 提供HTTP路由注册和管理功能。
// 本文件包含用户组管理相关的API路由处理函数，支持用户组的CRUD操作以及RBAC策略管理。
package routes

import (
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errgroupx"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// GroupRbacPolicyIn 定义组RBAC策略输入结构体。
// 用于为指定用户组添加或移除RBAC权限策略。
type GroupRbacPolicyIn struct {
	// Target 表示目标资源路径，如API端点或资源标识符。
	// 必须提供，长度限制为1-512个字符。
	Target string `json:"target" binding:"required,min=1,max=512"`

	// Action 表示对目标资源的操作类型，如HTTP方法或自定义操作。
	// 必须提供，长度限制为1-64个字符。
	Action string `json:"action" binding:"required,min=1,max=64"`

	// Effect 表示权限效果，只能是"allow"(允许)或"deny"(拒绝)。
	// 必须提供，用于定义策略是允许访问还是拒绝访问。
	Effect string `json:"effect" binding:"required,oneof=allow deny"`
}

// GroupUpdateIn 定义组更新输入结构体。
// 用于更新现有用户组的描述信息。
type GroupUpdateIn struct {
	// Description 表示用户组的描述信息。
	// 必须提供，长度限制为6-256个字符。
	Description string `json:"description" binding:"required,min=6,max=256"`
}

// GroupCreateIn 定义组创建输入结构体。
// 用于创建新的用户组，包含组名和描述信息。
type GroupCreateIn struct {
	GroupUpdateIn // 嵌入GroupUpdateIn以复用描述字段的验证规则

	// Name 表示用户组的唯一名称。
	// 必须提供，长度限制为3-32个字符，用于唯一标识用户组。
	Name string `json:"name" binding:"required,min=3,max=32"`
}

// GroupBaseDetailOut 定义组基础详情输出结构体。
// 包含用户组的基本信息和系统字段。
type GroupBaseDetailOut struct {
	utils.BaseItemOut // 嵌入基础输出结构，包含ID、创建时间、更新时间等系统字段

	// Name 表示用户组的唯一名称。
	Name string `json:"name"`

	// Description 表示用户组的描述信息。
	Description string `json:"description"`
}

// GroupDetailOut 定义组完整详情输出结构体。
// 包含用户组的基本信息及其关联的RBAC策略列表。
type GroupDetailOut struct {
	GroupBaseDetailOut // 嵌入基础详情结构

	// Policies 表示该用户组关联的所有RBAC权限策略。
	// 每个策略定义了组对特定资源的操作权限。
	Policies []GroupRbacPolicyDetailOut `json:"policies"`
}

// GroupListOut 定义组列表输出结构体。
// 用于分页查询用户组列表时的返回结构。
type GroupListOut struct {
	GroupBaseDetailOut // 嵌入基础详情结构

	utils.BaseTotalOut // 嵌入分页统计信息，包含总记录数等
}

// groupListGet 处理获取用户组列表请求。
// 支持分页、排序和过滤查询，返回符合条件的用户组列表。
//
// 参数:
//   - c *gin.Context: Gin框架的上下文对象，包含HTTP请求和响应信息
//
// 请求体:
//   - 使用GroupListGetSearchIn结构体，支持分页参数、排序条件和过滤条件
//
// 响应:
//   - 成功: 返回包含用户组列表和分页信息的JSON响应
//   - 失败: 返回相应的错误信息
func groupListGet(c *gin.Context) {
	// 初始化搜索输入参数，设置默认值
	var in GroupListGetSearchIn
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
	var items []*GroupListOut

	// 获取数据库连接并指定查询模型
	db := manager.GetCoreDatabase(c).Model(&core.Group{})

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

// groupDetailGet 处理获取用户组详情请求。
// 根据组名查询用户组的详细信息，包括基本信息和关联的RBAC策略。
//
// 参数:
//   - c *gin.Context: Gin框架的上下文对象，包含HTTP请求和响应信息
//
// URL参数:
//   - name: 用户组的唯一名称，通过路径参数传递
//
// 响应:
//   - 成功: 返回包含用户组基本信息和RBAC策略列表的JSON响应
//   - 失败: 返回相应的错误信息（如组不存在）
func groupDetailGet(c *gin.Context) {
	// 从URL路径参数中获取组名
	name, ok := getRawParam(c, "name")
	if !ok {
		return // getRawParam内部已处理错误响应
	}

	// 获取数据库连接
	db := manager.GetCoreDatabase(c)

	// 准备查询结果容器
	var baseOut GroupBaseDetailOut
	var policiesOut []GroupRbacPolicyDetailOut

	// 查询用户组基本信息
	if err := db.Model(&core.Group{}).First(&baseOut, "name = ?", name).Error; err != nil {
		// 处理查询错误（如组不存在）
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 查询该用户组关联的RBAC策略
	// 查询条件: ptype='p'(策略类型) 且 v0=组名
	if err := db.Model(&core.RbacPolicy{}).Find(&policiesOut, "ptype = ? AND v0 = ?", base.PolicyLabel, baseOut.Name).Error; err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 构建完整的响应数据
	manager.SimpleOK.AbortWithGinW(c, GroupDetailOut{
		GroupBaseDetailOut: baseOut,
		Policies:           policiesOut,
	})
}

// groupUpdate 处理更新用户组信息请求。
// 更新指定用户组的描述信息，组名作为唯一标识不可修改。
//
// 参数:
//   - c *gin.Context: Gin框架的上下文对象，包含HTTP请求和响应信息
//
// URL参数:
//   - name: 要更新的用户组的唯一名称
//
// 请求体:
//   - 使用GroupUpdateIn结构体，包含新的描述信息
//
// 响应:
//   - 成功: 返回操作成功的响应
//   - 失败: 返回相应的错误信息（如组不存在、验证失败）
func groupUpdate(c *gin.Context) {
	// 从URL路径参数中获取要更新的组名
	name, ok := getRawParam(c, "name")
	if !ok {
		return // getRawParam内部已处理错误响应
	}

	// 绑定并验证JSON请求体
	var in GroupUpdateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}

	// 获取数据库连接
	db := manager.GetCoreDatabase(c)

	// 更新用户组描述信息
	// 使用Updates方法只更新Description字段，保持其他字段不变
	result := db.Model(&core.Group{}).Where("name = ?", name).Updates(core.Group{Description: in.Description})
	if result.Error != nil {
		manager.CreateAccidentError(result.Error).AbortWithGinH(c)
		return
	}

	// 检查是否实际更新了记录
	if result.RowsAffected == 0 {
		// 用户组不存在
		manager.CreateAccidentError(gorm.ErrRecordNotFound).AbortWithGinH(c)
		return
	}

	// 返回操作成功的响应
	manager.SimpleOK.AbortWithGinH(c)
}

// groupCreate 处理创建用户组请求。
// 创建新的用户组，组名必须唯一，创建后不可修改组名。
//
// 参数:
//   - c *gin.Context: Gin框架的上下文对象，包含HTTP请求和响应信息
//
// 请求体:
//   - 使用GroupCreateIn结构体，包含组名和描述信息
//
// 响应:
//   - 成功: 返回操作成功的响应
//   - 失败: 返回相应的错误信息（如组名已存在、验证失败）
func groupCreate(c *gin.Context) {
	// 初始化创建输入参数，设置默认值
	var in GroupCreateIn
	if err := defaults.Set(&in); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 绑定并验证JSON请求体
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}

	// 获取数据库连接
	db := manager.GetCoreDatabase(c)

	// 创建新的用户组记录
	// 使用core.Group模型，确保数据完整性约束
	if err := db.Create(&core.Group{
		Name:        in.Name,
		Description: in.Description,
	}).Error; err != nil {
		// 处理创建错误（如唯一约束冲突）
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 返回操作成功的响应
	manager.SimpleOK.AbortWithGinH(c)
}

// groupDelete 处理删除用户组请求。
// 删除指定用户组，在删除前检查是否存在关联的RBAC策略记录。
// 使用数据库事务确保操作的原子性。
//
// 参数:
//   - c *gin.Context: Gin框架的上下文对象，包含HTTP请求和响应信息
//
// URL参数:
//   - name: 要删除的用户组的唯一名称
//
// 响应:
//   - 成功: 返回操作成功的响应
//   - 失败: 返回相应的错误信息（如组不存在、存在关联记录）
//
// 注意:
//   - 删除操作会检查用户组是否关联了RBAC策略
//   - 使用事务确保数据一致性
//   - 如果存在关联记录，返回特定错误提示
func groupDelete(c *gin.Context) {
	// 从URL路径参数中获取要删除的组名
	name, ok := getRawParam(c, "name")
	if !ok {
		return // getRawParam内部已处理错误响应
	}

	// 获取数据库连接
	db := manager.GetCoreDatabase(c)

	// 使用事务执行删除操作，确保数据一致性
	db.Transaction(func(tx *database.DB) error {
		// 检查用户组是否关联了RBAC策略记录
		// 查询条件:
		// 1. ptype='p' AND v0=组名: 组定义的策略
		// 2. ptype='g' AND v1=组名: 用户属于该组的关联关系
		var policy core.RbacPolicy
		if err := tx.First(
			&policy,
			`(ptype = ? AND v0 = ?) OR (ptype = ? AND v1 = ?)`,
			base.PolicyLabel,
			name,
			base.GroupLabel,
			name,
		).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				// 处理查询错误（非"记录不存在"错误）
				if errors.Is(err, gorm.ErrForeignKeyViolated) {
					// 外键约束冲突，表示存在关联记录
					manager.ErrCannotDeleteCauseRelateRecord.AbortWithGinH(c)
				} else {
					// 其他数据库错误
					manager.CreateAccidentError(err).AbortWithGinH(c)
				}
				return errors.WithStack(err)
			}
			// 记录不存在，表示没有关联策略，可以继续删除
		} else {
			// 找到关联策略，阻止删除
			manager.ErrCannotDeleteCauseRelateRecord.AbortWithGinH(c)
			return errors.WithStack(gorm.ErrRecordNotFound) // 返回一个错误以回滚事务
		}

		// 执行删除操作
		if err := tx.Delete(&core.Group{}, "name = ?", name).Error; err != nil {
			manager.CreateAccidentError(err).AbortWithGinH(c)
			return errors.WithStack(err)
		}

		// 返回操作成功的响应
		manager.SimpleOK.AbortWithGinH(c)
		return nil
	})
}

// groupRbacPolicyAdd 处理为组添加RBAC策略请求。
// 为指定用户组添加新的权限策略，同时验证策略模板是否存在。
// 使用事务和并发查询确保操作的原子性和性能。
//
// 参数:
//   - c *gin.Context: Gin框架的上下文对象，包含HTTP请求和响应信息
//
// URL参数:
//   - name: 要添加策略的用户组的唯一名称
//
// 请求体:
//   - 使用GroupRbacPolicyIn结构体，包含目标、操作和效果信息
//
// 响应:
//   - 成功: 返回创建的策略详情
//   - 失败: 返回相应的错误信息（如策略模板不存在、验证失败）
//
// 注意:
//   - 策略添加前会验证对应的策略模板是否存在
//   - 使用事务确保模板验证和策略创建的原子性
//   - 使用并发查询提高性能
func groupRbacPolicyAdd(c *gin.Context) {
	// 从URL路径参数中获取组名
	name, ok := getRawParam(c, "name")
	if !ok {
		return // getRawParam内部已处理错误响应
	}

	// 初始化策略输入参数，设置默认值
	var in GroupRbacPolicyIn
	if err := defaults.Set(&in); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 绑定并验证JSON请求体
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}

	// 获取数据库连接
	db := manager.GetCoreDatabase(c)

	// 创建新的组策略实例
	policy := core.NewGroupPolicy(name, in.Target, in.Action, in.Effect)

	// 使用事务执行策略添加操作
	err := db.Transaction(func(tx *database.DB) error {
		var template core.RbacPolicyTemplate

		// 创建错误组用于并发执行查询
		wg, innerCtx := errgroupx.WithContext(c)

		// 并发查询1: 验证策略模板是否存在
		wg.GoNamed("check", func() error {
			return tx.WithContext(innerCtx).First(&template, "target = ? AND action = ?", in.Target, in.Action).Error
		})

		// 并发查询2: 创建策略记录
		wg.GoNamed("create", func() error {
			return tx.WithContext(innerCtx).Create(&policy).Error
		})

		// 等待所有并发操作完成
		return wg.Wait()
	})

	// 处理事务执行错误
	if err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 返回创建成功的策略详情
	manager.SimpleOK.AbortWithGinW(c, GroupRbacPolicyDetailOut{
		BaseItemOut: utils.BaseItemOut{
			ID:        policy.ID,
			CreatedAt: policy.CreatedAt,
			UpdatedAt: policy.UpdatedAt,
		},
		V1: policy.V1, // 对应target字段
		V2: policy.V2, // 对应action字段
		V3: policy.V3, // 对应effect字段
	})
}

// groupRbacPolicyRemove 处理从组移除RBAC策略请求。
// 删除指定用户组的特定权限策略，通过精确匹配策略的所有属性来确保删除正确性。
//
// 参数:
//   - c *gin.Context: Gin框架的上下文对象，包含HTTP请求和响应信息
//
// URL参数:
//   - name: 要移除策略的用户组的唯一名称
//
// 请求体:
//   - 使用GroupRbacPolicyIn结构体，包含要移除策略的目标、操作和效果信息
//
// 响应:
//   - 成功: 返回操作成功的响应
//   - 失败: 返回相应的错误信息（如策略不存在）
//
// 注意:
//   - 策略删除需要精确匹配所有字段: ptype、v0(组名)、v1(target)、v2(action)、v3(effect)
//   - 使用软删除或硬删除取决于数据库配置
func groupRbacPolicyRemove(c *gin.Context) {
	// 从URL路径参数中获取组名
	name, ok := getRawParam(c, "name")
	if !ok {
		return // getRawParam内部已处理错误响应
	}

	// 绑定并验证JSON请求体
	var in GroupRbacPolicyIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}

	// 获取数据库连接
	db := manager.GetCoreDatabase(c)

	// 执行策略删除操作
	// 删除条件需要精确匹配所有字段以确保删除正确的策略
	if err := db.Delete(
		&core.RbacPolicy{},
		"ptype = ? AND v0 = ? AND v1 = ? AND v2 = ? AND v3 = ?",
		base.PolicyLabel, // 策略类型为'p'（组策略）
		name,             // 组名
		in.Target,        // 目标资源
		in.Action,        // 操作类型
		in.Effect,        // 效果（allow/deny）
	).Error; err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 返回操作成功的响应
	manager.SimpleOK.AbortWithGinW(c, nil)
}
