package utils

import (
	// stdlib
	"math"
	"testing"
)

// TestParseScalar 测试字面量按标量类型解析：字符串不修剪，数值与布尔修剪首尾空白。
// TestParseScalar verifies literal parsing by scalar kind: strings are not trimmed while
// numeric and boolean kinds trim surrounding whitespace.
func TestParseScalar(t *testing.T) {
	cases := []struct {
		kind    ScalarKind
		literal string
		want    any
	}{
		{ScalarKindString, "  keep me  ", "  keep me  "},
		{ScalarKindString, "", ""},
		{ScalarKindInt, "5", 5},
		{ScalarKindInt, " 5 ", 5},
		{ScalarKindInt, "-12", -12},
		{ScalarKindFloat, "0.25", 0.25},
		{ScalarKindFloat, " 2 ", 2.0},
		{ScalarKindBool, "true", true},
		{ScalarKindBool, " false ", false},
		{ScalarKindBool, "1", true},
	}
	for _, c := range cases {
		got, err := ParseScalar(c.kind, c.literal)
		if err != nil {
			t.Errorf("ParseScalar(%s, %q) error = %v", c.kind, c.literal, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseScalar(%s, %q) = %#v, want %#v", c.kind, c.literal, got, c.want)
		}
	}
}

// TestParseScalarErrors 测试解析失败与未知类型均返回错误。
// TestParseScalarErrors verifies a failed parse and an unknown kind both yield errors.
func TestParseScalarErrors(t *testing.T) {
	cases := []struct {
		kind    ScalarKind
		literal string
	}{
		{ScalarKindInt, "1.5"},
		{ScalarKindInt, ""},
		{ScalarKindInt, "abc"},
		{ScalarKindFloat, "abc"},
		{ScalarKindFloat, ""},
		{ScalarKindBool, "yes"},
		{ScalarKindBool, ""},
		{ScalarKind("unknown"), "1"},
	}
	for _, c := range cases {
		if _, err := ParseScalar(c.kind, c.literal); err == nil {
			t.Errorf("ParseScalar(%s, %q) error = nil, want an error", c.kind, c.literal)
		}
	}
}

// TestCoerceScalar 测试 JSON 原生值按标量类型收敛：数字与布尔字段的空串按零值处理，
// 整数字段拒绝非整数浮点。
// TestCoerceScalar verifies JSON native values coerced by scalar kind: an empty string on a
// numeric or boolean field yields the zero value, and an int field rejects a non-integral
// float.
func TestCoerceScalar(t *testing.T) {
	cases := []struct {
		kind  ScalarKind
		value any
		want  any
	}{
		{ScalarKindString, "text", "text"},
		{ScalarKindString, "", ""},
		{ScalarKindInt, "7", 7},
		{ScalarKindInt, "  ", 0},
		{ScalarKindInt, "", 0},
		{ScalarKindInt, float64(7), 7},
		{ScalarKindInt, 7, 7},
		{ScalarKindFloat, 7, 7.0},
		{ScalarKindFloat, "", 0.0},
		{ScalarKindFloat, float64(0.5), 0.5},
		{ScalarKindFloat, "0.5", 0.5},
		{ScalarKindBool, "", false},
		{ScalarKindBool, true, true},
		{ScalarKindBool, "1", true},
	}
	for _, c := range cases {
		got, err := CoerceScalar(c.kind, c.value)
		if err != nil {
			t.Errorf("CoerceScalar(%s, %#v) error = %v", c.kind, c.value, err)
			continue
		}
		if got != c.want {
			t.Errorf("CoerceScalar(%s, %#v) = %#v, want %#v", c.kind, c.value, got, c.want)
		}
	}
}

// TestCoerceScalarErrors 测试类型与标量类型不符时返回错误。
// TestCoerceScalarErrors verifies a type contradicting the scalar kind yields an error.
func TestCoerceScalarErrors(t *testing.T) {
	cases := []struct {
		kind  ScalarKind
		value any
	}{
		{ScalarKindInt, float64(1.5)},
		{ScalarKindInt, true},
		{ScalarKindFloat, true},
		{ScalarKindBool, float64(1)},
		{ScalarKindBool, "yes"},
		{ScalarKindString, float64(1)},
		{ScalarKindString, true},
		{ScalarKindString, 1},
		{ScalarKindBool, 1},
		{ScalarKindInt, []int{1}},
		{ScalarKind("unknown"), "1"},
	}
	for _, c := range cases {
		if _, err := CoerceScalar(c.kind, c.value); err == nil {
			t.Errorf("CoerceScalar(%s, %#v) error = nil, want an error", c.kind, c.value)
		}
	}
}

// TestCoerceScalar_IntZeroFloat 测试整数 0 的浮点表示被接受为整数字段（0 应可写回）。
// TestCoerceScalar_IntZeroFloat verifies the float representation of integer zero is accepted
// for an int field (zero must be writable).
func TestCoerceScalar_IntZeroFloat(t *testing.T) {
	got, err := CoerceScalar(ScalarKindInt, float64(0))
	if err != nil {
		t.Fatalf("CoerceScalar(int, 0.0) error = %v", err)
	}
	if got != 0 {
		t.Errorf("CoerceScalar(int, 0.0) = %#v, want 0", got)
	}
}

// TestFormatScalar 测试按标量类型格式化：与 ParseScalar 往返一致，浮点取最短表示。
// TestFormatScalar verifies formatting by scalar kind: it round-trips with ParseScalar and
// floats take their shortest form.
func TestFormatScalar(t *testing.T) {
	cases := []struct {
		kind  ScalarKind
		value any
		want  string
	}{
		{ScalarKindString, "  raw  ", "  raw  "},
		{ScalarKindString, "", ""},
		{ScalarKindInt, 5, "5"},
		{ScalarKindInt, float64(5), "5"},
		{ScalarKindInt, -12, "-12"},
		{ScalarKindFloat, 0.25, "0.25"},
		{ScalarKindFloat, 0.1, "0.1"},
		{ScalarKindFloat, "0.5", "0.5"},
		{ScalarKindBool, true, "true"},
		{ScalarKindBool, "1", "true"},
		{ScalarKindBool, "", "false"},
	}
	for _, c := range cases {
		got, err := FormatScalar(c.kind, c.value)
		if err != nil {
			t.Errorf("FormatScalar(%s, %#v) error = %v", c.kind, c.value, err)
			continue
		}
		if got != c.want {
			t.Errorf("FormatScalar(%s, %#v) = %q, want %q", c.kind, c.value, got, c.want)
		}
	}
}

// TestFormatScalarErrors 测试类型与标量类型不符时返回错误。
// TestFormatScalarErrors verifies a type contradicting the scalar kind yields an error.
func TestFormatScalarErrors(t *testing.T) {
	cases := []struct {
		kind  ScalarKind
		value any
	}{
		{ScalarKindInt, float64(1.5)},
		{ScalarKindInt, "abc"},
		{ScalarKindBool, "yes"},
		{ScalarKindString, []int{1}},
		{ScalarKind("unknown"), "1"},
	}
	for _, c := range cases {
		if _, err := FormatScalar(c.kind, c.value); err == nil {
			t.Errorf("FormatScalar(%s, %#v) error = nil, want an error", c.kind, c.value)
		}
	}
}

// TestScalarRoundTrip 测试「字面量 → 值 → 字面量」在数值与布尔类型上稳定不变。
// TestScalarRoundTrip verifies the literal → value → literal round trip is stable for
// numeric and boolean kinds.
func TestScalarRoundTrip(t *testing.T) {
	cases := []struct {
		kind    ScalarKind
		literal string
	}{
		{ScalarKindInt, "5"},
		{ScalarKindInt, "-12"},
		{ScalarKindFloat, "0.25"},
		{ScalarKindFloat, "100"},
		{ScalarKindBool, "true"},
		{ScalarKindBool, "false"},
	}
	for _, c := range cases {
		value, err := ParseScalar(c.kind, c.literal)
		if err != nil {
			t.Fatalf("ParseScalar(%s, %q) error = %v", c.kind, c.literal, err)
		}
		back, err := FormatScalar(c.kind, value)
		if err != nil {
			t.Fatalf("FormatScalar(%s, %#v) error = %v", c.kind, value, err)
		}
		if back != c.literal {
			t.Errorf("round trip %s %q = %q, want %q", c.kind, c.literal, back, c.literal)
		}
	}
}

// TestFloatNaNNotAnInt 测试 NaN 不会被误判为整数（math.Trunc(NaN) == NaN，须显式排除）。
// TestFloatNaNNotAnInt verifies NaN is never taken for an integer (math.Trunc(NaN) == NaN,
// so it must be excluded explicitly).
func TestFloatNaNNotAnInt(t *testing.T) {
	if _, err := CoerceScalar(ScalarKindInt, math.NaN()); err == nil {
		t.Errorf("CoerceScalar(int, NaN) error = nil, want an error")
	}
}
