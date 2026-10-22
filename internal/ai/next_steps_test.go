package ai

import (
	// 标准库
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	// 第三方
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
)

// TestParseNextSteps 覆盖推荐模型输出的容错解析：新对象形状（title/reason/text）、旧字符串形状、
// 代码围栏、裸数组与空数组；非法 JSON 返回错误。
func TestParseNextSteps(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []NextStep
	}{
		{
			name: "object items",
			in:   `{"steps": [{"title": "标题A", "reason": "理由A", "text": "文本A"}, {"title": "标题B", "reason": "理由B", "text": "文本B"}]}`,
			want: []NextStep{
				{Title: "标题A", Text: "文本A", Reason: "理由A"},
				{Title: "标题B", Text: "文本B", Reason: "理由B"},
			},
		},
		{
			name: "fenced",
			in:   "```json\n{\"steps\": [{\"title\": \"标题A\", \"reason\": \"理由A\", \"text\": \"文本A\"}]}\n```",
			want: []NextStep{{Title: "标题A", Text: "文本A", Reason: "理由A"}},
		},
		{
			name: "legacy strings",
			in:   `{"steps": ["A", "B"]}`,
			want: []NextStep{{Text: "A"}, {Text: "B"}},
		},
		{
			name: "legacy bare array",
			in:   `["A", "B"]`,
			want: []NextStep{{Text: "A"}, {Text: "B"}},
		},
		{name: "empty", in: `{"steps": []}`, want: nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseNextSteps(c.in)
			if err != nil {
				t.Fatalf("parseNextSteps(%q) error = %v", c.in, err)
			}
			if len(got) != len(c.want) {
				t.Fatalf("parseNextSteps(%q) len = %d, want %d (%+v)", c.in, len(got), len(c.want), got)
			}
			for i := range c.want {
				if got[i] != c.want[i] {
					t.Errorf("item %d = %+v, want %+v", i, got[i], c.want[i])
				}
			}
		})
	}
	if _, err := parseNextSteps("not-json"); err == nil {
		t.Error("parseNextSteps(invalid) = nil error, want an error")
	}
	if _, err := parseNextSteps("   "); err == nil {
		t.Error("parseNextSteps(blank) = nil error, want an error")
	}
}

// TestNormalizeNextSteps 覆盖归一化：标题 / 理由 / 文本去空白、折叠换行、按上限硬截断（标题 24、
// 理由 64）、按文本去重与最多 3 条；首条标记为最推荐。
func TestNormalizeNextSteps(t *testing.T) {
	got := normalizeNextSteps([]NextStep{
		{
			Title:  "  \"查看接口流量\"  ",
			Text:   " 查看该接口最近一小时的流量详情 ",
			Reason: " 你刚查看了该接口，建议确认异常是否持续 ",
		},
		{Title: "重复", Text: "查看该接口最近一小时的流量详情", Reason: "重复项"},
		{Title: strings.Repeat("标", 30), Text: "为该接口编写标签规则", Reason: strings.Repeat("理", 70)},
		{Text: ""},
		{Title: "A", Text: "A", Reason: "理由A"},
		{Title: "B", Text: "B", Reason: "理由B"},
	})
	if len(got) != 3 {
		t.Fatalf("normalize len = %d, want 3 (%+v)", len(got), got)
	}
	first := got[0]
	if first.Title != "查看接口流量" || first.Text != "查看该接口最近一小时的流量详情" ||
		first.Reason != "你刚查看了该接口，建议确认异常是否持续" || !first.Recommended {
		t.Errorf("first item = %+v, want trimmed fields marked recommended", first)
	}
	if got[1].Title == "" || len([]rune(got[1].Title)) > nextStepsTitleLimit {
		t.Errorf("second title = %q, want a non-empty title within %d runes", got[1].Title, nextStepsTitleLimit)
	}
	if len([]rune(got[1].Reason)) > nextStepsReasonLimit {
		t.Errorf("second reason length = %d, want <= %d", len([]rune(got[1].Reason)), nextStepsReasonLimit)
	}
	if !strings.HasSuffix(got[1].Reason, "…") {
		t.Errorf("second reason = %q, want a truncated value with an ellipsis", got[1].Reason)
	}
	if got[2].Text != "A" || got[2].Title != "A" {
		t.Errorf("third item = %+v, want A (dedupe and cap preserve order)", got[2])
	}
}

// TestBuildNextStepsPrompt 覆盖推荐上下文拼装：摘要、最近对话、本轮提问与本轮回答各成一段，
// 系统提示与工具消息被跳过；续跑时历史上的已外发正文不重复拼入。
func TestBuildNextStepsPrompt(t *testing.T) {
	agent := &Agent{params: testParams(t, nil)}
	in := AgentInput{
		Summary: "折叠后的会话摘要",
		Query:   "查看该接口最近的流量",
		HistoryMessages: []*schema.Message{
			schema.SystemMessage("完整系统提示，不应进入推荐上下文"),
			schema.UserMessage("上一轮提问"),
			schema.AssistantMessage("上一轮回答", nil),
			schema.ToolMessage("工具结果", "call-1"),
		},
	}
	got := agent.buildNextStepsPrompt(in, "这是本轮回答")
	for _, want := range []string{
		"## 会话摘要\n折叠后的会话摘要",
		"## 最近对话\n用户：上一轮提问\n助手：上一轮回答",
		"## 本轮提问\n查看该接口最近的流量",
		"## 本轮回答\n这是本轮回答",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "完整系统提示") || strings.Contains(got, "工具结果") {
		t.Errorf("prompt unexpectedly carries the system prompt or tool message:\n%s", got)
	}

	// 续跑：取到本轮最后一条 user 消息为止，其后的助手正文由本轮回答承载，避免重复。
	resumed := AgentInput{
		ResumeMessages: []*schema.Message{
			schema.UserMessage("本轮提问"),
			schema.AssistantMessage("挂起前的前导说明", nil),
			schema.ToolMessage("工具结果", "call-2"),
		},
	}
	got = agent.buildNextStepsPrompt(resumed, "续跑后的最终回答")
	if strings.Contains(got, "挂起前的前导说明") || strings.Contains(got, "工具结果") {
		t.Errorf("resume prompt unexpectedly carries the suspended preamble or tool message:\n%s", got)
	}
	if !strings.Contains(got, "用户：本轮提问") || !strings.Contains(got, "## 本轮回答\n续跑后的最终回答") {
		t.Errorf("resume prompt missing the current user turn or answer:\n%s", got)
	}
}

// nextStepsFixture 是推荐下一步接入用例的装配：会话存储、被测 Agent 与两个档位替身。
type nextStepsFixture struct {
	agent  *Agent
	store  *SessionStore
	steps  *stubChat
	answer *stubChat
}

// newNextStepsFixture 构造上述装配；stepsReply 为 flash 档位（推荐调用）的回复。
func newNextStepsFixture(t *testing.T, stepsReply string) *nextStepsFixture {
	t.Helper()
	ctx := context.Background()
	db := newTestSessionDB(t)
	store, err := NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	params := testParams(t, map[string]string{
		"agent.nextStepsEnabled":      "true",
		"agent.nextStepsSystemPrompt": "test next steps system prompt",
	})
	stepsChat := &stubChat{reply: stepsReply}
	answerChat := &stubChat{reply: `{"content": "回答", "draft": {}}`}
	agent, err := NewAgent(ctx,
		slotChatResolver(map[ai.Slot]model.BaseChatModel{
			ai.SlotFlash:    stepsChat,
			ai.SlotStandard: answerChat,
		}),
		&stubRetriever{}, store, params, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAgent error = %v", err)
	}
	return &nextStepsFixture{agent: agent, store: store, steps: stepsChat, answer: answerChat}
}

// runNextStepsTurn 在真实会话中执行一轮 StreamRun 并返回 done 项与本轮落库块。
func (f *nextStepsFixture) runNextStepsTurn(t *testing.T, session *ai.AgentSession) (AgentOutput, []*ai.AgentMessageBlock) {
	t.Helper()
	ctx := context.Background()
	reader, err := f.agent.StreamRun(ctx, AgentInput{
		Query:         "查看该接口的流量",
		NamespaceName: "default",
		SessionID:     session.ID,
		UserID:        session.UserID,
		Slot:          ai.SlotStandard,
	})
	if err != nil {
		t.Fatalf("StreamRun error = %v", err)
	}
	defer reader.Close()
	var done AgentOutput
	for {
		item, err := reader.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("stream error = %v", err)
		}
		if item.Done {
			done = item
		}
	}
	if done.MessageID == 0 {
		t.Fatal("done.MessageID = 0, want the persisted assistant message id")
	}
	blocks, err := f.store.ListBlocksByMessage(ctx, done.MessageID, "default", session.UserID)
	if err != nil {
		t.Fatalf("ListBlocksByMessage error = %v", err)
	}
	return done, blocks
}

// TestStreamRun_AppendsNextStepsBlock 覆盖真实会话的推荐块接入：回答完成后在正文之后落一个
// next_steps 块，载荷为 1-3 条推荐项且首条为最推荐。
func TestStreamRun_AppendsNextStepsBlock(t *testing.T) {
	f := newNextStepsFixture(t, `{"steps": [{"title": "查看流量详情", "reason": "你刚提到了该接口，建议确认异常", "text": "查看该接口最近一小时的流量详情"}, {"title": "编写标签规则", "reason": "该流量模式可沉淀为规则", "text": "为该接口编写标签规则"}]}`)
	session, err := f.store.CreateSession(context.Background(), "default", "t", ai.SlotStandard, ai.AgentKindKnowledge, testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	done, blocks := f.runNextStepsTurn(t, session)
	if done.Content != "回答" {
		t.Errorf("done content = %q, want the answer", done.Content)
	}
	var next *ai.AgentMessageBlock
	var contentSeq uint64
	for _, block := range blocks {
		switch block.Kind {
		case ai.AgentMessageBlockKindContent:
			contentSeq = block.BlockSeq
		case ai.AgentMessageBlockKindNextSteps:
			next = block
		}
	}
	if next == nil {
		t.Fatalf("blocks = %+v, want a next_steps block", blocks)
	}
	if contentSeq == 0 || next.BlockSeq <= contentSeq {
		t.Errorf("next_steps seq = %d, content seq = %d, want next_steps after content", next.BlockSeq, contentSeq)
	}
	var payload NextStepsBlockPayload
	if err := decodeBlockPayload(next.Payload.Value(), &payload); err != nil {
		t.Fatalf("decode next_steps error = %v", err)
	}
	if len(payload.Steps) != 2 || !payload.Steps[0].Recommended || payload.Steps[1].Recommended {
		t.Fatalf("steps = %+v, want two items with the first recommended", payload.Steps)
	}
	if payload.Steps[0].Title == "" || payload.Steps[0].Reason == "" || payload.Steps[0].Text == "" {
		t.Fatalf("first step = %+v, want title, reason and text all populated", payload.Steps[0])
	}
}

// TestStreamRun_NextStepsFailureDegrades 覆盖推荐失败降级：推荐模型返回非法 JSON 时，回答仍正常
// done 并落库，且不产生 next_steps 块。
func TestStreamRun_NextStepsFailureDegrades(t *testing.T) {
	f := newNextStepsFixture(t, "not-json")
	session, err := f.store.CreateSession(context.Background(), "default", "t", ai.SlotStandard, ai.AgentKindKnowledge, testUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	done, blocks := f.runNextStepsTurn(t, session)
	if done.Content != "回答" {
		t.Errorf("done content = %q, want the answer to survive the recommendation failure", done.Content)
	}
	for _, block := range blocks {
		if block.Kind == ai.AgentMessageBlockKindNextSteps {
			t.Fatalf("blocks = %+v, want no next_steps block on a parse failure", blocks)
		}
	}
}
