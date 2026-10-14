// 本文件定义 LLM 模型注册表（LlmModel）、档位绑定（LlmBinding）、Agent 参数（AgentParam）、
// MCP 服务器注册表（McpServer）与工具注册表（LlmTool）五个模型：模型端点由前端动态注册
// （APIKey 以密文落库），档位绑定把 flash / standard / advanced / embedding 四个固定槽位
// 指向注册表中的一条模型记录，参数表同时持有出厂值（default_value，由 migrate fixture 注入）
// 与当前生效值（value），参数类值的唯一来源是数据库而非代码；MCP 服务器注册表登记远程 MCP
// 端点（headers 以密文落库，首版仅支持 streamable_http / sse 两种远程传输），工具注册表登记
// Agent 可调用的工具（内置工具由代码内注册表声明、MCP 工具由所属服务器同步而来），二者共同
// 决定运行期可装配的工具集合。五者均为全局配置，不按命名空间隔离。
//
// 模型档位枚举（llmSlot）的取值空间 Slot / AllSlots 与本包同源（见 enum.go），其类名与
// search.toml 字段绑定的引用一致；LlmBinding.Slot 即以此为类型。包内文档注释见 document.go。
package ai

import (
	// 第三方
	"gorm.io/gorm"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// LlmModel 表示一个注册到平台的 LLM 模型端点（chat 或 embedding），是 LLM 配置的
// 唯一事实来源（不再来自配置文件）。Name 为全局唯一的本地标识，供档位绑定与运维
// 识别使用；Kind 区分为对话模型（chat）与向量模型（embedding）。APIKey 以加密形式
// 落库（crypto serializer，BYTEA 列），明文经 APIKeyData() 获取。对应表为 llm_models。
type LlmModel struct {
	gorm.Model // 内嵌 gorm.Model，包含 ID、CreatedAt、UpdatedAt、DeletedAt 字段

	// Name 模型的本地标识，全局唯一（uq_llm_models_name）
	Name string `gorm:"type:VARCHAR(64);uniqueIndex:uq_llm_models_name"`
	// Kind 模型类型：chat（对话）| embedding（向量化），取值见视图层 llmModelKind 枚举类
	Kind llm.ModelKind `gorm:"type:VARCHAR(16);index:idx_llm_models_kind"`
	// BaseURL 模型端点的基地址（OpenAI 兼容 API），如 https://api.openai.com/v1
	BaseURL string `gorm:"type:VARCHAR(512)"`
	// APIKey 访问模型端点的凭据，以加密形式持久化（crypto serializer 密文入库，BYTEA 列）；
	// 明文经 APIKeyData() 获取
	APIKey *serializer.Crypto `json:"apiKey" gorm:"column:api_key;type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=7001,length=32"`
	// ModelName 供应商侧的模型名，如 gpt-4o-mini、text-embedding-3-small；
	// 列名保持 model，Go 字段名取 ModelName 是因为内嵌的 gorm.Model 已占用 Model 标识符
	ModelName string `gorm:"column:model;type:VARCHAR(128)"`
	// TimeoutSeconds 单次请求的超时秒数，0 表示使用调用方默认值
	TimeoutSeconds int `gorm:"type:INTEGER"`
	// Temperature 采样温度，取值范围 [0, 2]，仅 chat 类模型有意义
	Temperature float64 `gorm:"type:DOUBLE PRECISION"`
	// MaxTokens 单次生成的最大 token 数，仅 chat 类模型有意义
	MaxTokens int `gorm:"type:INTEGER"`
	// ContextTokens 上下文窗口容量（token 数），仅 chat 类模型有意义：由用户创建模型时提供，
	// agent 使用该档位模型时据此计算上下文占用比例并判定是否自动压缩；0 表示未配置
	ContextTokens int `gorm:"type:INTEGER"`
	// Dimensions 向量维度，仅 embedding 类模型有意义，是后端探测后写入的只读缓存：该值
	// 不再由前端提交（前端的向量维度输入已移除），而是后端在绑定 embedding 槽位（或修改
	// 已绑定模型的端点）时经真实探测写入，恒等于 knowledge_chunks.embedding 列的 pgvector
	// 维度，供绑定兼容性校验与状态展示使用
	Dimensions int `gorm:"type:INTEGER"`
	// Enabled 是否启用，禁用的模型不可绑定到档位，也不参与可用性判定
	Enabled bool `gorm:"type:BOOLEAN;default:true"`
	// Description 模型描述，说明用途
	Description string `gorm:"type:VARCHAR(512)"`
	// ExtraBody 高级请求配置；为该模型每次对话请求自动合并的 JSON 对象字符串，
	// 承载供应商自定义字段（不同供应商字段格式各异、无法穷举，这是通用逃生通道）
	ExtraBody string `gorm:"column:extra_body;type:TEXT"`
	// ThinkingField 思考开关的请求字段名（如 enable_thinking）；仅 chat 类模型有意义
	ThinkingField string `gorm:"type:VARCHAR(64)"`
	// ThinkingOnValue 开启思考时写入 ThinkingField 的取值（JSON 文本，如 true）
	ThinkingOnValue string `gorm:"column:thinking_on_value;type:VARCHAR(256)"`
	// ThinkingOffValue 关闭思考时写入 ThinkingField 的取值（JSON 文本，如 false）；
	// 与 ThinkingField、ThinkingOnValue 三者齐备时该模型支持思考开关
	ThinkingOffValue string `gorm:"column:thinking_off_value;type:VARCHAR(256)"`
}

// APIKeyData 返回模型访问凭据的明文字节，nil 安全（APIKey 为空或 nil 时返回 nil）。
func (l *LlmModel) APIKeyData() []byte {
	if l == nil || l.APIKey == nil {
		return nil
	}
	return l.APIKey.Value()
}

// LlmBinding 表示一个档位到模型注册表记录的绑定关系，每个槽位至多一条记录
// （slot 唯一）。不同槽位允许复用同一条模型记录；被绑定的模型不允许删除
// （外键 ON DELETE RESTRICT）。对应表为 llm_bindings。
type LlmBinding struct {
	database.BaseModel // 内嵌基础模型，包含 ID、CreatedAt、UpdatedAt 字段（表无 deleted_at 列）

	// Slot 档位槽位：flash | standard | advanced | embedding，全局唯一（uq_llm_bindings_slot）
	Slot Slot `gorm:"type:VARCHAR(16);uniqueIndex:uq_llm_bindings_slot"`
	// ModelID 绑定的模型记录 ID（llm_models.id）
	ModelID uint `gorm:"type:BIGINT;index:idx_llm_bindings_model_id"`
	// Model 绑定的模型实体，用于 GORM 关联（foreignKey:ModelID;references:ID；删除受限）
	Model LlmModel `gorm:"foreignKey:ModelID;references:ID;constraint:OnDelete:RESTRICT"`
}

// AgentParam 表示一条 Agent 行为参数记录：出厂值（DefaultValue）由 migrate fixture 注入、
// 只读，当前生效值（Value）可由管理接口或直接改库调整。表内每个注册表键恒有且仅有一条
// 记录，参数键、类型、分组与说明由代码内参数注册表声明（注册表不再持有任何出厂值）。
// 对应表为 agent_params。
type AgentParam struct {
	database.BaseModel // 内嵌基础模型，包含 ID、CreatedAt、UpdatedAt 字段（表无 deleted_at 列）

	// Key 参数键（点号分层，如 agent.systemPrompt、retrieval.topK），全局唯一
	// （uq_agent_params_key）
	Key string `gorm:"type:VARCHAR(128);uniqueIndex:uq_agent_params_key"`
	// Value 参数当前生效值，按注册表声明的类型解释（文本形式存储）
	Value string `gorm:"type:TEXT"`
	// DefaultValue 参数出厂值（与种子化时的 Value 相同），只由 migrate fixture 维护，
	// 任何接口路径都不得写入；「恢复出厂值」即把 Value 写回本列
	DefaultValue string `gorm:"column:default_value;type:TEXT"`
}

// McpServer 表示一个注册到平台的远程 MCP（Model Context Protocol）端点，是 Agent 可调用
// 的 MCP 工具的来源。Name 为全局唯一的本地标识（uq_mcp_servers_name）。Transport 决定远端
// 连接方式，首版仅支持远程传输 streamable_http / sse；Headers 承载 Authorization 等请求
// 头，是一个 JSON 对象字符串，以加密形式落库（crypto serializer，BYTEA 列），明文经
// HeadersData() 获取。本表有意不出现 command / args / env 列——首版不支持 stdio 本地
// 进程，这是从 schema 层杜绝「管理员配置任意本地命令执行」。对应表为 mcp_servers，属全局
// 配置，不按命名空间隔离。
type McpServer struct {
	gorm.Model // 内嵌 gorm.Model，包含 ID、CreatedAt、UpdatedAt、DeletedAt 字段

	// Name MCP 服务器的本地标识，全局唯一（uq_mcp_servers_name）
	Name string `gorm:"type:VARCHAR(64);uniqueIndex:uq_mcp_servers_name"`
	// Transport 远端传输方式：streamable_http | sse，取值见 mcp.Transport（枚举类 mcpTransport，
	// 声明于 internal/ai/tools/mcp）
	Transport string `gorm:"type:VARCHAR(32);index:idx_mcp_servers_transport"`
	// URL MCP 端点的地址（http/https）
	URL string `gorm:"column:url;type:VARCHAR(512)"`
	// Headers 请求头（JSON 对象字符串，承载 Authorization 等凭据），以加密形式持久化
	// （crypto serializer 密文入库，BYTEA 列）；明文经 HeadersData() 获取
	Headers *serializer.Crypto `json:"headers" gorm:"column:headers;type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=7002,length=32"`
	// TimeoutSeconds 单次请求的超时秒数，0 表示使用调用方默认值
	TimeoutSeconds int `gorm:"type:INTEGER"`
	// Enabled 是否启用，禁用的服务器及其全部工具不参与装配
	Enabled bool `gorm:"type:BOOLEAN;default:true"`
	// Description 服务器描述，说明用途
	Description string `gorm:"type:VARCHAR(512)"`
}

// HeadersData 返回请求头的明文字节，nil 安全（Headers 为空或 nil 时返回 nil）。
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
type LlmTool struct {
	gorm.Model // 内嵌 gorm.Model，包含 ID、CreatedAt、UpdatedAt、DeletedAt 字段

	// Name Agent 面向的工具名，全局唯一（uq_llm_tools_name）
	Name string `gorm:"type:VARCHAR(128);uniqueIndex:uq_llm_tools_name"`
	// Source 工具来源：builtin | mcp，取值见 ToolSource（枚举类 llmToolSource，声明于
	// internal/models/ai）
	Source ToolSource `gorm:"type:VARCHAR(16);index:idx_llm_tools_source"`
	// McpServerID 所属 MCP 服务器的 ID（llm_tools.mcp_server_id），可空：source='mcp' 时
	// 非空、source='builtin' 时为空
	McpServerID *uint `gorm:"type:BIGINT;index:idx_llm_tools_mcp_server_id"`
	// McpServer 所属 MCP 服务器实体，用于 GORM 关联（foreignKey:McpServerID;references:ID；
	// 级联删除）
	McpServer *McpServer `gorm:"foreignKey:McpServerID;references:ID;constraint:OnDelete:CASCADE"`
	// RemoteName MCP 服务器侧的工具名；source='builtin' 时为空
	RemoteName string `gorm:"type:VARCHAR(128)"`
	// Description 展示用描述缓存；source='builtin' 时为空，其文案取自 i18n
	Description string `gorm:"type:VARCHAR(512)"`
	// Enabled 是否启用，默认 false（安全默认，由管理员在工具面板显式启用）；禁用的工具不参与装配
	Enabled bool `gorm:"type:BOOLEAN;default:false"`
}
