package ai

import (
	"net/http"

	"zhongkui.com/zhongkui/internal/manager/views"
)

// RegisterRoutes 注册 /api/v1/ai AI 助手（RAG + Agent）接口。
// 分组挂认证/授权/命名空间/核心库/功能开关/AI 组件中间件；子组：
//   - /doc    知识库文档 CRUD/list/reindex
//   - /session  Agent 会话 create/list/get/rename/delete/messages
//   - /agent/:sessionID/stream  SSE 流式生成
//   - /status  AI 状态（可用性 + 维度差异）
func RegisterRoutes(ctx *views.RegisterContext) {
	aiGroup := ctx.V1Group.Group(
		"/ai",
		ctx.Authenticator,
		ctx.ForceAuthenticate,
		ctx.Policy,
		ctx.Namespace,
		ctx.Authorize,
		ctx.CoreDatabase,
		ctx.FeatureRegistry,
		ctx.Ai,
	)

	// ========== 知识库文档：/api/v1/ai/doc ==========
	// 知识库文档 CRUD 与重新索引；文档为命名空间级资源，落库后经 Knowledge 切块向量化。
	docGroup := aiGroup.Group("/doc")
	// 知识库文档列表查询 - 支持分页、搜索和排序
	docGroup.POST("/list", KnowledgeDocumentListGet)
	// 知识库文档列表元信息
	docGroup.OPTIONS("/list", views.ListMetaGinHandler[*views.KnowledgeDocumentListGetSearchIn, *KnowledgeDocumentListOut]())
	// 创建知识库文档 - 落库后切块向量化（AI 未启用时文档保留，可稍后重新索引）
	docGroup.POST("/create", KnowledgeDocumentCreate)
	// 创建知识库文档元信息
	docGroup.OPTIONS("/create", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[KnowledgeDocumentCreateIn]())))
	// 知识库文档详情查询 - 根据ID获取文档完整信息（标题/正文/分块数/摘要）
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
	// 会话列表查询 - 按更新时间倒序
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

	// ========== Agent 流式生成与状态：/api/v1/ai ==========
	// SSE 流式问答（带会话持久化）与 AI 状态查询。
	aiGroup.POST("/agent/:sessionID/stream", AgentStream)
	aiGroup.OPTIONS("/agent/:sessionID/stream", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[AgentStreamIn]())))
	aiGroup.POST("/status", AgentStatusGet)
	aiGroup.OPTIONS("/status", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[AgentStatusOut]())))
}
