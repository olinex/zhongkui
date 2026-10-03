// Package tag 提供标签域（规则 / 规则样本 / 事件规则 / 事件记录 / S3 样本）的 API 处理器与视图
// In/Out 结构体。
package tag

import (
	// 标准库
	"net/http"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/manager/views"
)

// RegisterRoutes 注册 /api/v1/tag-event 标签事件接口路由。
// 标签事件分类下的规则管理，与流量审计同级的独立分类；
// 后续标签事件记录、告警配置、告警记录等功能均在此分类下扩展。
// 认证 / RBAC / raw 库（标签事件规则 dry-run 查询 tag_summings）/ S3 / 命名空间等公共中间件挂在父组上继承。
func RegisterRoutes(ctx *views.RegisterContext) {
	tagEventGroup := ctx.V1Group.Group(
		"/tag-event",          // 标签事件接口路径前缀
		ctx.Authenticator,     // 认证器注入
		ctx.ForceAuthenticate, // 强制JWT认证
		ctx.Policy,            // RBAC策略引擎
		ctx.Namespace,         // 命名空间隔离
		ctx.Authorize,         // 权限验证（api:{FullPath} + HTTP 方法动态授权）
		ctx.RawDatabase,       // raw 库（ClickHouse）连接（标签事件规则 dry-run）
		ctx.S3,                // S3 客户端（tag-rule-sample body 副本）
		ctx.SQLiteVTabPools,   // SQLite vtable 池（数据源 CRUD 及规则 dry-run 查询裸名 vtable）
	)

	// ========== 标签识别规则：/api/v1/tag-event/tag-rule ==========
	// 标签事件命名空间下的标签识别规则管理（增删改查、启停、校验、dry-run 与版本回滚）。
	// 命名空间化：规则归属请求命名空间；写路径需要标签引擎中间件失效缓存与广播变更。
	{
		tagRuleGroup := tagEventGroup.Group("/tag-rule", ctx.CoreDatabase, ctx.TagRuleEngine)
		// 标签规则列表查询 - 支持分页、搜索和排序
		tagRuleGroup.POST("/list", TagRuleListGet)
		// 标签规则列表自省信息 - 返回搜索选项、表格列与各 HTTP 方法的入参出参结构体定义
		tagRuleGroup.OPTIONS("/list", views.ListMetaGinHandler[*views.TagRuleListGetSearchIn, *TagRuleListOut]())
		// 创建标签规则 - 创建新的标签识别规则（归属当前请求命名空间）
		tagRuleGroup.POST("/create", TagRuleCreate)
		// 创建标签规则元信息
		tagRuleGroup.OPTIONS("/create", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[TagRuleCreateIn]())))
		// 标签规则详情查询 - 根据ID获取规则详细信息
		tagRuleGroup.POST("/:id/get", TagRuleDetailGet)
		// 标签规则详情元信息
		tagRuleGroup.OPTIONS("/:id/get", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[TagRuleDetailOut]())))
		// 标签规则信息更新 - 更新规则描述/内容/启用状态
		tagRuleGroup.POST("/:id/update", TagRuleUpdate)
		// 标签规则信息更新元信息
		tagRuleGroup.OPTIONS("/:id/update", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[TagRuleUpdateIn]())))
		// 删除标签规则 - 批量删除规则及其全部历史版本
		tagRuleGroup.POST("/delete", TagRuleBatchDelete)
		// 删除标签规则元信息
		tagRuleGroup.OPTIONS("/delete", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[TagRuleBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 启用标签规则 - 批量启用规则
		tagRuleGroup.POST("/enable", TagRuleBatchSetEnabled(true))
		// 启用标签规则元信息
		tagRuleGroup.OPTIONS("/enable", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[TagRuleBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 停用标签规则 - 批量停用规则
		tagRuleGroup.POST("/disable", TagRuleBatchSetEnabled(false))
		// 停用标签规则元信息
		tagRuleGroup.OPTIONS("/disable", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[TagRuleBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 标签规则语法校验 - 校验 Rego 源码语法与契约，返回带行列号的错误
		tagRuleGroup.POST("/validate", TagRuleValidate)
		// 标签规则语法校验元信息
		tagRuleGroup.OPTIONS("/validate", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[TagRuleValidateIn](), views.WithBodyOutput[TagRuleValidateOut]()),
		))
		// 标签规则单规则 dry-run - 对一条候选规则评估 JSON 输入文档，输出运行细节
		tagRuleGroup.POST("/single/dry-run", TagRuleDryRun)
		// 标签规则单规则 dry-run 元信息
		tagRuleGroup.OPTIONS("/single/dry-run", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[TagRuleDryRunRuleIn](), views.WithBodyOutput[TagRuleDryRunOut]()),
		))
		// 标签规则规则组 dry-run - 对一组候选规则评估 JSON 输入文档，输出逐规则与合并标签
		tagRuleGroup.POST("/group/dry-run", TagRuleDryRunGroup)
		// 标签规则规则组 dry-run 元信息
		tagRuleGroup.OPTIONS("/group/dry-run", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[TagRuleDryRunGroupIn](), views.WithBodyOutput[TagRuleDryRunOut]()),
		))
		// 标签规则版本列表 - 获取规则的全部历史版本
		tagRuleGroup.POST("/:id/version/list", TagRuleVersionList)
		// 标签规则版本列表元信息
		tagRuleGroup.OPTIONS("/:id/version/list", views.StructMetaGinHandler(views.WithMethod(http.MethodPost)))
		// 标签规则回滚 - 将规则回滚到指定历史版本
		tagRuleGroup.POST("/:id/version/:version/rollback", TagRuleVersionRollback)
		// 标签规则回滚元信息
		tagRuleGroup.OPTIONS("/:id/version/:version/rollback", views.StructMetaGinHandler(views.WithMethod(http.MethodPost)))
		// 标签规则目标模型元信息 - 返回输入文档字段元信息，供 Rego 编辑器补全与字段面板
		tagRuleGroup.POST("/model/metadata", TagRuleModelMetadata)
		// 标签规则目标模型元信息元信息
		tagRuleGroup.OPTIONS("/model/metadata", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[TagRuleModelIn](), views.WithBodyOutput[TagRuleModelMetadataOut]())))
		// 标签规则目标模型样例 - 返回当前命名空间下的系统样例列表，缺失时懒供给
		tagRuleGroup.POST("/model/samples", TagRuleModelSamples)
		// 标签规则目标模型样例元信息
		tagRuleGroup.OPTIONS("/model/samples", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[TagRuleModelIn](), views.WithBodyOutput[TagRuleModelSamplesOut]())))
		// 标签规则自定义 OPA 内置函数元信息 - 供 Rego 编辑器函数补全/悬停提示
		tagRuleGroup.POST("/functions", TagRuleFunctions)
		// 标签规则自定义 OPA 内置函数元信息
		tagRuleGroup.OPTIONS("/functions", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[TagRuleFunctionsOut]())))
		// 标签规则 sqlite 语句元信息 - 返回当前命名空间已注册数据源 schema，供 Rego 编辑器 SQL 补全/提示
		tagRuleGroup.POST("/sqlite/meta", TagRuleSqliteMeta)
		// 标签规则 sqlite 语句元信息
		tagRuleGroup.OPTIONS("/sqlite/meta", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[TagRuleSqliteMetaOut]())))
	}

	// ========== 标签规则样例：/api/v1/tag-event/tag-rule-sample ==========
	// 标签事件命名空间下的标签规则验证样例管理（样例归属命名空间，model/input 创建后不可变）
	{
		tagRuleSampleGroup := tagEventGroup.Group("/tag-rule-sample", ctx.CoreDatabase)
		// 标签规则样例列表查询 - 支持分页、搜索和排序
		tagRuleSampleGroup.POST("/list", TagRuleSampleListGet)
		// 标签规则样例列表自省信息 - 返回搜索选项、表格列与各 HTTP 方法的入参出参结构体定义
		tagRuleSampleGroup.OPTIONS("/list", views.ListMetaGinHandler[*views.TagRuleSampleListGetSearchIn, *TagRuleSampleListOut]())
		// 创建标签规则样例 - 创建新的验证样例（归属当前请求命名空间，model/input 创建后不可变）
		tagRuleSampleGroup.POST("/create", TagRuleSampleCreate)
		// 创建标签规则样例元信息
		tagRuleSampleGroup.OPTIONS("/create", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[TagRuleSampleCreateIn]())))
		// 标签规则样例详情查询 - 根据ID获取样例完整信息
		tagRuleSampleGroup.POST("/:id/get", TagRuleSampleDetailGet)
		// 标签规则样例详情元信息
		tagRuleSampleGroup.OPTIONS("/:id/get", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[TagRuleSampleDetailOut]())))
		// 标签规则样例信息更新 - 仅允许更新样例名称与说明（model/input 不可变）
		tagRuleSampleGroup.POST("/:id/update", TagRuleSampleUpdate)
		// 标签规则样例信息更新元信息
		tagRuleSampleGroup.OPTIONS("/:id/update", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[TagRuleSampleUpdateIn]())))
		// 删除标签规则样例 - 批量删除指定样例
		tagRuleSampleGroup.POST("/delete", TagRuleSampleBatchDelete)
		// 删除标签规则样例元信息
		tagRuleSampleGroup.OPTIONS("/delete", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[TagRuleSampleBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
	}

	// ========== 标签事件规则：/api/v1/tag-event/tag-event-rule ==========
	// 标签事件命名空间下的标签事件规则管理（增删改查、启停、表达式校验、dry-run 与版本回滚）。
	// dry-run 查询 tag_summings 所需 raw 库中间件已由 tagEventGroup 父组提供。
	{
		tagEventRuleGroup := tagEventGroup.Group("/tag-event-rule", ctx.CoreDatabase)
		// 标签事件规则列表查询 - 支持分页、搜索和排序
		tagEventRuleGroup.POST("/list", TagEventRuleListGet)
		// 标签事件规则列表自省信息 - 返回搜索选项、表格列与各 HTTP 方法的入参出参结构体定义
		tagEventRuleGroup.OPTIONS("/list", views.ListMetaGinHandler[*views.TagEventRuleListGetSearchIn, *TagEventRuleListOut]())
		// 创建标签事件规则 - 创建新的标签事件规则，创建前逐条件校验表达式语法
		tagEventRuleGroup.POST("/create", TagEventRuleCreate)
		// 创建标签事件规则元信息
		tagEventRuleGroup.OPTIONS("/create", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[TagEventRuleCreateIn]())))
		// 标签事件规则详情查询 - 根据ID获取规则详细信息
		tagEventRuleGroup.POST("/:id/get", TagEventRuleDetailGet)
		// 标签事件规则详情元信息
		tagEventRuleGroup.OPTIONS("/:id/get", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[TagEventRuleDetailOut]())))
		// 标签事件规则信息更新 - 更新规则描述/匹配策略/启用状态/条件集合，更新前快照当前版本
		tagEventRuleGroup.POST("/:id/update", TagEventRuleUpdate)
		// 标签事件规则信息更新元信息
		tagEventRuleGroup.OPTIONS("/:id/update", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[TagEventRuleUpdateIn]())))
		// 删除标签事件规则 - 批量删除规则及其全部条件与历史版本
		tagEventRuleGroup.POST("/delete", TagEventRuleBatchDelete)
		// 删除标签事件规则元信息
		tagEventRuleGroup.OPTIONS("/delete", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[TagEventRuleBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 启用标签事件规则 - 批量启用规则
		tagEventRuleGroup.POST("/enable", TagEventRuleBatchSetEnabled(true))
		// 启用标签事件规则元信息
		tagEventRuleGroup.OPTIONS("/enable", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[TagEventRuleBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 停用标签事件规则 - 批量停用规则
		tagEventRuleGroup.POST("/disable", TagEventRuleBatchSetEnabled(false))
		// 停用标签事件规则元信息
		tagEventRuleGroup.OPTIONS("/disable", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[TagEventRuleBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 标签事件规则表达式校验 - 校验条件表达式语法，返回精炼后的标准表达式或错误消息
		tagEventRuleGroup.POST("/validate", TagEventRuleValidate)
		// 标签事件规则表达式校验元信息
		tagEventRuleGroup.OPTIONS("/validate", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[TagEventRuleValidateIn](), views.WithBodyOutput[TagEventRuleValidateOut]()),
		))
		// 标签事件规则 dry-run - 对一条候选规则（不落库）执行事件检测，查询 tag_summings
		tagEventRuleGroup.POST("/dry-run", TagEventRuleDryRun)
		// 标签事件规则 dry-run 元信息
		tagEventRuleGroup.OPTIONS("/dry-run", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[TagEventRuleDryRunIn](), views.WithBodyOutput[TagEventRuleDryRunOut]()),
		))
		// 标签事件规则版本列表 - 获取规则的全部历史版本
		tagEventRuleGroup.POST("/:id/version/list", TagEventRuleVersionList)
		// 标签事件规则版本列表元信息
		tagEventRuleGroup.OPTIONS("/:id/version/list", views.StructMetaGinHandler(views.WithMethod(http.MethodPost)))
		// 标签事件规则回滚 - 将规则回滚到指定历史版本，回滚前先快照当前内容
		tagEventRuleGroup.POST("/:id/version/:version/rollback", TagEventRuleVersionRollback)
		// 标签事件规则回滚元信息
		tagEventRuleGroup.OPTIONS("/:id/version/:version/rollback", views.StructMetaGinHandler(views.WithMethod(http.MethodPost)))
	}

	// ========== 标签事件记录：/api/v1/tag-event/record ==========
	// 标签事件记录管理：记录列表/详情查询与手动关闭（置为 closed），
	// 详情关联展示该事件触发的多条通知发送记录。
	{
		eventGroup := tagEventGroup.Group("/record", ctx.CoreDatabase)
		// 标签事件记录列表 - 支持分页、搜索、排序与筛选
		eventGroup.POST("/list", TagEventRecordListGet)
		// 标签事件记录列表自省信息 - 返回搜索选项、表格列与出参结构体定义
		eventGroup.OPTIONS("/list", views.ListMetaGinHandler[*views.TagEventRecordListGetSearchIn, *TagEventRecordListOut]())
		// 标签事件记录详情 - 返回事件基础信息、命中明细与关联发送记录
		eventGroup.POST("/:id/get", TagEventRecordDetailGet)
		// 标签事件记录详情元信息
		eventGroup.OPTIONS("/:id/get", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyOutput[TagEventRecordDetailOut]()),
		))
		// 批量关闭标签事件记录 - 将当前命名空间下未关闭的事件记录置为 closed
		eventGroup.POST("/close", TagEventRecordBatchClose)
		// 批量关闭标签事件记录元信息
		eventGroup.OPTIONS("/close", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[TagEventRuleBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 命中条件原始数据反查 - 前端透传命中明细的模型、原始记录 ID 列表与入库时间范围，
		// 后端按 created_at 分区预筛查询对应明细行（不再回查命中明细归属）
		eventGroup.POST("/hit/records", TagEventRecordHitRecordsGet)
		// 命中条件原始数据反查元信息
		eventGroup.OPTIONS("/hit/records", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[TagEventRecordHitRecordsIn](), views.WithBodyOutput[TagEventRecordHitRecordsOut]()),
		))
	}
}
