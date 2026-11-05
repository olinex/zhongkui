// Package datasource 提供第三方数据源（Redis / MySQL）通用管理接口的路由注册与处理器。
package datasource

import (
	"net/http"

	"zhongkuisec.com/zhongkui/internal/ai/capability"
	"zhongkuisec.com/zhongkui/internal/manager/views"
)

// RegisterRoutes 注册 /api/v1/datasource 第三方数据源通用管理接口。
// 第三方数据源（Redis / MySQL）把外部数据暴露为按命名空间隔离的 SQLite vtable，属于通用
// 能力（当前由标签事件消费）。数据源归属请求命名空间，注册为所在命名空间池（vtab.Pools）
// 内的裸名 vtable，供 rego 规则经 sqlite.query 直接引用。
// 认证 / RBAC / 核心库 / 命名空间等公共中间件挂在父组上继承；需 SQLiteVTabPools 中间件
// 提供按命名空间池，供 CRUD 处理器注册/注销 vtable。
func RegisterRoutes(ctx *views.RegisterContext) {
	dsGroup := ctx.V1Group.Group(
		"/datasource",         // 第三方数据源接口路径前缀
		ctx.Authenticator,     // 认证器注入
		ctx.ForceAuthenticate, // 强制JWT认证
		ctx.Policy,            // RBAC策略引擎
		ctx.Namespace,         // 命名空间隔离
		ctx.Authorize,         // 权限验证（api:{FullPath} + HTTP 方法动态授权）
		ctx.CoreDatabase,      // 核心库（GORM）连接
		ctx.Etcd,              // etcd 客户端（写路径 broadcastDatasourceChange 热同步）
		ctx.SQLiteVTabPools,   // SQLite vtable 池（数据源 CRUD 注册/注销裸名 vtable）
	)

	// ========== 第三方数据源：/api/v1/datasource ==========
	// 第三方数据源（Redis / MySQL）动态配置管理（增删改查、启停）。
	// 每个数据源注册为所在命名空间池的裸名 vtable {name}，供 rego 规则经 sqlite.query
	// 直接引用；数据按命名空间隔离（每命名空间独立 SQLitePool，连接池 Key 带命名空间维度）。
	{
		// 第三方数据源类型枚举 - 返回 redis / mysql 类型及各自配置字段 schema，供前端动态渲染
		dsGroup.POST("/types", DatasourceTypeConfigGet)
		// 第三方数据源类型枚举元信息
		views.RegisterApi(dsGroup, "/types", "第三方数据源类型枚举 - 返回 redis / mysql 类型及各自配置字段 schema，供前端动态渲染",
			views.WithMethod(http.MethodPost, views.WithBodyOutput[DatasourceTypesOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "datasource.types",
				Domain:    "datasource",
				Intent:    "查看可用数据源类型及其配置字段",
				Entities:  []string{},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 第三方数据源列表查询 - 支持分页、搜索和排序
		dsGroup.POST("/list", DatasourceListGet)
		// 第三方数据源列表自省信息 - 返回搜索选项、表格列与出参结构体定义
		views.RegisterListApi[*views.DatasourceListGetSearchIn, *DatasourceListOut](dsGroup, "/list", "第三方数据源列表查询 - 支持分页、搜索和排序",
			views.WithCapability(views.CapabilityMeta{
				ID:        "datasource.list",
				Domain:    "datasource",
				Intent:    "查看已接入的第三方数据源及其启用状态",
				Entities:  []string{"datasources"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 创建第三方数据源 - 注册 vtable 校验配置后落库（归属当前请求命名空间）
		dsGroup.POST("/create", DatasourceCreate)
		// 创建第三方数据源元信息
		views.RegisterApi(dsGroup, "/create", "创建第三方数据源 - 注册 vtable 校验配置后落库（归属当前请求命名空间）", views.WithMethod(http.MethodPost, views.WithBodyInput[DatasourceCreateIn]()))
		// 测试第三方数据源连接 - 预检连通性，不落库不注册 vtable，失败原因放 body 返回
		dsGroup.POST("/test", DatasourceTest)
		// 测试第三方数据源连接元信息
		views.RegisterApi(dsGroup, "/test", "测试第三方数据源连接 - 预检连通性，不落库不注册 vtable，失败原因放 body 返回",
			views.WithMethod(http.MethodPost, views.WithBodyInput[DatasourceTestIn](), views.WithBodyOutput[DatasourceTestOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "datasource.test",
				Domain:    "datasource",
				Intent:    "测试第三方数据源连通性（不落库）",
				Entities:  []string{},
				Access:    capability.AccessRead,
				Invokable: false,
			}),
		)
		// 自省第三方数据源表结构 - 预检列定义，不落库不注册 vtable；sqlite_file 上传 .db 文件
		dsGroup.POST("/introspect", DatasourceIntrospect)
		// 自省第三方数据源表结构元信息
		views.RegisterApi(dsGroup, "/introspect", "自省第三方数据源表结构 - 预检列定义，不落库不注册 vtable；sqlite_file 上传 .db 文件",
			views.WithMethod(http.MethodPost, views.WithBodyInput[DatasourceIntrospectIn](), views.WithBodyOutput[DatasourceIntrospectOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "datasource.introspect",
				Domain:    "datasource",
				Intent:    "预检第三方数据源表结构（不落库）",
				Entities:  []string{},
				Access:    capability.AccessRead,
				Invokable: false,
			}),
		)
		// 第三方数据源详情查询 - 根据ID获取数据源完整信息
		dsGroup.POST("/:id/get", DatasourceDetailGet)
		// 第三方数据源详情元信息
		views.RegisterApi(dsGroup, "/:id/get", "第三方数据源详情查询 - 根据ID获取数据源完整信息",
			views.WithMethod(http.MethodPost, views.WithBodyOutput[DatasourceDetailOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "datasource.detail",
				Domain:    "datasource",
				Intent:    "查看单个第三方数据源的完整配置",
				Entities:  []string{"datasources"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 更新第三方数据源 - 更新启用状态/配置/描述（名称不可变），覆盖注册相应 vtable
		dsGroup.POST("/:id/update", DatasourceUpdate)
		// 更新第三方数据源元信息
		views.RegisterApi(dsGroup, "/:id/update", "更新第三方数据源 - 更新启用状态/配置/描述（名称不可变），覆盖注册相应 vtable", views.WithMethod(http.MethodPost, views.WithBodyInput[DatasourceUpdateIn]()))
		// 删除第三方数据源 - 批量删除并注销相应 vtable
		dsGroup.POST("/delete", DatasourceBatchDelete)
		// 删除第三方数据源元信息
		views.RegisterApi(dsGroup, "/delete", "删除第三方数据源 - 批量删除并注销相应 vtable",
			views.WithMethod(http.MethodPost, views.WithBodyInput[DatasourceBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		)
		// 启用第三方数据源 - 批量启用并注册相应 vtable
		dsGroup.POST("/enable", DatasourceBatchSetEnabled(true))
		// 启用第三方数据源元信息
		views.RegisterApi(dsGroup, "/enable", "启用第三方数据源 - 批量启用并注册相应 vtable",
			views.WithMethod(http.MethodPost, views.WithBodyInput[DatasourceBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		)
		// 停用第三方数据源 - 批量停用并注销相应 vtable
		dsGroup.POST("/disable", DatasourceBatchSetEnabled(false))
		// 停用第三方数据源元信息
		views.RegisterApi(dsGroup, "/disable", "停用第三方数据源 - 批量停用并注销相应 vtable",
			views.WithMethod(http.MethodPost, views.WithBodyInput[DatasourceBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		)
	}
}
