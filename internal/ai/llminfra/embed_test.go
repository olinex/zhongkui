package llminfra

import (
	// 标准库
	"context"
	"testing"

	// 第三方
	"github.com/cloudwego/eino/components/embedding"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/aitest"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
)

// usageStubEmbedder 是同时实现 port.UsageEmbedder 的测试桩：向量化结果沿用 aitest.StubEmbedder 的空向量，
// 并返回固定的 token 用量，用于验证 EmbedStringsWithUsage 的用量转换路径。
type usageStubEmbedder struct {
	// aitest.StubEmbedder 内嵌最小桩，提供 EmbedStrings（无用量能力）。
	aitest.StubEmbedder
	// usage 期望返回的 token 用量。
	usage embedding.TokenUsage
}

// EmbedStringsWithUsage 返回与入参等长的空向量表与固定 token 用量。
func (u usageStubEmbedder) EmbedStringsWithUsage(_ context.Context, texts []string, _ ...embedding.Option) ([][]float64, embedding.TokenUsage, error) {
	out := make([][]float64, len(texts))
	return out, u.usage, nil
}

// TestEmbedStringsWithUsage_FallbackWithoutUsage 校验 emb 不支持用量能力时回退 EmbedStrings，
// 返回等长向量与零值 contract.EmbedUsage。
func TestEmbedStringsWithUsage_FallbackWithoutUsage(t *testing.T) {
	vectors, usage, err := EmbedStringsWithUsage(context.Background(), aitest.StubEmbedder{}, []string{"a", "b"})
	if err != nil {
		t.Fatalf("EmbedStringsWithUsage() error = %v", err)
	}
	if len(vectors) != 2 {
		t.Errorf("len(vectors) = %d, want 2", len(vectors))
	}
	if usage != (contract.EmbedUsage{}) {
		t.Errorf("usage = %+v, want zero value", usage)
	}
}

// TestEmbedStringsWithUsage_UsesOptionalUsage 校验 emb 实现 port.UsageEmbedder 时调用带用量方法并
// 把 embedding.TokenUsage 转换为 contract.EmbedUsage。
func TestEmbedStringsWithUsage_UsesOptionalUsage(t *testing.T) {
	emb := usageStubEmbedder{usage: embedding.TokenUsage{PromptTokens: 3, CompletionTokens: 1, TotalTokens: 4}}
	vectors, usage, err := EmbedStringsWithUsage(context.Background(), emb, []string{"a", "b"})
	if err != nil {
		t.Fatalf("EmbedStringsWithUsage() error = %v", err)
	}
	if len(vectors) != 2 {
		t.Errorf("len(vectors) = %d, want 2", len(vectors))
	}
	want := contract.EmbedUsage{PromptTokens: 3, CompletionTokens: 1, TotalTokens: 4}
	if usage != want {
		t.Errorf("usage = %+v, want %+v", usage, want)
	}
}
