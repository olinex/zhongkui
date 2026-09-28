// Package features 提供全局功能开关（feature flags）能力。
// 本文件验证功能开关列表接口的「不可用原因」转发契约。
package features

import (
	// 标准库
	"testing"

	// 第三方
	. "github.com/smartystreets/goconvey/convey"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/base"
)

// stubReasoned 具备不可用原因说明能力的桩功能：模拟 AI 助手（可用性与原因均由运行时实时判定）。
type stubReasoned struct {
	key    string
	enable bool
	reason string
}

// Key 返回桩功能键。
func (s stubReasoned) Key() string { return s.key }

// Enabled 返回桩功能的启用状态。
func (s stubReasoned) Enabled() bool { return s.enable }

// DisabledReason 返回桩功能的不可用原因。
func (s stubReasoned) DisabledReason(*base.Language) string { return s.reason }

// stubPlain 未实现 Reasoned 的桩功能：模拟密码重置（无专用不可用文案）。
type stubPlain struct{}

// Key 返回桩功能键。
func (stubPlain) Key() string { return "plain" }

// Enabled 返回桩功能的启用状态。
func (stubPlain) Enabled() bool { return false }

// TestRegistryListReason 验证列表接口按装配顺序转发实现方的不可用原因，并让未实现
// Reasoned 的功能保持空原因（前端据此回退到通用提示，而不是展示裸键或误解为可用）。
func TestRegistryListReason(t *testing.T) {
	lang := base.SupportLanguages[0]
	Convey("功能开关列表带上不可用原因", t, func() {
		registry := NewRegistry(
			stubReasoned{key: AiKey, enable: false, reason: "档位未绑定模型"},
			stubPlain{},
		)
		items := registry.List(&lang)
		So(items, ShouldHaveLength, 2)
		So(items[0].Key, ShouldEqual, AiKey)
		So(items[0].Enabled, ShouldBeFalse)
		So(items[0].Reason, ShouldEqual, "档位未绑定模型")
		So(items[1].Key, ShouldEqual, "plain")
		So(items[1].Reason, ShouldEqual, "")
	})
}
