// Package sse 提供 Server-Sent Events（SSE）流式响应的编码与写手封装：事件线格式
// 编码（SSEEvent.Encode/WriteSSE）与语义化发送方法（SSEWriter 的增量/完成/错误/心跳），
// 供 HTTP 流式接口（如 AI 助手对话流）复用。不依赖任何业务模块。
// Package sse provides the encoding and writer wrapper of Server-Sent Events (SSE)
// streaming responses: the wire-format encoding (SSEEvent.Encode/WriteSSE) and the
// semantic send methods (SSEWriter delta / done / error / heartbeat), reused by HTTP
// streaming endpoints (e.g. the AI assistant dialogue stream). It depends on no
// business module.
package sse

import (
	// stdlib
	"encoding/json"
	"net/http"
	"strings"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// SSEEventValue 是 SSE 事件类型值：SSEEvent.Event 的取值空间（独立类型，禁止与
// 普通字符串混用，保证事件类型枚举的类型安全）。
// SSEEventValue is the SSE event-type value: the value space of SSEEvent.Event (a
// dedicated type, kept apart from plain strings for type-safe event typing).
type SSEEventValue string

// SSE 事件类型常量：SSE 流上事件的 event 行取值。前端按事件类型分发解析：
// delta 累积正文、stage 呈现执行阶段的状态与用量、done 呈现结构化产物与引用、
// error 中断提示、ping 保活。
// SSE event type constants: the event-line values of the SSE stream. The frontend
// dispatches by event type: delta accumulates the content, stage presents the status and
// usage of an execution stage, done presents the structured artifact and citations, error
// aborts with a hint, ping keeps alive.
const (
	// SSEEventDelta 增量文本事件（data 为 {"delta": "..."}，可带 {"stage": "..."}）
	// SSEEventDelta the delta-text event (data is {"delta": "..."}, optionally with {"stage": "..."})
	SSEEventDelta SSEEventValue = "delta"
	// SSEEventStage 阶段事件（data 为调用方的阶段状态负载，如阶段开始/完成/失败的耗时与用量）
	// SSEEventStage the stage event (data is the caller's stage-status payload, e.g. the
	// elapsed time and usage of a stage start / completion / failure)
	SSEEventStage SSEEventValue = "stage"
	// SSEEventDone 完成事件（data 为调用方产物负载，如完整对话产物）
	// SSEEventDone the done event (data is the caller's artifact payload, e.g. the
	// full dialogue artifact)
	SSEEventDone SSEEventValue = "done"
	// SSEEventError 错误事件（data 为 {"error": "..."}）
	// SSEEventError the error event (data is {"error": "..."})
	SSEEventError SSEEventValue = "error"
	// SSEEventPing 心跳事件（无 data 行，保持代理/网关连接）
	// SSEEventPing the heartbeat event (no data line, keeping proxy/gateway connections alive)
	SSEEventPing SSEEventValue = "ping"
)

// SSEEvent 是 SSE 流上的一条事件：event 行携带类型，data 行携带 JSON 负载。
// SSEEvent is a single event on the SSE stream: the event line carries the type,
// the data line carries the JSON payload.
type SSEEvent struct {
	// Event 事件类型（见 SSEEvent* 常量）
	// Event the event type (see the SSEEvent* constants)
	Event SSEEventValue `json:"event"`
	// Data 事件负载，经 JSON 序列化写入 data 行；nil 时不输出 data 行
	// Data the event payload, JSON-serialized into the data line; nil emits no data line
	Data any `json:"data,omitempty"`
}

// Encode 将事件编码为 SSE 线格式（event: <type>\n + data: <json>\n + 空行）。
// data 负载经紧凑 JSON 序列化（Go 的 encoding/json 对 Marshaler 输出同样做
// 压缩），始终为单行，符合 SSE 的 data 行规范。
// Encode serializes the event into the SSE wire format (event: <type>\n plus
// data: <json>\n plus a blank line). The data payload is compact JSON (Go's
// encoding/json compacts Marshaler output too), always a single line per the
// SSE data-line spec.
func (e SSEEvent) Encode() ([]byte, error) {
	var sb strings.Builder
	if e.Event != "" {
		sb.WriteString("event: ")
		sb.WriteString(string(e.Event))
		sb.WriteString("\n")
	}
	if e.Data != nil {
		payload, err := json.Marshal(e.Data)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		sb.WriteString("data: ")
		sb.Write(payload)
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	return []byte(sb.String()), nil
}

// WriteSSE 向响应写入一条 SSE 事件并刷新（响应头须先经 NewSSEWriter 设置）。
// 返回事件编码或写入失败的错误。
// WriteSSE writes one SSE event to the response and flushes it (the headers must
// be set by NewSSEWriter first). It returns the encode or write error.
func WriteSSE(w http.ResponseWriter, ev SSEEvent) error {
	buf, err := ev.Encode()
	if err != nil {
		return errors.WithStack(err)
	}
	if _, err := w.Write(buf); err != nil {
		return errors.WithStack(err)
	}
	flush(w)
	return nil
}

// SSEWriter 是 SSE 响应写手的便捷封装：构造时设置 text/event-stream 响应头，
// 提供增量/完成/错误/心跳的语义化发送方法（内部统一复用 WriteSSE）。
// SSEWriter is a convenience wrapper around the SSE response: it sets the
// text/event-stream headers at construction and provides semantic send methods
// for delta / done / error / heartbeat events (reusing WriteSSE internally).
type SSEWriter struct {
	w http.ResponseWriter
}

// NewSSEWriter 构造 SSEWriter 并设置 text/event-stream 响应头（no-cache、
// keep-alive、禁止网关缓冲，保证事件即时送达）。
// NewSSEWriter constructs an SSEWriter and sets the text/event-stream headers
// (no-cache, keep-alive, gateway buffering disabled, delivering events promptly).
func NewSSEWriter(w http.ResponseWriter) *SSEWriter {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	return &SSEWriter{w: w}
}

// SendDelta 发送一段增量文本（delta 事件）。
// SendDelta sends a delta text (delta event).
func (s *SSEWriter) SendDelta(delta string) error {
	return WriteSSE(s.w, SSEEvent{Event: SSEEventDelta, Data: map[string]string{"delta": delta}})
}

// SendStageDelta 发送带阶段标识的增量文本（delta 事件，data 为
// {"stage": "...", "delta": "..."}），供多阶段流式过程（如检索试运行）把增量
// 归属到具体阶段。
// SendStageDelta sends a delta text tagged with its stage (delta event, data is
// {"stage": "...", "delta": "..."}), letting a multi-stage streaming process (e.g. the
// retrieval dry-run) attribute deltas to a specific stage.
func (s *SSEWriter) SendStageDelta(stage string, delta string) error {
	return WriteSSE(s.w, SSEEvent{Event: SSEEventDelta, Data: map[string]string{"stage": stage, "delta": delta}})
}

// SendStage 发送阶段事件（stage 事件，payload 为调用方的阶段状态负载）。
// SendStage sends a stage event (stage event, payload is the caller's stage-status payload).
func (s *SSEWriter) SendStage(payload any) error {
	return WriteSSE(s.w, SSEEvent{Event: SSEEventStage, Data: payload})
}

// SendDone 发送完成事件（payload 为调用方结构化产物，任意可 JSON 序列化负载）。
// SendDone sends the done event (payload is the caller's structured artifact, any
// JSON-serializable value).
func (s *SSEWriter) SendDone(payload any) error {
	return WriteSSE(s.w, SSEEvent{Event: SSEEventDone, Data: payload})
}

// SendError 发送错误事件（错误信息写入 data 行；err 为 nil 时输出空串）。
// SendError sends the error event (the message goes into the data line; a nil
// err outputs an empty string).
func (s *SSEWriter) SendError(err error) error {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	return WriteSSE(s.w, SSEEvent{Event: SSEEventError, Data: map[string]string{"error": msg}})
}

// Ping 发送心跳事件（无负载，保持代理/网关连接不被超时断开）。
// Ping sends a heartbeat event (no payload, keeping proxy/gateway connections
// from being timed out).
func (s *SSEWriter) Ping() error {
	return WriteSSE(s.w, SSEEvent{Event: SSEEventPing})
}

// flush 在响应支持 http.Flusher 时刷新，保证事件即时送达客户端。
// flush flushes the response when it supports http.Flusher, delivering events to
// the client immediately.
func flush(w http.ResponseWriter) {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
