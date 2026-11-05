// Package port 声明 internal/ai 各能力包共用的小接口：向量化、对话生成与模型名查询。
// 这些接口只描述能力消费者真正需要的最小方法集，便于各能力包单向依赖而不引入装配细节。
package port

import (
	// 标准库
	"context"
	"io"
	"strings"
	"time"

	// 第三方
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// Embedder 是向量化客户端的最小接口；*llm.EmbedClient 与 eino 实现均满足。
type Embedder interface {
	// EmbedStrings 把一批文本向量化
	EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error)
}

// UsageEmbedder 是能报告 token 用量的向量化客户端（*llm.EmbedClient 天然满足），
// 调用方经类型断言取可选能力，不支持时回退无用量路径。
type UsageEmbedder interface {
	// EmbedStringsWithUsage 把一批文本向量化并返回 token 用量
	EmbedStringsWithUsage(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, embedding.TokenUsage, error)
}

// EmbedStringsWithUsage 向量化文本并尽力取回 token 用量：emb 实现 UsageEmbedder 时调用其带用量
// 的方法并把 embedding.TokenUsage 转为 contract.EmbedUsage；否则回退 EmbedStrings 并返回零值
// contract.EmbedUsage。错误一律经 errors.WithStack 包装。
func EmbedStringsWithUsage(ctx context.Context, emb Embedder, texts []string) ([][]float64, contract.EmbedUsage, error) {
	if withUsage, ok := emb.(UsageEmbedder); ok {
		vectors, usage, err := withUsage.EmbedStringsWithUsage(ctx, texts)
		if err != nil {
			return nil, contract.EmbedUsage{}, errors.WithStack(err)
		}
		return vectors, contract.EmbedUsage{
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
		}, nil
	}
	vectors, err := emb.EmbedStrings(ctx, texts)
	if err != nil {
		return nil, contract.EmbedUsage{}, errors.WithStack(err)
	}
	return vectors, contract.EmbedUsage{}, nil
}

// ChatResolver 按档位解析对话模型（使用者侧行为契约）：由上层装配方实现，能力包按需调用它
// 取回对话客户端，从而在同一流程内支持按请求切换模型而不重建装配。
type ChatResolver = func(slot modelsai.Slot) (model.BaseChatModel, error)

// ChatModelNamer 是能报告自身模型名的对话客户端的最小接口（*llm.ChatClient 天然满足）。
type ChatModelNamer interface {
	// ModelName 返回供应商侧的模型名
	ModelName() string
}

// ChatGenerator 是能力包所需的最小对话接口；*ChatClient 与 eino 实现均满足。
// 重排与评估使用非流式 Generate；dry-run 的流式链路使用 Stream 以实时透传模型输出。
type ChatGenerator interface {
	// Generate 非流式生成一次对话
	Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error)
	// Stream 流式生成一次对话
	Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error)
}

// FlashChat 按 flash 档位解析对话客户端：resolver 缺失、档位未绑定或解析失败均返回英文错误，
// 绝不静默切换到其他档位。供严格使用低档位的辅助任务（记忆抽取、检索重排等）复用。
func FlashChat(resolver ChatResolver) (model.BaseChatModel, error) {
	if resolver == nil {
		return nil, errors.New("ai: nil chat resolver")
	}
	chat, err := resolver(modelsai.SlotFlash)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if chat == nil {
		return nil, errors.Newf("ai: no chat model resolved for slot %q", modelsai.SlotFlash)
	}
	return chat, nil
}

// FlashOrFallbackChat 优先按 flash 档位解析对话客户端，失败时回退到本次运行档位；两者都失败返回
// 英文错误。供审核评估、推荐下一步等「优先低成本、允许使用运行档位」的辅助任务复用。
func FlashOrFallbackChat(resolver ChatResolver, runSlot modelsai.Slot) (model.BaseChatModel, error) {
	if resolver == nil {
		return nil, errors.New("ai: nil chat resolver")
	}
	if chat, err := resolver(modelsai.SlotFlash); err == nil && chat != nil {
		return chat, nil
	}
	if chat, err := resolver(runSlot); err == nil && chat != nil {
		return chat, nil
	}
	return nil, errors.Newf("ai: no chat model resolved for slot %q", runSlot)
}

// StreamChat 以流式方式执行一次对话：逐块累积正文，把每个非空正文增量交给 onDelta（可空），
// 并返回完整正文、末帧携带的 token 用量（供应商未返回时为 nil）与本次调用耗时。onDelta 返回
// 错误（如客户端断开）时立即中止并透传该错误，已累积的正文一并返回。
func StreamChat(ctx context.Context, chat ChatGenerator, messages []*schema.Message, onDelta func(string) error) (string, *schema.TokenUsage, time.Duration, error) {
	start := time.Now()
	reader, err := chat.Stream(ctx, messages)
	if err != nil {
		return "", nil, time.Since(start), errors.WithStack(err)
	}
	defer reader.Close()
	var sb strings.Builder
	var usage *schema.TokenUsage
	for {
		chunk, recvErr := reader.Recv()
		if recvErr != nil {
			if errors.Is(recvErr, io.EOF) {
				break
			}
			return sb.String(), usage, time.Since(start), errors.WithStack(recvErr)
		}
		if chunk == nil {
			continue
		}
		// 用量常落在不带正文的末帧，故先于正文过滤采集；后到的非空用量覆盖先到的。
		if chunk.ResponseMeta != nil && chunk.ResponseMeta.Usage != nil {
			usage = chunk.ResponseMeta.Usage
		}
		if chunk.Content == "" {
			continue
		}
		sb.WriteString(chunk.Content)
		if onDelta != nil {
			if derr := onDelta(chunk.Content); derr != nil {
				return sb.String(), usage, time.Since(start), errors.WithStack(derr)
			}
		}
	}
	return sb.String(), usage, time.Since(start), nil
}
