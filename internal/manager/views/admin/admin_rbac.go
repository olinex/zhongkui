package admin

import (
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/manager/views"
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/utils"
)

// GroupRbacPolicyDetailOut 表示组RBAC策略详情输出结构
// 用于返回组策略的详细信息，继承BaseItemOut基础字段
type GroupRbacPolicyDetailOut struct {
	utils.BaseItemOut
	V1 string `json:"target"` // 目标对象名，表示被访问的资源
	V2 string `json:"action"` // 操作名，表示对资源的操作类型
	V3 string `json:"effect"` // 效果，只能是'allow'(允许)或'deny'(拒绝)
}

// UserRbacPolicyDetailOut 表示用户RBAC策略详情输出结构
// 用于返回用户策略的详细信息，继承BaseItemOut基础字段
type UserRbacPolicyDetailOut struct {
	utils.BaseItemOut
	V1 string `json:"name"`      // 用户名或用户标识
	V2 string `json:"namespace"` // 命名空间，用于隔离不同环境或租户的策略
}

// RbacPolicyTemplateDetailOut 表示RBAC策略模板详情输出结构
// 用于返回策略模板的详细信息，继承BaseItemOut基础字段
type RbacPolicyTemplateDetailOut struct {
	utils.BaseItemOut
	Target string `json:"target"` // 目标对象名，表示被访问的资源
	Action string `json:"action"` // 操作名，表示对资源的操作类型
}

// RbacPolicyTemplateListOut 表示RBAC策略模板列表输出结构
// 用于返回策略模板列表，包含详情和分页信息
type RbacPolicyTemplateListOut struct {
	RbacPolicyTemplateDetailOut
	utils.BaseTotalOut
}

// RbacPolicyTemplateListGet 获取RBAC策略模板列表
// 这是一个HTTP GET处理器，用于查询RBAC策略模板的列表
//
// 参数:
//   - c: Gin上下文，包含HTTP请求信息和响应方法
//
// 功能:
//  1. 接收并验证查询参数
//  2. 构建数据库查询
//  3. 执行查询并返回结果
//  4. 处理各种错误情况
//
// 返回值: 无，通过Gin上下文返回HTTP响应
func RbacPolicyTemplateListGet(c *gin.Context) {
	var in views.RbacPolicyTemplateListGetSearchIn
	// 设置默认值
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 绑定JSON请求体
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	var items []*RbacPolicyTemplateListOut
	// 获取数据库实例并指定模型
	db := middlewares.GetCoreDatabase(c).Model(&core.RbacPolicyTemplate{})
	// 构建搜索查询
	if query, err := utils.BuildSearchCountQuery(db, &in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&items).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 字段投影：按请求 fields 投影，未指定时回退全部白名单字段（封装于 utils.ProjectSearchItems）
	var total uint
	if len(items) > 0 {
		total = items[0].GetTotal()
	}
	projected := utils.ProjectSearchItems(items, &in, func(o **RbacPolicyTemplateListOut) any {
		return *o
	})
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(total, projected))
}
