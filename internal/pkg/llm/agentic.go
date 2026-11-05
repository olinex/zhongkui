package llm

import (
	"context"
	"maps"

	einoagenticopenai "github.com/cloudwego/eino-ext/components/model/agenticopenai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

// AgenticClient 封装 eino-ext 的 Response API 模型（agenticopenai），用于支持 /v1/responses
// 的 OpenAI 兼容端点；它是 ChatClient 的 responses 对应实现，满足 model.AgenticModel 接口。
type AgenticClient struct {
	// model 底层 Response API 模型（agenticopenai 实现）。
	model model.AgenticModel
	// cfg 模型配置（超时、温度、最大 token 等）。
	cfg ModelConfig
}

// Generate 执行一次非流式调用，产出一条 AgenticMessage；超时按 TimeoutRetry 策略重试。
func (c *AgenticClient) Generate(ctx context.Context, messages []*schema.AgenticMessage, opts ...model.Option) (*schema.AgenticMessage, error) {
	opts = append([]model.Option{
		model.WithTemperature(float32(c.cfg.Temperature)),
		model.WithMaxTokens(c.cfg.MaxTokens),
	}, opts...)
	return retryOnTimeout(ctx, c.cfg.Timeout, c.cfg.TimeoutRetry, func(attemptCtx context.Context) (*schema.AgenticMessage, error) {
		return c.model.Generate(attemptCtx, messages, opts...)
	})
}

// Stream 执行一次流式调用，返回 AgenticMessage 流；读取器由调用方负责关闭。
func (c *AgenticClient) Stream(ctx context.Context, messages []*schema.AgenticMessage, opts ...model.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	opts = append([]model.Option{
		model.WithTemperature(float32(c.cfg.Temperature)),
		model.WithMaxTokens(c.cfg.MaxTokens),
	}, opts...)
	reader, err := c.model.Stream(ctx, messages, opts...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return reader, nil
}

// RequestOptions 依据 ExtraBody 与思考开关构造按请求的模型选项。agenticopenai.WithExtraFields
// 采用覆盖语义，故先把各来源合并为一张表再一次传入。
func (c *AgenticClient) RequestOptions(thinking string) []model.Option {
	merged := requestExtraFields(c.cfg, thinking)
	if len(merged) == 0 {
		return nil
	}
	fields := make(map[string]any, len(merged))
	maps.Copy(fields, merged)
	return []model.Option{einoagenticopenai.WithExtraFields(fields)}
}

// ModelName 返回供应商侧的模型名。
func (c *AgenticClient) ModelName() string { return c.cfg.Model }

// NewAgenticChatClient 依据模型配置构造 AgenticClient。
func NewAgenticChatClient(ctx context.Context, cfg ModelConfig) (*AgenticClient, error) {
	if err := cfg.validateEndpoint(); err != nil {
		return nil, errors.Newf("incomplete agentic chat config: %s", validator.Describe(err))
	}
	timeout := cfg.Timeout
	maxTokens := cfg.MaxTokens
	temperature := float32(cfg.Temperature)
	config := &einoagenticopenai.ResponsesConfig{
		BaseURL:     cfg.BaseURL,
		APIKey:      cfg.APIKey,
		Model:       cfg.Model,
		HTTPClient:  newChatHTTPClient(cfg.Timeout),
		MaxTokens:   &maxTokens,
		Temperature: &temperature,
	}
	if timeout > 0 {
		config.Timeout = &timeout
	}
	cm, err := einoagenticopenai.NewResponsesModel(ctx, config)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &AgenticClient{model: cm, cfg: cfg}, nil
}
