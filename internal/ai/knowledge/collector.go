package knowledge

import (
	// 标准库
	"sync"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
)

// Collector 是单次运行内的知识检索采集器：并发安全地累积 knowledge_search 工具每次检索命中的
// retrieval.RetrievalResult，供 Agent 在流末尾取回并写入引用来源。工具在生成 goroutine 内执行、
// 收尾阶段随后读取，故以互斥锁保护内部切片。
// 采集器同时为每次命中去重并分配本次运行内稳定的 1 起来源编号：同一分块（命名空间 + 文档 +
// 分块序号）在多次检索中只占一个编号，正文中的 [n] 标注因而不受后续检索追加影响。
type Collector struct {
	// mu 保护 results 与 numbers 的并发读写
	mu sync.Mutex
	// results 已累积的检索命中（已去重），下标 + 1 即来源编号
	results []retrieval.RetrievalResult
	// numbers 分块键 → 来源编号（1 起）
	numbers map[retrievalKey]int
}

// retrievalKey 是采集器内识别同一分块的键：同一文档 ID 可能在租户与系统命名空间同时存在，
// 故命名空间必须参与区分。
type retrievalKey struct {
	// namespaceName 来源文档所属命名空间
	namespaceName string
	// documentID 来源文档 ID
	documentID string
	// chunkIndex 分块在文档内的序号
	chunkIndex int
}

// NewCollector 构造采集器，并把既有的检索命中（续跑时来自运行现场）预置进去；
// 预置同样经 Add 去重并建立编号索引，使续跑后新命中的编号与已有引用连续。
func NewCollector(initial []retrieval.RetrievalResult) *Collector {
	c := &Collector{numbers: make(map[retrievalKey]int)}
	c.Add(initial)
	return c
}

// Add 追加一批检索命中到采集器（并发安全），返回与入参等长的来源编号（1 起）：已在采集器内
// 的同一分块复用原编号且不重复入库。接收者为 nil 时静默返回 nil。
func (c *Collector) Add(results []retrieval.RetrievalResult) []int {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.numbers == nil {
		c.numbers = make(map[retrievalKey]int)
	}
	numbers := make([]int, len(results))
	for i, r := range results {
		key := retrievalKey{
			namespaceName: r.NamespaceName,
			documentID:    r.DocumentID,
			chunkIndex:    r.ChunkIndex,
		}
		if n, ok := c.numbers[key]; ok {
			numbers[i] = n
			continue
		}
		c.results = append(c.results, r)
		n := len(c.results)
		c.numbers[key] = n
		numbers[i] = n
	}
	return numbers
}

// Snapshot 返回当前已累积命中的副本（并发安全）；接收者为 nil 或无命中时返回 nil。
func (c *Collector) Snapshot() []retrieval.RetrievalResult {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.results) == 0 {
		return nil
	}
	out := make([]retrieval.RetrievalResult, len(c.results))
	copy(out, c.results)
	return out
}
