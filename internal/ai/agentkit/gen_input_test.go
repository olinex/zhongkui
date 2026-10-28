package agentkit

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/ai/contract"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
)

func TestBuildModelInputChatSections(t *testing.T) {
	params := mustParams(t, nil)
	history := []*schema.Message{
		{Role: schema.User, Content: "earlier question"},
		{Role: schema.Assistant, Content: "earlier answer"},
	}
	rc := RunContext[*schema.Message]{
		History: history,
		Request: Request{
			Query:        "how do I list traffic?",
			SessionID:    7,
			AgentKind:    modelsai.AgentKindGeneral,
			BusinessData: contract.BusinessData{"namespace": "default", "empty": ""},
			Slot:         modelsai.SlotStandard,
			ReviewValve:  modelsai.ReviewValveEach,
		},
		Summary: "the session is about traffic",
		Memories: []contract.MemoryHit{
			{Kind: modelsai.MemoryKindFact, Title: "preference", Content: "answers in Chinese"},
			{Kind: modelsai.MemoryKindFact, Content: "   "},
		},
	}

	messages, err := buildModelInput[*schema.Message](t.Context(), params, rc)
	if err != nil {
		t.Fatalf("buildModelInput() error = %v", err)
	}
	if len(messages) != 6 {
		t.Fatalf("messages len = %d, want 6 (%v)", len(messages), messageSummary(messages))
	}
	if !strings.Contains(messages[0].Content, "general system prompt") {
		t.Errorf("system prompt = %q, want base prompt", messages[0].Content)
	}
	if !strings.Contains(messages[0].Content, agentCapabilityHint) ||
		!strings.Contains(messages[0].Content, agentCitationHint) ||
		!strings.Contains(messages[0].Content, agentNextStepsHint) {
		t.Errorf("system prompt misses fixed sections: %q", messages[0].Content)
	}
	if !strings.Contains(messages[1].Content, agentSummarySectionTitle) ||
		!strings.Contains(messages[1].Content, "traffic") {
		t.Errorf("summary message = %q", messages[1].Content)
	}
	if !strings.Contains(messages[2].Content, agentMemorySectionTitle) ||
		!strings.Contains(messages[2].Content, "answers in Chinese") {
		t.Errorf("memory message = %q", messages[2].Content)
	}
	if messages[3].Content != "earlier question" || messages[4].Content != "earlier answer" {
		t.Errorf("history = %v", messageSummary(messages[3:5]))
	}
	user := messages[len(messages)-1]
	if user.Role != schema.User {
		t.Errorf("last message role = %q, want user", user.Role)
	}
	if !strings.Contains(user.Content, agentBusinessSectionTitle) ||
		!strings.Contains(user.Content, `"namespace":"default"`) ||
		strings.Contains(user.Content, "empty") {
		t.Errorf("user message business data = %q", user.Content)
	}
	if !strings.Contains(user.Content, agentQuestionSectionTitle) ||
		!strings.Contains(user.Content, "how do I list traffic?") {
		t.Errorf("user message query = %q", user.Content)
	}
}

func TestBuildModelInputAgenticSections(t *testing.T) {
	params := mustParams(t, nil)
	history := []*schema.AgenticMessage{
		schema.UserAgenticMessage("earlier question"),
		func() *schema.AgenticMessage {
			msg := &schema.AgenticMessage{Role: schema.AgenticRoleTypeAssistant}
			msg.ContentBlocks = append(msg.ContentBlocks, schema.NewContentBlock(&schema.AssistantGenText{Text: "earlier answer"}))
			return msg
		}(),
	}
	rc := RunContext[*schema.AgenticMessage]{
		History: history,
		Request: Request{
			Query:        "inspect",
			SessionID:    1,
			AgentKind:    modelsai.AgentKindKnowledge,
			BusinessData: contract.BusinessData{"id": 42},
		},
		Summary: "summary text",
		Memories: []contract.MemoryHit{
			{Kind: modelsai.MemoryKindDecision, Content: "use standard"},
		},
	}

	messages, err := buildModelInput[*schema.AgenticMessage](t.Context(), params, rc)
	if err != nil {
		t.Fatalf("buildModelInput() error = %v", err)
	}
	if len(messages) != 6 {
		t.Fatalf("messages len = %d, want 6", len(messages))
	}
	if got := msgopsText(messages[0]); !strings.Contains(got, "base system prompt") ||
		!strings.Contains(got, agentCapabilityHint) {
		t.Errorf("system message = %q", got)
	}
	if got := msgopsText(messages[1]); !strings.Contains(got, agentSummarySectionTitle) {
		t.Errorf("summary message = %q", got)
	}
	if got := msgopsText(messages[2]); !strings.Contains(got, agentMemorySectionTitle) {
		t.Errorf("memory message = %q", got)
	}
	user := msgopsText(messages[len(messages)-1])
	if !strings.Contains(user, `"id":42`) || !strings.Contains(user, "inspect") {
		t.Errorf("user message = %q", user)
	}
}

func TestBuildModelInputResumePassesThrough(t *testing.T) {
	params := mustParams(t, nil)
	rt := &Runtime{params: params}
	gen := newGenModelInput[*schema.Message](rt)
	restored := []*schema.Message{schema.UserMessage("business data stays untouched")}
	input := &adk.TypedAgentInput[*schema.Message]{Messages: restored}
	messages, err := gen(t.Context(), "ignored", input)
	if err != nil {
		t.Fatalf("genModelInput() error = %v", err)
	}
	if len(messages) != 1 || messages[0].Content != "business data stays untouched" {
		t.Fatalf("resume messages = %v, want the restored messages unchanged", messageSummary(messages))
	}
}

func TestGenModelInputSkipsInlineNextStepsForNonGeneral(t *testing.T) {
	params := mustParams(t, nil)
	rc := RunContext[*schema.Message]{
		Request: Request{
			Query:     "q",
			SessionID: 1,
			AgentKind: modelsai.AgentKindKnowledge,
		},
	}
	messages, err := buildModelInput[*schema.Message](t.Context(), params, rc)
	if err != nil {
		t.Fatalf("buildModelInput() error = %v", err)
	}
	if strings.Contains(messages[0].Content, agentNextStepsHint) {
		t.Fatalf("system prompt for knowledge agent contains the next steps hint")
	}
}

// TestStripCitationMarkers 验证历史正文中的旧来源标注被移除，而代码段内的数组下标保持不变。
func TestStripCitationMarkers(t *testing.T) {
	text := "见 [13] 与 [2]，代码 `arr[1]` 不动。\n```go\nfmt.Println(x[2])\n```\n结尾[3]"
	want := "见  与 ，代码 `arr[1]` 不动。\n```go\nfmt.Println(x[2])\n```\n结尾"
	if got := stripCitationMarkers(text); got != want {
		t.Fatalf("stripCitationMarkers() = %q, want %q", got, want)
	}
}

// TestStripHistoryCitationMarkers 验证只清洗助手历史消息，用户与会话中的引用标注保持原样。
func TestStripHistoryCitationMarkers(t *testing.T) {
	history := []*schema.Message{
		schema.UserMessage("用户提到 [9]"),
		schema.AssistantMessage("旧回答引用 [13]", nil),
	}
	got := stripHistoryCitationMarkers[*schema.Message](history)
	if got[0].Content != "用户提到 [9]" {
		t.Fatalf("user message = %q, want unchanged", got[0].Content)
	}
	if got[1].Content != "旧回答引用 " {
		t.Fatalf("assistant message = %q, want markers stripped", got[1].Content)
	}
}

func TestSanitizeAppliedOnFreshRunOnly(t *testing.T) {
	params := mustParams(t, map[string]string{"sanitize.maxBytes": "40"})
	rc := RunContext[*schema.Message]{
		Request: Request{
			Query:        "q",
			BusinessData: contract.BusinessData{"huge": strings.Repeat("x", 200), "ok": "v"},
		},
	}
	messages, err := buildModelInput[*schema.Message](t.Context(), params, rc)
	if err != nil {
		t.Fatalf("buildModelInput() error = %v", err)
	}
	user := messages[len(messages)-1].Content
	if strings.Contains(user, strings.Repeat("x", 200)) {
		t.Fatalf("business data was not sanitized: %q", user)
	}
	if !strings.Contains(user, `"ok":"v"`) {
		t.Fatalf("sanitized business data dropped the small entry: %q", user)
	}
	payload, err := json.Marshal(contract.BusinessData{"huge": strings.Repeat("x", 200), "ok": "v"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(payload) > 0 && strings.Contains(user, string(payload)) {
		t.Fatalf("raw business data leaked into the prompt")
	}
}

func messageSummary[M schema.Message | schema.AgenticMessage](messages []*M) []string {
	out := make([]string, 0, len(messages))
	for _, message := range messages {
		out = append(out, msgopsTextAny(message))
	}
	return out
}

func msgopsText[M schema.Message | schema.AgenticMessage](message *M) string {
	return msgopsTextAny(message)
}

func msgopsTextAny(message any) string {
	switch m := message.(type) {
	case *schema.Message:
		if m == nil {
			return ""
		}
		return m.Content
	case *schema.AgenticMessage:
		if m == nil {
			return ""
		}
		var parts []string
		for _, block := range m.ContentBlocks {
			if block != nil && block.AssistantGenText != nil {
				parts = append(parts, block.AssistantGenText.Text)
			}
			if block != nil && block.UserInputText != nil {
				parts = append(parts, block.UserInputText.Text)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}
