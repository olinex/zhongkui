// Package routes 提供HTTP路由注册和管理功能。
package views

import (
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	message "zhongkui.com/zhongkui/internal/pkg/message"
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

// NamespaceListGet 处理获取命名空间列表请求。
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
func NamespaceListGet(c *gin.Context) {
	var in NamespaceListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	var items []*NamespaceListOut
	db := middlewares.GetCoreDatabase(c).Model(&core.Namespace{})
	if query, err := utils.BuildSearchQuery(db, &in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&items).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	out := utils.NewSearchOut(items)
	response.SimpleOK.AbortWithGinW(c, out)
}

// NamespaceDetailGet 处理获取命名空间详情请求。
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
func NamespaceDetailGet(c *gin.Context) {
	name, ok := getRawParam(c, "name")
	if !ok {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	var out NamespaceDetailOut
	if err := db.Model(&core.Namespace{}).First(&out, "name = ?", name).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else {
		response.SimpleOK.AbortWithGinW(c, out)
	}
}

// NamespaceBatchDeleteIn 批量删除命名空间的请求输入结构。
// Names 接受标量（单个命名空间名）或数组（多个命名空间名）。
type NamespaceBatchDeleteIn struct {
	Names FlexList[string] `json:"names"`
}

// NamespaceBatchDelete 批量删除命名空间。
// 逐条复用原 namespaceDelete 的引用校验与缓存钩子语义：
//   - 被 RBAC 策略引用（ptype='g' v2=命名空间名）→ 该条记入 Failed；
//   - 命名空间不存在 → 视为成功跳过（幂等）；
//   - 其他数据库错误 → 该条记入 Failed。
//
// 删除前先查出记录再按主键删除，确保触发 Namespace.AfterDelete 钩子，
// 同步清理命名空间缓存（批量 Delete 不触发 GORM 钩子）。
//
// 参数:
//   - c *gin.Context: Gin框架的上下文对象，包含HTTP请求和响应信息
//
// 请求体:
//   - 使用NamespaceBatchDeleteIn结构体，Names字段接受标量或数组
//
// 响应:
//   - 成功: 返回BatchResultOut汇总（succeeded + failed明细）
func NamespaceBatchDelete(c *gin.Context) {
	var in NamespaceBatchDeleteIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	names := []string(in.Names)
	if !validFlexLen(c, names) {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	batchExecute(c, names,
		func(name string) string { return name },
		func(name string) error {
			return db.Transaction(func(tx *database.DB) error {
				// 以 FOR UPDATE 行锁锁定命名空间行，与 UserGroupJoin 的命名空间校验串行化，
				// 避免"校验无引用后，并发加入引用再删除命名空间"导致的悬挂引用（SQLite 退化为普通读）
				var ns core.Namespace
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&ns, "name = ?", name).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						// 命名空间不存在，视为成功跳过（幂等）
						return nil
					}
					return errors.WithStack(err)
				}

				// 检查是否存在引用该命名空间的RBAC策略（ptype='g'，v2为命名空间名）
				var policy core.RbacPolicy
				if err := tx.First(
					&policy,
					"(ptype = ? AND v2 = ?)",
					base.GroupLabel,
					name,
				).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						// 没有引用关系，可以删除
						return tx.Delete(&ns).Error
					}
					// 其他数据库错误作为失败原因
					return errors.WithStack(err)
				}
				// 找到引用关系，不允许删除
				return response.ErrCannotDeleteCauseRelateRecord
			})
		},
	)
}

// NamespaceCreate 处理创建命名空间请求。
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
func NamespaceCreate(c *gin.Context) {
	var in NamespaceCreateIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	db := middlewares.GetCoreDatabase(c)
	// 事务内先创建命名空间，成功后再为其种子化「占位符」通知渠道（如 webhook：渠道级
	// 配置无参数，仅标明目标类型）。占位渠道为固定数据，命名空间创建时自动入库，
	// 前端不可创建、不可删除；任一步失败整事务回滚，保持命名空间与占位渠道原子一致。
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&core.Namespace{
			Name:        in.Name,
			Description: in.Description,
		}).Error; err != nil {
			return errors.WithStack(err)
		}
		for _, typ := range message.PlaceholderTypes() {
			if err := tx.Create(&core.NotificationChannel{
				NamespaceName: in.Name,
				Name:          typ,
				Type:          typ,
				Enabled:       true,
				Description:   "builtin placeholder channel: marks the notification target type",
			}).Error; err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// NamespaceUpdate 处理更新命名空间请求。
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
func NamespaceUpdate(c *gin.Context) {
	name, ok := getRawParam(c, "name")
	if !ok {
		return
	}
	var in NamespaceUpdateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	db := middlewares.GetCoreDatabase(c)
	result := db.Model(&core.Namespace{}).Where("name = ?", name).Updates(core.Namespace{Description: in.Description})
	if result.Error != nil {
		response.CreateAccidentError(result.Error).AbortWithGinH(c)
		return
	}
	// 检查是否有行被更新
	if result.RowsAffected == 0 {
		// 没有找到指定的命名空间
		response.CreateAccidentError(gorm.ErrRecordNotFound).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}
