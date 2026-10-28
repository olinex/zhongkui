package agentkit

import (
	"context"

	"github.com/cloudwego/eino/adk"
)

// runContextKey[M] 是 RunContext[M] 的上下文键。每个 M 使用不同的键
// 类型，确保 chat 和 agentic 的运行上下文永不冲突。
type runContextKey[M adk.MessageType] struct{}

// WithRunContext 在 Runner.Run 之前把运行上下文注入 ctx。GenModelInput
// 读取它以组装模型输入。RunContext 只在全新运行时注入；恢复的运行没有
// 运行上下文，因此 GenModelInput 会原样传递还原后的消息。
func WithRunContext[M adk.MessageType](ctx context.Context, rc RunContext[M]) context.Context {
	return context.WithValue(ctx, runContextKey[M]{}, rc)
}

// RunContextFrom 提取由 WithRunContext 注入的运行上下文。
func RunContextFrom[M adk.MessageType](ctx context.Context) (RunContext[M], bool) {
	value := ctx.Value(runContextKey[M]{})
	if value == nil {
		return RunContext[M]{}, false
	}
	rc, ok := value.(RunContext[M])
	return rc, ok
}
