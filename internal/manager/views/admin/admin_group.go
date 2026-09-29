package admin

import (
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/manager/views"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/database"
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

// GroupListGet 处理获取用户组列表请求。
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
func GroupListGet(c *gin.Context) {
	// 初始化搜索输入参数，设置默认值
	var in views.GroupListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 绑定并验证JSON请求体
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}

	// 准备查询结果容器
	var items []*GroupListOut

	// 获取数据库连接并指定查询模型
	db := middlewares.GetCoreDatabase(c).Model(&core.Group{})

	// 构建搜索查询，应用分页、排序和过滤条件
	if query, err := utils.BuildSearchCountQuery(db, &in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&items).Error; err != nil {
		// 执行查询并处理结果
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 字段投影：按请求 fields 投影，未指定时回退全部白名单字段（封装于 utils.ProjectSearchItems）
	var total uint
	if len(items) > 0 {
		total = items[0].GetTotal()
	}
	projected := utils.ProjectSearchItems(items, &in, func(o **GroupListOut) any {
		return *o
	})
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(total, projected))
}

// GroupDetailGet 处理获取用户组详情请求。
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
func GroupDetailGet(c *gin.Context) {
	// 从URL路径参数中获取组名
	name, ok := views.GetRawParam(c, "name")
	if !ok {
		return // getRawParam内部已处理错误响应
	}

	// 获取数据库连接
	db := middlewares.GetCoreDatabase(c)

	// 准备查询结果容器
	var baseOut GroupBaseDetailOut
	var policiesOut []GroupRbacPolicyDetailOut

	// 查询用户组基本信息
	if err := db.Model(&core.Group{}).First(&baseOut, "name = ?", name).Error; err != nil {
		// 处理查询错误（如组不存在）
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 查询该用户组关联的RBAC策略
	// 查询条件: ptype='p'(策略类型) 且 v0=组名
	if err := db.Model(&core.RbacPolicy{}).Find(&policiesOut, "ptype = ? AND v0 = ?", base.PolicyLabel, baseOut.Name).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 构建完整的响应数据
	response.SimpleOK.AbortWithGinW(c, GroupDetailOut{
		GroupBaseDetailOut: baseOut,
		Policies:           policiesOut,
	})
}

// GroupUpdate 处理更新用户组信息请求。
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
func GroupUpdate(c *gin.Context) {
	// 从URL路径参数中获取要更新的组名
	name, ok := views.GetRawParam(c, "name")
	if !ok {
		return // getRawParam内部已处理错误响应
	}

	// 绑定并验证JSON请求体
	var in GroupUpdateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}

	// 获取数据库连接
	db := middlewares.GetCoreDatabase(c)

	// 更新用户组描述信息
	// 使用Updates方法只更新Description字段，保持其他字段不变
	result := db.Model(&core.Group{}).Where("name = ?", name).Updates(core.Group{Description: in.Description})
	if result.Error != nil {
		response.CreateAccidentError(result.Error).AbortWithGinH(c)
		return
	}

	// 检查是否实际更新了记录
	if result.RowsAffected == 0 {
		// 用户组不存在
		response.CreateAccidentError(gorm.ErrRecordNotFound).AbortWithGinH(c)
		return
	}

	// 返回操作成功的响应
	response.SimpleOK.AbortWithGinH(c)
}

// GroupCreate 处理创建用户组请求。
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
func GroupCreate(c *gin.Context) {
	// 初始化创建输入参数，设置默认值
	var in GroupCreateIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 绑定并验证JSON请求体
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}

	// 获取数据库连接
	db := middlewares.GetCoreDatabase(c)

	// 创建新的用户组记录
	// 使用core.Group模型，确保数据完整性约束
	if err := db.Create(&core.Group{
		Name:        in.Name,
		Description: in.Description,
	}).Error; err != nil {
		// 处理创建错误（如唯一约束冲突）
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 返回操作成功的响应
	response.SimpleOK.AbortWithGinH(c)
}

// GroupBatchDeleteIn 批量删除用户组的请求输入结构。
// Names 接受标量（单个组名）或数组（多个组名）。
type GroupBatchDeleteIn struct {
	// Names 待删除的用户组名称列表。
	Names views.FlexList[string] `json:"names"`
}

// GroupBatchDelete 批量删除用户组。
// 逐条复用原 groupDelete 的引用校验与事务删除语义：
//   - 组被 RBAC 策略引用（ptype='p' v0=组名 或 ptype='g' v1=组名）→ 该条记入 Failed；
//   - 组不存在 → 视为成功跳过（幂等）；
//   - 其他数据库错误 → 该条记入 Failed。
//
// 参数:
//   - c *gin.Context: Gin框架的上下文对象，包含HTTP请求和响应信息
//
// 请求体:
//   - 使用GroupBatchDeleteIn结构体，Names字段接受标量或数组
//
// 响应:
//   - 成功: 返回BatchResultOut汇总（succeeded + failed明细）
func GroupBatchDelete(c *gin.Context) {
	var in GroupBatchDeleteIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	names := []string(in.Names)
	if !views.ValidFlexLen(c, names) {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	views.BatchExecute(c, names,
		func(name string) string { return name },
		func(name string) error {
			return db.Transaction(func(tx *database.DB) error {
				// 以 FOR UPDATE 行锁锁定用户组行，与 UserGroupJoin 的组行校验串行化，
				// 避免"校验无引用后，并发加入引用再删除组"导致的悬挂引用（SQLite 退化为普通读）
				var group core.Group
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&group, "name = ?", name).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						// 组不存在，视为成功跳过（幂等）
						return nil
					}
					return errors.WithStack(err)
				}

				// 检查用户组是否关联了RBAC策略记录：
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
					if errors.Is(err, gorm.ErrRecordNotFound) {
						// 无引用关系，可以删除
						return tx.Delete(&core.Group{}, "name = ?", name).Error
					}
					// 其他数据库错误（含外键约束冲突）作为失败原因
					return errors.WithStack(err)
				}
				// 找到关联策略，阻止删除
				return response.ErrCannotDeleteCauseRelateRecord
			})
		},
	)
}

// GroupRbacPolicyAdd 处理为组添加RBAC策略请求。
// 为指定用户组添加新的权限策略，同时验证策略模板是否存在。
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
//   - 策略添加前会验证用户组与对应的策略模板均存在
//   - 事务内以 FOR UPDATE 行锁校验组与模板，锁保持到 enforcer 写入完成后再提交，
//     避免"校验通过后被并发删除组/模板"的竞态（SQLite 测试环境无 FOR UPDATE，退化为普通读）
func GroupRbacPolicyAdd(c *gin.Context) {
	// 从URL路径参数中获取组名
	name, ok := views.GetRawParam(c, "name")
	if !ok {
		return // getRawParam内部已处理错误响应
	}

	// 初始化策略输入参数，设置默认值
	var in GroupRbacPolicyIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 绑定并验证JSON请求体
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}

	// 获取数据库连接与 Casbin enforcer
	db := middlewares.GetCoreDatabase(c)
	enforcer := middlewares.GetRbacPolicy(c)

	// 在事务内以 FOR UPDATE 行锁校验模板，并在行锁保持期间执行 enforcer 增量写入，
	// 使内存与 rbac_policies 表同步（O(1)），同时杜绝校验与写入之间的并发修改
	if err := db.WithContext(c).Transaction(func(tx *database.DB) error {
		// 以 FOR UPDATE 行锁校验用户组存在（防并发删除组，与 GroupBatchDelete 串行化，
		// 杜绝"组删除瞬间仍写入策略"的悬挂引用；SQLite 退化为普通读）
		var group core.Group
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&group, "name = ?", name).Error; err != nil {
			return err
		}

		// 校验策略模板存在（行锁防并发删除模板）
		var template core.RbacPolicyTemplate
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&template, "target = ? AND action = ?", in.Target, in.Action).Error; err != nil {
			return err
		}

		// 行锁保持期间通过 enforcer 增量写入策略（独立连接写入，锁由本事务持有直至提交）
		if _, err := enforcer.AddPolicy(name, in.Target, in.Action, in.Effect); err != nil {
			return err
		}
		return nil
	}); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 回查落库行填充系统字段（ID/CreatedAt/UpdatedAt），保持前端响应契约
	var policy core.RbacPolicy
	if err := db.First(&policy, "ptype = ? AND v0 = ? AND v1 = ? AND v2 = ? AND v3 = ?",
		base.PolicyLabel, name, in.Target, in.Action, in.Effect).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 返回创建成功的策略详情
	response.SimpleOK.AbortWithGinW(c, GroupRbacPolicyDetailOut{
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

// GroupRbacPolicyRemove 处理从组移除RBAC策略请求。
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
func GroupRbacPolicyRemove(c *gin.Context) {
	// 从URL路径参数中获取组名
	name, ok := views.GetRawParam(c, "name")
	if !ok {
		return // getRawParam内部已处理错误响应
	}

	// 绑定并验证JSON请求体
	var in GroupRbacPolicyIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}

	// 通过 Casbin enforcer 增量移除策略，使内存与 rbac_policies 表同步（O(1)，无需全量重载）
	// 策略不存在时返回 (false, nil)，与原有 Delete 0 行视为成功的幂等语义一致
	if _, err := middlewares.GetRbacPolicy(c).RemovePolicy(name, in.Target, in.Action, in.Effect); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 返回操作成功的响应
	response.SimpleOK.AbortWithGinW(c, nil)
}
