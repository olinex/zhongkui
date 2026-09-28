// Package core 提供平台核心领域的持久化模型。本文件定义 LLM 模型注册表（LlmModel）、
// 档位绑定（LlmBinding）、Agent 参数（AgentParam）、MCP 服务器注册表（McpServer）与
// 工具注册表（LlmTool）五个模型：模型端点由前端动态注册（APIKey 以密文落库），档位绑定
// 把 flash / standard / advanced / embedding 四个固定槽位指向注册表中的一条模型记录，
// 参数表同时持有出厂值（default_value，由 migrate fixture 注入）与当前生效值（value），
// 参数类值的唯一来源是数据库而非代码；MCP 服务器注册表登记远程 MCP 端点（headers 以密文
// 落库，首版仅支持 streamable_http / sse 两种远程传输），工具注册表登记 Agent 可调用的
// 工具（内置工具由代码内注册表声明、MCP 工具由所属服务器同步而来），二者共同决定运行期
// 可装配的工具集合。五者均为全局配置，不按命名空间隔离。
//
// Package core provides the persistence models of the platform core domain. This file
// defines the LLM model registry (LlmModel), the slot binding (LlmBinding), the Agent
// parameter (AgentParam), the MCP server registry (McpServer) and the tool registry
// (LlmTool): model endpoints are registered dynamically by the frontend (API keys persisted
// encrypted at rest), slot bindings point the four fixed slots flash / standard / advanced /
// embedding to one record of the registry, and the parameter table carries both the factory
// value (default_value, injected by the migrate fixtures) and the effective value (value),
// making the database rather than the code the single source of parameter values; the MCP
// server registry records remote MCP endpoints (headers persisted encrypted at rest, only the
// streamable_http / sse remote transports are supported in the first version) and the tool
// registry records the tools callable by the Agent (builtin tools declared by the in-code
// registry, MCP tools synchronized from their server), together determining the tool set
// assembled at runtime. All five are global configuration, not namespace scoped.
package core

import (
	// third-party
	"gorm.io/gorm"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/llm"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// LlmModel 表示一个注册到平台的 LLM 模型端点（chat 或 embedding），是 LLM 配置的
// 唯一事实来源（不再来自配置文件）。Name 为全局唯一的本地标识，供档位绑定与运维
// 识别使用；Kind 区分为对话模型（chat）与向量模型（embedding）。APIKey 以加密形式
// 落库（crypto serializer，BYTEA 列），明文经 APIKeyData() 获取。对应表为 llm_models。
// LlmModel represents an LLM model endpoint (chat or embedding) registered on the
// platform, the single source of truth of the LLM configuration (no longer read from
// the config file). Name is the globally unique local identity used by slot bindings
// and operators; Kind distinguishes chat models from embedding models. APIKey is
// persisted encrypted at rest via the crypto serializer (BYTEA column), with plaintext
// obtained via APIKeyData(). The table is llm_models.
type LlmModel struct {
	gorm.Model // 内嵌 gorm.Model，包含 ID、CreatedAt、UpdatedAt、DeletedAt 字段

	// Name 模型的本地标识，全局唯一（uq_llm_models_name）
	// Name the local identity of the model, globally unique (uq_llm_models_name)
	Name string `gorm:"type:VARCHAR(64);uniqueIndex:uq_llm_models_name"`
	// Kind 模型类型：chat（对话）| embedding（向量化），取值见视图层 llmModelKind 枚举类
	// Kind the kind of the model: chat | embedding, valued from the llmModelKind enum class
	Kind llm.ModelKind `gorm:"type:VARCHAR(16);index:idx_llm_models_kind"`
	// BaseURL 模型端点的基地址（OpenAI 兼容 API），如 https://api.openai.com/v1
	// BaseURL the base address of the model endpoint (OpenAI compatible API),
	// e.g. https://api.openai.com/v1
	BaseURL string `gorm:"type:VARCHAR(512)"`
	// APIKey 访问模型端点的凭据，以加密形式持久化（crypto serializer 密文入库，BYTEA 列）；
	// 明文经 APIKeyData() 获取
	// APIKey the credential used to access the model endpoint, persisted encrypted at
	// rest via the crypto serializer (BYTEA column), plaintext obtained via APIKeyData()
	APIKey *serializer.Crypto `json:"apiKey" gorm:"column:api_key;type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=7001,length=32"`
	// ModelName 供应商侧的模型名，如 gpt-4o-mini、text-embedding-3-small；
	// 列名保持 model，Go 字段名取 ModelName 是因为内嵌的 gorm.Model 已占用 Model 标识符
	// ModelName the vendor-side model name, e.g. gpt-4o-mini, text-embedding-3-small;
	// the column stays model, the Go field is named ModelName because the embedded
	// gorm.Model already occupies the Model identifier
	ModelName string `gorm:"column:model;type:VARCHAR(128)"`
	// TimeoutSeconds 单次请求的超时秒数，0 表示使用调用方默认值
	// TimeoutSeconds the timeout in seconds of a single request, 0 means the caller default
	TimeoutSeconds int `gorm:"type:INTEGER"`
	// Temperature 采样温度，取值范围 [0, 2]，仅 chat 类模型有意义
	// Temperature the sampling temperature within [0, 2], only meaningful for chat models
	Temperature float64 `gorm:"type:DOUBLE PRECISION"`
	// MaxTokens 单次生成的最大 token 数，仅 chat 类模型有意义
	// MaxTokens the maximum number of tokens per generation, only meaningful for chat models
	MaxTokens int `gorm:"type:INTEGER"`
	// Dimensions 向量维度，仅 embedding 类模型有意义，是后端探测后写入的只读缓存：该值
	// 不再由前端提交（前端的向量维度输入已移除），而是后端在绑定 embedding 槽位（或修改
	// 已绑定模型的端点）时经真实探测写入，恒等于 knowledge_chunks.embedding 列的 pgvector
	// 维度，供绑定兼容性校验与状态展示使用
	// Dimensions the vector dimensions, meaningful for embedding models only; it is a
	// read-only cache written by the backend after a real probe. It is no longer submitted
	// by the frontend (the frontend dimension input has been removed): the backend writes it
	// when binding the embedding slot (or when the endpoint of a bound model changes), and it
	// always equals the pgvector dimension of the knowledge_chunks.embedding column, used by
	// the binding compatibility check and the status display
	Dimensions int `gorm:"type:INTEGER"`
	// Enabled 是否启用，禁用的模型不可绑定到档位，也不参与可用性判定
	// Enabled whether the model is enabled; disabled models cannot be bound to a slot
	// and do not take part in the availability check
	Enabled bool `gorm:"type:BOOLEAN;default:true"`
	// Description 模型描述，说明用途
	// Description the description of the model
	Description string `gorm:"type:VARCHAR(512)"`
}

// APIKeyData 返回模型访问凭据的明文字节，nil 安全（APIKey 为空或 nil 时返回 nil）。
// APIKeyData returns the plaintext bytes of the model credential, nil-safe (nil when
// APIKey is nil or empty).
func (l *LlmModel) APIKeyData() []byte {
	if l == nil || l.APIKey == nil {
		return nil
	}
	return l.APIKey.Value()
}

// LlmBinding 表示一个档位到模型注册表记录的绑定关系，每个槽位至多一条记录
// （slot 唯一）。不同槽位允许复用同一条模型记录；被绑定的模型不允许删除
// （外键 ON DELETE RESTRICT）。对应表为 llm_bindings。
// LlmBinding represents the binding between one slot and one record of the model
// registry, at most one record per slot (slot is unique). Different slots may reuse
// the same model record; a bound model cannot be deleted (foreign key ON DELETE
// RESTRICT). The table is llm_bindings.
type LlmBinding struct {
	database.BaseModel // 内嵌基础模型，包含 ID、CreatedAt、UpdatedAt 字段（表无 deleted_at 列）

	// Slot 档位槽位：flash | standard | advanced | embedding，全局唯一（uq_llm_bindings_slot）
	// Slot the slot: flash | standard | advanced | embedding, globally unique (uq_llm_bindings_slot)
	Slot llm.Slot `gorm:"type:VARCHAR(16);uniqueIndex:uq_llm_bindings_slot"`
	// ModelID 绑定的模型记录 ID（llm_models.id）
	// ModelID the id of the bound model record (llm_models.id)
	ModelID uint `gorm:"type:BIGINT;index:idx_llm_bindings_model_id"`
	// Model 绑定的模型实体，用于 GORM 关联（foreignKey:ModelID;references:ID；删除受限）
	// Model the bound model entity, for GORM association
	// (foreignKey:ModelID;references:ID;OnDelete:RESTRICT)
	Model LlmModel `gorm:"foreignKey:ModelID;references:ID;constraint:OnDelete:RESTRICT"`
}

// AgentParam 表示一条 Agent 行为参数记录：出厂值（DefaultValue）由 migrate fixture 注入、
// 只读，当前生效值（Value）可由管理接口或直接改库调整。表内每个注册表键恒有且仅有一条
// 记录，参数键、类型、分组与说明由代码内参数注册表声明（注册表不再持有任何出厂值）。
// 对应表为 agent_params。
// AgentParam represents one Agent behavior parameter record: the factory value
// (DefaultValue) is injected by the migrate fixtures and read-only, while the effective
// value (Value) can be adjusted through the admin API or by writing the database directly.
// Every registry key always has exactly one record; the keys, kinds, groups and comments are
// declared by the in-code parameter registry (which holds no factory value any more). The
// table is agent_params.
type AgentParam struct {
	database.BaseModel // 内嵌基础模型，包含 ID、CreatedAt、UpdatedAt 字段（表无 deleted_at 列）

	// Key 参数键（点号分层，如 agent.systemPrompt、retrieval.topK），全局唯一
	// （uq_agent_params_key）
	// Key the parameter key (dot-separated, e.g. agent.systemPrompt, retrieval.topK),
	// globally unique (uq_agent_params_key)
	Key string `gorm:"type:VARCHAR(128);uniqueIndex:uq_agent_params_key"`
	// Value 参数当前生效值，按注册表声明的类型解释（文本形式存储）
	// Value the effective value of the parameter, interpreted according to the kind
	// declared in the registry (stored as text)
	Value string `gorm:"type:TEXT"`
	// DefaultValue 参数出厂值（与种子化时的 Value 相同），只由 migrate fixture 维护，
	// 任何接口路径都不得写入；「恢复出厂值」即把 Value 写回本列
	// DefaultValue the factory value of the parameter (identical to the seeded Value); it is
	// maintained by the migrate fixtures only and never written through any API path.
	// "Restore the factory value" means writing Value back to this column
	DefaultValue string `gorm:"column:default_value;type:TEXT"`
}

// McpServer 表示一个注册到平台的远程 MCP（Model Context Protocol）端点，是 Agent 可调用
// 的 MCP 工具的来源。Name 为全局唯一的本地标识（uq_mcp_servers_name）。Transport 决定远端
// 连接方式，首版仅支持远程传输 streamable_http / sse；Headers 承载 Authorization 等请求
// 头，是一个 JSON 对象字符串，以加密形式落库（crypto serializer，BYTEA 列），明文经
// HeadersData() 获取。本表有意不出现 command / args / env 列——首版不支持 stdio 本地
// 进程，这是从 schema 层杜绝「管理员配置任意本地命令执行」。对应表为 mcp_servers，属全局
// 配置，不按命名空间隔离。
// McpServer represents a remote MCP (Model Context Protocol) endpoint registered on the
// platform, the source of the MCP tools callable by the Agent. Name is the globally unique
// local identity (uq_mcp_servers_name). Transport decides how the remote is reached, and the
// first version supports only the remote transports streamable_http / sse; Headers carries
// request headers such as Authorization as a JSON object string, persisted encrypted at rest
// via the crypto serializer (BYTEA column), plaintext obtained via HeadersData(). This table
// deliberately has no command / args / env column — stdio local processes are not supported in
// the first version, which rules out "an operator configuring arbitrary local command
// execution" at the schema level. The table is mcp_servers and is global configuration, not
// namespace scoped.
type McpServer struct {
	gorm.Model // 内嵌 gorm.Model，包含 ID、CreatedAt、UpdatedAt、DeletedAt 字段

	// Name MCP 服务器的本地标识，全局唯一（uq_mcp_servers_name）
	// Name the local identity of the MCP server, globally unique (uq_mcp_servers_name)
	Name string `gorm:"type:VARCHAR(64);uniqueIndex:uq_mcp_servers_name"`
	// Transport 远端传输方式：streamable_http | sse，取值见 mcp.Transport（枚举类 mcpTransport，
	// 声明于 internal/ai/tools/mcp）
	// Transport the remote transport: streamable_http | sse, valued from mcp.Transport (the
	// mcpTransport enum class, declared in internal/ai/tools/mcp)
	Transport string `gorm:"type:VARCHAR(32);index:idx_mcp_servers_transport"`
	// URL MCP 端点的地址（http/https）
	// URL the address of the MCP endpoint (http/https)
	URL string `gorm:"column:url;type:VARCHAR(512)"`
	// Headers 请求头（JSON 对象字符串，承载 Authorization 等凭据），以加密形式持久化
	// （crypto serializer 密文入库，BYTEA 列）；明文经 HeadersData() 获取
	// Headers the request headers (a JSON object string carrying credentials such as
	// Authorization), persisted encrypted at rest via the crypto serializer (BYTEA column),
	// plaintext obtained via HeadersData()
	Headers *serializer.Crypto `json:"headers" gorm:"column:headers;type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=7002,length=32"`
	// TimeoutSeconds 单次请求的超时秒数，0 表示使用调用方默认值
	// TimeoutSeconds the timeout in seconds of a single request, 0 means the caller default
	TimeoutSeconds int `gorm:"type:INTEGER"`
	// Enabled 是否启用，禁用的服务器及其全部工具不参与装配
	// Enabled whether the server is enabled; a disabled server and all its tools are not assembled
	Enabled bool `gorm:"type:BOOLEAN;default:true"`
	// Description 服务器描述，说明用途
	// Description the description of the server
	Description string `gorm:"type:VARCHAR(512)"`
}

// HeadersData 返回请求头的明文字节，nil 安全（Headers 为空或 nil 时返回 nil）。
// HeadersData returns the plaintext bytes of the request headers, nil-safe (nil when Headers
// is nil or empty).
func (m *McpServer) HeadersData() []byte {
	if m == nil || m.Headers == nil {
		return nil
	}
	return m.Headers.Value()
}

// LlmTool 表示一条注册到平台的工具记录，是 Agent 可调用的工具的唯一事实来源。Name 为
// Agent 面向的工具名，全局唯一（uq_llm_tools_name）。Source 取 builtin / mcp：内置工具的
// 描述文案来自 i18n（Description 行为空），其入参 schema 由代码内注册表声明；MCP 工具的
// 入参 schema 实时取自所属服务器的 tools/list，Description 仅作列表展示缓存。一致性约束：
// source='mcp' 当且仅当 McpServerID IS NOT NULL，由视图层与服务层校验；工具真正的入参
// schema 不落库，运行时按来源实时获取。对应表为 llm_tools，属全局配置，不按命名空间隔离。
// LlmTool represents one tool record registered on the platform, the single source of truth of
// the tools callable by the Agent. Name is the Agent-facing tool name, globally unique
// (uq_llm_tools_name). Source is builtin / mcp: builtin tools take their description text from
// i18n (their Description row is empty) and their input schema from the in-code registry, while
// MCP tools take their input schema live from the owning server's tools/list and use
// Description merely as a listing cache. Consistency constraint: source='mcp' if and only if
// McpServerID IS NOT NULL, validated by the view and service layers; the real input schema of a
// tool is never persisted and is fetched live per source at runtime. The table is llm_tools and
// is global configuration, not namespace scoped.
type LlmTool struct {
	gorm.Model // 内嵌 gorm.Model，包含 ID、CreatedAt、UpdatedAt、DeletedAt 字段

	// Name Agent 面向的工具名，全局唯一（uq_llm_tools_name）
	// Name the Agent-facing tool name, globally unique (uq_llm_tools_name)
	Name string `gorm:"type:VARCHAR(128);uniqueIndex:uq_llm_tools_name"`
	// Source 工具来源：builtin | mcp，取值见 llm.ToolSource（枚举类 llmToolSource，声明于
	// internal/pkg/llm）
	// Source the tool source: builtin | mcp, valued from llm.ToolSource (the llmToolSource enum
	// class, declared in internal/pkg/llm)
	Source llm.ToolSource `gorm:"type:VARCHAR(16);index:idx_llm_tools_source"`
	// McpServerID 所属 MCP 服务器的 ID（llm_tools.mcp_server_id），可空：source='mcp' 时
	// 非空、source='builtin' 时为空
	// McpServerID the id of the owning MCP server (mcp_servers.id), nullable: non-null when
	// source='mcp' and null when source='builtin'
	McpServerID *uint `gorm:"type:BIGINT;index:idx_llm_tools_mcp_server_id"`
	// McpServer 所属 MCP 服务器实体，用于 GORM 关联（foreignKey:McpServerID;references:ID；
	// 级联删除）
	// McpServer the owning MCP server entity, for GORM association
	// (foreignKey:McpServerID;references:ID;OnDelete:CASCADE)
	McpServer *McpServer `gorm:"foreignKey:McpServerID;references:ID;constraint:OnDelete:CASCADE"`
	// RemoteName MCP 服务器侧的工具名；source='builtin' 时为空
	// RemoteName the tool name on the MCP server side; empty when source='builtin'
	RemoteName string `gorm:"type:VARCHAR(128)"`
	// Description 展示用描述缓存；source='builtin' 时为空，其文案取自 i18n
	// Description the description cache for listing; empty when source='builtin', whose text
	// comes from i18n
	Description string `gorm:"type:VARCHAR(512)"`
	// Enabled 是否启用，默认 false（安全默认，由管理员在工具面板显式启用）；禁用的工具不参与装配
	// Enabled whether the tool is enabled, default false (a safe default, explicitly enabled by
	// an operator in the tool panel); a disabled tool is not assembled
	Enabled bool `gorm:"type:BOOLEAN;default:false"`
}
