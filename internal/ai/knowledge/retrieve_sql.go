package knowledge

import (
	// 标准库
	"context"

	// 第三方
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// vectorRetrieve 执行向量路检索：按余弦距离（embedding <=> ?）升序取 candidates 条。
func (r *Retriever) vectorRetrieve(ctx context.Context, o *RetrieveOptions, vec []float64) ([]*retrieval.RetrievalResult, error) {
	if o.Candidates <= 0 || len(vec) == 0 {
		return nil, nil
	}
	lit := retrieval.VectorLiteral(vec)
	q := r.db.Table("knowledge_chunks AS c").
		Select("c.document_id, c.namespace_name, c.chunk_index, c.heading_path, c.content, d.title, (1 - (c.embedding <=> ?::vector)) AS score", lit).
		Joins("JOIN knowledge_documents AS d ON d.id = c.document_id AND d.deleted_at IS NULL").
		Where("c.namespace_name IN ?", base.NamespaceScope(o.NamespaceName))
	if len(o.DocIDs) > 0 {
		q = q.Where("c.document_id IN ?", o.DocIDs)
	}
	if o.Format != "" {
		q = q.Where("d.format = ?", string(o.Format))
	}
	q = q.Order(clause.OrderBy{Expression: gorm.Expr("c.embedding <=> ?::vector", lit)}).
		Limit(o.Candidates)
	var rows []struct {
		ai.KnowledgeChunk
		Title string
		Score float64
	}
	if err := q.Scan(&rows).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return retrieval.RowsToResults(rows), nil
}

// keywordRetrieve 执行关键词路检索：search @@ plainto_tsquery 过滤，
// 按 ts_rank 降序取 candidates 条。
func (r *Retriever) keywordRetrieve(ctx context.Context, o *RetrieveOptions, query string) ([]*retrieval.RetrievalResult, error) {
	if o.Candidates <= 0 {
		return nil, nil
	}
	q := r.db.Table("knowledge_chunks AS c").
		Select("c.document_id, c.namespace_name, c.chunk_index, c.heading_path, c.content, d.title, ts_rank(c.search, plainto_tsquery('simple', ?)) AS score", query).
		Joins("JOIN knowledge_documents AS d ON d.id = c.document_id AND d.deleted_at IS NULL").
		Where("c.namespace_name IN ?", base.NamespaceScope(o.NamespaceName)).
		Where("c.search @@ plainto_tsquery('simple', ?)", query)
	if len(o.DocIDs) > 0 {
		q = q.Where("c.document_id IN ?", o.DocIDs)
	}
	if o.Format != "" {
		q = q.Where("d.format = ?", string(o.Format))
	}
	q = q.Order(clause.OrderBy{Expression: gorm.Expr("ts_rank(c.search, plainto_tsquery('simple', ?)) DESC", query)}).
		Limit(o.Candidates)
	var rows []struct {
		ai.KnowledgeChunk
		Title string
		Score float64
	}
	if err := q.Scan(&rows).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return retrieval.RowsToResults(rows), nil
}
