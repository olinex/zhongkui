package agentkit

import (
	"context"
	"sync"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// RunMetrics 汇总一次流式运行内的 token 用量与总耗时：
//   - token 用量由 eino 回调在每次对话模型调用结束时累加。同一 run 内工具调用多轮、知识检索
//     重排等都可能各发起一次模型调用，且流式场景下回调在独立 goroutine 中读取，故累加必须加锁；
//   - 耗时以运行开始的墙钟时刻为基准，到 Snapshot 调用时取差。用单调时钟（time.Since）即可。
//
// 同一份 Snapshot 供流式载荷与会话落库共用，保证两处数值一致。
type RunMetrics struct {
	// start 运行开始的墙钟时刻，用于计算总耗时。
	start time.Time

	// mu 保护各 token 累计字段的互斥锁。
	mu sync.Mutex
	// promptTokens 累计输入 token 数。
	promptTokens int
	// completionTokens 累计输出 token 数。
	completionTokens int
	// totalTokens 累计总 token 数。
	totalTokens int
}

// NewRunMetrics 以「当下」为起点创建一次运行的用量/耗时采集器（须在加载历史之前调用，
// 使耗时覆盖整轮回答而非仅模型推理）。
func NewRunMetrics() *RunMetrics {
	return &RunMetrics{start: time.Now()}
}

// AddUsage 累加一次模型调用的 token 用量；usage 为 nil 表示供应商未返回用量（或该回调并非
// 对话模型），此时保持原值、不报错。
func (m *RunMetrics) AddUsage(usage *model.TokenUsage) {
	if m == nil || usage == nil {
		return
	}
	m.mu.Lock()
	m.promptTokens += usage.PromptTokens
	m.completionTokens += usage.CompletionTokens
	m.totalTokens += usage.TotalTokens
	m.mu.Unlock()
}

// Snapshot 返回当前累计的 token 用量，以及从运行开始到此刻的耗时（毫秒）。
func (m *RunMetrics) Snapshot() (prompt, completion, total int, elapsedMs int64) {
	if m == nil {
		return 0, 0, 0, 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.promptTokens, m.completionTokens, m.totalTokens, time.Since(m.start).Milliseconds()
}

// Handler 构造挂在运行层 callbacks 上的回调处理器（一次运行一个实例）：
// 非流式调用走 OnEnd，流式调用走 OnEndWithStreamOutput；两条路径都只取模型回调输出的
// TokenUsage，其余组件类型经转换返回 nil 被忽略，天然只统计对话模型用量（chat 与 agentic
// 两种回调输出都支持）。
func (m *RunMetrics) Handler() callbacks.Handler {
	return callbacks.NewHandlerBuilder().
		OnEndFn(func(ctx context.Context, _ *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
			m.AddUsage(callbackTokenUsage(output))
			return ctx
		}).
		OnEndWithStreamOutputFn(func(ctx context.Context, _ *callbacks.RunInfo, output *schema.StreamReader[callbacks.CallbackOutput]) context.Context {
			// 流式用量随输出流末尾下发（usage 常在最后一帧、且 Message 可能为 nil），必须把这份
			// 回调副本抽干后才可见；抽干会阻塞到该次模型流结束，故放入独立 goroutine，避免拖住
			// 真正的流式输出。（框架要求处理器消费后关闭副本，见 drainUsage。）
			go drainUsage(output, m)
			return ctx
		}).
		Build()
}

// drainUsage 读空一次流式回调的输出副本并逐块累加其中的 token 用量。回调副本必须关闭，否则
// 整条流式管线泄漏；读到的块即使不含用量也继续读完。
func drainUsage(output *schema.StreamReader[callbacks.CallbackOutput], metrics *RunMetrics) {
	defer output.Close()
	for {
		item, err := output.Recv()
		if err != nil {
			return
		}
		metrics.AddUsage(callbackTokenUsage(item))
	}
}

// callbackTokenUsage 从回调输出中安全取出模型 token 用量：非对话模型（类型不匹配）或未携带
// 用量时返回 nil，供 AddUsage 跳过。chat 与 agentic 两种回调输出都识别。
func callbackTokenUsage(out callbacks.CallbackOutput) *model.TokenUsage {
	if chat := model.ConvCallbackOutput(out); chat != nil {
		return chat.TokenUsage
	}
	if agentic := model.ConvAgenticCallbackOutput(out); agentic != nil {
		return agentic.TokenUsage
	}
	return nil
}
