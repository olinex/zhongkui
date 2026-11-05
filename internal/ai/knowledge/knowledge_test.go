package knowledge

import (
	// 标准库
	"context"
	"math"
	"strings"
	"testing"

	// 第三方
	"github.com/cloudwego/eino/components/retriever"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/aitest"
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	"zhongkuisec.com/zhongkui/internal/pkg/docconv"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
)

// almostEqual 报告两个浮点数是否在容差内近似相等。
func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
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

// TestToSchemaDocuments_RoundTrip 测试 RetrievalResult → schema.Document →
// ToRetrievalResults 往返不丢字段（含 small-to-big 的 Complete 标记）。
func TestToSchemaDocuments_RoundTrip(t *testing.T) {
	results := []*retrieval.RetrievalResult{
		{DocumentID: "7", Title: "T1", ChunkIndex: 2, Content: "text", Score: 0.5, Complete: true},
		{DocumentID: "8", Title: "T2", ChunkIndex: 0, Content: "other", Score: 0.1},
	}
	docs := toSchemaDocuments(results)
	if len(docs) != 2 {
		t.Fatalf("toSchemaDocuments() returned %d docs, want 2", len(docs))
	}
	back := retrieval.ToRetrievalResults(docs)
	for i, r := range back {
		w := *results[i]
		if r.DocumentID != w.DocumentID || r.Title != w.Title || r.ChunkIndex != w.ChunkIndex ||
			r.Content != w.Content || !almostEqual(r.Score, w.Score) || r.Complete != w.Complete {
			t.Errorf("round trip[%d] = %+v, want %+v", i, r, w)
		}
	}
}

// TestRetrieveOptions 测试实现专属选项经 retriever.Option 注入并合并到默认值。
func TestRetrieveOptions(t *testing.T) {
	o := retriever.GetImplSpecificOptions(&RetrieveOptions{TopK: 5}, WithNamespace("ns"),
		WithDocIDs([]uint{1, 2}), WithFormat(docconv.FormatNativeApi), WithRetrieveTopK(3), WithCandidates(9),
		WithFusion("weighted"), WithWeights(retrieval.FusionWeights{Vector: 1, Keyword: 0}))
	if o.NamespaceName != "ns" {
		t.Errorf("NamespaceName = %q, want %q", o.NamespaceName, "ns")
	}
	if o.Format != docconv.FormatNativeApi {
		t.Errorf("Format = %q, want %q", o.Format, docconv.FormatNativeApi)
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
	params := aitest.Params(t, nil)
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
	r := &Retriever{params: aitest.Params(t, nil)}
	if _, err := r.Retrieve(context.Background(), "   "); err == nil {
		t.Errorf("Retrieve(blank query) error = nil, want error")
	}
}
