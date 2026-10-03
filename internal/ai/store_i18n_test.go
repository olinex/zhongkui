package ai

import (
	// 标准库
	"strings"
	"testing"

	// 项目内部
	"zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/base"
)

// TestDisabledReasonLocalized 验证 AI 能力不可用的原因在全部支持语言下都解析为真实文案，
// 而不是回退成 i18n 键本身：覆盖「快照未装配」「档位无绑定」「绑定模型被禁用」「绑定模型
// 类型与档位不符」四个分支，并顺带覆盖多条原因的分隔符。该文案直接随状态接口与档位绑定
// 接口返回前端，缺键会以裸键的形式暴露给运维，故以此测试兜底（与 param_i18n_test.go
// 的键完整性守卫同一约定）。
func TestDisabledReasonLocalized(t *testing.T) {
	disabled := emptySnapshot()
	disabled.infos[ai.SlotStandard] = SlotInfo{Slot: ai.SlotStandard, Name: "gpt-4o-mini", Enabled: false}
	mismatch := emptySnapshot()
	mismatch.infos[ai.SlotStandard] = SlotInfo{Slot: ai.SlotStandard, Name: "text-embedding-3-small", Enabled: true}

	cases := []struct {
		name     string
		snap     *Snapshot
		wantData string // 必须出现在文案中的数据（占位符渲染结果）
	}{
		{name: "not-assembled", snap: nil, wantData: ""},
		{name: "slot-unbound", snap: emptySnapshot(), wantData: "standard"},
		{name: "model-disabled", snap: disabled, wantData: "gpt-4o-mini"},
		{name: "kind-mismatch", snap: mismatch, wantData: "text-embedding-3-small"},
	}
	for _, supported := range base.SupportLanguages {
		lang := supported
		for _, tc := range cases {
			reason := tc.snap.DisabledReason(&lang)
			if reason == "" {
				t.Errorf("[%s] %s: empty reason", supported.String(), tc.name)
				continue
			}
			if strings.Contains(reason, "ai.disabled.") {
				t.Errorf("[%s] %s: unresolved i18n key in %q", supported.String(), tc.name, reason)
				continue
			}
			if tc.wantData != "" && !strings.Contains(reason, tc.wantData) {
				t.Errorf("[%s] %s: %q misses the data %q", supported.String(), tc.name, reason, tc.wantData)
			}
		}
	}
}
