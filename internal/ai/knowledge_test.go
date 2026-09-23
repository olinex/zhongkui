package ai

import (
	// stdlib
	"context"
	"math"
	"strings"
	"testing"

	// third-party
	"github.com/cloudwego/eino/components/retriever"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/llm"
)

// almostEqual 报告两个浮点数是否在容差内近似相等。
// almostEqual reports whether two floats are approximately equal within a tolerance.
func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// TestFuseRRF 测试 RRF 融合：两路命中按 sum(1/(k+rank)) 打分，序与分符合预期。
// TestFuseRRF verifies RRF fusion: hits from both paths are scored by
// sum(1/(k+rank)) with the expected order and scores.
func TestFuseRRF(t *testing.T) {
	vector := []*RetrievalResult{
		{DocumentID: "1", ChunkIndex: 0, Title: "A", Score: 0.9},
		{DocumentID: "2", ChunkIndex: 0, Title: "B", Score: 0.3},
	}
	keyword := []*RetrievalResult{
		{DocumentID: "2", ChunkIndex: 0, Title: "B", Score: 0.4},
		{DocumentID: "3", ChunkIndex: 0, Title: "C", Score: 0.6},
	}
	got := fuseRRF(vector, keyword)
	if len(got) != 3 {
		t.Fatalf("fuseRRF() returned %d hits, want 3", len(got))
	}
	wantOrder := []string{"B", "A", "C"}
	for i, hit := range got {
		if hit.Title != wantOrder[i] {
			t.Errorf("fused[%d] = %q, want %q", i, hit.Title, wantOrder[i])
		}
	}
	// B 两路同现：向量路 rank1（1/62）+ 关键词路 rank0（1/61）；A 仅向量路 rank0：1/61；
	// C 仅关键词路 rank1：1/62
	if !almostEqual(got[0].Score, 1.0/62+1.0/61) {
		t.Errorf("B score = %v, want %v", got[0].Score, 1.0/62+1.0/61)
	}
	if !almostEqual(got[1].Score, 1.0/61) {
		t.Errorf("A score = %v, want %v", got[1].Score, 1.0/61)
	}
	if !almostEqual(got[2].Score, 1.0/62) {
		t.Errorf("C score = %v, want %v", got[2].Score, 1.0/62)
	}
}

// TestFuseRRF_SinglePath 测试只有一路命中时 RRF 仍按该路排序输出。
// TestFuseRRF_SinglePath verifies RRF output follows the single path when only one path
// has hits.
func TestFuseRRF_SinglePath(t *testing.T) {
	keyword := []*RetrievalResult{
		{DocumentID: "1", ChunkIndex: 0, Score: 0.1},
		{DocumentID: "2", ChunkIndex: 0, Score: 0.9},
	}
	got := fuseRRF(nil, keyword)
	if len(got) != 2 {
		t.Fatalf("fuseRRF() returned %d hits, want 2", len(got))
	}
	if got[0].DocumentID != "1" || got[1].DocumentID != "2" {
		t.Errorf("fuseRRF() order = [%s, %s], want [1, 2]", got[0].DocumentID, got[1].DocumentID)
	}
}

// TestFuseWeighted 测试加权融合：每路 min-max 归一化后按权重线性叠加。
// TestFuseWeighted verifies the weighted fusion: each path is min-max normalized then
// linearly combined by the weights.
func TestFuseWeighted(t *testing.T) {
	weights := FusionWeights{Vector: 0.6, Keyword: 0.4}
	vector := []*RetrievalResult{
		{DocumentID: "a", ChunkIndex: 0, Score: 0.9}, // norm 1.0
		{DocumentID: "b", ChunkIndex: 0, Score: 0.3}, // norm 0.0
	}
	keyword := []*RetrievalResult{
		{DocumentID: "b", ChunkIndex: 0, Score: 0.4}, // norm 0.0
		{DocumentID: "c", ChunkIndex: 0, Score: 0.6}, // norm 1.0
	}
	got := fuseWeighted(vector, keyword, weights)
	if len(got) != 3 {
		t.Fatalf("fuseWeighted() returned %d hits, want 3", len(got))
	}
	wantOrder := []string{"a", "c", "b"}
	for i, hit := range got {
		if hit.DocumentID != wantOrder[i] {
			t.Errorf("fused[%d] = %q, want %q", i, hit.DocumentID, wantOrder[i])
		}
	}
	if !almostEqual(got[0].Score, 0.6) {
		t.Errorf("a score = %v, want 0.6", got[0].Score)
	}
	if !almostEqual(got[1].Score, 0.4) {
		t.Errorf("c score = %v, want 0.4", got[1].Score)
	}
	if !almostEqual(got[2].Score, 0) {
		t.Errorf("b score = %v, want 0", got[2].Score)
	}
}

// TestFuseWeighted_FlatPath 测试一路内所有分值相同（min==max）时归一化为 0.5。
// TestFuseWeighted_FlatPath verifies a path whose scores are all equal (min==max)
// normalizes to 0.5.
func TestFuseWeighted_FlatPath(t *testing.T) {
	weights := FusionWeights{Vector: 0.6, Keyword: 0.4}
	vector := []*RetrievalResult{{DocumentID: "a", ChunkIndex: 0, Score: 0.5}}
	keyword := []*RetrievalResult{{DocumentID: "a", ChunkIndex: 0, Score: 0.7}}
	got := fuseWeighted(vector, keyword, weights)
	if len(got) != 1 {
		t.Fatalf("fuseWeighted() returned %d hits, want 1", len(got))
	}
	if !almostEqual(got[0].Score, 0.5) {
		t.Errorf("score = %v, want 0.5 (flat path normalization)", got[0].Score)
	}
}

// TestSelectTopK 测试取前 topK 条：topK 非正数或超过长度时返回全部。
// TestSelectTopK verifies taking the first topK hits: a non-positive topK or topK beyond
// the length returns all hits.
func TestSelectTopK(t *testing.T) {
	cands := []*RetrievalResult{{DocumentID: "1"}, {DocumentID: "2"}, {DocumentID: "3"}}
	if got := selectTopK(cands, 0); len(got) != 3 {
		t.Errorf("selectTopK(cands, 0) = %d hits, want 3", len(got))
	}
	if got := selectTopK(cands, 10); len(got) != 3 {
		t.Errorf("selectTopK(cands, 10) = %d hits, want 3", len(got))
	}
	got := selectTopK(cands, 2)
	if len(got) != 2 || got[0].DocumentID != "1" || got[1].DocumentID != "2" {
		t.Errorf("selectTopK(cands, 2) = %v, want the first two hits", got)
	}
}

// TestParseRerankIndices 测试解析模型返回的序号数组（含浮点数组与 ```json 围栏包裹）。
// TestParseRerankIndices verifies parsing the index array returned by the model
// (including float arrays and ```json fence wrapping).
func TestParseRerankIndices(t *testing.T) {
	cases := []struct {
		in   string
		want []int
	}{
		{in: "[2, 0, 1]", want: []int{2, 0, 1}},
		{in: "[2.0, 0.5]", want: []int{2, 0}},
		{in: "```json\n[1, 0]\n```", want: []int{1, 0}},
		{in: "```\n[1]\n```", want: []int{1}},
		{in: "[]", want: nil},
	}
	for _, tt := range cases {
		got, err := parseRerankIndices(tt.in)
		if err != nil {
			t.Errorf("parseRerankIndices(%q) error = %v", tt.in, err)
			continue
		}
		if len(got) != len(tt.want) {
			t.Errorf("parseRerankIndices(%q) = %v, want %v", tt.in, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("parseRerankIndices(%q) = %v, want %v", tt.in, got, tt.want)
				break
			}
		}
	}
}

// TestParseRerankIndices_Invalid 测试非法响应返回错误（上层降级为融合排序）。
// TestParseRerankIndices_Invalid verifies an invalid response yields an error (the caller
// degrades to the fusion order).
func TestParseRerankIndices_Invalid(t *testing.T) {
	for _, in := range []string{"", "not json", "{\"a\":1}"} {
		if _, err := parseRerankIndices(in); err == nil {
			t.Errorf("parseRerankIndices(%q) error = nil, want error", in)
		}
	}
}

// TestVectorLiteral 测试向量序列化为 pgvector 字面量。
// TestVectorLiteral verifies serializing a vector into a pgvector literal.
func TestVectorLiteral(t *testing.T) {
	if got := vectorLiteral([]float64{0.1, 0.25, 1}); got != "[0.1,0.25,1]" {
		t.Errorf("vectorLiteral() = %q, want %q", got, "[0.1,0.25,1]")
	}
	if got := vectorLiteral(nil); got != "[]" {
		t.Errorf("vectorLiteral(nil) = %q, want %q", got, "[]")
	}
}

// TestToFloat32 测试 float64 向量转 float32 存储类型。
// TestToFloat32 verifies converting a float64 vector into the float32 storage type.
func TestToFloat32(t *testing.T) {
	got := toFloat32([]float64{0.1, 0.5, -2})
	want := []float32{0.1, 0.5, -2}
	if len(got) != len(want) {
		t.Fatalf("toFloat32() length = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("toFloat32()[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestSha256Hex 测试内容摘要为 sha256 十六进制（已知向量）。
// TestSha256Hex verifies the content digest is the sha256 hex (known vector).
func TestSha256Hex(t *testing.T) {
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got := sha256Hex("abc"); got != want {
		t.Errorf("sha256Hex(\"abc\") = %q, want %q", got, want)
	}
}

// TestTruncateText 测试文本截断：短文本原样返回，长文本保留前 max 字符并追加省略号。
// TestTruncateText verifies text truncation: short text is unchanged, long text keeps the
// first max characters and appends an ellipsis.
func TestTruncateText(t *testing.T) {
	if got := truncateText("short", 100); got != "short" {
		t.Errorf("truncateText(\"short\", 100) = %q, want %q", got, "short")
	}
	if got := truncateText("hello world", 5); got != "hello..." {
		t.Errorf("truncateText(\"hello world\", 5) = %q, want %q", got, "hello...")
	}
	// 多字节安全：按字符截断而非字节
	if got := truncateText("你好世界", 2); got != "你好..." {
		t.Errorf("truncateText(\"你好世界\", 2) = %q, want %q", got, "你好...")
	}
}

// TestToSchemaDocuments_RoundTrip 测试 RetrievalResult → schema.Document →
// ToRetrievalResults 往返不丢字段。
// TestToSchemaDocuments_RoundTrip verifies the RetrievalResult → schema.Document →
// ToRetrievalResults round trip keeps every field.
func TestToSchemaDocuments_RoundTrip(t *testing.T) {
	results := []*RetrievalResult{
		{DocumentID: "7", Title: "T1", ChunkIndex: 2, Content: "text", Score: 0.5},
		{DocumentID: "8", Title: "T2", ChunkIndex: 0, Content: "other", Score: 0.1},
	}
	docs := toSchemaDocuments(results)
	if len(docs) != 2 {
		t.Fatalf("toSchemaDocuments() returned %d docs, want 2", len(docs))
	}
	back := ToRetrievalResults(docs)
	for i, r := range back {
		w := *results[i]
		if r.DocumentID != w.DocumentID || r.Title != w.Title || r.ChunkIndex != w.ChunkIndex ||
			r.Content != w.Content || !almostEqual(r.Score, w.Score) {
			t.Errorf("round trip[%d] = %+v, want %+v", i, r, w)
		}
	}
}

// TestSortedResults_Deterministic 测试同分条目按 docID/chunkIndex 稳定排序。
// TestSortedResults_Deterministic verifies ties are broken deterministically by
// docID/chunkIndex.
func TestSortedResults_Deterministic(t *testing.T) {
	scores := map[chunkKey]*RetrievalResult{
		{docID: "a", index: 1}: {DocumentID: "a", ChunkIndex: 1, Score: 1},
		{docID: "b", index: 0}: {DocumentID: "b", ChunkIndex: 0, Score: 1},
		{docID: "c", index: 0}: {DocumentID: "c", ChunkIndex: 0, Score: 2},
	}
	got := sortedResults(scores)
	wantOrder := []string{"c", "a", "b"}
	if len(got) != 3 {
		t.Fatalf("sortedResults() returned %d hits, want 3", len(got))
	}
	for i, hit := range got {
		if hit.DocumentID != wantOrder[i] {
			t.Errorf("sorted[%d] = %q, want %q", i, hit.DocumentID, wantOrder[i])
		}
	}
}

// TestRetrieveOptions 测试实现专属选项经 retriever.Option 注入并合并到默认值。
// TestRetrieveOptions verifies implementation-specific options are injected via
// retriever.Option and merged onto the defaults.
func TestRetrieveOptions(t *testing.T) {
	o := retriever.GetImplSpecificOptions(&RetrieveOptions{TopK: 5}, WithNamespace("ns"),
		WithDocIDs([]uint{1, 2}), WithRetrieveTopK(3), WithCandidates(9),
		WithFusion("weighted"), WithWeights(FusionWeights{Vector: 1, Keyword: 0}))
	if o.NamespaceName != "ns" {
		t.Errorf("NamespaceName = %q, want %q", o.NamespaceName, "ns")
	}
	if len(o.DocIDs) != 2 || o.DocIDs[0] != 1 || o.DocIDs[1] != 2 {
		t.Errorf("DocIDs = %v, want [1 2]", o.DocIDs)
	}
	if o.TopK != 3 {
		t.Errorf("TopK = %d, want 3", o.TopK)
	}
	if o.Candidates != 9 {
		t.Errorf("Candidates = %d, want 9", o.Candidates)
	}
	if o.Fusion != "weighted" {
		t.Errorf("Fusion = %q, want %q", o.Fusion, "weighted")
	}
	if o.Weights.Vector != 1 || o.Weights.Keyword != 0 {
		t.Errorf("Weights = %+v, want vector=1 keyword=0", o.Weights)
	}
}

// TestNewKnowledge_Validation 测试构造参数校验：nil db/embed/params 返回英文错误。
// TestNewKnowledge_Validation verifies constructor validation: nil db/embed/params yield an
// English error.
func TestNewKnowledge_Validation(t *testing.T) {
	params := testParams(t, nil)
	if _, err := NewKnowledge(nil, nil, params); err == nil || !strings.Contains(err.Error(), "nil database") {
		t.Errorf("NewKnowledge(nil db) error = %v, want nil-database error", err)
	}
	embed := &llm.EmbedClient{}
	if _, err := NewKnowledge(nil, embed, params); err == nil || !strings.Contains(err.Error(), "nil database") {
		t.Errorf("NewKnowledge(nil db) error = %v, want nil-database error", err)
	}
	if _, err := NewKnowledge(nil, nil, params); err == nil {
		t.Errorf("NewKnowledge() error = nil, want error")
	}
	if _, err := NewRetriever(nil, embed, nil, params); err == nil || !strings.Contains(err.Error(), "nil database") {
		t.Errorf("NewRetriever(nil db) error = %v, want nil-database error", err)
	}
}

// TestRetriever_EmptyQuery 测试空查询直接报错（不调用 embed）。
// TestRetriever_EmptyQuery verifies an empty query errors out without calling the embed.
func TestRetriever_EmptyQuery(t *testing.T) {
	r := &Retriever{params: testParams(t, nil)}
	if _, err := r.Retrieve(context.Background(), "   "); err == nil {
		t.Errorf("Retrieve(blank query) error = nil, want error")
	}
}
