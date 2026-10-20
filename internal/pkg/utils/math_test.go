// 本文件覆盖 internal/pkg/utils 中三个纯计算能力的算法与域级用例：EvalMathExpression（表达式求值）、
// ConvertUnit（单位换算）与 ConvertBase（进制转换）。全部用例直接调用 utils 导出函数，不经过任何
// 适配器或运行期上下文；错误断言针对 utils 抛出的无工具名前缀的错误。

package utils

import (
	// 标准库
	"math"
	"strings"
	"testing"
)

// mathUtilNearlyEqual 判断两个浮点数是否在相对容差内相等，用于温度/速度等含分数的换算断言。
func mathUtilNearlyEqual(a, b float64) bool {
	diff := math.Abs(a - b)
	scale := math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
	return diff <= 1e-9*scale
}

// ============================================================================
// EvalMathExpression
// ============================================================================

// TestEvalMathExpression_Normal 覆盖正常路径：算术、幂、取模、比较与逻辑运算符。
func TestEvalMathExpression_Normal(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		want       any
	}{
		{"spec example", "(1+2)*3-4/2", 7.0},
		{"precedence", "1+2*3", 7.0},
		{"division", "5/2", 2.5},
		{"exponent", "2**3", 8.0},
		{"modulus", "10%3", 1.0},
		{"negate", "-3+1", -2.0},
		{"comparison true", "3 > 2", true},
		{"comparison false", "2 >= 3", false},
		{"equality", "1+1 == 2", true},
		{"logic and", "1 < 2 && 3 > 4", false},
		{"logic not", "!(1 == 1)", false},
		{"logic or", "false || true", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := EvalMathExpression(tc.expression)
			if err != nil {
				t.Fatalf("EvalMathExpression(%q) error = %v", tc.expression, err)
			}
			if got.Expression != tc.expression {
				t.Errorf("expression = %q, want %q", got.Expression, tc.expression)
			}
			switch want := tc.want.(type) {
			case float64:
				num, ok := got.Result.(float64)
				if !ok {
					t.Fatalf("result = %#v (%T), want a number", got.Result, got.Result)
				}
				if num != want {
					t.Errorf("result = %v, want %v", num, want)
				}
			case bool:
				val, ok := got.Result.(bool)
				if !ok {
					t.Fatalf("result = %#v (%T), want a bool", got.Result, got.Result)
				}
				if val != want {
					t.Errorf("result = %v, want %v", val, want)
				}
			}
		})
	}
}

// TestEvalMathExpression_Errors 覆盖错误路径：空表达式、缺失表达式与非法语法。
func TestEvalMathExpression_Errors(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		wantHint   string
	}{
		{"empty", "", "must be non-empty"},
		{"whitespace", "   ", "must be non-empty"},
		{"missing", "", "must be non-empty"},
		{"unbalanced", "(1+2", "invalid expression"},
		{"trailing op", "1+", "invalid expression"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := EvalMathExpression(tc.expression); err == nil {
				t.Fatal("error = nil, want a rejection")
			} else if !strings.Contains(err.Error(), tc.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantHint)
			}
		})
	}
}

// ============================================================================
// ConvertUnit
// ============================================================================

// TestConvertUnit_Normal 覆盖正常路径：跨倍数换算、二进制/十进制数据量、温度中点换算与单位规范化
// （大小写/空白）。
func TestConvertUnit_Normal(t *testing.T) {
	cases := []struct {
		name     string
		value    float64
		from     string
		to       string
		want     float64
		wantFrom string
		wantTo   string
		wantCat  string
	}{
		{"spec example km to m", 1, "km", "m", 1000, "km", "m", "length"},
		{"m to km", 1000, "m", "km", 1, "m", "km", "length"},
		{"km to mi", 1, "km", "mi", 1000.0 / 1609.344, "km", "mi", "length"},
		{"kg to g", 1, "kg", "g", 1000, "kg", "g", "mass"},
		{"h to s", 1, "h", "s", 3600, "h", "s", "time"},
		{"gb to mb", 1, "gb", "mb", 1000, "gb", "mb", "data"},
		{"mib to b", 1, "mib", "b", 1048576, "mib", "b", "data"},
		{"kmh to mps", 1, "kmh", "mps", 1.0 / 3.6, "kmh", "mps", "speed"},
		{"c to f", 100, "c", "f", 212, "c", "f", "temperature"},
		{"f to c", 32, "f", "c", 0, "f", "c", "temperature"},
		{"c to k", 0, "c", "k", 273.15, "c", "k", "temperature"},
		{"normalized units", 1, "  KM ", "M", 1000, "km", "m", "length"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ConvertUnit(tc.value, tc.from, tc.to)
			if err != nil {
				t.Fatalf("ConvertUnit(%v, %q, %q) error = %v", tc.value, tc.from, tc.to, err)
			}
			if !mathUtilNearlyEqual(got.Result, tc.want) {
				t.Errorf("result = %v, want %v", got.Result, tc.want)
			}
			if got.From != tc.wantFrom || got.To != tc.wantTo {
				t.Errorf("from/to = %q/%q, want %q/%q", got.From, got.To, tc.wantFrom, tc.wantTo)
			}
			if got.Category != tc.wantCat {
				t.Errorf("category = %q, want %q", got.Category, tc.wantCat)
			}
		})
	}
}

// TestConvertUnit_Errors 覆盖错误路径：未知单位、跨类别换算与空单位。
func TestConvertUnit_Errors(t *testing.T) {
	cases := []struct {
		name     string
		value    float64
		from     string
		to       string
		wantHint string
	}{
		{"unknown from", 1, "lightyear", "m", "unknown unit"},
		{"unknown to", 1, "m", "parsec", "unknown unit"},
		{"cross category", 1, "km", "kg", "different categories"},
		{"empty from", 1, "", "m", "from must be non-empty"},
		{"empty to", 1, "m", "", "to must be non-empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ConvertUnit(tc.value, tc.from, tc.to)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), tc.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantHint)
			}
		})
	}
}

// ============================================================================
// ConvertBase
// ============================================================================

// TestConvertBase_Normal 覆盖正常路径：十六进制/二进制往返、大数、负数与 36 进制。
func TestConvertBase_Normal(t *testing.T) {
	cases := []struct {
		name        string
		value       string
		fromBase    int
		toBase      int
		wantResult  string
		wantDecimal string
	}{
		{"spec example ff to 2", "ff", 16, 2, "11111111", "255"},
		{"255 to hex", "255", 10, 16, "ff", "255"},
		{"1010 to 10", "1010", 2, 10, "10", "10"},
		{"uppercase hex", "FF", 16, 10, "255", "255"},
		{"negative value", "-1a", 16, 10, "-26", "-26"},
		{"base36", "z", 36, 10, "35", "35"},
		{"big to fill", "ffffffffffffffff", 16, 10, "18446744073709551615", "18446744073709551615"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ConvertBase(tc.value, tc.fromBase, tc.toBase)
			if err != nil {
				t.Fatalf("ConvertBase(%q, %d, %d) error = %v", tc.value, tc.fromBase, tc.toBase, err)
			}
			if got.Result != tc.wantResult {
				t.Errorf("result = %q, want %q", got.Result, tc.wantResult)
			}
			if got.Decimal != tc.wantDecimal {
				t.Errorf("decimal = %q, want %q", got.Decimal, tc.wantDecimal)
			}
		})
	}
}

// TestConvertBase_Errors 覆盖错误路径：进制越界、value 对源进制非法与空 value。
func TestConvertBase_Errors(t *testing.T) {
	cases := []struct {
		name     string
		value    string
		fromBase int
		toBase   int
		wantHint string
	}{
		{"from_base too small", "101", 1, 10, "from_base 1 is out of range"},
		{"from_base too big", "101", 37, 10, "from_base 37 is out of range"},
		{"to_base too small", "101", 2, 1, "to_base 1 is out of range"},
		{"to_base too big", "101", 2, 37, "to_base 37 is out of range"},
		{"invalid digits", "g", 16, 10, "not a valid base-16 integer"},
		{"empty value", "", 10, 2, "value must be non-empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ConvertBase(tc.value, tc.fromBase, tc.toBase)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), tc.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantHint)
			}
		})
	}
}
