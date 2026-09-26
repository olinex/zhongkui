// Package llm 提供 AI 助手 admin 配置接口（/api/v1/admin/llm/*）的路由注册与处理器。
//
// 接口分五组，共同构成 AI 运行时的配置面：
//   - 模型注册表（model.*）：LLM 端点（OpenAI 兼容）的增删改查与连通性测试；
//     apiKey 明文提交、密文落库、密文回显（不提供明文回显）。
//   - 档位绑定（binding.*）：把注册表内的模型绑定到 flash / standard / advanced / embedding
//     四个槽位，槽位可用性以 AI 运行时快照为权威。
//   - Agent 参数覆盖（param.*）：按代码内参数注册表（唯一事实来源）暴露参数元信息与
//     默认值，并支持对 agent_params 表做覆盖写入与重置。
//   - MCP 服务器注册表（mcp.*）：远程 MCP 端点的增删改查、连通性测试与工具清单同步；
//     headers 明文提交、密文落库、密文回显。
//   - 工具注册表（tool.*）：工具清单查询、批量启用/停用与批量删除（内置工具不可删除）。
//
// 五组写路径在 DB 提交后统一执行 afterConfigWrite：先重建本进程运行时，再经 etcd 广播
// 变更事件通知其他 manager 实例失效本地快照。
package llm

import (
	// stdlib
	"net/http"

	// internal
	"zhongkui.com/zhongkui/internal/manager/views"
)

// RegisterRoutes 注册 /api/v1/admin/llm AI 助手 admin 配置接口。
// 认证 / RBAC / 核心库 / 命名空间 / etcd 等公共中间件挂在分组上继承；需 Ai 中间件提供
// AI 运行时（校验档位可用性、执行端点连通性测试）。
func RegisterRoutes(ctx *views.RegisterContext) {
	llmGroup := ctx.V1Group.Group(
		"/admin/llm",          // AI 助手 admin 配置接口路径前缀
		ctx.Authenticator,     // 认证器注入
		ctx.ForceAuthenticate, // 强制JWT认证
		ctx.Policy,            // RBAC策略引擎
		ctx.Namespace,         // 命名空间隔离
		ctx.Authorize,         // 权限验证（api:{FullPath} + HTTP 方法动态授权）
		ctx.CoreDatabase,      // 核心库（GORM）连接
		ctx.Etcd,              // etcd 客户端（写路径广播 AI 配置变更事件）
		ctx.Ai,                // AI 助手运行时（快照读取与端点连通性测试）
	)

	// ========== 模型注册表：/api/v1/admin/llm/model ==========
	// LLM 端点（OpenAI 兼容）注册表管理：增删改查与连通性测试。
	// 注册表为全局配置（不按命名空间隔离）；apiKey 明文提交、密文落库与回显。
	{
		// 模型列表查询 - 支持分页、排序与过滤（时间/名称/类型/启用状态），不返回凭据
		llmGroup.POST("/model/list", LlmModelListGet)
		// 模型列表自省信息 - 返回搜索选项、表格列与出参结构体定义
		llmGroup.OPTIONS("/model/list", views.ListMetaGinHandler[*views.LlmModelListGetSearchIn, *LlmModelListOut]())
		// 注册模型端点 - 校验名称唯一、类型合法与端点字段合法后落库
		llmGroup.POST("/model/create", LlmModelCreate)
		// 注册模型端点元信息
		llmGroup.OPTIONS("/model/create", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[LlmModelCreateIn]()),
		))
		// 模型详情查询 - 根据ID获取端点完整配置（apiKey 为密文）
		llmGroup.POST("/model/:id/get", LlmModelDetailGet)
		// 模型详情元信息
		llmGroup.OPTIONS("/model/:id/get", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyOutput[LlmModelDetailOut]()),
		))
		// 更新模型端点 - 类型不可变；apiKey 留空表示沿用原凭据
		llmGroup.POST("/model/:id/update", LlmModelUpdate)
		// 更新模型端点元信息
		llmGroup.OPTIONS("/model/:id/update", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[LlmModelUpdateIn]()),
		))
		// 删除模型 - 批量软删除；仍被档位绑定的模型拒绝删除
		llmGroup.POST("/model/delete", LlmModelBatchDelete)
		// 删除模型元信息
		llmGroup.OPTIONS("/model/delete", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[LlmModelBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 测试模型端点连通性 - 不落库；支持「已注册模型 id」或「完整端点配置」两种用法
		llmGroup.POST("/model/test", LlmModelTest)
		// 测试模型端点连通性元信息
		llmGroup.OPTIONS("/model/test", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[LlmModelTestIn](), views.WithBodyOutput[LlmModelTestOut]()),
		))
	}

	// ========== 档位绑定：/api/v1/admin/llm/binding ==========
	// 把注册表内的模型绑定到固定档位（flash / standard / advanced / embedding）。
	// 绑定需满足类型匹配（对话档位要求 chat 模型、向量化槽位要求 embedding 模型）且模型已启用。
	{
		// 档位绑定列表 - 返回四个槽位的绑定摘要、槽位可用性与 AI 能力整体可用性
		llmGroup.POST("/binding/list", LlmBindingListGet)
		// 档位绑定列表元信息
		llmGroup.OPTIONS("/binding/list", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyOutput[LlmBindingListOut]()),
		))
		// 绑定档位 - 校验模型存在、已启用且类型与档位匹配后写入（重复绑定覆盖为最新模型）
		llmGroup.POST("/binding/:slot/set", LlmBindingSet)
		// 绑定档位元信息
		llmGroup.OPTIONS("/binding/:slot/set", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[LlmBindingSetIn]()),
		))
		// 解绑档位 - 删除该槽位绑定记录（不存在时幂等成功）
		llmGroup.POST("/binding/:slot/unbind", LlmBindingUnbind)
		// 解绑档位元信息（无请求体与响应体）
		llmGroup.OPTIONS("/binding/:slot/unbind", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost),
		))
	}

	// ========== Agent 参数覆盖：/api/v1/admin/llm/param ==========
	// 参数元信息来自代码内注册表（唯一事实来源）；覆盖值落 agent_params 表，空值等价于重置。
	{
		// 参数列表查询 - 返回元信息、默认值与当前生效值（label / comment 跟随请求语言）
		llmGroup.POST("/param/list", LlmParamListGet)
		// 参数列表元信息
		llmGroup.OPTIONS("/param/list", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyOutput[LlmParamListOut]()),
		))
		// 覆盖参数 - 批量提交 {key, value}；键必须已声明、值须符合声明类型
		llmGroup.POST("/param/set", LlmParamSet)
		// 覆盖参数元信息
		llmGroup.OPTIONS("/param/set", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[LlmParamSetIn]()),
		))
		// 重置参数 - 按分组（优先）或按键列表删除覆盖记录
		llmGroup.POST("/param/reset", LlmParamReset)
		// 重置参数元信息
		llmGroup.OPTIONS("/param/reset", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[LlmParamResetIn]()),
		))
	}

	// ========== MCP 服务器注册表：/api/v1/admin/llm/mcp ==========
	// 远程 MCP 端点管理：增删改查、连通性测试与工具清单同步。
	// 注册表为全局配置（不按命名空间隔离）；headers 明文提交、密文落库与回显。
	{
		// MCP 服务器列表查询 - 支持分页、排序与过滤（时间/名称/传输方式/启用状态），headers 为密文
		llmGroup.POST("/mcp/list", McpServerListGet)
		// MCP 服务器列表自省信息 - 返回搜索选项、表格列与出参结构体定义
		llmGroup.OPTIONS("/mcp/list", views.ListMetaGinHandler[*views.McpServerListGetSearchIn, *McpServerListOut]())
		// 注册 MCP 服务器 - 校验名称唯一、传输方式合法与端点字段合法后落库
		llmGroup.POST("/mcp/create", McpServerCreate)
		// 注册 MCP 服务器元信息
		llmGroup.OPTIONS("/mcp/create", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[McpServerCreateIn]()),
		))
		// MCP 服务器详情查询 - 根据ID获取端点完整配置（headers 为密文）
		llmGroup.POST("/mcp/:id/get", McpServerDetailGet)
		// MCP 服务器详情元信息
		llmGroup.OPTIONS("/mcp/:id/get", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyOutput[McpServerDetailOut]()),
		))
		// 更新 MCP 服务器 - 传输方式可改；headers 留空表示沿用原凭据
		llmGroup.POST("/mcp/:id/update", McpServerUpdate)
		// 更新 MCP 服务器元信息
		llmGroup.OPTIONS("/mcp/:id/update", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[McpServerUpdateIn]()),
		))
		// 删除 MCP 服务器 - 批量软删除，并在事务内一并软删其同步出的工具行
		llmGroup.POST("/mcp/delete", McpServerBatchDelete)
		// 删除 MCP 服务器元信息
		llmGroup.OPTIONS("/mcp/delete", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[McpServerBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 测试 MCP 服务器连通性 - 不落库；返回远端 tools/list 发现的工具数量与名称清单
		llmGroup.POST("/mcp/test", McpServerTest)
		// 测试 MCP 服务器连通性元信息
		llmGroup.OPTIONS("/mcp/test", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[McpServerTestIn](), views.WithBodyOutput[McpServerTestOut]()),
		))
		// 同步工具清单 - 把远端 tools/list 对账进工具注册表（事务）
		llmGroup.POST("/mcp/:id/sync", McpServerSync)
		// 同步工具清单元信息 - 返回新增/刷新/清理的工具行数
		llmGroup.OPTIONS("/mcp/:id/sync", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyOutput[McpServerSyncOut]()),
		))
	}

	// ========== 工具注册表：/api/v1/admin/llm/tool ==========
	// 工具清单查询与批量操作；内置工具由代码内注册表声明，接口不得删除。
	{
		// 工具列表查询 - 支持分页、排序与过滤（时间/名称/来源/启用状态）
		llmGroup.POST("/tool/list", LlmToolListGet)
		// 工具列表自省信息 - 返回搜索选项、表格列与出参结构体定义
		llmGroup.OPTIONS("/tool/list", views.ListMetaGinHandler[*views.LlmToolListGetSearchIn, *LlmToolListOut]())
		// 批量启用/停用工具 - 按 id 统一写入目标启用状态
		llmGroup.POST("/tool/set-enabled", LlmToolSetEnabled)
		// 批量启用/停用工具元信息
		llmGroup.OPTIONS("/tool/set-enabled", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[LlmToolSetEnabledIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 批量删除工具 - 批内含内置工具则整批拒绝
		llmGroup.POST("/tool/delete", LlmToolBatchDelete)
		// 批量删除工具元信息
		llmGroup.OPTIONS("/tool/delete", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[LlmToolBatchIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
	}
}
