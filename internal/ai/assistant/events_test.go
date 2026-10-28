package assistant

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// fixedNow 返回固定时间，保证事件时间戳断言可重复。
func fixedNow() time.Time {
	return time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
}

// TestEmitterSequence 验证实时事件游标、信封字段与块载荷类型。
func TestEmitterSequence(t *testing.T) {
	emitter := NewEmitter(7, "run-1")
	emitter.now = fixedNow
	emitter.SetMessageID(42)

	first := emitter.BlockCreated(1, BlockKindReasoning, ReasoningBlockPayload{Text: "思考"})
	second := emitter.BlockDelta(1, BlockKindReasoning, "增量")

	if first.Validate() != nil || second.Validate() != nil {
		t.Fatalf("事件校验失败: %v / %v", first.Validate(), second.Validate())
	}
	if first.Seq != 1 || second.Seq != 2 {
		t.Fatalf("seq = %d / %d, want 1 / 2", first.Seq, second.Seq)
	}
	if first.V != Version || first.SessionID != 7 || first.RunID != "run-1" || first.MessageID != 42 {
		t.Fatalf("信封字段不符: %+v", first)
	}
	if !first.TS.Equal(fixedNow()) {
		t.Fatalf("ts = %v", first.TS)
	}
	created, ok := first.Payload.(BlockCreatedPayload)
	if !ok {
		t.Fatalf("payload 类型 = %T", first.Payload)
	}
	if created.BlockSeq != 1 || created.Kind != BlockKindReasoning {
		t.Fatalf("created payload = %+v", created)
	}
	delta, ok := second.Payload.(BlockDeltaPayload)
	if !ok || delta.Text != "增量" {
		t.Fatalf("delta payload = %+v (%T)", second.Payload, second.Payload)
	}
}

// TestSnapshotEmitter 验证快照事件不携带 runID，并按消息回填信封 messageID。
func TestSnapshotEmitter(t *testing.T) {
	emitter := NewSnapshotEmitter(9)
	emitter.now = fixedNow

	begin := emitter.SnapshotBegin()
	message := emitter.SnapshotMessage(MessageSnapshot{ID: 88, Role: "assistant"})
	end := emitter.SnapshotEnd()

	if begin.RunID != "" || message.RunID != "" || end.RunID != "" {
		t.Fatalf("快照事件不应携带 runID: %+v %+v %+v", begin, message, end)
	}
	if message.MessageID != 88 {
		t.Fatalf("snapshot messageID = %d, want 88", message.MessageID)
	}
	if end.MessageID != 0 {
		t.Fatalf("snapshot end messageID = %d, want 0", end.MessageID)
	}
	if begin.Seq != 1 || message.Seq != 2 || end.Seq != 3 {
		t.Fatalf("snapshot seq = %d / %d / %d", begin.Seq, message.Seq, end.Seq)
	}
}

// TestEventJSON 验证信封与工具调用事件的 JSON 字段名。
func TestEventJSON(t *testing.T) {
	emitter := NewEmitter(3, "run-2")
	emitter.now = fixedNow
	event := emitter.ToolCallUpdated(5, ToolCallRecord{
		CallID:    "call-1",
		ToolName:  "knowledge_search",
		Status:    ToolCallStatusDone,
		ElapsedMs: 12,
	})

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(data)
	for _, key := range []string{`"v":1`, `"seq":1`, `"type":"tool.call.updated"`, `"sessionID":3`, `"runID":"run-2"`, `"blockSeq":5`, `"callID":"call-1"`} {
		if !strings.Contains(text, key) {
			t.Fatalf("JSON 缺少 %s: %s", key, text)
		}
	}
	if strings.Contains(text, `"messageID"`) {
		t.Fatalf("未知消息 ID 不应下发 messageID: %s", text)
	}
}

// TestEventValidate 验证协议版本、游标与必填字段的基础校验。
func TestEventValidate(t *testing.T) {
	event := Event{V: Version, Seq: 1, Type: EventTypeSnapshotEnd, SessionID: 1}
	if err := event.Validate(); err != nil {
		t.Fatalf("合法事件不应报错: %v", err)
	}
	bad := []Event{
		{V: 2, Seq: 1, Type: EventTypeSnapshotEnd, SessionID: 1},
		{V: Version, Seq: 0, Type: EventTypeSnapshotEnd, SessionID: 1},
		{V: Version, Seq: 1, SessionID: 1},
		{V: Version, Seq: 1, Type: EventTypeSnapshotEnd},
	}
	for i, item := range bad {
		if err := item.Validate(); err == nil {
			t.Fatalf("第 %d 个非法事件未被拒绝: %+v", i, item)
		}
	}
}
