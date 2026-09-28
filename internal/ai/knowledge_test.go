package ai

import (
	// 标准库
	"context"
	"math"
	"strings"
	"testing"

	// 第三方
	"github.com/cloudwego/eino/components/retriever"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/llm"
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
			got := dedupeSimilarResults(tt.hits)
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
	got := dedupeSimilarResults([]*RetrievalResult{
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
	if sim := jaccardSimilarity(contentShingles(prev), contentShingles(next)); sim >= dedupeSimilarityCutoff {
		t.Fatalf("seam similarity = %v, want below %v", sim, dedupeSimilarityCutoff)
	}
	got := dedupeSimilarResults([]*RetrievalResult{
		{DocumentID: "1", ChunkIndex: 0, Content: prev},
		{DocumentID: "1", ChunkIndex: 1, Content: next},
	})
	if len(got) != 2 {
		t.Fatalf("seam-overlapped chunks kept %d hits, want 2", len(got))
	}
}

// TestParseRerankIndices 测试解析模型返回的序号数组（含浮点数组与 ```json 围栏包裹）。
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
func TestParseRerankIndices_Invalid(t *testing.T) {
	for _, in := range []string{"", "not json", "{\"a\":1}"} {
		if _, err := parseRerankIndices(in); err == nil {
			t.Errorf("parseRerankIndices(%q) error = nil, want error", in)
		}
	}
}

// TestVectorLiteral 测试向量序列化为 pgvector 字面量。
func TestVectorLiteral(t *testing.T) {
	if got := vectorLiteral([]float64{0.1, 0.25, 1}); got != "[0.1,0.25,1]" {
		t.Errorf("vectorLiteral() = %q, want %q", got, "[0.1,0.25,1]")
	}
	if got := vectorLiteral(nil); got != "[]" {
		t.Errorf("vectorLiteral(nil) = %q, want %q", got, "[]")
	}
}

// TestToFloat32 测试 float64 向量转 float32 存储类型。
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
func TestSha256Hex(t *testing.T) {
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got := sha256Hex("abc"); got != want {
		t.Errorf("sha256Hex(\"abc\") = %q, want %q", got, want)
	}
}

// TestTruncateText 测试文本截断：短文本原样返回，长文本保留前 max 字符并追加省略号。
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
	if _, err := NewRetriever(nil, embed, nil, params, nil); err == nil || !strings.Contains(err.Error(), "nil database") {
		t.Errorf("NewRetriever(nil db) error = %v, want nil-database error", err)
	}
}

// TestRetriever_EmptyQuery 测试空查询直接报错（不调用 embed）。
func TestRetriever_EmptyQuery(t *testing.T) {
	r := &Retriever{params: testParams(t, nil)}
	if _, err := r.Retrieve(context.Background(), "   "); err == nil {
		t.Errorf("Retrieve(blank query) error = nil, want error")
	}
}
