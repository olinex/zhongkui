package ai

import (
	// 标准库

	"net/http"
	"testing"

	// 第三方
	. "github.com/smartystreets/goconvey/convey"

	// 项目内部
	modelsai "zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/docconv"
)

// seedImportedDocument 在测试命名空间内插入一份「导入文档」行（带 S3 原件键），返回其 ID。
func seedImportedDocument(t *testing.T, db *database.DB, status string) uint {
	t.Helper()
	doc := &modelsai.KnowledgeDocument{
		NamespaceName: knowledgeTestNamespace,
		Title:         "imported",
		Content:       "parsed body",
		Format:        string(docconv.FormatMarkdown),
		Status:        status,
		OriginalName:  "notes.md",
		ObjectKey:     "knowledge/" + knowledgeTestNamespace + "/1.markdown",
		SizeBytes:     11,
	}
	if err := db.Create(doc).Error; err != nil {
		t.Fatalf("seed imported document: %v", err)
	}
	return doc.ID
}

// countNamespaceDocuments 统计测试命名空间内的文档行数。
func countNamespaceDocuments(db *database.DB) int64 {
	var count int64
	if err := db.Model(&modelsai.KnowledgeDocument{}).
		Where("namespace_name = ?", knowledgeTestNamespace).Count(&count).Error; err != nil {
		panic("failed to count knowledge documents: " + err.Error())
	}
	return count
}

// reloadDocument 按 ID 重新读取文档行。
func reloadDocument(t *testing.T, db *database.DB, id uint) *modelsai.KnowledgeDocument {
	t.Helper()
	doc := &modelsai.KnowledgeDocument{}
	if err := db.Where("id = ?", id).First(doc).Error; err != nil {
		t.Fatalf("reload knowledge document %d: %v", id, err)
	}
	return doc
}

// TestKnowledgeDocumentUpload 导入上传的边界分支：不支持格式 / 超限 / AI 未启用均返回 400
// 且不落库（成功上传路径需要真实 S3 对象缓存，见文件末尾的取舍说明）。
func TestKnowledgeDocumentUpload(t *testing.T) {
	Convey("知识库文档导入上传的边界分支", t, func() {
		db := createKnowledgeTestDB(t)
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		rt := newDisabledAiRuntime(t, db)
		r := newKnowledgeTestRouter(db, knowledgeTestNamespace, rt)

		Convey("不支持的扩展名返回 400 且不落库", func() {
			w := doKnowledgeUploadRequest(r, "/ai/doc/upload", "notes.xyz", "", []byte("body"), -1)
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			So(countNamespaceDocuments(db), ShouldEqual, 0)
		})

		Convey("超出体积上限返回 400 且不落库", func() {
			w := doKnowledgeUploadRequest(r, "/ai/doc/upload", "notes.md", "", []byte("body"), maxUploadBytes+1)
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			So(countNamespaceDocuments(db), ShouldEqual, 0)
		})

		Convey("合法 markdown 但 AI 未启用返回 400 且不落库", func() {
			w := doKnowledgeUploadRequest(r, "/ai/doc/upload", "notes.md", "My Notes", []byte("# Hello"), -1)
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			So(w.Body.String(), ShouldContainSubstring, "ai feature is disabled")
			So(countNamespaceDocuments(db), ShouldEqual, 0)
		})
	})
}

// TestKnowledgeDocumentUpdateImported 导入文档的更新语义：正文只读（携带非空内容显式 400），
// 仅改标题成功且不改变状态与正文（不触发重新解析）。
func TestKnowledgeDocumentUpdateImported(t *testing.T) {
	Convey("导入文档更新语义", t, func() {
		db := createKnowledgeTestDB(t)
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		rt := newDisabledAiRuntime(t, db)
		r := newKnowledgeTestRouter(db, knowledgeTestNamespace, rt)
		id := seedImportedDocument(t, db, string(modelsai.KnowledgeDocStatusReady))

		Convey("携带非空正文被拒绝且不落库", func() {
			w := doKnowledgeJSONRequest(r, http.MethodPost, knowledgeDocPath(id, "/update"), map[string]any{
				"id": id, "title": "renamed", "content": "overwritten",
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			So(w.Body.String(), ShouldContainSubstring, "read-only")
			doc := reloadDocument(t, db, id)
			So(doc.Title, ShouldEqual, "imported")
			So(doc.Content, ShouldEqual, "parsed body")
		})

		Convey("仅改标题成功且状态与正文不变", func() {
			w := doKnowledgeJSONRequest(r, http.MethodPost, knowledgeDocPath(id, "/update"), map[string]any{
				"id": id, "title": "renamed",
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			doc := reloadDocument(t, db, id)
			So(doc.Title, ShouldEqual, "renamed")
			So(doc.Status, ShouldEqual, string(modelsai.KnowledgeDocStatusReady))
			So(doc.Content, ShouldEqual, "parsed body")
			So(doc.ObjectKey, ShouldNotBeEmpty)
		})
	})
}

// TestKnowledgeDocumentReindexGuard 重新索引的来源语义：AI 未启用返回 400；导入文档在
// pending/parsing 进行中态被拒绝重复投递（在触达管道客户端之前返回，故无需真实 asynq）。
func TestKnowledgeDocumentReindexGuard(t *testing.T) {
	Convey("重新索引的来源守卫", t, func() {
		db := createKnowledgeTestDB(t)
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})

		Convey("AI 未启用时返回 400", func() {
			rt := newDisabledAiRuntime(t, db)
			r := newKnowledgeTestRouter(db, knowledgeTestNamespace, rt)
			id := seedImportedDocument(t, db, string(modelsai.KnowledgeDocStatusReady))

			w := doKnowledgeJSONRequest(r, http.MethodPost, knowledgeDocPath(id, "/reindex"), nil)
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			So(w.Body.String(), ShouldContainSubstring, "ai feature is disabled")
		})

		Convey("导入文档处于 pending/parsing 时拒绝重复投递", func() {
			rt := newEnabledAiRuntime(t, db)
			r := newKnowledgeTestRouter(db, knowledgeTestNamespace, rt)
			pendingID := seedImportedDocument(t, db, string(modelsai.KnowledgeDocStatusPending))
			parsingID := seedImportedDocument(t, db, string(modelsai.KnowledgeDocStatusParsing))

			w := doKnowledgeJSONRequest(r, http.MethodPost, knowledgeDocPath(pendingID, "/reindex"), nil)
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			So(w.Body.String(), ShouldContainSubstring, "already being parsed")

			w = doKnowledgeJSONRequest(r, http.MethodPost, knowledgeDocPath(parsingID, "/reindex"), nil)
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			So(w.Body.String(), ShouldContainSubstring, "already being parsed")
		})
	})
}

// TestKnowledgeDocumentContentType 按格式映射下载响应 Content-Type（未知格式回退
// application/octet-stream）。下载成功路径需真实 S3 对象缓存，故此处仅覆盖纯映射逻辑。
func TestKnowledgeDocumentContentType(t *testing.T) {
	Convey("下载响应 Content-Type 映射", t, func() {
		Convey("已知格式返回对应类型", func() {
			So(knowledgeDocumentContentType(string(docconv.FormatMarkdown)), ShouldEqual, "text/plain; charset=utf-8")
			So(knowledgeDocumentContentType(string(docconv.FormatTxt)), ShouldEqual, "text/plain; charset=utf-8")
			So(knowledgeDocumentContentType(string(docconv.FormatPDF)), ShouldEqual, "application/pdf")
			So(knowledgeDocumentContentType(string(docconv.FormatDocx)), ShouldEqual,
				"application/vnd.openxmlformats-officedocument.wordprocessingml.document")
		})

		Convey("未知格式回退为 application/octet-stream", func() {
			So(knowledgeDocumentContentType("unknown"), ShouldEqual, "application/octet-stream")
			So(knowledgeDocumentContentType(""), ShouldEqual, "application/octet-stream")
		})
	})
}
