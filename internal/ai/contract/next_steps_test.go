package contract

import (
	// 标准库
	"strings"
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

// TestNextStepValidate 校验 NextStep 的 binding 标签：合法项通过；标题 / 文本 / 理由超长，以及
// 缺失文本或理由的项均被拒绝。
func TestNextStepValidate(t *testing.T) {
	valid := NextStep{Title: "继续追问", Text: "请展开讲讲", Reason: "上下文相关"}
	if err := validator.Check(&valid); err != nil {
		t.Fatalf("validator.Check(valid) error = %v, want nil", err)
	}

	overlong := func(n int) string { return strings.Repeat("字", n) }
	cases := []struct {
		name string
		step NextStep
	}{
		{"title too long", NextStep{Title: overlong(NextStepTitleMaxLength + 1), Text: "text", Reason: "reason"}},
		{"text too long", NextStep{Text: overlong(NextStepTextMaxLength + 1), Reason: "reason"}},
		{"reason too long", NextStep{Text: "text", Reason: overlong(NextStepReasonMaxLength + 1)}},
		{"empty text", NextStep{Reason: "reason"}},
		{"empty reason", NextStep{Text: "text"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := validator.Check(&c.step); err == nil {
				t.Errorf("validator.Check(%+v) error = nil, want non-nil", c.step)
			}
		})
	}
}
