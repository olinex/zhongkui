package knowledge

import (
	// stdlib
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	// third-party
	"github.com/smartystreets/goconvey/convey"

	// internal
	modelsai "zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/testdb"
)

// fakeMaterializer 实现 objectMaterializer 接口：返回预置的本地文件路径或固定的拉取错误。
// fakeMaterializer implements objectMaterializer: it returns a preset local file path or a
// fixed materialize error.
type fakeMaterializer struct {
	path string
	err  error
	// panicValue 非 nil 时 Materialize 以该值 panic，用于验证解析主体对第三方库 panic 的兜底。
	// panicValue, when non-nil, makes Materialize panic with it to verify the parse body's
	// convergence of third-party library panics.
	panicValue any
}

// Materialize 返回预置路径；配置了错误时返回该错误，配置了 panicValue 时以该值 panic。
// Materialize returns the preset path, or the configured error when set, or panics with the
// configured panicValue when set.
func (f *fakeMaterializer) Materialize(_ context.Context, _ string) (string, error) {
	if f.panicValue != nil {
		panic(f.panicValue)
	}
	if f.err != nil {
		return "", f.err
	}
	return f.path, nil
}

// fakeIndexer 实现 documentIndexer 接口：把固定分块数写入文档行并回填内存对象，或返回固定错误。
// fakeIndexer implements documentIndexer: it writes a fixed chunk count to the document row and
// back-fills the in-memory object, or returns a fixed error.
type fakeIndexer struct {
	db     *database.DB
	chunks int
	err    error
}

// IndexDocument 写入分块数并回填 doc.ChunkCount；配置了错误时返回该错误。
// IndexDocument writes the chunk count and back-fills doc.ChunkCount, or returns the
// configured error when set.
func (f *fakeIndexer) IndexDocument(ctx context.Context, doc *modelsai.KnowledgeDocument) error {
	if f.err != nil {
		return f.err
	}
	if err := f.db.WithContext(ctx).Model(&modelsai.KnowledgeDocument{}).Where("id = ?", doc.ID).
		Update("chunk_count", f.chunks).Error; err != nil {
		return errors.WithStack(err)
	}
	doc.ChunkCount = f.chunks
	return nil
}

// newParseTestDB 返回已迁移的共享 PostgreSQL 测试库（testdb.Core）。
// newParseTestDB returns the migrated shared PostgreSQL test DB (testdb.Core).
func newParseTestDB(t *testing.T) *database.DB {
	t.Helper()
	return testdb.Core(t)
}

// newParseHandler 构造直连测试依赖的解析处理器：注入假原件拉取器、假索引器与 AI 可用性，
// 不依赖真实 S3 与真实 LLM。
// newParseHandler builds a parse handler wired to test doubles: a fake materializer, a fake
// indexer and an AI availability flag, depending on neither real S3 nor a real LLM.
func newParseHandler(t *testing.T, db *database.DB, files objectMaterializer, indexer documentIndexer, enabled bool) *ParseHandler {
	t.Helper()
	logger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatal(err)
	}
	h := &ParseHandler{
		db:      db,
		logger:  logger,
		files:   files,
		enabled: func() bool { return enabled },
	}
	if indexer != nil {
		h.indexer = func() (documentIndexer, error) { return indexer, nil }
	}
	return h
}

// seedParseDocument 写入一条测试知识文档（缺省命名空间 default、状态 pending）。
// seedParseDocument inserts a test knowledge document (defaulting to namespace default and
// status pending).
func seedParseDocument(t *testing.T, db *database.DB, doc *modelsai.KnowledgeDocument) *modelsai.KnowledgeDocument {
	t.Helper()
	if doc.NamespaceName == "" {
		doc.NamespaceName = "default"
	}
	if doc.Status == "" {
		doc.Status = string(modelsai.KnowledgeDocStatusPending)
	}
	if err := db.Create(doc).Error; err != nil {
		t.Fatal(err)
	}
	return doc
}

// writeTempSource 把内容写入临时目录下的 markdown 文件并返回路径。
// writeTempSource writes the content into a markdown file under a temp dir and returns its path.
func writeTempSource(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// reloadDocument 从库中重新读取文档行。
// reloadDocument re-reads the document row from the database.
func reloadDocument(t *testing.T, db *database.DB, id uint) modelsai.KnowledgeDocument {
	t.Helper()
	var doc modelsai.KnowledgeDocument
	if err := db.First(&doc, id).Error; err != nil {
		t.Fatal(err)
	}
	return doc
}

// TestParseHandler_Do_Success 验证成功路径：状态收敛为 ready、content 落库、chunk_count 由
// 索引器写入且 parse_error 为空。
// TestParseHandler_Do_Success verifies the success path: the status converges to ready, the
// content is persisted, the indexer writes chunk_count and parse_error stays empty.
func TestParseHandler_Do_Success(t *testing.T) {
	convey.Convey("ParseHandler 成功路径", t, func() {
		db := newParseTestDB(t)
		const source = "# 标题\n\n正文内容\n"
		doc := seedParseDocument(t, db, &modelsai.KnowledgeDocument{
			Title:        "doc",
			Format:       "markdown",
			OriginalName: "doc.md",
			ObjectKey:    "knowledge/default/1.md",
			SizeBytes:    int64(len(source)),
		})
		handler := newParseHandler(t, db, &fakeMaterializer{path: writeTempSource(t, source)},
			&fakeIndexer{db: db, chunks: 2}, true)

		task, err := NewParseTask(doc.ID)
		convey.So(err, convey.ShouldBeNil)
		convey.So(handler.Do(context.Background(), task), convey.ShouldBeNil)

		got := reloadDocument(t, db, doc.ID)
		convey.So(got.Status, convey.ShouldEqual, string(modelsai.KnowledgeDocStatusReady))
		convey.So(got.Content, convey.ShouldEqual, source)
		convey.So(got.ChunkCount, convey.ShouldEqual, 2)
		convey.So(got.ParseError, convey.ShouldBeEmpty)
	})
}

// TestParseHandler_Do_MaterializeFails 验证原件拉取失败路径：状态为 failed、parse_error 非空
// 且返回错误。
// TestParseHandler_Do_MaterializeFails verifies the materialize failure path: the status is
// failed, parse_error is non-empty and an error is returned.
func TestParseHandler_Do_MaterializeFails(t *testing.T) {
	convey.Convey("ParseHandler 原件拉取失败", t, func() {
		db := newParseTestDB(t)
		doc := seedParseDocument(t, db, &modelsai.KnowledgeDocument{
			Title: "doc", Format: "markdown", ObjectKey: "knowledge/default/2.md",
		})
		handler := newParseHandler(t, db, &fakeMaterializer{err: errors.New("object not found")},
			&fakeIndexer{db: db, chunks: 1}, true)

		task, err := NewParseTask(doc.ID)
		convey.So(err, convey.ShouldBeNil)
		convey.So(handler.Do(context.Background(), task), convey.ShouldNotBeNil)

		got := reloadDocument(t, db, doc.ID)
		convey.So(got.Status, convey.ShouldEqual, string(modelsai.KnowledgeDocStatusFailed))
		convey.So(got.ParseError, convey.ShouldNotBeEmpty)
		convey.So(got.Content, convey.ShouldBeEmpty)
	})
}

// TestParseHandler_Do_AIDisabled 验证 AI 能力不可用路径：状态为 failed 且原因为明确的英文说明。
// TestParseHandler_Do_AIDisabled verifies the AI-disabled path: the status is failed with an
// explicit English reason.
func TestParseHandler_Do_AIDisabled(t *testing.T) {
	convey.Convey("ParseHandler AI 能力不可用", t, func() {
		db := newParseTestDB(t)
		doc := seedParseDocument(t, db, &modelsai.KnowledgeDocument{
			Title: "doc", Format: "markdown", ObjectKey: "knowledge/default/3.md",
		})
		handler := newParseHandler(t, db, &fakeMaterializer{path: writeTempSource(t, "body")},
			&fakeIndexer{db: db, chunks: 1}, false)

		task, err := NewParseTask(doc.ID)
		convey.So(err, convey.ShouldBeNil)
		convey.So(handler.Do(context.Background(), task), convey.ShouldNotBeNil)

		got := reloadDocument(t, db, doc.ID)
		convey.So(got.Status, convey.ShouldEqual, string(modelsai.KnowledgeDocStatusFailed))
		convey.So(got.ParseError, convey.ShouldContainSubstring, "disabled")
	})
}

// TestParseHandler_Do_EmptyObjectKey 验证无原件键路径：视为解析失败并置 failed。
// TestParseHandler_Do_EmptyObjectKey verifies the missing source-key path: it is treated as a
// parse failure and marked failed.
func TestParseHandler_Do_EmptyObjectKey(t *testing.T) {
	convey.Convey("ParseHandler 原件键为空", t, func() {
		db := newParseTestDB(t)
		doc := seedParseDocument(t, db, &modelsai.KnowledgeDocument{
			Title: "doc", Format: "markdown",
		})
		handler := newParseHandler(t, db, &fakeMaterializer{path: writeTempSource(t, "body")},
			&fakeIndexer{db: db, chunks: 1}, true)

		task, err := NewParseTask(doc.ID)
		convey.So(err, convey.ShouldBeNil)
		convey.So(handler.Do(context.Background(), task), convey.ShouldNotBeNil)

		got := reloadDocument(t, db, doc.ID)
		convey.So(got.Status, convey.ShouldEqual, string(modelsai.KnowledgeDocStatusFailed))
		convey.So(got.ParseError, convey.ShouldNotBeEmpty)
	})
}

// TestParseHandler_Do_PanicConvergedToFailed 验证第三方解析库 panic 被兜底到失败路径：状态为
// failed 且 parse_error 含 panic 说明，而不是停留在 parsing（该状态会被重新索引拒绝，用户
// 无法自助恢复）。
// TestParseHandler_Do_PanicConvergedToFailed verifies a third-party parser panic is converged
// into the failure path: the status is failed with the panic reason in parse_error, instead of
// remaining in parsing (a state reindex rejects, leaving the user unable to recover).
func TestParseHandler_Do_PanicConvergedToFailed(t *testing.T) {
	convey.Convey("ParseHandler panic 兜底", t, func() {
		db := newParseTestDB(t)
		doc := seedParseDocument(t, db, &modelsai.KnowledgeDocument{
			Title: "doc", Format: "pdf", ObjectKey: "knowledge/default/5.pdf",
		})
		handler := newParseHandler(t, db, &fakeMaterializer{panicValue: "malformed pdf"},
			&fakeIndexer{db: db, chunks: 1}, true)

		task, err := NewParseTask(doc.ID)
		convey.So(err, convey.ShouldBeNil)
		convey.So(handler.Do(context.Background(), task), convey.ShouldNotBeNil)

		got := reloadDocument(t, db, doc.ID)
		convey.So(got.Status, convey.ShouldEqual, string(modelsai.KnowledgeDocStatusFailed))
		convey.So(got.ParseError, convey.ShouldContainSubstring, "panicked")
		convey.So(got.ParseError, convey.ShouldContainSubstring, "malformed pdf")
	})
}

// TestParseHandler_Do_TruncatesParseError 验证超长失败原因被截断到上限，且截断点为合法 UTF-8
// 字符边界。
// TestParseHandler_Do_TruncatesParseError verifies an over-long failure reason is truncated to
// the limit and the cut point is a valid UTF-8 character boundary.
func TestParseHandler_Do_TruncatesParseError(t *testing.T) {
	convey.Convey("ParseHandler 截断超长失败原因", t, func() {
		db := newParseTestDB(t)
		doc := seedParseDocument(t, db, &modelsai.KnowledgeDocument{
			Title: "doc", Format: "markdown", ObjectKey: "knowledge/default/4.md",
		})
		longReason := strings.Repeat("啊", maxParseErrorBytes)
		handler := newParseHandler(t, db, &fakeMaterializer{err: errors.New(longReason)},
			&fakeIndexer{db: db, chunks: 1}, true)

		task, err := NewParseTask(doc.ID)
		convey.So(err, convey.ShouldBeNil)
		convey.So(handler.Do(context.Background(), task), convey.ShouldNotBeNil)

		got := reloadDocument(t, db, doc.ID)
		convey.So(got.Status, convey.ShouldEqual, string(modelsai.KnowledgeDocStatusFailed))
		convey.So(len(got.ParseError), convey.ShouldBeLessThanOrEqualTo, maxParseErrorBytes)
		convey.So(utf8.ValidString(got.ParseError), convey.ShouldBeTrue)
		convey.So(strings.HasPrefix(longReason, got.ParseError), convey.ShouldBeTrue)
	})
}
