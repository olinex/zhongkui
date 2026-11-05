package audit

import (
	// 标准库
	"context"

	// 第三方
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// 本文件集中声明审核重要级评估的提示词模板：待评估调用清单由模板循环渲染，工具用途说明等
// 变量只作为数据注入，不在调用处拼接字符串。

// auditUserPromptTemplate 是重要级评估请求的用户消息模板。
const auditUserPromptTemplate = `待评估的工具调用：
{{range .Calls}}- callID: {{.CallID}}
  toolName: {{.ToolName}}
{{if .Description}}  description: {{.Description}}
{{end}}  arguments: {{.Arguments}}
{{end}}`

// auditPromptCall 是模板中的单条待评估调用数据。
type auditPromptCall struct {
	// CallID 调用唯一标识
	CallID string
	// ToolName 工具名
	ToolName string
	// Description 工具用途说明（未注册时为空）
	Description string
	// Arguments 模型给出的入参原文 JSON
	Arguments string
}

// buildPrompt 按模板构造评估请求的用户消息：逐条列出本次全部待执行调用（client 与 backend 皆可），
// 每条含 callID、工具名、工具用途说明与模型给出的入参原文 JSON（后端不做解释）。
func (e *Evaluator) buildPrompt(ctx context.Context, calls []contract.PendingCall) (*schema.Message, error) {
	promptCalls := make([]auditPromptCall, 0, len(calls))
	for _, call := range calls {
		promptCalls = append(promptCalls, auditPromptCall{
			CallID:      call.CallID,
			ToolName:    call.ToolName,
			Description: e.toolDescription(call.ToolName),
			Arguments:   call.Arguments,
		})
	}
	messages, err := schema.UserMessage(auditUserPromptTemplate).Format(ctx, map[string]any{
		"Calls": promptCalls,
	}, schema.GoTemplate)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return messages[0], nil
}
