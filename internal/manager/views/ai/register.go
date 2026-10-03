package ai

import (
	"net/http"

	"zhongkuisec.com/zhongkui/internal/manager/views"
)

// RegisterRoutes 注册 /api/v1/ai AI 助手（RAG + Agent）接口。
// 分组挂认证/授权/命名空间/核心库/功能开关/AI 组件/S3/对象缓存/管道投递客户端中间件；子组：
//   - /doc    知识库文档 CRUD/list/reindex/chunks + 导入上传/原件下载
//   - /session  Agent 会话 create/list/get/rename/delete/messages
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
	docGroup.OPTIONS("/list", views.ListMetaGinHandler[*views.KnowledgeDocumentListGetSearchIn, *KnowledgeDocumentListOut]())
	// 创建知识库文档 - 落库后切块向量化（AI 未启用时文档保留，可稍后重新索引）
	docGroup.POST("/create", KnowledgeDocumentCreate)
	// 创建知识库文档元信息
	docGroup.OPTIONS("/create", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[KnowledgeDocumentCreateIn]())))
	// 导入知识库文档 - multipart 上传原件（流式加密存 S3）后投递异步解析任务
	docGroup.POST("/upload", KnowledgeDocumentUpload)
	// 导入知识库文档元信息（multipart 输入无法用结构体描述，仅登记请求方法与出参）
	docGroup.OPTIONS("/upload", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[KnowledgeDocumentUploadOut]())))
	// 按文档 ID 查询当前命名空间内某篇知识库文档的完整信息。
	docGroup.POST("/:id/get", KnowledgeDocumentDetailGet)
	// 知识库文档详情元信息
	docGroup.OPTIONS("/:id/get", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[KnowledgeDocumentDetailOut]())))
	// 更新知识库文档 - 更新标题/正文并触发重新索引
	docGroup.POST("/:id/update", KnowledgeDocumentUpdate)
	// 更新知识库文档元信息
	docGroup.OPTIONS("/:id/update", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[KnowledgeDocumentUpdateIn]())))
	// 批量删除知识库文档 - 删除文档及其分块索引
	docGroup.POST("/delete", KnowledgeDocumentBatchDelete)
	// 批量删除知识库文档元信息
	docGroup.OPTIONS("/delete", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[KnowledgeDocumentBatchIn](), views.WithBodyOutput[views.BatchResultOut]())))
	// 重新索引知识库文档 - 按最新内容重建分块与向量索引
	docGroup.POST("/:id/reindex", KnowledgeDocumentReindex)
	// 重新索引知识库文档元信息
	docGroup.OPTIONS("/:id/reindex", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[KnowledgeDocumentReindexOut]())))
	// 知识库文档分块明细 - 聚合索引摘要（分块统计/切片参数/向量维度/索引新鲜度）与逐块明细
	docGroup.POST("/:id/chunks", KnowledgeDocumentChunksGet)
	// 知识库文档分块明细元信息
	docGroup.OPTIONS("/:id/chunks", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[KnowledgeDocumentChunksOut]())))
	// 下载知识库文档原件 - 解密 S3 原件以附件流式返回（二进制流，不经统一 JSON 响应封装）
	docGroup.POST("/:id/download", KnowledgeDocumentDownload)
	// 下载知识库文档原件元信息（二进制流响应不适配结构体元信息，仅登记请求方法）
	docGroup.OPTIONS("/:id/download", views.StructMetaGinHandler(views.WithMethod(http.MethodPost)))

	// ========== 知识库检索试运行：/api/v1/ai/retrieval ==========
	// 知识库 RAG dry-run（SSE 流式）：混合检索 → 生成回答 → RAGAS 风格量化评估
	aiGroup.POST("/retrieval/dry-run", RetrievalDryRunStream)
	// 知识库检索试运行元信息（SSE 流式：仅注册请求体输入）
	aiGroup.OPTIONS("/retrieval/dry-run", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[DryRunIn]())))
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
	sessionGroup.OPTIONS("/create", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[AgentSessionCreateIn]())))
	// 会话详情查询
	sessionGroup.POST("/:id/get", AgentSessionDetailGet)
	// 会话详情元信息
	sessionGroup.OPTIONS("/:id/get", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[AgentSessionItemOut]())))
	// 重命名会话
	sessionGroup.POST("/:id/rename", AgentSessionRename)
	// 重命名会话元信息
	sessionGroup.OPTIONS("/:id/rename", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[AgentSessionRenameIn]())))
	// 删除会话 - 消息在同一事务内级联删除
	sessionGroup.POST("/:id/delete", AgentSessionDelete)
	// 会话消息列表查询 - 按创建时间升序
	sessionGroup.POST("/:id/messages", AgentMessageListGet)
	// 会话消息列表元信息
	sessionGroup.OPTIONS("/:id/messages", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[AgentMessageListOut]())))
	// 压缩会话上下文并抽取长期记忆 - 折叠未摘要消息进滚动摘要并按开关抽取长期记忆
	sessionGroup.POST("/:id/compact", AgentSessionCompact)
	// 压缩会话上下文元信息
	sessionGroup.OPTIONS("/:id/compact", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[AgentSessionCompactOut]())))

	// ========== 长期记忆：/api/v1/ai/memory ==========
	// 会话级长期记忆的管理接口（列表/详情/批量删除/启停）；记忆由 Agent 运行期按会话生成，
	// 不在管理端新增或修改。
	memoryGroup := aiGroup.Group("/memory")
	// 记忆列表查询 - 支持分页、keyword/agentKind/kind/status 过滤与字段投影
	memoryGroup.POST("/list", AgentMemoryListGet)
	// 记忆列表元信息
	memoryGroup.OPTIONS("/list", views.ListMetaGinHandler[*views.AgentMemoryListGetSearchIn, *AgentMemoryListOut]())
	// 记忆详情查询
	memoryGroup.POST("/:id/get", AgentMemoryDetailGet)
	// 记忆详情元信息
	memoryGroup.OPTIONS("/:id/get", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[AgentMemoryDetailOut]())))
	// 批量删除记忆 - 软删除
	memoryGroup.POST("/delete", AgentMemoryBatchDelete)
	// 批量删除记忆元信息
	memoryGroup.OPTIONS("/delete", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[AgentMemoryBatchIn](), views.WithBodyOutput[views.BatchResultOut]())))
	// 启停记忆 - active/archived
	memoryGroup.POST("/:id/status", AgentMemorySetStatus)
	// 启停记忆元信息
	memoryGroup.OPTIONS("/:id/status", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[AgentMemoryStatusIn]())))

	// ========== Agent 流式生成与状态：/api/v1/ai ==========
	// SSE 流式问答（带会话持久化）、回传客户端工具调用结果续跑挂起运行与 AI 状态查询。
	aiGroup.POST("/agent/:sessionID/stream", AgentStream)
	aiGroup.OPTIONS("/agent/:sessionID/stream", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[AgentStreamIn]())))
	// 回传客户端工具调用结果并续跑挂起运行（SSE 流式：继续输出 delta/stage/done 或再次挂起）
	aiGroup.POST("/agent/:sessionID/tool-result", AgentToolResult)
	// 回传工具调用结果元信息（SSE 流式：仅注册请求体输入）
	aiGroup.OPTIONS("/agent/:sessionID/tool-result", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[AgentToolResultIn]())))
	// 查询 AI 助手运行状态：是否启用、各档位绑定模型与可用性、生效参数覆盖数量。
	aiGroup.POST("/status", AgentStatusGet)
	aiGroup.OPTIONS("/status", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[AgentStatusOut]())))
}
