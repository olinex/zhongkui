package router

import (
	"net/http"

	"zhongkui.com/zhongkui/internal/manager/views"
)

// registerDatasourceRoutes 注册 /api/v1/datasource 第三方数据源通用管理接口。
// 第三方数据源（Redis / MySQL）把外部数据暴露为按命名空间隔离的 SQLite vtable，属于通用
// 能力（当前由标签事件消费）。数据源归属请求命名空间，注册为所在命名空间池（vtab.Pools）
// 内的裸名 vtable，供 rego 规则经 sqlite.query 直接引用。
// 认证 / RBAC / 核心库 / 命名空间等公共中间件挂在父组上继承；需 sqliteVTabPoolsMiddleware
// 提供按命名空间池，供 CRUD 处理器注册/注销 vtable。
func registerDatasourceRoutes(ctx *RegisterContext) {
	dsGroup := ctx.v1Group.Group(
		"/datasource",                   // 第三方数据源接口路径前缀
		ctx.authenticatorMiddleware,     // 认证器注入
		ctx.forceAuthenticateMiddleware, // 强制JWT认证
		ctx.policyMiddleware,            // RBAC策略引擎
		ctx.namespaceMiddleware,         // 命名空间隔离
		ctx.authorizeMiddleware,         // 权限验证（api:{FullPath} + HTTP 方法动态授权）
		ctx.coreDatabaseMiddleware,      // 核心库（GORM）连接
		ctx.etcdMiddleware,              // etcd 客户端（写路径 broadcastDatasourceChange 热同步）
		ctx.sqliteVTabPoolsMiddleware,   // SQLite vtable 池（数据源 CRUD 注册/注销裸名 vtable）
	)

	// ========== 第三方数据源：/api/v1/datasource ==========
	// 第三方数据源（Redis / MySQL）动态配置管理（增删改查、启停）。
	// 每个数据源注册为所在命名空间池的裸名 vtable {name}，供 rego 规则经 sqlite.query
	// 直接引用；数据按命名空间隔离（每命名空间独立 SQLitePool，连接池 Key 带命名空间维度）。
	{
		// 第三方数据源类型枚举 - 返回 redis / mysql 类型及各自配置字段 schema，供前端动态渲染
		dsGroup.POST("/types", views.DatasourceTypeConfigGet)
		// 第三方数据源类型枚举元信息
		dsGroup.OPTIONS("/types", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.DatasourceTypesOut]())))
		// 第三方数据源列表查询 - 支持分页、搜索和排序
		dsGroup.POST("/list", views.DatasourceListGet)
		// 第三方数据源列表自省信息 - 返回搜索选项、表格列与出参结构体定义
		dsGroup.OPTIONS("/list", views.ListMetaGinHandler[*views.DatasourceListGetSearchIn, *views.DatasourceListOut]())
		// 创建第三方数据源 - 注册 vtable 校验配置后落库（归属当前请求命名空间）
		dsGroup.POST("/create", views.DatasourceCreate)
		// 创建第三方数据源元信息
		dsGroup.OPTIONS("/create", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.DatasourceCreateIn]())))
		// 测试第三方数据源连接 - 预检连通性，不落库不注册 vtable，失败原因放 body 返回
		dsGroup.POST("/test", views.DatasourceTest)
		// 测试第三方数据源连接元信息
		dsGroup.OPTIONS("/test", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.DatasourceTestIn](), views.WithBodyOutput[views.DatasourceTestOut]())))
		// 自省第三方数据源表结构 - 预检列定义，不落库不注册 vtable；sqlite_file 上传 .db 文件
		dsGroup.POST("/introspect", views.DatasourceIntrospect)
		// 自省第三方数据源表结构元信息
		dsGroup.OPTIONS("/introspect", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.DatasourceIntrospectIn](), views.WithBodyOutput[views.DatasourceIntrospectOut]())))
		// 第三方数据源详情查询 - 根据ID获取数据源完整信息
		dsGroup.POST("/:id/get", views.DatasourceDetailGet)
		// 第三方数据源详情元信息
		dsGroup.OPTIONS("/:id/get", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.DatasourceDetailOut]())))
		// 更新第三方数据源 - 更新启用状态/配置/描述（名称不可变），覆盖注册相应 vtable
		dsGroup.POST("/:id/update", views.DatasourceUpdate)
		// 更新第三方数据源元信息
		dsGroup.OPTIONS("/:id/update", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.DatasourceUpdateIn]())))
		// 删除第三方数据源 - 批量删除并注销相应 vtable
		dsGroup.POST("/delete", views.DatasourceBatchDelete)
		// 删除第三方数据源元信息
		dsGroup.OPTIONS("/delete", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.DatasourceBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 启用第三方数据源 - 批量启用并注册相应 vtable
		dsGroup.POST("/enable", views.DatasourceBatchSetEnabled(true))
		// 启用第三方数据源元信息
		dsGroup.OPTIONS("/enable", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.DatasourceBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 停用第三方数据源 - 批量停用并注销相应 vtable
		dsGroup.POST("/disable", views.DatasourceBatchSetEnabled(false))
		// 停用第三方数据源元信息
		dsGroup.OPTIONS("/disable", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.DatasourceBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
	}
}
