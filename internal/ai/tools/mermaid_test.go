// 本文件是图表校验类内置工具 mermaid_check 的适配器级测试：覆盖 Info schema、经 InvokableRun
// 的合法/非法源码出参（JSON 线形状与行号）以及非法入参 JSON 的拒绝路径。官方解析器接入本身的
// 用例已随实现下沉到 internal/pkg/mermaid/mermaid_test.go；此处自建一份 JS 引擎用于装配工具。

package tools

import (
	// 标准库
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	// 第三方
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/jsengine"
	"zhongkuisec.com/zhongkui/internal/pkg/mermaid"
)

var (
	// mermaidEngineOnce 保证整个包只启动一份测试引擎（bundle 加载约 0.7s）。
	mermaidEngineOnce sync.Once
	// mermaidEngine 是包级共享测试引擎。
	mermaidEngine *jsengine.Engine
	// mermaidEngineErr 记录引擎启动失败。
	mermaidEngineErr error
)

// mermaidCheckForTest 返回已装配 JS 运行时的 mermaid_check 工具。
func mermaidCheckForTest(t *testing.T) tool.InvokableTool {
	t.Helper()
	mermaidEngineOnce.Do(func() {
		mermaidEngine = jsengine.New()
		mermaidEngineErr = mermaidEngine.Start(context.Background())
	})
	if mermaidEngineErr != nil {
		t.Fatalf("start js engine: %v", mermaidEngineErr)
	}
	return buildMermaidCheckTool(mermaidEngine)
}

// TestMermaidCheck_Info 覆盖 Info：工具名为 mermaid_check；code 为必填 string。
func TestMermaidCheck_Info(t *testing.T) {
	info, err := mermaidCheckForTest(t).Info(context.Background())
	if err != nil {
		t.Fatalf("Info error = %v", err)
	}
	if info.Name != "mermaid_check" {
		t.Errorf("Info name = %q, want mermaid_check", info.Name)
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema error = %v", err)
	}
	code, ok := js.Properties.Get("code")
	if !ok || code.Type != string(schema.String) {
		t.Errorf("code property = %+v (present=%v), want a string", code, ok)
	}
	if !textTestRequiredContains(js.Required, "code") {
		t.Errorf("required = %v, want code required", js.Required)
	}
}

// TestMermaidCheck_Invokable 覆盖经 InvokableRun 的合法与非法源码出参：合法源码 valid=true；
// 非法源码按行号返回官方报错与中文建议。
func TestMermaidCheck_Invokable(t *testing.T) {
	checkTool := mermaidCheckForTest(t)

	valid, err := checkTool.InvokableRun(context.Background(), `{"code":"flowchart TD\n  A --> B"}`)
	if err != nil {
		t.Fatalf("InvokableRun(valid) error = %v", err)
	}
	var validResult mermaid.CheckResult
	if uerr := json.Unmarshal([]byte(valid), &validResult); uerr != nil {
		t.Fatalf("decode valid result error = %v", uerr)
	}
	if !validResult.Valid || validResult.DiagramType != "flowchart-v2" {
		t.Errorf("valid result = %+v, want valid flowchart-v2", validResult)
	}

	invalid, err := checkTool.InvokableRun(context.Background(), `{"code":"flowchart TD\n  A -- 是 ==> B"}`)
	if err != nil {
		t.Fatalf("InvokableRun(invalid) error = %v", err)
	}
	var invalidResult mermaid.CheckResult
	if uerr := json.Unmarshal([]byte(invalid), &invalidResult); uerr != nil {
		t.Fatalf("decode invalid result error = %v", uerr)
	}
	if invalidResult.Valid {
		t.Errorf("invalid result = %+v, want valid=false", invalidResult)
	}
	if len(invalidResult.Issues) != 1 || invalidResult.Issues[0].Line != 2 {
		t.Fatalf("issues = %+v, want one issue on line 2", invalidResult.Issues)
	}
	if !strings.Contains(invalidResult.Issues[0].Hint, "== 文本 ==>") {
		t.Errorf("hint = %q, want it to suggest `== 文本 ==>`", invalidResult.Issues[0].Hint)
	}
}

// TestMermaidCheck_InvalidArguments 覆盖非法入参 JSON：由 eino 的入参解析器拒绝。
func TestMermaidCheck_InvalidArguments(t *testing.T) {
	if _, err := mermaidCheckForTest(t).InvokableRun(context.Background(), `not-json`); err == nil {
		t.Fatal("error = nil, want a rejection")
	}
}
