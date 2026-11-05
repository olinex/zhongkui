package audit

import (
	// 标准库
	"context"
	"strings"
	"testing"

	// 第三方
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/aitest"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// stubChat 是测试用对话模型：记录每次输入的 messages 并返回预置回复或错误。
type stubChat struct {
	// reply 非流式回复正文
	reply string
	// err 非 nil 时 Generate 直接失败（模拟模型不可用）
	err error
	// gotInputs 每次 Generate 收到的消息列表，供断言提示词内容
	gotInputs [][]*schema.Message
}

// Generate 记录输入并返回预置回复。
func (c *stubChat) Generate(_ context.Context, messages []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	c.gotInputs = append(c.gotInputs, messages)
	if c.err != nil {
		return nil, c.err
	}
	return schema.AssistantMessage(c.reply, nil), nil
}

// Stream 评估器不使用流式调用，测试中直接返回错误以暴露误用。
func (c *stubChat) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("audit: stream is not used by the evaluator")
}

// slotResolver 构造仅绑定给定档位的解析器：其余档位返回英文错误（模拟未绑定）。
func slotResolver(bySlot map[modelsai.Slot]model.BaseChatModel) ChatResolver {
	return func(slot modelsai.Slot) (model.BaseChatModel, error) {
		if chat, ok := bySlot[slot]; ok && chat != nil {
			return chat, nil
		}
		return nil, errors.Newf("test resolver: slot %q is not bound", slot)
	}
}

// newEvaluator 构造带测试参数与工具 schema 索引的评估器。
func newEvaluator(t *testing.T, resolver ChatResolver, toolInfos map[string]*schema.ToolInfo) *Evaluator {
	t.Helper()
	return New(Deps{Chats: resolver, Params: aitest.Params(t, nil), ToolInfos: toolInfos})
}

// assertLevels 断言实际档位与期望逐条一致且长度相等。
func assertLevels(t *testing.T, got, want []contract.AuditLevel) {
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

// testCalls 构造按序排列的待评估调用（仅填评估所需的 callID 与工具名/入参）。
func testCalls(callIDs ...string) []contract.PendingCall {
	calls := make([]contract.PendingCall, 0, len(callIDs))
	for _, id := range callIDs {
		calls = append(calls, contract.PendingCall{CallID: id, ToolName: "client_tool", Arguments: `{}`})
	}
	return calls
}

// TestLevels_ClassifiesByCallID 测试正常批量分类：模型输出按 callID 对齐回填、与 calls 顺序一致
// （乱序输出也能正确对齐），并容忍 ```json 围栏；请求侧须带配置的评估提示词与逐条调用信息。
func TestLevels_ClassifiesByCallID(t *testing.T) {
	chat := &stubChat{reply: "```json\n{\"levels\": [" +
		"{\"callID\": \"c1\", \"level\": \"none\"}, " +
		"{\"callID\": \"c2\", \"level\": \"must\"}, " +
		"{\"callID\": \"c3\", \"level\": \"auto\"}]}\n```"}
	e := newEvaluator(t, slotResolver(map[modelsai.Slot]model.BaseChatModel{modelsai.SlotFlash: chat}), nil)

	got := e.Levels(context.Background(), modelsai.SlotStandard, testCalls("c3", "c1", "c2"))
	assertLevels(t, got, []contract.AuditLevel{contract.AuditLevelAuto, contract.AuditLevelNone, contract.AuditLevelMust})

	if len(chat.gotInputs) != 1 {
		t.Fatalf("chat calls = %d, want 1", len(chat.gotInputs))
	}
	input := chat.gotInputs[0]
	if len(input) != 2 {
		t.Fatalf("audit request carries %d messages, want system + user", len(input))
	}
	if input[0].Role != schema.System || input[0].Content != "test audit level system prompt" {
		t.Errorf("system message = %+v, want the configured audit level prompt", input[0])
	}
	for _, callID := range []string{"c1", "c2", "c3"} {
		if !strings.Contains(input[1].Content, callID) {
			t.Errorf("user message missing callID %q: %q", callID, input[1].Content)
		}
	}
}

// TestLevels_EmptyCallsReturnsEmpty 测试空调用列表直接返回空结果且不发起模型调用。
func TestLevels_EmptyCallsReturnsEmpty(t *testing.T) {
	chat := &stubChat{reply: `{"levels":[]}`}
	e := newEvaluator(t, slotResolver(map[modelsai.Slot]model.BaseChatModel{modelsai.SlotFlash: chat}), nil)
	got := e.Levels(context.Background(), modelsai.SlotStandard, nil)
	if len(got) != 0 {
		t.Errorf("audit levels = %v, want empty", got)
	}
	if len(chat.gotInputs) != 0 {
		t.Errorf("chat calls = %d, want 0 (no calls to evaluate)", len(chat.gotInputs))
	}
}

// TestLevels_GenerateErrorFallsBackToMust 测试模型调用报错时全部保守回落 must。
func TestLevels_GenerateErrorFallsBackToMust(t *testing.T) {
	chat := &stubChat{err: errors.New("model unavailable")}
	e := newEvaluator(t, slotResolver(map[modelsai.Slot]model.BaseChatModel{modelsai.SlotFlash: chat}), nil)
	got := e.Levels(context.Background(), modelsai.SlotStandard, testCalls("c1", "c2"))
	assertLevels(t, got, []contract.AuditLevel{contract.AuditLevelMust, contract.AuditLevelMust})
}

// TestLevels_EmptyResponseFallsBackToMust 测试空回复（模型返回空正文）时全部回落 must。
func TestLevels_EmptyResponseFallsBackToMust(t *testing.T) {
	chat := &stubChat{reply: "  "}
	e := newEvaluator(t, slotResolver(map[modelsai.Slot]model.BaseChatModel{modelsai.SlotFlash: chat}), nil)
	got := e.Levels(context.Background(), modelsai.SlotStandard, testCalls("c1"))
	assertLevels(t, got, []contract.AuditLevel{contract.AuditLevelMust})
}

// TestLevels_InvalidLevelFallsBackToMust 测试非法档位仅使该条回落 must，其余照常回填。
func TestLevels_InvalidLevelFallsBackToMust(t *testing.T) {
	chat := &stubChat{reply: `{"levels": [` +
		`{"callID": "c1", "level": "none"}, ` +
		`{"callID": "c2", "level": "bogus"}, ` +
		`{"callID": "c3", "level": "suggest"}]}`}
	e := newEvaluator(t, slotResolver(map[modelsai.Slot]model.BaseChatModel{modelsai.SlotFlash: chat}), nil)
	got := e.Levels(context.Background(), modelsai.SlotStandard, testCalls("c1", "c2", "c3"))
	assertLevels(t, got, []contract.AuditLevel{contract.AuditLevelNone, contract.AuditLevelMust, contract.AuditLevelSuggest})
}

// TestLevels_CountMismatchFallsBackMissingToMust 测试条数不匹配：仅对已在输出中出现的调用回填其
// 档位，缺失项一律回落 must。
func TestLevels_CountMismatchFallsBackMissingToMust(t *testing.T) {
	chat := &stubChat{reply: `{"levels": [{"callID": "c1", "level": "none"}]}`}
	e := newEvaluator(t, slotResolver(map[modelsai.Slot]model.BaseChatModel{modelsai.SlotFlash: chat}), nil)
	got := e.Levels(context.Background(), modelsai.SlotStandard, testCalls("c1", "c2", "c3"))
	assertLevels(t, got, []contract.AuditLevel{contract.AuditLevelNone, contract.AuditLevelMust, contract.AuditLevelMust})
}

// TestLevels_FallsBackToRunSlot 测试档位回退：flash 档位解析失败时回退本次运行档位，用该档位完成评估。
func TestLevels_FallsBackToRunSlot(t *testing.T) {
	chat := &stubChat{reply: `{"levels": [{"callID": "c1", "level": "auto"}]}`}
	e := newEvaluator(t, slotResolver(map[modelsai.Slot]model.BaseChatModel{modelsai.SlotStandard: chat}), nil)
	got := e.Levels(context.Background(), modelsai.SlotStandard, testCalls("c1"))
	assertLevels(t, got, []contract.AuditLevel{contract.AuditLevelAuto})
	if len(chat.gotInputs) != 1 {
		t.Fatalf("fallback chat calls = %d, want 1", len(chat.gotInputs))
	}
}

// TestLevels_NoChatResolverFallsBackToMust 测试无可用对话客户端（解析器为空）时全部回落 must。
func TestLevels_NoChatResolverFallsBackToMust(t *testing.T) {
	e := New(Deps{Params: aitest.Params(t, nil)})
	got := e.Levels(context.Background(), modelsai.SlotStandard, testCalls("c1", "c2"))
	assertLevels(t, got, []contract.AuditLevel{contract.AuditLevelMust, contract.AuditLevelMust})
}

// TestBuildPrompt_IncludesToolDescription 测试评估上下文的工具用途说明注入：client 注册表与
// builtin / MCP schema 索引中已登记的工具在其调用条目中携带 description，未登记的工具则省略该行，
// 避免评估模型仅凭工具名字面猜测副作用而误判审核重要级。
func TestBuildPrompt_IncludesToolDescription(t *testing.T) {
	const (
		describedClientTool  = "audit_probe_described_client_tool"
		describedBackendTool = "audit_probe_described_backend_tool"
		undescribedTool      = "audit_probe_undescribed_tool"
	)
	contract.RegisterClientToolInfo(describedClientTool, &schema.ToolInfo{
		Name: describedClientTool,
		Desc: "删除指定数据源的全部采集记录（破坏性写操作）",
	})
	e := newEvaluator(t, slotResolver(nil), map[string]*schema.ToolInfo{
		describedBackendTool: {
			Name: describedBackendTool,
			Desc: "读取数据源列表（只读查询）",
		},
	})
	promptMessage, err := e.buildPrompt(context.Background(), []contract.PendingCall{
		{CallID: "c1", ToolName: describedClientTool, Arguments: `{"id": 1}`},
		{CallID: "c2", ToolName: describedBackendTool, Arguments: `{}`},
		{CallID: "c3", ToolName: undescribedTool, Arguments: `{}`},
	})
	if err != nil {
		t.Fatalf("buildPrompt() error = %v", err)
	}
	prompt := promptMessage.Content
	for _, want := range []string{
		"description: 删除指定数据源的全部采集记录（破坏性写操作）",
		"description: 读取数据源列表（只读查询）",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q: %q", want, prompt)
		}
	}
	if got := strings.Count(prompt, "description:"); got != 2 {
		t.Errorf("prompt has %d description lines, want 2 (unregistered tools must omit it): %q", got, prompt)
	}
}

// TestParseLevels_TolerantShapes 测试评估输出的容错解析：围栏、裸数组与非法 JSON。
func TestParseLevels_TolerantShapes(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    int
		wantErr bool
	}{
		{name: "envelope", content: `{"levels":[{"callID":"c1","level":"must"}]}`, want: 1},
		{name: "fenced", content: "```json\n{\"levels\":[{\"callID\":\"c1\",\"level\":\"none\"}]}\n```", want: 1},
		{name: "bare-array", content: `[{"callID":"c1","level":"auto"}]`, want: 1},
		{name: "empty", content: "   ", wantErr: true},
		{name: "invalid-json", content: "not json", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items, err := parseLevels(tc.content)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseLevels(%q) = %v, want an error", tc.content, items)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseLevels(%q) error = %v", tc.content, err)
			}
			if len(items) != tc.want {
				t.Errorf("parseLevels(%q) returned %d items, want %d", tc.content, len(items), tc.want)
			}
		})
	}
}
