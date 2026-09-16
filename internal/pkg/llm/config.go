// Package llm 提供 AI 助手（RAG + Agent）的外部 LLM 接入与行为配置体系：基于 [llm] 配置段
// 声明对话（Chat）/ 向量化（Embedding）OpenAI 兼容端点、混合检索融合与 LLM 重排（Rerank）、
// Agent 会话（Session）与 Markdown 结构感知切片（Chunking）参数。本包与 GORM / 具体 LLM
// 客户端解耦，仅依赖 stdlib 与 internal/pkg 内共享包（config、errors），配置来源为
// config.toml 的 [llm] 段（见 LLMConfig）。
//
// Package llm provides the external-LLM integration and behavior config system of the AI
// assistant (RAG + Agent): it declares the OpenAI-compatible endpoints of the chat /
// embedding models, the hybrid-retrieval fusion and LLM rerank (Rerank), the Agent session
// (Session) and the Markdown structure-aware chunking (Chunking) parameters via the [llm]
// config section. This package is decoupled from GORM and concrete LLM clients, depending
// only on the stdlib and the shared internal/pkg packages (config, errors), with its config
// sourced from the [llm] section of config.toml (see LLMConfig).
package llm

import (
	// stdlib
	"time"

	// third-party
	"github.com/creasty/defaults"
	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// Prefix 是 AI 助手配置在配置文件中的前缀（"llm"）。
// 作为单一事实来源，供 provider / 配置提取引用。
// Prefix is the config-file prefix of the AI assistant config ("llm"); it is the single
// source of truth referenced by providers and config extraction.
const Prefix = "llm"

// LLMConfig 是 AI 能力（RAG + Agent）的外部 LLM 接入与行为配置，对应 config.toml 的 [llm] 段。
// 字段通过 mapstructure 标签映射、default 标签填充默认值、comment 标签提供中文说明
// （DescribeConfig 的渲染来源）；能力可用性经 Enabled 聚合判断（[llm].enabled 开关开启且
// chat/embedding 端点配置完整），供全局功能开关（features）使用。注意：因 Enabled 方法
// 需实现 features.IsEnabled 接口，[llm].enabled 开关字段命名为 EnabledSwitch（配置键不变）。
// LLMConfig is the config struct of the AI capabilities (RAG + Agent) external LLM
// integration and behavior, mapped to the [llm] section of config.toml. Fields are mapped
// via the mapstructure tag, defaulted via the default tag, and documented in Chinese via
// the comment tag (rendered by DescribeConfig); the capability availability is aggregated
// by Enabled ([llm].enabled switch on and chat/embedding endpoint configs complete) for the
// global feature toggles (features). Note the [llm].enabled switch field is named
// EnabledSwitch (the config key is unchanged) because the Enabled method must implement the
// features.IsEnabled interface.
type LLMConfig struct {
	// EnabledSwitch 是否启用 AI 助手（RAG + Agent）能力（配置键为 [llm].enabled）
	// EnabledSwitch whether the AI assistant (RAG + Agent) capability is enabled
	// (config key [llm].enabled)
	EnabledSwitch bool `mapstructure:"enabled" default:"false" comment:"是否启用 AI 助手（RAG + Agent）能力"`
	// Chat 对话/生成（Chat）模型端点配置，OpenAI 兼容
	// Chat the chat/generation (Chat) model endpoint config, OpenAI compatible
	Chat ChatConfig `mapstructure:"chat" comment:"对话/生成（Chat）模型端点配置，OpenAI 兼容"`
	// Embedding 向量化（Embedding）模型端点配置，OpenAI 兼容
	// Embedding the embedding model endpoint config, OpenAI compatible
	Embedding EmbeddingConfig `mapstructure:"embedding" comment:"向量化（Embedding）模型端点配置，OpenAI 兼容"`
	// Rerank 混合检索候选集融合与 LLM 重排参数
	// Rerank the hybrid-retrieval candidate fusion and LLM rerank parameters
	Rerank RerankConfig `mapstructure:"rerank" comment:"混合检索候选集融合与 LLM 重排参数"`
	// Session Agent 会话与上下文注入参数
	// Session the Agent session and context-injection parameters
	Session SessionConfig `mapstructure:"session" comment:"Agent 会话与上下文注入参数"`
	// Chunking Markdown 结构感知切片参数
	// Chunking the Markdown structure-aware chunking parameters
	Chunking ChunkingConfig `mapstructure:"chunking" comment:"Markdown 结构感知切片参数"`
}

// ChatConfig Chat 模型端点配置，OpenAI 兼容。
// ChatConfig the chat model endpoint config, OpenAI compatible.
type ChatConfig struct {
	// BaseURL Chat 模型 OpenAI 兼容接口 Base URL
	// BaseURL the OpenAI-compatible interface Base URL of the chat model
	BaseURL string `mapstructure:"baseURL" default:"" comment:"Chat 模型 OpenAI 兼容接口 Base URL"`
	// APIKey Chat 模型 API 密钥
	// APIKey the API key of the chat model
	APIKey string `mapstructure:"apiKey" default:"" comment:"Chat 模型 API 密钥"`
	// Model Chat 模型名称
	// Model the chat model name
	Model string `mapstructure:"model" default:"" comment:"Chat 模型名称"`
	// Timeout Chat 模型调用超时
	// Timeout the call timeout of the chat model
	Timeout time.Duration `mapstructure:"timeout" default:"60s" comment:"Chat 模型调用超时"`
	// Temperature Chat 生成温度，越低越确定
	// Temperature the chat generation temperature; lower is more deterministic
	Temperature float64 `mapstructure:"temperature" default:"0.2" comment:"Chat 生成温度，越低越确定"`
	// MaxTokens Chat 单次生成最大 token 数
	// MaxTokens the maximum number of tokens generated per chat call
	MaxTokens int `mapstructure:"maxTokens" default:"4096" comment:"Chat 单次生成最大 token 数"`
}

// EmbeddingConfig Embedding 模型端点配置，OpenAI 兼容。
// EmbeddingConfig the embedding model endpoint config, OpenAI compatible.
type EmbeddingConfig struct {
	// BaseURL Embedding 模型 OpenAI 兼容接口 Base URL
	// BaseURL the OpenAI-compatible interface Base URL of the embedding model
	BaseURL string `mapstructure:"baseURL" default:"" comment:"Embedding 模型 OpenAI 兼容接口 Base URL"`
	// APIKey Embedding 模型 API 密钥
	// APIKey the API key of the embedding model
	APIKey string `mapstructure:"apiKey" default:"" comment:"Embedding 模型 API 密钥"`
	// Model Embedding 模型名称
	// Model the embedding model name
	Model string `mapstructure:"model" default:"" comment:"Embedding 模型名称"`
	// Dimensions 向量维度。为唯一事实来源：需与所选 embedding 模型的输出维度一致，
	// 且通常与 knowledge_chunks.embedding 列维度一致；若列维度与配置不一致，
	// 服务启动会中止并提示执行 migrate 子命令（此时 migrate 会重建列，见
	// ReconcileEmbeddingColumn），更换模型维度只需改这里后执行 migrate。
	// Dimensions the vector dimension. It is the single source of truth and must match the
	// output dimension of the chosen embedding model; normally it also aligns with the
	// knowledge_chunks.embedding column. If the column differs, server startup aborts and
	// directs the user to run the migrate subcommand (which rebuilds the column, see
	// ReconcileEmbeddingColumn), so switching model dimensions only requires editing this
	// value and running migrate.
	Dimensions int `mapstructure:"dimensions" default:"1536" comment:"向量维度，需与 embedding 模型输出维度一致，变更后执行 migrate 重建列"`
}

// RerankConfig 混合检索融合与 LLM 重排参数。
// RerankConfig the hybrid-retrieval fusion and LLM rerank parameters.
type RerankConfig struct {
	// TopK LLM 重排后保留的最终引用条数
	// TopK the number of final references kept after the LLM rerank
	TopK int `mapstructure:"topK" default:"5" comment:"LLM 重排后保留的最终引用条数"`
	// Candidates 融合后送入 LLM 重排的候选条数
	// Candidates the number of candidates fed into the LLM rerank after fusion
	Candidates int `mapstructure:"candidates" default:"30" comment:"融合后送入 LLM 重排的候选条数"`
	// Fusion 两路检索融合策略：rrf 或 weighted
	// Fusion the two-way retrieval fusion strategy: rrf or weighted
	Fusion string `mapstructure:"fusion" default:"rrf" comment:"两路检索融合策略：rrf 或 weighted"`
	// Weights weighted 融合时的权重
	// Weights the weights used by the weighted fusion
	Weights WeightConfig `mapstructure:"weights" comment:"weighted 融合时的权重"`
}

// WeightConfig 加权融合权重。
// WeightConfig the weighted-fusion weights.
type WeightConfig struct {
	// Vector 向量检索路权重
	// Vector the weight of the vector retrieval path
	Vector float64 `mapstructure:"vector" default:"0.6" comment:"向量检索路权重"`
	// Keyword 关键词检索路权重
	// Keyword the weight of the keyword retrieval path
	Keyword float64 `mapstructure:"keyword" default:"0.4" comment:"关键词检索路权重"`
}

// SessionConfig Agent 会话参数。
// SessionConfig the Agent session parameters.
type SessionConfig struct {
	// MaxContextMessages 注入 LLM 上下文的历史消息条数上限
	// MaxContextMessages the upper bound of history messages injected into the LLM context
	MaxContextMessages int `mapstructure:"maxContextMessages" default:"20" comment:"注入 LLM 上下文的历史消息条数上限"`
	// MaxContextBytes 前端透传 businessData 单次注入的总大小上限（字节），超限时清洗截断
	// MaxContextBytes the upper bound of the total size (bytes) of the frontend-passed
	// businessData injected per call; over-limit data is sanitized and truncated
	MaxContextBytes int `mapstructure:"maxContextBytes" default:"32768" comment:"前端透传 businessData 单次注入的总大小上限（字节），超限时清洗截断"`
}

// ChunkingConfig Markdown 结构感知切片参数。
// ChunkingConfig the Markdown structure-aware chunking parameters.
type ChunkingConfig struct {
	// Size 切片亲和目标长度（字符），结构原子单元内尽量靠近
	// Size the affinity target length (characters) of a chunk, kept close within a
	// structural atomic unit
	Size int `mapstructure:"size" default:"800" comment:"切片亲和目标长度（字符），结构原子单元内尽量靠近"`
	// Overlap 相邻切块的重叠衔接长度（字符）
	// Overlap the overlap length (characters) between adjacent chunks
	Overlap int `mapstructure:"overlap" default:"100" comment:"相邻切块的重叠衔接长度（字符）"`
}

// DescribeConfig 返回 AI 助手配置的结构化描述信息（供 describe config 渲染）。
// DescribeConfig returns the structured description of the AI assistant config (rendered by describe config).
func DescribeConfig() config.ConfigDescriptions {
	return config.DescribeConfig[LLMConfig](Prefix)
}

// ExtractConfig 从 Viper 配置管理器中提取 [llm] 配置并校验。
// 注意：仅当配置文件中存在 [llm] 段时可用；完全缺省时返回错误（可能缺省时请使用 ExtractConfigOrEmpty）。
// ExtractConfig extracts and validates the [llm] config from the Viper instance. Note it
// works only when the [llm] section exists in the config file; a fully absent section
// returns an error (use ExtractConfigOrEmpty when the section may be missing).
func ExtractConfig(v *viper.Viper, validate *validator.Validate) (*LLMConfig, error) {
	return config.ExtractConfig[LLMConfig](v, validate, Prefix)
}

// ExtractConfigOrEmpty 从 Viper 配置管理器中提取 [llm] 配置：当配置文件中不存在 [llm] 段时
// 返回填充默认值的空配置（nil error），保证未配置 [llm] 时功能开关正常关闭、服务正常启动；
// 当 [llm] 段存在时行为与 ExtractConfig 一致，配置解析或校验失败仍返回错误（不吞错）。
// ExtractConfigOrEmpty extracts the [llm] config from the Viper instance, returning a
// defaulted empty config (nil error) when the [llm] section is absent from the config file
// so the feature toggle stays disabled and the service starts normally without the section;
// when the [llm] section exists it behaves exactly like ExtractConfig and still returns an
// error on parse or validation failure (errors are never swallowed).
func ExtractConfigOrEmpty(v *viper.Viper, validate *validator.Validate) (*LLMConfig, error) {
	if v.Sub(Prefix) == nil {
		var cfg LLMConfig
		if err := defaults.Set(&cfg); err != nil {
			return nil, errors.WithStack(err)
		}
		return &cfg, nil
	}
	return ExtractConfig(v, validate)
}

// Enabled 报告 AI 能力是否可用：仅当 [llm] 启用且 chat/embedding 两套端点配置
// （baseURL/apiKey/model）均非空时可用。该能力判断供全局功能开关（features）使用，
// 使底层 LLM 接入细节仅在本包内可见，features 层无需直接读取 LLMConfig。
// Enabled reports whether the AI capability is available: only when [llm] is enabled and
// both the chat and embedding endpoint configs (baseURL/apiKey/model) are non-empty. It is
// used by the global feature toggles (features) so the underlying LLM access details stay
// visible within this package only.
func (c *LLMConfig) Enabled() bool {
	return c.EnabledSwitch &&
		c.Chat.BaseURL != "" && c.Chat.APIKey != "" && c.Chat.Model != "" &&
		c.Embedding.BaseURL != "" && c.Embedding.APIKey != "" && c.Embedding.Model != ""
}
