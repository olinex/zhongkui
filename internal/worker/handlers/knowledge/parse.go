package knowledge

import (
	// stdlib
	"context"
	"encoding/json"
	"os"
	"runtime/debug"
	"time"
	"unicode/utf8"

	// third-party
	"github.com/samber/do/v2"
	"gorm.io/gorm"

	// internal
	modelsai "zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/docconv"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etl"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/provider"
)

const (
	// parseTimeout 单次解析任务的执行超时上限。解析包含 S3 拉取解密、文本抽取与向量化，
	// 耗时可达分钟级；任务由 worker 消费且不绑定任何 HTTP 请求生命周期，故在传入 ctx 上
	// 再套一层独立超时，避免异常任务无限占用 worker 槽位。
	// parseTimeout is the execution timeout of a single parse task. Parsing involves an S3
	// fetch/decrypt, text extraction and embedding and can take minutes; the task is consumed
	// by a worker and bound to no HTTP request lifecycle, so an independent timeout is layered
	// onto the incoming ctx to keep an abnormal task from occupying a worker slot forever.
	parseTimeout = 10 * time.Minute

	// maxParseErrorBytes 写入 parse_error 的失败原因字节上限。失败原因来自第三方库或模型
	// 端点的原始报错，可能极长；截断可避免撑爆日志与列表出参，同时保留足够定位信息。
	// maxParseErrorBytes is the byte limit of a parse error persisted into parse_error. The
	// reason comes from a third-party library or a model endpoint and may be very long;
	// truncating avoids flooding logs and list responses while keeping enough context to
	// locate the failure.
	maxParseErrorBytes = 2048
)

// 编译期断言：确保 ParseHandler 实现了 etl.Handler 接口。
// Compile-time assertion that ParseHandler implements etl.Handler.
var _ etl.Handler = (*ParseHandler)(nil)

// objectMaterializer 原件拉取接口，定义在消费侧并与 s3 包解耦（*s3.ObjectCache 满足该契约），
// 便于单元测试注入替代实现，避免测试依赖真实 S3。
// objectMaterializer is the source-file materializing interface, defined on the consumer side
// and kept decoupled from the s3 package (*s3.ObjectCache satisfies it), so tests can inject a
// substitute implementation and avoid depending on real S3.
type objectMaterializer interface {
	Materialize(ctx context.Context, objectKey string) (string, error)
}

// documentIndexer 知识库索引接口，定义在消费侧并与 ai 包解耦（*ai.Knowledge 满足该契约），
// 便于单元测试注入替代实现，避免测试依赖真实 LLM 与向量库。
// documentIndexer is the knowledge-base indexing interface, defined on the consumer side and
// kept decoupled from the ai package (*ai.Knowledge satisfies it), so tests can inject a
// substitute implementation and avoid depending on a real LLM and vector store.
type documentIndexer interface {
	IndexDocument(ctx context.Context, doc *modelsai.KnowledgeDocument) error
}

// ParseHandler 知识文档导入解析处理器，消费 knowledge.ParseTask（ai:doc:parse）任务。
// 流程：读取文档行 -> 置 status=parsing 并清空 parse_error -> 经 ObjectCache.Materialize 拉取
// 解密原件为本地文件 -> docconv.Convert 抽取文本 -> 落库 content -> 经当前知识库索引器切块
// 向量化 -> 置 status=ready；任一步失败置 status=failed 并写入截断后的 parse_error 后返回错误。
// ParseHandler is the knowledge document import parse handler consuming
// knowledge.ParseTask (ai:doc:parse) tasks. Flow: load the document row -> set status=parsing
// and clear parse_error -> materialize/decrypt the source file locally via
// ObjectCache.Materialize -> extract text via docconv.Convert -> persist content -> chunk and
// embed via the current knowledge indexer -> set status=ready; a failure in any step sets
// status=failed with a truncated parse_error and returns the error.
type ParseHandler struct {
	db     *database.DB    // core 库，读取文档行并回写状态与正文
	logger *logging.Logger // 日志记录器
	// files 原件拉取器（*s3.ObjectCache），把 S3 加密原件解密为本地可打开文件
	// files the source-file materializer (*s3.ObjectCache), decrypting the S3 source into a
	// locally-openable file
	files objectMaterializer
	// indexer 惰性解析当前 AI 运行时快照里的知识库索引器。AI 运行时会在配置变更时整体重建
	// 快照，启动时固化的 *Knowledge 指针会随重建过期，故必须在每次任务执行时按当前快照重新
	// 取用；测试注入假实现时直接提供该回调。
	// indexer lazily resolves the knowledge indexer from the current AI runtime snapshot. The
	// AI runtime wholly rebuilds its snapshot on config changes, so a *Knowledge pointer
	// captured at startup goes stale after a rebuild; it must therefore be re-resolved from the
	// current snapshot on every task execution. Tests inject a fake by supplying this callback.
	indexer func() (documentIndexer, error)
	// enabled 报告 AI 能力当前是否可用。同样惰性读取当前快照，避免固化启动时的启用状态。
	// enabled reports whether the AI capability is currently available. It is likewise read
	// lazily from the current snapshot to avoid freezing the startup state.
	enabled func() bool
}

// NewParseHandler 从 do.Injector 解析依赖并创建知识文档解析处理器。AI 运行时以回调形式
// 惰性持有（见 ParseHandler.indexer 的说明），不固化启动时的知识库组件指针。
// NewParseHandler resolves dependencies from the do.Injector and creates the knowledge
// document parse handler. The AI runtime is held lazily as callbacks (see
// ParseHandler.indexer) instead of freezing the startup knowledge component pointer.
func NewParseHandler(i do.Injector) *ParseHandler {
	coreDB := provider.MustInvokeCoreDatabase(i)
	logger := provider.MustInvokeDatabaseLogger(i)
	files := provider.MustInvokeObjectCache(i)
	// provider 装配发生在启动阶段，此处无请求上下文，用 Background 作为一次性的装配上下文。
	rt := provider.MustInvokeAi(context.Background(), i)
	return &ParseHandler{
		db:      coreDB,
		logger:  logger,
		files:   files,
		enabled: rt.Enabled,
		indexer: func() (documentIndexer, error) {
			comps := rt.Current()
			if comps == nil || comps.Knowledge == nil {
				return nil, errors.New("knowledge parse: knowledge indexer is unavailable")
			}
			return comps.Knowledge, nil
		},
	}
}

// Do 消费一条 ai:doc:parse 任务：解析载荷后按文档 ID 加载文档行，先置 parsing 再执行解析，
// 成功置 ready、失败置 failed 并返回错误（不自动重试由投递侧 asynq.MaxRetry(0) 保证）。
// Do consumes one ai:doc:parse task: after parsing the payload it loads the document row by ID,
// marks it parsing and runs the parse; on success it marks it ready, and on failure it marks it
// failed and returns the error (no automatic retry, guaranteed by the producing side's
// asynq.MaxRetry(0)).
func (h *ParseHandler) Do(ctx context.Context, task etl.Task) error {
	ctx, cancel := context.WithTimeout(ctx, parseTimeout)
	defer cancel()

	var payload ParsePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return errors.WithStack(err)
	}
	if err := validateParsePayload(payload); err != nil {
		return err
	}

	var doc modelsai.KnowledgeDocument
	if err := h.db.WithContext(ctx).First(&doc, payload.DocumentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.Newf("knowledge parse: document %d not found", payload.DocumentID)
		}
		return errors.WithStack(err)
	}

	// 置 parsing 并清空上一次的 parse_error，使前端能看到「进行中」状态并按需轮询
	if err := h.setStatus(ctx, &doc, modelsai.KnowledgeDocStatusParsing, ""); err != nil {
		h.logger.Ctx(ctx).Errorf("knowledge parse: mark document %d parsing failed: %v", doc.ID, err)
		return errors.WithStack(err)
	}
	h.logger.Ctx(ctx).Infof("knowledge parse: document %d started (format=%s objectKey=%s)",
		doc.ID, doc.Format, doc.ObjectKey)

	if err := h.parseSafely(ctx, &doc); err != nil {
		h.logger.Ctx(ctx).Errorf("knowledge parse: document %d failed: %v", doc.ID, err)
		return h.fail(ctx, &doc, err)
	}
	h.logger.Ctx(ctx).Infof("knowledge parse: document %d is ready", doc.ID)
	return nil
}

// parseSafely 执行解析主体并把 panic 收敛为普通错误，交由 Do 的 fail 路径统一置 failed。
// 文本抽取依赖第三方解析库（PDF/DOCX 解析器），畸形输入可能触发其内部 panic；本 handler 若
// 不拦截，panic 会越过 Do 直接由 asynq 的 processor.perform recover 并记为任务失败，而
// 「置 failed + 写 parse_error」只存在于错误返回路径上，文档将永久停留在 parsing；叠加投递侧
// asynq.MaxRetry(0)（不重试）与重新索引对 parsing 返回 400，用户无法自助恢复，只能删除重传。
// panic 的完整调用栈只写日志，不进入对用户可见的 parse_error（后者按 maxParseErrorBytes 截断）。
// parseSafely runs the parse body and converges panics into ordinary errors so Do's fail path
// uniformly marks failed. Text extraction relies on third-party parser libraries (PDF/DOCX
// parsers) whose internal panics can be triggered by malformed input; unhandled, such a panic
// would bypass Do and be recovered by asynq's processor.perform as a task failure, while
// "mark failed + write parse_error" only lives on the error-returning path, leaving the document
// stuck in parsing forever; combined with the producing side's asynq.MaxRetry(0) (no retry) and
// reindex rejecting parsing documents with 400, the user could not recover without deleting and
// re-uploading. The full stack trace is logged only and never reaches the user-visible parse_error
// (the latter is truncated to maxParseErrorBytes).
func (h *ParseHandler) parseSafely(ctx context.Context, doc *modelsai.KnowledgeDocument) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			h.logger.Ctx(ctx).Errorf("knowledge parse: document %d panicked: %v\n%s", doc.ID, rec, debug.Stack())
			err = errors.Newf("knowledge parse: document %d panicked during parsing: %v", doc.ID, rec)
		}
	}()
	return h.parse(ctx, doc)
}

// parse 执行解析主体并收敛状态：AI 可用性校验 -> 原件键校验 -> 拉取解密原件 -> 抽取文本 ->
// 落库 content -> 切块向量化 -> 置 ready。任一步失败把错误交还调用方，由 Do 统一置 failed。
// parse runs the parse body and converges the status: AI availability check -> source-key
// check -> materialize/decrypt the source file -> extract text -> persist content -> chunk and
// embed -> mark ready. A failing step hands the error back to the caller, and Do uniformly
// marks failed.
func (h *ParseHandler) parse(ctx context.Context, doc *modelsai.KnowledgeDocument) error {
	if h.enabled == nil || !h.enabled() {
		return errors.New("knowledge parse: ai feature is disabled")
	}
	if doc.ObjectKey == "" {
		return errors.New("knowledge parse: document has no source file")
	}
	path, err := h.files.Materialize(ctx, doc.ObjectKey)
	if err != nil {
		return errors.WithStack(err)
	}
	text, err := h.extract(ctx, doc, path)
	if err != nil {
		return err
	}
	doc.Content = text
	if err := h.db.WithContext(ctx).Model(&modelsai.KnowledgeDocument{}).Where("id = ?", doc.ID).
		Update("content", doc.Content).Error; err != nil {
		return errors.WithStack(err)
	}
	if err := h.index(ctx, doc); err != nil {
		return err
	}
	if err := h.setStatus(ctx, doc, modelsai.KnowledgeDocStatusReady, ""); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// extract 打开本地原件、读取其大小并按文档格式经 docconv.Convert 抽取 Markdown 文本。
// extract opens the local source file, reads its size and extracts Markdown text according to
// the document format via docconv.Convert.
func (h *ParseHandler) extract(ctx context.Context, doc *modelsai.KnowledgeDocument, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", errors.WithStack(err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", errors.WithStack(err)
	}
	text, err := docconv.Convert(ctx, docconv.Format(doc.Format), f, info.Size())
	if err != nil {
		return "", errors.WithStack(err)
	}
	return text, nil
}

// index 取当前 AI 快照里的知识库索引器，对文档执行切块与向量化（索引器会读取 doc.Content
// 并写入 chunk_count / hash）。
// index resolves the knowledge indexer from the current AI snapshot and chunks/embeds the
// document (the indexer reads doc.Content and writes chunk_count / hash).
func (h *ParseHandler) index(ctx context.Context, doc *modelsai.KnowledgeDocument) error {
	if h.indexer == nil {
		return errors.New("knowledge parse: knowledge indexer is unavailable")
	}
	indexer, err := h.indexer()
	if err != nil {
		return errors.WithStack(err)
	}
	if err := indexer.IndexDocument(ctx, doc); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// setStatus 持久化文档状态与 parse_error，并同步更新内存对象（status 与 parse_error 一并写入，
// 避免遗留上一次的失败原因）。
// setStatus persists the document status and parse_error and updates the in-memory object
// accordingly (status and parse_error are written together, leaving no stale failure reason).
func (h *ParseHandler) setStatus(ctx context.Context, doc *modelsai.KnowledgeDocument, status modelsai.KnowledgeDocStatus, parseErr string) error {
	doc.Status = string(status)
	doc.ParseError = parseErr
	return h.db.WithContext(ctx).Model(&modelsai.KnowledgeDocument{}).Where("id = ?", doc.ID).
		Updates(map[string]any{"status": doc.Status, "parse_error": doc.ParseError}).Error
}

// fail 把文档收敛为 failed 并写入截断后的失败原因，随后返回原始错误（供 worker 记录日志）。
// 置失败态的写入自身失败时，用 errors.Join 把两个错误一并返回，绝不静默吞掉。
// fail converges the document to failed with the truncated failure reason and then returns the
// original error (for the worker to log). When the failed-state write itself fails, the two
// errors are joined and returned together, never silently swallowed.
func (h *ParseHandler) fail(ctx context.Context, doc *modelsai.KnowledgeDocument, cause error) error {
	if err := h.setStatus(ctx, doc, modelsai.KnowledgeDocStatusFailed, truncateParseError(cause.Error())); err != nil {
		h.logger.Ctx(ctx).Errorf("knowledge parse: mark document %d failed failed: %v", doc.ID, err)
		return errors.Join(cause, errors.WithStack(err))
	}
	return cause
}

// truncateParseError 把失败原因截断到 maxParseErrorBytes 上限；截断点回退到 UTF-8 字符边界，
// 保证结果仍是合法 UTF-8（不会出现半个多字节字符）。
// truncateParseError truncates a failure reason to the maxParseErrorBytes limit; the cut point
// backs off to a UTF-8 character boundary so the result stays valid UTF-8 (no partial multibyte
// character).
func truncateParseError(msg string) string {
	if len(msg) <= maxParseErrorBytes {
		return msg
	}
	cut := maxParseErrorBytes
	for cut > 0 && !utf8.RuneStart(msg[cut]) {
		cut--
	}
	return msg[:cut]
}
