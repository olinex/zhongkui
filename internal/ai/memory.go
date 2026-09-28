// 本文件实现长期记忆（agent_memories）的存储与检索：按「命名空间 + Agent 类型 + 会话」归属与
// 隔离（记忆只在其来源会话内可见），写入时做「规范化正文 sha256 精确去重 + pgvector 余弦相似度
// 语义合并」并同步向量列与全文检索列，检索时做「向量路 + 关键词路」混合召回、RRF 融合、按参数
// 阈值内容去重后取 topK，命中异步回写使用统计。检索结果的去重复用知识库同包的文本 n-gram
// Jaccard 逻辑（与写入的余弦去重是两回事，勿混用）。
package ai

import (
	// 标准库
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	// 第三方
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	// 项目内部
	"zhongkui.com/zhongkui/internal/ai/tools"
	"zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/llm"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// summaryEvictedMessageByteLimit 是滚动摘要/记忆抽取提示词中单条被折叠消息正文的字节上限：
// 控制 prompt 体积，避免一条超长消息挤占摘要预算（按 UTF-8 rune 边界截断，不产生残缺字符）。
const summaryEvictedMessageByteLimit = 2000

// sessionTopicByteLimit 是会话短主题的字节上限（按 UTF-8 rune 边界截断，不产生残缺字符）。
const sessionTopicByteLimit = 64

// ErrMemoryNotFound 表示在指定命名空间下未找到目标记忆。
var ErrMemoryNotFound = errors.New("memory: not found")

// MemoryStore 是长期记忆的存储与检索入口：持有一个 gorm DB、向量化客户端、行为参数解析器与
// 日志记录器，负责记忆的 CRUD、向量/全文混合检索与去重落库。
type MemoryStore struct {
	db     *database.DB
	embed  embedder
	params *llm.Params
	logger *logging.Logger // 可空：为 nil 时跳过告警日志
}

// NewMemoryStore 构造 MemoryStore；db/embed/params 任一为空时返回英文错误，logger 允许为 nil
// （内部判空后再用）。
func NewMemoryStore(db *database.DB, embed embedder, params *llm.Params, logger *logging.Logger) (*MemoryStore, error) {
	if db == nil {
		return nil, errors.New("memory: nil database")
	}
	if embed == nil {
		return nil, errors.New("memory: nil embedder")
	}
	if params == nil {
		return nil, errors.New("memory: nil params")
	}
	return &MemoryStore{db: db, embed: embed, params: params, logger: logger}, nil
}

// MemoryHit 是一条检索到的长期记忆。
type MemoryHit struct {
	// ID 记忆 ID
	ID uint
	// Kind 记忆类别
	Kind llm.MemoryKind
	// Title 记忆标题
	Title string
	// Content 记忆正文（按 memory.maxItemBytes 截断）
	Content string
	// Score 融合后的相关度分数
	Score float64
}

// MemoryQuery 是记忆列表的过滤与分页条件。
type MemoryQuery struct {
	// NamespaceName 限定命名空间（必填，非空）
	NamespaceName string
	// AgentKind 按 Agent 类型过滤（非空时生效）
	AgentKind llm.AgentKind
	// Kind 按记忆类别过滤（非空时生效）
	Kind llm.MemoryKind
	// Status 按记忆状态过滤（非空时生效）
	Status llm.MemoryStatus
	// Keyword 按标题/正文模糊匹配（非空时生效）
	Keyword string
	// Offset 分页偏移（小于 0 时取 0）
	Offset int
	// Limit 每页条数（小于等于 0 时取默认 20）
	Limit int
}

// defaultMemoryListLimit 是记忆列表的默认每页条数。
const defaultMemoryListLimit = 20

// 记忆检索 SQL 行：仅含检索所需列，避免载入 embedding 大字段。
type memorySearchRow struct {
	ID      uint
	Kind    string
	Title   string
	Content string
	Score   float64
}

// Write 写入一条长期记忆：先做规范化正文的 sha256 精确去重，未命中再按 pgvector 余弦相似度
// 做语义合并，仍无则新增；三条路径均在同一事务内完成并同步 search/embedding 列。精确去重与语义
// 合并都命中同一行时刷新该行（正文/标题/类别/向量/全文表示/状态/时间）而非新增。
// 记忆按「命名空间 + Agent 类型 + 会话」归属与隔离，故 SessionID 为 0（无会话运行）时拒绝写入，
// 避免落进任何会话都检索不到的孤儿桶。
func (s *MemoryStore) Write(ctx context.Context, in tools.MemoryWriteInput) (*ai.AgentMemory, error) {
	namespaceName := strings.TrimSpace(in.NamespaceName)
	if namespaceName == "" {
		return nil, errors.New("memory: empty namespace name")
	}
	if in.SessionID == 0 {
		return nil, errors.New("memory: empty session id, long-term memory requires a session")
	}
	content := normalizeMemoryContent(in.Content)
	if content == "" {
		return nil, errors.New("memory: empty content")
	}
	agentKind, err := llm.ParseAgentKind(in.AgentKind)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	kind, err := llm.ParseMemoryKind(in.Kind)
	if err != nil {
		return nil, err
	}
	title := strings.TrimSpace(in.Title)
	hash := sha256Hex(content)
	// 检索文本 = 标题 + 正文（与知识库同约定：to_tsvector('simple', 该文本)）
	searchText := memorySearchText(title, content)
	vectors, err := s.embed.EmbedStrings(ctx, []string{searchText})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if len(vectors) == 0 {
		return nil, errors.New("memory: embedder returned no vector")
	}
	lit := vectorLiteral(vectors[0])
	assign := memoryWriteAssignments(in, kind, title, content, hash, lit)

	var out *ai.AgentMemory
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1) 精确去重：按唯一索引四列查既有行（含非 active 与被软删除的行，后者一并复活，
		// 避免唯一约束在并发/复活场景下报冲突）
		var existing ai.AgentMemory
		findErr := tx.Unscoped().
			Where("namespace_name = ? AND agent_kind = ? AND session_id = ? AND hash = ?",
				namespaceName, agentKind, in.SessionID, hash).
			Order("id ASC").First(&existing).Error
		if findErr == nil {
			if err := s.refreshMemory(tx, existing.ID, true, assign, searchText); err != nil {
				return err
			}
			return s.reload(tx, namespaceName, existing.ID, &out)
		}
		if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return errors.WithStack(findErr)
		}

		// 2) 语义合并：取同命名空间 + Agent 类型 + 会话 + active 的最近邻一条，相似度达标则刷新
		var nearest struct {
			ID       uint
			Distance float64
		}
		if err := tx.Table("agent_memories").
			Select("id, (embedding <=> ?::vector) AS distance", lit).
			Where("namespace_name = ? AND agent_kind = ? AND session_id = ? AND status = ? AND deleted_at IS NULL",
				namespaceName, agentKind, in.SessionID, llm.MemoryStatusActive).
			Order(clause.OrderBy{Expression: gorm.Expr("embedding <=> ?::vector", lit)}).
			Limit(1).Scan(&nearest).Error; err != nil {
			return errors.WithStack(err)
		}
		if nearest.ID != 0 && 1-nearest.Distance >= s.params.Float("memory.dedupeThreshold") {
			if err := s.refreshMemory(tx, nearest.ID, false, assign, searchText); err != nil {
				return err
			}
			return s.reload(tx, namespaceName, nearest.ID, &out)
		}

		// 3) 新增：唯一索引并发冲突时改走更新，确保不返回 DB 冲突错误
		mem := &ai.AgentMemory{
			NamespaceName: namespaceName,
			AgentKind:     agentKind,
			SessionID:     in.SessionID,
			Kind:          kind,
			Title:         title,
			Content:       content,
			Embedding:     toFloat32(vectors[0]),
			Hash:          hash,
			Status:        llm.MemoryStatusActive,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "namespace_name"},
				{Name: "agent_kind"},
				{Name: "session_id"},
				{Name: "hash"},
			},
			DoUpdates: clause.Assignments(assign),
		}).Create(mem).Error; err != nil {
			return errors.WithStack(err)
		}
		id := mem.ID
		if id == 0 {
			// 冲突改走更新时部分方言不回填主键，按唯一索引四列反查
			var got ai.AgentMemory
			if err := tx.Where("namespace_name = ? AND agent_kind = ? AND session_id = ? AND hash = ?",
				namespaceName, agentKind, in.SessionID, hash).First(&got).Error; err != nil {
				return errors.WithStack(err)
			}
			id = got.ID
		}
		if err := s.writeMemorySearch(tx, id, searchText); err != nil {
			return err
		}
		return s.reload(tx, namespaceName, id, &out)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Retrieve 执行长期记忆的混合检索：向量路 + 关键词路各取 memory.candidates 条 → RRF 融合 →
// 按 memory.dedupeThreshold 内容去重 → 取 memory.topK，再受 memory.maxItems 约束；限定
// namespace + agent_kind + session_id + status='active'，即记忆只在其来源会话内可见。
// memory.enabled=false、命名空间为空、会话为 0 或查询空白时返回 (nil, nil) 且不报错。
// 命中后异步回写使用统计（失败仅告警，不影响返回）。
func (s *MemoryStore) Retrieve(ctx context.Context, namespaceName string, agentKind llm.AgentKind, sessionID uint, query string) ([]MemoryHit, error) {
	if !s.params.Bool("memory.enabled") {
		return nil, nil
	}
	namespaceName = strings.TrimSpace(namespaceName)
	if namespaceName == "" || sessionID == 0 || strings.TrimSpace(query) == "" {
		return nil, nil
	}
	agentKind, err := llm.ParseAgentKind(agentKind)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	candidates := s.params.Int("memory.candidates")
	if candidates <= 0 {
		return nil, nil
	}
	vectors, err := s.embed.EmbedStrings(ctx, []string{query})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if len(vectors) == 0 {
		return nil, errors.New("memory: embedder returned no vector")
	}
	vecHits, vecKinds, err := s.vectorSearch(ctx, namespaceName, agentKind, sessionID, vectors[0], candidates)
	if err != nil {
		return nil, err
	}
	kwHits, kwKinds, err := s.keywordSearch(ctx, namespaceName, agentKind, sessionID, query, candidates)
	if err != nil {
		return nil, err
	}
	fused := fuseRRF(vecHits, kwHits)
	fused = dedupeSimilarResultsWithCutoff(fused, s.params.Float("memory.dedupeThreshold"))
	fused = selectTopK(fused, s.params.Int("memory.topK"))
	if maxItems := s.params.Int("memory.maxItems"); maxItems > 0 && len(fused) > maxItems {
		fused = fused[:maxItems]
	}
	maxBytes := s.params.Int("memory.maxItemBytes")
	hits := make([]MemoryHit, 0, len(fused))
	ids := make([]uint, 0, len(fused))
	for _, hit := range fused {
		id64, err := strconv.ParseUint(hit.DocumentID, 10, 64)
		if err != nil {
			continue
		}
		kind, ok := vecKinds[hit.DocumentID]
		if !ok {
			kind = kwKinds[hit.DocumentID]
		}
		hits = append(hits, MemoryHit{
			ID:      uint(id64),
			Kind:    kind,
			Title:   hit.Title,
			Content: truncateBytes(hit.Content, maxBytes),
			Score:   hit.Score,
		})
		ids = append(ids, uint(id64))
	}
	if len(ids) > 0 {
		go func() {
			if err := s.TouchUsage(ctx, ids); err != nil {
				s.warnf(ctx, "memory: touch usage failed: %v", err)
			}
		}()
	}
	return hits, nil
}

// TouchUsage 递增给定记忆的 use_count 与 last_used_at；ids 为空直接返回 nil。
func (s *MemoryStore) TouchUsage(ctx context.Context, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	if err := s.db.WithContext(ctx).Model(&ai.AgentMemory{}).
		Where("id IN ?", ids).
		Updates(map[string]any{
			"use_count":    gorm.Expr("use_count + 1"),
			"last_used_at": gorm.Expr("NOW()"),
		}).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// Get 读取指定命名空间下的一条记忆；不存在时返回 ErrMemoryNotFound。
func (s *MemoryStore) Get(ctx context.Context, namespaceName string, id uint) (*ai.AgentMemory, error) {
	namespaceName = strings.TrimSpace(namespaceName)
	if namespaceName == "" {
		return nil, errors.New("memory: empty namespace name")
	}
	var mem ai.AgentMemory
	if err := s.db.WithContext(ctx).
		Where("namespace_name = ? AND id = ?", namespaceName, id).
		First(&mem).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMemoryNotFound
		}
		return nil, errors.WithStack(err)
	}
	return &mem, nil
}

// List 分页列出命名空间下的记忆：AgentKind/Kind/Status 非空时过滤，Keyword 非空时按
// 标题/正文 ILIKE 模糊匹配；按 updated_at 降序排序，并同时返回过滤后的总数。
func (s *MemoryStore) List(ctx context.Context, q MemoryQuery) ([]*ai.AgentMemory, int64, error) {
	namespaceName := strings.TrimSpace(q.NamespaceName)
	if namespaceName == "" {
		return nil, 0, errors.New("memory: empty namespace name")
	}
	base := func() *gorm.DB {
		tx := s.db.WithContext(ctx).Model(&ai.AgentMemory{}).Where("namespace_name = ?", namespaceName)
		if q.AgentKind != "" {
			tx = tx.Where("agent_kind = ?", q.AgentKind)
		}
		if q.Kind != "" {
			tx = tx.Where("kind = ?", q.Kind)
		}
		if q.Status != "" {
			tx = tx.Where("status = ?", q.Status)
		}
		if keyword := strings.TrimSpace(q.Keyword); keyword != "" {
			like := "%" + keyword + "%"
			tx = tx.Where("title ILIKE ? OR content ILIKE ?", like, like)
		}
		return tx
	}
	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, errors.WithStack(err)
	}
	limit := q.Limit
	if limit <= 0 {
		limit = defaultMemoryListLimit
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}
	var items []*ai.AgentMemory
	if err := base().Order("updated_at DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, errors.WithStack(err)
	}
	return items, total, nil
}

// Delete 软删除指定命名空间下的若干记忆；ids 为空直接返回 nil。
func (s *MemoryStore) Delete(ctx context.Context, namespaceName string, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	namespaceName = strings.TrimSpace(namespaceName)
	if namespaceName == "" {
		return errors.New("memory: empty namespace name")
	}
	if err := s.db.WithContext(ctx).
		Where("namespace_name = ? AND id IN ?", namespaceName, ids).
		Delete(&ai.AgentMemory{}).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// SetStatus 设置指定命名空间下一条记忆的状态；status 不在取值空间内时返回英文错误。
func (s *MemoryStore) SetStatus(ctx context.Context, namespaceName string, id uint, status llm.MemoryStatus) error {
	if _, err := llm.ParseMemoryStatus(status); err != nil {
		return err
	}
	namespaceName = strings.TrimSpace(namespaceName)
	if namespaceName == "" {
		return errors.New("memory: empty namespace name")
	}
	res := s.db.WithContext(ctx).Model(&ai.AgentMemory{}).
		Where("namespace_name = ? AND id = ?", namespaceName, id).
		Update("status", string(status))
	if res.Error != nil {
		return errors.WithStack(res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrMemoryNotFound
	}
	return nil
}

// memoryStoreWriter 把 MemoryStore 适配为 internal/ai/tools 的写入器接口：该包的
// memory_write 工具在工具循环中经 ctx 取回本适配器，把模型给出的内容落库。适配器的入参即
// tools.MemoryWriteInput，直接透传给 MemoryStore.Write，规范化、去重与语义合并全部由其承担。
// 适配器声明在本包内是因为 internal/ai/tools 不能引用本包（导入环），故由本包单向满足其接口。
type memoryStoreWriter struct {
	// store 底层长期记忆存储（nil 时 WriteMemory 返回英文错误）
	store *MemoryStore
}

// WriteMemory 落库一条由 memory_write 工具提交的记忆并返回其 ID：store 缺失、命名空间为空或
// 内容为空时返回英文错误，其余校验与落库语义完全委托 MemoryStore.Write。
func (w memoryStoreWriter) WriteMemory(ctx context.Context, in tools.MemoryWriteInput) (uint, error) {
	if w.store == nil {
		return 0, errors.New("memory: nil store")
	}
	mem, err := w.store.Write(ctx, in)
	if err != nil {
		return 0, err
	}
	return mem.ID, nil
}

// vectorSearch 执行向量路检索：在「命名空间 + Agent 类型 + 会话」范围内按余弦距离升序取前
// limit 条 active 记忆，返回检索结果与 DocumentID → 记忆类别的映射。
func (s *MemoryStore) vectorSearch(ctx context.Context, namespaceName string, agentKind llm.AgentKind, sessionID uint, vec []float64, limit int) ([]*RetrievalResult, map[string]llm.MemoryKind, error) {
	if limit <= 0 || len(vec) == 0 {
		return nil, nil, nil
	}
	lit := vectorLiteral(vec)
	var rows []memorySearchRow
	if err := s.db.WithContext(ctx).Table("agent_memories").
		Select("id, kind, title, content, (1 - (embedding <=> ?::vector)) AS score", lit).
		Where("namespace_name = ? AND agent_kind = ? AND session_id = ? AND status = ? AND deleted_at IS NULL",
			namespaceName, agentKind, sessionID, llm.MemoryStatusActive).
		Order(clause.OrderBy{Expression: gorm.Expr("embedding <=> ?::vector", lit)}).
		Limit(limit).Scan(&rows).Error; err != nil {
		return nil, nil, errors.WithStack(err)
	}
	hits, kinds := memoryRowsToResults(rows)
	return hits, kinds, nil
}

// keywordSearch 执行关键词路检索：在「命名空间 + Agent 类型 + 会话」范围内以
// search @@ plainto_tsquery 过滤，按 ts_rank 降序取前 limit 条。
func (s *MemoryStore) keywordSearch(ctx context.Context, namespaceName string, agentKind llm.AgentKind, sessionID uint, query string, limit int) ([]*RetrievalResult, map[string]llm.MemoryKind, error) {
	if limit <= 0 {
		return nil, nil, nil
	}
	var rows []memorySearchRow
	if err := s.db.WithContext(ctx).Table("agent_memories").
		Select("id, kind, title, content, ts_rank(search, plainto_tsquery('simple', ?)) AS score", query).
		Where("namespace_name = ? AND agent_kind = ? AND session_id = ? AND status = ? AND deleted_at IS NULL",
			namespaceName, agentKind, sessionID, llm.MemoryStatusActive).
		Where("search @@ plainto_tsquery('simple', ?)", query).
		Order(clause.OrderBy{Expression: gorm.Expr("ts_rank(search, plainto_tsquery('simple', ?)) DESC", query)}).
		Limit(limit).Scan(&rows).Error; err != nil {
		return nil, nil, errors.WithStack(err)
	}
	hits, kinds := memoryRowsToResults(rows)
	return hits, kinds, nil
}

// refreshMemory 刷新既有记忆行：写入给定字段并重写 search 列。unscoped 为 true 时同时复活被
// 软删除的行（软删除行的唯一 hash 仍占用唯一索引，复活可避免唯一约束冲突）。
func (s *MemoryStore) refreshMemory(tx *gorm.DB, id uint, unscoped bool, assign map[string]any, searchText string) error {
	q := tx.Model(&ai.AgentMemory{})
	if unscoped {
		q = tx.Unscoped().Model(&ai.AgentMemory{})
	}
	if err := q.Where("id = ?", id).Updates(assign).Error; err != nil {
		return errors.WithStack(err)
	}
	return s.writeMemorySearch(tx, id, searchText)
}

// writeMemorySearch 经 to_tsvector('simple', ...) SQL 表达式写入 search 列（tsvector 列不能经
// 模型直接赋值）。
func (s *MemoryStore) writeMemorySearch(tx *gorm.DB, id uint, searchText string) error {
	if err := tx.Model(&ai.AgentMemory{}).Where("id = ?", id).
		Update("search", gorm.Expr("to_tsvector('simple', ?)", searchText)).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// reload 按命名空间 + ID 重新读取行到 out。
func (s *MemoryStore) reload(tx *gorm.DB, namespaceName string, id uint, out **ai.AgentMemory) error {
	var got ai.AgentMemory
	if err := tx.Where("namespace_name = ? AND id = ?", namespaceName, id).First(&got).Error; err != nil {
		return errors.WithStack(err)
	}
	*out = &got
	return nil
}

// warnf 在 logger 非空时输出告警。
func (s *MemoryStore) warnf(ctx context.Context, format string, args ...any) {
	if s.logger == nil {
		return
	}
	s.logger.Ctx(ctx).Warnf(format, args...)
}

// memoryRowsToResults 将检索 SQL 行转换为检索结果，并建立 DocumentID → 记忆类别的映射，
// 供融合后还原记忆类别（RetrievalResult 本身不携带类别）。
func memoryRowsToResults(rows []memorySearchRow) ([]*RetrievalResult, map[string]llm.MemoryKind) {
	hits := make([]*RetrievalResult, 0, len(rows))
	kinds := make(map[string]llm.MemoryKind, len(rows))
	for _, row := range rows {
		id := strconv.FormatUint(uint64(row.ID), 10)
		hits = append(hits, &RetrievalResult{
			DocumentID: id,
			Title:      row.Title,
			Content:    row.Content,
			Score:      row.Score,
		})
		kinds[id] = llm.MemoryKind(row.Kind)
	}
	return hits, kinds
}

// memoryWriteAssignments 构造写入/刷新记忆行的字段表：embedding 经 pgvector 字面量表达式写入
// （与模型 serializer 等价且更显式），deleted_at 置空以复活软删除行，status 复位为 active。
func memoryWriteAssignments(in tools.MemoryWriteInput, kind llm.MemoryKind, title, content, hash, lit string) map[string]any {
	return map[string]any{
		"session_id": in.SessionID,
		"kind":       string(kind),
		"title":      title,
		"content":    content,
		"embedding":  gorm.Expr("?::vector", lit),
		"hash":       hash,
		"status":     string(llm.MemoryStatusActive),
		"deleted_at": nil,
		"updated_at": time.Now(),
	}
}

// memorySearchText 计算记忆的全文检索文本：标题 + 正文（与知识库同一约定）。
func memorySearchText(title, content string) string {
	return strings.TrimSpace(title + "\n" + content)
}

// normalizeMemoryContent 规范化记忆正文：折叠全部空白串为单个空格并去除首尾空白，使排版差异
// 不影响 sha256 精确去重。
func normalizeMemoryContent(content string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(content), " "))
}

// Memory 是会话记忆服务：短期记忆（滚动摘要 / 会话压缩）与长期记忆（检索 / 抽取写入）的读
// 写入口。它持有一个 MemoryStore（长期记忆落库与检索）、会话存储（摘要读写用）、按档位解析
// 对话客户端的回调与行为参数解析器。视图层的会话压缩接口与 Agent 的滚动摘要均经它执行。
type Memory struct {
	store    *MemoryStore
	sessions *SessionStore // 会话存储，由 MemoryStore 的 db 同源派生
	resolver ChatResolver  // 可空：Summarize/抽取时缺失即返回英文错误
	params   *llm.Params
	logger   *logging.Logger // 可空：为 nil 时跳过告警日志
}

// NewMemory 构造 Memory 服务：store/params 为空时返回英文错误，resolver 与 logger 允许为 nil
// （resolver 缺失在调用 Summarize/抽取时才报错）。会话存储由 store 的 db 同源派生。
func NewMemory(store *MemoryStore, resolver ChatResolver, params *llm.Params, logger *logging.Logger) (*Memory, error) {
	if store == nil {
		return nil, errors.New("memory: nil store")
	}
	if params == nil {
		return nil, errors.New("memory: nil params")
	}
	sessions, err := NewSessionStore(store.db)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &Memory{store: store, sessions: sessions, resolver: resolver, params: params, logger: logger}, nil
}

// warnf 在 logger 非空时输出告警。
func (m *Memory) warnf(ctx context.Context, format string, args ...any) {
	if m.logger == nil {
		return
	}
	m.logger.Ctx(ctx).Warnf(format, args...)
}

// CompactResult 是一次会话压缩的产物：刷新后的摘要、成功落库的长期记忆条数与不影响主结果的
// 告警（长期记忆抽取/落库失败、记忆开关关闭等）。
type CompactResult struct {
	// Summary 刷新后的会话摘要
	Summary string
	// Extracted 成功落库的长期记忆条数
	Extracted int
	// Warnings 非致命告警（英文）
	Warnings []string
}

// Recall 以查询文本在「命名空间 + Agent 类型 + 会话」范围内检索长期记忆，记忆只在其来源会话内
// 可见；语义完全委托 MemoryStore.Retrieve（memory.enabled=false 时返回空且不报错）。
func (m *Memory) Recall(ctx context.Context, namespaceName string, agentKind llm.AgentKind, sessionID uint, query string) ([]MemoryHit, error) {
	return m.store.Retrieve(ctx, namespaceName, agentKind, sessionID, query)
}

// 以下薄委托方法把长期记忆的管理接口（列表/详情/删除/启停）暴露给视图层，使视图层经
// Components.Memory 直接复用 MemoryStore 的落库与去重语义，无需自己持有 store 或重写任何
// SQL。语义与参数完全等同 MemoryStore 的同名方法。

// List 分页列出命名空间下的长期记忆，语义完全委托 MemoryStore.List。
func (m *Memory) List(ctx context.Context, q MemoryQuery) ([]*ai.AgentMemory, int64, error) {
	return m.store.List(ctx, q)
}

// Get 读取指定命名空间下的一条长期记忆，未找到时返回 ErrMemoryNotFound。
func (m *Memory) Get(ctx context.Context, namespaceName string, id uint) (*ai.AgentMemory, error) {
	return m.store.Get(ctx, namespaceName, id)
}

// Write 写入一条长期记忆（向量化 + 精确哈希去重 + 语义合并），语义完全委托 MemoryStore.Write。
func (m *Memory) Write(ctx context.Context, in tools.MemoryWriteInput) (*ai.AgentMemory, error) {
	return m.store.Write(ctx, in)
}

// Delete 软删除指定命名空间下的若干长期记忆，语义完全委托 MemoryStore.Delete。
func (m *Memory) Delete(ctx context.Context, namespaceName string, ids []uint) error {
	return m.store.Delete(ctx, namespaceName, ids)
}

// SetStatus 设置指定命名空间下一条长期记忆的状态（active/archived），语义完全委托
// MemoryStore.SetStatus。
func (m *Memory) SetStatus(ctx context.Context, namespaceName string, id uint, status llm.MemoryStatus) error {
	return m.store.SetStatus(ctx, namespaceName, id, status)
}

// Summarize 用 flash 档位把「既有摘要 + 被挤出消息」重写为新的滚动摘要：返回新摘要与被挤出
// 消息的最后一条 ID（即新的 summary_upto_message_id）。evicted 为空时原样返回既有摘要与游标。
// resolver 为 nil、档位未绑定或解析失败、模型返回空内容时均返回英文错误（由调用方降级为不推进
// 游标）。摘要自身按 session.maxContextBytes 截断，避免摘要膨胀。
func (m *Memory) Summarize(ctx context.Context, session *ai.AgentSession, evicted []*ai.AgentMessage) (string, uint, error) {
	if session == nil {
		return "", 0, errors.New("memory: nil session")
	}
	if len(evicted) == 0 {
		return session.Summary, session.SummaryUptoMessageID, nil
	}
	chat, err := m.resolveFlashChat()
	if err != nil {
		return "", 0, err
	}
	messages := []*schema.Message{
		schema.SystemMessage(m.params.Text("memory.summarySystemPrompt")),
		schema.UserMessage(buildSummaryPrompt(session.Summary, evicted)),
	}
	resp, err := chat.Generate(ctx, messages)
	if err != nil {
		return "", 0, errors.WithStack(err)
	}
	if resp == nil || strings.TrimSpace(resp.Content) == "" {
		return "", 0, errors.New("memory: summary model returned an empty response")
	}
	summary := truncateBytes(strings.TrimSpace(resp.Content), m.params.Int("session.maxContextBytes"))
	return summary, evicted[len(evicted)-1].ID, nil
}

// SessionTopicFallback 是会话主题生成失败时的兜底：把首个用户问题按 sessionTopicByteLimit 截断
// 作为主题，保证会话在任何情况下都有一个可辨识的标题。
func SessionTopicFallback(query string) string {
	return truncateBytes(strings.TrimSpace(query), sessionTopicByteLimit)
}

// GenerateTopic 用 flash 档位据首个用户问题生成会话短主题，结果按 sessionTopicByteLimit 截断。
// query 为空时返回空串；resolver 缺失、档位未绑定或解析失败、模型调用失败、空响应一律告警并回退
// SessionTopicFallback，绝不返回错误，以免阻断会话创建。
func (m *Memory) GenerateTopic(ctx context.Context, query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	fallback := SessionTopicFallback(query)
	chat, err := m.resolveFlashChat()
	if err != nil {
		m.warnf(ctx, "memory: session topic generation skipped, using the query as the topic: %v", err)
		return fallback
	}
	messages := []*schema.Message{
		schema.SystemMessage(m.params.Text("session.topicSystemPrompt")),
		schema.UserMessage(query),
	}
	resp, err := chat.Generate(ctx, messages)
	if err != nil {
		m.warnf(ctx, "memory: session topic generation failed, using the query as the topic: %v", err)
		return fallback
	}
	if resp == nil || strings.TrimSpace(resp.Content) == "" {
		m.warnf(ctx, "memory: session topic model returned an empty response, using the query as the topic")
		return fallback
	}
	return truncateBytes(strings.TrimSpace(resp.Content), sessionTopicByteLimit)
}

// Compact 压缩一个会话：先把 summary_upto_message_id 之后的全部消息折叠进滚动摘要并落库，再
// 用 flash 从中抽取不超过 memory.extractMaxItems 条长期记忆写入记忆库。摘要失败（flash 不可用
// 或模型空响应）返回 (nil, err)；长期记忆抽取或落库失败只追加进 Warnings、不影响摘要结果，函数
// 仍返回 (result, nil)。memory.enabled=false 时仍刷新摘要，但跳过长期记忆抽取（Extracted=0）。
func (m *Memory) Compact(ctx context.Context, session *ai.AgentSession) (*CompactResult, error) {
	if session == nil {
		return nil, errors.New("memory: nil session")
	}
	result := &CompactResult{Summary: session.Summary}
	msgs, err := m.sessions.ListMessagesAfter(ctx, session.ID, session.SummaryUptoMessageID)
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return result, nil
	}
	summary, upto, err := m.Summarize(ctx, session, msgs)
	if err != nil {
		return nil, err
	}
	if err := m.sessions.UpdateSummary(ctx, session.ID, summary, upto); err != nil {
		return nil, err
	}
	session.Summary = summary
	session.SummaryUptoMessageID = upto
	result.Summary = summary
	if !m.params.Bool("memory.enabled") {
		result.Warnings = append(result.Warnings, "memory: long-term memory is disabled, extraction skipped")
		return result, nil
	}
	extracted, warnings := m.extract(ctx, session, msgs)
	result.Extracted = extracted
	result.Warnings = append(result.Warnings, warnings...)
	return result, nil
}

// extract 从被摘要的消息中抽取长期记忆并落库：用 flash 档位生成结果、容错解析 JSON，逐条经
// MemoryStore.Write 落库（归属取自会话）。返回成功条数与非致命告警；任何失败都不返回错误，
// 以告警形式由 Compact 汇总，从而绝不影响摘要结果。
func (m *Memory) extract(ctx context.Context, session *ai.AgentSession, msgs []*ai.AgentMessage) (int, []string) {
	chat, err := m.resolveFlashChat()
	if err != nil {
		return 0, []string{"memory: extraction skipped: " + err.Error()}
	}
	messages := []*schema.Message{
		schema.SystemMessage(m.params.Text("memory.extractSystemPrompt")),
		schema.UserMessage(buildMessageTranscript(msgs, summaryEvictedMessageByteLimit)),
	}
	resp, err := chat.Generate(ctx, messages)
	if err != nil {
		return 0, []string{"memory: extraction model call failed: " + err.Error()}
	}
	if resp == nil {
		return 0, []string{"memory: extraction model returned no response"}
	}
	items, err := parseExtractedMemories(resp.Content, m.params.Int("memory.extractMaxItems"))
	if err != nil {
		return 0, []string{"memory: cannot parse the extraction response: " + err.Error()}
	}
	agentKind, err := llm.ParseAgentKind(session.AgentKind)
	if err != nil {
		return 0, []string{"memory: extraction skipped: " + err.Error()}
	}
	count := 0
	var warnings []string
	for _, item := range items {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		// Kind 缺失按 fact 兜底（llm.ParseMemoryKind），非法取值跳过该条并记 warning。
		kind, kerr := llm.ParseMemoryKind(llm.MemoryKind(item.Kind))
		if kerr != nil {
			warnings = append(warnings, "memory: extraction skipped an item with an invalid kind: "+item.Kind)
			continue
		}
		if _, werr := m.store.Write(ctx, tools.MemoryWriteInput{
			NamespaceName: session.NamespaceName,
			AgentKind:     agentKind,
			SessionID:     session.ID,
			Kind:          kind,
			Title:         strings.TrimSpace(item.Title),
			Content:       content,
		}); werr != nil {
			warnings = append(warnings, "memory: extraction write failed: "+werr.Error())
			continue
		}
		count++
	}
	return count, warnings
}

// resolveFlashChat 经 resolver 解析 flash 档位的对话客户端；resolver 为 nil、档位未绑定或解析
// 失败均返回英文错误（滚动摘要与记忆抽取共用低档位，绝不静默切换到其他档位）。
func (m *Memory) resolveFlashChat() (model.BaseChatModel, error) {
	if m.resolver == nil {
		return nil, errors.New("memory: nil chat resolver")
	}
	chat, err := m.resolver(llm.SlotFlash)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if chat == nil {
		return nil, errors.Newf("memory: no chat model resolved for slot %q", llm.SlotFlash)
	}
	return chat, nil
}

// extractedMemory 是压缩时模型抽取的一条长期记忆（JSON 形状）。
type extractedMemory struct {
	// Kind 记忆类别（可缺失，缺省 fact）
	Kind string `json:"kind"`
	// Title 记忆标题（可缺失）
	Title string `json:"title"`
	// Content 记忆正文
	Content string `json:"content"`
}

// extractionPayload 是抽取结果的 JSON 形状：{"memories":[{"kind","title","content"}]}。
type extractionPayload struct {
	Memories []extractedMemory `json:"memories"`
}

// parseExtractedMemories 容错解析抽取结果：先去掉 ```json 代码围栏，再按 {"memories":[...]} 解析，
// 失败时退回裸数组解析；条数超过 max（max>0 时）则截断。空白响应返回英文错误。
func parseExtractedMemories(content string, max int) ([]extractedMemory, error) {
	trimmed := stripJSONFence(content)
	if strings.TrimSpace(trimmed) == "" {
		return nil, errors.New("memory: extraction response is empty")
	}
	var payload extractionPayload
	var items []extractedMemory
	if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
		items = payload.Memories
	} else if err := json.Unmarshal([]byte(trimmed), &items); err != nil {
		return nil, errors.Newf("memory: cannot parse the extraction response as a memory list: %q", truncateText(trimmed, 200))
	}
	if max > 0 && len(items) > max {
		items = items[:max]
	}
	return items, nil
}

// buildSummaryPrompt 构造滚动摘要请求的用户消息：先给出既有摘要（无则标注 (none)），再逐条
// 列出被挤出消息（role: content，单条按 perMessageByteLimit 截断，控制 prompt 体积）。
func buildSummaryPrompt(existing string, evicted []*ai.AgentMessage) string {
	var sb strings.Builder
	sb.WriteString("Existing summary:\n")
	if strings.TrimSpace(existing) == "" {
		sb.WriteString("(none)\n")
	} else {
		sb.WriteString(existing)
		sb.WriteString("\n")
	}
	sb.WriteString("\nNew messages to fold into the summary:\n")
	sb.WriteString(buildMessageTranscript(evicted, summaryEvictedMessageByteLimit))
	return sb.String()
}

// buildMessageTranscript 把消息逐条拼成 "role: content" 的文本块，单条正文按 perMessageByteLimit
// 字节截断（UTF-8 边界安全），供摘要与记忆抽取提示词复用。
func buildMessageTranscript(messages []*ai.AgentMessage, perMessageByteLimit int) string {
	var sb strings.Builder
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		sb.WriteString(string(msg.Role))
		sb.WriteString(": ")
		sb.WriteString(truncateBytes(strings.TrimSpace(msg.Content), perMessageByteLimit))
		sb.WriteString("\n")
	}
	return sb.String()
}
