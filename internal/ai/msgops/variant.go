package msgops

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
)

// IsNil 表示泛型消息值是否为 nil。
func IsNil[M adk.MessageType](msg M) bool {
	var zero M
	return any(msg) == any(zero)
}

// VariantRoleLabel 返回展示标签，且不会消费流。
func VariantRoleLabel[M adk.MessageType](mv *adk.TypedMessageVariant[M]) string {
	if mv == nil {
		return "Agent"
	}
	if APIModeOf[M]() == llm.APIModeResponses {
		switch mv.AgenticRole {
		case schema.AgenticRoleTypeUser:
			return "You"
		case schema.AgenticRoleTypeSystem:
			return "System"
		case schema.AgenticRoleTypeAssistant, "":
			return "Agent"
		default:
			return string(mv.AgenticRole)
		}
	}
	switch mv.Role {
	case schema.User:
		return "You"
	case schema.Assistant, "":
		return "Agent"
	case schema.Tool:
		return "Tool"
	case schema.System:
		return "System"
	default:
		return string(mv.Role)
	}
}

// VariantIsToolResult 表示消息变体是否携带工具输出。
func VariantIsToolResult[M adk.MessageType](mv *adk.TypedMessageVariant[M]) bool {
	if mv == nil {
		return false
	}
	if APIModeOf[M]() == llm.APIModeChat {
		if mv.Role == schema.Tool {
			return true
		}
		if !IsNil(mv.Message) {
			return len(ToolResults(mv.Message)) > 0
		}
		return false
	}
	if !IsNil(mv.Message) {
		return len(ToolResults(mv.Message)) > 0
	}
	return mv.AgenticRole == schema.AgenticRoleTypeUser
}

// DrainToolResult 消费工具结果变体，返回其文本和调用 ID。
func DrainToolResult[M adk.MessageType](mv *adk.TypedMessageVariant[M]) (content, id, name string) {
	if mv == nil {
		return "", "", ""
	}
	if mv.IsStreaming && mv.MessageStream != nil {
		var buf strings.Builder
		for {
			chunk, err := mv.MessageStream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				break
			}
			for _, result := range ToolResults(chunk) {
				if id == "" {
					id = result.ID
				}
				if name == "" {
					name = result.Name
				}
				buf.WriteString(result.Content)
			}
			if text := Text(chunk); text != "" && len(ToolResults(chunk)) == 0 {
				buf.WriteString(text)
			}
		}
		return buf.String(), id, name
	}
	if IsNil(mv.Message) {
		return "", "", ""
	}
	results := ToolResults(mv.Message)
	if len(results) == 0 {
		return Text(mv.Message), "", ""
	}
	for _, result := range results {
		if id == "" {
			id = result.ID
		}
		if name == "" {
			name = result.Name
		}
		content += result.Content
	}
	return content, id, name
}

// ConcatChunks 使用 eino schema 辅助函数合并流式消息分片。
func ConcatChunks[M adk.MessageType](chunks []M) (M, error) {
	var zero M
	if len(chunks) == 0 {
		return zero, errors.New("no chunks to concat")
	}
	if len(chunks) == 1 {
		return chunks[0], nil
	}

	if APIModeOf[M]() == llm.APIModeResponses {
		msgs := make([]*schema.AgenticMessage, 0, len(chunks))
		for _, chunk := range chunks {
			msg, ok := any(chunk).(*schema.AgenticMessage)
			if !ok || msg == nil {
				return zero, errors.Newf("unexpected agentic chunk type %T", chunk)
			}
			msgs = append(msgs, msg)
		}
		merged, err := schema.ConcatAgenticMessages(msgs)
		if err != nil {
			return zero, errors.WithStack(err)
		}
		return any(merged).(M), nil
	}

	msgs := make([]*schema.Message, 0, len(chunks))
	for _, chunk := range chunks {
		msg, ok := any(chunk).(*schema.Message)
		if !ok || msg == nil {
			return zero, errors.Newf("unexpected message chunk type %T", chunk)
		}
		msgs = append(msgs, msg)
	}
	merged, err := schema.ConcatMessages(msgs)
	if err != nil {
		return zero, errors.WithStack(err)
	}
	return any(merged).(M), nil
}

// UnmarshalMessage 把一行 JSON 消息反序列化为 M。
func UnmarshalMessage[M adk.MessageType](data []byte) (M, error) {
	if APIModeOf[M]() == llm.APIModeResponses {
		var msg schema.AgenticMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			var zero M
			return zero, errors.WithStack(err)
		}
		return NormalizeForSession(any(&msg).(M)), nil
	}
	var msg schema.Message
	err := json.Unmarshal(data, &msg)
	return any(&msg).(M), errors.WithStack(err)
}
