package memory

import (
	// 标准库
	"context"
	"strings"

	// 第三方
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// 本文件集中声明滚动摘要与长期记忆抽取的提示词模板：消息清单由模板循环渲染，单条正文只作为
// 变量注入，不在调用处拼接字符串。

// memoryTranscriptTemplate 是「role: content」消息清单模板，供摘要与记忆抽取复用。
const memoryTranscriptTemplate = `{{range .Messages}}{{.Role}}: {{.Content}}
{{end}}`

// memorySummaryPromptTemplate 是滚动摘要请求的用户消息模板：先给出既有摘要（无则标注（无）），
// 再逐条列出被挤出消息。
const memorySummaryPromptTemplate = `既有摘要：
{{if .Existing}}{{.Existing}}{{else}}（无）{{end}}

需并入摘要的新消息：
{{range .Messages}}{{.Role}}: {{.Content}}
{{end}}`

// promptMessage 是提示词模板中的单条消息数据。
type promptMessage struct {
	// Role 消息角色（user / assistant 等）
	Role string
	// Content 按 perMessageByteLimit 截断后的正文
	Content string
}

// buildSummaryPrompt 按模板构造滚动摘要请求的用户消息：先给出既有摘要（无则标注（无）），再逐条
// 列出被挤出消息（role: content，单条按 perMessageByteLimit 截断，控制 prompt 体积）。
func buildSummaryPrompt(ctx context.Context, existing string, evicted []*ai.AgentMessage, perMessageByteLimit int) (*schema.Message, error) {
	return renderUserMessage(ctx, memorySummaryPromptTemplate, map[string]any{
		"Existing": strings.TrimSpace(existing),
		"Messages": promptMessages(evicted, perMessageByteLimit),
	})
}

// buildMessageTranscript 按模板把消息逐条渲染为 "role: content" 的文本块，单条正文按
// perMessageByteLimit 字节截断（UTF-8 边界安全），供摘要与记忆抽取提示词复用。
func buildMessageTranscript(ctx context.Context, messages []*ai.AgentMessage, perMessageByteLimit int) (*schema.Message, error) {
	return renderUserMessage(ctx, memoryTranscriptTemplate, map[string]any{
		"Messages": promptMessages(messages, perMessageByteLimit),
	})
}

// renderUserMessage 按 GoTemplate 渲染单条 user 消息。
func renderUserMessage(ctx context.Context, tpl string, vars map[string]any) (*schema.Message, error) {
	messages, err := schema.UserMessage(tpl).Format(ctx, vars, schema.GoTemplate)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return messages[0], nil
}

// promptMessages 把持久化消息转换为提示词模板数据：跳过 nil，正文去首尾空白后按字节截断。
func promptMessages(messages []*ai.AgentMessage, perMessageByteLimit int) []promptMessage {
	out := make([]promptMessage, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		out = append(out, promptMessage{
			Role:    string(message.Role),
			Content: retrieval.TruncateBytes(strings.TrimSpace(string(message.Content.Value())), perMessageByteLimit),
		})
	}
	return out
}

// messageTranscriptBytes 返回消息清单模板产物的字节数（role + ": " + content + "\n"），
// 供压缩后占用估算使用；与模板同源，避免为了量长度而构造提示词字符串。
func messageTranscriptBytes(messages []*ai.AgentMessage, perMessageByteLimit int) int {
	total := 0
	for _, message := range promptMessages(messages, perMessageByteLimit) {
		total += len(message.Role) + len(": ") + len(message.Content) + len("\n")
	}
	return total
}
