package tag

// Tag 为顶层结构，包含由逗号分隔的键值对列表
type Tag struct {
	Pairs []*Pair `parser:"@@ (',' @@)*"`
}

// Pair 为一个键值对，例如 mode="aes-gcm"
type Pair struct {
	Key   string `parser:"@Ident '='"`
	Value *Value `parser:"@@"`
}

// Value 可以是带引号的字符串或整数
type Value struct {
	String *string `parser:"@String"`
	Int    *int    `parser:"| @Int"`
}
