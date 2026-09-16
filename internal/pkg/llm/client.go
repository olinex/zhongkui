package llm

import (
	// stdlib
	"context"
	"strings"

	// third-party
	embeddingopenai "github.com/cloudwego/eino-ext/components/embedding/openai"
	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// ChatClient 包装 eino OpenAI 兼容对话模型（eino-ext components/model/openai），
// 提供流式（Stream）与非流式（Generate）两种生成方式。构造时固定注入
// response_format=json_object（经 eino-ext 的 ChatModelConfig.ResponseFormat 配置），
// 保证结构化输出（重排等场景）可用；调用 Stream 时 eino-ext 底层请求固定携带
// stream:true，无需额外配置。温度与 maxTokens 在每次调用时以 eino model.Option 注入，
// 超时经 ChatModelConfig.Timeout 在构造时注入。
// ChatClient wraps the eino OpenAI-compatible chat model (eino-ext components/model/openai)
// and exposes both streaming (Stream) and non-streaming (Generate) generation. The
// response_format=json_object is injected at construction time via the eino-ext
// ChatModelConfig.ResponseFormat, keeping structured outputs (e.g. rerank) available;
// calling Stream makes the underlying eino-ext request always carry stream:true, so no
// extra config is needed. Temperature and maxTokens are injected as eino model.Option per
// call, while the timeout is injected via ChatModelConfig.Timeout at construction time.
type ChatClient struct {
	model model.ChatModel // 底层 eino 对话模型
	cfg   ChatConfig      // 对话模型配置快照
}

// Generate 以非流式方式生成回复，调用时注入温度与 maxTokens。
// Generate produces a complete reply in a non-streaming manner, injecting the temperature
// and maxTokens per call.
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
// Stream produces a streaming reply (the underlying request carries stream:true),
// injecting the temperature and maxTokens per call. The caller is responsible for
// closing the returned reader.
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

// NewChatClient 依据 ChatConfig 构造 ChatClient。
// 要求 baseURL/apiKey/model 均非空，否则返回英文错误（不允许静默降级）。
// NewChatClient constructs a ChatClient from a ChatConfig. It requires non-empty
// baseURL/apiKey/model and returns an English error otherwise (no silent degradation).
func NewChatClient(ctx context.Context, cfg ChatConfig) (*ChatClient, error) {
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
// EmbedClient wraps the eino OpenAI-compatible embedding model (eino-ext
// components/embedding/openai), implementing the eino embedding.Embedder interface so it
// can be injected directly into the eino retrieval pipeline.
type EmbedClient struct {
	embedder embedding.Embedder // 底层 eino 向量化模型
	cfg      EmbeddingConfig    // 向量化模型配置快照
}

// EmbedStrings 批量向量化文本，返回与输入顺序一致的向量列表。
// EmbedStrings embeds a batch of texts, returning vectors aligned with the input order.
func (e *EmbedClient) EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	vectors, err := e.embedder.EmbedStrings(ctx, texts, opts...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return vectors, nil
}

// NewEmbedClient 依据 EmbeddingConfig 构造 EmbedClient。
// 要求 baseURL/apiKey/model 均非空，否则返回英文错误（不允许静默降级）。
// NewEmbedClient constructs an EmbedClient from an EmbeddingConfig. It requires non-empty
// baseURL/apiKey/model and returns an English error otherwise (no silent degradation).
func NewEmbedClient(ctx context.Context, cfg EmbeddingConfig) (*EmbedClient, error) {
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

// Clients 聚合对话与向量化两类客户端，供上层一次性构造与持有。
// Clients aggregates the chat and embedding clients for one-shot construction and holding
// by upper layers.
type Clients struct {
	// Chat 对话客户端（重排等场景使用）
	// Chat the chat client (used by rerank etc.)
	Chat *ChatClient
	// Embed 向量化客户端（索引与向量检索使用）
	// Embed the embedding client (used by indexing and vector retrieval)
	Embed *EmbedClient
}

// NewClients 依据 LLMConfig 一次性构造对话与向量化客户端，构造底层模型时透传 ctx。
// 仅校验端点配置（baseURL/apiKey/model）完整性，不校验 [llm].enabled 开关——开关的
// 调用时机由功能开关层控制；chat 或 embedding 任一配置缺失即返回指明缺失部分的英文
// 错误，不允许静默降级（避免检索链路以残缺能力运行）。需要仅构造单类客户端的场景
// 请使用 NewChatClient / NewEmbedClient。
// NewClients constructs both the chat and embedding clients from an LLMConfig in one call,
// threading ctx into the underlying model construction. It only validates the endpoint
// configs (baseURL/apiKey/model) and does not check the [llm].enabled switch, whose
// invocation timing is controlled by the feature-toggle layer; a missing chat or embedding
// config returns an English error naming the missing part and never degrades silently (so
// the retrieval pipeline never runs with crippled capabilities). Use NewChatClient /
// NewEmbedClient when only one client kind is needed.
func NewClients(ctx context.Context, cfg *LLMConfig) (*Clients, error) {
	if cfg == nil {
		return nil, errors.New("ai config is nil")
	}
	var missing []string
	if cfg.Chat.BaseURL == "" || cfg.Chat.APIKey == "" || cfg.Chat.Model == "" {
		missing = append(missing, "chat")
	}
	if cfg.Embedding.BaseURL == "" || cfg.Embedding.APIKey == "" || cfg.Embedding.Model == "" {
		missing = append(missing, "embedding")
	}
	if len(missing) > 0 {
		return nil, errors.Newf("ai clients cannot be constructed: incomplete endpoint config for: %s", strings.Join(missing, ", "))
	}
	chat, err := NewChatClient(ctx, cfg.Chat)
	if err != nil {
		return nil, err
	}
	embed, err := NewEmbedClient(ctx, cfg.Embedding)
	if err != nil {
		return nil, err
	}
	return &Clients{Chat: chat, Embed: embed}, nil
}
