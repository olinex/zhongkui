package validator

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// jsonTestObject 是 JSON 解码测试的目标结构体。
type jsonTestObject struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestDecodeObject(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr string
		want    jsonTestObject
	}{
		{"valid object", `{"name":"alice","age":3}`, "", jsonTestObject{Name: "alice", Age: 3}},
		{"surrounding whitespace", " \n\t{\"name\": \"bob\", \"age\": 4}\n ", "", jsonTestObject{Name: "bob", Age: 4}},
		{"empty", "", "json object is empty", jsonTestObject{}},
		{"blank whitespace", "  \n\t ", "json object is empty", jsonTestObject{}},
		{"pure markdown", "# title\n\n- item", "json object must be a bare object", jsonTestObject{}},
		{"code fence is rejected", "```json\n{\"name\":\"alice\"}\n```", "json object must be a bare object", jsonTestObject{}},
		{"array is not an object", `[1,2,3]`, "json object must be a bare object", jsonTestObject{}},
		{"string is not an object", `"hello"`, "json object must be a bare object", jsonTestObject{}},
		{"invalid json", `{"name":}`, "json object is invalid:", jsonTestObject{}},
		{"prefix explanation", `Here is the object: {"name":"alice"}`, "json object must be a bare object", jsonTestObject{}},
		{"suffix explanation", `{"name":"alice"} done`, "json object must be a bare object", jsonTestObject{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got jsonTestObject
			err := DecodeObject(tc.raw, &got)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("DecodeObject() = %v, want nil", err)
				}
				if got != tc.want {
					t.Fatalf("DecodeObject() = %+v, want %+v", got, tc.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("DecodeObject() = nil, want error prefixed %q", tc.wantErr)
			}
			if !strings.HasPrefix(err.Error(), tc.wantErr) {
				t.Fatalf("DecodeObject() error = %q, want prefix %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// TestDecodeOptionalObject 校验可选 JSON 对象语义：空串、空白与 null 视为未提供且不修改 dst；
// 合法对象正常解码；非对象与非法 JSON 返回错误。
func TestDecodeOptionalObject(t *testing.T) {
	cases := []struct {
		name        string
		raw         string
		wantPresent bool
		wantErr     bool
		want        jsonTestObject
	}{
		{"empty", "", false, false, jsonTestObject{}},
		{"blank", "  \n\t ", false, false, jsonTestObject{}},
		{"null", "null", false, false, jsonTestObject{}},
		{"valid object", `{"name":"alice","age":3}`, true, false, jsonTestObject{Name: "alice", Age: 3}},
		{"empty object", `{}`, true, false, jsonTestObject{Name: "untouched", Age: 99}},
		{"array", `[1,2]`, false, true, jsonTestObject{}},
		{"invalid json", `{"name":}`, false, true, jsonTestObject{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := jsonTestObject{Name: "untouched", Age: 99}
			present, err := DecodeOptionalObject(tc.raw, &got)
			if tc.wantErr {
				if err == nil {
					t.Fatal("DecodeOptionalObject() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("DecodeOptionalObject() error = %v", err)
			}
			if present != tc.wantPresent {
				t.Fatalf("DecodeOptionalObject() present = %v, want %v", present, tc.wantPresent)
			}
			if present && got != tc.want {
				t.Fatalf("DecodeOptionalObject() = %+v, want %+v", got, tc.want)
			}
			if !present && got.Name != "untouched" {
				t.Fatalf("DecodeOptionalObject() modified dst on absent input: %+v", got)
			}
		})
	}
}

// TestDecodeOptionalValue 校验可选任意 JSON 值语义：空串、空白与 null 视为未提供；其余 JSON 值
// （布尔 / 对象 / 字符串 / 数字 / 数组）正常解析；非法 JSON 返回错误。
func TestDecodeOptionalValue(t *testing.T) {
	cases := []struct {
		name        string
		raw         string
		wantPresent bool
		wantErr     bool
		want        any
	}{
		{"empty", "", false, false, nil},
		{"blank", "  \n\t ", false, false, nil},
		{"null", "null", false, false, nil},
		{"boolean", "true", true, false, true},
		{"object", `{"top_k":3}`, true, false, map[string]any{"top_k": float64(3)}},
		{"string", `"text"`, true, false, "text"},
		{"number", "42", true, false, float64(42)},
		{"array", `[1,2]`, true, false, []any{float64(1), float64(2)}},
		{"invalid json", `{enable: }`, false, true, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got any
			present, err := DecodeOptionalValue(tc.raw, &got)
			if tc.wantErr {
				if err == nil {
					t.Fatal("DecodeOptionalValue() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("DecodeOptionalValue() error = %v", err)
			}
			if present != tc.wantPresent {
				t.Fatalf("DecodeOptionalValue() present = %v, want %v", present, tc.wantPresent)
			}
			if present && !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("DecodeOptionalValue() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

// TestJSONValueKind 测试原始 JSON 值类型识别，含数字、字符串、布尔、对象、数组、null 与非法值。
func TestJSONValueKind(t *testing.T) {
	cases := []struct {
		raw  string
		want JSONKind
		ok   bool
	}{
		{`1`, JSONKindNumber, true},
		{`-1.5`, JSONKindNumber, true},
		{`1e3`, JSONKindNumber, true},
		{`"text"`, JSONKindString, true},
		{`true`, JSONKindBoolean, true},
		{`false`, JSONKindBoolean, true},
		{`{}`, JSONKindObject, true},
		{`[]`, JSONKindArray, true},
		{`null`, JSONKindNull, true},
		{` true `, JSONKindBoolean, true},
		{``, "", false},
		{`   `, "", false},
		{`{`, "", false},
		{`nul`, "", false},
		{`{"a":}`, "", false},
	}
	for _, c := range cases {
		kind, err := JSONValueKind(json.RawMessage(c.raw))
		if c.ok {
			if err != nil || kind != c.want {
				t.Errorf("JSONValueKind(%q) = (%q, %v), want (%q, nil)", c.raw, kind, err, c.want)
			}
			continue
		}
		if err == nil {
			t.Errorf("JSONValueKind(%q) error = nil, want an error", c.raw)
		}
		if kind != JSONKindInvalid {
			t.Errorf("JSONValueKind(%q) kind = %q on failure, want %q", c.raw, kind, JSONKindInvalid)
		}
	}
}

// TestJSONKindOrInvalid 测试展示用类型名：合法值返回规范类型名，空值与非法 JSON 返回 invalid。
func TestJSONKindOrInvalid(t *testing.T) {
	cases := map[string]JSONKind{
		`1`:      JSONKindNumber,
		`"text"`: JSONKindString,
		`true`:   JSONKindBoolean,
		`{}`:     JSONKindObject,
		`[]`:     JSONKindArray,
		`null`:   JSONKindNull,
		``:       JSONKindInvalid,
		`{`:      JSONKindInvalid,
	}
	for raw, want := range cases {
		if got := JSONKindOrInvalid(json.RawMessage(raw)); got != want {
			t.Errorf("JSONKindOrInvalid(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestDecodeObjectLenient(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr string
		want    jsonTestObject
	}{
		{"json fence", "```json\n{\"name\":\"alice\",\"age\":3}\n```", "", jsonTestObject{Name: "alice", Age: 3}},
		{"plain fence", "```\n{\"name\":\"bob\",\"age\":4}\n```", "", jsonTestObject{Name: "bob", Age: 4}},
		{"single line fence", "```{\"name\":\"carol\",\"age\":5}```", "", jsonTestObject{Name: "carol", Age: 5}},
		{"bare object", `{"name":"dave","age":6}`, "", jsonTestObject{Name: "dave", Age: 6}},
		{"empty", "", "json object is empty", jsonTestObject{}},
		{"invalid json inside fence", "```json\n{\"name\":}\n```", "json object is invalid:", jsonTestObject{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got jsonTestObject
			err := DecodeObjectLenient(tc.raw, &got)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("DecodeObjectLenient() = %v, want nil", err)
				}
				if got != tc.want {
					t.Fatalf("DecodeObjectLenient() = %+v, want %+v", got, tc.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("DecodeObjectLenient() = nil, want error prefixed %q", tc.wantErr)
			}
			if !strings.HasPrefix(err.Error(), tc.wantErr) {
				t.Fatalf("DecodeObjectLenient() error = %q, want prefix %q", err.Error(), tc.wantErr)
			}
		})
	}
}
