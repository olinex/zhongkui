package sse

import (
	// stdlib
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSSEEvent_Encode 测试事件的 SSE 线格式编码：event 行 + data 行 + 空行；
// 负载为紧凑单行 JSON；ping 事件无 data 行。
// TestSSEEvent_Encode verifies the SSE wire-format encoding: an event line plus
// a single compact data line plus a blank line; the ping event has no data line.
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

// TestSSEWriter_HeadersAndEvents 测试 SSEWriter 设置流式响应头，语义化发送方法
// 产出正确的 event/data 序列。
// TestSSEWriter_HeadersAndEvents verifies SSEWriter sets the streaming headers
// and the semantic send methods emit the expected event/data sequences.
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
// errText builds an error (a test helper only, avoiding production error
// package dependencies).
type errText string

func (e errText) Error() string { return string(e) }
