package knowledge

import (
	// 标准库
	"context"
	"strconv"
	"strings"
	"unicode/utf8"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// expandDocuments 对最终检索结果做 small-to-big 展开（父块 = 整篇文档）：命中分块所属文档整篇
// 拼回后替换该文档首条命中的内容并标记 Complete，同文档的其余命中删除，从而在保持检索粒度
// 的同时让消费方拿到完整规格（分块内容在工具与评估侧会被截断，整篇展开则不再截断）。
// 展开受 retrieval.expandDocLimit 限制（整篇 rune 数超限的文档保持原分块结果）；取数失败仅告警，
// 与使用统计回写同策略：增强逻辑失败绝不中断检索。
func (r *Retriever) expandDocuments(ctx context.Context, results []*retrieval.RetrievalResult) []*retrieval.RetrievalResult {
	if len(results) == 0 {
		return results
	}
	limit := r.params.Int("retrieval.expandDocLimit")
	if limit <= 0 {
		return results
	}
	docIDs := resultDocumentIDs(results)
	if len(docIDs) == 0 {
		return results
	}
	fullTexts, err := r.loadDocFullTexts(ctx, docIDs)
	if err != nil {
		r.warnf(ctx, "retriever: load documents for small-to-big expansion failed: %v", err)
		return results
	}
	return mergeExpandedResults(results, fullTexts, limit)
}

// resultDocumentIDs 提取检索结果中的去重文档 ID（保持首次出现顺序），非数字 ID 直接跳过。
func resultDocumentIDs(results []*retrieval.RetrievalResult) []uint {
	ids := make([]uint, 0, len(results))
	seen := make(map[uint]struct{}, len(results))
	for _, res := range results {
		if res == nil {
			continue
		}
		id, err := strconv.ParseUint(res.DocumentID, 10, 64)
		if err != nil || id == 0 {
			continue
		}
		if _, dup := seen[uint(id)]; dup {
			continue
		}
		seen[uint(id)] = struct{}{}
		ids = append(ids, uint(id))
	}
	return ids
}

// loadDocFullTexts 读取给定文档的整篇检索文本：按分块序号升序拼接「标题路径 + 正文」，
// 与单条命中返回的文本形态一致（见 rowsToResults）。仅读取未软删除的文档；无分块的文档不出现。
func (r *Retriever) loadDocFullTexts(ctx context.Context, docIDs []uint) (map[uint]string, error) {
	var alive []uint
	// KnowledgeDocument 使用 gorm.Model（含 DeletedAt），软删除文档由 GORM 默认作用域自动排除
	if err := r.db.WithContext(ctx).Model(&ai.KnowledgeDocument{}).
		Where("id IN ?", docIDs).
		Pluck("id", &alive).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	if len(alive) == 0 {
		return nil, nil
	}
	var chunks []ai.KnowledgeChunk
	if err := r.db.WithContext(ctx).
		Select("document_id", "chunk_index", "heading_path", "content").
		Where("document_id IN ?", alive).
		Order("document_id, chunk_index").
		Find(&chunks).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	full := make(map[uint]string, len(alive))
	for _, chunk := range chunks {
		full[chunk.DocumentID] += strings.TrimSpace(string(chunk.HeadingPath.Value())+"\n"+string(chunk.Content.Value())) + "\n\n"
	}
	for id, text := range full {
		full[id] = strings.TrimSpace(text)
	}
	return full, nil
}

// mergeExpandedResults 以整篇文档文本合并检索结果（纯函数，便于单测）：按原顺序遍历，同文档
// 仅保留首条命中，其内容替换为整篇全文并标记 Complete，同文档其余命中删除；整篇展开的总量受
// limit 约束——单篇超限、或展开后累计超过 limit 的文档一律降级保持原分块结果（绝不截断整篇），
// 使一次检索注入模型的整篇内容有确定上界（limit 是「一次检索的展开预算」而非「单篇上限」）；
// fullTexts 未覆盖的文档原样保留。
func mergeExpandedResults(results []*retrieval.RetrievalResult, fullTexts map[uint]string, limit int) []*retrieval.RetrievalResult {
	if len(results) == 0 || len(fullTexts) == 0 || limit <= 0 {
		return results
	}
	out := make([]*retrieval.RetrievalResult, 0, len(results))
	emitted := make(map[uint]bool, len(fullTexts))
	expanded := 0
	for _, res := range results {
		if res == nil {
			continue
		}
		id, err := strconv.ParseUint(res.DocumentID, 10, 64)
		full, ok := fullTexts[uint(id)]
		if err != nil || !ok {
			out = append(out, res)
			continue
		}
		// 该文档已经展开过：同文档其余命中直接丢弃（内容已由整篇覆盖），不可再作为分块重复返回
		if emitted[uint(id)] {
			continue
		}
		size := utf8.RuneCountInString(full)
		if size > limit || expanded+size > limit {
			out = append(out, res)
			continue
		}
		emitted[uint(id)] = true
		expanded += size
		clone := *res
		clone.Content = full
		clone.Complete = true
		out = append(out, &clone)
	}
	return out
}
