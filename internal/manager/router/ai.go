package router

import (
	"net/http"

	"zhongkui.com/zhongkui/internal/manager/views"
)

// registerAiRoutes 注册 /api/v1/ai AI 助手（RAG + Agent）接口。
// 分组挂认证/授权/命名空间/核心库/功能开关/AI 组件中间件；子组：
//   - /doc    知识库文档 CRUD/list/reindex
//   - /session  Agent 会话 create/list/get/rename/delete/messages
//   - /agent/:sessionID/stream  SSE 流式生成
//   - /status  AI 状态（可用性 + 维度差异）
func registerAiRoutes(ctx *RegisterContext) {
	aiGroup := ctx.v1Group.Group(
		"/ai",
		ctx.authenticatorMiddleware,
		ctx.forceAuthenticateMiddleware,
		ctx.policyMiddleware,
		ctx.namespaceMiddleware,
		ctx.authorizeMiddleware,
		ctx.coreDatabaseMiddleware,
		ctx.featureRegistryMiddleware,
		ctx.aiMiddleware,
	)

	// ========== 知识库文档：/api/v1/ai/doc ==========
	// 知识库文档 CRUD 与重新索引；文档为命名空间级资源，落库后经 Knowledge 切块向量化。
	docGroup := aiGroup.Group("/doc")
	// 知识库文档列表查询 - 支持分页、搜索和排序
	docGroup.POST("/list", views.KnowledgeDocumentListGet)
	// 知识库文档列表元信息
	docGroup.OPTIONS("/list", views.ListMetaGinHandler[*views.KnowledgeDocumentListGetSearchIn, *views.KnowledgeDocumentListOut]())
	// 创建知识库文档 - 落库后切块向量化（AI 未启用时文档保留，可稍后重新索引）
	docGroup.POST("/create", views.KnowledgeDocumentCreate)
	// 创建知识库文档元信息
	docGroup.OPTIONS("/create", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.KnowledgeDocumentCreateIn]())))
	// 知识库文档详情查询 - 根据ID获取文档完整信息（标题/正文/分块数/摘要）
	docGroup.POST("/:id/get", views.KnowledgeDocumentDetailGet)
	// 知识库文档详情元信息
	docGroup.OPTIONS("/:id/get", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.KnowledgeDocumentDetailOut]())))
	// 更新知识库文档 - 更新标题/正文并触发重新索引
	docGroup.POST("/:id/update", views.KnowledgeDocumentUpdate)
	// 更新知识库文档元信息
	docGroup.OPTIONS("/:id/update", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.KnowledgeDocumentUpdateIn]())))
	// 批量删除知识库文档 - 删除文档及其分块索引
	docGroup.POST("/delete", views.KnowledgeDocumentBatchDelete)
	// 批量删除知识库文档元信息
	docGroup.OPTIONS("/delete", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.KnowledgeDocumentBatchIn](), views.WithBodyOutput[views.BatchResultOut]())))
	// 重新索引知识库文档 - 按最新内容重建分块与向量索引
	docGroup.POST("/:id/reindex", views.KnowledgeDocumentReindex)
	// 重新索引知识库文档元信息
	docGroup.OPTIONS("/:id/reindex", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.KnowledgeDocumentReindexOut]())))

	// ========== Agent 会话：/api/v1/ai/session ==========
	// AI 助手会话的创建/列表/详情/重命名/删除与消息查询；会话为命名空间级资源。
	sessionGroup := aiGroup.Group("/session")
	// 会话列表查询 - 按更新时间倒序
	sessionGroup.POST("/list", views.AgentSessionListGet)
	// 创建会话
	sessionGroup.POST("/create", views.AgentSessionCreate)
	// 创建会话元信息
	sessionGroup.OPTIONS("/create", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.AgentSessionCreateIn]())))
	// 会话详情查询
	sessionGroup.POST("/:id/get", views.AgentSessionDetailGet)
	// 会话详情元信息
	sessionGroup.OPTIONS("/:id/get", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.AgentSessionItemOut]())))
	// 重命名会话
	sessionGroup.POST("/:id/rename", views.AgentSessionRename)
	// 重命名会话元信息
	sessionGroup.OPTIONS("/:id/rename", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.AgentSessionRenameIn]())))
	// 删除会话 - 消息在同一事务内级联删除
	sessionGroup.POST("/:id/delete", views.AgentSessionDelete)
	// 会话消息列表查询 - 按创建时间升序
	sessionGroup.POST("/:id/messages", views.AgentMessageListGet)
	// 会话消息列表元信息
	sessionGroup.OPTIONS("/:id/messages", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.AgentMessageListOut]())))

	// ========== Agent 流式生成与状态：/api/v1/ai ==========
	// SSE 流式问答（带会话持久化）与 AI 状态查询。
	aiGroup.POST("/agent/:sessionID/stream", views.AgentStream)
	aiGroup.OPTIONS("/agent/:sessionID/stream", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.AgentStreamIn]())))
	aiGroup.POST("/status", views.AgentStatusGet)
	aiGroup.OPTIONS("/status", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.AgentStatusOut]())))
}
