package msgops

import (
	"maps"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/pkg/llm"
)

const (
	arkItemIDKey        = "ark-item-id"
	arkItemStatusKey    = "ark-item-status"
	openAIItemIDKey     = "openai-item-id"
	openAIItemStatusKey = "openai-item-status"
	itemStatusCompleted = "completed"
)

// NormalizeForSession 返回可安全交给服务商持久化的消息。
// Agentic 模型实现会把 Responses API 的条目 ID/状态附加到
// 内容块上；重放服务商原生输出条目时需要这些 ID。
// 仅用于流式的元数据会被丢弃。
func NormalizeForSession[M adk.MessageType](msg M) M {
	if APIModeOf[M]() != llm.APIModeResponses {
		return msg
	}
	agenticMsg, ok := any(msg).(*schema.AgenticMessage)
	if !ok || agenticMsg == nil {
		return msg
	}
	return any(normalizeAgenticMessage(agenticMsg)).(M)
}

// NormalizeMessagesForModelInput 在把存储的消息传回同类
// Runner/ChatModel 之前对其进行预处理。
func NormalizeMessagesForModelInput[M adk.MessageType](messages []M) []M {
	out := make([]M, 0, len(messages))
	for _, msg := range messages {
		out = append(out, NormalizeForSession(msg))
	}
	return out
}

func normalizeAgenticMessage(msg *schema.AgenticMessage) *schema.AgenticMessage {
	if msg == nil {
		return nil
	}

	out := &schema.AgenticMessage{
		Role:          msg.Role,
		ContentBlocks: make([]*schema.ContentBlock, 0, len(msg.ContentBlocks)),
		Extra:         cloneMap(msg.Extra),
	}

	for _, block := range msg.ContentBlocks {
		normalized := normalizeAgenticContentBlock(block)
		if normalized != nil {
			out.ContentBlocks = append(out.ContentBlocks, normalized)
		}
	}

	return out
}

func normalizeAgenticContentBlock(block *schema.ContentBlock) *schema.ContentBlock {
	if block == nil {
		return nil
	}

	out := *block
	out.Extra = normalizeAgenticBlockExtra(block.Type, block.Extra)
	out.StreamingMeta = nil
	return &out
}

func normalizeAgenticBlockExtra(blockType schema.ContentBlockType, extra map[string]any) map[string]any {
	out := make(map[string]any, len(extra)+2)
	maps.Copy(out, extra)

	if needsCompletedStatus(blockType) {
		out[arkItemStatusKey] = itemStatusCompleted
		out[openAIItemStatusKey] = itemStatusCompleted
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func needsCompletedStatus(blockType schema.ContentBlockType) bool {
	switch blockType {
	case schema.ContentBlockTypeReasoning,
		schema.ContentBlockTypeAssistantGenText,
		schema.ContentBlockTypeFunctionToolCall:
		return true
	default:
		return false
	}
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	maps.Copy(out, in)
	return out
}
