package msgops

import (
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// Text 返回消息中面向用户的文本，用于历史回放与通用展示。
//
// 对 *schema.Message：优先返回 Content；Content 为空时依次拼接
// UserInputMultiContent 与 AssistantGenMultiContent 中的文本分片。
// 对 *schema.AgenticMessage：按 ContentBlocks 的顺序拼接用户输入文本、
// 助手生成文本与函数工具结果文本（以换行分隔）。
// msg 为 nil 或未知类型时返回空串；本函数不区分角色。
func Text[M adk.MessageType](msg M) string {
	switch m := any(msg).(type) {
	case *schema.Message:
		return messageText(m)
	case *schema.AgenticMessage:
		return agenticText(m)
	default:
		return ""
	}
}

// AssistantText 返回消息中由模型生成的 assistant 文本。
//
// 对 *schema.Message：优先返回 Content；为空时拼接
// AssistantGenMultiContent 中的文本分片（不含用户输入与工具结果）。
// 对 *schema.AgenticMessage：拼接全部 AssistantGenText 块的文本，不含
// reasoning、用户输入与函数工具结果。msg 为 nil 时返回空串。
func AssistantText[M adk.MessageType](msg M) string {
	switch m := any(msg).(type) {
	case *schema.Message:
		if m == nil {
			return ""
		}
		return messageAssistantText(m)
	case *schema.AgenticMessage:
		if m == nil {
			return ""
		}
		var parts []string
		for _, block := range m.ContentBlocks {
			if block != nil && block.Type == schema.ContentBlockTypeAssistantGenText && block.AssistantGenText != nil {
				parts = append(parts, block.AssistantGenText.Text)
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

// AssistantDeltaText 返回一个流式分片中的 assistant 文本。
//
// 流式分片与完整消息共用同一套文本字段（*schema.Message.Content /
// AssistantGenMultiContent、*schema.AgenticMessage.AssistantGenText），
// 因此当前直接委托 AssistantText；保留独立入口是为了让流式调用点语义
// 明确，并为后续按分片处理留出扩展点。
func AssistantDeltaText[M adk.MessageType](msg M) string {
	return AssistantText(msg)
}

// ReasoningText 返回消息或流式分片中的 reasoning（思考过程）文本。
//
// 对 *schema.Message：返回 ReasoningContent。
// 对 *schema.AgenticMessage：拼接全部 Reasoning 块的 Text；不含正文与
// 工具调用。msg 为 nil 或未知类型时返回空串。
func ReasoningText[M adk.MessageType](msg M) string {
	switch m := any(msg).(type) {
	case *schema.Message:
		if m == nil {
			return ""
		}
		return m.ReasoningContent
	case *schema.AgenticMessage:
		if m == nil {
			return ""
		}
		var parts []string
		for _, block := range m.ContentBlocks {
			if block != nil && block.Type == schema.ContentBlockTypeReasoning && block.Reasoning != nil {
				parts = append(parts, block.Reasoning.Text)
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

// HasContent 表示消息是否携带值得持久化的内容。
//
// 对 *schema.Message：Content、ReasoningContent、ToolCalls、
// MultiContent 或 AssistantGenMultiContent 任一非空即返回 true。
// 对 *schema.AgenticMessage：ContentBlocks 非空即返回 true（每个块自身
// 已是结构化内容）。nil 消息与未知类型返回 false。该判断只回答「是否有
// 内容」，不校验内容是否有效。
func HasContent[M adk.MessageType](msg M) bool {
	switch m := any(msg).(type) {
	case *schema.Message:
		if m == nil {
			return false
		}
		return m.Content != "" ||
			m.ReasoningContent != "" ||
			len(m.ToolCalls) > 0 ||
			len(m.MultiContent) > 0 ||
			len(m.AssistantGenMultiContent) > 0
	case *schema.AgenticMessage:
		return m != nil && len(m.ContentBlocks) > 0
	default:
		return false
	}
}

// UserText 仅对 user 角色的消息返回文本。
//
// 对 *schema.Message：role 必须为 schema.User，返回 messageText。
// 对 *schema.AgenticMessage：role 必须为 AgenticRoleTypeUser，拼接全部
// UserInputText 块并以换行分隔。非 user 角色、nil 消息与未知类型返回
// 空串；工具结果消息（agentic 的 user 角色可能承载工具结果）不会在这里
// 被误当成用户提问，因为其块类型不是 UserInputText。
func UserText[M adk.MessageType](msg M) string {
	switch m := any(msg).(type) {
	case *schema.Message:
		if m == nil || m.Role != schema.User {
			return ""
		}
		return messageText(m)
	case *schema.AgenticMessage:
		if m == nil || m.Role != schema.AgenticRoleTypeUser {
			return ""
		}
		var parts []string
		for _, block := range m.ContentBlocks {
			if block != nil && block.Type == schema.ContentBlockTypeUserInputText && block.UserInputText != nil {
				parts = append(parts, block.UserInputText.Text)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

// ToolCalls 从消息或流式分片中提取函数工具调用，并归一化为 ToolCall。
//
// 对 *schema.Message：读取 m.ToolCalls，保留原始顺序；Index 取
// ToolCall.Index（nil 时按 0 处理）。
// 对 *schema.AgenticMessage：读取 FunctionToolCall 内容块，保留块顺序；
// Index 取块的 StreamingMeta.Index（无流式元数据时按 0 处理）。
// 不存在的调用返回 nil；未知类型返回 nil。调用方不应假设 Index 唯一，
// 流式分片下同一调用可能跨多个分片、由上层按 Index 归并。
func ToolCalls[M adk.MessageType](msg M) []ToolCall {
	switch m := any(msg).(type) {
	case *schema.Message:
		if m == nil {
			return nil
		}
		out := make([]ToolCall, 0, len(m.ToolCalls))
		for _, tc := range m.ToolCalls {
			idx := 0
			if tc.Index != nil {
				idx = *tc.Index
			}
			out = append(out, ToolCall{
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Args:  tc.Function.Arguments,
				Index: idx,
			})
		}
		return out
	case *schema.AgenticMessage:
		if m == nil {
			return nil
		}
		var out []ToolCall
		for _, block := range m.ContentBlocks {
			if block == nil || block.Type != schema.ContentBlockTypeFunctionToolCall || block.FunctionToolCall == nil {
				continue
			}
			idx := 0
			if block.StreamingMeta != nil {
				idx = block.StreamingMeta.Index
			}
			out = append(out, ToolCall{
				ID:    block.FunctionToolCall.CallID,
				Name:  block.FunctionToolCall.Name,
				Args:  block.FunctionToolCall.Arguments,
				Index: idx,
			})
		}
		return out
	default:
		return nil
	}
}

// ToolResults 从消息或流式分片中提取函数工具结果，并归一化为 ToolResult。
//
// 对 *schema.Message：role 为 schema.Tool 或 ToolCallID 非空时视为工具
// 结果，返回单条记录（ID=ToolCallID、Name=ToolName、Content=messageText）。
// 对 *schema.AgenticMessage：读取 FunctionToolResult 内容块，按块顺序返回
// 每条结果；Content 由 functionToolResultText 拼接文本分片，非文本分片
// 回退为 block.String()。无工具结果、nil 消息与未知类型返回 nil。
func ToolResults[M adk.MessageType](msg M) []ToolResult {
	switch m := any(msg).(type) {
	case *schema.Message:
		if m == nil || (m.Role != schema.Tool && m.ToolCallID == "") {
			return nil
		}
		return []ToolResult{{
			ID:      m.ToolCallID,
			Name:    m.ToolName,
			Content: messageText(m),
		}}
	case *schema.AgenticMessage:
		if m == nil {
			return nil
		}
		var out []ToolResult
		for _, block := range m.ContentBlocks {
			if block == nil || block.Type != schema.ContentBlockTypeFunctionToolResult || block.FunctionToolResult == nil {
				continue
			}
			out = append(out, ToolResult{
				ID:      block.FunctionToolResult.CallID,
				Name:    block.FunctionToolResult.Name,
				Content: functionToolResultText(block.FunctionToolResult),
			})
		}
		return out
	default:
		return nil
	}
}

// RoleLabel 返回便于阅读的角色标签，用于历史与工具块的 caption。
//
// 对 *schema.Message：User→You、Assistant→Agent、Tool→Tool、
// System→System，其余非空角色原样返回。
// 对 *schema.AgenticMessage：User 角色若实际承载工具结果则显示为 Tool，
// 否则为 You；Assistant→Agent、System→System，其余非空角色原样返回。
// nil 消息与未知类型统一回退为 Agent，保证展示层永远有标签。
func RoleLabel[M adk.MessageType](msg M) string {
	switch m := any(msg).(type) {
	case *schema.Message:
		if m == nil {
			return "Agent"
		}
		switch m.Role {
		case schema.User:
			return "You"
		case schema.Assistant:
			return "Agent"
		case schema.Tool:
			return "Tool"
		case schema.System:
			return "System"
		default:
			if m.Role != "" {
				return string(m.Role)
			}
			return "Agent"
		}
	case *schema.AgenticMessage:
		if m == nil {
			return "Agent"
		}
		switch m.Role {
		case schema.AgenticRoleTypeUser:
			if len(ToolResults(msg)) > 0 {
				return "Tool"
			}
			return "You"
		case schema.AgenticRoleTypeAssistant:
			return "Agent"
		case schema.AgenticRoleTypeSystem:
			return "System"
		default:
			if m.Role != "" {
				return string(m.Role)
			}
			return "Agent"
		}
	default:
		return "Agent"
	}
}

// messageText 抽取 *schema.Message 的展示文本：Content 非空时直接返回；
// 否则先拼接 UserInputMultiContent 的文本分片，再拼接
// AssistantGenMultiContent 的文本分片，并以换行分隔。nil 消息返回空串。
func messageText(msg *schema.Message) string {
	if msg == nil {
		return ""
	}
	if msg.Content != "" {
		return msg.Content
	}
	var parts []string
	for _, part := range msg.UserInputMultiContent {
		if part.Type == schema.ChatMessagePartTypeText && part.Text != "" {
			parts = append(parts, part.Text)
		}
	}
	for _, part := range msg.AssistantGenMultiContent {
		if part.Type == schema.ChatMessagePartTypeText && part.Text != "" {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// messageAssistantText 抽取 *schema.Message 中由模型生成的 assistant 文本：
// Content 非空时直接返回；否则仅拼接 AssistantGenMultiContent 的文本分片，
// 不包含用户输入。nil 消息返回空串。
func messageAssistantText(msg *schema.Message) string {
	if msg == nil {
		return ""
	}
	if msg.Content != "" {
		return msg.Content
	}
	var parts []string
	for _, part := range msg.AssistantGenMultiContent {
		if part.Type == schema.ChatMessagePartTypeText && part.Text != "" {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "")
}

// agenticText 按 ContentBlocks 顺序抽取 *schema.AgenticMessage 的展示文本：
// 用户输入文本、助手生成文本与函数工具结果文本各自取出后以换行拼接；
// reasoning、服务端工具与 MCP 等其它块不参与。nil 消息返回空串。
func agenticText(msg *schema.AgenticMessage) string {
	if msg == nil {
		return ""
	}
	var parts []string
	for _, block := range msg.ContentBlocks {
		if block == nil {
			continue
		}
		switch block.Type {
		case schema.ContentBlockTypeUserInputText:
			if block.UserInputText != nil {
				parts = append(parts, block.UserInputText.Text)
			}
		case schema.ContentBlockTypeAssistantGenText:
			if block.AssistantGenText != nil {
				parts = append(parts, block.AssistantGenText.Text)
			}
		case schema.ContentBlockTypeFunctionToolResult:
			if block.FunctionToolResult != nil {
				parts = append(parts, functionToolResultText(block.FunctionToolResult))
			}
		}
	}
	return strings.Join(parts, "\n")
}

// functionToolResultText 把一条 *schema.FunctionToolResult 的内容分片拼成
// 展示文本：文本分片直接取 Text；图片/音频/视频/文件等非文本分片回退为
// block.String()，以便历史回放至少能显示占位说明。分片间以换行分隔；
// result 为 nil 时返回空串。
func functionToolResultText(result *schema.FunctionToolResult) string {
	if result == nil {
		return ""
	}
	var parts []string
	for _, block := range result.Content {
		if block == nil {
			continue
		}
		switch block.Type {
		case schema.FunctionToolResultContentBlockTypeText:
			if block.Text != nil {
				parts = append(parts, block.Text.Text)
			}
		default:
			parts = append(parts, strings.TrimSpace(block.String()))
		}
	}
	return strings.Join(parts, "\n")
}
