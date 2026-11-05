package msgops

import (
	"encoding/json"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
)

// DecodeForModel 把持久化的原生消息负载解码为 M。存储的类型
// 可能与 M 不同（例如某个槽位的 api_mode 变更后）；消息会经由
// 文本/工具调用/工具结果字段转换，从而让历史在
// 跨表示时仍可用。
func DecodeForModel[M adk.MessageType](payload []byte, stored llm.APIMode) (M, error) {
	switch stored {
	case llm.APIModeChat:
		var msg schema.Message
		if err := json.Unmarshal(payload, &msg); err != nil {
			var zero M
			return zero, errors.WithStack(err)
		}
		return convertChatTo[M](&msg)
	case llm.APIModeResponses:
		var msg schema.AgenticMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			var zero M
			return zero, errors.WithStack(err)
		}
		return convertAgenticTo[M](&msg)
	default:
		var zero M
		return zero, errors.Newf("msgops: unknown stored message kind %q", stored)
	}
}

func convertChatTo[M adk.MessageType](msg *schema.Message) (M, error) {
	if msg == nil {
		var zero M
		return zero, errors.New("msgops: nil chat message")
	}
	if APIModeOf[M]() == llm.APIModeChat {
		return any(msg).(M), nil
	}

	out := &schema.AgenticMessage{Extra: cloneMap(msg.Extra)}
	switch msg.Role {
	case schema.System:
		out.Role = schema.AgenticRoleTypeSystem
		out.ContentBlocks = []*schema.ContentBlock{schema.NewContentBlock(&schema.UserInputText{Text: messageText(msg)})}
	case schema.User:
		out.Role = schema.AgenticRoleTypeUser
		out.ContentBlocks = []*schema.ContentBlock{schema.NewContentBlock(&schema.UserInputText{Text: messageText(msg)})}
	case schema.Tool:
		out.Role = schema.AgenticRoleTypeUser
		out.ContentBlocks = []*schema.ContentBlock{schema.NewContentBlock(&schema.FunctionToolResult{
			CallID: msg.ToolCallID,
			Name:   msg.ToolName,
			Content: []*schema.FunctionToolResultContentBlock{{
				Type: schema.FunctionToolResultContentBlockTypeText,
				Text: &schema.UserInputText{Text: messageText(msg)},
			}},
		})}
	default:
		out.Role = schema.AgenticRoleTypeAssistant
		if text := messageAssistantText(msg); text != "" {
			out.ContentBlocks = append(out.ContentBlocks, schema.NewContentBlock(&schema.AssistantGenText{Text: text}))
		}
		if msg.ReasoningContent != "" {
			out.ContentBlocks = append([]*schema.ContentBlock{schema.NewContentBlock(&schema.Reasoning{Text: msg.ReasoningContent})}, out.ContentBlocks...)
		}
		for _, tc := range msg.ToolCalls {
			out.ContentBlocks = append(out.ContentBlocks, schema.NewContentBlock(&schema.FunctionToolCall{
				CallID:    tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			}))
		}
	}
	return any(out).(M), nil
}

func convertAgenticTo[M adk.MessageType](msg *schema.AgenticMessage) (M, error) {
	if msg == nil {
		var zero M
		return zero, errors.New("msgops: nil agentic message")
	}
	if APIModeOf[M]() == llm.APIModeResponses {
		return any(normalizeAgenticMessage(msg)).(M), nil
	}

	switch msg.Role {
	case schema.AgenticRoleTypeSystem:
		return any(schema.SystemMessage(agenticText(msg))).(M), nil
	case schema.AgenticRoleTypeUser:
		if results := ToolResults(any(msg).(*schema.AgenticMessage)); len(results) > 0 {
			first := results[0]
			return any(schema.ToolMessage(first.Content, first.ID, schema.WithToolName(first.Name))).(M), nil
		}
		return any(schema.UserMessage(agenticText(msg))).(M), nil
	default:
		calls := ToolCalls(any(msg).(*schema.AgenticMessage))
		schemaCalls := make([]schema.ToolCall, 0, len(calls))
		for _, call := range calls {
			idx := call.Index
			schemaCalls = append(schemaCalls, schema.ToolCall{
				ID:       call.ID,
				Index:    &idx,
				Function: schema.FunctionCall{Name: call.Name, Arguments: call.Args},
			})
		}
		out := schema.AssistantMessage(AssistantText(any(msg).(*schema.AgenticMessage)), schemaCalls)
		out.ReasoningContent = ReasoningText(any(msg).(*schema.AgenticMessage))
		return any(out).(M), nil
	}
}
