package ai

import (
	// 标准库
	"context"
	"testing"

	// 第三方
	"github.com/cloudwego/eino/components/embedding"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/docconv"
)

// recordingReporter 是 SyncReporter 的测试替身：记录 Start/Step/Finish 的调用次数与参数。
type recordingReporter struct {
	// starts Start 收到的待处理总数序列。
	starts []int
	// steps Step 收到的进度明细序列。
	steps []SyncProgressItem
	// finishs Finish 的调用次数。
	finishs int
}

// Start 记录一次 Start 的待处理总数。
func (r *recordingReporter) Start(total int) {
	r.starts = append(r.starts, total)
}

// Step 记录一条进度明细。
func (r *recordingReporter) Step(item SyncProgressItem) {
	r.steps = append(r.steps, item)
}

// Finish 累计一次 Finish 调用。
func (r *recordingReporter) Finish() {
	r.finishs++
}

// usageStubEmbedder 是同时实现 usageEmbedder 的测试桩：向量化结果沿用 stubEmbedder 的空向量，
// 并返回固定的 token 用量，用于验证 embedStringsWithUsage 的用量转换路径。
type usageStubEmbedder struct {
	// stubEmbedder 内嵌最小桩，提供 EmbedStrings（无用量能力）。
	stubEmbedder
	// usage 期望返回的 token 用量。
	usage embedding.TokenUsage
}

// EmbedStringsWithUsage 返回与入参等长的空向量表与固定 token 用量。
func (u usageStubEmbedder) EmbedStringsWithUsage(_ context.Context, texts []string, _ ...embedding.Option) ([][]float64, embedding.TokenUsage, error) {
	out := make([][]float64, len(texts))
	return out, u.usage, nil
}

// TestReindexNamespace_ReportsEmptyRun 校验知识库重索引在未知命名空间（零文档）下仍上报一次
// Start(0)、一次 Finish 且零个 Step。
func TestReindexNamespace_ReportsEmptyRun(t *testing.T) {
	knowledge, err := NewKnowledge(newTestReindexDB(t), stubEmbedder{}, testParams(t, nil))
	if err != nil {
		t.Fatalf("NewKnowledge error = %v", err)
	}
	rec := &recordingReporter{}
	got, err := knowledge.ReindexNamespace(context.Background(), "absent", rec)
	if err != nil {
		t.Fatalf("ReindexNamespace(absent) error = %v", err)
	}
	if got != 0 {
		t.Errorf("ReindexNamespace(absent) = %d, want 0", got)
	}
	if len(rec.starts) != 1 || rec.starts[0] != 0 {
		t.Errorf("reporter starts = %v, want [0]", rec.starts)
	}
	if rec.finishs != 1 {
		t.Errorf("reporter finishs = %d, want 1", rec.finishs)
	}
	if len(rec.steps) != 0 {
		t.Errorf("reporter steps = %v, want none", rec.steps)
	}
}

// TestMemoryStoreReindexEmbeddings_ReportsEmptyRun 校验记忆重嵌入在未知命名空间（零记忆）下仍
// 上报一次 Start(0)、一次 Finish 且零个 Step。
func TestMemoryStoreReindexEmbeddings_ReportsEmptyRun(t *testing.T) {
	store, err := NewMemoryStore(newTestReindexDB(t), stubEmbedder{}, testParams(t, nil), nil)
	if err != nil {
		t.Fatalf("NewMemoryStore error = %v", err)
	}
	rec := &recordingReporter{}
	got, err := store.ReindexEmbeddings(context.Background(), "absent", rec)
	if err != nil {
		t.Fatalf("ReindexEmbeddings(absent) error = %v", err)
	}
	if got != 0 {
		t.Errorf("ReindexEmbeddings(absent) = %d, want 0", got)
	}
	if len(rec.starts) != 1 || rec.starts[0] != 0 {
		t.Errorf("reporter starts = %v, want [0]", rec.starts)
	}
	if rec.finishs != 1 {
		t.Errorf("reporter finishs = %d, want 1", rec.finishs)
	}
	if len(rec.steps) != 0 {
		t.Errorf("reporter steps = %v, want none", rec.steps)
	}
}

// TestReportHelpers_NilSafe 校验三个 nil 安全辅助函数在 report 为 nil 时不 panic。
func TestReportHelpers_NilSafe(t *testing.T) {
	reportStart(nil, 3)
	reportStep(nil, SyncProgressItem{Action: SyncActionReindex, Count: 1})
	reportFinish(nil)
}

// TestEmbedStringsWithUsage_FallbackWithoutUsage 校验 emb 不支持用量能力时回退 EmbedStrings，
// 返回等长向量与零值 EmbedUsage。
func TestEmbedStringsWithUsage_FallbackWithoutUsage(t *testing.T) {
	vectors, usage, err := embedStringsWithUsage(context.Background(), stubEmbedder{}, []string{"a", "b"})
	if err != nil {
		t.Fatalf("embedStringsWithUsage() error = %v", err)
	}
	if len(vectors) != 2 {
		t.Errorf("len(vectors) = %d, want 2", len(vectors))
	}
	if usage != (EmbedUsage{}) {
		t.Errorf("usage = %+v, want zero value", usage)
	}
}

// TestEmbedStringsWithUsage_UsesOptionalUsage 校验 emb 实现 usageEmbedder 时调用带用量方法并
// 把 embedding.TokenUsage 转换为 EmbedUsage。
func TestEmbedStringsWithUsage_UsesOptionalUsage(t *testing.T) {
	emb := usageStubEmbedder{usage: embedding.TokenUsage{PromptTokens: 3, CompletionTokens: 1, TotalTokens: 4}}
	vectors, usage, err := embedStringsWithUsage(context.Background(), emb, []string{"a", "b"})
	if err != nil {
		t.Fatalf("embedStringsWithUsage() error = %v", err)
	}
	if len(vectors) != 2 {
		t.Errorf("len(vectors) = %d, want 2", len(vectors))
	}
	want := EmbedUsage{PromptTokens: 3, CompletionTokens: 1, TotalTokens: 4}
	if usage != want {
		t.Errorf("usage = %+v, want %+v", usage, want)
	}
}

// TestSyncGeneratedDocs_EmptyPlanReports 校验空差异计划（库为空且无派生文档）下上报 Start(0) 与
// Finish，且零个 Step；不触发任何向量化落库（sqlite 不识别 to_tsvector，故仅覆盖空计划路径）。
func TestSyncGeneratedDocs_EmptyPlanReports(t *testing.T) {
	knowledge, err := NewKnowledge(newTestReindexDB(t), stubEmbedder{}, testParams(t, nil))
	if err != nil {
		t.Fatalf("NewKnowledge error = %v", err)
	}
	rec := &recordingReporter{}
	if err := knowledge.SyncGeneratedDocs(context.Background(), "ns", docconv.FormatGenerated, nil, rec); err != nil {
		t.Fatalf("SyncGeneratedDocs() error = %v", err)
	}
	if len(rec.starts) != 1 || rec.starts[0] != 0 {
		t.Errorf("reporter starts = %v, want [0]", rec.starts)
	}
	if rec.finishs != 1 {
		t.Errorf("reporter finishs = %d, want 1", rec.finishs)
	}
	if len(rec.steps) != 0 {
		t.Errorf("reporter steps = %v, want none", rec.steps)
	}
}
