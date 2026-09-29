package d // want "包 d 缺少包注释"

// Thing 带注释的导出类型。
type Thing struct {
	// Value 带注释的导出字段。
	Value string
}
