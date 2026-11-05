package agentkit

import (
	"context"
	"encoding/json"
)

// Automation 是无人值守运行上下文：只在周期报告等后台运行中注入，使工具层不再依赖前端交互。
//
// 它携带回调函数，不可 gob 序列化，因此不得放入 agentkit.Request、turns.ChatItem 或任何会写入
// checkpoint 的结构；它只通过 context 在单次运行内传递。
type Automation struct {
	// InvokeAPI 执行一次已校验的 api_invoke 调用（method / path / params 均为后端解析后的规范值）。
	// 为空表示服务端调用器未配置，api_invoke 将回灌「不可用」提示。
	InvokeAPI func(ctx context.Context, method, path string, params json.RawMessage) (string, error)
}

// automationContextKey 是无人值守上下文在 context 中的不可导出键类型，避免与其他包冲突。
type automationContextKey struct{}

// WithAutomation 把无人值守上下文写入 ctx；automation 为 nil 时原样返回 ctx。
func WithAutomation(ctx context.Context, automation *Automation) context.Context {
	if automation == nil {
		return ctx
	}
	return context.WithValue(ctx, automationContextKey{}, automation)
}

// AutomationFrom 读取当前 ctx 中的无人值守上下文。
func AutomationFrom(ctx context.Context) (*Automation, bool) {
	if ctx == nil {
		return nil, false
	}
	automation, ok := ctx.Value(automationContextKey{}).(*Automation)
	if !ok || automation == nil {
		return nil, false
	}
	return automation, true
}

const (
	// AskUserUnattendedNotice 是无人值守时 ask_user 的降级提示：明确告知模型无人作答并给出继续策略。
	AskUserUnattendedNotice = "当前处于后台无人值守运行模式，没有用户可作答，无法收集任何答案。" +
		"请不要再次调用 ask_user，直接依据技能指引与本次计划参数继续执行；不得编造任何信息，" +
		"对确实缺失的信息按降级方案处理，并在结果中明确标注缺失项与所采用的降级方案。"
	// ApiInvokeUnavailableNotice 是无人值守且服务端调用器未配置时 api_invoke 的回灌文本。
	ApiInvokeUnavailableNotice = "api_invoke is unavailable in unattended mode: " +
		"no server-side invoker is configured; continue without this call and note the missing data"
	// ApiInvokeFailedPrefix 是无人值守时 api_invoke 执行失败的回灌文本前缀。
	ApiInvokeFailedPrefix = "api_invoke failed: "
)
