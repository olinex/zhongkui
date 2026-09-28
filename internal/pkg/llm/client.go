// Package llm 提供 AI 助手（RAG + Agent）的 LLM 接入与行为参数体系：OpenAI 兼容的对话
// （Chat）与向量化（Embedding）客户端封装、模型档位（flash / standard / advanced / embedding）
// 取值与解析、可调行为参数注册表（提示词、检索融合与重排、会话上下文、切片、评估与
// 清洗限界），以及知识库向量列的维度校准。本包与 GORM / 具体持久化模型解耦，仅依赖
// stdlib 与 internal/pkg 内共享包；LLM 端点配置的唯一事实来源是数据库模型注册表
// （见 ModelConfig），不再来自配置文件。
package llm

import (
	// 标准库
	"context"

	// 第三方
	embeddingopenai "github.com/cloudwego/eino-ext/components/embedding/openai"
	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// ChatClient 包装 eino OpenAI 兼容对话模型（eino-ext components/model/openai），
// 提供流式（Stream）与非流式（Generate）两种生成方式。构造时固定注入
// response_format=json_object（经 eino-ext 的 ChatModelConfig.ResponseFormat 配置），
// 保证结构化输出（重排等场景）可用；调用 Stream 时 eino-ext 底层请求固定携带
// stream:true，无需额外配置。温度与 maxTokens 在每次调用时以 eino model.Option 注入，
// 超时经 ChatModelConfig.Timeout 在构造时注入。
type ChatClient struct {
	model model.ChatModel // 底层 eino 对话模型
	cfg   ModelConfig     // 模型端点配置快照
}

// Generate 以非流式方式生成回复，调用时注入温度与 maxTokens。
func (c *ChatClient) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	opts = append([]model.Option{
		model.WithTemperature(float32(c.cfg.Temperature)),
		model.WithMaxTokens(c.cfg.MaxTokens),
	}, opts...)
	if msg, err := c.model.Generate(ctx, messages, opts...); err != nil {
		return nil, errors.WithStack(err)
	} else {
		return msg, nil
	}
}

// Stream 以流式方式生成回复（底层请求 stream:true），调用时注入温度与 maxTokens。
// 调用方负责关闭返回的 reader。
func (c *ChatClient) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	opts = append([]model.Option{
		model.WithTemperature(float32(c.cfg.Temperature)),
		model.WithMaxTokens(c.cfg.MaxTokens),
	}, opts...)
	if reader, err := c.model.Stream(ctx, messages, opts...); err != nil {
		return nil, errors.WithStack(err)
	} else {
		return reader, nil
	}
}

// ModelName 返回底层供应商模型名（如 gpt-4o-mini），供 dry-run 元信息等展示场景使用。
func (c *ChatClient) ModelName() string {
	return c.cfg.Model
}

// NewChatClient 依据模型端点配置快照构造 ChatClient。
// 要求 baseURL/apiKey/model 均非空，否则返回英文错误（不允许静默降级）。
func NewChatClient(ctx context.Context, cfg ModelConfig) (*ChatClient, error) {
	if cfg.BaseURL == "" || cfg.APIKey == "" || cfg.Model == "" {
		return nil, errors.New("incomplete chat config: baseURL/apiKey/model must all be non-empty")
	}
	maxTokens := cfg.MaxTokens
	config := &einoopenai.ChatModelConfig{
		BaseURL:   cfg.BaseURL,
		APIKey:    cfg.APIKey,
		Model:     cfg.Model,
		Timeout:   cfg.Timeout,
		MaxTokens: &maxTokens,
		// response_format=json_object：保证重排等结构化输出场景可用
		ResponseFormat: &einoopenai.ChatCompletionResponseFormat{
			Type: einoopenai.ChatCompletionResponseFormatTypeJSONObject,
		},
	}
	cm, err := einoopenai.NewChatModel(ctx, config)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &ChatClient{model: cm, cfg: cfg}, nil
}

// EmbedClient 包装 eino OpenAI 兼容向量化模型（eino-ext components/embedding/openai），
// 实现 eino embedding.Embedder 接口，可直接注入 eino 检索链路。
type EmbedClient struct {
	embedder embedding.Embedder // 底层 eino 向量化模型
	cfg      ModelConfig        // 模型端点配置快照
}

// embedBatchSize 单次向量化请求允许提交的最大文本条数。上游 eino 的 OpenAI 兼容向量化客户端
// 会把整批文本一次提交、不做切分，而部分供应商对单请求 contents 条数设有硬上限（本实例的
// 百炼模型即返回「batch size is invalid, it should not be larger than 20」）。此处取一个对常见
// 供应商都安全的保守值，由 EmbedClient 在内部切批提交，对外仍保持「一次批量向量化、结果顺序
// 与入参一致」的语义。
const embedBatchSize = 10

// EmbedStrings 批量向量化文本，返回与输入顺序一致的向量列表。超过 embedBatchSize 时按批切分
// 逐批提交再顺序拼接，规避供应商的单请求条数上限。
func (e *EmbedClient) EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	if len(texts) <= embedBatchSize {
		vectors, err := e.embedder.EmbedStrings(ctx, texts, opts...)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return vectors, nil
	}
	vectors := make([][]float64, 0, len(texts))
	for start := 0; start < len(texts); start += embedBatchSize {
		batch, err := e.embedder.EmbedStrings(ctx, texts[start:min(start+embedBatchSize, len(texts))], opts...)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		vectors = append(vectors, batch...)
	}
	return vectors, nil
}

// ModelName 返回底层供应商模型名（如 text-embedding-3-small），供状态展示使用。
func (e *EmbedClient) ModelName() string {
	return e.cfg.Model
}

// NewEmbedClient 依据模型端点配置快照构造 EmbedClient。
// 要求 baseURL/apiKey/model 均非空，否则返回英文错误（不允许静默降级）。
func NewEmbedClient(ctx context.Context, cfg ModelConfig) (*EmbedClient, error) {
	if cfg.BaseURL == "" || cfg.APIKey == "" || cfg.Model == "" {
		return nil, errors.New("incomplete embedding config: baseURL/apiKey/model must all be non-empty")
	}
	config := &embeddingopenai.EmbeddingConfig{
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Model:   cfg.Model,
	}
	if cfg.Dimensions > 0 {
		dimensions := cfg.Dimensions
		config.Dimensions = &dimensions
	}
	embedder, err := embeddingopenai.NewEmbedder(ctx, config)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &EmbedClient{embedder: embedder, cfg: cfg}, nil
}
