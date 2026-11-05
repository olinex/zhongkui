package audit

import (
	// 标准库
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
)

// TestNeedsHumanReview_TruthTable 用表驱动逐项验证判定矩阵：4 档审核重要级（must / suggest /
// auto / none）与 3 档审核阀门（each / auto / none）的全部 12 种组合，断言返回值与矩阵中
// 「人工=true / 自动=false」的结论一致。
func TestNeedsHumanReview_TruthTable(t *testing.T) {
	tests := []struct {
		name  string
		level contract.AuditLevel
		valve modelsai.ReviewValve
		want  bool
	}{
		{"必须审核且逐一审核时需人工", contract.AuditLevelMust, modelsai.ReviewValveEach, true},
		{"必须审核且自动审核时需人工", contract.AuditLevelMust, modelsai.ReviewValveAuto, true},
		{"必须审核且无需审核时需人工", contract.AuditLevelMust, modelsai.ReviewValveNone, true},
		{"建议审核且逐一审核时需人工", contract.AuditLevelSuggest, modelsai.ReviewValveEach, true},
		{"建议审核且自动审核时需人工", contract.AuditLevelSuggest, modelsai.ReviewValveAuto, true},
		{"建议审核且无需审核时可自动", contract.AuditLevelSuggest, modelsai.ReviewValveNone, false},
		{"可自动审核且逐一审核时需人工", contract.AuditLevelAuto, modelsai.ReviewValveEach, true},
		{"可自动审核且自动审核时可自动", contract.AuditLevelAuto, modelsai.ReviewValveAuto, false},
		{"可自动审核且无需审核时可自动", contract.AuditLevelAuto, modelsai.ReviewValveNone, false},
		{"无需审核且逐一审核时需人工", contract.AuditLevelNone, modelsai.ReviewValveEach, true},
		{"无需审核且自动审核时可自动", contract.AuditLevelNone, modelsai.ReviewValveAuto, false},
		{"无需审核且无需审核时可自动", contract.AuditLevelNone, modelsai.ReviewValveNone, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NeedsHumanReview(tt.level, tt.valve); got != tt.want {
				t.Errorf("NeedsHumanReview(%q, %q) = %v, want %v", tt.level, tt.valve, got, tt.want)
			}
		})
	}
}
