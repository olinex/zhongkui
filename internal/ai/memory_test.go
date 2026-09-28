package ai

import (
	// stdlib
	"context"
	"strings"
	"testing"

	// third-party
	"github.com/cloudwego/eino/components/embedding"

	// internal

	"zhongkui.com/zhongkui/internal/ai/tools"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/llm"
)

// TestNewMemoryStore_NilArgs 测试构造 MemoryStore 时的空值校验：db/embed/params 为空返回
// 英文错误，logger 为空允许通过。
// TestNewMemoryStore_NilArgs verifies the nil checks of constructing a MemoryStore: a nil
// db/embedder/params returns an English error, while a nil logger is allowed.
func TestNewMemoryStore_NilArgs(t *testing.T) {
	if _, err := NewMemoryStore(nil, nil, nil, nil); err == nil {
		t.Error("NewMemoryStore(nil, nil, nil, nil) = nil error, want an error")
	}
	if _, err := NewMemoryStore(nil, stubEmbedder{}, llm.NewParams(nil), nil); err == nil {
		t.Error("NewMemoryStore(nil, embed, params, nil) = nil error, want an error")
	}
	if _, err := NewMemoryStore(&database.DB{}, nil, llm.NewParams(nil), nil); err == nil {
		t.Error("NewMemoryStore(db, nil, params, nil) = nil error, want an error")
	}
	if _, err := NewMemoryStore(&database.DB{}, stubEmbedder{}, nil, nil); err == nil {
		t.Error("NewMemoryStore(db, embed, nil, nil) = nil error, want an error")
	}
	// logger 允许为空
	if _, err := NewMemoryStore(&database.DB{}, stubEmbedder{}, llm.NewParams(nil), nil); err != nil {
		t.Errorf("NewMemoryStore(db, embed, params, nil) error = %v, want nil", err)
	}
}

// stubEmbedder 是满足 embedder 接口的最小桩实现。
// stubEmbedder is the minimal stub satisfying the embedder interface.
type stubEmbedder struct{}

// EmbedStrings 返回与入参等长的空向量表。
// EmbedStrings returns an empty-vector list as long as the input.
func (stubEmbedder) EmbedStrings(_ context.Context, texts []string, _ ...embedding.Option) ([][]float64, error) {
	out := make([][]float64, len(texts))
	return out, nil
}

// TestParseMemoryKind 测试记忆类别解析：空串取缺省 fact，合法取值原样返回，非法取值报英文错误。
// TestParseMemoryKind verifies the memory kind parsing: an empty string yields the default fact, a
// valid value is returned unchanged, and an invalid value yields an English error.
func TestParseMemoryKind(t *testing.T) {
	cases := []struct {
		name    string
		in      llm.MemoryKind
		want    llm.MemoryKind
		wantErr bool
	}{
		{name: "空取缺省", in: "", want: llm.MemoryKindFact},
		{name: "合法事实", in: llm.MemoryKindFact, want: llm.MemoryKindFact},
		{name: "合法偏好", in: llm.MemoryKindPreference, want: llm.MemoryKindPreference},
		{name: "合法决策", in: llm.MemoryKindDecision, want: llm.MemoryKindDecision},
		{name: "合法产物", in: llm.MemoryKindArtifact, want: llm.MemoryKindArtifact},
		{name: "非法取值", in: llm.MemoryKind("bogus"), wantErr: true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := llm.ParseMemoryKind(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("llm.ParseMemoryKind(%q) = nil error, want an error", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("llm.ParseMemoryKind(%q) error = %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("llm.ParseMemoryKind(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestValidMemoryStatus 测试记忆状态取值空间校验。
// TestValidMemoryStatus verifies the memory status value-space check.
func TestValidMemoryStatus(t *testing.T) {
	if _, err := llm.ParseMemoryStatus(llm.MemoryStatusActive); err != nil {
		t.Errorf("llm.ParseMemoryStatus(active) error = %v, want nil", err)
	}
	if _, err := llm.ParseMemoryStatus(llm.MemoryStatusArchived); err != nil {
		t.Errorf("llm.ParseMemoryStatus(archived) error = %v, want nil", err)
	}
	if _, err := llm.ParseMemoryStatus(llm.MemoryStatus("bogus")); err == nil {
		t.Error("llm.ParseMemoryStatus accepted an undeclared status")
	}
}

// TestNormalizeMemoryContent 测试正文规范化：折叠全部空白并去首尾，且哈希基于规范化正文保持稳定。
// TestNormalizeMemoryContent verifies body normalization: every whitespace run is collapsed and the
// ends trimmed, and the hash based on the normalized body stays stable.
func TestNormalizeMemoryContent(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "折叠空白", in: "  甲\t乙\n\n丙  ", want: "甲 乙 丙"},
		{name: "纯空白归一为空", in: " \t\n ", want: ""},
		{name: "已是规范形", in: "rego 规则生成", want: "rego 规则生成"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeMemoryContent(tt.in); got != tt.want {
				t.Errorf("normalizeMemoryContent(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
	// 不同排版的同一正文规范化后哈希一致
	a := sha256Hex(normalizeMemoryContent("甲乙  丙\n丁"))
	b := sha256Hex(normalizeMemoryContent("甲乙 丙 丁"))
	if a != b {
		t.Errorf("hash of equivalent bodies differ: %s vs %s", a, b)
	}
}

// TestMemoryWrite_ValidationErrors 测试写入的入参校验（db/embed 未就绪即应被拦下，说明校验先于
// 落库与向量化）：空命名空间、空正文、非法 Agent 类型、非法记忆类别均返回英文错误。
// TestMemoryWrite_ValidationErrors verifies the write input validation (db/embed being unusable is
// never reached, proving validation precedes persistence and embedding): an empty namespace, an
// empty body, an invalid agent kind and an invalid memory kind all return an English error.
func TestMemoryWrite_ValidationErrors(t *testing.T) {
	store := &MemoryStore{}
	cases := []struct {
		name string
		in   tools.MemoryWriteInput
	}{
		{name: "空命名空间", in: tools.MemoryWriteInput{NamespaceName: "  ", Content: "内容"}},
		{name: "空正文", in: tools.MemoryWriteInput{NamespaceName: "default", Content: "   \n"}},
		{name: "非法 Agent 类型", in: tools.MemoryWriteInput{NamespaceName: "default", AgentKind: llm.AgentKind("unknown"), Content: "内容"}},
		{name: "非法记忆类别", in: tools.MemoryWriteInput{NamespaceName: "default", Kind: llm.MemoryKind("bogus"), Content: "内容"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.Write(context.Background(), tt.in)
			if err == nil {
				t.Fatalf("Write() = %+v, want an error", got)
			}
			if got != nil {
				t.Errorf("Write() returned a memory with an error: %+v", got)
			}
		})
	}
}

// TestTruncateBytes_Multibyte 测试按字节截断的 UTF-8 边界安全：不切断中文/emoji，maxBytes<=0
// 或未超限时原样返回。本用例复用包内既有 truncateBytes（同时被知识库工具结果与记忆注入使用）。
// TestTruncateBytes_Multibyte verifies the UTF-8-boundary safety of the byte truncation: no Chinese
// character or emoji is split, and a non-positive maxBytes or an in-range text returns the input
// unchanged. This case reuses the in-package truncateBytes (shared by the knowledge tool results
// and the memory injection).
func TestTruncateBytes_Multibyte(t *testing.T) {
	if got := truncateBytes("short", 100); got != "short" {
		t.Errorf("truncateBytes(\"short\", 100) = %q, want %q", got, "short")
	}
	if got := truncateBytes("你好世界", 0); got != "你好世界" {
		t.Errorf("truncateBytes(\"你好世界\", 0) = %q, want the input unchanged", got)
	}
	if got := truncateBytes("你好世界", -1); got != "你好世界" {
		t.Errorf("truncateBytes(\"你好世界\", -1) = %q, want the input unchanged", got)
	}
	// 4 字节处落在「好」的中间，必须回退到 3 字节边界，只保留完整的「你」
	if got := truncateBytes("你好世界", 4); got != "你..." {
		t.Errorf("truncateBytes(\"你好世界\", 4) = %q, want %q", got, "你...")
	}
	// emoji 占 4 字节，3 字节处落在其内部，必须回退到 1 字节边界
	if got := truncateBytes("a😀b", 3); got != "a..." {
		t.Errorf("truncateBytes(\"a😀b\", 3) = %q, want %q", got, "a...")
	}
	// 截断结果的前缀不得出现残缺的多字节字符
	if prefix := strings.TrimSuffix(truncateBytes("甲乙丙丁戊己庚辛", 10), "..."); !strings.Contains("甲乙丙丁戊己庚辛", prefix) {
		t.Errorf("truncateBytes prefix %q is not a valid prefix of the input", prefix)
	}
}

// TestJaccardSimilarityWithCutoff 测试带阈值的 Jaccard 相似度：阈值之上返回真实相似度，阈值
// 之下被上限剪枝归零，阈值为 0 时不做剪枝。
// TestJaccardSimilarityWithCutoff verifies the threshold-parameterized Jaccard similarity: above the
// cutoff it returns the true similarity, below it the upper-bound prune yields 0, and a cutoff of 0
// disables the prune.
func TestJaccardSimilarityWithCutoff(t *testing.T) {
	a := contentShingles("一二三四五六七八九十")     // 7 个 4-gram
	b := contentShingles("一二三四五六七八九十甲乙丙丁") // 11 个 4-gram，含 a 的全部指纹
	sim := jaccardSimilarityWithCutoff(a, b, 0)
	if !almostEqual(sim, 7.0/11.0) {
		t.Errorf("jaccard similarity = %v, want %v", sim, 7.0/11.0)
	}
	if got := jaccardSimilarityWithCutoff(a, b, 0.5); !almostEqual(got, 7.0/11.0) {
		t.Errorf("similarity above cutoff = %v, want %v", got, 7.0/11.0)
	}
	// 下限比值 |a|/|b| = 7/11 ≈ 0.636 < 0.7，阈值剪枝直接归零
	if got := jaccardSimilarityWithCutoff(a, b, 0.7); got != 0 {
		t.Errorf("similarity below cutoff = %v, want 0", got)
	}
	// 空集合始终为 0
	if got := jaccardSimilarityWithCutoff(nil, b, 0); got != 0 {
		t.Errorf("similarity with an empty set = %v, want 0", got)
	}
}

// TestDedupeSimilarResultsWithCutoff 测试带阈值的内容去重：阈值低于相似度时判重、高于相似度
// 时保留；且以 dedupeSimilarityCutoff 调用时与既有 dedupeSimilarResults 行为完全一致。
// TestDedupeSimilarResultsWithCutoff verifies the threshold-parameterized content dedupe: a cutoff
// below the similarity dedupes while one above keeps both; called with dedupeSimilarityCutoff it
// matches the existing dedupeSimilarResults exactly.
func TestDedupeSimilarResultsWithCutoff(t *testing.T) {
	hits := func() []*RetrievalResult {
		return []*RetrievalResult{
			{DocumentID: "1", Content: "一二三四五六七八九十"},     // 7 个 4-gram
			{DocumentID: "2", Content: "一二三四五六七八九十甲乙丙丁"}, // 相似度 ≈ 0.636
		}
	}
	// 阈值 0.5 < 0.636：后位被判重丢弃
	if got := dedupeSimilarResultsWithCutoff(hits(), 0.5); len(got) != 1 || got[0].DocumentID != "1" {
		t.Errorf("cutoff 0.5 kept %d hits (%v), want only the earlier one", len(got), docIDs(got))
	}
	// 阈值 0.7 > 0.636：两条都保留
	if got := dedupeSimilarResultsWithCutoff(hits(), 0.7); len(got) != 2 {
		t.Errorf("cutoff 0.7 kept %d hits, want 2", len(got))
	}
	// 与既有默认阈值入口一致，保证不回归
	legacy := docIDs(dedupeSimilarResults(hits()))
	withCutoff := docIDs(dedupeSimilarResultsWithCutoff(hits(), dedupeSimilarityCutoff))
	if strings.Join(legacy, ",") != strings.Join(withCutoff, ",") {
		t.Errorf("dedupeSimilarResults = %v, dedupeSimilarResultsWithCutoff(default) = %v", legacy, withCutoff)
	}
}

// docIDs 提取检索结果的文档 ID 序列。
// docIDs extracts the document IDs of the retrieval results.
func docIDs(hits []*RetrievalResult) []string {
	out := make([]string, 0, len(hits))
	for _, hit := range hits {
		out = append(out, hit.DocumentID)
	}
	return out
}
