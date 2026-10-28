package knowledge

import (
	// 标准库
	"context"

	// 第三方
	"github.com/cloudwego/eino/components/retriever"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	"zhongkuisec.com/zhongkui/internal/ai/tools"
	"zhongkuisec.com/zhongkui/internal/pkg/docconv"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// Searcher 是 tools.KnowledgeSearcher 的检索侧实现：持有检索器与本次运行的命名空间/文档限定，
// 把模型入参 query 转成一次 Retriever.Retrieve 调用，并把命中同时记入采集器与转换为
// tools.KnowledgeHit 返回。
type Searcher struct {
	// Retriever 知识检索器
	Retriever retriever.Retriever
	// NamespaceName 本次运行限定的命名空间
	NamespaceName string
	// DocIDs 本次运行限定的文档 ID 列表（为空表示不限定）
	DocIDs []uint
	// Collector 命中采集器（可空）
	Collector *Collector
}

// Search 按 query 与可选分区对当前命名空间的知识库执行一次混合检索：命名空间与文档限定由
// Searcher 持有，模型只能选择分区（format）；检索失败以 errors.WithStack 包装返回（英文），
// 成功时把命中记入采集器并转换为 tools.KnowledgeHit 列表返回。
func (s *Searcher) Search(ctx context.Context, query string, format docconv.Format) ([]tools.KnowledgeHit, error) {
	opts := []retriever.Option{WithNamespace(s.NamespaceName), WithDocIDs(s.DocIDs)}
	if format != "" {
		opts = append(opts, WithFormat(format))
	}
	docs, err := s.Retriever.Retrieve(ctx, query, opts...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	results := retrieval.ToRetrievalResults(docs)
	numbers := s.Collector.Add(results)
	hits := make([]tools.KnowledgeHit, 0, len(results))
	for i, r := range results {
		citation := 0
		if i < len(numbers) {
			citation = numbers[i]
		}
		hits = append(hits, tools.KnowledgeHit{
			DocumentID: r.DocumentID,
			Title:      r.Title,
			ChunkIndex: r.ChunkIndex,
			Content:    r.Content,
			Score:      r.Score,
			Complete:   r.Complete,
			Citation:   citation,
		})
	}
	return hits, nil
}
