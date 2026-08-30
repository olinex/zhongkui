package router

import (
	"net/http"

	"zhongkui.com/zhongkui/internal/manager/views"
)

// registerNotificationRoutes 注册 /api/v1/notification 通知管理接口路由。
// 通知分类下的渠道 / 目标 / 组 / 发送记录管理，与标签事件同级的独立分类。
// 认证 / RBAC / 核心库 / 命名空间等公共中间件挂在父组上继承。
func registerNotificationRoutes(ctx *RegisterContext) {
	notificationGroup := ctx.v1Group.Group(
		"/notification",                 // 通知接口路径前缀
		ctx.authenticatorMiddleware,     // 认证器注入
		ctx.forceAuthenticateMiddleware, // 强制JWT认证
		ctx.policyMiddleware,            // RBAC策略引擎
		ctx.coreDatabaseMiddleware,      // 核心库（GORM）连接
		ctx.namespaceMiddleware,         // 命名空间隔离
	)

	// ========== 通知渠道：/api/v1/notification/channel ==========
	// 通知渠道管理（渠道类型枚举、增删改查、启停与批量删除）。
	{
		channelGroup := notificationGroup.Group("/channel")
		// 通知渠道类型枚举 - 返回已注册类型及其配置 schema
		channelGroup.POST("/types", views.NotificationChannelTypesGet)
		// 通知渠道类型枚举元信息
		channelGroup.OPTIONS("/types", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.NotificationChannelTypesOut]())))
		// 通知渠道列表查询 - 支持分页、搜索和排序
		channelGroup.POST("/list", views.NotificationChannelListGet)
		// 通知渠道列表自省信息
		channelGroup.OPTIONS("/list", views.ListMetaGinHandler[*views.NotificationChannelListGetSearchIn, *views.NotificationChannelListOut]())
		// 创建通知渠道
		channelGroup.POST("/create", views.NotificationChannelCreate)
		// 创建通知渠道元信息
		channelGroup.OPTIONS("/create", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.NotificationChannelCreateIn]())))
		// 通知渠道详情查询
		channelGroup.POST("/:id/get", views.NotificationChannelDetailGet)
		// 通知渠道详情元信息
		channelGroup.OPTIONS("/:id/get", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.NotificationChannelDetailOut]())))
		// 通知渠道信息更新 - 更新启用状态/配置/描述（名称与类型不可变）
		channelGroup.POST("/:id/update", views.NotificationChannelUpdate)
		// 通知渠道信息更新元信息
		channelGroup.OPTIONS("/:id/update", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.NotificationChannelUpdateIn]())))
		// 删除通知渠道 - 批量删除
		channelGroup.POST("/delete", views.NotificationChannelBatchDelete)
		// 删除通知渠道元信息
		channelGroup.OPTIONS("/delete", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.NotificationChannelBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 启用通知渠道 - 批量启用
		channelGroup.POST("/enable", views.NotificationChannelBatchSetEnabled(true))
		// 启用通知渠道元信息
		channelGroup.OPTIONS("/enable", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.NotificationChannelBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 停用通知渠道 - 批量停用
		channelGroup.POST("/disable", views.NotificationChannelBatchSetEnabled(false))
		// 停用通知渠道元信息
		channelGroup.OPTIONS("/disable", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.NotificationChannelBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
	}

	// ========== 通知目标：/api/v1/notification/target ==========
	// 通知目标管理（增删改查、启停、批量删除与测试发送）。
	{
		targetGroup := notificationGroup.Group("/target")
		// 通知目标列表查询 - 支持分页、搜索和排序（含所属渠道信息）
		targetGroup.POST("/list", views.NotificationTargetListGet)
		targetGroup.OPTIONS("/list", views.ListMetaGinHandler[*views.NotificationTargetListGetSearchIn, *views.NotificationTargetListOut]())
		// 创建通知目标 - 类型由所选渠道派生
		targetGroup.POST("/create", views.NotificationTargetCreate)
		targetGroup.OPTIONS("/create", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.NotificationTargetCreateIn]())))
		// 通知目标详情查询
		targetGroup.POST("/:id/get", views.NotificationTargetDetailGet)
		targetGroup.OPTIONS("/:id/get", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.NotificationTargetDetailOut]())))
		// 通知目标信息更新 - 更新启用状态/配置/语言/描述（名称与渠道不可变）
		targetGroup.POST("/:id/update", views.NotificationTargetUpdate)
		targetGroup.OPTIONS("/:id/update", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.NotificationTargetUpdateIn]())))
		// 通知目标测试发送 - 按目标偏好语言渲染测试消息并实际投递
		targetGroup.POST("/:id/test-send", views.NotificationTargetTestSend)
		targetGroup.OPTIONS("/:id/test-send", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.NotificationTargetTestSendOut]())))
		// 向单个通知目标发送通知（投递 notify:send:target 异步任务，复用通知全链路）
		targetGroup.POST("/:id/send", ctx.pipelineClientMiddleware, views.NotificationTargetSend)
		targetGroup.OPTIONS("/:id/send", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.NotificationTargetSendIn](), views.WithBodyOutput[views.NotificationTargetSendOut]())))
		// 删除通知目标 - 批量删除
		targetGroup.POST("/delete", views.NotificationTargetBatchDelete)
		targetGroup.OPTIONS("/delete", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.NotificationTargetBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 启用通知目标 - 批量启用
		targetGroup.POST("/enable", views.NotificationTargetBatchSetEnabled(true))
		targetGroup.OPTIONS("/enable", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.NotificationTargetBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 停用通知目标 - 批量停用
		targetGroup.POST("/disable", views.NotificationTargetBatchSetEnabled(false))
		targetGroup.OPTIONS("/disable", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.NotificationTargetBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
	}

	// ========== 通知组：/api/v1/notification/group ==========
	// 通知目标组管理（增删改查、批量删除、默认组设置与目标集合替换）。
	{
		groupGroup := notificationGroup.Group("/group")
		// 通知组列表查询 - 支持分页、搜索和排序（含目标计数）
		groupGroup.POST("/list", views.NotificationGroupListGet)
		groupGroup.OPTIONS("/list", views.ListMetaGinHandler[*views.NotificationGroupListGetSearchIn, *views.NotificationGroupListOut]())
		// 创建通知组 - 可标记为命名空间默认组
		groupGroup.POST("/create", views.NotificationGroupCreate)
		groupGroup.OPTIONS("/create", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.NotificationGroupCreateIn]())))
		// 通知组详情查询 - 含已绑定目标列表
		groupGroup.POST("/:id/get", views.NotificationGroupDetailGet)
		groupGroup.OPTIONS("/:id/get", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.NotificationGroupDetailOut]())))
		// 通知组信息更新 - 仅更新描述
		groupGroup.POST("/:id/update", views.NotificationGroupUpdate)
		groupGroup.OPTIONS("/:id/update", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.NotificationGroupUpdateIn]())))
		// 设置默认通知组
		groupGroup.POST("/:id/set-default", views.NotificationGroupSetDefault)
		groupGroup.OPTIONS("/:id/set-default", views.StructMetaGinHandler(views.WithMethod(http.MethodPost)))
		// 删除通知组 - 批量删除
		groupGroup.POST("/delete", views.NotificationGroupBatchDelete)
		groupGroup.OPTIONS("/delete", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.NotificationGroupBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
	}

	// ========== 通知模板：/api/v1/notification/template ==========
	// 通知模板管理（事件枚举、列表、创建/更新、恢复默认与渲染预览）。
	{
		templateGroup := notificationGroup.Group("/template")
		// 通知模板事件枚举 - 返回各事件支持渠道/语言与默认标题/正文
		templateGroup.POST("/events", views.NotificationTemplateEventsGet)
		// 通知模板事件枚举元信息
		templateGroup.OPTIONS("/events", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.NotificationTemplateEventsOut]())))
		// 通知模板列表查询 - 支持事件/来源/语言/渠道类型过滤与分页
		templateGroup.POST("/list", views.NotificationTemplateListGet)
		// 通知模板列表元信息
		templateGroup.OPTIONS("/list", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.NotificationTemplateListGetSearchIn](), views.WithBodyOutput[views.NotificationTemplateListOut]()),
		))
		// 创建通知模板（用户覆盖模板，source=override）
		templateGroup.POST("/create", views.NotificationTemplateCreate)
		// 创建通知模板元信息
		templateGroup.OPTIONS("/create", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.NotificationTemplateCreateIn]())))
		// 更新通知模板（标题/正文/描述，强制 source=override）
		templateGroup.POST("/update", views.NotificationTemplateUpdate)
		// 更新通知模板元信息
		templateGroup.OPTIONS("/update", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.NotificationTemplateUpdateIn]())))
		// 恢复事件模板为内置默认（整事件重置）
		templateGroup.POST("/restore-default", views.NotificationTemplateRestoreDefault)
		// 恢复默认元信息
		templateGroup.OPTIONS("/restore-default", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.NotificationTemplateRestoreDefaultIn]())))
		// 模板渲染预览（render-test）
		templateGroup.POST("/render-test", views.NotificationTemplateRenderTest)
		// 模板渲染预览元信息
		templateGroup.OPTIONS("/render-test", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.NotificationTemplateRenderTestIn](), views.WithBodyOutput[views.NotificationTemplateRenderTestOut]()),
		))
		// 模板语法校验（校验编辑器内尚未保存的标题/正文）
		templateGroup.POST("/validate", views.NotificationTemplateValidate)
		templateGroup.OPTIONS("/validate", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.NotificationTemplateValidateIn](), views.WithBodyOutput[views.NotificationTemplateValidateOut]()),
		))
		// 模板试运行（使用事件内置样例参数渲染编辑器内尚未保存的标题/正文）
		templateGroup.POST("/dry-run", views.NotificationTemplateDryRun)
		templateGroup.OPTIONS("/dry-run", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.NotificationTemplateDryRunIn](), views.WithBodyOutput[views.NotificationTemplateDryRunOut]()),
		))
	}

	// ========== 通知发送记录：/api/v1/notification/record ==========
	// 通知发送记录管理（只读：列表与详情）。
	{
		recordGroup := notificationGroup.Group("/record")
		// 通知记录列表查询 - 支持分页、搜索和排序（状态/事件/渠道/时间过滤）
		recordGroup.POST("/list", views.NotificationRecordListGet)
		recordGroup.OPTIONS("/list", views.ListMetaGinHandler[*views.NotificationRecordListGetSearchIn, *views.NotificationRecordListOut]())
		// 通知记录详情查询 - 含 payload/error/messageId/notifiedAt
		recordGroup.POST("/:id/get", views.NotificationRecordDetailGet)
		recordGroup.OPTIONS("/:id/get", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.NotificationRecordDetailOut]())))
	}
}
