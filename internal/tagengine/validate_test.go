package tagengine

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

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
