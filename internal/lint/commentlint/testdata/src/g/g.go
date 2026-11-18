// Package g 校验 panic 消息英文约束。
package g

import "fmt"

// good 使用英文 panic。
func good() {
	panic("boom")
}

// bad 使用中文 panic。
func bad() {
	panic("中文") // want "panic 消息必须使用英文"
}

// badWrapped 使用中文包装错误。
func badWrapped() {
	panic(fmt.Errorf("中文错误")) // want "panic 消息必须使用英文"
}

// badMultiline 多行 panic。
func badMultiline() {
	panic(
		fmt.Sprintf(
			"中文 %s", // want "panic 消息必须使用英文"
			"x",
		),
	)
}

// badRaw 使用中文原始字符串。
func badRaw() {
	panic(`中文`) // want "panic 消息必须使用英文"
}

// notPanic 普通中文业务字符串不应被检查。
func notPanic() {
	_ = fmt.Sprintf("中文")
}
