// 本文件覆盖 Check 的校验口径：合法源码（含 HTML 标签、链式连线、subgraph、classDef、style）
// 通过官方解析器；典型非法写法定位到行并给出中文建议；围栏剥离不改变行号；其他图表类型与
// 并发调用可用。JS 引擎由 provider 统一管理，测试内构造一份并启动（首次加载约 0.7s）。

package mermaid

import (
	// 标准库
	"context"
	"strings"
	"sync"
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/jsengine"
)

// validFlowchart 是含 HTML 标签与链式连线的合法源码：官方解析需要真实 DOM，标签预处理后
// 应通过。
const validFlowchart = `flowchart TB
    subgraph S1["① 备料与预热"]
        direction LR
        A(["开始：制作蛋糕"]) --> B["清点材料<br/>面粉·鸡蛋·糖"]
        B --> C{"材料齐全?"}
        C -- 否 --> B1["补充采购"] --> B
        C -- 是 --> D["预热烤箱<br/>150℃ / 10 分钟"]
    end
    D ==> E["分离蛋清与蛋黄"]
    H == 是 ==> I["混合"]
    classDef judge fill:#fff3e0,stroke:#ef6c00,color:#e65100;
    class C judge;
    style S1 fill:#f1f8ff,stroke:#4a90d9,stroke-width:1.5px;`

// invalidFlowchart 是真实会话里出现过的错误：带文字的普通标签配了粗箭头。
const invalidFlowchart = `flowchart TD
  A --> B
  B -- 是 ==> C`

var (
	// testEngineOnce 保证整个包只启动一份测试引擎（bundle 加载约 0.7s）。
	testEngineOnce sync.Once
	// testEngine 是包级共享测试引擎。
	testEngine *jsengine.Engine
	// testEngineErr 记录引擎启动失败。
	testEngineErr error
)

// runtime 返回包级共享测试引擎，首次调用时启动。
func runtime(t *testing.T) *jsengine.Engine {
	t.Helper()
	testEngineOnce.Do(func() {
		testEngine = jsengine.New()
		testEngineErr = testEngine.Start(context.Background())
	})
	if testEngineErr != nil {
		t.Fatalf("start js engine: %v", testEngineErr)
	}
	return testEngine
}

// TestCheck_ValidFlowchart 覆盖合法源码：官方解析器应返回 flowchart-v2 且无问题。
func TestCheck_ValidFlowchart(t *testing.T) {
	result := Check(runtime(t), validFlowchart)
	if !result.Valid {
		t.Fatalf("valid = false, issues = %+v", result.Issues)
	}
	if result.DiagramType != "flowchart-v2" {
		t.Errorf("diagramType = %q, want flowchart-v2", result.DiagramType)
	}
}

// TestCheck_InvalidLabeledThick 覆盖 `-- 文本 ==>`：应定位到第 3 行，并给出可执行的修改建议。
func TestCheck_InvalidLabeledThick(t *testing.T) {
	result := Check(runtime(t), invalidFlowchart)
	if result.Valid {
		t.Fatalf("valid = true, want false")
	}
	if len(result.Issues) != 1 {
		t.Fatalf("issues = %+v, want exactly 1", result.Issues)
	}
	issue := result.Issues[0]
	if issue.Line != 3 {
		t.Errorf("line = %d, want 3", issue.Line)
	}
	if !strings.Contains(issue.Message, "Parse error on line 3") {
		t.Errorf("message = %q, want official parse error with line 3", issue.Message)
	}
	if !strings.Contains(issue.Hint, "== 文本 ==>") {
		t.Errorf("hint = %q, want it to suggest `== 文本 ==>`", issue.Hint)
	}
}

// TestCheck_FencedLineNumbers 覆盖带围栏的入参：围栏行被等长空白替换后，报错行号仍按原始入参计。
func TestCheck_FencedLineNumbers(t *testing.T) {
	source := "```mermaid\nflowchart TD\n  A --> B\n  B -- 是 ==> C\n```"
	result := Check(runtime(t), source)
	if result.Valid {
		t.Fatalf("valid = true, want false")
	}
	if len(result.Issues) != 1 || result.Issues[0].Line != 4 {
		t.Fatalf("issues = %+v, want one issue on line 4", result.Issues)
	}
}

// TestCheck_Sequence 覆盖非 flowchart 类型：官方解析器同样给出结论，不需要我们维护类型清单。
func TestCheck_Sequence(t *testing.T) {
	result := Check(runtime(t), "sequenceDiagram\n  A->>B: 你好")
	if !result.Valid {
		t.Fatalf("valid = false, issues = %+v", result.Issues)
	}
	if result.DiagramType != "sequence" {
		t.Errorf("diagramType = %q, want sequence", result.DiagramType)
	}
}

// TestCheck_UnknownType 覆盖无法识别图表类型：应判为不合法并提示首行声明。
func TestCheck_UnknownType(t *testing.T) {
	result := Check(runtime(t), "这不是一个图表")
	if result.Valid {
		t.Fatalf("valid = true, want false")
	}
	if len(result.Issues) != 1 {
		t.Fatalf("issues = %+v, want exactly 1", result.Issues)
	}
	if !strings.Contains(result.Issues[0].Hint, "首行需声明图表类型") {
		t.Errorf("hint = %q, want 首行需声明图表类型", result.Issues[0].Hint)
	}
}

// TestCheck_Empty 覆盖空源码。
func TestCheck_Empty(t *testing.T) {
	result := Check(runtime(t), "   \n  ")
	if result.Valid {
		t.Fatalf("valid = true, want false")
	}
}

// TestCheck_NilEngine 覆盖未注入引擎：返回可定位的问题而不是 panic。
func TestCheck_NilEngine(t *testing.T) {
	result := Check(nil, validFlowchart)
	if result.Valid {
		t.Fatalf("valid = true, want false")
	}
	if !strings.Contains(result.Issues[0].Message, "未注入 JS 引擎") {
		t.Errorf("message = %q, want 未注入 JS 引擎", result.Issues[0].Message)
	}
}

// TestCheck_Concurrent 覆盖并发调用：共享 VM 由引擎互斥串行化，合法/非法结论应稳定。
func TestCheck_Concurrent(t *testing.T) {
	engine := runtime(t)
	const workers = 6
	var waitGroup sync.WaitGroup
	waitGroup.Add(workers)
	for index := 0; index < workers; index++ {
		go func(worker int) {
			defer waitGroup.Done()
			for round := 0; round < 3; round++ {
				if result := Check(engine, validFlowchart); !result.Valid {
					t.Errorf("worker %d round %d: valid = false, issues = %+v", worker, round, result.Issues)
					return
				}
				if result := Check(engine, invalidFlowchart); result.Valid {
					t.Errorf("worker %d round %d: valid = true, want false", worker, round)
					return
				}
			}
		}(index)
	}
	waitGroup.Wait()
}

// TestNormalizeSource_PreservesPositions 覆盖预处理不改变行数、也不改变标签前后的列位置。
func TestNormalizeSource_PreservesPositions(t *testing.T) {
	source := "```mermaid\nflowchart TD\n  A[\"甲<br/>乙\"] --> B\n```"
	normalized, ok := normalizeSource(source)
	if !ok {
		t.Fatalf("normalizeSource ok = false")
	}
	if strings.Count(normalized, "\n") != strings.Count(source, "\n") {
		t.Errorf("line count changed: %q", normalized)
	}
	if strings.Contains(normalized, "<br/>") || strings.Contains(normalized, "```") {
		t.Errorf("normalized source still contains fence or html tag: %q", normalized)
	}
	if len(normalized) != len(source) {
		t.Errorf("length changed from %d to %d", len(source), len(normalized))
	}
}
