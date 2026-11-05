package notification

import (
	"net/http"

	"zhongkuisec.com/zhongkui/internal/ai/capability"
	"zhongkuisec.com/zhongkui/internal/manager/views"
)

// RegisterRoutes 注册 /api/v1/notification 通知管理接口路由。
// 通知分类下的渠道 / 目标 / 组 / 发送记录管理，与标签事件同级的独立分类。
// 认证 / RBAC / 核心库 / 命名空间等公共中间件挂在父组上继承。
func RegisterRoutes(ctx *views.RegisterContext) {
	notificationGroup := ctx.V1Group.Group(
		"/notification",       // 通知接口路径前缀
		ctx.Authenticator,     // 认证器注入
		ctx.ForceAuthenticate, // 强制JWT认证
		ctx.Policy,            // RBAC策略引擎
		ctx.Namespace,         // 命名空间隔离
		ctx.Authorize,         // 权限验证（api:{FullPath} + HTTP 方法动态授权）
		ctx.CoreDatabase,      // 核心库（GORM）连接
	)

	// ========== 通知渠道：/api/v1/notification/channel ==========
	// 通知渠道管理（渠道类型枚举、增删改查、启停与批量删除）。
	{
		channelGroup := notificationGroup.Group("/channel")
		// 通知渠道类型枚举 - 返回已注册类型及其配置 schema
		channelGroup.POST("/types", NotificationChannelTypesGet)
		// 通知渠道类型枚举元信息
		views.RegisterApi(channelGroup, "/types", "通知渠道类型枚举 - 返回已注册类型及其配置 schema",
			views.WithMethod(http.MethodPost, views.WithBodyOutput[NotificationChannelTypesOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "notification.channel.types",
				Domain:    "notification",
				Intent:    "查看通知渠道类型及其配置字段",
				Entities:  []string{},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 通知渠道列表查询 - 支持分页、搜索和排序
		channelGroup.POST("/list", NotificationChannelListGet)
		// 通知渠道列表自省信息
		views.RegisterListApi[*views.NotificationChannelListGetSearchIn, *NotificationChannelListOut](channelGroup, "/list", "通知渠道列表查询 - 支持分页、搜索和排序",
			views.WithCapability(views.CapabilityMeta{
				ID:        "notification.channel.list",
				Domain:    "notification",
				Intent:    "查看通知渠道列表",
				Entities:  []string{"notification_channels"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 创建通知渠道
		channelGroup.POST("/create", NotificationChannelCreate)
		// 创建通知渠道元信息
		views.RegisterApi(channelGroup, "/create", "创建通知渠道", views.WithMethod(http.MethodPost, views.WithBodyInput[NotificationChannelCreateIn]()))
		// 通知渠道详情查询
		channelGroup.POST("/:id/get", NotificationChannelDetailGet)
		// 通知渠道详情元信息
		views.RegisterApi(channelGroup, "/:id/get", "通知渠道详情查询",
			views.WithMethod(http.MethodPost, views.WithBodyOutput[NotificationChannelDetailOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "notification.channel.detail",
				Domain:    "notification",
				Intent:    "查看通知渠道详情",
				Entities:  []string{"notification_channels"},
				Access:    capability.AccessRead,
				Invokable: false,
			}),
		)
		// 通知渠道信息更新 - 更新启用状态/配置/描述（名称与类型不可变）
		channelGroup.POST("/:id/update", NotificationChannelUpdate)
		// 通知渠道信息更新元信息
		views.RegisterApi(channelGroup, "/:id/update", "通知渠道信息更新 - 更新启用状态/配置/描述（名称与类型不可变）", views.WithMethod(http.MethodPost, views.WithBodyInput[NotificationChannelUpdateIn]()))
		// 删除通知渠道 - 批量删除
		channelGroup.POST("/delete", NotificationChannelBatchDelete)
		// 删除通知渠道元信息
		views.RegisterApi(channelGroup, "/delete", "删除通知渠道 - 批量删除",
			views.WithMethod(http.MethodPost, views.WithBodyInput[NotificationChannelBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		)
		// 启用通知渠道 - 批量启用
		channelGroup.POST("/enable", NotificationChannelBatchSetEnabled(true))
		// 启用通知渠道元信息
		views.RegisterApi(channelGroup, "/enable", "启用通知渠道 - 批量启用",
			views.WithMethod(http.MethodPost, views.WithBodyInput[NotificationChannelBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		)
		// 停用通知渠道 - 批量停用
		channelGroup.POST("/disable", NotificationChannelBatchSetEnabled(false))
		// 停用通知渠道元信息
		views.RegisterApi(channelGroup, "/disable", "停用通知渠道 - 批量停用",
			views.WithMethod(http.MethodPost, views.WithBodyInput[NotificationChannelBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		)
	}

	// ========== 通知目标：/api/v1/notification/target ==========
	// 通知目标管理（增删改查、启停、批量删除与测试发送）。
	{
		targetGroup := notificationGroup.Group("/target", ctx.MessagePool)
		// 通知目标列表查询 - 支持分页、搜索和排序（含所属渠道信息）
		targetGroup.POST("/list", NotificationTargetListGet)
		views.RegisterListApi[*views.NotificationTargetListGetSearchIn, *NotificationTargetListOut](targetGroup, "/list", "通知目标列表查询 - 支持分页、搜索和排序（含所属渠道信息）",
			views.WithCapability(views.CapabilityMeta{
				ID:        "notification.target.list",
				Domain:    "notification",
				Intent:    "查看通知目标列表",
				Entities:  []string{"notification_targets"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 创建通知目标 - 类型由所选渠道派生
		targetGroup.POST("/create", NotificationTargetCreate)
		views.RegisterApi(targetGroup, "/create", "创建通知目标 - 类型由所选渠道派生", views.WithMethod(http.MethodPost, views.WithBodyInput[NotificationTargetCreateIn]()))
		// 通知目标详情查询
		targetGroup.POST("/:id/get", NotificationTargetDetailGet)
		views.RegisterApi(targetGroup, "/:id/get", "通知目标详情查询",
			views.WithMethod(http.MethodPost, views.WithBodyOutput[NotificationTargetDetailOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "notification.target.detail",
				Domain:    "notification",
				Intent:    "查看通知目标详情",
				Entities:  []string{"notification_targets"},
				Access:    capability.AccessRead,
				Invokable: false,
			}),
		)
		// 通知目标信息更新 - 更新启用状态/配置/语言/描述（名称与渠道不可变）
		targetGroup.POST("/:id/update", NotificationTargetUpdate)
		views.RegisterApi(targetGroup, "/:id/update", "通知目标信息更新 - 更新启用状态/配置/语言/描述（名称与渠道不可变）", views.WithMethod(http.MethodPost, views.WithBodyInput[NotificationTargetUpdateIn]()))
		// 通知目标测试发送 - 按目标偏好语言渲染测试消息并实际投递
		targetGroup.POST("/:id/test-send", NotificationTargetTestSend)
		views.RegisterApi(targetGroup, "/:id/test-send", "通知目标测试发送 - 按目标偏好语言渲染测试消息并实际投递", views.WithMethod(http.MethodPost, views.WithBodyOutput[NotificationTargetTestSendOut]()))
		// 向单个通知目标发送通知（投递 notify:send:target 异步任务，复用通知全链路）
		targetGroup.POST("/:id/send", ctx.PipelineClient, NotificationTargetSend)
		views.RegisterApi(targetGroup, "/:id/send", "向单个通知目标发送通知（投递 notify:send:target 异步任务，复用通知全链路）", views.WithMethod(http.MethodPost, views.WithBodyInput[NotificationTargetSendIn](), views.WithBodyOutput[NotificationTargetSendOut]()))
		// 删除通知目标 - 批量删除
		targetGroup.POST("/delete", NotificationTargetBatchDelete)
		views.RegisterApi(targetGroup, "/delete", "删除通知目标 - 批量删除",
			views.WithMethod(http.MethodPost, views.WithBodyInput[NotificationTargetBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		)
		// 启用通知目标 - 批量启用
		targetGroup.POST("/enable", NotificationTargetBatchSetEnabled(true))
		views.RegisterApi(targetGroup, "/enable", "启用通知目标 - 批量启用",
			views.WithMethod(http.MethodPost, views.WithBodyInput[NotificationTargetBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		)
		// 停用通知目标 - 批量停用
		targetGroup.POST("/disable", NotificationTargetBatchSetEnabled(false))
		views.RegisterApi(targetGroup, "/disable", "停用通知目标 - 批量停用",
			views.WithMethod(http.MethodPost, views.WithBodyInput[NotificationTargetBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		)
	}

	// ========== 通知组：/api/v1/notification/group ==========
	// 通知目标组管理（增删改查、批量删除、默认组设置与目标集合替换）。
	{
		groupGroup := notificationGroup.Group("/group")
		// 通知组列表查询 - 支持分页、搜索和排序（含目标计数）
		groupGroup.POST("/list", NotificationGroupListGet)
		views.RegisterListApi[*views.NotificationGroupListGetSearchIn, *NotificationGroupListOut](groupGroup, "/list", "通知组列表查询 - 支持分页、搜索和排序（含目标计数）",
			views.WithCapability(views.CapabilityMeta{
				ID:        "notification.group.list",
				Domain:    "notification",
				Intent:    "查看通知组及其绑定目标",
				Entities:  []string{"notification_groups"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 创建通知组 - 可标记为命名空间默认组
		groupGroup.POST("/create", NotificationGroupCreate)
		views.RegisterApi(groupGroup, "/create", "创建通知组 - 可标记为命名空间默认组", views.WithMethod(http.MethodPost, views.WithBodyInput[NotificationGroupCreateIn]()))
		// 通知组详情查询 - 含已绑定目标列表
		groupGroup.POST("/:id/get", NotificationGroupDetailGet)
		views.RegisterApi(groupGroup, "/:id/get", "通知组详情查询 - 含已绑定目标列表",
			views.WithMethod(http.MethodPost, views.WithBodyOutput[NotificationGroupDetailOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "notification.group.detail",
				Domain:    "notification",
				Intent:    "查看通知组详情及其绑定目标",
				Entities:  []string{"notification_groups", "notification_group_targets"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 通知组信息更新 - 仅更新描述
		groupGroup.POST("/:id/update", NotificationGroupUpdate)
		views.RegisterApi(groupGroup, "/:id/update", "通知组信息更新 - 仅更新描述", views.WithMethod(http.MethodPost, views.WithBodyInput[NotificationGroupUpdateIn]()))
		// 设置默认通知组
		groupGroup.POST("/:id/set-default", NotificationGroupSetDefault)
		views.RegisterApi(groupGroup, "/:id/set-default", "设置默认通知组", views.WithMethod(http.MethodPost))
		// 删除通知组 - 批量删除
		groupGroup.POST("/delete", NotificationGroupBatchDelete)
		views.RegisterApi(groupGroup, "/delete", "删除通知组 - 批量删除",
			views.WithMethod(http.MethodPost, views.WithBodyInput[NotificationGroupBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		)
	}

	// ========== 通知模板：/api/v1/notification/template ==========
	// 通知模板管理（列表、创建/更新、恢复默认与渲染预览）。
	{
		templateGroup := notificationGroup.Group("/template", ctx.MessagePool)
		// 通知模板列表查询 - 支持事件/来源/语言/渠道类型过滤与分页
		templateGroup.POST("/list", NotificationTemplateListGet)
		// 通知模板列表元信息
		views.RegisterListApi[*views.NotificationTemplateListGetSearchIn, *NotificationTemplateListOut](templateGroup, "/list", "通知模板列表查询 - 支持事件/来源/语言/渠道类型过滤与分页",
			views.WithCapability(views.CapabilityMeta{
				ID:        "notification.template.list",
				Domain:    "notification",
				Intent:    "查看通知模板列表",
				Entities:  []string{"notification_templates"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 通知事件精简元数据 - 事件支持语言与模板变量清单（供编辑器语言下拉提示与智能补全）
		templateGroup.POST("/events/meta", NotificationTemplateEventsMetaGet)
		views.RegisterApi(templateGroup, "/events/meta", "通知事件精简元数据 - 事件支持语言与模板变量清单（供编辑器语言下拉提示与智能补全）",
			views.WithMethod(http.MethodPost),
			views.WithCapability(views.CapabilityMeta{
				ID:        "notification.template.events-meta",
				Domain:    "notification",
				Intent:    "查看通知事件支持的语言与模板变量",
				Entities:  []string{},
				Access:    capability.AccessRead,
				Invokable: false,
			}),
		)
		// 创建通知模板（用户覆盖模板，source=override）
		templateGroup.POST("/create", NotificationTemplateCreate)
		// 创建通知模板元信息
		views.RegisterApi(templateGroup, "/create", "创建通知模板（用户覆盖模板，source=override）", views.WithMethod(http.MethodPost, views.WithBodyInput[NotificationTemplateCreateIn]()))
		// 更新通知模板（标题/正文/描述，强制 source=override）
		templateGroup.POST("/update", NotificationTemplateUpdate)
		// 更新通知模板元信息
		views.RegisterApi(templateGroup, "/update", "更新通知模板（标题/正文/描述，强制 source=override）", views.WithMethod(http.MethodPost, views.WithBodyInput[NotificationTemplateUpdateIn]()))
		// 恢复事件模板为内置默认（整事件重置）
		templateGroup.POST("/restore-default", NotificationTemplateRestoreDefault)
		// 恢复默认元信息
		views.RegisterApi(templateGroup, "/restore-default", "恢复事件模板为内置默认（整事件重置）", views.WithMethod(http.MethodPost, views.WithBodyInput[NotificationTemplateRestoreDefaultIn]()))
		// 模板渲染预览（render-test）
		templateGroup.POST("/render-test", NotificationTemplateRenderTest)
		// 模板渲染预览元信息
		views.RegisterApi(templateGroup, "/render-test", "模板渲染预览（render-test）",
			views.WithMethod(http.MethodPost, views.WithBodyInput[NotificationTemplateRenderTestIn](), views.WithBodyOutput[NotificationTemplateRenderTestOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "notification.template.render",
				Domain:    "notification",
				Intent:    "预览通知模板渲染结果（不落库）",
				Entities:  []string{},
				Access:    capability.AccessRead,
				Invokable: false,
			}),
		)
		// 模板语法校验（校验编辑器内尚未保存的标题/正文）
		templateGroup.POST("/validate", NotificationTemplateValidate)
		// 模板语法校验元信息
		views.RegisterApi(templateGroup, "/validate", "模板语法校验（校验编辑器内尚未保存的标题/正文）",
			views.WithMethod(http.MethodPost, views.WithBodyInput[NotificationTemplateValidateIn](), views.WithBodyOutput[NotificationTemplateValidateOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "notification.template.validate",
				Domain:    "notification",
				Intent:    "校验通知模板语法（不落库）",
				Entities:  []string{},
				Access:    capability.AccessRead,
				Invokable: false,
			}),
		)
		// 模板试运行（使用事件内置样例参数渲染编辑器内尚未保存的标题/正文）
		templateGroup.POST("/dry-run", NotificationTemplateDryRun)
		// 模板试运行元信息
		views.RegisterApi(templateGroup, "/dry-run", "模板试运行（使用事件内置样例参数渲染编辑器内尚未保存的标题/正文）",
			views.WithMethod(http.MethodPost, views.WithBodyInput[NotificationTemplateDryRunIn](), views.WithBodyOutput[NotificationTemplateDryRunOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "notification.template.dry-run",
				Domain:    "notification",
				Intent:    "使用内置样例参数试运行通知模板（不落库）",
				Entities:  []string{},
				Access:    capability.AccessRead,
				Invokable: false,
			}),
		)
	}

	// ========== 通知发送记录：/api/v1/notification/record ==========
	// 通知发送记录管理（只读：列表与详情）。
	{
		recordGroup := notificationGroup.Group("/record")
		// 通知记录列表查询 - 支持分页、搜索和排序（状态/事件/渠道/时间过滤）
		recordGroup.POST("/list", NotificationRecordListGet)
		// 通知记录列表元信息
		views.RegisterListApi[*views.NotificationRecordListGetSearchIn, *NotificationRecordListOut](recordGroup, "/list", "通知记录列表查询 - 支持分页、搜索和排序（状态/事件/渠道/时间过滤）",
			views.WithCapability(views.CapabilityMeta{
				ID:        "notification.record.list",
				Domain:    "notification",
				Intent:    "查看通知发送记录",
				Entities:  []string{"notification_records"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 通知记录详情查询 - 含 payload/error/messageId/notifiedAt
		recordGroup.POST("/:id/get", NotificationRecordDetailGet)
		// 通知记录详情元信息
		views.RegisterApi(recordGroup, "/:id/get", "通知记录详情查询 - 含 payload/error/messageId/notifiedAt",
			views.WithMethod(http.MethodPost, views.WithBodyOutput[NotificationRecordDetailOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "notification.record.detail",
				Domain:    "notification",
				Intent:    "查看通知发送记录详情",
				Entities:  []string{"notification_records"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
	}
}
