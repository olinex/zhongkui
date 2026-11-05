package ai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"zhongkuisec.com/zhongkui/internal/ai/assistant"
	"zhongkuisec.com/zhongkui/internal/ai/turns"
)

func newAssistantTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/ai/agent/1/events?after=0", nil)
	return c, rec
}

func parseAssistantLines(t *testing.T, body string) []assistant.Event {
	t.Helper()
	blocks := strings.Split(strings.TrimSpace(body), "\n\n")
	out := make([]assistant.Event, 0, len(blocks))
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" || strings.HasPrefix(block, ":") {
			continue
		}
		lines := strings.Split(block, "\n")
		payload := ""
		for _, line := range lines {
			if strings.HasPrefix(line, "data: ") {
				payload = strings.TrimPrefix(line, "data: ")
			}
		}
		if payload == "" {
			continue
		}
		var event assistant.Event
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			t.Fatalf("decode event: %v", err)
		}
		out = append(out, event)
	}
	return out
}

func TestStreamAssistantEventsResumeReplay(t *testing.T) {
	c, rec := newAssistantTestContext(t)
	created := assistant.Event{
		V:         assistant.Version,
		Seq:       3,
		Type:      assistant.EventTypeBlockCreated,
		SessionID: 1,
		RunID:     "run-1",
		Payload:   assistant.BlockCreatedPayload{BlockSeq: 1, Kind: assistant.BlockKindContent},
	}
	stream := &turns.Stream{
		SessionID: 1,
		Replay:    []turns.StreamItem{{Event: &created}},
		Resume:    &turns.ResumeInfo{RunID: "run-1", FromSeq: 2},
	}
	streamAssistantEvents(c, stream)

	body := rec.Body.String()
	if !strings.Contains(body, "id: 3") {
		t.Fatalf("缺少事件 id 行: %q", body)
	}
	events := parseAssistantLines(t, body)
	if len(events) != 2 {
		t.Fatalf("事件数 = %d, body = %q", len(events), body)
	}
	if events[0].Type != assistant.EventTypeResumeBegin {
		t.Fatalf("首个事件 = %s", events[0].Type)
	}
	begin, ok := events[0].Payload.(map[string]any)
	if !ok || begin["runID"] != "run-1" || begin["fromSeq"] != float64(2) {
		t.Fatalf("resume.begin = %+v", events[0].Payload)
	}
	if events[1].Type != assistant.EventTypeBlockCreated || events[1].Seq != 3 {
		t.Fatalf("重放事件 = %+v", events[1])
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content type = %q", ct)
	}
}

func TestStreamAssistantEventsResumeReset(t *testing.T) {
	c, rec := newAssistantTestContext(t)
	stream := &turns.Stream{
		SessionID: 1,
		Events:    make(chan turns.StreamItem),
		Resume:    &turns.ResumeInfo{RunID: "run-1", FromSeq: 9, Reset: "gap"},
	}
	streamAssistantEvents(c, stream)

	events := parseAssistantLines(t, rec.Body.String())
	if len(events) != 1 || events[0].Type != assistant.EventTypeResumeReset {
		t.Fatalf("事件 = %+v", events)
	}
	payload, ok := events[0].Payload.(map[string]any)
	if !ok || payload["reason"] != "gap" {
		t.Fatalf("resume.reset = %+v", events[0].Payload)
	}
}

func TestStreamAssistantEventsSnapshotOmitsID(t *testing.T) {
	c, rec := newAssistantTestContext(t)
	begin := assistant.Event{
		V:         assistant.Version,
		Seq:       1,
		Type:      assistant.EventTypeSnapshotBegin,
		SessionID: 1,
		Payload:   assistant.SnapshotBeginPayload{Mode: "history"},
	}
	message := assistant.Event{
		V:         assistant.Version,
		Seq:       2,
		Type:      assistant.EventTypeSnapshotMessage,
		SessionID: 1,
		Payload:   assistant.SnapshotMessagePayload{Message: assistant.MessageSnapshot{ID: 7, Role: "user"}},
	}
	live := assistant.Event{
		V:         assistant.Version,
		Seq:       1,
		Type:      assistant.EventTypeBlockCreated,
		SessionID: 1,
		RunID:     "run-live",
		Payload:   assistant.BlockCreatedPayload{BlockSeq: 1, Kind: assistant.BlockKindContent},
	}
	streamAssistantEvents(c, &turns.Stream{
		SessionID: 1,
		Replay: []turns.StreamItem{
			{Event: &begin},
			{Event: &message},
			{Event: &live},
		},
	})

	idLines := 0
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if strings.HasPrefix(line, "id:") {
			idLines++
		}
	}
	if idLines != 1 {
		t.Fatalf("快照事件不应携带 id，实时事件应携带一条；实际 %d，body = %q", idLines, rec.Body.String())
	}
}

func TestStreamAssistantEventsTurnCompleted(t *testing.T) {
	c, rec := newAssistantTestContext(t)
	done := assistant.Event{
		V:         assistant.Version,
		Seq:       1,
		Type:      assistant.EventTypeTurnCompleted,
		SessionID: 1,
		Payload:   assistant.TurnCompletedPayload{Phase: "done"},
	}
	streamAssistantEvents(c, &turns.Stream{
		SessionID: 1,
		Replay:    []turns.StreamItem{{Event: &done}},
	})
	if !strings.Contains(rec.Body.String(), `"turn.completed"`) {
		t.Fatalf("流缺少 turn.completed: %q", rec.Body.String())
	}
}
