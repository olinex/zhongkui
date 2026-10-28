package agentkit

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/ai/contract"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
)

func TestInlineNextStepsFilterStripsSection(t *testing.T) {
	filter := NewInlineNextStepsFilter()
	chunks := []string{
		"the answer ",
		"<!--next_",
		`steps
{"steps":[{"title":"Follow up","reason":"Because","text":"ask about DNS"}]}
-->`,
		"tail is dropped",
	}
	var body strings.Builder
	for _, chunk := range chunks {
		body.WriteString(filter.Write(chunk))
	}
	body.WriteString(filter.Close())
	if got := body.String(); got != "the answer " {
		t.Fatalf("body = %q, want only the prose before the marker", got)
	}
	if filter.Body() != "the answer " {
		t.Fatalf("Body() = %q", filter.Body())
	}
	steps, seen, err := filter.Steps()
	if err != nil || !seen {
		t.Fatalf("Steps() = seen:%v err:%v", seen, err)
	}
	if len(steps) != 1 || steps[0].Title != "Follow up" || steps[0].Text != "ask about DNS" {
		t.Fatalf("steps = %+v", steps)
	}
	if !steps[0].Recommended {
		t.Fatalf("first step must be marked recommended")
	}
}

func TestInlineNextStepsFilterWithoutMarker(t *testing.T) {
	filter := NewInlineNextStepsFilter()
	var body strings.Builder
	body.WriteString(filter.Write("plain "))
	body.WriteString(filter.Write("text"))
	body.WriteString(filter.Close())
	if body.String() != "plain text" {
		t.Fatalf("body = %q", body.String())
	}
	if _, seen, err := filter.Steps(); seen || err != nil {
		t.Fatalf("Steps() = seen:%v err:%v, want no marker", seen, err)
	}
}

func TestInlineNextStepsFilterSplitsMultibyteSafely(t *testing.T) {
	filter := NewInlineNextStepsFilter()
	preamble := "中文正文"
	var body strings.Builder
	for _, r := range preamble {
		body.WriteString(filter.Write(string(r)))
	}
	body.WriteString(filter.Close())
	if body.String() != preamble {
		t.Fatalf("body = %q, want %q", body.String(), preamble)
	}
}

// TestNormalizeNextSteps 测试归一化：去空白 / 去重 / 最多保留 3 条 / 首条标记为最推荐，且不再做
// 长度截断（取值原样保留，长度合法性交由 contract.NextStep.Validate 判定）。
func TestNormalizeNextSteps(t *testing.T) {
	items := []contract.NextStep{
		{Title: "  A  ", Reason: " r1 ", Text: " t1 "},
		{Title: "A2", Reason: "r2", Text: "t2"},
		{Title: "dup", Reason: "r4", Text: "t1"},
		{Title: "B", Reason: "r5", Text: "t5"},
		{Title: "C", Reason: "r6", Text: "t6"},
		{Title: "D", Reason: "r7", Text: "t7"},
	}
	steps, err := NormalizeNextSteps(items)
	if err != nil {
		t.Fatalf("NormalizeNextSteps() error = %v, want nil", err)
	}
	if len(steps) != 3 {
		t.Fatalf("len(steps) = %d, want 3", len(steps))
	}
	if steps[0].Title != "A" || steps[0].Reason != "r1" || steps[0].Text != "t1" || !steps[0].Recommended {
		t.Fatalf("first step = %+v", steps[0])
	}
	if steps[1].Text != "t2" || steps[2].Text != "t5" {
		t.Fatalf("steps = %+v, want deduplicated order t2, t5", steps)
	}
}

// TestNormalizeNextStepsReturnsViolation 测试超长 / 缺字段的条目不再被本地截断或静默丢弃，而是返回
// 错误供调用方回传模型重新生成。
func TestNormalizeNextStepsReturnsViolation(t *testing.T) {
	overlong := func(n int) string { return strings.Repeat("字", n) }
	cases := []struct {
		name  string
		items []contract.NextStep
	}{
		{"title too long", []contract.NextStep{{Title: overlong(contract.NextStepTitleMaxLength + 1), Reason: "r", Text: "t"}}},
		{"text too long", []contract.NextStep{{Text: overlong(contract.NextStepTextMaxLength + 1), Reason: "r"}}},
		{"reason too long", []contract.NextStep{{Text: "t", Reason: overlong(contract.NextStepReasonMaxLength + 1)}}},
		{"missing reason", []contract.NextStep{{Title: "A", Text: "t"}}},
		{"missing text", []contract.NextStep{{Reason: "r"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if steps, err := NormalizeNextSteps(c.items); err == nil {
				t.Errorf("NormalizeNextSteps() = (%+v, nil), want an error", steps)
			}
		})
	}
}

func TestRecommendNextStepsFallback(t *testing.T) {
	chat := &fakeChat{script: []*schema.Message{{
		Role:    schema.Assistant,
		Content: `{"steps":[{"title":"Next","reason":"Relevant","text":"show me more"}]}`,
	}}}
	rt := &Runtime{
		params: testParams(nil),
		deps: Deps{
			ChatResolver: func(modelsai.Slot) (model.BaseChatModel, error) { return chat, nil },
		},
	}
	steps := RecommendNextSteps[*schema.Message](context.Background(), rt, Request{
		SessionID: 1,
		Query:     "what next?",
	}, "summary", []*schema.Message{schema.UserMessage("hi")}, "answer")
	if len(steps) != 1 || steps[0].Text != "show me more" {
		t.Fatalf("steps = %+v", steps)
	}
}

// TestRecommendNextStepsRegeneratesOnViolation 测试首次输出不满足长度约束时，把校验错误回传给模型
// 重新生成（而非本地截断），并采用第二次输出。
func TestRecommendNextStepsRegeneratesOnViolation(t *testing.T) {
	overlongTitle := strings.Repeat("x", contract.NextStepTitleMaxLength+1)
	chat := &fakeChat{script: []*schema.Message{
		{Role: schema.Assistant, Content: `{"steps":[{"title":"` + overlongTitle + `","reason":"r","text":"t"}]}`},
		{Role: schema.Assistant, Content: `{"steps":[{"title":"Next","reason":"Relevant","text":"show me more"}]}`},
	}}
	rt := &Runtime{
		params: testParams(nil),
		deps: Deps{
			ChatResolver: func(modelsai.Slot) (model.BaseChatModel, error) { return chat, nil },
		},
	}
	steps := RecommendNextSteps[*schema.Message](context.Background(), rt, Request{
		SessionID: 1,
		Query:     "what next?",
	}, "summary", []*schema.Message{schema.UserMessage("hi")}, "answer")
	if len(steps) != 1 || steps[0].Text != "show me more" {
		t.Fatalf("steps = %+v, want the regenerated output", steps)
	}
	if chat.idx != 2 {
		t.Fatalf("Generate call count = %d, want 2", chat.idx)
	}
}
