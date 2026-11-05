package knowledge

import (
	// 标准库
	"context"
	"strconv"

	// 第三方
	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// TouchUsage 递增给定知识文档的 use_count 与 last_used_at；ids 为空直接返回 nil。
// 语义与长期记忆的 MemoryStore.TouchUsage 一致，供检索命中后回写使用统计。
func (r *Retriever) TouchUsage(ctx context.Context, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&ai.KnowledgeDocument{}).
		Where("id IN ?", ids).
		Updates(map[string]any{
			"use_count":    gorm.Expr("use_count + 1"),
			"last_used_at": gorm.Expr("NOW()"),
		}).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// touchUsageAsync 以命中文档 ID 异步回写使用统计（每篇命中文档一次自增）：无 ID 时直接返回，
// 写入失败仅告警、绝不中断或影响检索结果。长期记忆与知识库采用同一策略：使用统计是增强信息，
// 不能因它失败而让生成/检索失败。
func (r *Retriever) touchUsageAsync(ctx context.Context, docs []*schema.Document) {
	ids := documentIDsOf(docs)
	if len(ids) == 0 {
		return
	}
	go func() {
		if err := r.TouchUsage(ctx, ids); err != nil {
			r.warnf(ctx, "retriever: touch document usage failed: %v", err)
		}
	}()
}

// documentIDsOf 从检索结果中提取去重后的文档 ID：MetaData 的 documentID 为字符串形式，
// 解析失败或缺失的条目直接跳过，保证非数字 ID 不会污染统计。
func documentIDsOf(docs []*schema.Document) []uint {
	ids := make([]uint, 0, len(docs))
	seen := make(map[uint]struct{}, len(docs))
	for _, doc := range docs {
		if doc == nil || doc.MetaData == nil {
			continue
		}
		raw, ok := doc.MetaData["documentID"].(string)
		if !ok {
			continue
		}
		id, err := strconv.ParseUint(raw, 10, 64)
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

// warnf 在 logger 非空时输出告警。
func (r *Retriever) warnf(ctx context.Context, format string, args ...any) {
	if r.logger == nil {
		return
	}
	r.logger.Ctx(ctx).Warnf(format, args...)
}
