// Package sse 提供 Server-Sent Events（SSE）流式响应的编码与写手封装：事件线格式
// 编码（SSEEvent / SSEIDEvent 的 Encode、WriteSSE）与语义化发送方法（SSEWriter 的
// 增量/完成/错误/心跳），供 HTTP 流式接口（如 AI 助手对话流）复用。不依赖任何业务模块。
package sse

import (
	// 标准库
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// SSEEventValue 是 SSE 事件类型值：SSEEvent.Event 的取值空间（独立类型，禁止与
// 普通字符串混用，保证事件类型枚举的类型安全）。
type SSEEventValue string

// SSE 事件类型常量：SSE 流上事件的 event 行取值。前端按事件类型分发解析：
// delta 累积正文、reasoning 累积思考过程、stage 呈现执行阶段的状态与用量、
// block 呈现有序内容块的创建与收敛、done 呈现结构化产物与引用、error 中断提示、
// ping 保活、tool_request 挂起等待前端执行客户端工具，compress_start/compress_delta/
// compress_done 呈现会话上下文压缩的开始、摘要增量与完成。
const (
	// SSEEventDelta 增量文本事件（data 为 {"delta": "..."}，可带 {"stage": "..."}）
	SSEEventDelta SSEEventValue = "delta"
	// SSEEventReasoning 思考过程增量事件（data 为 {"delta": "..."}，模型返回的思考内容增量）
	SSEEventReasoning SSEEventValue = "reasoning"
	// SSEEventStage 阶段事件（data 为调用方的阶段状态负载，如阶段开始/完成/失败的耗时与用量）
	SSEEventStage SSEEventValue = "stage"
	// SSEEventDone 完成事件（data 为调用方产物负载，如完整对话产物）
	SSEEventDone SSEEventValue = "done"
	// SSEEventError 错误事件（data 为 {"error": "..."}）
	SSEEventError SSEEventValue = "error"
	// SSEEventPing 心跳事件（无 data 行，保持代理/网关连接）
	SSEEventPing SSEEventValue = "ping"
	// SSEEventToolRequest 工具执行请求事件：表示当前流已挂起、等待前端执行并审批
	// 客户端工具调用；调用方不应期待本次流再出现 done 事件。
	SSEEventToolRequest SSEEventValue = "tool_request"
	// SSEEventCompressStart 上下文压缩开始事件（data 含压缩原因、占用 token、容量与阈值）
	SSEEventCompressStart SSEEventValue = "compress_start"
	// SSEEventCompressDelta 上下文压缩摘要增量事件（data 为 {"delta": "..."}，摘要生成的增量文本）
	SSEEventCompressDelta SSEEventValue = "compress_delta"
	// SSEEventCompressDone 上下文压缩完成事件（data 含压缩前后占用、折叠条数与滚动摘要游标）
	SSEEventCompressDone SSEEventValue = "compress_done"
	// SSEEventState 会话交互状态事件（data 为 {"state": "...", "substate": "..."}，下发
	// 后端推导出的会话交互主状态与子状态）
	SSEEventState SSEEventValue = "state"
	// SSEEventBlock 内容块事件（data 含 {seq, kind, payload} 的有序内容块，块创建与收敛各下发
	// 一次；前端据 {seq, kind} 归并增量到当前开放块）
	SSEEventBlock SSEEventValue = "block"
)

// SSEEvent 是 SSE 流上的一条无序号事件：event 行携带类型，data 行携带 JSON 负载。
// 不参与断线续播的流（如检索试运行、审批续跑）使用本类型，线格式不含 id 行。
type SSEEvent struct {
	// Event 事件类型（见 SSEEvent* 常量）
	Event SSEEventValue `json:"event"`
	// Data 事件负载，经 JSON 序列化写入 data 行；nil 时不输出 data 行
	Data any `json:"data,omitempty"`
}

// Encode 将无序号事件编码为 SSE 线格式（event: <type>\n + data: <json>\n + 空行）。
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

// SSEIDEvent 是 SSE 流上的一条带序号事件：在无序号事件前追加 id 行（单调自增序号），
// 供断线续播时以 ?after= 指定回放起点。可续播的流（如 AI 助手驻留运行）使用本类型，
// 序号由运行事件的自增序列给出，恒不为 0。
type SSEIDEvent struct {
	// ID 事件序号（单调自增，供 Last-Event-ID / ?after= 续播定位）
	ID uint64 `json:"id"`
	// SSEEvent 事件本体（event 行 + data 行）
	SSEEvent
}

// Encode 将带序号事件编码为 SSE 线格式（id: <n>\n + event: <type>\n + data: <json>\n + 空行）。
// id 行恒输出：序号是续播的定位依据，调用方必须提供有效序号（见 SSEIDEvent 说明）。
func (e SSEIDEvent) Encode() ([]byte, error) {
	body, err := e.SSEEvent.Encode()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var sb strings.Builder
	sb.WriteString("id: ")
	sb.WriteString(strconv.FormatUint(e.ID, 10))
	sb.WriteString("\n")
	sb.Write(body)
	return []byte(sb.String()), nil
}

// WriteSSE 向响应写入一条无序号 SSE 事件并刷新（响应头须先经 NewSSEWriter 设置）。
// 返回事件编码或写入失败的错误。
func WriteSSE(w http.ResponseWriter, ev SSEEvent) error {
	buf, err := ev.Encode()
	if err != nil {
		return errors.WithStack(err)
	}
	return writeFlush(w, buf)
}

// writeFlush 写出已编码的事件字节并刷新；返回写入失败的错误。
func writeFlush(w http.ResponseWriter, buf []byte) error {
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

// SendEvent 原样发送一条已构造的带序号 SSE 事件（含 id 行），供续播场景把带序号的
// 事件透传给客户端；其余语义化方法（SendDelta/SendStage/...）不携带序号。
func (s *SSEWriter) SendEvent(ev SSEIDEvent) error {
	buf, err := ev.Encode()
	if err != nil {
		return errors.WithStack(err)
	}
	return writeFlush(s.w, buf)
}

// SendDelta 发送一段增量文本（delta 事件）。
func (s *SSEWriter) SendDelta(delta string) error {
	return WriteSSE(s.w, SSEEvent{Event: SSEEventDelta, Data: map[string]string{"delta": delta}})
}

// SendReasoning 发送一段思考过程增量文本（reasoning 事件，data 为 {"delta": "..."}）。
func (s *SSEWriter) SendReasoning(delta string) error {
	return WriteSSE(s.w, SSEEvent{Event: SSEEventReasoning, Data: map[string]string{"delta": delta}})
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

// SendState 发送会话交互状态事件（state 事件，payload 由调用方提供，须含描述交互主状态的
// state 与子状态的 substate 字段），供前端以单一状态源驱动界面。
func (s *SSEWriter) SendState(payload any) error {
	return WriteSSE(s.w, SSEEvent{Event: SSEEventState, Data: payload})
}

// SendToolRequest 发送工具执行请求事件（tool_request 事件，表示本次流挂起等待
// 前端执行并审批客户端工具调用；发送后调用方不应再发送 done）。payload 由调用方
// 提供，须包含 callID、接口 ID、参数、已合成的请求描述符与是否需审批。
func (s *SSEWriter) SendToolRequest(payload any) error {
	return WriteSSE(s.w, SSEEvent{Event: SSEEventToolRequest, Data: payload})
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
