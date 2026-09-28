// Package tag 是结构体标签（struct tag）语法的 AST 与解析基础：定义键值对列表的
// 类型结构，供 unmarshal.go 基于 participle 语法解析器完成串到结构的映射。
package tag

// Tag 为顶层结构，包含由逗号分隔的键值对列表
type Tag struct {
	// Pairs 由逗号分隔的键值对序列
	Pairs []*Pair `parser:"@@ (',' @@)*"`
}

// Pair 为一个键值对，例如 mode="aes-gcm"
type Pair struct {
	// Key 键名，须为合法标识符
	Key string `parser:"@Ident '='"`
	// Value 键值，可为带引号的字符串或整数
	Value *Value `parser:"@@"`
}

// Value 可以是带引号的字符串或整数
type Value struct {
	// String 字符串取值，与 Int 二选一
	String *string `parser:"@String"`
	// Int 整数取值，与 String 二选一
	Int *int `parser:"| @Int"`
}
