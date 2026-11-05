package knowledge

import (
	// 标准库
	"context"
	"strings"
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
)

// TestPromptTemplates_ChunkAndClaims 校验分块/声明/反推提示词模板的变量注入。
func TestPromptTemplates_ChunkAndClaims(t *testing.T) {
	ctx := context.Background()
	results := []retrieval.RetrievalResult{
		{Title: "A", Content: "alpha"},
		{Title: "B", Content: "beta", Complete: true},
	}
	chunkMessage, err := buildEvalChunkPrompt(ctx, "q", results, 100)
	if err != nil {
		t.Fatalf("buildEvalChunkPrompt() error = %v", err)
	}
	for _, want := range []string{"查询：q", "上下文分块：", "[0] A", "alpha", "[1] B", "beta"} {
		if !strings.Contains(chunkMessage.Content, want) {
			t.Errorf("chunk prompt missing %q: %q", want, chunkMessage.Content)
		}
	}
	claimsMessage, err := buildClaimsPrompt(ctx, "answer", results, 100)
	if err != nil {
		t.Fatalf("buildClaimsPrompt() error = %v", err)
	}
	for _, want := range []string{"回答：\nanswer", "[0] A", "[1] B"} {
		if !strings.Contains(claimsMessage.Content, want) {
			t.Errorf("claims prompt missing %q: %q", want, claimsMessage.Content)
		}
	}
	reverseMessage, err := buildReverseQuestionsPrompt(ctx, "answer")
	if err != nil {
		t.Fatalf("buildReverseQuestionsPrompt() error = %v", err)
	}
	if reverseMessage.Content != "回答：\nanswer" {
		t.Errorf("reverse prompt = %q, want %q", reverseMessage.Content, "回答：\nanswer")
	}
}

// TestPromptTemplates_Suggestions 校验改进建议提示词模板：生效参数、回答、指标、带判定/未判定
// 的分块与告警均由变量注入。
func TestPromptTemplates_Suggestions(t *testing.T) {
	ctx := context.Background()
	results := []retrieval.RetrievalResult{
		{Title: "A", Content: "alpha"},
		{Title: "B", Content: "beta"},
	}
	ev := &DryRunEvaluation{
		Answer:         "final answer",
		Metrics:        DryRunMetrics{ContextPrecision: 0.5, Faithfulness: 1, AnswerRelevance: 0.8, ContextUtilization: 0.25},
		RelevantCount:  1,
		TotalChunks:    2,
		ChunkJudgments: []ChunkJudgment{{Index: 0, Relevant: true, Score: 0.9}},
		Warnings:       []string{"first warning", "second warning"},
	}
	opts := RetrieveOptions{TopK: 5, Candidates: 30, Fusion: "rrf", Weights: retrieval.FusionWeights{Vector: 0.6, Keyword: 0.4}}
	message, err := buildSuggestionsPrompt(ctx, "query", ev, results, opts, 100)
	if err != nil {
		t.Fatalf("buildSuggestionsPrompt() error = %v", err)
	}
	for _, want := range []string{
		"查询：query",
		"topK=5, candidates=30, fusion=rrf, weightVector=0.6, weightKeyword=0.4",
		"回答：\nfinal answer",
		"contextPrecision=0.5, faithfulness=1, answerRelevance=0.8, contextUtilization=0.25, relevantChunks=1/2",
		"[0] A（relevant=true, score=0.9）",
		"[1] B（未评判）",
		"告警：\n- first warning\n- second warning",
	} {
		if !strings.Contains(message.Content, want) {
			t.Errorf("suggestions prompt missing %q: %q", want, message.Content)
		}
	}

	// 回答缺失时模板切换到「生成失败」文案。
	failed := &DryRunEvaluation{TotalChunks: 2}
	message, err = buildSuggestionsPrompt(ctx, "query", failed, results, opts, 100)
	if err != nil {
		t.Fatalf("buildSuggestionsPrompt(no answer) error = %v", err)
	}
	if !strings.Contains(message.Content, "回答：（生成失败，未产出回答）") {
		t.Errorf("suggestions prompt missing the failure text: %q", message.Content)
	}
}

// TestPromptTemplates_Rerank 校验重排提示词模板：查询与候选块均按序号注入。
func TestPromptTemplates_Rerank(t *testing.T) {
	candidates := []*retrieval.RetrievalResult{
		{Title: "A", Content: "alpha"},
		{Title: "B", Content: "beta"},
	}
	message, err := buildRerankPrompt(context.Background(), "query", candidates, 100)
	if err != nil {
		t.Fatalf("buildRerankPrompt() error = %v", err)
	}
	for _, want := range []string{"查询：query", "候选分块：", "[0] A", "[1] B", "alpha", "beta"} {
		if !strings.Contains(message.Content, want) {
			t.Errorf("rerank prompt missing %q: %q", want, message.Content)
		}
	}
}
