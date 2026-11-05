package msgops

import (
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/pkg/llm"
)

// ToolCall 包含跨表示和流式分片使用的通用函数工具
// 调用字段。
type ToolCall struct {
	// ID 工具调用 ID。
	ID string
	// Name 函数名。
	Name string
	// Args 参数 JSON 文本。
	Args string
	// Index 调用在流式分片中的序号。
	Index int
}

// ToolResult 包含通用的函数工具结果字段。
type ToolResult struct {
	// ID 对应的工具调用 ID。
	ID string
	// Name 工具名。
	Name string
	// Content 工具结果文本。
	Content string
}

// NewUser 为 M 构造一条用户消息。
func NewUser[M adk.MessageType](text string) M {
	if APIModeOf[M]() == llm.APIModeResponses {
		return any(schema.UserAgenticMessage(text)).(M)
	}
	return any(schema.UserMessage(text)).(M)
}

// NewSystem 为 M 构造一条系统消息。
func NewSystem[M adk.MessageType](text string) M {
	if APIModeOf[M]() == llm.APIModeResponses {
		return any(schema.SystemAgenticMessage(text)).(M)
	}
	return any(schema.SystemMessage(text)).(M)
}

// NewAssistant 构造一条 assistant 消息，可附带函数
// 工具调用。
func NewAssistant[M adk.MessageType](text string, calls []ToolCall) M {
	if APIModeOf[M]() == llm.APIModeResponses {
		blocks := make([]*schema.ContentBlock, 0, len(calls)+1)
		if text != "" {
			blocks = append(blocks, normalizeAgenticContentBlock(schema.NewContentBlock(&schema.AssistantGenText{Text: text})))
		}
		for _, call := range calls {
			blocks = append(blocks, normalizeAgenticContentBlock(schema.NewContentBlock(&schema.FunctionToolCall{
				CallID:    call.ID,
				Name:      call.Name,
				Arguments: call.Args,
			})))
		}
		return any(&schema.AgenticMessage{
			Role:          schema.AgenticRoleTypeAssistant,
			ContentBlocks: blocks,
		}).(M)
	}

	schemaCalls := make([]schema.ToolCall, 0, len(calls))
	for _, call := range calls {
		idx := call.Index
		schemaCalls = append(schemaCalls, schema.ToolCall{
			ID:       call.ID,
			Index:    &idx,
			Function: schema.FunctionCall{Name: call.Name, Arguments: call.Args},
		})
	}
	return any(schema.AssistantMessage(text, schemaCalls)).(M)
}

// ReplaceAssistantText 返回把 assistant 生成正文替换为 text 的消息副本：思考、工具调用、
// 多模态分片与块元数据原样保留。用于剥离模型输出中仅供机器读取的段落后再持久化。
// msg 为 nil 或未知表示时原样返回。
func ReplaceAssistantText[M adk.MessageType](msg M, text string) M {
	switch m := any(msg).(type) {
	case *schema.Message:
		if m == nil {
			return msg
		}
		out := *m
		out.Content = text
		return any(&out).(M)
	case *schema.AgenticMessage:
		if m == nil {
			return msg
		}
		out := *m
		blocks := make([]*schema.ContentBlock, 0, len(m.ContentBlocks)+1)
		replaced := false
		for _, block := range m.ContentBlocks {
			if block == nil {
				continue
			}
			if block.Type != schema.ContentBlockTypeAssistantGenText {
				blocks = append(blocks, block)
				continue
			}
			if replaced || text == "" {
				continue
			}
			copied := *block
			copied.AssistantGenText = &schema.AssistantGenText{Text: text}
			blocks = append(blocks, &copied)
			replaced = true
		}
		if !replaced && text != "" {
			blocks = append(blocks, schema.NewContentBlock(&schema.AssistantGenText{Text: text}))
		}
		out.ContentBlocks = blocks
		return any(&out).(M)
	default:
		return msg
	}
}

// NewToolResult 为 M 构造一条工具结果消息。
func NewToolResult[M adk.MessageType](id, name, content string) M {
	if APIModeOf[M]() == llm.APIModeResponses {
		var blocks []*schema.FunctionToolResultContentBlock
		if content != "" {
			blocks = []*schema.FunctionToolResultContentBlock{{
				Type: schema.FunctionToolResultContentBlockTypeText,
				Text: &schema.UserInputText{Text: content},
			}}
		}
		return any(&schema.AgenticMessage{
			Role: schema.AgenticRoleTypeUser,
			ContentBlocks: []*schema.ContentBlock{
				schema.NewContentBlock(&schema.FunctionToolResult{
					CallID:  id,
					Name:    name,
					Content: blocks,
				}),
			},
		}).(M)
	}
	return any(schema.ToolMessage(content, id, schema.WithToolName(name))).(M)
}
