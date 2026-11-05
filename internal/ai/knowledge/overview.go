package knowledge

import (
	// 标准库
	"context"
	"strings"
	"time"
	"unicode/utf8"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/config"
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
)

// chunkOverviewRow 是分块明细的读取行：内嵌 KnowledgeChunk 以复用其字段的 crypto
// 序列化器标签（否则按原始密文字节扫入 string，得到的是密文而非明文），
// 查询仅 Select 明细所需列，避免载入 embedding 大字段。
type chunkOverviewRow struct {
	ai.KnowledgeChunk
}

// ChunkOverviewSummary 是文档分块构建摘要：读时从知识文档与其分块聚合派生，不持久化。
type ChunkOverviewSummary struct {
	// ChunkCount 分块数（实际读取到的分块行数）
	ChunkCount int
	// ContentChars 文档正文字符数
	ContentChars int
	// ChunkCharsTotal 各分块正文字符数之和（含 overlap 造成的重复）
	ChunkCharsTotal int
	// ChunkCharsAvg 分块正文字符数均值（整数除法；无分块时为 0）
	ChunkCharsAvg int
	// ChunkCharsMin 分块正文字符数最小值（无分块时为 0）
	ChunkCharsMin int
	// ChunkCharsMax 分块正文字符数最大值（无分块时为 0）
	ChunkCharsMax int
	// HeadingPathCount 去重后的标题路径数（空路径不计入）
	HeadingPathCount int
	// AtomicChunkCount 含代码块或表格的分块数（超长也不切分的原子块）
	AtomicChunkCount int
	// ChunkSize 生效参数 chunking.size（params 为 nil 时为 0）
	ChunkSize int
	// ChunkOverlap 生效参数 chunking.overlap（params 为 nil 时为 0）
	ChunkOverlap int
	// Indexed 是否已建立索引（存在分块且内容摘要非空）
	Indexed bool
	// Stale 索引摘要与当前正文不一致（索引落后于正文）
	Stale bool
	// IndexedAt 最近一次索引写入时间（文档更新时间）
	IndexedAt time.Time
	// Hash 文档内容的 HMAC-SHA256 十六进制摘要
	Hash string
}

// ChunkOverviewItem 是单个分块的构建明细。
type ChunkOverviewItem struct {
	// Index 分块在文档内的序号
	Index int
	// HeadingPath 所属标题路径（空表示该块不在任何标题之下）
	HeadingPath string
	// Content 分块正文
	Content string
	// Chars 正文字符数
	Chars int
	// Lines 正文行数
	Lines int
	// Kinds 识别出的结构单元类型（取值为 chunkUnitKind 枚举类的值，按声明顺序）
	Kinds []llm.ChunkUnitKind
}

// ChunkOverview 是文档分块构建明细的聚合结果。
type ChunkOverview struct {
	// Summary 文档级索引摘要
	Summary ChunkOverviewSummary
	// Chunks 逐块明细（按 chunk_index 升序；无分块时为空切片而非 nil）
	Chunks []ChunkOverviewItem
}

// BuildChunkOverview 读取文档分块并聚合构建摘要：按 chunk_index 升序读取分块（仅选取
// chunk_index/heading_path/content，不载入 embedding），逐块统计字符数/行数/结构单元类型，
// 并派生标题路径数、原子块数、索引状态与陈旧标记。本函数为包级函数（不挂 Knowledge 方法），
// 使 AI 能力未启用时也能读取明细（此时文档通常未索引，返回空分块 + Indexed=false）。
// params 为 nil 时切片长度与重叠长度记 0；db 或 doc 为 nil 时返回英文错误。
func BuildChunkOverview(ctx context.Context, db *database.DB, params *config.Params, doc *ai.KnowledgeDocument) (*ChunkOverview, error) {
	if db == nil {
		return nil, errors.New("knowledge: nil database")
	}
	if doc == nil {
		return nil, errors.New("knowledge: nil document")
	}
	var rows []chunkOverviewRow
	if err := db.WithContext(ctx).
		Table("knowledge_chunks").
		Select("chunk_index, heading_path, content").
		Where("document_id = ? AND namespace_name = ?", doc.ID, doc.NamespaceName).
		Order("chunk_index ASC").
		Scan(&rows).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	plaintext := string(doc.Content.Value())
	summary := ChunkOverviewSummary{
		ChunkCount:   len(rows),
		ContentChars: utf8.RuneCountInString(plaintext),
		IndexedAt:    doc.UpdatedAt,
		Hash:         doc.Hash.Digest(),
	}
	if params != nil {
		summary.ChunkSize = params.Int("chunking.size")
		summary.ChunkOverlap = params.Int("chunking.overlap")
	}
	chunks := make([]ChunkOverviewItem, 0, len(rows))
	paths := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		content := string(row.Content.Value())
		headingPath := string(row.HeadingPath.Value())
		chars := utf8.RuneCountInString(content)
		kinds := llm.DescribeChunk(content)
		for _, kind := range kinds {
			// 代码块与表格为原子单元（超长也不切分），单独计数供运维判断是否出现超长块
			if kind == llm.ChunkUnitCode || kind == llm.ChunkUnitTable {
				summary.AtomicChunkCount++
				break
			}
		}
		if headingPath != "" {
			paths[headingPath] = struct{}{}
		}
		summary.ChunkCharsTotal += chars
		if len(chunks) == 0 || chars < summary.ChunkCharsMin {
			summary.ChunkCharsMin = chars
		}
		if chars > summary.ChunkCharsMax {
			summary.ChunkCharsMax = chars
		}
		chunks = append(chunks, ChunkOverviewItem{
			Index:       row.ChunkIndex,
			HeadingPath: headingPath,
			Content:     content,
			Chars:       chars,
			Lines:       strings.Count(content, "\n") + 1,
			Kinds:       kinds,
		})
	}
	if len(chunks) > 0 {
		summary.ChunkCharsAvg = summary.ChunkCharsTotal / len(chunks)
	}
	summary.HeadingPathCount = len(paths)
	summary.Indexed = summary.ChunkCount > 0 && doc.Hash.Digest() != ""
	if summary.Indexed {
		// 索引失败时 IndexDocument 的整事务回滚，hash 会落后于当前正文：此即索引陈旧信号
		current, err := crypto.DigestForField(&ai.KnowledgeDocument{}, "hash", "json", []byte(plaintext))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		summary.Stale = current != doc.Hash.Digest()
	}
	return &ChunkOverview{Summary: summary, Chunks: chunks}, nil
}
