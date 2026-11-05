package sse

import (
	// 标准库
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSSEEvent_Encode 测试事件的 SSE 线格式编码：event 行 + data 行 + 空行；
// 负载为紧凑单行 JSON；ping 事件无 data 行。
func TestSSEEvent_Encode(t *testing.T) {
	delta := SSEEvent{Event: SSEEventDelta, Data: map[string]string{"delta": "hi"}}
	buf, err := delta.Encode()
	if err != nil {
		t.Fatalf("Encode(delta) error = %v", err)
	}
	if got, want := string(buf), "event: delta\ndata: {\"delta\":\"hi\"}\n\n"; got != want {
		t.Errorf("Encode(delta) = %q, want %q", got, want)
	}

	ping := SSEEvent{Event: SSEEventPing}
	buf, err = ping.Encode()
	if err != nil {
		t.Fatalf("Encode(ping) error = %v", err)
	}
	if got, want := string(buf), "event: ping\n\n"; got != want {
		t.Errorf("Encode(ping) = %q, want %q", got, want)
	}

	done := SSEEvent{Event: SSEEventDone, Data: map[string]any{"content": "c", "done": true, "valid": true}}
	buf, err = done.Encode()
	if err != nil {
		t.Fatalf("Encode(done) error = %v", err)
	}
	if got, want := string(buf), "event: done\ndata: {\"content\":\"c\",\"done\":true,\"valid\":true}\n\n"; got != want {
		t.Errorf("Encode(done) = %q, want %q", got, want)
	}
}

// TestSSEIDEvent_Encode 测试带序号事件的 SSE 线格式编码：在 event 行前恒输出
// `id: <n>` 行；无序号事件（SSEEvent）则不含 id 行。
func TestSSEIDEvent_Encode(t *testing.T) {
	ev := SSEIDEvent{ID: 42, SSEEvent: SSEEvent{Event: SSEEventDelta, Data: map[string]string{"delta": "hi"}}}
	buf, err := ev.Encode()
	if err != nil {
		t.Fatalf("Encode(id) error = %v", err)
	}
	if got, want := string(buf), "id: 42\nevent: delta\ndata: {\"delta\":\"hi\"}\n\n"; got != want {
		t.Errorf("Encode(id) = %q, want %q", got, want)
	}

	plain := SSEEvent{Event: SSEEventPing}
	buf, err = plain.Encode()
	if err != nil {
		t.Fatalf("Encode(plain) error = %v", err)
	}
	if strings.Contains(string(buf), "id:") {
		t.Errorf("Encode(plain) should not contain id line, got %q", string(buf))
	}
}

// TestSSEWriter_SendEvent 测试 SendEvent 原样透传带序号事件，写出的字节与 Encode 一致。
func TestSSEWriter_SendEvent(t *testing.T) {
	ev := SSEIDEvent{ID: 7, SSEEvent: SSEEvent{Event: SSEEventStage, Data: map[string]string{"stage": "retrieve"}}}
	rec := httptest.NewRecorder()
	w := NewSSEWriter(rec)
	if err := w.SendEvent(ev); err != nil {
		t.Fatalf("SendEvent error = %v", err)
	}
	want, err := ev.Encode()
	if err != nil {
		t.Fatalf("Encode error = %v", err)
	}
	if got := rec.Body.String(); got != string(want) {
		t.Errorf("SendEvent body = %q, want %q", got, string(want))
	}
}

// TestSSEWriter_HeadersAndEvents 测试 SSEWriter 设置流式响应头，语义化发送方法
// 产出正确的 event/data 序列。
func TestSSEWriter_HeadersAndEvents(t *testing.T) {
	rec := httptest.NewRecorder()
	w := NewSSEWriter(rec)
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", got)
	}
	if rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", rec.Header().Get("Cache-Control"))
	}

	if err := w.SendDelta(" 增量 "); err != nil {
		t.Fatalf("SendDelta error = %v", err)
	}
	if err := w.SendDone(map[string]any{"content": "c", "done": true, "valid": true}); err != nil {
		t.Fatalf("SendDone error = %v", err)
	}
	if err := w.SendError(errText("boom")); err != nil {
		t.Fatalf("SendError error = %v", err)
	}
	if err := w.Ping(); err != nil {
		t.Fatalf("Ping error = %v", err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"event: delta\ndata: {\"delta\":\" 增量 \"}\n\n",
		"event: done\ndata: {",
		`"valid":true`,
		"event: error\ndata: {\"error\":\"boom\"}\n\n",
		"event: ping\n\n",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("SSE body missing %q:\n%s", want, body)
		}
	}
}

// errText 构造一个错误（仅测试辅助，避免生产错误包依赖）。
type errText string

// Error 返回测试错误的描述文本。
func (e errText) Error() string { return string(e) }

// TestSSEWriter_SendReasoning 测试思考过程增量事件：reasoning 事件 + data {"delta": "..."}。
func TestSSEWriter_SendReasoning(t *testing.T) {
	rec := httptest.NewRecorder()
	w := NewSSEWriter(rec)
	if err := w.SendReasoning("think"); err != nil {
		t.Fatalf("SendReasoning error = %v", err)
	}
	if got, want := rec.Body.String(), "event: reasoning\ndata: {\"delta\":\"think\"}\n\n"; got != want {
		t.Errorf("SendReasoning body = %q, want %q", got, want)
	}
}

// TestSSEEvent_EncodeToolRequest 测试 tool_request 事件的 SSE 线格式编码：
// event 行为 tool_request，data 行为单行紧凑 JSON，以空行结尾。
func TestSSEEvent_EncodeToolRequest(t *testing.T) {
	ev := SSEEvent{Event: SSEEventToolRequest, Data: map[string]any{
		"callID":           "call-1",
		"apiID":            "rule.list",
		"params":           map[string]string{"name": "x"},
		"request":          map[string]string{"method": "GET", "path": "/api/v1/rules"},
		"approvalRequired": true,
	}}
	buf, err := ev.Encode()
	if err != nil {
		t.Fatalf("Encode(tool_request) error = %v", err)
	}
	got := string(buf)
	want := "event: tool_request\ndata: {\"apiID\":\"rule.list\",\"approvalRequired\":true," +
		"\"callID\":\"call-1\",\"params\":{\"name\":\"x\"}," +
		"\"request\":{\"method\":\"GET\",\"path\":\"/api/v1/rules\"}}\n\n"
	if got != want {
		t.Errorf("Encode(tool_request) = %q, want %q", got, want)
	}
	// data 行须为单行：去掉首尾以空行分隔后，data 段不含裸换行。
	dataLine := got[len("event: tool_request\n"):]
	if strings.Count(strings.TrimSuffix(dataLine, "\n\n"), "\n") != 0 {
		t.Errorf("tool_request data line contains bare newline: %q", dataLine)
	}
}

// TestSSEWriter_SendToolRequest 测试 SendToolRequest 经 httptest.NewRecorder
// 写出的完整字节内容与 Encode() 一致。
func TestSSEWriter_SendToolRequest(t *testing.T) {
	payload := map[string]any{
		"callID":           "call-2",
		"apiID":            "rule.get",
		"params":           map[string]any{"id": 7},
		"request":          map[string]string{"method": "GET", "path": "/api/v1/rules/7"},
		"approvalRequired": true,
	}
	rec := httptest.NewRecorder()
	w := NewSSEWriter(rec)
	if err := w.SendToolRequest(payload); err != nil {
		t.Fatalf("SendToolRequest error = %v", err)
	}
	want, err := (SSEEvent{Event: SSEEventToolRequest, Data: payload}).Encode()
	if err != nil {
		t.Fatalf("Encode(tool_request) error = %v", err)
	}
	if got := rec.Body.String(); got != string(want) {
		t.Errorf("SendToolRequest body = %q, want %q", got, string(want))
	}
}
