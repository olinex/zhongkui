// 本文件覆盖 knowledge_search 适配器的分区过滤：模型选择的 format 必须原样落入检索选项，
// 未指定分区时不得注入 Format；命名空间与文档限定始终由运行上下文持有。

package knowledge

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/pkg/docconv"
)

// stubRetriever 记录查询与选项并返回预置文档，用于验证 Searcher 注入的检索选项。
type stubRetriever struct {
	// docs 预置的检索结果。
	docs []*schema.Document
	// query 最近一次检索的查询文本。
	query string
	// opts 最近一次检索的选项。
	opts []retriever.Option
}

// Retrieve 记录查询与选项并返回预置文档。
func (s *stubRetriever) Retrieve(_ context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	s.query = query
	s.opts = opts
	return s.docs, nil
}

// TestKnowledgeSearcherAdapter_FormatOption 校验适配器把 format 转为 WithFormat 选项，且保留
// 命名空间与文档限定；format 为空时不设置分区。
func TestKnowledgeSearcherAdapter_FormatOption(t *testing.T) {
	cases := []struct {
		name   string
		format docconv.Format
		want   docconv.Format
	}{
		{"no format", "", ""},
		{"native api", docconv.FormatNativeApi, docconv.FormatNativeApi},
		{"schema", docconv.FormatSchema, docconv.FormatSchema},
		{"capability", docconv.FormatCapability, docconv.FormatCapability},
		{"generated", docconv.FormatGenerated, docconv.FormatGenerated},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rtv := &stubRetriever{}
			adapter := &Searcher{
				Retriever:     rtv,
				NamespaceName: "tenant-a",
				DocIDs:        []uint{7, 9},
			}
			if _, err := adapter.Search(context.Background(), "query", c.format); err != nil {
				t.Fatalf("Search error = %v", err)
			}
			if rtv.query != "query" {
				t.Errorf("retriever query = %q, want query", rtv.query)
			}
			opts := retriever.GetImplSpecificOptions(&RetrieveOptions{}, rtv.opts...)
			if opts.Format != c.want {
				t.Errorf("retriever format = %q, want %q", opts.Format, c.want)
			}
			if opts.NamespaceName != "tenant-a" {
				t.Errorf("retriever namespace = %q, want tenant-a", opts.NamespaceName)
			}
			if len(opts.DocIDs) != 2 || opts.DocIDs[0] != 7 || opts.DocIDs[1] != 9 {
				t.Errorf("retriever docIDs = %v, want [7 9]", opts.DocIDs)
			}
		})
	}
}
