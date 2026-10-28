package agentkit

import (
	"context"

	"zhongkuisec.com/zhongkui/internal/ai/contract"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
)

// TrimMessagesByBytes 按字节预算从最旧开始丢弃整条消息，直到剩余消息的正文字节总数不超过
// maxBytes；至少保留最新一条（只丢整条消息，故不会切断多字节字符），maxBytes 非正数或
// 未超预算时原样返回。
func TrimMessagesByBytes(messages []*modelsai.AgentMessage, maxBytes int) []*modelsai.AgentMessage {
	if maxBytes <= 0 || len(messages) == 0 {
		return messages
	}
	total := 0
	for _, message := range messages {
		if message != nil {
			total += len(message.Content.Value())
		}
	}
	if total <= maxBytes {
		return messages
	}
	cut := 0
	for cut < len(messages)-1 && total > maxBytes {
		if messages[cut] != nil {
			total -= len(messages[cut].Content.Value())
		}
		cut++
	}
	return messages[cut:]
}

// EvictedForSummary 返回「窗口之外（位于历史末条之前 session.maxContextMessages 条之外）
// 且尚未被摘要覆盖（Seq > summary_upto_seq）」的消息，供滚动摘要与压缩圈定被挤出的历史。
// history 须为按 Seq 升序的完整会话消息；窗口容量非正数时视为不做窗口裁剪、返回 nil。
func (r *Runtime) EvictedForSummary(session *modelsai.AgentSession, history []*modelsai.AgentMessage) []*modelsai.AgentMessage {
	if session == nil || len(history) == 0 {
		return nil
	}
	window := r.params.Int("session.maxContextMessages")
	if window <= 0 || len(history) <= window {
		return nil
	}
	windowStart := len(history) - window
	out := make([]*modelsai.AgentMessage, 0, windowStart)
	for _, message := range history[:windowStart] {
		if message == nil {
			continue
		}
		if message.Seq <= uint64(session.SummaryUptoSeq) {
			continue
		}
		out = append(out, message)
	}
	return out
}

// RefreshSummary 在加载窗口历史后推进滚动摘要：条数达到 memory.summaryMinEvictedMessages 且
// Deps.Memory 非 nil 时用 flash 重写摘要并落库，同时把新摘要写回会话副本使本次生成即用新摘要。
// 摘要失败仅告警、不中断生成、不推进游标；不受 memory.enabled 开关约束（该开关只管长期记忆）。
func (r *Runtime) RefreshSummary(ctx context.Context, session *modelsai.AgentSession, history []*modelsai.AgentMessage) string {
	return r.RefreshSummaryWithMetrics(ctx, session, history, 0)
}

// RefreshSummaryWithMetrics 与 RefreshSummary 语义一致，额外接受摘要推进前的上下文占用
// （tokensBefore）以写入压缩检查点度量；无真实测量时传 0。
func (r *Runtime) RefreshSummaryWithMetrics(ctx context.Context, session *modelsai.AgentSession, history []*modelsai.AgentMessage, tokensBefore int) string {
	if session == nil {
		return ""
	}
	existing := string(session.Summary.Value())
	if r.memory == nil {
		return existing
	}
	evicted := r.EvictedForSummary(session, history)
	minEvicted := r.params.Int("memory.summaryMinEvictedMessages")
	if len(evicted) == 0 || len(evicted) < minEvicted {
		return existing
	}
	result, err := r.memory.CompactStream(ctx, session, evicted, "rolling",
		contract.CompactionMetrics{TokensBefore: tokensBefore, FoldedCount: len(evicted)}, nil)
	if err != nil {
		r.warnf(ctx, "agentkit: rolling summary failed, keeping the previous summary: %v", err)
		return existing
	}
	if result == nil {
		return existing
	}
	return result.Summary
}

// CompressSession 执行一次流式上下文压缩：把 evicted 折叠进滚动摘要并落库，按需抽取长期记忆，
// 过程经 report 依次上报 compress_start / compress_delta / compress_done。失败仅告警（上报
// foldedCount=0 的 compress_done），不返回错误、不推进游标。reason 取 auto / manual。
// slot 为本次运行的有效档位，为空时依次回退到会话档位 session.TargetModel 与参数
// agent.defaultSlot，据此取该档位的上下文容量。
//
// 旧实现从会话存储读取压缩前占用作为度量；agentkit 的 Deps 不持有会话存储，故此签名版本以
// tokensBefore=0 记录度量，需要真实占用时使用 CompressSessionWithMetrics。
func (r *Runtime) CompressSession(ctx context.Context, session *modelsai.AgentSession, evicted []*modelsai.AgentMessage, reason string, slot modelsai.Slot, report contract.CompressReporter) *contract.CompactResult {
	return r.CompressSessionWithMetrics(ctx, session, evicted, reason, slot,
		contract.CompactionMetrics{TokensBefore: 0, FoldedCount: len(evicted)}, report)
}

// CompressSessionWithMetrics 与 CompressSession 语义一致，但接受完整的压缩度量（含压缩前占用）。
func (r *Runtime) CompressSessionWithMetrics(ctx context.Context, session *modelsai.AgentSession, evicted []*modelsai.AgentMessage, reason string, slot modelsai.Slot, metrics contract.CompactionMetrics, report contract.CompressReporter) *contract.CompactResult {
	if session == nil || r.memory == nil {
		return nil
	}
	if slot == "" {
		slot = session.TargetModel
	}
	if slot == "" {
		slot = modelsai.Slot(r.params.String("agent.defaultSlot"))
	}
	limit := r.ContextLimit(slot)
	threshold := r.params.Float("session.compactThresholdRatio")
	if report != nil {
		if err := report(contract.CompressEvent{
			Type:      contract.CompressEventStart,
			Reason:    reason,
			Tokens:    metrics.TokensBefore,
			Limit:     limit,
			Threshold: threshold,
		}); err != nil {
			return nil
		}
	}
	if len(evicted) == 0 {
		if report != nil {
			report(contract.CompressEvent{Type: contract.CompressEventDone, FoldedCount: 0})
		}
		return &contract.CompactResult{Summary: string(session.Summary.Value())}
	}
	metrics.FoldedCount = len(evicted)
	result, err := r.memory.CompactStream(ctx, session, evicted, reason, metrics, func(delta string) error {
		if report == nil {
			return nil
		}
		return report(contract.CompressEvent{Type: contract.CompressEventDelta, Delta: delta})
	})
	if err != nil {
		r.warnf(ctx, "agentkit: session compression failed, keeping the previous summary: %v", err)
		if report != nil {
			report(contract.CompressEvent{Type: contract.CompressEventDone, FoldedCount: 0})
		}
		return nil
	}
	if report != nil {
		report(contract.CompressEvent{
			Type:           contract.CompressEventDone,
			Reason:         reason,
			TokensBefore:   metrics.TokensBefore,
			TokensAfter:    result.TokensAfter,
			FoldedCount:    len(evicted),
			Summary:        result.Summary,
			SummaryUptoSeq: session.SummaryUptoSeq,
		})
	}
	return result
}
