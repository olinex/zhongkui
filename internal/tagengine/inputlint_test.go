// 本文件测试 Rego 输入文档字段感知 lint（LintInputRefs）。
package tagengine_test

import (
	// 标准库
	"testing"

	// 第三方
	. "github.com/smartystreets/goconvey/convey"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/tagengine"
)

// lintFields 字段感知 lint 测试使用的样例输入字段元信息。
var lintFields = []tagengine.InputFieldMeta{
	{Field: "method", Type: "string"},
	{Field: "clientIp", Type: "string"},
	{Field: "tags", Type: "array"},
}

// TestLintInputRefs 验证 LintInputRefs 对 input.<field> 引用的字段感知告警。
func TestLintInputRefs(t *testing.T) {
	Convey("字段感知 lint", t, func() {
		Convey("input.methodd 拼写错误产出 warning 且行列定位到 methodd", func() {
			// 第 3 行 "tags contains x if { input." 共 27 字符，methodd 起始列为 28
			source := "package zki\n\ntags contains x if { input.methodd == \"GET\" }"
			warnings := tagengine.LintInputRefs(source, lintFields)
			So(len(warnings), ShouldEqual, 1)
			So(warnings[0].Type, ShouldEqual, "warning")
			// Message/Hint 为 i18n 消息键，文案由接口层按请求语言翻译后输出
			So(warnings[0].Message, ShouldEqual, "tagrule.lint-field-not-exist")
			So(warnings[0].Hint, ShouldEqual, "tagrule.lint-field-hint")
			So(warnings[0].Line, ShouldEqual, 3)
			So(warnings[0].Column, ShouldEqual, 28)
			// 字段名进入 Args，由接口层翻译消息键后渲染进模板
			So(warnings[0].Args, ShouldResemble, []any{"methodd"})
		})

		Convey("合法字段 input.method 无告警", func() {
			source := "package zki\n\ntags contains x if { input.method == \"GET\" }"
			warnings := tagengine.LintInputRefs(source, lintFields)
			So(warnings, ShouldBeEmpty)
		})

		Convey("input[\"x\"] 方括号形式产出告警且字段名进入 Args", func() {
			source := "package zki\n\ntags contains v if { input[\"x\"] == \"GET\" }"
			warnings := tagengine.LintInputRefs(source, lintFields)
			So(len(warnings), ShouldEqual, 1)
			So(warnings[0].Type, ShouldEqual, "warning")
			// 方括号形式的字段名进入 Args，由接口层渲染进模板
			So(warnings[0].Args, ShouldResemble, []any{"x"})
			So(warnings[0].Line, ShouldEqual, 3)
			So(warnings[0].Column, ShouldBeGreaterThanOrEqualTo, 1)
		})

		Convey("无 input 引用无告警", func() {
			source := "package zki\n\ntags contains \"x\" if { true }"
			warnings := tagengine.LintInputRefs(source, lintFields)
			So(warnings, ShouldBeEmpty)
		})

		Convey("同一字段重复引用仅一条告警", func() {
			source := "package zki\n\ntags contains v if {\n\tinput.bad == \"GET\"\n\tinput.bad != \"POST\"\n}"
			warnings := tagengine.LintInputRefs(source, lintFields)
			So(len(warnings), ShouldEqual, 1)
			So(warnings[0].Args, ShouldResemble, []any{"bad"})
		})

		Convey("data.zki 引用不参与 lint", func() {
			source := "package zki\n\ntags contains x if { data.zki.tags[x] }"
			warnings := tagengine.LintInputRefs(source, lintFields)
			So(warnings, ShouldBeEmpty)
		})

		Convey("语法错误源码返回空列表", func() {
			source := "package zki\n\ntags contains v if { input.bad == "
			warnings := tagengine.LintInputRefs(source, lintFields)
			So(warnings, ShouldBeNil)
		})
	})
}
