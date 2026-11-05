package periodicreport

import (
	// 标准库
	"encoding/json"
	"strings"
	"testing"
	"time"

	// 项目内部
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
)

// TestParsePlanID 验证计划 ID 在各种 JSON 取值形态下的解析与非正整数拒绝。
func TestParsePlanID(t *testing.T) {
	cases := []struct {
		// name 用例名称
		name string
		// payload 任务固定参数
		payload map[string]any
		// want 期望解析结果
		want uint
		// wantErr 是否期望错误
		wantErr bool
	}{
		{"float64 正整数", map[string]any{"planId": float64(42)}, 42, false},
		{"json.Number", map[string]any{"planId": json.Number("7")}, 7, false},
		{"字符串", map[string]any{"planId": "9"}, 9, false},
		{"uint", map[string]any{"planId": uint(5)}, 5, false},
		{"int", map[string]any{"planId": 11}, 11, false},
		{"float64 非整数", map[string]any{"planId": float64(3.5)}, 0, true},
		{"float64 零值", map[string]any{"planId": float64(0)}, 0, true},
		{"字符串非数字", map[string]any{"planId": "abc"}, 0, true},
		{"uint 零值", map[string]any{"planId": uint(0)}, 0, true},
		{"缺失", map[string]any{}, 0, true},
		{"不支持的类型", map[string]any{"planId": true}, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePlanID(tc.payload)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parsePlanID(%v) = %d, want an error", tc.payload, got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("parsePlanID(%v) = %d, %v, want %d", tc.payload, got, err, tc.want)
			}
		})
	}
}

// TestBuildPeriodicReportQuery 验证提示词包含计划、技能、触发时间、有效参数（保留类型）与三个
// 运行时时间，并保留输出约束。
func TestBuildPeriodicReportQuery(t *testing.T) {
	triggerAt := time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC)
	plan := &modelsai.PeriodicReportPlan{Name: "每周安全周报"}
	skill := &modelsai.AgentSkill{Name: "安全周报技能"}
	fixedFields := map[string]json.RawMessage{
		"timeOffset": json.RawMessage(`"-24h"`),
		"timeWindow": json.RawMessage(`"24h"`),
		"threshold":  json.RawMessage(`0.5`),
		"region":     json.RawMessage(`"cn"`),
	}
	bias := map[string]json.RawMessage{
		"threshold": json.RawMessage(`0.75`),
		"note":      json.RawMessage(`"重点看高危事件"`),
	}
	effective, err := buildEffectiveParameters(fixedFields, bias, triggerAt)
	if err != nil {
		t.Fatalf("buildEffectiveParameters error = %v", err)
	}
	query := buildPeriodicReportQuery(plan, skill, triggerAt, effective)
	for _, want := range []string{
		plan.Name,
		skill.Name,
		triggerAt.Format(time.RFC3339),
		`"threshold":0.75`,
		`"note":"重点看高危事件"`,
		"nowTime：2026-10-30T08:00:00Z",
		"startTime：2026-10-29T08:00:00Z",
		"endTime：2026-10-30T08:00:00Z",
		"只回复一个 JSON 对象",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("buildPeriodicReportQuery() = %q, want it to contain %q", query, want)
		}
	}
}

// TestPeriodicReportTitle 验证从 Markdown 一级标题提取标题、无标题回退与超长标题按 UTF-8 边界截断。
func TestPeriodicReportTitle(t *testing.T) {
	plan := &modelsai.PeriodicReportPlan{Name: "周报"}
	triggerAt := time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC)
	if got := periodicReportTitle("# 2026 年第 44 周安全周报\n\n正文", plan, triggerAt); got != "2026 年第 44 周安全周报" {
		t.Fatalf("periodicReportTitle() = %q, want the Markdown heading", got)
	}
	if got := periodicReportTitle("没有标题\n正文", plan, triggerAt); got != "周报 2026-10-30 08:00" {
		t.Fatalf("periodicReportTitle() = %q, want the fallback title", got)
	}
	long := "# " + strings.Repeat("长", 200)
	got := periodicReportTitle(long, plan, triggerAt)
	if len(got) > periodicReportTitleMaxBytes {
		t.Fatalf("periodicReportTitle() length = %d, want <= %d", len(got), periodicReportTitleMaxBytes)
	}
	if !strings.HasPrefix(got, "长") {
		t.Fatalf("periodicReportTitle() = %q, want a valid UTF-8 truncated title", got)
	}
}
