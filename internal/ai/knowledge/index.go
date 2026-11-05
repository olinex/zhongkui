package knowledge

import (
	// 标准库
	"context"
	"strconv"
	"strings"
	"time"

	// 第三方
	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/config"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/port"
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// Knowledge 是知识库索引的入口：负责文档切块、向量化落库（knowledge_chunks）与
// 文档/分块删除。持有一个 gorm DB、向量化客户端与切片器。
type Knowledge struct {
	// db core 库连接（knowledge_chunks / knowledge_documents 读写）。
	db *database.DB
	// embed 向量化客户端（把分块检索文本转为向量）。
	embed  port.Embedder
	params *config.Params // 行为参数解析器（切片长度与重叠）
	// chunker Markdown 切片器（按参数长度与重叠切块）。
	chunker *llm.MarkdownChunker
}

// NewKnowledge 构造 Knowledge；db/embed/params 任一为空时返回英文错误。
func NewKnowledge(db *database.DB, embed port.Embedder, params *config.Params) (*Knowledge, error) {
	if db == nil {
		return nil, errors.New("knowledge: nil database")
	}
	if embed == nil {
		return nil, errors.New("knowledge: nil embedder")
	}
	if params == nil {
		return nil, errors.New("knowledge: nil params")
	}
	return &Knowledge{
		db:      db,
		embed:   embed,
		params:  params,
		chunker: llm.NewMarkdownChunker(params.Int("chunking.size"), params.Int("chunking.overlap")),
	}, nil
}

// IndexDocument 对文档执行切片与向量化，并在单事务内删除该文档旧分块后插入新分块，
// 最后更新 doc.ChunkCount 与 doc.Hash。分块正文的全文检索表示经
// to_tsvector('simple', ...) SQL 表达式写入 search 列。
func (k *Knowledge) IndexDocument(ctx context.Context, doc *ai.KnowledgeDocument) error {
	_, err := k.indexDocument(ctx, doc)
	return errors.WithStack(err)
}

// indexDocument 是 IndexDocument 的实现体，同时返回本次向量化的 token 用量（contract.EmbedUsage）：
// 逐块计算索引文本后经 embedStringsWithUsage 向量化，其余切片、hash 计算与事务落库逻辑与
// IndexDocument 完全一致。供需要上报用量的同步链路（ReindexNamespace、SyncGeneratedDocs）复用。
func (k *Knowledge) indexDocument(ctx context.Context, doc *ai.KnowledgeDocument) (contract.EmbedUsage, error) {
	if doc == nil {
		return contract.EmbedUsage{}, errors.New("knowledge: nil document")
	}
	plaintext := string(doc.Content.Value())
	chunks, err := k.chunker.Transform(ctx, []*schema.Document{{
		ID:      strconv.FormatUint(uint64(doc.ID), 10),
		Content: plaintext,
		MetaData: map[string]any{
			"namespace":  doc.NamespaceName,
			"documentID": strconv.FormatUint(uint64(doc.ID), 10),
			"title":      doc.Title,
		},
	}})
	if err != nil {
		return contract.EmbedUsage{}, errors.WithStack(err)
	}
	// 每块索引文本 = 标题路径 + 正文，向量化与全文检索共用该文本
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		path, _ := c.MetaData["headingPath"].(string)
		texts[i] = strings.TrimSpace(path + "\n" + c.Content)
	}
	vectors := make([][]float64, len(chunks))
	var usage contract.EmbedUsage
	if len(texts) > 0 {
		if vectors, usage, err = port.EmbedStringsWithUsage(ctx, k.embed, texts); err != nil {
			return contract.EmbedUsage{}, errors.WithStack(err)
		}
	}
	// 内容摘要为带密钥的 HMAC-SHA256：明文不落库，同时摘要确定可比，供过期判定使用
	hash, err := crypto.DigestForField(&ai.KnowledgeDocument{}, "hash", "json", []byte(plaintext))
	if err != nil {
		return contract.EmbedUsage{}, errors.WithStack(err)
	}
	return usage, k.db.Transaction(func(tx *gorm.DB) error {
		// 单事务内先清旧块再插入新块，保证索引原子替换
		if err := tx.Where("document_id = ?", doc.ID).Delete(&ai.KnowledgeChunk{}).Error; err != nil {
			return errors.WithStack(err)
		}
		for i, c := range chunks {
			path, _ := c.MetaData["headingPath"].(string)
			// 标题路径与正文经 crypto 序列化器加密落库（Create 走结构体路径，序列化器生效）
			chunk := &ai.KnowledgeChunk{
				DocumentID:    doc.ID,
				NamespaceName: doc.NamespaceName,
				ChunkIndex:    i,
				HeadingPath:   serializer.NewCrypto([]byte(path)),
				Content:       serializer.NewCrypto([]byte(c.Content)),
				Embedding:     retrieval.ToFloat32(vectors[i]),
			}
			if err := tx.Create(chunk).Error; err != nil {
				return errors.WithStack(err)
			}
			// search 列为 tsvector，须经 SQL 表达式写入
			if err := tx.Model(&ai.KnowledgeChunk{}).
				Where("id = ?", chunk.ID).
				Updates(map[string]any{"search": gorm.Expr("to_tsvector('simple', ?)", texts[i])}).Error; err != nil {
				return errors.WithStack(err)
			}
		}
		// 采用结构体 + Select 更新而非 map：hash 经 hash 序列化器落库，
		// map 更新走 clause.Assignment 直写会绕过序列化器
		return tx.Model(&ai.KnowledgeDocument{}).
			Where("id = ?", doc.ID).
			Select("chunk_count", "hash").
			Updates(&ai.KnowledgeDocument{
				ChunkCount: len(chunks),
				Hash:       serializer.NewHashDigest(hash),
			}).Error
	})
}

// Reindex 重新切片并重建文档的分块索引。IndexDocument 本身即在事务内先清旧块，
// 因此 Reindex 直接复用 IndexDocument。
func (k *Knowledge) Reindex(ctx context.Context, doc *ai.KnowledgeDocument) error {
	return k.IndexDocument(ctx, doc)
}

// ReindexNamespace 以当前 embedding 模型为准，重建命名空间下全部知识文档的分块与向量索引：
// 逐篇读取文档正文（经 crypto 序列化器解密）后复用 indexDocument 重新切片、向量化与落库，
// 供更换 embedding 模型（维度或语义变化）后让 knowledge_chunks 与当前模型对齐。仅处理未软删除
// 的文档，不改文档正文；返回实际重建的文档篇数，中途失败即返回该错误（已处理的文档不回滚，
// 再次执行按同一全集继续收敛）。report 为进度上报入口（为 nil 时不上报）：查询文档后上报一次
// Start(文档总数)，每篇成功后上报一次 Step（Action=contract.SyncActionReindex，含标题、耗时与 token 用量），
// 结束（含提前失败）时经 defer 上报一次 Finish。namespaceName 为空时返回英文错误。
func (k *Knowledge) ReindexNamespace(ctx context.Context, namespaceName string, report contract.SyncReporter) (int, error) {
	if strings.TrimSpace(namespaceName) == "" {
		return 0, errors.New("knowledge: empty namespace for reindex")
	}
	var docs []ai.KnowledgeDocument
	if err := k.db.WithContext(ctx).
		Select("id", "namespace_name", "title", "content").
		Where("namespace_name = ?", namespaceName).
		Find(&docs).Error; err != nil {
		return 0, errors.WithStack(err)
	}
	contract.ReportStart(report, len(docs))
	defer contract.ReportFinish(report)
	for i := range docs {
		start := time.Now()
		usage, err := k.indexDocument(ctx, &docs[i])
		if err != nil {
			return i, errors.WithStack(err)
		}
		contract.ReportStep(report, contract.SyncProgressItem{
			Title:     docs[i].Title,
			Action:    contract.SyncActionReindex,
			Count:     1,
			ElapsedMs: time.Since(start).Milliseconds(),
			Usage:     usage,
		})
	}
	return len(docs), nil
}

// DeleteDocument 删除文档及其分块：先显式删除分块（文档为软删除，DB 级联不生效），
// 再软删除文档本身，两步在同一事务内完成。
func (k *Knowledge) DeleteDocument(ctx context.Context, docID uint) error {
	return k.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("document_id = ?", docID).Delete(&ai.KnowledgeChunk{}).Error; err != nil {
			return errors.WithStack(err)
		}
		if err := tx.Where("id = ?", docID).Delete(&ai.KnowledgeDocument{}).Error; err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
}
