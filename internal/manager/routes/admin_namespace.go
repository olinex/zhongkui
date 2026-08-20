// Package routes 提供HTTP路由注册和管理功能。
package routes

import (
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// NamespaceUpdateIn 定义命名空间更新输入结构体。
//
// 该结构体用于接收更新命名空间的请求参数，包含以下字段：
//   - Description: 命名空间描述，必填，长度限制为3-256个字符
//
// 字段说明：
//   - json:"description": JSON序列化时使用"description"作为字段名
//   - binding:"required,min=3,max=256": Gin绑定验证规则，要求必填且长度在3-256字符之间
type NamespaceUpdateIn struct {
	Description string `json:"description" binding:"required,min=3,max=256"`
}

// NamespaceCreateIn 定义命名空间创建输入结构体。
//
// 该结构体用于接收创建命名空间的请求参数，包含以下字段：
//   - NamespaceUpdateIn: 嵌入的命名空间更新结构体，包含Description字段
//   - Name: 命名空间名称，必填，长度限制为3-32个字符，且全局唯一
//
// 字段说明：
//   - json:"name": JSON序列化时使用"name"作为字段名
//   - binding:"required,min=3,max=32": Gin绑定验证规则，要求必填且长度在3-32字符之间
//
// 注意：命名空间名称一旦创建不可修改，具有唯一性约束。
// 名称限制为 Unicode 字母数字与 _-（alphanumspecial），
// 保证名称可作为 URL 路径参数安全传递（如 /admin/namespace/:name/delete）。
type NamespaceCreateIn struct {
	NamespaceUpdateIn
	Name string `json:"name" binding:"required,min=3,max=32,alphanumspecial=_-"`
}

// NamespaceDetailOut 定义命名空间详情输出结构体。
//
// 该结构体用于返回单个命名空间的详细信息，包含以下字段：
//   - BaseItemOut: 嵌入的基础项输出结构体，包含ID、CreatedAt、UpdatedAt等通用字段
//   - Name: 命名空间名称，字符串类型
//   - Description: 命名空间描述，字符串类型
//
// 字段说明：
//   - json:"name": JSON序列化时使用"name"作为字段名
//   - json:"description": JSON序列化时使用"description"作为字段名
//
// 该结构体在查询单个命名空间详情时使用。
type NamespaceDetailOut struct {
	utils.BaseItemOut
	Name        string `json:"name"`
	Description string `json:"description"`
}

// NamespaceListOut 定义命名空间列表输出结构体。
//
// 该结构体用于返回命名空间列表查询结果，包含以下字段：
//   - NamespaceDetailOut: 嵌入的命名空间详情结构体，包含Name、Description等字段
//   - BaseTotalOut: 嵌入的基础总数输出结构体，包含分页相关的Total等字段
//
// 该结构体在分页查询命名空间列表时使用，支持搜索、过滤、排序等功能。
// 注意：由于嵌入NamespaceDetailOut，因此也包含ID、CreatedAt、UpdatedAt等基础字段。
type NamespaceListOut struct {
	NamespaceDetailOut
	utils.BaseTotalOut
}

// namespaceListGet 处理获取命名空间列表请求。
//
// 该函数提供分页查询命名空间列表的功能，支持以下特性：
//   - 分页查询：通过page和pageSize参数控制分页
//   - 排序功能：支持按createdAt和updatedAt字段排序
//   - 过滤功能：支持按名称、创建时间、更新时间等字段过滤
//   - 搜索功能：支持前缀匹配、后缀匹配、包含匹配等多种搜索方式
//
// 请求流程：
//  1. 接收并验证请求参数，包括分页、排序、过滤条件
//  2. 从Gin上下文中获取数据库连接
//  3. 构建搜索查询，应用过滤和排序条件
//  4. 执行查询并返回结果
//
// 参数：
//   - c: Gin请求上下文，包含请求参数和数据库连接等信息
//
// 错误处理：
//   - 参数绑定失败：返回BadBodyError错误（HTTP 400）
//   - 数据库查询错误：返回AccidentError错误（HTTP 500）
//   - 搜索查询构建错误：返回AccidentError错误（HTTP 500）
//
// 成功响应：
//   - HTTP 200状态码
//   - 包含分页信息的命名空间列表数据
func namespaceListGet(c *gin.Context) {
	var in NamespaceListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	var items []*NamespaceListOut
	db := manager.GetCoreDatabase(c).Model(&core.Namespace{})
	if query, err := utils.BuildSearchQuery(db, &in); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&items).Error; err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	out := utils.NewSearchOut(items)
	manager.SimpleOK.AbortWithGinW(c, out)
}

// namespaceDetailGet 处理获取命名空间详情请求。
//
// 该函数根据命名空间名称查询单个命名空间的详细信息。
//
// 请求流程：
//  1. 从URL路径参数中获取命名空间名称
//  2. 验证名称参数是否有效
//  3. 从Gin上下文中获取数据库连接
//  4. 根据名称查询命名空间详情
//  5. 返回查询结果或相应的错误信息
//
// 参数：
//   - c: Gin请求上下文，包含请求参数和数据库连接等信息
//
// URL路径参数：
//   - name: 命名空间名称，作为URL路径参数传递
//
// 错误处理：
//   - 名称参数缺失：由getRawParam函数处理，返回相应的错误响应
//   - 数据库查询错误：返回AccidentError错误（HTTP 500）
//   - 记录不存在：返回AccidentError错误（HTTP 500），包含gorm.ErrRecordNotFound
//
// 成功响应：
//   - HTTP 200状态码
//   - 包含命名空间详情的JSON数据
func namespaceDetailGet(c *gin.Context) {
	name, ok := getRawParam(c, "name")
	if !ok {
		return
	}
	db := manager.GetCoreDatabase(c)
	var out NamespaceDetailOut
	if err := db.Model(&core.Namespace{}).First(&out, "name = ?", name).Error; err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else {
		manager.SimpleOK.AbortWithGinW(c, out)
	}
}

// namespaceDelete 处理删除命名空间请求。
//
// 该函数删除指定的命名空间，但在删除前会进行安全检查：
//  1. 检查该命名空间是否被RBAC策略引用
//  2. 如果存在引用关系，则拒绝删除并返回错误
//  3. 如果不存在引用关系，则执行删除操作
//
// 注意：由于命名空间在多租户架构中的重要性，删除操作需要特别谨慎。
// 命名空间一旦删除，相关的权限策略、用户组等关联数据将无法正常工作。
//
// 请求流程：
//  1. 从URL路径参数中获取命名空间名称
//  2. 验证名称参数是否有效
//  3. 在数据库事务中执行以下操作：
//     a. 检查该命名空间是否被RBAC策略引用（ptype为group_label，v2为命名空间名称）
//     b. 如果存在引用，返回CannotDeleteCauseRelateRecord错误
//     c. 如果不存在引用，执行删除操作
//  4. 提交事务或回滚事务
//
// 参数：
//   - c: Gin请求上下文，包含请求参数和数据库连接等信息
//
// URL路径参数：
//   - name: 命名空间名称，作为URL路径参数传递
//
// 错误处理：
//   - 名称参数缺失：由getRawParam函数处理，返回相应的错误响应
//   - 存在引用关系：返回CannotDeleteCauseRelateRecord错误（HTTP 400）
//   - 数据库查询错误：返回AccidentError错误（HTTP 500）
//   - 删除操作错误：返回AccidentError错误（HTTP 500）
//
// 成功响应：
//   - HTTP 200状态码
//   - 简单的成功响应，不包含数据体
func namespaceDelete(c *gin.Context) {
	name, ok := getRawParam(c, "name")
	if !ok {
		return
	}
	db := manager.GetCoreDatabase(c)
	db.Transaction(func(tx *database.DB) error {
		// 检查是否存在引用该命名空间的RBAC策略
		var policy core.RbacPolicy
		if err := tx.First(
			&policy,
			"(ptype = ? AND v2 = ?)",
			base.GroupLabel,
			name,
		).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// 没有引用关系，可以删除
				// 先查出记录再按主键删除，确保触发 Namespace.AfterDelete 钩子，
				// 同步清理命名空间缓存（批量 Delete 不触发 GORM 钩子）
				var ns core.Namespace
				if err := tx.First(&ns, "name = ?", name).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						// 没有找到指定的命名空间
						manager.CreateAccidentError(gorm.ErrRecordNotFound).AbortWithGinH(c)
						return errors.WithStack(gorm.ErrRecordNotFound)
					}
					manager.CreateAccidentError(err).AbortWithGinH(c)
					return errors.WithStack(err)
				}
				if err := tx.Delete(&ns).Error; err != nil {
					manager.CreateAccidentError(err).AbortWithGinH(c)
					return errors.WithStack(err)
				}
				manager.SimpleOK.AbortWithGinH(c)
				return nil
			} else {
				// 其他数据库错误
				manager.CreateAccidentError(err).AbortWithGinH(c)
				return errors.WithStack(err)
			}
		} else {
			// 找到引用关系，不允许删除
			manager.ErrCannotDeleteCauseRelateRecord.AbortWithGinH(c)
			return errors.WithStack(gorm.ErrRecordNotFound) // 返回一个错误以回滚事务
		}
	})
}

// namespaceCreate 处理创建命名空间请求。
//
// 该函数创建新的命名空间，命名空间名称必须全局唯一。
// 命名空间在多租户架构中用于隔离不同租户的数据和权限。
//
// 请求流程：
//  1. 接收并验证请求参数，包括名称和描述
//  2. 设置参数的默认值（如果适用）
//  3. 从Gin上下文中获取数据库连接
//  4. 创建新的命名空间记录
//  5. 返回创建结果或相应的错误信息
//
// 参数：
//   - c: Gin请求上下文，包含请求参数和数据库连接等信息
//
// 请求体参数：
//   - name: 命名空间名称，必填，长度3-32字符，全局唯一
//   - description: 命名空间描述，必填，长度3-256字符
//
// 验证规则：
//   - 名称验证：required,min=3,max=32
//   - 描述验证：required,min=3,max=256
//
// 错误处理：
//   - 参数默认值设置失败：返回AccidentError错误（HTTP 500）
//   - 参数绑定失败：返回BadBodyError错误（HTTP 400）
//   - 数据库创建错误：返回AccidentError错误（HTTP 500）
//   - 唯一性约束冲突：名称已存在时返回数据库错误
//
// 成功响应：
//   - HTTP 200状态码
//   - 简单的成功响应，不包含数据体
func namespaceCreate(c *gin.Context) {
	var in NamespaceCreateIn
	if err := defaults.Set(&in); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	db := manager.GetCoreDatabase(c)
	if err := db.Create(&core.Namespace{
		Name:        in.Name,
		Description: in.Description,
	}).Error; err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	manager.SimpleOK.AbortWithGinH(c)
}

// namespaceUpdate 处理更新命名空间请求。
//
// 该函数更新指定命名空间的描述信息。
// 注意：命名空间名称不可修改，只能更新描述字段。
//
// 请求流程：
//  1. 从URL路径参数中获取命名空间名称
//  2. 验证名称参数是否有效
//  3. 接收并验证请求体中的描述参数
//  4. 从Gin上下文中获取数据库连接
//  5. 根据名称更新命名空间的描述字段
//  6. 返回更新结果或相应的错误信息
//
// 参数：
//   - c: Gin请求上下文，包含请求参数和数据库连接等信息
//
// URL路径参数：
//   - name: 命名空间名称，作为URL路径参数传递，用于定位要更新的命名空间
//
// 请求体参数：
//   - description: 命名空间描述，必填，长度3-256字符
//
// 验证规则：
//   - 描述验证：required,min=3,max=256
//
// 错误处理：
//   - 名称参数缺失：由getRawParam函数处理，返回相应的错误响应
//   - 参数绑定失败：返回BadBodyError错误（HTTP 400）
//   - 数据库更新错误：返回AccidentError错误（HTTP 500）
//   - 记录不存在：更新操作会返回错误
//
// 成功响应：
//   - HTTP 200状态码
//   - 简单的成功响应，不包含数据体
func namespaceUpdate(c *gin.Context) {
	name, ok := getRawParam(c, "name")
	if !ok {
		return
	}
	var in NamespaceUpdateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	db := manager.GetCoreDatabase(c)
	result := db.Model(&core.Namespace{}).Where("name = ?", name).Updates(core.Namespace{Description: in.Description})
	if result.Error != nil {
		manager.CreateAccidentError(result.Error).AbortWithGinH(c)
		return
	}
	// 检查是否有行被更新
	if result.RowsAffected == 0 {
		// 没有找到指定的命名空间
		manager.CreateAccidentError(gorm.ErrRecordNotFound).AbortWithGinH(c)
		return
	}
	manager.SimpleOK.AbortWithGinH(c)
}
