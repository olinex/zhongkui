package cliinput

import (
	"testing"
	"time"

	"github.com/spf13/viper"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

// testValidate 测试共享的验证器实例（binding tag）。
// 复用项目 validator 包导出的 Validate（从 Gin binding engine 获取），
// 测试用例中校验 binding:"..." 标签（如 required、min）时使用。
var testValidate = validator.Validate

// ---------------------------------------------------------------------------
// 基础错误场景
// ---------------------------------------------------------------------------

// 测试非指针目标返回错误。
func TestExtract_NonPointerTarget(t *testing.T) {
	v := viper.New()
	type S struct {
		Name string `viper:"name"`
	}
	var s S
	err := Extract(v, testValidate, s, "")
	if err == nil {
		t.Fatal("expected error for non-pointer target")
	}
}

// 测试指向非结构体的指针返回错误。
func TestExtract_PointerToNonStruct(t *testing.T) {
	v := viper.New()
	n := 42
	err := Extract(v, testValidate, &n, "")
	if err == nil {
		t.Fatal("expected error for pointer to non-struct")
	}
}

// 测试 nil 指针返回错误。
func TestExtract_NilPointer(t *testing.T) {
	v := viper.New()
	type S struct {
		Name string `viper:"name"`
	}
	var s *S
	err := Extract(v, testValidate, s, "")
	if err == nil {
		t.Fatal("expected error for nil pointer")
	}
}

// ---------------------------------------------------------------------------
// String 字段
// ---------------------------------------------------------------------------

// 测试从 viper 提取字符串字段。
func TestExtract_StringFromViper(t *testing.T) {
	v := viper.New()
	v.Set("username", "alice")

	type Input struct {
		Username string `viper:"username"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Username != "alice" {
		t.Fatalf("expected 'alice', got '%s'", in.Username)
	}
}

// 测试字符串未设置且无 prompt 时保持零值。
func TestExtract_StringEmptyFromViper_NoPrompt(t *testing.T) {
	v := viper.New()
	// viper 中未设置该 key，且没有 prompt tag，应保持零值
	type Input struct {
		Name string `viper:"name"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Name != "" {
		t.Fatalf("expected empty string, got '%s'", in.Name)
	}
}

// 测试无 viper tag 的字段被跳过。
func TestExtract_StringFieldNoViperTag(t *testing.T) {
	v := viper.New()
	v.Set("name", "alice")

	type Input struct {
		Name    string `viper:"name"`
		Ignored string // 没有 viper tag，应被跳过
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Ignored != "" {
		t.Fatalf("expected Ignored to be empty, got '%s'", in.Ignored)
	}
}

// ---------------------------------------------------------------------------
// Bool 字段
// ---------------------------------------------------------------------------

// 测试提取布尔真值。
func TestExtract_BoolFromViper_True(t *testing.T) {
	v := viper.New()
	v.Set("verbose", true)

	type Input struct {
		Verbose bool `viper:"verbose"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !in.Verbose {
		t.Fatal("expected true")
	}
}

// 测试提取布尔假值。
func TestExtract_BoolFromViper_False(t *testing.T) {
	v := viper.New()
	v.Set("verbose", false)

	type Input struct {
		Verbose bool `viper:"verbose"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Verbose {
		t.Fatal("expected false")
	}
}

// 测试布尔未设置且无 prompt 时保持零值。
func TestExtract_BoolNotSet_NoPrompt(t *testing.T) {
	v := viper.New()
	// 未设置且无 prompt，应保持零值 false
	type Input struct {
		Verbose bool `viper:"verbose"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Verbose {
		t.Fatal("expected false for unset bool without prompt")
	}
}

// ---------------------------------------------------------------------------
// 有符号整数字段
// ---------------------------------------------------------------------------

// 测试提取 int 字段。
func TestExtract_IntFromViper(t *testing.T) {
	v := viper.New()
	v.Set("count", 42)

	type Input struct {
		Count int `viper:"count"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Count != 42 {
		t.Fatalf("expected 42, got %d", in.Count)
	}
}

// 测试提取负 int 值。
func TestExtract_IntNegative(t *testing.T) {
	v := viper.New()
	v.Set("offset", -10)

	type Input struct {
		Offset int `viper:"offset"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Offset != -10 {
		t.Fatalf("expected -10, got %d", in.Offset)
	}
}

// 测试提取 int8 字段。
func TestExtract_Int8FromViper(t *testing.T) {
	v := viper.New()
	v.Set("val", int64(127))

	type Input struct {
		Val int8 `viper:"val"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Val != 127 {
		t.Fatalf("expected 127, got %d", in.Val)
	}
}

// 测试提取 int16 字段。
func TestExtract_Int16FromViper(t *testing.T) {
	v := viper.New()
	v.Set("val", int64(-32000))

	type Input struct {
		Val int16 `viper:"val"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Val != -32000 {
		t.Fatalf("expected -32000, got %d", in.Val)
	}
}

// 测试提取 int32 字段。
func TestExtract_Int32FromViper(t *testing.T) {
	v := viper.New()
	v.Set("val", int64(100000))

	type Input struct {
		Val int32 `viper:"val"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Val != 100000 {
		t.Fatalf("expected 100000, got %d", in.Val)
	}
}

// 测试提取 int64 字段。
func TestExtract_Int64FromViper(t *testing.T) {
	v := viper.New()
	v.Set("val", int64(9999999999))

	type Input struct {
		Val int64 `viper:"val"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Val != 9999999999 {
		t.Fatalf("expected 9999999999, got %d", in.Val)
	}
}

// 测试 int 未设置且无 prompt 时保持零值。
func TestExtract_IntNotSet_NoPrompt(t *testing.T) {
	v := viper.New()
	type Input struct {
		Count int `viper:"count"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Count != 0 {
		t.Fatalf("expected 0, got %d", in.Count)
	}
}

// 测试提取 int 零值。
func TestExtract_IntZeroFromViper(t *testing.T) {
	v := viper.New()
	v.Set("count", 0)

	type Input struct {
		Count int `viper:"count"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Count != 0 {
		t.Fatalf("expected 0, got %d", in.Count)
	}
}

// ---------------------------------------------------------------------------
// 无符号整数字段
// ---------------------------------------------------------------------------

// 测试提取 uint 字段。
func TestExtract_UintFromViper(t *testing.T) {
	v := viper.New()
	v.Set("port", uint64(8080))

	type Input struct {
		Port uint `viper:"port"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Port != 8080 {
		t.Fatalf("expected 8080, got %d", in.Port)
	}
}

// 测试提取 uint8 字段。
func TestExtract_Uint8FromViper(t *testing.T) {
	v := viper.New()
	v.Set("val", uint64(255))

	type Input struct {
		Val uint8 `viper:"val"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Val != 255 {
		t.Fatalf("expected 255, got %d", in.Val)
	}
}

// 测试提取 uint16 字段。
func TestExtract_Uint16FromViper(t *testing.T) {
	v := viper.New()
	v.Set("val", uint64(65535))

	type Input struct {
		Val uint16 `viper:"val"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Val != 65535 {
		t.Fatalf("expected 65535, got %d", in.Val)
	}
}

// 测试提取 uint32 字段。
func TestExtract_Uint32FromViper(t *testing.T) {
	v := viper.New()
	v.Set("val", uint64(4294967295))

	type Input struct {
		Val uint32 `viper:"val"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Val != 4294967295 {
		t.Fatalf("expected 4294967295, got %d", in.Val)
	}
}

// 测试提取 uint64 字段。
func TestExtract_Uint64FromViper(t *testing.T) {
	v := viper.New()
	v.Set("val", uint64(18446744073709551615))

	type Input struct {
		Val uint64 `viper:"val"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Val != 18446744073709551615 {
		t.Fatalf("expected max uint64, got %d", in.Val)
	}
}

// 测试 uint 未设置且无 prompt 时保持零值。
func TestExtract_UintNotSet_NoPrompt(t *testing.T) {
	v := viper.New()
	type Input struct {
		Port uint `viper:"port"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Port != 0 {
		t.Fatalf("expected 0, got %d", in.Port)
	}
}

// ---------------------------------------------------------------------------
// 浮点数字段
// ---------------------------------------------------------------------------

// 测试提取 float32 字段。
func TestExtract_Float32FromViper(t *testing.T) {
	v := viper.New()
	v.Set("ratio", 3.14)

	type Input struct {
		Ratio float32 `viper:"ratio"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// float32 精度有限，允许误差
	if in.Ratio < 3.13 || in.Ratio > 3.15 {
		t.Fatalf("expected ~3.14, got %f", in.Ratio)
	}
}

// 测试提取 float64 字段。
func TestExtract_Float64FromViper(t *testing.T) {
	v := viper.New()
	v.Set("ratio", 3.141592653589793)

	type Input struct {
		Ratio float64 `viper:"ratio"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Ratio != 3.141592653589793 {
		t.Fatalf("expected 3.141592653589793, got %f", in.Ratio)
	}
}

// 测试提取 float64 零值。
func TestExtract_Float64Zero(t *testing.T) {
	v := viper.New()
	v.Set("ratio", 0.0)

	type Input struct {
		Ratio float64 `viper:"ratio"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Ratio != 0.0 {
		t.Fatalf("expected 0.0, got %f", in.Ratio)
	}
}

// 测试提取负 float64 值。
func TestExtract_Float64Negative(t *testing.T) {
	v := viper.New()
	v.Set("temp", -273.15)

	type Input struct {
		Temp float64 `viper:"temp"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Temp != -273.15 {
		t.Fatalf("expected -273.15, got %f", in.Temp)
	}
}

// 测试浮点未设置且无 prompt 时保持零值。
func TestExtract_FloatNotSet_NoPrompt(t *testing.T) {
	v := viper.New()
	type Input struct {
		Ratio float64 `viper:"ratio"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Ratio != 0.0 {
		t.Fatalf("expected 0.0, got %f", in.Ratio)
	}
}

// ---------------------------------------------------------------------------
// time.Duration 字段
// ---------------------------------------------------------------------------

// 测试提取 time.Duration 字段。
func TestExtract_DurationFromViper(t *testing.T) {
	v := viper.New()
	v.Set("timeout", "30s")

	type Input struct {
		Timeout time.Duration `viper:"timeout"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Timeout != 30*time.Second {
		t.Fatalf("expected 30s, got %v", in.Timeout)
	}
}

// 测试以分钟为单位的 Duration。
func TestExtract_DurationMinutes(t *testing.T) {
	v := viper.New()
	v.Set("interval", "5m")

	type Input struct {
		Interval time.Duration `viper:"interval"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Interval != 5*time.Minute {
		t.Fatalf("expected 5m, got %v", in.Interval)
	}
}

// 测试复合时长的 Duration。
func TestExtract_DurationComplex(t *testing.T) {
	v := viper.New()
	v.Set("ttl", "1h30m")

	type Input struct {
		TTL time.Duration `viper:"ttl"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := 1*time.Hour + 30*time.Minute
	if in.TTL != expected {
		t.Fatalf("expected %v, got %v", expected, in.TTL)
	}
}

// 测试 Duration 未设置且无 prompt 时保持零值。
func TestExtract_DurationNotSet_NoPrompt(t *testing.T) {
	v := viper.New()
	type Input struct {
		Timeout time.Duration `viper:"timeout"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Timeout != 0 {
		t.Fatalf("expected 0, got %v", in.Timeout)
	}
}

// ---------------------------------------------------------------------------
// []string 字段
// ---------------------------------------------------------------------------

// 测试提取 []string 字段。
func TestExtract_StringSliceFromViper(t *testing.T) {
	v := viper.New()
	v.Set("tags", []string{"a", "b", "c"})

	type Input struct {
		Tags []string `viper:"tags"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(in.Tags) != 3 || in.Tags[0] != "a" || in.Tags[1] != "b" || in.Tags[2] != "c" {
		t.Fatalf("expected [a b c], got %v", in.Tags)
	}
}

// 测试空 []string 未设置时保持 nil。
func TestExtract_StringSliceEmpty(t *testing.T) {
	v := viper.New()
	// 未设置，无 prompt，应保持 nil
	type Input struct {
		Tags []string `viper:"tags"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Tags != nil {
		t.Fatalf("expected nil, got %v", in.Tags)
	}
}

// 测试提取单元素 []string。
func TestExtract_StringSliceSingleElement(t *testing.T) {
	v := viper.New()
	v.Set("hosts", []string{"localhost"})

	type Input struct {
		Hosts []string `viper:"hosts"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(in.Hosts) != 1 || in.Hosts[0] != "localhost" {
		t.Fatalf("expected [localhost], got %v", in.Hosts)
	}
}

// ---------------------------------------------------------------------------
// 嵌套结构体
// ---------------------------------------------------------------------------

// 测试提取嵌套结构体字段。
func TestExtract_NestedStruct(t *testing.T) {
	v := viper.New()
	v.Set("Server.Host", "localhost")
	v.Set("Server.Port", uint64(8080))

	type ServerConfig struct {
		Host string `viper:"Server.Host"`
		Port uint   `viper:"Server.Port"`
	}
	type Input struct {
		Server ServerConfig
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Server.Host != "localhost" {
		t.Fatalf("expected 'localhost', got '%s'", in.Server.Host)
	}
	if in.Server.Port != 8080 {
		t.Fatalf("expected 8080, got %d", in.Server.Port)
	}
}

// 测试提取匿名嵌入结构体字段。
func TestExtract_EmbeddedStruct(t *testing.T) {
	v := viper.New()
	v.Set("Base.Name", "test")

	type Base struct {
		Name string `viper:"Base.Name"`
	}
	type Input struct {
		Base // 匿名嵌入
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Base.Name != "test" {
		t.Fatalf("expected 'test', got '%s'", in.Name)
	}
}

// 测试提取多层嵌套结构体字段。
func TestExtract_DeeplyNestedStruct(t *testing.T) {
	v := viper.New()
	v.Set("A.B.Value", "deep")

	type Inner struct {
		Value string `viper:"A.B.Value"`
	}
	type Middle struct {
		B Inner
	}
	type Input struct {
		A Middle
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.A.B.Value != "deep" {
		t.Fatalf("expected 'deep', got '%s'", in.A.B.Value)
	}
}

// ---------------------------------------------------------------------------
// 混合类型
// ---------------------------------------------------------------------------

// 测试提取混合类型字段。
func TestExtract_MixedTypes(t *testing.T) {
	v := viper.New()
	v.Set("name", "alice")
	v.Set("age", 30)
	v.Set("score", 95.5)
	v.Set("active", true)
	v.Set("timeout", "10s")
	v.Set("tags", []string{"admin", "user"})

	type Input struct {
		Name    string        `viper:"name"`
		Age     int           `viper:"age"`
		Score   float64       `viper:"score"`
		Active  bool          `viper:"active"`
		Timeout time.Duration `viper:"timeout"`
		Tags    []string      `viper:"tags"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Name != "alice" {
		t.Fatalf("Name: expected 'alice', got '%s'", in.Name)
	}
	if in.Age != 30 {
		t.Fatalf("Age: expected 30, got %d", in.Age)
	}
	if in.Score != 95.5 {
		t.Fatalf("Score: expected 95.5, got %f", in.Score)
	}
	if !in.Active {
		t.Fatal("Active: expected true")
	}
	if in.Timeout != 10*time.Second {
		t.Fatalf("Timeout: expected 10s, got %v", in.Timeout)
	}
	if len(in.Tags) != 2 || in.Tags[0] != "admin" || in.Tags[1] != "user" {
		t.Fatalf("Tags: expected [admin user], got %v", in.Tags)
	}
}

// ---------------------------------------------------------------------------
// 空结构体
// ---------------------------------------------------------------------------

// 测试空结构体不报错。
func TestExtract_EmptyStruct(t *testing.T) {
	v := viper.New()
	type Input struct{}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 不支持的字段类型（应被跳过）
// ---------------------------------------------------------------------------

// 测试不支持的字段类型被跳过。
func TestExtract_UnsupportedFieldType(t *testing.T) {
	v := viper.New()
	v.Set("data", "something")

	type Input struct {
		Data map[string]string `viper:"data"` // map 类型不支持
		Name string            `viper:"name"`
	}
	v.Set("name", "alice")
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Name != "alice" {
		t.Fatalf("expected 'alice', got '%s'", in.Name)
	}
}

// 测试不支持的切片类型被跳过。
func TestExtract_UnsupportedSliceType(t *testing.T) {
	v := viper.New()
	v.Set("nums", []int{1, 2, 3}) // []int 不支持，仅支持 []string

	type Input struct {
		Nums []int  `viper:"nums"`
		Name string `viper:"name"`
	}
	v.Set("name", "bob")
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Name != "bob" {
		t.Fatalf("expected 'bob', got '%s'", in.Name)
	}
	if in.Nums != nil {
		t.Fatalf("expected nil for unsupported []int, got %v", in.Nums)
	}
}

// ---------------------------------------------------------------------------
// 带 prefix 的验证路径
// ---------------------------------------------------------------------------

// 测试带 prefix 的验证路径。
func TestExtract_WithPrefix(t *testing.T) {
	v := viper.New()
	v.Set("name", "alice")

	type Input struct {
		Name string `viper:"name"`
	}
	var in Input
	// 使用非空 prefix，验证路径会变为 "Config.Name"
	if err := Extract(v, testValidate, &in, "Config."); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Name != "alice" {
		t.Fatalf("expected 'alice', got '%s'", in.Name)
	}
}

// ---------------------------------------------------------------------------
// 验证器集成
// ---------------------------------------------------------------------------

// 测试绑定校验通过。
func TestExtract_ValidationPass(t *testing.T) {
	v := viper.New()
	v.Set("username", "alice123")

	type Input struct {
		Username string `viper:"username" binding:"required,min=6"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Username != "alice123" {
		t.Fatalf("expected 'alice123', got '%s'", in.Username)
	}
}

// 测试绑定校验失败。
func TestExtract_ValidationFail(t *testing.T) {
	v := viper.New()
	v.Set("username", "abc") // 太短，min=6

	type Input struct {
		Username string `viper:"username" binding:"required,min=6"`
	}
	var in Input
	err := Extract(v, testValidate, &in, "")
	if err == nil {
		t.Fatal("expected validation error for short username")
	}
}

// 测试嵌套字段绑定校验失败。
func TestExtract_ValidationNestedField(t *testing.T) {
	v := viper.New()
	v.Set("Server.Port", uint64(0)) // 0 不满足 min=1

	type ServerConfig struct {
		Port uint `viper:"Server.Port" binding:"required,min=1"`
	}
	type Input struct {
		Server ServerConfig
	}
	var in Input
	err := Extract(v, testValidate, &in, "")
	if err == nil {
		t.Fatal("expected validation error for port=0")
	}
}

// ---------------------------------------------------------------------------
// 多个字段部分有值
// ---------------------------------------------------------------------------

// 测试部分字段有值时的提取。
func TestExtract_PartialFieldsSet(t *testing.T) {
	v := viper.New()
	v.Set("name", "alice")
	// age 未设置

	type Input struct {
		Name string `viper:"name"`
		Age  int    `viper:"age"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Name != "alice" {
		t.Fatalf("expected 'alice', got '%s'", in.Name)
	}
	if in.Age != 0 {
		t.Fatalf("expected 0 for unset age, got %d", in.Age)
	}
}

// ---------------------------------------------------------------------------
// 边界条件：特殊字符和空字符串
// ---------------------------------------------------------------------------

// 测试含特殊字符的字符串。
func TestExtract_StringWithSpecialChars(t *testing.T) {
	v := viper.New()
	v.Set("path", "/usr/local/bin:/usr/bin")

	type Input struct {
		Path string `viper:"path"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Path != "/usr/local/bin:/usr/bin" {
		t.Fatalf("expected '/usr/local/bin:/usr/bin', got '%s'", in.Path)
	}
}

// 测试 viper 中的空字符串等同于未设置。
func TestExtract_EmptyStringFromViper(t *testing.T) {
	v := viper.New()
	v.Set("name", "")

	type Input struct {
		Name string `viper:"name"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// viper.GetString 返回空字符串，等同于未设置
	if in.Name != "" {
		t.Fatalf("expected empty, got '%s'", in.Name)
	}
}

// 测试空切片保持 nil。
func TestExtract_EmptyStringSliceFromViper(t *testing.T) {
	v := viper.New()
	v.Set("tags", []string{})

	type Input struct {
		Tags []string `viper:"tags"`
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 空切片 len==0，不满足 len(val) > 0，保持 nil
	if in.Tags != nil {
		t.Fatalf("expected nil for empty slice, got %v", in.Tags)
	}
}

// ---------------------------------------------------------------------------
// 多个嵌套结构体同级
// ---------------------------------------------------------------------------

// 测试多个同级嵌套结构体。
func TestExtract_MultipleNestedSiblings(t *testing.T) {
	v := viper.New()
	v.Set("Database.Host", "db.local")
	v.Set("Cache.Host", "cache.local")

	type DBConfig struct {
		Host string `viper:"Database.Host"`
	}
	type CacheConfig struct {
		Host string `viper:"Cache.Host"`
	}
	type Input struct {
		Database DBConfig
		Cache    CacheConfig
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Database.Host != "db.local" {
		t.Fatalf("expected 'db.local', got '%s'", in.Database.Host)
	}
	if in.Cache.Host != "cache.local" {
		t.Fatalf("expected 'cache.local', got '%s'", in.Cache.Host)
	}
}

// ---------------------------------------------------------------------------
// 所有字段都没有 viper tag
// ---------------------------------------------------------------------------

// 测试全部字段均无 viper tag 时保持零值。
func TestExtract_AllFieldsWithoutViperTag(t *testing.T) {
	v := viper.New()
	type Input struct {
		Name string
		Age  int
	}
	var in Input
	if err := Extract(v, testValidate, &in, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Name != "" || in.Age != 0 {
		t.Fatal("expected all fields to remain zero values")
	}
}
