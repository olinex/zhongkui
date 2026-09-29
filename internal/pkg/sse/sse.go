// Package sse 提供 Server-Sent Events（SSE）流式响应的编码与写手封装：事件线格式
// 编码（SSEEvent.Encode/WriteSSE）与语义化发送方法（SSEWriter 的增量/完成/错误/心跳），
// 供 HTTP 流式接口（如 AI 助手对话流）复用。不依赖任何业务模块。
package sse

import (
	// 标准库
	"encoding/json"
	"net/http"
	"strings"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// SSEEventValue 是 SSE 事件类型值：SSEEvent.Event 的取值空间（独立类型，禁止与
// 普通字符串混用，保证事件类型枚举的类型安全）。
type SSEEventValue string

// SSE 事件类型常量：SSE 流上事件的 event 行取值。前端按事件类型分发解析：
// delta 累积正文、stage 呈现执行阶段的状态与用量、done 呈现结构化产物与引用、
// error 中断提示、ping 保活。
const (
	// SSEEventDelta 增量文本事件（data 为 {"delta": "..."}，可带 {"stage": "..."}）
	SSEEventDelta SSEEventValue = "delta"
	// SSEEventStage 阶段事件（data 为调用方的阶段状态负载，如阶段开始/完成/失败的耗时与用量）
	SSEEventStage SSEEventValue = "stage"
	// SSEEventDone 完成事件（data 为调用方产物负载，如完整对话产物）
	SSEEventDone SSEEventValue = "done"
	// SSEEventError 错误事件（data 为 {"error": "..."}）
	SSEEventError SSEEventValue = "error"
	// SSEEventPing 心跳事件（无 data 行，保持代理/网关连接）
	SSEEventPing SSEEventValue = "ping"
)

// SSEEvent 是 SSE 流上的一条事件：event 行携带类型，data 行携带 JSON 负载。
type SSEEvent struct {
	// Event 事件类型（见 SSEEvent* 常量）
	Event SSEEventValue `json:"event"`
	// Data 事件负载，经 JSON 序列化写入 data 行；nil 时不输出 data 行
	Data any `json:"data,omitempty"`
}

// Encode 将事件编码为 SSE 线格式（event: <type>\n + data: <json>\n + 空行）。
// data 负载经紧凑 JSON 序列化（Go 的 encoding/json 对 Marshaler 输出同样做
// 压缩），始终为单行，符合 SSE 的 data 行规范。
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
type SSEWriter struct {
	// w 底层 HTTP 响应写手，事件经其写出并刷新。
	w http.ResponseWriter
}

// NewSSEWriter 构造 SSEWriter 并设置 text/event-stream 响应头（no-cache、
// keep-alive、禁止网关缓冲，保证事件即时送达）。
func NewSSEWriter(w http.ResponseWriter) *SSEWriter {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	return &SSEWriter{w: w}
}

// SendDelta 发送一段增量文本（delta 事件）。
func (s *SSEWriter) SendDelta(delta string) error {
	return WriteSSE(s.w, SSEEvent{Event: SSEEventDelta, Data: map[string]string{"delta": delta}})
}

// SendStageDelta 发送带阶段标识的增量文本（delta 事件，data 为
// {"stage": "...", "delta": "..."}），供多阶段流式过程（如检索试运行）把增量
// 归属到具体阶段。
func (s *SSEWriter) SendStageDelta(stage string, delta string) error {
	return WriteSSE(s.w, SSEEvent{Event: SSEEventDelta, Data: map[string]string{"stage": stage, "delta": delta}})
}

// SendStage 发送阶段事件（stage 事件，payload 为调用方的阶段状态负载）。
func (s *SSEWriter) SendStage(payload any) error {
	return WriteSSE(s.w, SSEEvent{Event: SSEEventStage, Data: payload})
}

// SendDone 发送完成事件（payload 为调用方结构化产物，任意可 JSON 序列化负载）。
func (s *SSEWriter) SendDone(payload any) error {
	return WriteSSE(s.w, SSEEvent{Event: SSEEventDone, Data: payload})
}

// SendError 发送错误事件（错误信息写入 data 行；err 为 nil 时输出空串）。
func (s *SSEWriter) SendError(err error) error {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	return WriteSSE(s.w, SSEEvent{Event: SSEEventError, Data: map[string]string{"error": msg}})
}

// Ping 发送心跳事件（无负载，保持代理/网关连接不被超时断开）。
func (s *SSEWriter) Ping() error {
	return WriteSSE(s.w, SSEEvent{Event: SSEEventPing})
}

// flush 在响应支持 http.Flusher 时刷新，保证事件即时送达客户端。
func flush(w http.ResponseWriter) {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
