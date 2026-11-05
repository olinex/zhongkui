package llm

import (
	// 标准库
	"testing"

	// 第三方
	"github.com/creasty/defaults"

	// 项目内部
	pkgllm "zhongkuisec.com/zhongkui/internal/pkg/llm"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

// TestLlmModelCreateInValidation 校验创建入口的形状规则由结构体标签表达：缺失端点三要素、
// 名称/模型名、非法类型均失败；完整配置在默认值填充后通过。
func TestLlmModelCreateInValidation(t *testing.T) {
	cases := []struct {
		name    string
		in      LlmModelCreateIn
		wantErr bool
	}{
		{
			name:    "empty",
			in:      LlmModelCreateIn{},
			wantErr: true,
		},
		{
			name: "invalid kind",
			in: LlmModelCreateIn{
				Name: "model-a", Kind: "other", BaseURL: "http://example.com", APIKey: "key", Model: "gpt",
			},
			wantErr: true,
		},
		{
			name: "whitespace padded name",
			in: LlmModelCreateIn{
				Name: " model-a ", Kind: pkgllm.ModelKindChat, BaseURL: "http://example.com", APIKey: "key", Model: "gpt",
			},
			wantErr: true,
		},
		{
			name: "valid",
			in: LlmModelCreateIn{
				Name: "model-a", Kind: pkgllm.ModelKindChat, BaseURL: "http://example.com", APIKey: "key", Model: "gpt",
			},
			wantErr: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := defaults.Set(&tc.in); err != nil {
				t.Fatalf("defaults.Set() error = %v", err)
			}
			err := validator.Check(&tc.in)
			if tc.wantErr && err == nil {
				t.Fatal("validator.Check() = nil, want error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validator.Check() = %v, want nil", err)
			}
		})
	}
}

// TestLlmModelTestInConditionalRequired 校验连通性测试入口的双模式：未传 id 时端点三要素必填，
// 传 id 时其余字段被忽略（空值不触发必填）。
func TestLlmModelTestInConditionalRequired(t *testing.T) {
	cases := []struct {
		name    string
		in      LlmModelTestIn
		wantErr bool
	}{
		{
			name:    "no id and no endpoint fields",
			in:      LlmModelTestIn{},
			wantErr: true,
		},
		{
			name:    "with id ignores endpoint fields",
			in:      LlmModelTestIn{ID: 7},
			wantErr: false,
		},
		{
			name: "no id with full endpoint fields",
			in: LlmModelTestIn{
				Kind: pkgllm.ModelKindChat, BaseURL: "http://example.com", APIKey: "key", Model: "gpt",
			},
			wantErr: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := defaults.Set(&tc.in); err != nil {
				t.Fatalf("defaults.Set() error = %v", err)
			}
			err := validator.Check(&tc.in)
			if tc.wantErr && err == nil {
				t.Fatal("validator.Check() = nil, want error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validator.Check() = %v, want nil", err)
			}
		})
	}
}
