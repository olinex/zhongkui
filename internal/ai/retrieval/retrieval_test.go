package retrieval

import (
	// 标准库
	"math"
	"strings"
	"testing"
)

// almostEqual 报告两个浮点数是否在容差内近似相等。
func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// TestFuseRRF 测试 RRF 融合：两路命中按 sum(1/(k+rank)) 打分，序与分符合预期。
func TestFuseRRF(t *testing.T) {
	vector := []*RetrievalResult{
		{DocumentID: "1", ChunkIndex: 0, Title: "A", Score: 0.9},
		{DocumentID: "2", ChunkIndex: 0, Title: "B", Score: 0.3},
	}
	keyword := []*RetrievalResult{
		{DocumentID: "2", ChunkIndex: 0, Title: "B", Score: 0.4},
		{DocumentID: "3", ChunkIndex: 0, Title: "C", Score: 0.6},
	}
	got := FuseRRF(vector, keyword)
	if len(got) != 3 {
		t.Fatalf("FuseRRF() returned %d hits, want 3", len(got))
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
func TestFuseRRF_SinglePath(t *testing.T) {
	keyword := []*RetrievalResult{
		{DocumentID: "1", ChunkIndex: 0, Score: 0.1},
		{DocumentID: "2", ChunkIndex: 0, Score: 0.9},
	}
	got := FuseRRF(nil, keyword)
	if len(got) != 2 {
		t.Fatalf("FuseRRF() returned %d hits, want 2", len(got))
	}
	if got[0].DocumentID != "1" || got[1].DocumentID != "2" {
		t.Errorf("FuseRRF() order = [%s, %s], want [1, 2]", got[0].DocumentID, got[1].DocumentID)
	}
}

// TestFuseWeighted 测试加权融合：每路 min-max 归一化后按权重线性叠加。
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
func TestSelectTopK(t *testing.T) {
	cands := []*RetrievalResult{{DocumentID: "1"}, {DocumentID: "2"}, {DocumentID: "3"}}
	if got := SelectTopK(cands, 0); len(got) != 3 {
		t.Errorf("SelectTopK(cands, 0) = %d hits, want 3", len(got))
	}
	if got := SelectTopK(cands, 10); len(got) != 3 {
		t.Errorf("SelectTopK(cands, 10) = %d hits, want 3", len(got))
	}
	got := SelectTopK(cands, 2)
	if len(got) != 2 || got[0].DocumentID != "1" || got[1].DocumentID != "2" {
		t.Errorf("SelectTopK(cands, 2) = %v, want the first two hits", got)
	}
}

// TestDedupeSimilarResults 测试内容去重：内容实质相同的后位命中被丢弃并保留排序更靠前的
// 一条，部分重合（相似度低于阈值）与空正文的命中全部保留，不足两条的输入原样返回。
func TestDedupeSimilarResults(t *testing.T) {
	// 逐字重复的长正文：真实场景即自重复文档被切成相邻块后两块内容几乎一致
	repeated := strings.Repeat("这是一个无聊的故事，通过无限递归的方式讲一个故事重复下去。", 30)
	// 部分重合：共享一段前言、各自接不同的独有正文，相似度约四成
	shared := "本段说明检索命中后的处理流程：先融合排序，再按候选截断，最后交给重排模型判定。"
	cases := []struct {
		name    string
		hits    []*RetrievalResult
		wantIDs []string
	}{
		{
			name: "相同正文只留排序更靠前的一条",
			hits: []*RetrievalResult{
				{DocumentID: "1", ChunkIndex: 0, Content: repeated},
				{DocumentID: "1", ChunkIndex: 1, Content: repeated},
				{DocumentID: "2", ChunkIndex: 0, Content: "另一篇文档的正文，与上面无关。"},
			},
			wantIDs: []string{"1", "2"},
		},
		{
			name: "部分重合低于阈值时全部保留",
			hits: []*RetrievalResult{
				{DocumentID: "3", ChunkIndex: 0, Content: shared + "独有正文甲：向量路的召回与分数归一化。"},
				{DocumentID: "4", ChunkIndex: 0, Content: shared + "独有正文乙：关键词路的分词与命中统计。"},
			},
			wantIDs: []string{"3", "4"},
		},
		{
			name: "空正文不互相判重",
			hits: []*RetrievalResult{
				{DocumentID: "5", ChunkIndex: 0, Content: ""},
				{DocumentID: "6", ChunkIndex: 0, Content: ""},
			},
			wantIDs: []string{"5", "6"},
		},
		{
			name:    "不足两条原样返回",
			hits:    []*RetrievalResult{{DocumentID: "7", ChunkIndex: 0, Content: repeated}},
			wantIDs: []string{"7"},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := DedupeSimilarResults(tt.hits)
			gotIDs := make([]string, 0, len(got))
			for _, hit := range got {
				gotIDs = append(gotIDs, hit.DocumentID)
			}
			if strings.Join(gotIDs, ",") != strings.Join(tt.wantIDs, ",") {
				t.Errorf("kept IDs = %v, want %v", gotIDs, tt.wantIDs)
			}
		})
	}
}

// TestDedupeSimilarResults_RepeatedDocument 测试自重复文档的相邻分块被判为重复：同一段
// 无限递归的笑话被顺序切成两块后，两块内容几乎相同（并非逐字节相同），后者应被丢弃。
func TestDedupeSimilarResults_RepeatedDocument(t *testing.T) {
	sentence := "这是一个无聊的故事，通过无限递归的方式讲一个故事重复下去，用来逗弄小孩子的。"
	runes := []rune(strings.Repeat(sentence, 60))
	got := DedupeSimilarResults([]*RetrievalResult{
		{DocumentID: "1", ChunkIndex: 0, Content: string(runes[:800])},
		{DocumentID: "1", ChunkIndex: 1, Content: string(runes[800:1600])},
	})
	if len(got) != 1 {
		t.Fatalf("kept %d hits, want 1", len(got))
	}
	if got[0].ChunkIndex != 0 {
		t.Errorf("kept chunkIndex = %d, want 0 (the earlier hit wins)", got[0].ChunkIndex)
	}
}

// TestDedupeSimilarResults_KeepsSeamOverlap 测试分片接缝处的重叠复读不被判为重复：chunker
// 弥合 size 截断接缝时会把这前一块尾部 overlap 个字符复读到下一块开头（默认 100 对
// size=800，约占一成），相似度远低于阈值，两块都必须保留，否则会丢掉后一块的独有正文。
func TestDedupeSimilarResults_KeepsSeamOverlap(t *testing.T) {
	prevRunes := []rune(strings.Repeat("甲乙丙丁戊己庚辛壬癸", 80))
	prev := string(prevRunes)
	next := string(prevRunes[len(prevRunes)-100:]) + "\n" + strings.Repeat("子丑寅卯辰巳午未申酉", 80)
	if sim := jaccardSimilarityWithCutoff(contentShingles(prev), contentShingles(next), dedupeSimilarityCutoff); sim >= dedupeSimilarityCutoff {
		t.Fatalf("seam similarity = %v, want below %v", sim, dedupeSimilarityCutoff)
	}
	got := DedupeSimilarResults([]*RetrievalResult{
		{DocumentID: "1", ChunkIndex: 0, Content: prev},
		{DocumentID: "1", ChunkIndex: 1, Content: next},
	})
	if len(got) != 2 {
		t.Fatalf("seam-overlapped chunks kept %d hits, want 2", len(got))
	}
}

// TestVectorLiteral 测试向量序列化为 pgvector 字面量。
func TestVectorLiteral(t *testing.T) {
	if got := VectorLiteral([]float64{0.1, 0.25, 1}); got != "[0.1,0.25,1]" {
		t.Errorf("VectorLiteral() = %q, want %q", got, "[0.1,0.25,1]")
	}
	if got := VectorLiteral(nil); got != "[]" {
		t.Errorf("VectorLiteral(nil) = %q, want %q", got, "[]")
	}
}

// TestToFloat32 测试 float64 向量转 float32 存储类型。
func TestToFloat32(t *testing.T) {
	got := ToFloat32([]float64{0.1, 0.5, -2})
	want := []float32{0.1, 0.5, -2}
	if len(got) != len(want) {
		t.Fatalf("ToFloat32() length = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("ToFloat32()[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestTruncateText 测试文本截断：短文本原样返回，长文本保留前 max 字符并追加省略号。
func TestTruncateText(t *testing.T) {
	if got := TruncateText("short", 100); got != "short" {
		t.Errorf("TruncateText(\"short\", 100) = %q, want %q", got, "short")
	}
	if got := TruncateText("hello world", 5); got != "hello..." {
		t.Errorf("TruncateText(\"hello world\", 5) = %q, want %q", got, "hello...")
	}
	// 多字节安全：按字符截断而非字节
	if got := TruncateText("你好世界", 2); got != "你好..." {
		t.Errorf("TruncateText(\"你好世界\", 2) = %q, want %q", got, "你好...")
	}
}

// TestSortedResults_Deterministic 测试同分条目按 docID/chunkIndex 稳定排序。
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

// TestJaccardSimilarityWithCutoff 测试带阈值的 Jaccard 相似度：阈值之上返回真实相似度，阈值
// 之下被上限剪枝归零，阈值为 0 时不做剪枝。
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
func TestDedupeSimilarResultsWithCutoff(t *testing.T) {
	hits := func() []*RetrievalResult {
		return []*RetrievalResult{
			{DocumentID: "1", Content: "一二三四五六七八九十"},     // 7 个 4-gram
			{DocumentID: "2", Content: "一二三四五六七八九十甲乙丙丁"}, // 相似度 ≈ 0.636
		}
	}
	// 阈值 0.5 < 0.636：后位被判重丢弃
	if got := DedupeSimilarResultsWithCutoff(hits(), 0.5); len(got) != 1 || got[0].DocumentID != "1" {
		t.Errorf("cutoff 0.5 kept %d hits (%v), want only the earlier one", len(got), docIDs(got))
	}
	// 阈值 0.7 > 0.636：两条都保留
	if got := DedupeSimilarResultsWithCutoff(hits(), 0.7); len(got) != 2 {
		t.Errorf("cutoff 0.7 kept %d hits, want 2", len(got))
	}
	// 与既有默认阈值入口一致，保证不回归
	legacy := docIDs(DedupeSimilarResults(hits()))
	withCutoff := docIDs(DedupeSimilarResultsWithCutoff(hits(), dedupeSimilarityCutoff))
	if strings.Join(legacy, ",") != strings.Join(withCutoff, ",") {
		t.Errorf("dedupeSimilarResults = %v, DedupeSimilarResultsWithCutoff(default) = %v", legacy, withCutoff)
	}
}

// docIDs 提取检索结果的文档 ID 序列。
func docIDs(hits []*RetrievalResult) []string {
	out := make([]string, 0, len(hits))
	for _, hit := range hits {
		out = append(out, hit.DocumentID)
	}
	return out
}
