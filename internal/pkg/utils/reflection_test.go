package utils

import "testing"

// testInner 内层嵌入结构体，用于验证多段路径解析与指针嵌入下降
type testInner struct {
	Name    string `json:"name" gorm:"column:name" mapstructure:"innerName"`
	Payload []byte `json:"payload" crypto:"mode=\"aes-gcm\",offset=100,length=32"`
}

// testOuter 外层结构体，以指针嵌入 testInner 并声明同名 Payload 字段，
// 用于验证单段路径解析到最外层（offset=200）、多段路径精确定位内层（offset=100）。
type testOuter struct {
	*testInner
	Payload []byte `json:"payload" crypto:"mode=\"aes-gcm\",offset=200,length=32"`
}

// TestFindField 测试 FindField 的单段/多段路径解析与标签匹配。
func TestFindField(t *testing.T) {
	// 单段路径（按 json 名匹配）：浅层优先，解析到最外层直接声明的字段（外层遮蔽内层同名嵌入字段）
	f, err := FindField(&testOuter{}, "payload", "json")
	if err != nil {
		t.Fatalf("FindField(payload) failed: %v", err)
	}
	if got := f.Tag.Get("crypto"); got != `mode="aes-gcm",offset=200,length=32` {
		t.Fatalf("outer payload crypto tag mismatch: %q", got)
	}

	// 多段路径（按 Go 字段名匹配，nameTag 传空）：逐层下降（含指针嵌入解引用），定位内层嵌入字段
	f, err = FindField(&testOuter{}, "testInner.Payload", "")
	if err != nil {
		t.Fatalf("FindField(testInner.Payload) failed: %v", err)
	}
	if got := f.Tag.Get("crypto"); got != `mode="aes-gcm",offset=100,length=32` {
		t.Fatalf("inner payload crypto tag mismatch: %q", got)
	}

	// 单段路径亦可命中内层字段（外层无同名冲突时），nameTag 指定 mapstructure 标签名匹配
	f, err = FindField(&testOuter{}, "innerName", "mapstructure")
	if err != nil {
		t.Fatalf("FindField(innerName) failed: %v", err)
	}
	if f.Name != "Name" {
		t.Fatalf("name field mismatch: got %s", f.Name)
	}

	// nameTag 传空时仅按 Go 字段名匹配，json 名 "payload" 应解析失败
	if _, err = FindField(&testOuter{}, "payload", ""); err == nil {
		t.Fatal("FindField(payload, \"\") should fail when only Go field names are matched")
	}
}

// TestExtractFieldTag 测试 ExtractFieldTag 提取结构体标签的行为。
func TestExtractFieldTag(t *testing.T) {
	// 按 Go 字段名解析 + 提取 gorm 标签
	got, err := ExtractFieldTag(&testOuter{}, "testInner.Name", "", "gorm")
	if err != nil {
		t.Fatalf("ExtractFieldTag(testInner.Name, gorm) failed: %v", err)
	}
	if got != "column:name" {
		t.Fatalf("gorm tag mismatch: %q", got)
	}

	// 按 json 名解析 + 提取 gorm 标签
	got, err = ExtractFieldTag(&testOuter{}, "name", "json", "gorm")
	if err != nil {
		t.Fatalf("ExtractFieldTag(name, gorm) failed: %v", err)
	}
	if got != "column:name" {
		t.Fatalf("gorm tag mismatch: %q", got)
	}

	// 标签缺失返回空字符串而非错误
	got, err = ExtractFieldTag(&testOuter{}, "payload", "json", "nonexistent")
	if err != nil {
		t.Fatalf("ExtractFieldTag missing tag failed: %v", err)
	}
	if got != "" {
		t.Fatalf("missing tag should be empty, got %q", got)
	}
}

// TestFindFieldErrors 测试 FindField 在字段不存在或输入非法时的错误处理。
func TestFindFieldErrors(t *testing.T) {
	if _, err := FindField(&testOuter{}, "NotExist", "json"); err == nil {
		t.Fatal("FindField(NotExist) should fail")
	}
	if _, err := FindField(&testOuter{}, "testInner.NotExist", ""); err == nil {
		t.Fatal("FindField(testInner.NotExist) should fail")
	}
	if _, err := FindField(42, "x", ""); err == nil {
		t.Fatal("FindField on non-struct should fail")
	}
	if _, err := FindField(nil, "x", ""); err == nil {
		t.Fatal("FindField on nil should fail")
	}
}
