package ai

import (
	// 标准库
	"context"
	"strings"
	"testing"

	// 第三方
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// newAuditTestAgent 构造仅用于审核重要级评估用例的 Agent：内存会话库 + 给定档位解析器 +
// 测试参数读取器，不装配任何工具与记忆。
func newAuditTestAgent(t *testing.T, resolver ChatResolver) *Agent {
	t.Helper()
	store, err := NewSessionStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	agent, err := NewAgent(context.Background(), resolver, &stubRetriever{}, store, testParams(t, nil), nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAgent error = %v", err)
	}
	return agent
}

// assertAuditLevels 断言实际档位与期望逐条一致且长度相等。
func assertAuditLevels(t *testing.T, got, want []AuditLevel) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("audit levels length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("audit levels[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestEvaluateAuditLevels_ClassifiesByCallID 测试正常批量分类：一次调用的输出按 callID 对齐回填、
// 与 calls 顺序一致（乱序输出也能正确对齐），并容忍 ```json 围栏。
func TestEvaluateAuditLevels_ClassifiesByCallID(t *testing.T) {
	chat := &stubChat{reply: "```json\n{\"levels\": [" +
		"{\"callID\": \"c1\", \"level\": \"none\"}, " +
		"{\"callID\": \"c2\", \"level\": \"must\"}, " +
		"{\"callID\": \"c3\", \"level\": \"auto\"}]}\n```"}
	agent := newAuditTestAgent(t, stubResolver(chat))
	calls := []pendingToolCall{
		{CallID: "c3", ToolName: "client_tool", Arguments: `{}`},
		{CallID: "c1", ToolName: "client_tool", Arguments: `{}`},
		{CallID: "c2", ToolName: "client_tool", Arguments: `{}`},
	}
	got := agent.evaluateAuditLevels(context.Background(), ai.SlotStandard, calls)
	assertAuditLevels(t, got, []AuditLevel{AuditLevelAuto, AuditLevelNone, AuditLevelMust})

	// 评估请求应携带配置的评估提示词，且用户消息逐条列出待评估调用的 callID 与工具名。
	if len(chat.gotInputs) != 1 {
		t.Fatalf("chat calls = %d, want 1", len(chat.gotInputs))
	}
	input := chat.gotInputs[0]
	if input[0].Role != schema.System || input[0].Content != "test audit level system prompt" {
		t.Errorf("system message = %+v, want the configured audit level prompt", input[0])
	}
	for _, callID := range []string{"c1", "c2", "c3"} {
		if !strings.Contains(input[1].Content, callID) {
			t.Errorf("user message missing callID %q: %q", callID, input[1].Content)
		}
	}
}

// TestEvaluateAuditLevels_GenerateErrorFallsBackToMust 测试模型调用报错时全部保守回落 must。
func TestEvaluateAuditLevels_GenerateErrorFallsBackToMust(t *testing.T) {
	chat := &fakeChat{gen: func(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
		return nil, errors.New("model unavailable")
	}}
	agent := newAuditTestAgent(t, stubResolver(chat))
	calls := []pendingToolCall{{CallID: "c1"}, {CallID: "c2"}}
	got := agent.evaluateAuditLevels(context.Background(), ai.SlotStandard, calls)
	assertAuditLevels(t, got, []AuditLevel{AuditLevelMust, AuditLevelMust})
}

// TestEvaluateAuditLevels_InvalidLevelFallsBackToMust 测试非法档位仅使该条回落 must，其余照常回填。
func TestEvaluateAuditLevels_InvalidLevelFallsBackToMust(t *testing.T) {
	chat := &stubChat{reply: `{"levels": [` +
		`{"callID": "c1", "level": "none"}, ` +
		`{"callID": "c2", "level": "bogus"}, ` +
		`{"callID": "c3", "level": "suggest"}]}`}
	agent := newAuditTestAgent(t, stubResolver(chat))
	calls := []pendingToolCall{{CallID: "c1"}, {CallID: "c2"}, {CallID: "c3"}}
	got := agent.evaluateAuditLevels(context.Background(), ai.SlotStandard, calls)
	assertAuditLevels(t, got, []AuditLevel{AuditLevelNone, AuditLevelMust, AuditLevelSuggest})
}

// TestEvaluateAuditLevels_CountMismatchFallsBackMissingToMust 测试条数不匹配：仅对已在输出中出现的
// 调用回填其档位，缺失项一律回落 must。
func TestEvaluateAuditLevels_CountMismatchFallsBackMissingToMust(t *testing.T) {
	chat := &stubChat{reply: `{"levels": [{"callID": "c1", "level": "none"}]}`}
	agent := newAuditTestAgent(t, stubResolver(chat))
	calls := []pendingToolCall{{CallID: "c1"}, {CallID: "c2"}, {CallID: "c3"}}
	got := agent.evaluateAuditLevels(context.Background(), ai.SlotStandard, calls)
	assertAuditLevels(t, got, []AuditLevel{AuditLevelNone, AuditLevelMust, AuditLevelMust})
}

// TestEvaluateAuditLevels_FallsBackToRunSlot 测试档位回退：flash 档位解析失败时回退本次运行档位，
// 用该档位完成评估。
func TestEvaluateAuditLevels_FallsBackToRunSlot(t *testing.T) {
	chat := &stubChat{reply: `{"levels": [{"callID": "c1", "level": "auto"}]}`}
	agent := newAuditTestAgent(t, slotChatResolver(map[ai.Slot]model.BaseChatModel{ai.SlotStandard: chat}))
	calls := []pendingToolCall{{CallID: "c1"}}
	got := agent.evaluateAuditLevels(context.Background(), ai.SlotStandard, calls)
	assertAuditLevels(t, got, []AuditLevel{AuditLevelAuto})
	if len(chat.gotInputs) != 1 {
		t.Fatalf("fallback chat calls = %d, want 1", len(chat.gotInputs))
	}
}

// TestNeedsHumanReview_TruthTable 用表驱动逐项验证 spec 判定矩阵：4 档审核重要级
// （must / suggest / auto / none）与 3 档审核阀门（each / auto / none）的全部 12 种组合，
// 断言 NeedsHumanReview 的返回值与矩阵中「人工=true / 自动=false」的结论一致。
func TestNeedsHumanReview_TruthTable(t *testing.T) {
	tests := []struct {
		name  string
		level AuditLevel
		valve ai.ReviewValve
		want  bool
	}{
		{"必须审核且逐一审核时需人工", AuditLevelMust, ai.ReviewValveEach, true},
		{"必须审核且自动审核时需人工", AuditLevelMust, ai.ReviewValveAuto, true},
		{"必须审核且无需审核时需人工", AuditLevelMust, ai.ReviewValveNone, true},
		{"建议审核且逐一审核时需人工", AuditLevelSuggest, ai.ReviewValveEach, true},
		{"建议审核且自动审核时需人工", AuditLevelSuggest, ai.ReviewValveAuto, true},
		{"建议审核且无需审核时可自动", AuditLevelSuggest, ai.ReviewValveNone, false},
		{"可自动审核且逐一审核时需人工", AuditLevelAuto, ai.ReviewValveEach, true},
		{"可自动审核且自动审核时可自动", AuditLevelAuto, ai.ReviewValveAuto, false},
		{"可自动审核且无需审核时可自动", AuditLevelAuto, ai.ReviewValveNone, false},
		{"无需审核且逐一审核时需人工", AuditLevelNone, ai.ReviewValveEach, true},
		{"无需审核且自动审核时可自动", AuditLevelNone, ai.ReviewValveAuto, false},
		{"无需审核且无需审核时可自动", AuditLevelNone, ai.ReviewValveNone, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NeedsHumanReview(tt.level, tt.valve); got != tt.want {
				t.Errorf("NeedsHumanReview(%q, %q) = %v, want %v", tt.level, tt.valve, got, tt.want)
			}
		})
	}
}

// TestBuildAuditLevelPrompt_IncludesToolDescription 测试评估上下文的工具用途说明注入：已登记的
// client 工具在其调用条目中携带 description（取 clientToolInfo 的 Desc），未登记的工具则省略该行，
// 避免评估模型仅凭工具名字面猜测副作用而误判审核重要级。
func TestBuildAuditLevelPrompt_IncludesToolDescription(t *testing.T) {
	const describedTool = "audit_probe_described_tool"
	registerClientToolInfo(describedTool, &schema.ToolInfo{
		Name: describedTool,
		Desc: "删除指定数据源的全部采集记录（破坏性写操作）",
	})
	calls := []pendingToolCall{
		{CallID: "c1", ToolName: describedTool, Arguments: `{"id": 1}`},
		{CallID: "c2", ToolName: "audit_probe_undescribed_tool", Arguments: `{}`},
	}
	agent := newAuditTestAgent(t, stubResolver(&stubChat{}))
	prompt := agent.buildAuditLevelPrompt(calls)
	if !strings.Contains(prompt, "description: 删除指定数据源的全部采集记录（破坏性写操作）") {
		t.Errorf("prompt missing the registered tool description: %q", prompt)
	}
	if got := strings.Count(prompt, "description:"); got != 1 {
		t.Errorf("prompt has %d description lines, want 1 (unregistered tools must omit it): %q", got, prompt)
	}
}
