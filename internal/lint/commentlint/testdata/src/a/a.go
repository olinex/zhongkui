// Package a 演示注释完备性检查：带注释的声明不报告，缺失注释的声明按 want 注释报告。
package a

// Inner 被内嵌的导出类型（内嵌成员不检查）。
type Inner struct{}

// Displayer 被内嵌的导出接口。
type Displayer interface {
	// Display 带注释的接口方法。
	Display()
}

// Good 带完整注释的导出类型。
type Good struct {
	// Doc 带注释的导出字段。
	Doc string
	// hidden 带注释的非导出字段。
	hidden bool
	inline int    // 行尾注释视为已有注释
	tagged string `comment:"带 comment tag 的字段"`
	Inner
}

// want +2 "导出类型 Bad 缺少注释"

type Bad struct {
	// want +2 "字段 Bad.Missing 缺少注释"

	Missing string
	// want +2 "字段 Bad.hidden 缺少注释"

	hidden int
	A, b   string
}

// Iface 带完整注释的导出接口。
type Iface interface {
	Displayer
	// Do 带注释的接口方法。
	Do()
	// want +2 "接口方法 Iface.Missing 缺少注释"

	Missing()
}

// GoodFunc 带注释的导出函数。
func GoodFunc() {}

func BadFunc() {} // want "导出函数 BadFunc 缺少注释"

// GoodMethod 带注释的导出方法。
func (Good) GoodMethod() {}

func (Good) BadMethod() {} // want "导出方法 BadMethod 缺少注释"

// GoodConst 带注释的导出常量。
const GoodConst = 1

// want +2 "导出常量 BadConst 缺少注释"

const BadConst = 2

// want +2 "导出变量 BadVar 缺少注释"

var BadVar = 3
