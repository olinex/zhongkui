// 本文件承载 AI 助手领域事件的 SSE 写手：/chat、/render、/action、/events 均输出领域事件。

package ai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"zhongkuisec.com/zhongkui/internal/ai/assistant"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/turns"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// assistantKeepaliveInterval 是事件流空闲时下发 SSE 注释行的心跳周期。
const assistantKeepaliveInterval = 15 * time.Second

// errAssistantClientClosed 标记 SSE 写失败（客户端断开），用于提前结束转发而不中止回合。
var errAssistantClientClosed = errors.New("assistant: client connection closed")

// assistantSSEWriter 把事件写成 SSE（id: <seq> + data: <json>）并立即刷新。
type assistantSSEWriter struct {
	w       gin.ResponseWriter
	flusher http.Flusher
	broken  bool
}

// newAssistantSSEWriter 写出 SSE 响应头并返回写手；调用方须先完成全部校验。
func newAssistantSSEWriter(c *gin.Context) *assistantSSEWriter {
	h := c.Writer.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	flusher, _ := c.Writer.(http.Flusher)
	return &assistantSSEWriter{w: c.Writer, flusher: flusher}
}

// WriteEvent 写出一条事件；写失败后写手永久标记为断开。快照与连接级控制事件的 seq
// 不推进客户端续播游标，故不写 `id:` 行。
func (w *assistantSSEWriter) WriteEvent(event assistant.Event) error {
	if w == nil || w.broken {
		return errAssistantClientClosed
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return errors.WithStack(err)
	}
	var line string
	if isAssistantControlEvent(event.Type) {
		line = fmt.Sprintf("data: %s\n\n", payload)
	} else {
		line = fmt.Sprintf("id: %d\ndata: %s\n\n", event.Seq, payload)
	}
	if _, err := fmt.Fprint(w.w, line); err != nil {
		w.broken = true
		return errors.WithStack(err)
	}
	if w.flusher != nil {
		w.flusher.Flush()
	}
	return nil
}

// isAssistantControlEvent 报告事件是否属于连接级控制 / 历史快照（其 seq 不用于续播）。
func isAssistantControlEvent(eventType assistant.EventType) bool {
	switch eventType {
	case assistant.EventTypeSnapshotBegin,
		assistant.EventTypeSnapshotMessage,
		assistant.EventTypeSnapshotEnd,
		assistant.EventTypeResumeBegin,
		assistant.EventTypeResumeReset:
		return true
	default:
		return false
	}
}

// WriteControl 写出续播控制事件（resume.begin / resume.reset）：游标沿用客户端最后看到的
// seq，不推进运行内的事件游标。
func (w *assistantSSEWriter) WriteControl(sessionID uint, eventType assistant.EventType, seq int, runID string, payload any) error {
	return w.WriteEvent(assistant.Event{
		V:         assistant.Version,
		Seq:       seq,
		Type:      eventType,
		SessionID: sessionID,
		RunID:     runID,
		TS:        time.Now().UTC(),
		Payload:   payload,
	})
}

// Ping 写出一条 SSE 注释行作为心跳。
func (w *assistantSSEWriter) Ping() error {
	if w == nil || w.broken {
		return errAssistantClientClosed
	}
	if _, err := fmt.Fprint(w.w, ": ping\n\n"); err != nil {
		w.broken = true
		return errors.WithStack(err)
	}
	if w.flusher != nil {
		w.flusher.Flush()
	}
	return nil
}

// streamAssistantEvents 把一轮的事件透传为 SSE：续播时先写 resume.begin（或
// resume.reset 后结束），再写已缓冲回放，最后持续转发实时事件；通道关闭或客户端断开即结束。
func streamAssistantEvents(c *gin.Context, stream *turns.Stream) {
	if stream == nil {
		return
	}
	if stream.Close != nil {
		defer stream.Close()
	}
	writer := newAssistantSSEWriter(c)

	if stream.Resume != nil {
		if stream.Resume.Reset != "" {
			_ = writer.WriteControl(stream.SessionID, assistant.EventTypeResumeReset, stream.Resume.FromSeq, stream.Resume.RunID,
				assistant.ResumeResetPayload{Reason: stream.Resume.Reset})
			return
		}
		if err := writer.WriteControl(stream.SessionID, assistant.EventTypeResumeBegin, stream.Resume.FromSeq, stream.Resume.RunID,
			assistant.ResumeBeginPayload{RunID: stream.Resume.RunID, FromSeq: stream.Resume.FromSeq}); err != nil {
			return
		}
	}

	for _, item := range stream.Replay {
		if item.Event == nil {
			continue
		}
		if err := writer.WriteEvent(*item.Event); err != nil {
			return
		}
	}
	if stream.Events == nil {
		return
	}
	ticker := time.NewTicker(assistantKeepaliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case item, ok := <-stream.Events:
			if !ok {
				return
			}
			if item.Event == nil {
				continue
			}
			if err := writer.WriteEvent(*item.Event); err != nil {
				return
			}
		case <-ticker.C:
			if err := writer.Ping(); err != nil {
				return
			}
		}
	}
}

// streamCompactEvents 把一次上下文压缩投影为 message.compress 事件：开始（running）、
// 摘要增量（draft）与完成 / 失败。压缩不新建助手气泡，事件携带锚点助手消息 ID。
func streamCompactEvents(c *gin.Context, sessionID, messageID uint, folded int, run func(contract.CompressReporter)) {
	writer := newAssistantSSEWriter(c)
	emitter := assistant.NewEmitter(sessionID, "")
	emitter.SetMessageID(messageID)
	done := false
	report := func(ev contract.CompressEvent) error {
		var state assistant.CompressState
		switch ev.Type {
		case contract.CompressEventStart:
			state = assistant.CompressState{
				Phase:        assistant.CompressPhaseRunning,
				Reason:       ev.Reason,
				TokensBefore: ev.Tokens,
			}
		case contract.CompressEventDelta:
			state = assistant.CompressState{Phase: assistant.CompressPhaseRunning, Draft: ev.Delta}
		case contract.CompressEventDone:
			phase := assistant.CompressPhaseDone
			if ev.FoldedCount == 0 && folded > 0 {
				// 有待折叠消息却未产出摘要：压缩失败。
				phase = assistant.CompressPhaseFailed
			}
			state = assistant.CompressState{
				Phase:        phase,
				Reason:       ev.Reason,
				TokensBefore: ev.TokensBefore,
				FoldedCount:  ev.FoldedCount,
				TokensAfter:  ev.TokensAfter,
				Summary:      ev.Summary,
			}
			done = true
		default:
			return nil
		}
		return writer.WriteEvent(emitter.Next(assistant.EventTypeMessageCompress, assistant.MessageCompressPayload{Compress: state}))
	}
	run(report)
	if !done {
		// 压缩未上报终态（例如记忆服务未装配）：显式收敛为失败，避免前端停留在 running。
		_ = writer.WriteEvent(emitter.Next(assistant.EventTypeMessageCompress, assistant.MessageCompressPayload{
			Compress: assistant.CompressState{Phase: assistant.CompressPhaseFailed, Reason: "manual"},
		}))
	}
}
