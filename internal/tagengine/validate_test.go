package tagengine

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

func TestValidateRego(t *testing.T) {
	Convey("Test ValidateRego", t, func() {
		Convey("合法的 contains 规则应返回 nil", func() {
			source := "package zki\n" + `tags contains "x" if { input.method == "GET" }`
			So(ValidateRego(source), ShouldBeNil)
		})

		Convey("合法的整体赋值规则应返回 nil", func() {
			So(ValidateRego("package zki\n"+`tags := {"x", "y"}`), ShouldBeNil)
			So(ValidateRego("package zki\n"+`tags := {"x", "y"} if { input.method == "GET" }`), ShouldBeNil)
		})

		Convey("语法错误应返回非 nil 错误", func() {
			So(ValidateRego("package zki\n"+`tags contains (`), ShouldNotBeNil)
		})

		Convey("同名完整赋值与部分定义并存应返回非 nil 错误", func() {
			source := "package zki\n" +
				"tags := {\"x\"}\n" +
				`tags contains "y" if { input.method == "GET" }`
			So(ValidateRego(source), ShouldNotBeNil)
		})

		Convey("包名非 zki 应返回 ErrInvalidRego", func() {
			err := ValidateRego("package other\n" + `tags contains "x" if { input.method == "GET" }`)
			So(errors.Is(err, ErrInvalidRego), ShouldBeTrue)
		})

		Convey("无 tags 规则应返回 ErrInvalidRego", func() {
			err := ValidateRego("package zki\n" + `allow := true`)
			So(errors.Is(err, ErrInvalidRego), ShouldBeTrue)
		})

		Convey("空字符串应返回非 nil 错误", func() {
			So(ValidateRego(""), ShouldNotBeNil)
		})
	})
}

func TestInspectRego(t *testing.T) {
	Convey("Test InspectRego", t, func() {
		Convey("合法规则应返回空错误列表与 true", func() {
			errs, ok := InspectRego("package zki\n" + `tags contains "x" if { input.method == "GET" }`)
			So(ok, ShouldBeTrue)
			So(errs, ShouldBeEmpty)
		})

		Convey("语法错误应返回 false 且错误带行号", func() {
			errs, ok := InspectRego("package zki\n" + `tags contains (`)
			So(ok, ShouldBeFalse)
			So(errs, ShouldNotBeEmpty)
			So(errs[0].Line, ShouldBeGreaterThan, 0)
		})

		Convey("包名非 zki 应返回 ErrInvalidRego 描述且无行列定位", func() {
			errs, ok := InspectRego("package other\n" + `tags contains "x"`)
			So(ok, ShouldBeFalse)
			So(errs, ShouldHaveLength, 1)
			So(errs[0].Message, ShouldContainSubstring, ErrInvalidRego.Error())
		})

		Convey("空字符串应返回 false", func() {
			_, ok := InspectRego("")
			So(ok, ShouldBeFalse)
		})
	})
}

func TestValidateGroup(t *testing.T) {
	Convey("Test ValidateGroup", t, func() {
		Convey("多条 contains 规则共存应通过", func() {
			rules := []*tag.TagRule{
				{Name: "r1", Rego: "package zki\n" + `tags contains "a" if { input.method == "GET" }`},
				{Name: "r2", Rego: "package zki\n" + `tags contains "b" if { input.method == "POST" }`},
			}
			So(ValidateGroup(context.Background(), rules), ShouldBeNil)
		})

		Convey("完整赋值与 contains 跨规则冲突应被拦截", func() {
			rules := []*tag.TagRule{
				{Name: "r1", Rego: "package zki\n" + `tags := {"a"}`},
				{Name: "r2", Rego: "package zki\n" + `tags contains "b" if { input.method == "POST" }`},
			}
			So(ValidateGroup(context.Background(), rules), ShouldNotBeNil)
		})

		Convey("空规则集合应通过", func() {
			So(ValidateGroup(context.Background(), nil), ShouldBeNil)
		})
	})
}
