package llm

import (
	// 标准库
	"reflect"
	"testing"
)

// TestParseExtraBody 表驱动测试 ParseExtraBody：空串、空白与 null 返回 (nil, nil)，合法 JSON 对象
// 解析为 map[string]any，非法 JSON 与非对象 JSON（数组、数字、字符串、布尔）均返回英文错误。
func TestParseExtraBody(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    map[string]any
		wantErr bool
	}{
		{name: "empty string", raw: "", want: nil, wantErr: false},
		{name: "blank string", raw: "   \n\t ", want: nil, wantErr: false},
		{name: "valid object", raw: `{"enable_thinking": false}`, want: map[string]any{"enable_thinking": false}, wantErr: false},
		{name: "empty object", raw: `{}`, want: map[string]any{}, wantErr: false},
		{name: "valid object with padding", raw: "  {\"top_k\": 3}  ", want: map[string]any{"top_k": float64(3)}, wantErr: false},
		{name: "invalid json", raw: `{enable: }`, want: nil, wantErr: true},
		{name: "array", raw: `[1, 2]`, want: nil, wantErr: true},
		{name: "number", raw: `123`, want: nil, wantErr: true},
		{name: "string", raw: `"abc"`, want: nil, wantErr: true},
		{name: "boolean", raw: `true`, want: nil, wantErr: true},
		{name: "null", raw: `null`, want: nil, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseExtraBody(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseExtraBody(%q) error = nil, want non-nil", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseExtraBody(%q) error = %v, want nil", tt.raw, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseExtraBody(%q) = %#v, want %#v", tt.raw, got, tt.want)
			}
		})
	}
}

// TestParseAPIMode 测试接口形态解析：空串取缺省 chat，chat / responses 合法（容忍空白与大小写），
// 其余取值返回英文错误。
func TestParseAPIMode(t *testing.T) {
	cases := []struct {
		raw  APIMode
		want APIMode
		ok   bool
	}{
		{"", APIModeChat, true},
		{" chat ", APIModeChat, true},
		{"RESPONSES", APIModeResponses, true},
		{"completions", "", false},
	}
	for _, c := range cases {
		mode, err := ParseAPIMode(c.raw)
		if c.ok {
			if err != nil || mode != c.want {
				t.Errorf("ParseAPIMode(%q) = (%q, %v), want (%q, nil)", c.raw, mode, err, c.want)
			}
			continue
		}
		if err == nil || mode != "" {
			t.Errorf("ParseAPIMode(%q) = (%q, %v), want an error", c.raw, mode, err)
		}
	}
}

// TestValidateAPIMode 测试模型类型与接口形态的组合校验：chat 类模型两种形态均可用，
// embedding 等非 chat 类模型只允许 chat。
func TestValidateAPIMode(t *testing.T) {
	if err := ValidateAPIMode(ModelKindChat, APIModeResponses); err != nil {
		t.Errorf("ValidateAPIMode(chat, responses) error = %v, want nil", err)
	}
	if err := ValidateAPIMode(ModelKindEmbedding, ""); err != nil {
		t.Errorf("ValidateAPIMode(embedding, default) error = %v, want nil", err)
	}
	if err := ValidateAPIMode(ModelKindEmbedding, APIModeResponses); err == nil {
		t.Error("ValidateAPIMode(embedding, responses) error = nil, want an error")
	}
	if err := ValidateAPIMode(ModelKindChat, "bogus"); err == nil {
		t.Error("ValidateAPIMode(chat, bogus) error = nil, want an error")
	}
}

// TestParseJSONValue 表驱动测试 ParseJSONValue：空串返回 (nil, nil)，合法 JSON 值解析为对应的
// any（bool / object / string / number / array），非法 JSON 返回英文错误。
func TestParseJSONValue(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    any
		wantErr bool
	}{
		{name: "empty string", raw: "", want: nil, wantErr: false},
		{name: "blank string", raw: " \t\n ", want: nil, wantErr: false},
		{name: "boolean true", raw: `true`, want: true, wantErr: false},
		{name: "boolean false", raw: `false`, want: false, wantErr: false},
		{name: "object", raw: `{"level": "high"}`, want: map[string]any{"level": "high"}, wantErr: false},
		{name: "string", raw: `"enabled"`, want: "enabled", wantErr: false},
		{name: "number", raw: `42`, want: float64(42), wantErr: false},
		{name: "array", raw: `[1, "a"]`, want: []any{float64(1), "a"}, wantErr: false},
		{name: "invalid json bare word", raw: `enabled`, want: nil, wantErr: true},
		{name: "invalid json", raw: `{level: }`, want: nil, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseJSONValue(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseJSONValue(%q) error = nil, want non-nil", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseJSONValue(%q) error = %v, want nil", tt.raw, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseJSONValue(%q) = %#v, want %#v", tt.raw, got, tt.want)
			}
		})
	}
}
