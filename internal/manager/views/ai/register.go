package ai

import (
	"net/http"

	"zhongkuisec.com/zhongkui/internal/ai/capability"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/manager/views"
)

// RegisterRoutes 注册 /api/v1/ai AI 助手（RAG + Agent）接口。
// 分组挂认证/授权/命名空间/核心库/功能开关/AI 组件/S3/对象缓存/管道投递客户端中间件；子组：
//   - /doc    知识库文档 CRUD/list/reindex/chunks + 导入上传/原件下载
//   - /session  Agent 会话 create/list/get/rename/valve/delete/messages
//   - /memory  长期记忆 create/list(get/update)/delete/status
//   - /skill   技能 create/list/detail/update/delete/status
//   - /periodic-report  周期性报告：技能选项、计划 CRUD/启停/立即执行、运行记录、关注与信息流
//   - /agent/:sessionID/{chat,render,action,events,abort}  事件流式接口
//   - /status  AI 状态（可用性 + 维度差异 + 会话回合快照）
//
// 知识库文档导入需 S3 原件存储与解析任务异步投递，故在父组挂 S3（原件读写）、对象缓存
// （原件加密上传/下载/清理）与管道投递客户端（asynq 解析任务与报告计划任务入队）中间件供
// 子路由共享。报告计划由分发器按 next_run_at 调度，视图层不再读写 etcd。
func RegisterRoutes(ctx *views.RegisterContext) {
	aiGroup := ctx.V1Group.Group(
		"/ai",
		ctx.Authenticator,     // 认证器注入
		ctx.ForceAuthenticate, // 强制JWT认证
		ctx.Policy,            // RBAC策略引擎
		ctx.Namespace,         // 命名空间隔离
		ctx.Authorize,         // 权限验证（api:{FullPath} + HTTP 方法动态授权）
		ctx.CoreDatabase,      // 核心库连接（知识库文档/会话持久化）
		ctx.FeatureRegistry,   // 功能开关注册表
		ctx.Ai,                // AI 助手组件（RAG 知识库 + Agent）
		ctx.S3,                // S3 客户端（知识库原件读写）
		ctx.ObjectCache,       // 对象缓存（原件加密上传/下载/清理）
		ctx.PipelineClient,    // 管道投递客户端（解析任务与报告计划任务入队）
	)

	// ========== 知识库文档：/api/v1/ai/doc ==========
	// 知识库文档 CRUD 与重新索引；文档为命名空间级资源，落库后经 Knowledge 切块向量化。
	docGroup := aiGroup.Group("/doc")
	// 知识库文档列表查询 - 支持分页、搜索、排序与创建/更新时间过滤。
	docGroup.POST("/list", KnowledgeDocumentListGet)
	// 知识库文档列表元信息
	views.RegisterListApi[*views.KnowledgeDocumentListGetSearchIn, *KnowledgeDocumentListOut](docGroup, "/list", "知识库文档列表查询 - 支持分页、搜索、排序与创建/更新时间过滤。",
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.knowledge.list",
			Domain:    "ai",
			Intent:    "查看知识库文档列表",
			Entities:  []string{"knowledge_documents"},
			Access:    capability.AccessRead,
			Invokable: true,
		}),
	)
	// 创建知识库文档 - 落库后切块向量化（AI 未启用时文档保留，可稍后重新索引）
	docGroup.POST("/create", KnowledgeDocumentCreate)
	// 创建知识库文档元信息
	views.RegisterApi(docGroup, "/create", "创建知识库文档 - 落库后切块向量化（AI 未启用时文档保留，可稍后重新索引）", views.WithMethod(http.MethodPost, views.WithBodyInput[KnowledgeDocumentCreateIn]()))
	// 导入知识库文档 - multipart 上传原件（流式加密存 S3）后投递异步解析任务
	docGroup.POST("/upload", KnowledgeDocumentUpload)
	// 导入知识库文档元信息（multipart 输入无法用结构体描述，仅登记请求方法与出参）
	views.RegisterApi(docGroup, "/upload", "导入知识库文档 - multipart 上传原件（流式加密存 S3）后投递异步解析任务", views.WithMethod(http.MethodPost, views.WithBodyOutput[KnowledgeDocumentUploadOut]()))
	// 按文档 ID 查询当前命名空间内某篇知识库文档的完整信息。
	docGroup.POST("/:id/get", KnowledgeDocumentDetailGet)
	// 知识库文档详情元信息
	views.RegisterApi(docGroup, "/:id/get", "按文档 ID 查询当前命名空间内某篇知识库文档的完整信息。",
		views.WithMethod(http.MethodPost, views.WithBodyOutput[KnowledgeDocumentDetailOut]()),
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.knowledge.detail",
			Domain:    "ai",
			Intent:    "查看知识库文档详情",
			Entities:  []string{"knowledge_documents"},
			Access:    capability.AccessRead,
			Invokable: true,
		}),
	)
	// 更新知识库文档 - 更新标题/正文并触发重新索引
	docGroup.POST("/:id/update", KnowledgeDocumentUpdate)
	// 更新知识库文档元信息
	views.RegisterApi(docGroup, "/:id/update", "更新知识库文档 - 更新标题/正文并触发重新索引", views.WithMethod(http.MethodPost, views.WithBodyInput[KnowledgeDocumentUpdateIn]()))
	// 批量删除知识库文档 - 删除文档及其分块索引
	docGroup.POST("/delete", KnowledgeDocumentBatchDelete)
	// 批量删除知识库文档元信息
	views.RegisterApi(docGroup, "/delete", "批量删除知识库文档 - 删除文档及其分块索引", views.WithMethod(http.MethodPost, views.WithBodyInput[KnowledgeDocumentBatchIn](), views.WithBodyOutput[views.BatchResultOut]()))
	// 重新索引知识库文档 - 按最新内容重建分块与向量索引
	docGroup.POST("/:id/reindex", KnowledgeDocumentReindex)
	// 重新索引知识库文档元信息
	views.RegisterApi(docGroup, "/:id/reindex", "重新索引知识库文档 - 按最新内容重建分块与向量索引", views.WithMethod(http.MethodPost, views.WithBodyOutput[KnowledgeDocumentReindexOut]()))
	// 知识库文档分块明细 - 聚合索引摘要（分块统计/切片参数/向量维度/索引新鲜度）与逐块明细
	docGroup.POST("/:id/chunks", KnowledgeDocumentChunksGet)
	// 知识库文档分块明细元信息
	views.RegisterApi(docGroup, "/:id/chunks", "知识库文档分块明细 - 聚合索引摘要（分块统计/切片参数/向量维度/索引新鲜度）与逐块明细",
		views.WithMethod(http.MethodPost, views.WithBodyOutput[KnowledgeDocumentChunksOut]()),
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.knowledge.chunks",
			Domain:    "ai",
			Intent:    "查看知识库文档的分块与索引摘要",
			Entities:  []string{"knowledge_documents", "knowledge_chunks"},
			Access:    capability.AccessRead,
			Invokable: true,
		}),
	)
	// 下载知识库文档原件 - 解密 S3 原件以附件流式返回（二进制流，不经统一 JSON 响应封装）
	docGroup.POST("/:id/download", KnowledgeDocumentDownload)
	// 下载知识库文档原件元信息（二进制流响应不适配结构体元信息，仅登记请求方法）
	views.RegisterApi(docGroup, "/:id/download", "下载知识库文档原件 - 解密 S3 原件以附件流式返回（二进制流，不经统一 JSON 响应封装）",
		views.WithMethod(http.MethodPost),
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.knowledge.download",
			Domain:    "ai",
			Intent:    "下载知识库文档原件（二进制流，仅参考）",
			Entities:  []string{"knowledge_documents"},
			Access:    capability.AccessRead,
			Invokable: false,
		}),
	)

	// ========== 系统命名空间知识库（只读）：/api/v1/ai/doc/system ==========
	// 系统命名空间承载代码声明的公开共享知识，对前端以只读方式开放；不提供任何写方法。
	systemGroup := docGroup.Group("/system")
	// 系统命名空间知识库文档列表查询 - 只读，数据范围恒为内置系统命名空间。
	systemGroup.POST("/list", KnowledgeSystemDocumentListGet)
	// 系统命名空间知识库文档列表元信息
	views.RegisterListApi[*views.KnowledgeDocumentListGetSearchIn, *KnowledgeDocumentListOut](systemGroup, "/list", "系统命名空间知识库文档列表查询 - 只读，数据范围恒为内置系统命名空间",
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.knowledge.system.list",
			Domain:    "ai",
			Intent:    "查看系统命名空间的共享知识文档",
			Entities:  []string{"knowledge_documents"},
			Access:    capability.AccessRead,
			Invokable: true,
		}),
	)
	// 系统命名空间知识库文档详情 - 只读。
	systemGroup.POST("/:id/get", KnowledgeSystemDocumentDetailGet)
	// 系统命名空间知识库文档详情元信息
	views.RegisterApi(systemGroup, "/:id/get", "系统命名空间知识库文档详情 - 只读",
		views.WithMethod(http.MethodPost, views.WithBodyOutput[KnowledgeDocumentDetailOut]()),
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.knowledge.system.detail",
			Domain:    "ai",
			Intent:    "查看系统知识文档详情",
			Entities:  []string{"knowledge_documents"},
			Access:    capability.AccessRead,
			Invokable: true,
		}),
	)
	// 系统命名空间知识库文档分块明细 - 只读。
	systemGroup.POST("/:id/chunks", KnowledgeSystemDocumentChunksGet)
	// 系统命名空间知识库文档分块明细元信息
	views.RegisterApi(systemGroup, "/:id/chunks", "系统命名空间知识库文档分块明细 - 只读",
		views.WithMethod(http.MethodPost, views.WithBodyOutput[KnowledgeDocumentChunksOut]()),
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.knowledge.system.chunks",
			Domain:    "ai",
			Intent:    "查看系统知识文档的分块与索引摘要",
			Entities:  []string{"knowledge_documents", "knowledge_chunks"},
			Access:    capability.AccessRead,
			Invokable: true,
		}),
	)

	// ========== 知识库检索试运行：/api/v1/ai/retrieval ==========
	// 知识库 RAG dry-run（SSE 流式）：混合检索 → 生成回答 → RAGAS 风格量化评估
	aiGroup.POST("/retrieval/dry-run", RetrievalDryRunStream)
	// 知识库检索试运行元信息（SSE 流式：仅注册请求体输入）
	views.RegisterApi(aiGroup, "/retrieval/dry-run", "知识库 RAG dry-run（SSE 流式）：混合检索 → 生成回答 → RAGAS 风格量化评估",
		views.WithMethod(http.MethodPost, views.WithBodyInput[DryRunIn]()),
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.retrieval.dry-run",
			Domain:    "ai",
			Intent:    "执行知识库检索试运行（SSE 流式，仅参考）",
			Entities:  []string{"knowledge_documents", "knowledge_chunks"},
			Access:    capability.AccessRead,
			Invokable: false,
		}),
	)
	// 知识库检索试运行参数默认值（来自行为参数注册表，供前端对话框预填）
	aiGroup.POST("/retrieval/options", RetrievalOptions)
	views.RegisterApi(aiGroup, "/retrieval/options", "知识库检索试运行参数默认值（来自行为参数注册表，供前端对话框预填）",
		views.WithMethod(http.MethodPost, views.WithBodyOutput[RetrievalOptionsOut]()),
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.retrieval.options",
			Domain:    "ai",
			Intent:    "查看知识库检索试运行的参数默认值",
			Entities:  []string{},
			Access:    capability.AccessRead,
			Invokable: true,
		}),
	)

	// ========== Agent 会话：/api/v1/ai/session ==========
	// AI 助手会话的创建/列表/详情/重命名/删除与消息查询；会话为命名空间级资源。
	sessionGroup := aiGroup.Group("/session")
	// 查询当前命名空间内的 AI 助手会话列表，按最近更新时间倒序。
	sessionGroup.POST("/list", AgentSessionListGet)
	views.RegisterApi(sessionGroup, "/list", "查询当前命名空间内的 AI 助手会话列表，按最近更新时间倒序。",
		views.WithMethod(http.MethodPost, views.WithBodyOutput[AgentSessionListOut]()),
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.session.list",
			Domain:    "ai",
			Intent:    "查看当前命名空间的 AI 会话列表",
			Entities:  []string{"agent_sessions"},
			Access:    capability.AccessRead,
			Invokable: true,
		}),
	)
	// 创建会话
	sessionGroup.POST("/create", AgentSessionCreate)
	// 创建会话元信息
	views.RegisterApi(sessionGroup, "/create", "创建会话", views.WithMethod(http.MethodPost, views.WithBodyInput[AgentSessionCreateIn]()))
	// 会话详情查询
	sessionGroup.POST("/:id/get", AgentSessionDetailGet)
	// 会话详情元信息
	views.RegisterApi(sessionGroup, "/:id/get", "会话详情查询",
		views.WithMethod(http.MethodPost, views.WithBodyOutput[AgentSessionItemOut]()),
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.session.detail",
			Domain:    "ai",
			Intent:    "查看 AI 会话详情",
			Entities:  []string{"agent_sessions"},
			Access:    capability.AccessRead,
			Invokable: true,
		}),
	)
	// 重命名会话
	sessionGroup.POST("/:id/rename", AgentSessionRename)
	// 重命名会话元信息
	views.RegisterApi(sessionGroup, "/:id/rename", "重命名会话", views.WithMethod(http.MethodPost, views.WithBodyInput[AgentSessionRenameIn]()))
	// 更新会话审核阀门 - 校验会话归属与阀门枚举合法性后落库
	sessionGroup.POST("/:id/valve", AgentSessionValveSet)
	// 更新会话审核阀门元信息
	views.RegisterApi(sessionGroup, "/:id/valve", "更新会话审核阀门", views.WithMethod(http.MethodPost, views.WithBodyInput[AgentSessionValveIn](), views.WithBodyOutput[AgentSessionItemOut]()))
	// 删除会话 - 消息、长期记忆与挂起运行现场在同一事务内级联删除
	sessionGroup.POST("/:id/delete", AgentSessionDelete)
	// 会话消息列表查询 - 按创建时间升序
	sessionGroup.POST("/:id/messages", AgentMessageListGet)
	// 会话消息列表元信息
	views.RegisterApi(sessionGroup, "/:id/messages", "会话消息列表查询 - 按创建时间升序",
		views.WithMethod(http.MethodPost, views.WithBodyOutput[AgentMessageListOut]()),
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.session.messages",
			Domain:    "ai",
			Intent:    "查看 AI 会话的消息记录",
			Entities:  []string{"agent_messages"},
			Access:    capability.AccessRead,
			Invokable: true,
		}),
	)
	// 从某一轮用户消息起截断会话消息 - 删除该轮提问及其之后的全部消息（回退 / 删除）
	sessionGroup.POST("/:id/messages/truncate", AgentMessageTruncate)
	// 截断会话消息元信息
	views.RegisterApi(sessionGroup, "/:id/messages/truncate", "从某一轮用户消息起截断会话消息 - 删除该轮提问及其之后的全部消息（回退 / 删除）", views.WithMethod(http.MethodPost, views.WithBodyInput[AgentMessageTruncateIn]()))
	// 以某一轮用户消息为结尾创建会话分支 - 把该轮及其之前的全部消息复制到新会话
	sessionGroup.POST("/:id/branch", AgentSessionBranch)
	// 会话分支元信息
	views.RegisterApi(sessionGroup, "/:id/branch", "以某一轮用户消息为结尾创建会话分支 - 把该轮及其之前的全部消息复制到新会话", views.WithMethod(http.MethodPost, views.WithBodyInput[AgentSessionBranchIn](), views.WithBodyOutput[AgentSessionItemOut]()))
	// 压缩会话上下文并抽取长期记忆（message.compress SSE）- 折叠未摘要消息进滚动摘要并按开关抽取长期记忆
	sessionGroup.POST("/:id/compact", AgentSessionCompact)
	// 压缩会话上下文元信息（message.compress SSE：仅注册请求体输入）
	views.RegisterApi(sessionGroup, "/:id/compact", "压缩会话上下文并抽取长期记忆（message.compress SSE 流式进度，可选 assistantMessageID 指定折叠边界）", views.WithMethod(http.MethodPost, views.WithBodyInput[AgentSessionCompactIn]()))

	// ========== 长期记忆：/api/v1/ai/memory ==========
	// 会话级长期记忆的管理接口（列表/详情/批量删除/启停）；记忆由 Agent 运行期按会话生成，
	// 不在管理端新增或修改。
	memoryGroup := aiGroup.Group("/memory")
	// 记忆列表查询 - 支持分页、keyword/agentKind/kind/status 与创建/更新时间过滤及字段投影
	memoryGroup.POST("/list", AgentMemoryListGet)
	// 记忆列表元信息
	views.RegisterListApi[*views.AgentMemoryListGetSearchIn, *AgentMemoryListOut](memoryGroup, "/list", "记忆列表查询 - 支持分页、keyword/agentKind/kind/status 与创建/更新时间过滤及字段投影",
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.memory.list",
			Domain:    "ai",
			Intent:    "查看长期记忆列表",
			Entities:  []string{"agent_memories"},
			Access:    capability.AccessRead,
			Invokable: true,
		}),
	)
	// 记忆详情查询
	memoryGroup.POST("/:id/get", AgentMemoryDetailGet)
	// 记忆详情元信息
	views.RegisterApi(memoryGroup, "/:id/get", "记忆详情查询",
		views.WithMethod(http.MethodPost, views.WithBodyOutput[AgentMemoryDetailOut]()),
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.memory.detail",
			Domain:    "ai",
			Intent:    "查看长期记忆详情",
			Entities:  []string{"agent_memories"},
			Access:    capability.AccessRead,
			Invokable: true,
		}),
	)
	// 批量删除记忆 - 软删除
	memoryGroup.POST("/delete", AgentMemoryBatchDelete)
	// 批量删除记忆元信息
	views.RegisterApi(memoryGroup, "/delete", "批量删除记忆 - 软删除", views.WithMethod(http.MethodPost, views.WithBodyInput[AgentMemoryBatchIn](), views.WithBodyOutput[views.BatchResultOut]()))
	// 启停记忆 - active/archived
	memoryGroup.POST("/:id/status", AgentMemorySetStatus)
	// 启停记忆元信息
	views.RegisterApi(memoryGroup, "/:id/status", "启停记忆 - active/archived", views.WithMethod(http.MethodPost, views.WithBodyInput[AgentMemoryStatusIn]()))

	// ========== 技能：/api/v1/ai/skill ==========
	// AI 技能的命名空间级管理接口（创建 / 列表 / 详情 / 更新 / 批量删除 / 启停）；技能正文加密落库，
	// 列表不返回正文，由 Agent 运行时按需装配。
	skillGroup := aiGroup.Group("/skill")
	// 技能列表查询 - 支持分页、keyword/enabled 与创建/更新时间过滤及字段投影
	skillGroup.POST("/list", AgentSkillListGet)
	// 技能列表元信息
	views.RegisterListApi[*views.AgentSkillListGetSearchIn, *AgentSkillListOut](skillGroup, "/list", "技能列表查询 - 支持分页、keyword/enabled 与创建/更新时间过滤及字段投影",
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.skill.list",
			Domain:    "ai",
			Intent:    "查看技能列表",
			Entities:  []string{"agent_skills"},
			Access:    capability.AccessRead,
			Invokable: true,
		}),
	)
	// 技能选择器选项 - 合并内置技能与当前命名空间已启用技能（不返回正文）
	skillGroup.POST("/options", AgentSkillOptionsGet)
	// 技能选择器选项元信息：只读合并结果，供 AI 助手在推荐或复用技能前确认可用技能；
	// 不返回技能正文，因此与技能列表同属可安全调用的只读能力。
	views.RegisterApi(skillGroup, "/options", "技能选择器选项 - 合并内置技能与当前命名空间已启用技能",
		views.WithMethod(http.MethodPost, views.WithBodyOutput[AgentSkillOptionListOut]()),
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.skill.options",
			Domain:    "ai",
			Intent:    "查看当前可用的技能选项（内置技能与已启用自定义技能）",
			Entities:  []string{"agent_skills"},
			Access:    capability.AccessRead,
			Invokable: true,
		}),
	)
	// 创建技能 - 名称在同命名空间内唯一，新建默认启用；周期能力、输入字段与固定参数由用户提交
	skillGroup.POST("/create", AgentSkillCreate)
	// 创建技能元信息：写入能力，供 AI 在「技能共创」流程中经 api_invoke 在用户审批后创建技能；
	// 名称在同命名空间内唯一，新建默认启用，周期能力、输入字段与固定参数随创建一次写入。
	views.RegisterApi(skillGroup, "/create", "创建技能 - 名称在同命名空间内唯一，新建默认启用，周期能力、输入字段与固定参数由用户提交；周期技能须含 nowTime 输入字段及 timeOffset / timeWindow 固定参数",
		views.WithMethod(http.MethodPost, views.WithBodyInput[AgentSkillCreateIn](), views.WithBodyOutput[AgentSkillDetailOut]()),
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.skill.create",
			Domain:    "ai",
			Intent:    "创建 AI 技能（写入，需用户在审批卡片上确认）",
			Entities:  []string{"agent_skills"},
			Access:    capability.AccessWrite,
			Invokable: true,
		}),
	)
	// 技能详情查询 - 含解密后的技能正文
	skillGroup.POST("/:id/get", AgentSkillDetailGet)
	// 技能详情元信息
	views.RegisterApi(skillGroup, "/:id/get", "技能详情查询 - 含解密后的技能正文",
		views.WithMethod(http.MethodPost, views.WithBodyOutput[AgentSkillDetailOut]()),
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.skill.detail",
			Domain:    "ai",
			Intent:    "查看技能详情",
			Entities:  []string{"agent_skills"},
			Access:    capability.AccessRead,
			Invokable: true,
		}),
	)
	// 更新技能 - 名称/描述/正文整体覆盖
	skillGroup.POST("/:id/update", AgentSkillUpdate)
	// 更新技能元信息（不开放给 api_invoke：AI 侧只允许创建，创建必须一次成功）
	views.RegisterApi(skillGroup, "/:id/update", "更新技能 - 名称/描述/正文与周期能力、输入字段、固定参数整体覆盖；周期技能须含 nowTime 输入字段及 timeOffset / timeWindow 固定参数",
		views.WithMethod(http.MethodPost, views.WithBodyInput[AgentSkillUpdateIn](), views.WithBodyOutput[AgentSkillDetailOut]()))
	// 批量删除技能 - 软删除
	skillGroup.POST("/delete", AgentSkillBatchDelete)
	// 批量删除技能元信息
	views.RegisterApi(skillGroup, "/delete", "批量删除技能 - 软删除",
		views.WithMethod(http.MethodPost, views.WithBodyInput[AgentSkillBatchIn](), views.WithBodyOutput[views.BatchResultOut]()))
	// 启停技能 - 停用的技能不参与运行时装配
	skillGroup.POST("/:id/status", AgentSkillSetEnabled)
	// 启停技能元信息
	views.RegisterApi(skillGroup, "/:id/status", "启停技能 - 停用的技能不参与运行时装配",
		views.WithMethod(http.MethodPost, views.WithBodyInput[AgentSkillStatusIn]()))

	// ========== 周期性报告：/api/v1/ai/periodic-report ==========
	// 周期性报告计划的命名空间级管理接口（技能选项 / 计划 CRUD / 启停 / 立即执行）与运行记录、
	// 关注关系、信息流查询；计划由 periodic-report:plan:dispatch 分发器按 next_run_at 定时投递。
	periodicReportGroup := aiGroup.Group("/periodic-report")
	// 可作为报告来源的技能选项 - 仅返回可周期执行且含时间窗口字段的已启用自定义技能
	periodicReportGroup.POST("/skill/options", PeriodicReportSkillOptionsGet)
	// 报告技能选项元信息
	views.RegisterApi(periodicReportGroup, "/skill/options", "可作为报告来源的技能选项 - 仅返回可周期执行且含时间窗口字段的已启用自定义技能",
		views.WithMethod(http.MethodPost, views.WithBodyOutput[PeriodicReportSkillOptionListOut]()),
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.periodicReport.skill.options",
			Domain:    "ai",
			Intent:    "查看可作为报告来源的技能选项",
			Entities:  []string{"agent_skills"},
			Access:    capability.AccessRead,
			Invokable: true,
		}),
	)
	// 报告计划列表查询 - 支持 keyword / enabled 与创建/更新时间过滤及分页
	periodicReportGroup.POST("/plan/list", PeriodicReportPlanListGet)
	// 报告计划列表元信息
	views.RegisterListApi[*views.PeriodicReportPlanListGetSearchIn, *PeriodicReportPlanListOut](periodicReportGroup, "/plan/list", "报告计划列表查询 - 支持 keyword / enabled 与创建/更新时间过滤及分页",
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.periodicReport.plan.list",
			Domain:    "ai",
			Intent:    "查看报告计划列表",
			Entities:  []string{"periodic_report_plans"},
			Access:    capability.AccessRead,
			Invokable: true,
		}),
	)
	// 创建报告计划 - 校验技能资格与 cron 后落库
	periodicReportGroup.POST("/plan/create", PeriodicReportPlanCreate)
	// 创建报告计划元信息：写入能力，供「周期报告技能共创」完成技能后经 api_invoke 在用户审批后
	// 把技能排入周期计划；未登记能力时模型在能力地图中看不到该入口，api_invoke 也无法调用。
	views.RegisterApi(periodicReportGroup, "/plan/create", "创建报告计划 - 校验技能资格与 cron 后落库",
		views.WithMethod(http.MethodPost, views.WithBodyInput[PeriodicReportPlanCreateIn](), views.WithBodyOutput[PeriodicReportPlanItemOut]()),
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.periodicReport.plan.create",
			Domain:    "ai",
			Intent:    "创建周期性报告计划（写入，需用户在审批卡片上确认）",
			Entities:  []string{"periodic_report_plans"},
			Access:    capability.AccessWrite,
			Invokable: true,
		}),
	)
	// 更新报告计划 - 可编辑字段整体覆盖，下次执行时间交由分发器重算
	periodicReportGroup.POST("/plan/:id/update", PeriodicReportPlanUpdate)
	// 更新报告计划元信息
	views.RegisterApi(periodicReportGroup, "/plan/:id/update", "更新报告计划 - 可编辑字段整体覆盖，下次执行时间交由分发器重算",
		views.WithMethod(http.MethodPost, views.WithBodyInput[PeriodicReportPlanUpdateIn](), views.WithBodyOutput[PeriodicReportPlanItemOut]()))
	// 启停报告计划 - 停用后不再按 cron 触发
	periodicReportGroup.POST("/plan/:id/toggle", PeriodicReportPlanToggle)
	// 启停报告计划元信息
	views.RegisterApi(periodicReportGroup, "/plan/:id/toggle", "启停报告计划 - 停用后不再按 cron 触发",
		views.WithMethod(http.MethodPost, views.WithBodyInput[PeriodicReportPlanToggleIn](), views.WithBodyOutput[PeriodicReportPlanItemOut]()))
	// 删除报告计划 - 软删除计划
	periodicReportGroup.POST("/plan/:id/delete", PeriodicReportPlanDelete)
	// 删除报告计划元信息
	views.RegisterApi(periodicReportGroup, "/plan/:id/delete", "删除报告计划 - 软删除计划",
		views.WithMethod(http.MethodPost))
	// 立即执行报告计划 - 跳过 cron 直接投递一条生成任务（已到期计划返回 400）
	periodicReportGroup.POST("/plan/:id/run", PeriodicReportPlanRun)
	// 立即执行报告计划元信息：写入能力，创建计划后由用户确认立即试用一次，运行产物按真实数据生成
	views.RegisterApi(periodicReportGroup, "/plan/:id/run", "立即执行报告计划 - 跳过 cron 直接投递一条生成任务（已到期计划返回 400）",
		views.WithMethod(http.MethodPost),
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.periodicReport.plan.run",
			Domain:    "ai",
			Intent:    "立即执行周期性报告计划并生成一条真实报告（写入，需用户在审批卡片上确认）",
			Entities:  []string{"periodic_report_plans", "periodic_report_runs"},
			Access:    capability.AccessWrite,
			Invokable: true,
		}),
	)
	// 报告运行记录列表查询 - 可按计划 / 状态过滤并分页
	periodicReportGroup.POST("/run/list", PeriodicReportRunListGet)
	// 报告运行记录列表元信息
	views.RegisterListApi[*views.PeriodicReportRunListGetSearchIn, *PeriodicReportRunListOut](periodicReportGroup, "/run/list", "报告运行记录列表查询 - 可按计划 / 状态过滤并分页",
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.periodicReport.run.list",
			Domain:    "ai",
			Intent:    "查看报告运行记录列表",
			Entities:  []string{"periodic_report_runs"},
			Access:    capability.AccessRead,
			Invokable: true,
		}),
	)
	// 报告运行详情查询 - 含报告正文（对象缺失返回 404）
	periodicReportGroup.POST("/run/:id/get", PeriodicReportRunDetailGet)
	// 报告运行详情元信息
	views.RegisterApi(periodicReportGroup, "/run/:id/get", "报告运行详情查询 - 含报告正文（对象缺失返回 404）",
		views.WithMethod(http.MethodPost, views.WithBodyOutput[PeriodicReportRunDetailOut]()),
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.periodicReport.run.detail",
			Domain:    "ai",
			Intent:    "查看报告运行详情与正文",
			Entities:  []string{"periodic_report_runs"},
			Access:    capability.AccessRead,
			Invokable: true,
		}),
	)
	// 删除报告运行记录 - 一并删除报告对象；运行中与已结束的记录均可删除
	periodicReportGroup.POST("/run/:id/delete", PeriodicReportRunDelete)
	// 删除报告运行记录元信息
	views.RegisterApi(periodicReportGroup, "/run/:id/delete", "删除报告运行记录 - 一并删除报告对象；运行中与已结束的记录均可删除",
		views.WithMethod(http.MethodPost))
	// 关注计划候选列表 - 分页返回可关注计划，并回填当前用户关注状态
	periodicReportGroup.POST("/watch/list", PeriodicReportWatchListGet)
	// 关注计划候选列表元信息
	views.RegisterApi(periodicReportGroup, "/watch/list", "关注计划候选列表 - 分页返回可关注计划，并回填当前用户关注状态",
		views.WithMethod(http.MethodPost, views.WithBodyInput[views.PeriodicReportWatchListGetSearchIn](), views.WithBodyOutput[PeriodicReportWatchListOut]()),
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.periodicReport.watch.list",
			Domain:    "ai",
			Intent:    "查看当前用户关注的报告计划",
			Entities:  []string{"periodic_report_plans"},
			Access:    capability.AccessRead,
			Invokable: true,
		}),
	)
	// 设置计划关注状态 - 幂等关注 / 取消关注
	periodicReportGroup.POST("/watch/set", PeriodicReportWatchSet)
	// 设置计划关注状态元信息
	views.RegisterApi(periodicReportGroup, "/watch/set", "设置计划关注状态 - 幂等关注 / 取消关注",
		views.WithMethod(http.MethodPost, views.WithBodyInput[PeriodicReportWatchSetIn]()))
	// 报告中心信息流查询 - 仅返回关注计划的成功运行记录
	periodicReportGroup.POST("/feed/list", PeriodicReportFeedListGet)
	// 报告中心信息流元信息
	views.RegisterListApi[*views.PeriodicReportFeedListGetSearchIn, *PeriodicReportFeedListOut](periodicReportGroup, "/feed/list", "报告中心信息流查询 - 仅返回关注计划的成功运行记录",
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.periodicReport.feed.list",
			Domain:    "ai",
			Intent:    "查看报告中心信息流",
			Entities:  []string{"periodic_report_runs"},
			Access:    capability.AccessRead,
			Invokable: true,
		}),
	)

	// ========== Agent 回合（领域事件）与状态：/api/v1/ai ==========
	// 所有服务端→前端输出均为领域事件，以 `id: <seq>` + `data: <json>` 的 SSE 行下发；运行与
	// 连接解耦，刷新后经 render 重建历史、经 events?after= 续接实时增量。
	// 同 path 的 GET/POST 元信息必须合并进单次 RegisterApi（其内部只注册一次 OPTIONS，重复注册同一
	// path 的 OPTIONS 会使 gin 抛出路由冲突 panic）。
	aiGroup.POST("/agent/:sessionID/chat", AgentChat)
	// 启动（或抢占）一轮对话元信息（SSE：仅注册请求体输入）
	views.RegisterApi(aiGroup, "/agent/:sessionID/chat", "启动或抢占一轮 AI 助手对话（SSE）", views.WithMethod(http.MethodPost, views.WithBodyInput[AgentChatIn]()))
	aiGroup.GET("/agent/:sessionID/render", AgentRender)
	// 历史回放元信息（SSE：回放历史快照与挂起交互后结束）
	views.RegisterApi(aiGroup, "/agent/:sessionID/render", "回放会话的历史快照与当前挂起交互（SSE）", views.WithMethod(http.MethodGet))
	aiGroup.POST("/agent/:sessionID/action", AgentAction)
	// 审批 / 问卷动作元信息（SSE：仅注册请求体输入）
	views.RegisterApi(aiGroup, "/agent/:sessionID/action", "提交审批 / 问卷动作并续跑（SSE）", views.WithMethod(http.MethodPost, views.WithBodyInput[contract.ActionRequest]()))
	aiGroup.POST("/agent/:sessionID/interaction/reopen", AgentInteractionReopen)
	// 重新审批 / 重新回答元信息（回退到该次交互的待处理状态，不重跑整轮回答）
	views.RegisterApi(aiGroup, "/agent/:sessionID/interaction/reopen", "回退到某次审批 / 问卷交互的待处理状态（可重新决策 / 作答，不重跑整轮回答）", views.WithMethod(http.MethodPost, views.WithBodyInput[AgentInteractionReopenIn]()))
	aiGroup.GET("/agent/:sessionID/events", AgentEvents)
	// 运行中重连订阅元信息（SSE：?after= 指定回放起点）
	views.RegisterApi(aiGroup, "/agent/:sessionID/events", "运行中重连订阅后续事件（SSE，?after= 指定回放起点）", views.WithMethod(http.MethodGet))
	aiGroup.POST("/agent/:sessionID/abort", AgentAbort)
	// 中止当前回合元信息
	views.RegisterApi(aiGroup, "/agent/:sessionID/abort", "中止当前回合（下发 turn.completed(cancelled) 并作废挂起现场）", views.WithMethod(http.MethodPost, views.WithBodyOutput[AgentCancelOut]()))
	// 提示词优化 - 把当前诉求改写为更清晰、更易被正确理解的提问（单轮、非流式）
	aiGroup.POST("/prompt/optimize", AgentPromptOptimize)
	// 提示词优化元信息
	views.RegisterApi(aiGroup, "/prompt/optimize", "优化用户提问的提示词 - 把当前诉求改写为更清晰、更易被正确理解的提问（单轮、非流式）", views.WithMethod(http.MethodPost, views.WithBodyInput[AgentPromptOptimizeIn](), views.WithBodyOutput[AgentPromptOptimizeOut]()))
	// 查询 AI 助手运行状态：是否启用、各档位绑定模型与可用性、生效参数覆盖数量，可选附指定会话的回合状态快照。
	aiGroup.POST("/status", AgentStatusGet)
	views.RegisterApi(aiGroup, "/status", "查询 AI 助手运行状态：是否启用、各档位绑定模型与可用性、生效参数覆盖数量，可选附指定会话的回合状态快照。",
		views.WithMethod(http.MethodPost, views.WithBodyInput[AgentStatusIn](), views.WithBodyOutput[AgentStatusOut]()),
		views.WithCapability(views.CapabilityMeta{
			ID:        "ai.status",
			Domain:    "ai",
			Intent:    "查看 AI 助手运行状态与档位可用性",
			Entities:  []string{},
			Access:    capability.AccessRead,
			Invokable: true,
		}),
	)
}
