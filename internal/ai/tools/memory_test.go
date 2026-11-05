// 本文件覆盖内置工具 memory_write 的纯逻辑：缺写入器、空 content、kind 校验/缺省、正常写入时
// 传入写入器的字段时间与返回 JSON。全部用例不依赖 PostgreSQL（用假 MemoryWriter 断言收到的入参），
// 也不依赖 internal/ai（本包不能反向导入）。

package tools

import (
	// 标准库
	"context"
	"encoding/json"
	"strings"
	"testing"

	// 第三方
	"github.com/cloudwego/eino/schema"

	// 项目内部
	aimodel "zhongkuisec.com/zhongkui/internal/models/ai"
)

// testSessionID 是各用例共用的固定会话键：memory_write 原样透传 SessionID，故注入与断言需一致。
var testSessionID uint = 42

// fakeMemoryWriter 是 MemoryWriter 的测试替身：记录收到的入参与调用次数，按预设返回 ID 或错误。
type fakeMemoryWriter struct {
	id    uint
	err   error
	calls int
	got   []MemoryWriteInput
}

// WriteMemory 记录入参并返回预设结果。
func (w *fakeMemoryWriter) WriteMemory(_ context.Context, in MemoryWriteInput) (uint, error) {
	w.calls++
	w.got = append(w.got, in)
	return w.id, w.err
}

// memoryRuntimeCtx 构造一个已注入假写入器与归属信息的 ctx，便于各用例复用。
func memoryRuntimeCtx(w MemoryWriter) context.Context {
	return WithMemoryRuntime(context.Background(), MemoryRuntime{
		Writer:        w,
		NamespaceName: "ns-demo",
		AgentKind:     aimodel.AgentKindRego,
		SessionID:     testSessionID,
	})
}

// TestMemoryWrite_MissingWriter 覆盖未注入运行期或写入器为 nil 时返回英文错误且不误报写入。
func TestMemoryWrite_MissingWriter(t *testing.T) {
	run := memoryWriteTool
	args := `{"content":"a stable fact"}`

	if _, err := run.InvokableRun(context.Background(), args); err == nil {
		t.Fatal("no-runtime error = nil, want an English error")
	} else if !strings.Contains(err.Error(), "no memory runtime in context") {
		t.Errorf("no-runtime error = %q, want it to mention the missing runtime", err)
	}

	nilWriter := memoryRuntimeCtx(nil)
	if _, err := run.InvokableRun(nilWriter, args); err == nil {
		t.Fatal("nil-writer error = nil, want an English error")
	}
}

// TestMemoryWrite_EmptyContent 覆盖 content 为空或全空白时被拒且不调用写入器。
func TestMemoryWrite_EmptyContent(t *testing.T) {
	cases := map[string]string{
		"empty":      `{"content":""}`,
		"whitespace": `{"content":"   \t\n"}`,
		"missing":    `{}`,
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			w := &fakeMemoryWriter{}
			if _, err := memoryWriteTool.InvokableRun(memoryRuntimeCtx(w), args); err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if w.calls != 0 {
				t.Errorf("writer calls = %d, want 0 (nothing written)", w.calls)
			}
		})
	}
}

// TestMemoryWrite_Kind 覆盖：非法 kind 被拒；空 kind 落为 fact；合法 kind 原样传递。
func TestMemoryWrite_Kind(t *testing.T) {
	invalid := &fakeMemoryWriter{}
	if _, err := memoryWriteTool.InvokableRun(memoryRuntimeCtx(invalid), `{"content":"x","kind":"bogus"}`); err == nil {
		t.Fatal("invalid-kind error = nil, want a rejection")
	} else if !strings.Contains(err.Error(), "invalid kind") {
		t.Errorf("invalid-kind error = %q, want it to mention the invalid kind", err)
	}
	if invalid.calls != 0 {
		t.Errorf("writer calls = %d, want 0 for an invalid kind", invalid.calls)
	}

	defaulted := &fakeMemoryWriter{}
	if _, err := memoryWriteTool.InvokableRun(memoryRuntimeCtx(defaulted), `{"content":"x"}`); err != nil {
		t.Fatalf("empty-kind run error = %v", err)
	}
	if defaulted.calls != 1 || defaulted.got[0].Kind != aimodel.MemoryKindFact {
		t.Errorf("empty-kind input = %+v, want kind defaulted to fact", defaulted.got)
	}

	explicit := &fakeMemoryWriter{}
	if _, err := memoryWriteTool.InvokableRun(memoryRuntimeCtx(explicit), `{"content":"x","kind":"decision"}`); err != nil {
		t.Fatalf("explicit-kind run error = %v", err)
	}
	if explicit.calls != 1 || explicit.got[0].Kind != aimodel.MemoryKindDecision {
		t.Errorf("explicit-kind input = %+v, want kind decision", explicit.got)
	}
}

// TestMemoryWrite_Success 覆盖正常调用：写入器收到的入参各字段与 ctx 注入值及工具入参一致，且返回
// 的 JSON 可解出 ID 与 saved。
func TestMemoryWrite_Success(t *testing.T) {
	w := &fakeMemoryWriter{id: 123}
	out, err := memoryWriteTool.InvokableRun(memoryRuntimeCtx(w),
		`{"content":"  normalize me  ","title":"T","kind":"preference"}`)
	if err != nil {
		t.Fatalf("InvokableRun error = %v", err)
	}
	if w.calls != 1 {
		t.Fatalf("writer calls = %d, want 1", w.calls)
	}
	got := w.got[0]
	want := MemoryWriteInput{
		NamespaceName: "ns-demo",
		AgentKind:     aimodel.AgentKindRego,
		SessionID:     testSessionID,
		Kind:          aimodel.MemoryKindPreference,
		Title:         "T",
		Content:       "normalize me",
	}
	if got != want {
		t.Errorf("writer input = %+v, want %+v", got, want)
	}

	var decoded memoryWriteResult
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("result %q is not valid JSON: %v", out, err)
	}
	if decoded.ID != 123 || !decoded.Saved {
		t.Errorf("decoded result = %+v, want id 123 and saved true", decoded)
	}
}

// TestMemoryWrite_Info 覆盖 Info 的工具名与参数 schema：content 必填、kind 带枚举取值、title 存在。
func TestMemoryWrite_Info(t *testing.T) {
	info, err := memoryWriteTool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info error = %v", err)
	}
	if info.Name != "memory_write" {
		t.Errorf("Info name = %q, want memory_write", info.Name)
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema error = %v", err)
	}
	content, ok := js.Properties.Get("content")
	if !ok || content.Type != string(schema.String) {
		t.Errorf("content property = %+v (present=%v), want a required string", content, ok)
	}
	required := false
	for _, name := range js.Required {
		if name == "content" {
			required = true
		}
	}
	if !required {
		t.Errorf("schema required = %v, want content required", js.Required)
	}
	if _, ok := js.Properties.Get("title"); !ok {
		t.Errorf("title property missing from the schema")
	}
	kind, ok := js.Properties.Get("kind")
	if !ok || len(kind.Enum) != 4 {
		t.Errorf("kind property = %+v, want the four memory kinds", kind)
	}
}

// TestMemoryWrite_BuildResolves 覆盖 init 期登记生效：Build 可按名解析 memory_write 并返回可用的
// eino 工具（内容与直接构造的实现等价）。
func TestMemoryWrite_BuildResolves(t *testing.T) {
	built, err := Build(context.Background(), "memory_write", Deps{})
	if err != nil {
		t.Fatalf("Build(memory_write) error = %v", err)
	}
	info, err := built.Info(context.Background())
	if err != nil {
		t.Fatalf("built tool Info error = %v", err)
	}
	if info.Name != "memory_write" {
		t.Errorf("built tool name = %q, want memory_write", info.Name)
	}
}

// 仅用于确认假替身满足接口（编译期断言）。
var _ MemoryWriter = (*fakeMemoryWriter)(nil)
