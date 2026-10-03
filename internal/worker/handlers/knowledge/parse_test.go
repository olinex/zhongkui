package knowledge

import (
	// 标准库
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	// 第三方
	"github.com/smartystreets/goconvey/convey"

	// 项目内部
	"zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/testdb"
)

// TestMain 初始化全局 KeyDerivation：解析成功后文档正文经 crypto 序列化器加密落库，
// 测试须与生产一致地派生密钥，否则写库会报 KeyDerivation not initialized。
func TestMain(m *testing.M) {
	pb, err := key.LoadPasswordBook("")
	if err != nil {
		panic("failed to load password book: " + err.Error())
	}
	key.InitGlobalKeyDerivation(key.NewConfigFileSecretKeyProvider("this-is-a-32-byte-secret-key!!"), pb)
	os.Exit(m.Run())
}

// fakeMaterializer 实现 objectMaterializer 接口：返回预置的本地文件路径或固定的拉取错误。
type fakeMaterializer struct {
	path string
	err  error
	// panicValue 非 nil 时 Materialize 以该值 panic，用于验证解析主体对第三方库 panic 的兜底。
	panicValue any
}

// Materialize 返回预置路径；配置了错误时返回该错误，配置了 panicValue 时以该值 panic。
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
type fakeIndexer struct {
	db     *database.DB
	chunks int
	err    error
}

// IndexDocument 写入分块数并回填 doc.ChunkCount；配置了错误时返回该错误。
func (f *fakeIndexer) IndexDocument(ctx context.Context, doc *ai.KnowledgeDocument) error {
	if f.err != nil {
		return f.err
	}
	if err := f.db.WithContext(ctx).Model(&ai.KnowledgeDocument{}).Where("id = ?", doc.ID).
		Update("chunk_count", f.chunks).Error; err != nil {
		return errors.WithStack(err)
	}
	doc.ChunkCount = f.chunks
	return nil
}

// newParseTestDB 返回已迁移的共享 PostgreSQL 测试库（testdb.Core）。
func newParseTestDB(t *testing.T) *database.DB {
	t.Helper()
	return testdb.Core(t)
}

// newParseHandler 构造直连测试依赖的解析处理器：注入假原件拉取器、假索引器与 AI 可用性，
// 不依赖真实 S3 与真实 LLM。
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
func seedParseDocument(t *testing.T, db *database.DB, doc *ai.KnowledgeDocument) *ai.KnowledgeDocument {
	t.Helper()
	if doc.NamespaceName == "" {
		doc.NamespaceName = "default"
	}
	if doc.Status == "" {
		doc.Status = string(ai.KnowledgeDocStatusPending)
	}
	if err := db.Create(doc).Error; err != nil {
		t.Fatal(err)
	}
	return doc
}

// writeTempSource 把内容写入临时目录下的 markdown 文件并返回路径。
func writeTempSource(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// reloadDocument 从库中重新读取文档行。
func reloadDocument(t *testing.T, db *database.DB, id uint) ai.KnowledgeDocument {
	t.Helper()
	var doc ai.KnowledgeDocument
	if err := db.First(&doc, id).Error; err != nil {
		t.Fatal(err)
	}
	return doc
}

// TestParseHandler_Do_Success 验证成功路径：状态收敛为 ready、content 落库、chunk_count 由
// 索引器写入且 parse_error 为空。
func TestParseHandler_Do_Success(t *testing.T) {
	convey.Convey("ParseHandler 成功路径", t, func() {
		db := newParseTestDB(t)
		const source = "# 标题\n\n正文内容\n"
		doc := seedParseDocument(t, db, &ai.KnowledgeDocument{
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
		convey.So(got.Status, convey.ShouldEqual, string(ai.KnowledgeDocStatusReady))
		convey.So(string(got.Content.Value()), convey.ShouldEqual, source)
		convey.So(got.ChunkCount, convey.ShouldEqual, 2)
		convey.So(got.ParseError, convey.ShouldBeEmpty)
	})
}

// TestParseHandler_Do_MaterializeFails 验证原件拉取失败路径：状态为 failed、parse_error 非空
// 且返回错误。
func TestParseHandler_Do_MaterializeFails(t *testing.T) {
	convey.Convey("ParseHandler 原件拉取失败", t, func() {
		db := newParseTestDB(t)
		doc := seedParseDocument(t, db, &ai.KnowledgeDocument{
			Title: "doc", Format: "markdown", ObjectKey: "knowledge/default/2.md",
		})
		handler := newParseHandler(t, db, &fakeMaterializer{err: errors.New("object not found")},
			&fakeIndexer{db: db, chunks: 1}, true)

		task, err := NewParseTask(doc.ID)
		convey.So(err, convey.ShouldBeNil)
		convey.So(handler.Do(context.Background(), task), convey.ShouldNotBeNil)

		got := reloadDocument(t, db, doc.ID)
		convey.So(got.Status, convey.ShouldEqual, string(ai.KnowledgeDocStatusFailed))
		convey.So(got.ParseError, convey.ShouldNotBeEmpty)
		convey.So(string(got.Content.Value()), convey.ShouldBeEmpty)
	})
}

// TestParseHandler_Do_AIDisabled 验证 AI 能力不可用路径：状态为 failed 且原因为明确的英文说明。
func TestParseHandler_Do_AIDisabled(t *testing.T) {
	convey.Convey("ParseHandler AI 能力不可用", t, func() {
		db := newParseTestDB(t)
		doc := seedParseDocument(t, db, &ai.KnowledgeDocument{
			Title: "doc", Format: "markdown", ObjectKey: "knowledge/default/3.md",
		})
		handler := newParseHandler(t, db, &fakeMaterializer{path: writeTempSource(t, "body")},
			&fakeIndexer{db: db, chunks: 1}, false)

		task, err := NewParseTask(doc.ID)
		convey.So(err, convey.ShouldBeNil)
		convey.So(handler.Do(context.Background(), task), convey.ShouldNotBeNil)

		got := reloadDocument(t, db, doc.ID)
		convey.So(got.Status, convey.ShouldEqual, string(ai.KnowledgeDocStatusFailed))
		convey.So(got.ParseError, convey.ShouldContainSubstring, "disabled")
	})
}

// TestParseHandler_Do_EmptyObjectKey 验证无原件键路径：视为解析失败并置 failed。
func TestParseHandler_Do_EmptyObjectKey(t *testing.T) {
	convey.Convey("ParseHandler 原件键为空", t, func() {
		db := newParseTestDB(t)
		doc := seedParseDocument(t, db, &ai.KnowledgeDocument{
			Title: "doc", Format: "markdown",
		})
		handler := newParseHandler(t, db, &fakeMaterializer{path: writeTempSource(t, "body")},
			&fakeIndexer{db: db, chunks: 1}, true)

		task, err := NewParseTask(doc.ID)
		convey.So(err, convey.ShouldBeNil)
		convey.So(handler.Do(context.Background(), task), convey.ShouldNotBeNil)

		got := reloadDocument(t, db, doc.ID)
		convey.So(got.Status, convey.ShouldEqual, string(ai.KnowledgeDocStatusFailed))
		convey.So(got.ParseError, convey.ShouldNotBeEmpty)
	})
}

// TestParseHandler_Do_PanicConvergedToFailed 验证第三方解析库 panic 被兜底到失败路径：状态为
// failed 且 parse_error 含 panic 说明，而不是停留在 parsing（该状态会被重新索引拒绝，用户
// 无法自助恢复）。
func TestParseHandler_Do_PanicConvergedToFailed(t *testing.T) {
	convey.Convey("ParseHandler panic 兜底", t, func() {
		db := newParseTestDB(t)
		doc := seedParseDocument(t, db, &ai.KnowledgeDocument{
			Title: "doc", Format: "pdf", ObjectKey: "knowledge/default/5.pdf",
		})
		handler := newParseHandler(t, db, &fakeMaterializer{panicValue: "malformed pdf"},
			&fakeIndexer{db: db, chunks: 1}, true)

		task, err := NewParseTask(doc.ID)
		convey.So(err, convey.ShouldBeNil)
		convey.So(handler.Do(context.Background(), task), convey.ShouldNotBeNil)

		got := reloadDocument(t, db, doc.ID)
		convey.So(got.Status, convey.ShouldEqual, string(ai.KnowledgeDocStatusFailed))
		convey.So(got.ParseError, convey.ShouldContainSubstring, "panicked")
		convey.So(got.ParseError, convey.ShouldContainSubstring, "malformed pdf")
	})
}

// TestParseHandler_Do_TruncatesParseError 验证超长失败原因被截断到上限，且截断点为合法 UTF-8
// 字符边界。
func TestParseHandler_Do_TruncatesParseError(t *testing.T) {
	convey.Convey("ParseHandler 截断超长失败原因", t, func() {
		db := newParseTestDB(t)
		doc := seedParseDocument(t, db, &ai.KnowledgeDocument{
			Title: "doc", Format: "markdown", ObjectKey: "knowledge/default/4.md",
		})
		longReason := strings.Repeat("啊", maxParseErrorBytes)
		handler := newParseHandler(t, db, &fakeMaterializer{err: errors.New(longReason)},
			&fakeIndexer{db: db, chunks: 1}, true)

		task, err := NewParseTask(doc.ID)
		convey.So(err, convey.ShouldBeNil)
		convey.So(handler.Do(context.Background(), task), convey.ShouldNotBeNil)

		got := reloadDocument(t, db, doc.ID)
		convey.So(got.Status, convey.ShouldEqual, string(ai.KnowledgeDocStatusFailed))
		convey.So(len(got.ParseError), convey.ShouldBeLessThanOrEqualTo, maxParseErrorBytes)
		convey.So(utf8.ValidString(got.ParseError), convey.ShouldBeTrue)
		convey.So(strings.HasPrefix(longReason, got.ParseError), convey.ShouldBeTrue)
	})
}
