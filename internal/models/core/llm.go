// Package core 提供平台核心领域的持久化模型。本文件定义 LLM 模型注册表（LlmModel）、
// 档位绑定（LlmBinding）与 Agent 参数（AgentParam）三个模型：模型端点由前端动态
// 注册（APIKey 以密文落库），档位绑定把 flash / standard / advanced / embedding 四个固定槽位
// 指向注册表中的一条模型记录，参数表同时持有出厂值（default_value，由 migrate fixture
// 注入）与当前生效值（value），参数类值的唯一来源是数据库而非代码。三者均为全局配置，
// 不按命名空间隔离。
//
// Package core provides the persistence models of the platform core domain. This file
// defines the LLM model registry (LlmModel), the slot binding (LlmBinding) and the Agent
// parameter (AgentParam): model endpoints are registered dynamically by the frontend (API
// keys persisted encrypted at rest), slot bindings point the four fixed slots
// flash / standard / advanced / embedding to one record of the registry, and the parameter
// table carries both the factory value (default_value, injected by the migrate fixtures) and
// the effective value (value), making the database rather than the code the single source of
// parameter values. All three are global configuration, not namespace scoped.
package core

import (
	// third-party
	"gorm.io/gorm"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/database"
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
	Kind string `gorm:"type:VARCHAR(16);index:idx_llm_models_kind"`
	// BaseURL 模型端点的基地址（OpenAI 兼容 API），如 https://api.openai.com/v1
	// BaseURL the base address of the model endpoint (OpenAI compatible API),
	// e.g. https://api.openai.com/v1
	BaseURL string `gorm:"type:VARCHAR(512)"`
	// APIKey 访问模型端点的凭据，以加密形式持久化（crypto serializer 密文入库，BYTEA 列）；
	// 明文经 APIKeyData() 获取
	// APIKey the credential used to access the model endpoint, persisted encrypted at
	// rest via the crypto serializer (BYTEA column), plaintext obtained via APIKeyData()
	APIKey *serializer.Crypto `gorm:"column:api_key;type:BYTEA;serializer:crypto" secure:"mode=\"aes-gcm\",offset=7001,length=32"`
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
	Slot string `gorm:"type:VARCHAR(16);uniqueIndex:uq_llm_bindings_slot"`
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
