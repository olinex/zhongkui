package router

import (
	"net/http"

	"zhongkui.com/zhongkui/internal/manager/views"
)

// registerTagEventRoutes 注册 /api/v1/tag-event 标签事件接口路由。
// 标签事件分类下的规则管理，与流量审计同级的独立分类；
// 后续标签事件记录、告警配置、告警记录等功能均在此分类下扩展。
// 认证 / RBAC / raw 库（标签事件规则 dry-run 查询 tag_summings）/ S3 / 命名空间等公共中间件挂在父组上继承。
func registerTagEventRoutes(ctx *RegisterContext) {
	tagEventGroup := ctx.v1Group.Group(
		"/tag-event",                    // 标签事件接口路径前缀
		ctx.authenticatorMiddleware,     // 认证器注入
		ctx.forceAuthenticateMiddleware, // 强制JWT认证
		ctx.policyMiddleware,            // RBAC策略引擎
		ctx.namespaceMiddleware,         // 命名空间隔离
		ctx.authorizeMiddleware,         // 权限验证（api:{FullPath} + HTTP 方法动态授权）
		ctx.rawDatabaseMiddleware,       // raw 库（ClickHouse）连接（标签事件规则 dry-run）
		ctx.s3Middleware,                // S3 客户端（tag-rule-sample body 副本）
		ctx.sqliteVTabPoolsMiddleware,   // SQLite vtable 池（数据源 CRUD 及规则 dry-run 查询裸名 vtable）
	)

	// ========== 标签识别规则：/api/v1/tag-event/tag-rule ==========
	// 标签事件命名空间下的标签识别规则管理（增删改查、启停、校验、dry-run 与版本回滚）。
	// 命名空间化：规则归属请求命名空间；写路径需要标签引擎中间件失效缓存与广播变更。
	{
		tagRuleGroup := tagEventGroup.Group("/tag-rule", ctx.coreDatabaseMiddleware, ctx.tagRuleEngineMiddleware)
		// 标签规则列表查询 - 支持分页、搜索和排序
		tagRuleGroup.POST("/list", views.TagRuleListGet)
		// 标签规则列表自省信息 - 返回搜索选项、表格列与各 HTTP 方法的入参出参结构体定义
		tagRuleGroup.OPTIONS("/list", views.ListMetaGinHandler[*views.TagRuleListGetSearchIn, *views.TagRuleListOut]())
		// 创建标签规则 - 创建新的标签识别规则（归属当前请求命名空间）
		tagRuleGroup.POST("/create", views.TagRuleCreate)
		// 创建标签规则元信息
		tagRuleGroup.OPTIONS("/create", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.TagRuleCreateIn]())))
		// 标签规则详情查询 - 根据ID获取规则详细信息
		tagRuleGroup.POST("/:id/get", views.TagRuleDetailGet)
		// 标签规则详情元信息
		tagRuleGroup.OPTIONS("/:id/get", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.TagRuleDetailOut]())))
		// 标签规则信息更新 - 更新规则描述/内容/启用状态
		tagRuleGroup.POST("/:id/update", views.TagRuleUpdate)
		// 标签规则信息更新元信息
		tagRuleGroup.OPTIONS("/:id/update", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.TagRuleUpdateIn]())))
		// 删除标签规则 - 批量删除规则及其全部历史版本
		tagRuleGroup.POST("/delete", views.TagRuleBatchDelete)
		// 删除标签规则元信息
		tagRuleGroup.OPTIONS("/delete", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.TagRuleBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 启用标签规则 - 批量启用规则
		tagRuleGroup.POST("/enable", views.TagRuleBatchSetEnabled(true))
		// 启用标签规则元信息
		tagRuleGroup.OPTIONS("/enable", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.TagRuleBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 停用标签规则 - 批量停用规则
		tagRuleGroup.POST("/disable", views.TagRuleBatchSetEnabled(false))
		// 停用标签规则元信息
		tagRuleGroup.OPTIONS("/disable", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.TagRuleBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 标签规则语法校验 - 校验 Rego 源码语法与契约，返回带行列号的错误
		tagRuleGroup.POST("/validate", views.TagRuleValidate)
		// 标签规则语法校验元信息
		tagRuleGroup.OPTIONS("/validate", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.TagRuleValidateIn](), views.WithBodyOutput[views.TagRuleValidateOut]()),
		))
		// 标签规则单规则 dry-run - 对一条候选规则评估 JSON 输入文档，输出运行细节
		tagRuleGroup.POST("/single/dry-run", views.TagRuleDryRun)
		// 标签规则单规则 dry-run 元信息
		tagRuleGroup.OPTIONS("/single/dry-run", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.TagRuleDryRunRuleIn](), views.WithBodyOutput[views.TagRuleDryRunOut]()),
		))
		// 标签规则规则组 dry-run - 对一组候选规则评估 JSON 输入文档，输出逐规则与合并标签
		tagRuleGroup.POST("/group/dry-run", views.TagRuleDryRunGroup)
		// 标签规则规则组 dry-run 元信息
		tagRuleGroup.OPTIONS("/group/dry-run", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.TagRuleDryRunGroupIn](), views.WithBodyOutput[views.TagRuleDryRunOut]()),
		))
		// 标签规则版本列表 - 获取规则的全部历史版本
		tagRuleGroup.POST("/:id/version/list", views.TagRuleVersionList)
		// 标签规则版本列表元信息
		tagRuleGroup.OPTIONS("/:id/version/list", views.StructMetaGinHandler(views.WithMethod(http.MethodPost)))
		// 标签规则回滚 - 将规则回滚到指定历史版本
		tagRuleGroup.POST("/:id/version/:version/rollback", views.TagRuleVersionRollback)
		// 标签规则回滚元信息
		tagRuleGroup.OPTIONS("/:id/version/:version/rollback", views.StructMetaGinHandler(views.WithMethod(http.MethodPost)))
		// 标签规则目标模型选项 - 返回已注册目标模型的下拉选项列表
		tagRuleGroup.POST("/model/options", views.TagRuleModelOptions)
		// 标签规则目标模型选项元信息
		tagRuleGroup.OPTIONS("/model/options", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.TagRuleModelOptionsOut]())))
		// 标签规则目标模型元信息 - 返回输入文档字段元信息，供 Rego 编辑器补全与字段面板
		tagRuleGroup.POST("/model/metadata", views.TagRuleModelMetadata)
		// 标签规则目标模型元信息元信息
		tagRuleGroup.OPTIONS("/model/metadata", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.TagRuleModelIn](), views.WithBodyOutput[views.TagRuleModelMetadataOut]())))
		// 标签规则目标模型样例 - 返回当前命名空间下的系统样例列表，缺失时懒供给
		tagRuleGroup.POST("/model/samples", views.TagRuleModelSamples)
		// 标签规则目标模型样例元信息
		tagRuleGroup.OPTIONS("/model/samples", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.TagRuleModelIn](), views.WithBodyOutput[views.TagRuleModelSamplesOut]())))
		// 标签规则自定义 OPA 内置函数元信息 - 供 Rego 编辑器函数补全/悬停提示
		tagRuleGroup.POST("/functions", views.TagRuleFunctions)
		// 标签规则自定义 OPA 内置函数元信息
		tagRuleGroup.OPTIONS("/functions", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.TagRuleFunctionsOut]())))
		// 标签规则 sqlite 语句元信息 - 返回当前命名空间已注册数据源 schema，供 Rego 编辑器 SQL 补全/提示
		tagRuleGroup.POST("/sqlite/meta", views.TagRuleSqliteMeta)
		// 标签规则 sqlite 语句元信息
		tagRuleGroup.OPTIONS("/sqlite/meta", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.TagRuleSqliteMetaOut]())))
	}

	// ========== 标签规则样例：/api/v1/tag-event/tag-rule-sample ==========
	// 标签事件命名空间下的标签规则验证样例管理（样例归属命名空间，model/input 创建后不可变）
	{
		tagRuleSampleGroup := tagEventGroup.Group("/tag-rule-sample", ctx.coreDatabaseMiddleware)
		// 标签规则样例列表查询 - 支持分页、搜索和排序
		tagRuleSampleGroup.POST("/list", views.TagRuleSampleListGet)
		// 标签规则样例列表自省信息 - 返回搜索选项、表格列与各 HTTP 方法的入参出参结构体定义
		tagRuleSampleGroup.OPTIONS("/list", views.ListMetaGinHandler[*views.TagRuleSampleListGetSearchIn, *views.TagRuleSampleListOut]())
		// 创建标签规则样例 - 创建新的验证样例（归属当前请求命名空间，model/input 创建后不可变）
		tagRuleSampleGroup.POST("/create", views.TagRuleSampleCreate)
		// 创建标签规则样例元信息
		tagRuleSampleGroup.OPTIONS("/create", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.TagRuleSampleCreateIn]())))
		// 标签规则样例详情查询 - 根据ID获取样例完整信息
		tagRuleSampleGroup.POST("/:id/get", views.TagRuleSampleDetailGet)
		// 标签规则样例详情元信息
		tagRuleSampleGroup.OPTIONS("/:id/get", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.TagRuleSampleDetailOut]())))
		// 标签规则样例信息更新 - 仅允许更新样例名称与说明（model/input 不可变）
		tagRuleSampleGroup.POST("/:id/update", views.TagRuleSampleUpdate)
		// 标签规则样例信息更新元信息
		tagRuleSampleGroup.OPTIONS("/:id/update", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.TagRuleSampleUpdateIn]())))
		// 删除标签规则样例 - 批量删除指定样例
		tagRuleSampleGroup.POST("/delete", views.TagRuleSampleBatchDelete)
		// 删除标签规则样例元信息
		tagRuleSampleGroup.OPTIONS("/delete", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.TagRuleSampleBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
	}

	// ========== 标签事件规则：/api/v1/tag-event/tag-event-rule ==========
	// 标签事件命名空间下的标签事件规则管理（增删改查、启停、表达式校验、dry-run 与版本回滚）。
	// dry-run 查询 tag_summings 所需 raw 库中间件已由 tagEventGroup 父组提供。
	{
		tagEventRuleGroup := tagEventGroup.Group("/tag-event-rule", ctx.coreDatabaseMiddleware)
		// 标签事件规则列表查询 - 支持分页、搜索和排序
		tagEventRuleGroup.POST("/list", views.TagEventRuleListGet)
		// 标签事件规则列表自省信息 - 返回搜索选项、表格列与各 HTTP 方法的入参出参结构体定义
		tagEventRuleGroup.OPTIONS("/list", views.ListMetaGinHandler[*views.TagEventRuleListGetSearchIn, *views.TagEventRuleListOut]())
		// 创建标签事件规则 - 创建新的标签事件规则，创建前逐条件校验表达式语法
		tagEventRuleGroup.POST("/create", views.TagEventRuleCreate)
		// 创建标签事件规则元信息
		tagEventRuleGroup.OPTIONS("/create", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.TagEventRuleCreateIn]())))
		// 标签事件规则详情查询 - 根据ID获取规则详细信息
		tagEventRuleGroup.POST("/:id/get", views.TagEventRuleDetailGet)
		// 标签事件规则详情元信息
		tagEventRuleGroup.OPTIONS("/:id/get", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.TagEventRuleDetailOut]())))
		// 标签事件规则信息更新 - 更新规则描述/匹配策略/启用状态/条件集合，更新前快照当前版本
		tagEventRuleGroup.POST("/:id/update", views.TagEventRuleUpdate)
		// 标签事件规则信息更新元信息
		tagEventRuleGroup.OPTIONS("/:id/update", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.TagEventRuleUpdateIn]())))
		// 删除标签事件规则 - 批量删除规则及其全部条件与历史版本
		tagEventRuleGroup.POST("/delete", views.TagEventRuleBatchDelete)
		// 删除标签事件规则元信息
		tagEventRuleGroup.OPTIONS("/delete", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.TagEventRuleBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 启用标签事件规则 - 批量启用规则
		tagEventRuleGroup.POST("/enable", views.TagEventRuleBatchSetEnabled(true))
		// 启用标签事件规则元信息
		tagEventRuleGroup.OPTIONS("/enable", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.TagEventRuleBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 停用标签事件规则 - 批量停用规则
		tagEventRuleGroup.POST("/disable", views.TagEventRuleBatchSetEnabled(false))
		// 停用标签事件规则元信息
		tagEventRuleGroup.OPTIONS("/disable", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.TagEventRuleBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 标签事件规则表达式校验 - 校验条件表达式语法，返回精炼后的标准表达式或错误消息
		tagEventRuleGroup.POST("/validate", views.TagEventRuleValidate)
		// 标签事件规则表达式校验元信息
		tagEventRuleGroup.OPTIONS("/validate", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.TagEventRuleValidateIn](), views.WithBodyOutput[views.TagEventRuleValidateOut]()),
		))
		// 标签事件规则 dry-run - 对一条候选规则（不落库）执行事件检测，查询 tag_summings
		tagEventRuleGroup.POST("/dry-run", views.TagEventRuleDryRun)
		// 标签事件规则 dry-run 元信息
		tagEventRuleGroup.OPTIONS("/dry-run", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.TagEventRuleDryRunIn](), views.WithBodyOutput[views.TagEventRuleDryRunOut]()),
		))
		// 标签事件规则版本列表 - 获取规则的全部历史版本
		tagEventRuleGroup.POST("/:id/version/list", views.TagEventRuleVersionList)
		// 标签事件规则版本列表元信息
		tagEventRuleGroup.OPTIONS("/:id/version/list", views.StructMetaGinHandler(views.WithMethod(http.MethodPost)))
		// 标签事件规则回滚 - 将规则回滚到指定历史版本，回滚前先快照当前内容
		tagEventRuleGroup.POST("/:id/version/:version/rollback", views.TagEventRuleVersionRollback)
		// 标签事件规则回滚元信息
		tagEventRuleGroup.OPTIONS("/:id/version/:version/rollback", views.StructMetaGinHandler(views.WithMethod(http.MethodPost)))
	}

	// ========== 标签事件记录：/api/v1/tag-event/record ==========
	// 标签事件记录管理：记录列表/详情查询与手动关闭（置为 closed），
	// 详情关联展示该事件触发的多条通知发送记录。
	{
		eventGroup := tagEventGroup.Group("/record", ctx.coreDatabaseMiddleware)
		// 标签事件记录列表 - 支持分页、搜索、排序与筛选
		eventGroup.POST("/list", views.TagEventRecordListGet)
		// 标签事件记录列表自省信息 - 返回搜索选项、表格列与出参结构体定义
		eventGroup.OPTIONS("/list", views.ListMetaGinHandler[*views.TagEventRecordListGetSearchIn, *views.TagEventRecordListOut]())
		// 标签事件记录详情 - 返回事件基础信息、命中明细与关联发送记录
		eventGroup.POST("/:id/get", views.TagEventRecordDetailGet)
		// 标签事件记录详情元信息
		eventGroup.OPTIONS("/:id/get", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyOutput[views.TagEventRecordDetailOut]()),
		))
		// 批量关闭标签事件记录 - 将当前命名空间下未关闭的事件记录置为 closed
		eventGroup.POST("/close", views.TagEventRecordBatchClose)
		// 批量关闭标签事件记录元信息
		eventGroup.OPTIONS("/close", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.TagEventRuleBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 命中条件原始数据反查 - 前端透传命中明细的模型、原始记录 ID 列表与入库时间范围，
		// 后端按 created_at 分区预筛查询对应明细行（不再回查命中明细归属）
		eventGroup.POST("/hit/records", views.TagEventRecordHitRecordsGet)
		// 命中条件原始数据反查元信息
		eventGroup.OPTIONS("/hit/records", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.TagEventRecordHitRecordsIn](), views.WithBodyOutput[views.TagEventRecordHitRecordsOut]()),
		))
	}
}
