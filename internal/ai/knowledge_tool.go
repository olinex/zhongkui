package ai

import (
	// 标准库
	"context"
	"sync"

	// 第三方
	"github.com/cloudwego/eino/components/retriever"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/tools"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// knowledgeCollector 是单次运行内的知识检索采集器：并发安全地累积 knowledge_search 工具每次
// 检索命中的 RetrievalResult，供节点 4 在流末尾取回并写入 AgentOutput.Citations。工具在
// generate 的 goroutine 内执行、节点 4 随后读取，故以互斥锁保护内部切片。
type knowledgeCollector struct {
	// mu 保护 results 的并发读写
	mu sync.Mutex
	// results 已累积的检索命中
	results []RetrievalResult
}

// add 追加一批检索命中到采集器（并发安全）；接收者为 nil 时静默返回。
func (c *knowledgeCollector) add(results []RetrievalResult) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.results = append(c.results, results...)
	c.mu.Unlock()
}

// snapshot 返回当前已累积命中的副本（并发安全）；接收者为 nil 或无命中时返回 nil。
func (c *knowledgeCollector) snapshot() []RetrievalResult {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.results) == 0 {
		return nil
	}
	out := make([]RetrievalResult, len(c.results))
	copy(out, c.results)
	return out
}

// knowledgeSearcherAdapter 是 tools.KnowledgeSearcher 在 internal/ai 侧的适配器：持有检索器与
// 本次运行的命名空间/文档限定，把模型入参 query 转成一次 Retriever.Retrieve 调用，并把命中同时
// 记入采集器与转换为 tools.KnowledgeHit 返回。
type knowledgeSearcherAdapter struct {
	// retriever 知识检索器
	retriever retriever.Retriever
	// namespaceName 本次运行限定的命名空间
	namespaceName string
	// docIDs 本次运行限定的文档 ID 列表（为空表示不限定）
	docIDs []uint
	// collector 命中采集器（可空）
	collector *knowledgeCollector
}

// Search 按 query 对当前命名空间的知识库执行一次混合检索：命名空间与文档限定由适配器持有，
// 模型入参不参与；检索失败以 errors.WithStack 包装返回（英文），成功时把命中记入采集器并转换为
// tools.KnowledgeHit 列表返回。
func (a *knowledgeSearcherAdapter) Search(ctx context.Context, query string) ([]tools.KnowledgeHit, error) {
	docs, err := a.retriever.Retrieve(ctx, query, WithNamespace(a.namespaceName), WithDocIDs(a.docIDs))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	results := ToRetrievalResults(docs)
	a.collector.add(results)
	hits := make([]tools.KnowledgeHit, 0, len(results))
	for _, r := range results {
		hits = append(hits, tools.KnowledgeHit{
			DocumentID: r.DocumentID,
			Title:      r.Title,
			ChunkIndex: r.ChunkIndex,
			Content:    r.Content,
			Score:      r.Score,
			Complete:   r.Complete,
		})
	}
	return hits, nil
}

// knowledgeToolContext 在派发工具前把知识检索运行期信息（检索器适配器与单条命中截断长度）
// 注入 ctx，供 knowledge_search 在工具循环中取回。
func (a *Agent) knowledgeToolContext(ctx context.Context, in AgentInput, collector *knowledgeCollector) context.Context {
	return tools.WithKnowledgeRuntime(ctx, tools.KnowledgeRuntime{
		Searcher: &knowledgeSearcherAdapter{
			retriever:     a.retriever,
			namespaceName: in.NamespaceName,
			docIDs:        in.DocIDs,
			collector:     collector,
		},
		SnippetLimit: a.params.Int("agent.retrievedDocSnippetLimit"),
	})
}
