// Package llm 提供 AI 助手（RAG + Agent）的 LLM 接入底座：OpenAI 兼容的对话（Chat）与
// 向量化（Embedding）客户端封装、模型与档位（flash / standard / advanced / embedding）的
// 取值契约与解析、Markdown 结构感知切片，以及 embedding 端点的维度探测。本包与 GORM /
// 具体持久化模型解耦，仅依赖 stdlib 与 internal/pkg 内共享包；LLM 端点配置的唯一事实来源
// 是数据库模型注册表（见 ModelConfig），不再来自配置文件。
package llm

import (
	// 标准库
	"context"
	"maps"
	"net"
	"net/http"
	"time"

	// 第三方
	embeddingopenai "github.com/cloudwego/eino-ext/components/embedding/openai"
	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// ChatClient 包装 eino OpenAI 兼容对话模型（eino-ext components/model/openai），
// 提供流式（Stream）与非流式（Generate）两种生成方式。调用 Stream 时 eino-ext 底层请求
// 固定携带 stream:true，无需额外配置。温度与 maxTokens 在每次调用时以 eino model.Option
// 注入。超时经自定义 HTTPClient 注入：见 newChatHTTPClient，非流式 Generate 以调用方 context
// 施加整请求超时，流式 Stream 仅以响应头超时约束首字节、整体时长交由调用方 context 约束。
//
// 本客户端**不注入** response_format：结构化输出（重排、记忆抽取、审核分级、dry-run 评估）
// 与自由文本输出（会话主题、滚动摘要）共用同一份档位客户端，而 OpenAI 兼容供应商在启用
// response_format=json_object 时会校验消息中必须出现 "json" 字样，自由文本提示词不含该字样
// 会被整请求以 400 拒绝（本实例的百炼 qwen 即如此）。结构化输出改由各自系统提示词显式声明
// 要求的 JSON 形状保证，且各调用点的解析函数均已容忍 ```json 围栏包裹（见 ai 包的
// stripJSONFence / parseRerankIndices / parseExtractedMemories / parseAuditLevels）。
type ChatClient struct {
	model model.ChatModel // 底层 eino 对话模型
	cfg   ModelConfig     // 模型端点配置快照
}

// Generate 以非流式方式生成回复，调用时注入温度与 maxTokens。以模型超时给 context 施加
// 整请求截止时间（等价于原 http.Client.Timeout），超时为正数时才生效。
func (c *ChatClient) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if c.cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.cfg.Timeout)
		defer cancel()
	}
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
// 调用方负责关闭返回的 reader。此处不对 context 施加整请求截止时间：一次长回答的流式
// 生成时长可远超模型超时，若以整请求超时约束会在读取正文阶段被中断（见 newChatHTTPClient
// 的说明）。整体时长由调用方 context（如 HTTP 请求生命周期）约束。
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

// requestExtraFields 依据思考模式开关把模型端点的高级请求字段与思考开关合并为「本次请求」要
// 注入的字段集合：以 ExtraBody 的浅拷贝为基底，thinking 为 "on"/"off" 且已配置 ThinkingField
// 时，把对应取值写入该字段（会覆盖 ExtraBody 中的同名键）；thinking 为空或其他取值时不注入
// 思考字段。返回 nil 表示本次请求无需注入任何高级字段。拷贝基底是为了避免向配置快照写入。
func requestExtraFields(cfg ModelConfig, thinking string) map[string]any {
	merged := make(map[string]any, len(cfg.ExtraBody)+1)
	maps.Copy(merged, cfg.ExtraBody)
	if cfg.ThinkingField != "" {
		switch thinking {
		case "on":
			merged[cfg.ThinkingField] = cfg.ThinkingOnValue
		case "off":
			merged[cfg.ThinkingField] = cfg.ThinkingOffValue
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

// RequestOptions 依据会话的思考模式开关（"on" 开启 / "off" 关闭 / 其余含空串不改变思考）构造
// 本次对话请求要注入的 eino 选项。以模型配置快照的 ExtraBody 为基底拷贝一份，再按开关把
// ThinkingField 对应取值合并进同一 map。
//
// 注意 eino 的 WithExtraFields（einoopenai.WithExtraFields）是「替换」语义而非「合并」语义：
// 同一次请求内多次调用只有最后一个生效，因此必须把 ExtraBody 与思考字段合并为一个 map 后单次
// 调用；thinking 为空或未知取值时不注入思考字段。合并结果为空时返回空切片（不注入任何高级字段）。
func (c *ChatClient) RequestOptions(thinking string) []model.Option {
	merged := requestExtraFields(c.cfg, thinking)
	if len(merged) == 0 {
		return nil
	}
	return []model.Option{einoopenai.WithExtraFields(merged)}
}

// newChatHTTPClient 构造对话模型专用的 HTTP 客户端：不设置 http.Client.Timeout。
// eino-ext 在未提供 HTTPClient 时以 &http.Client{Timeout: t} 承载请求，而该超时覆盖整个
// 请求生命周期（包含响应正文的读取），对 SSE 流式生成同样生效——一条耗时超过 t 的长回答
// 会在读取正文阶段被中断，报 “context deadline exceeded (Client.Timeout or context
// cancellation while reading body)”。因此这里把 t 用作连接建立与响应头（首字节）上限，
// 保证端点不可达能快速失败，同时不截断正常的长回答；整体生成时长交由调用方 context 约束
// （非流式 Generate 另有整请求超时）。t 非正数时不施加任何超时。
func newChatHTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if timeout > 0 {
		transport.DialContext = (&net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}).DialContext
		transport.TLSHandshakeTimeout = timeout
		transport.ResponseHeaderTimeout = timeout
	}
	return &http.Client{Transport: transport}
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
		BaseURL:    cfg.BaseURL,
		APIKey:     cfg.APIKey,
		Model:      cfg.Model,
		HTTPClient: newChatHTTPClient(cfg.Timeout),
		MaxTokens:  &maxTokens,
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

// EmbedStringsWithUsage 与 EmbedStrings 行为一致（含按 embedBatchSize 切批与顺序拼接），并额外
// 返回本次调用累计的 token 用量：经上下文级 eino 回调（callbacks.InitCallbacks 注入，不写进程级
// 全局回调）汇总底层向量化组件在每批结束时上报的 embedding.CallbackOutput.TokenUsage，多批自动
// 累加；供应商未返回用量时为零值。
func (e *EmbedClient) EmbedStringsWithUsage(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, embedding.TokenUsage, error) {
	var usage embedding.TokenUsage
	handler := callbacks.NewHandlerBuilder().OnEndFn(
		func(ctx context.Context, _ *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
			if out, ok := output.(*embedding.CallbackOutput); ok && out.TokenUsage != nil {
				usage.PromptTokens += out.TokenUsage.PromptTokens
				usage.CompletionTokens += out.TokenUsage.CompletionTokens
				usage.TotalTokens += out.TokenUsage.TotalTokens
			}
			return ctx
		}).Build()
	ctx = callbacks.InitCallbacks(ctx, &callbacks.RunInfo{
		Type:      "EmbedClient",
		Component: components.ComponentOfEmbedding,
	}, handler)
	vectors, err := e.EmbedStrings(ctx, texts, opts...)
	if err != nil {
		return nil, embedding.TokenUsage{}, err
	}
	return vectors, usage, nil
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
