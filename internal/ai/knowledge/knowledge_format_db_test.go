// 本文件用 PostgreSQL 测试库验证分区过滤真正进入检索 SQL：同一查询下，限定 native_api 只返回
// 原生接口文档的分块，不限定格式时同时返回 generated 分区；向量路与关键词路分别覆盖。

package knowledge

import (
	"context"
	"strconv"
	"testing"
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"

	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/docconv"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/pkg/testdb"
)

// createFormatTestDoc 插入一篇指定分区的知识文档并返回其 ID。
func createFormatTestDoc(t *testing.T, ctx context.Context, db *database.DB, title string, format docconv.Format) uint {
	t.Helper()
	doc := &ai.KnowledgeDocument{
		NamespaceName: "default",
		Title:         title,
		Content:       serializer.NewCrypto([]byte(title)),
		Format:        string(format),
		Status:        string(ai.KnowledgeDocStatusReady),
	}
	if err := db.WithContext(ctx).Create(doc).Error; err != nil {
		t.Fatalf("create document %q error = %v", title, err)
	}
	return doc.ID
}

// createFormatTestChunk 插入一个可分块并补齐 tsvector 检索列；向量取零向量，仅用于验证分区过滤。
func createFormatTestChunk(t *testing.T, ctx context.Context, db *database.DB, docID uint, text string) {
	t.Helper()
	chunk := &ai.KnowledgeChunk{
		DocumentID:    docID,
		NamespaceName: "default",
		ChunkIndex:    0,
		HeadingPath:   serializer.NewCrypto([]byte("## 测试")),
		Content:       serializer.NewCrypto([]byte(text)),
		Embedding:     make([]float32, 1536),
	}
	if err := db.WithContext(ctx).Create(chunk).Error; err != nil {
		t.Fatalf("create chunk for document %d error = %v", docID, err)
	}
	if err := db.WithContext(ctx).Exec("UPDATE knowledge_chunks SET search = to_tsvector('simple', ?) WHERE id = ?", text, chunk.ID).Error; err != nil {
		t.Fatalf("fill tsvector for chunk %d error = %v", chunk.ID, err)
	}
}

// TestRetriever_FormatFilter 校验向量路与关键词路都按 knowledge_documents.format 过滤。
func TestRetriever_FormatFilter(t *testing.T) {
	ctx := context.Background()
	db := testdb.Core(t)
	nativeID := createFormatTestDoc(t, ctx, db, "原生接口 列表", docconv.FormatNativeApi)
	generatedID := createFormatTestDoc(t, ctx, db, "叙述页 说明", docconv.FormatGenerated)
	createFormatTestChunk(t, ctx, db, nativeID, "alpha beta native")
	createFormatTestChunk(t, ctx, db, generatedID, "alpha beta generated")

	retriever := &Retriever{db: db}
	options := func(format docconv.Format) *RetrieveOptions {
		return &RetrieveOptions{NamespaceName: "default", Format: format, TopK: 10, Candidates: 10}
	}

	vector, err := retriever.vectorRetrieve(ctx, options(docconv.FormatNativeApi), make([]float64, 1536))
	if err != nil {
		t.Fatalf("vectorRetrieve(native_api) error = %v", err)
	}
	assertFormatHits(t, "vector native_api", vector, nativeID)

	keyword, err := retriever.keywordRetrieve(ctx, options(docconv.FormatNativeApi), "alpha")
	if err != nil {
		t.Fatalf("keywordRetrieve(native_api) error = %v", err)
	}
	assertFormatHits(t, "keyword native_api", keyword, nativeID)

	vectorAll, err := retriever.vectorRetrieve(ctx, options(""), make([]float64, 1536))
	if err != nil {
		t.Fatalf("vectorRetrieve(all) error = %v", err)
	}
	if len(vectorAll) != 2 {
		t.Errorf("vectorRetrieve(all) hits = %d, want 2", len(vectorAll))
	}

	keywordAll, err := retriever.keywordRetrieve(ctx, options(""), "alpha")
	if err != nil {
		t.Fatalf("keywordRetrieve(all) error = %v", err)
	}
	if len(keywordAll) != 2 {
		t.Errorf("keywordRetrieve(all) hits = %d, want 2", len(keywordAll))
	}
}

// assertFormatHits 断言命中只包含指定文档，且恰好一条。
func assertFormatHits(t *testing.T, label string, hits []*retrieval.RetrievalResult, wantDocID uint) {
	t.Helper()
	if len(hits) != 1 {
		t.Fatalf("%s hits = %d, want 1", label, len(hits))
	}
	if hits[0].DocumentID != strconv.FormatUint(uint64(wantDocID), 10) {
		t.Errorf("%s documentID = %s, want %d", label, hits[0].DocumentID, wantDocID)
	}
}
