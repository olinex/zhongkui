package ai

import (
	"net/http"

	"zhongkuisec.com/zhongkui/internal/manager/views"
)

// RegisterRoutes 注册 /api/v1/ai AI 助手（RAG + Agent）接口。
// 分组挂认证/授权/命名空间/核心库/功能开关/AI 组件/S3/对象缓存/管道投递客户端中间件；子组：
//   - /doc    知识库文档 CRUD/list/reindex/chunks + 导入上传/原件下载
//   - /session  Agent 会话 create/list/get/rename/valve/delete/messages
//   - /memory  长期记忆 create/list/get/update/delete/status
//   - /agent/:sessionID/stream  SSE 流式生成
//   - /agent/:sessionID/tool-result  回传客户端工具调用结果并续跑挂起运行（SSE）
//   - /status  AI 状态（可用性 + 维度差异）
//
// 知识库文档导入需 S3 原件存储与解析任务异步投递，故在父组挂 S3（原件读写）、对象缓存
// （原件加密上传/下载/清理）与管道投递客户端（asynq 解析任务入队）中间件供子路由共享。
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
		ctx.PipelineClient,    // 管道投递客户端（解析任务入队）
	)

	// ========== 知识库文档：/api/v1/ai/doc ==========
	// 知识库文档 CRUD 与重新索引；文档为命名空间级资源，落库后经 Knowledge 切块向量化。
	docGroup := aiGroup.Group("/doc")
	// 知识库文档列表查询 - 支持分页、搜索与排序。
	docGroup.POST("/list", KnowledgeDocumentListGet)
	// 知识库文档列表元信息
	views.RegisterListApi[*views.KnowledgeDocumentListGetSearchIn, *KnowledgeDocumentListOut](docGroup, "/list", "知识库文档列表查询 - 支持分页、搜索与排序。")
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
	views.RegisterApi(docGroup, "/:id/get", "按文档 ID 查询当前命名空间内某篇知识库文档的完整信息。", views.WithMethod(http.MethodPost, views.WithBodyOutput[KnowledgeDocumentDetailOut]()))
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
	views.RegisterApi(docGroup, "/:id/chunks", "知识库文档分块明细 - 聚合索引摘要（分块统计/切片参数/向量维度/索引新鲜度）与逐块明细", views.WithMethod(http.MethodPost, views.WithBodyOutput[KnowledgeDocumentChunksOut]()))
	// 下载知识库文档原件 - 解密 S3 原件以附件流式返回（二进制流，不经统一 JSON 响应封装）
	docGroup.POST("/:id/download", KnowledgeDocumentDownload)
	// 下载知识库文档原件元信息（二进制流响应不适配结构体元信息，仅登记请求方法）
	views.RegisterApi(docGroup, "/:id/download", "下载知识库文档原件 - 解密 S3 原件以附件流式返回（二进制流，不经统一 JSON 响应封装）", views.WithMethod(http.MethodPost))

	// ========== 系统命名空间知识库（只读）：/api/v1/ai/doc/system ==========
	// 系统命名空间承载代码声明的公开共享知识，对前端以只读方式开放；不提供任何写方法。
	systemGroup := docGroup.Group("/system")
	// 系统命名空间知识库文档列表查询 - 只读，数据范围恒为内置系统命名空间。
	systemGroup.POST("/list", KnowledgeSystemDocumentListGet)
	// 系统命名空间知识库文档列表元信息
	views.RegisterListApi[*views.KnowledgeDocumentListGetSearchIn, *KnowledgeDocumentListOut](systemGroup, "/list", "系统命名空间知识库文档列表查询 - 只读，数据范围恒为内置系统命名空间")
	// 系统命名空间知识库文档详情 - 只读。
	systemGroup.POST("/:id/get", KnowledgeSystemDocumentDetailGet)
	// 系统命名空间知识库文档详情元信息
	views.RegisterApi(systemGroup, "/:id/get", "系统命名空间知识库文档详情 - 只读", views.WithMethod(http.MethodPost, views.WithBodyOutput[KnowledgeDocumentDetailOut]()))
	// 系统命名空间知识库文档分块明细 - 只读。
	systemGroup.POST("/:id/chunks", KnowledgeSystemDocumentChunksGet)
	// 系统命名空间知识库文档分块明细元信息
	views.RegisterApi(systemGroup, "/:id/chunks", "系统命名空间知识库文档分块明细 - 只读", views.WithMethod(http.MethodPost, views.WithBodyOutput[KnowledgeDocumentChunksOut]()))

	// ========== 知识库检索试运行：/api/v1/ai/retrieval ==========
	// 知识库 RAG dry-run（SSE 流式）：混合检索 → 生成回答 → RAGAS 风格量化评估
	aiGroup.POST("/retrieval/dry-run", RetrievalDryRunStream)
	// 知识库检索试运行元信息（SSE 流式：仅注册请求体输入）
	views.RegisterApi(aiGroup, "/retrieval/dry-run", "知识库 RAG dry-run（SSE 流式）：混合检索 → 生成回答 → RAGAS 风格量化评估", views.WithMethod(http.MethodPost, views.WithBodyInput[DryRunIn]()))
	// 知识库检索试运行参数默认值（来自行为参数注册表，供前端对话框预填）
	aiGroup.POST("/retrieval/options", RetrievalOptions)

	// ========== Agent 会话：/api/v1/ai/session ==========
	// AI 助手会话的创建/列表/详情/重命名/删除与消息查询；会话为命名空间级资源。
	sessionGroup := aiGroup.Group("/session")
	// 查询当前命名空间内的 AI 助手会话列表，按最近更新时间倒序。
	sessionGroup.POST("/list", AgentSessionListGet)
	// 创建会话
	sessionGroup.POST("/create", AgentSessionCreate)
	// 创建会话元信息
	views.RegisterApi(sessionGroup, "/create", "创建会话", views.WithMethod(http.MethodPost, views.WithBodyInput[AgentSessionCreateIn]()))
	// 会话详情查询
	sessionGroup.POST("/:id/get", AgentSessionDetailGet)
	// 会话详情元信息
	views.RegisterApi(sessionGroup, "/:id/get", "会话详情查询", views.WithMethod(http.MethodPost, views.WithBodyOutput[AgentSessionItemOut]()))
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
	views.RegisterApi(sessionGroup, "/:id/messages", "会话消息列表查询 - 按创建时间升序", views.WithMethod(http.MethodPost, views.WithBodyOutput[AgentMessageListOut]()))
	// 从某一轮用户消息起截断会话消息 - 删除该轮提问及其之后的全部消息（回退 / 删除）
	sessionGroup.POST("/:id/messages/truncate", AgentMessageTruncate)
	// 截断会话消息元信息
	views.RegisterApi(sessionGroup, "/:id/messages/truncate", "从某一轮用户消息起截断会话消息 - 删除该轮提问及其之后的全部消息（回退 / 删除）", views.WithMethod(http.MethodPost, views.WithBodyInput[AgentMessageTruncateIn]()))
	// 以某一轮用户消息为结尾创建会话分支 - 把该轮及其之前的全部消息复制到新会话
	sessionGroup.POST("/:id/branch", AgentSessionBranch)
	// 会话分支元信息
	views.RegisterApi(sessionGroup, "/:id/branch", "以某一轮用户消息为结尾创建会话分支 - 把该轮及其之前的全部消息复制到新会话", views.WithMethod(http.MethodPost, views.WithBodyInput[AgentSessionBranchIn](), views.WithBodyOutput[AgentSessionItemOut]()))
	// 压缩会话上下文并抽取长期记忆（SSE 流式）- 折叠未摘要消息进滚动摘要并按开关抽取长期记忆
	sessionGroup.POST("/:id/compact", AgentSessionCompact)
	// 压缩会话上下文元信息（SSE 流式：仅注册请求体输入）
	views.RegisterApi(sessionGroup, "/:id/compact", "压缩会话上下文并抽取长期记忆（SSE 流式：依次下发 compress_start / compress_delta / compress_done，可选 userMessageIndex 指定折叠边界）", views.WithMethod(http.MethodPost, views.WithBodyInput[AgentSessionCompactIn]()))

	// ========== 长期记忆：/api/v1/ai/memory ==========
	// 会话级长期记忆的管理接口（列表/详情/批量删除/启停）；记忆由 Agent 运行期按会话生成，
	// 不在管理端新增或修改。
	memoryGroup := aiGroup.Group("/memory")
	// 记忆列表查询 - 支持分页、keyword/agentKind/kind/status 过滤与字段投影
	memoryGroup.POST("/list", AgentMemoryListGet)
	// 记忆列表元信息
	views.RegisterListApi[*views.AgentMemoryListGetSearchIn, *AgentMemoryListOut](memoryGroup, "/list", "记忆列表查询 - 支持分页、keyword/agentKind/kind/status 过滤与字段投影")
	// 记忆详情查询
	memoryGroup.POST("/:id/get", AgentMemoryDetailGet)
	// 记忆详情元信息
	views.RegisterApi(memoryGroup, "/:id/get", "记忆详情查询", views.WithMethod(http.MethodPost, views.WithBodyOutput[AgentMemoryDetailOut]()))
	// 批量删除记忆 - 软删除
	memoryGroup.POST("/delete", AgentMemoryBatchDelete)
	// 批量删除记忆元信息
	views.RegisterApi(memoryGroup, "/delete", "批量删除记忆 - 软删除", views.WithMethod(http.MethodPost, views.WithBodyInput[AgentMemoryBatchIn](), views.WithBodyOutput[views.BatchResultOut]()))
	// 启停记忆 - active/archived
	memoryGroup.POST("/:id/status", AgentMemorySetStatus)
	// 启停记忆元信息
	views.RegisterApi(memoryGroup, "/:id/status", "启停记忆 - active/archived", views.WithMethod(http.MethodPost, views.WithBodyInput[AgentMemoryStatusIn]()))

	// ========== Agent 流式生成与状态：/api/v1/ai ==========
	// SSE 流式问答（带会话持久化）、续播在途运行、回传客户端工具调用结果续跑挂起运行与 AI 状态查询。
	// 同 path 的 GET/POST 元信息必须合并进单次 RegisterApi（其内部只注册一次 OPTIONS，重复注册同一
	// path 的 OPTIONS 会使 gin 抛出路由冲突 panic）。
	aiGroup.POST("/agent/:sessionID/stream", AgentStream)
	// 续播某会话在途运行（SSE 流式，刷新 / 重连后经 ?after= 续接，运行已结束则回放后 EOF）
	aiGroup.GET("/agent/:sessionID/stream", AgentStreamResume)
	views.RegisterApi(aiGroup, "/agent/:sessionID/stream", "AI 助手 SSE 流式问答（POST 启动驻留运行并转发事件）：运行与连接解耦，刷新后经 GET 续播。",
		views.WithMethod(http.MethodPost, views.WithBodyInput[AgentStreamIn]()),
		views.WithMethod(http.MethodGet))
	// 回传客户端工具调用结果并续跑挂起运行（SSE 流式：继续输出 delta/stage/done 或再次挂起）
	aiGroup.POST("/agent/:sessionID/tool-result", AgentToolResult)
	// 回传工具调用结果元信息（SSE 流式：仅注册请求体输入）
	views.RegisterApi(aiGroup, "/agent/:sessionID/tool-result", "回传客户端工具调用结果并续跑挂起运行（SSE 流式：继续输出 delta/stage/done 或再次挂起）", views.WithMethod(http.MethodPost, views.WithBodyInput[AgentToolResultIn]()))
	// 回退（重新回答）一次已提交问卷 - 删除锚点之后的回复、重置锚点与现场，返回问卷载荷供预填重开
	aiGroup.POST("/agent/:sessionID/questionnaire/reanswer", AgentQuestionnaireReanswer)
	// 问卷回退元信息
	views.RegisterApi(aiGroup, "/agent/:sessionID/questionnaire/reanswer", "回退（重新回答）一次已提交问卷 - 删除该问卷锚点之后的回复、把锚点与本轮运行现场重置为待回答，返回问卷载荷（题目 + 上次答案）供前端预填重开", views.WithMethod(http.MethodPost, views.WithBodyInput[AgentQuestionnaireReanswerIn](), views.WithBodyOutput[AgentQuestionnaireOut]()))
	// 取消某会话在途运行（落 cancelled 终态，可回放）
	aiGroup.POST("/agent/:sessionID/run/cancel", AgentRunCancel)
	// 取消某会话在途运行元信息
	views.RegisterApi(aiGroup, "/agent/:sessionID/run/cancel", "取消某会话在途运行（落 cancelled 终态，可回放）", views.WithMethod(http.MethodPost, views.WithBodyOutput[AgentCancelOut]()))
	// 提示词优化 - 把当前诉求改写为更清晰、更易被正确理解的提问（单轮、非流式）
	aiGroup.POST("/prompt/optimize", AgentPromptOptimize)
	// 提示词优化元信息
	views.RegisterApi(aiGroup, "/prompt/optimize", "优化用户提问的提示词 - 把当前诉求改写为更清晰、更易被正确理解的提问（单轮、非流式）", views.WithMethod(http.MethodPost, views.WithBodyInput[AgentPromptOptimizeIn](), views.WithBodyOutput[AgentPromptOptimizeOut]()))
	// 查询 AI 助手运行状态：是否启用、各档位绑定模型与可用性、生效参数覆盖数量，可选附指定会话的在途运行快照。
	aiGroup.POST("/status", AgentStatusGet)
	views.RegisterApi(aiGroup, "/status", "查询 AI 助手运行状态：是否启用、各档位绑定模型与可用性、生效参数覆盖数量，可选附指定会话的在途运行快照。", views.WithMethod(http.MethodPost, views.WithBodyInput[AgentStatusIn](), views.WithBodyOutput[AgentStatusOut]()))
}
