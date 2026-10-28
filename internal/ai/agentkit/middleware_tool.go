package agentkit

import (
	"context"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"

	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// toolExecMiddleware 是所有工具最内层的行为包装：结果按 agent.maxToolResultBytes 截断，
// 非中断错误转成可读的工具结果文本（让模型自行纠错而不是中断整条流），中断错误原样透传。
// 阶段事件由 turns 的事件投影根据事件流产出，本中间件不产生展示事件。
type toolExecMiddleware[M adk.MessageType] struct {
	*adk.TypedBaseChatModelAgentMiddleware[M]
	// maxBytes 工具结果注入模型前的字节上限；非正数表示不截断
	maxBytes int
}

// newToolExecMiddleware 构造工具结果包装中间件。
func newToolExecMiddleware[M adk.MessageType](maxBytes int) adk.TypedChatModelAgentMiddleware[M] {
	return &toolExecMiddleware[M]{
		TypedBaseChatModelAgentMiddleware: &adk.TypedBaseChatModelAgentMiddleware[M]{},
		maxBytes:                          maxBytes,
	}
}

// WrapInvokableToolCall 包装同步工具调用。
func (m *toolExecMiddleware[M]) WrapInvokableToolCall(_ context.Context, endpoint adk.InvokableToolCallEndpoint, _ *adk.ToolContext) (adk.InvokableToolCallEndpoint, error) {
	return func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		result, err := endpoint(ctx, argumentsInJSON, opts...)
		if err != nil {
			if _, ok := compose.IsInterruptRerunError(err); ok {
				return "", errors.WithStack(err)
			}
			return "tool execution failed: " + err.Error(), nil
		}
		return retrieval.TruncateBytes(result, m.maxBytes), nil
	}, nil
}
