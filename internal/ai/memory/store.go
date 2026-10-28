// 本文件实现长期记忆（agent_memories）的存储与检索：按「命名空间 + Agent 类型 + 会话」归属与
// 隔离（记忆只在其来源会话内可见），写入时做「规范化正文带密钥 HMAC-SHA256 精确去重 + pgvector
// 余弦相似度语义合并」并同步向量列与全文检索列，检索时做「向量路 + 关键词路」混合召回、RRF
// 融合、按参数阈值内容去重后取 topK，命中异步回写使用统计。记忆正文经 crypto 序列化器加密存储
// （列类型 BYTEA）。检索结果的去重复用知识库同包的文本 n-gram Jaccard 逻辑（与写入的余弦去重
// 是两回事，勿混用）。

package memory

import (
	// 标准库
	"context"
	"strconv"
	"strings"
	"time"

	// 第三方
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/config"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/port"
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	"zhongkuisec.com/zhongkui/internal/ai/tools"
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// approxBytesPerToken 估算压缩后占用时用于「字节 → token」折算的经验比：中英文混合的提示词大致
// 每 3 字节 1 token（英文偏 4、中文偏 2）。该折算只服务压缩完成即刻展示的估算，后续生成由供应商
// 测得的真实 prompt tokens 到达后即被覆盖（见 session.Store.ContextUsage）。
const approxBytesPerToken = 3

// SessionTopicByteLimit 是会话短主题的字节上限（按 UTF-8 rune 边界截断，不产生残缺字符）。
const SessionTopicByteLimit = 64

// memoryReindexBatchSize 是长期记忆重嵌入时单批提交的最大条数：避免一次性向量化全部记忆导致
// 单请求过大，同时让进度上报以批为粒度推进。
const memoryReindexBatchSize = 20

// ErrMemoryNotFound 表示在指定命名空间下未找到目标记忆。
var ErrMemoryNotFound = errors.New("memory: not found")

// Store 是长期记忆的存储与检索入口：持有一个 gorm DB、向量化客户端、行为参数解析器与
// 日志记录器，负责记忆的 CRUD、向量/全文混合检索与去重落库。
type Store struct {
	// db core 库连接（agent_memories 读写）。
	db *database.DB
	// embed 向量化客户端（把记忆检索文本转为向量）。
	embed port.Embedder
	// params 行为参数解析器（去重阈值、候选数、topK 与截断上限）。
	params *config.Params
	logger *logging.Logger // 可空：为 nil 时跳过告警日志
}

// NewMemoryStore 构造 Store；db/embed/params 任一为空时返回英文错误，logger 允许为 nil
// （内部判空后再用）。
func NewStore(db *database.DB, embed port.Embedder, params *config.Params, logger *logging.Logger) (*Store, error) {
	if db == nil {
		return nil, errors.New("memory: nil database")
	}
	if embed == nil {
		return nil, errors.New("memory: nil embedder")
	}
	if params == nil {
		return nil, errors.New("memory: nil params")
	}
	return &Store{db: db, embed: embed, params: params, logger: logger}, nil
}

// MemoryQuery 是记忆列表的过滤与分页条件。
type MemoryQuery struct {
	// NamespaceName 限定命名空间（必填，非空）
	NamespaceName string
	// UserID 限定属主用户（users.id），用于跨用户隔离
	UserID uint
	// AgentKind 按 Agent 类型过滤（非空时生效）
	AgentKind ai.AgentKind
	// Kind 按记忆类别过滤（非空时生效）
	Kind ai.MemoryKind
	// Status 按记忆状态过滤（非空时生效）
	Status ai.MemoryStatus
	// Keyword 按标题/正文模糊匹配（非空时生效）
	Keyword string
	// Offset 分页偏移（小于 0 时取 0）
	Offset int
	// Limit 每页条数（小于等于 0 时取默认 20）
	Limit int
}

// defaultMemoryListLimit 是记忆列表的默认每页条数。
const defaultMemoryListLimit = 20

// 记忆检索 SQL 行：内嵌 AgentMemory 以复用其字段的 crypto 序列化器标签（本地结构体的
// 同名字段无标签，会把原始密文字节扫入 string，静默得到密文而非明文），查询仅 Select 检索
// 所需列，避免载入 embedding 大字段。
type memorySearchRow struct {
	ai.AgentMemory
	// Score 融合前的相关度分数（向量余弦相似度或 ts_rank）
	Score float64
}

// memoryReindexRow 是记忆重嵌入的读取行：内嵌 AgentMemory 以复用其字段的 crypto 序列化器标签
// （本地结构体的同名字段无标签，会把原始密文字节扫入 string，静默得到密文而非明文）。
type memoryReindexRow struct {
	ai.AgentMemory
}

// Write 写入一条长期记忆：先做规范化正文的带密钥 HMAC-SHA256 精确去重，未命中再按 pgvector
// 余弦相似度做语义合并，仍无则新增；三条路径均在同一事务内完成并同步 search/embedding 列。
// 精确去重与语义合并都命中同一行时刷新该行（正文/标题/类别/向量/全文表示/状态/时间）而非新增。
// 记忆按「命名空间 + Agent 类型 + 会话」归属与隔离，故 SessionID 为 0（无会话运行）时拒绝写入，
// 避免落进任何会话都检索不到的孤儿桶。
func (s *Store) Write(ctx context.Context, in tools.MemoryWriteInput) (*ai.AgentMemory, error) {
	namespaceName := strings.TrimSpace(in.NamespaceName)
	if namespaceName == "" {
		return nil, errors.New("memory: empty namespace name")
	}
	if in.SessionID == 0 {
		return nil, errors.New("memory: empty session id, long-term memory requires a session")
	}
	content := NormalizeMemoryContent(in.Content)
	if content == "" {
		return nil, errors.New("memory: empty content")
	}
	agentKind, err := ai.ParseAgentKind(in.AgentKind)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	kind, err := ai.ParseMemoryKind(in.Kind)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	title := strings.TrimSpace(in.Title)
	// 内容摘要为带密钥的 HMAC-SHA256：明文不落库，同时摘要确定可比，供精确去重与唯一索引使用
	hash, err := crypto.DigestForField(&ai.AgentMemory{}, "hash", "json", []byte(content))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// 检索文本 = 标题 + 正文（与知识库同约定：to_tsvector('simple', 该文本)）
	searchText := MemorySearchText(title, content)
	vectors, err := s.embed.EmbedStrings(ctx, []string{searchText})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if len(vectors) == 0 {
		return nil, errors.New("memory: embedder returned no vector")
	}
	lit := retrieval.VectorLiteral(vectors[0])
	// 新增与刷新共用的字段载体：content 经 crypto 序列化器加密（结构体路径才走序列化器，
	// map 更新会把明文直写列，故刷新一律走结构体 Select 更新）。
	values := &ai.AgentMemory{
		NamespaceName: namespaceName,
		UserID:        in.UserID,
		AgentKind:     agentKind,
		SessionID:     in.SessionID,
		Kind:          kind,
		Title:         title,
		Content:       serializer.NewCrypto([]byte(content)),
		Embedding:     retrieval.ToFloat32(vectors[0]),
		Hash:          serializer.NewHashDigest(hash),
		Status:        ai.MemoryStatusActive,
	}

	var out *ai.AgentMemory
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1) 精确去重：按唯一索引四列查既有行（含非 active 与被软删除的行，后者一并复活，
		// 避免唯一约束在并发/复活场景下报冲突）
		var existing ai.AgentMemory
		findErr := tx.Unscoped().
			Where("namespace_name = ? AND user_id = ? AND agent_kind = ? AND session_id = ? AND hash = ?",
				namespaceName, in.UserID, agentKind, in.SessionID, hash).
			Order("id ASC").First(&existing).Error
		if findErr == nil {
			if err := s.refreshMemory(tx, existing.ID, in.UserID, true, values, searchText); err != nil {
				return errors.WithStack(err)
			}
			return s.reload(tx, namespaceName, in.UserID, existing.ID, &out)
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
			Where("namespace_name = ? AND user_id = ? AND agent_kind = ? AND session_id = ? AND status = ? AND deleted_at IS NULL",
				namespaceName, in.UserID, agentKind, in.SessionID, ai.MemoryStatusActive).
			Order(clause.OrderBy{Expression: gorm.Expr("embedding <=> ?::vector", lit)}).
			Limit(1).Scan(&nearest).Error; err != nil {
			return errors.WithStack(err)
		}
		if nearest.ID != 0 && 1-nearest.Distance >= s.params.Float("memory.dedupeThreshold") {
			if err := s.refreshMemory(tx, nearest.ID, in.UserID, false, values, searchText); err != nil {
				return errors.WithStack(err)
			}
			return s.reload(tx, namespaceName, in.UserID, nearest.ID, &out)
		}

		// 3) 新增：唯一索引并发冲突时不报错（DoNothing），改为反查既有行并刷新（同一 hash 即同一
		// 规范化正文），与上面两条刷新路径共用同一字段载体与序列化链路。
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "namespace_name"},
				{Name: "user_id"},
				{Name: "agent_kind"},
				{Name: "session_id"},
				{Name: "hash"},
			},
			DoNothing: true,
		}).Create(values).Error; err != nil {
			return errors.WithStack(err)
		}
		id := values.ID
		if id == 0 {
			// 冲突改走刷新时部分方言不回填主键，按唯一索引四列反查
			var got ai.AgentMemory
			if err := tx.Unscoped().Where("namespace_name = ? AND user_id = ? AND agent_kind = ? AND session_id = ? AND hash = ?",
				namespaceName, in.UserID, agentKind, in.SessionID, hash).First(&got).Error; err != nil {
				return errors.WithStack(err)
			}
			if err := s.refreshMemory(tx, got.ID, in.UserID, true, values, searchText); err != nil {
				return errors.WithStack(err)
			}
			return s.reload(tx, namespaceName, in.UserID, got.ID, &out)
		}
		if err := s.writeMemorySearch(tx, id, searchText); err != nil {
			return errors.WithStack(err)
		}
		return s.reload(tx, namespaceName, in.UserID, id, &out)
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return out, nil
}

// ReindexEmbeddings 以当前 embedding 模型为准，重算命名空间下全部 active 长期记忆的向量表示：
// 读取标题与正文（经 crypto 序列化器解密），按 MemorySearchText 约定拼出检索文本并按
// memoryReindexBatchSize 分批向量化（经 embedStringsWithUsage 同时取回 token 用量），再逐行更新
// embedding 列。记忆正文与标题未变，故不改 content 与 search（全文检索）列，仅让向量与当前模型
// 对齐。供更换 embedding 模型（维度或语义变化）后使用；返回实际重算条数，中途失败即返回已完成
// 条数与错误。report 为进度上报入口（为 nil 时不上报）：查询后立即上报一次 Start(记忆总数)
// （空命名空间亦上报 Start(0)），每批成功后上报一次 Step（Action=contract.SyncActionMemory，Title 为空），
// 结束（含提前失败）时经 defer 上报一次 Finish。namespaceName 为空时返回英文错误。
func (s *Store) ReindexEmbeddings(ctx context.Context, namespaceName string, report contract.SyncReporter) (int, error) {
	namespaceName = strings.TrimSpace(namespaceName)
	if namespaceName == "" {
		return 0, errors.New("memory: empty namespace name")
	}
	var rows []memoryReindexRow
	if err := s.db.WithContext(ctx).Table("agent_memories").
		Select("id", "title", "content").
		Where("namespace_name = ? AND status = ? AND deleted_at IS NULL", namespaceName, ai.MemoryStatusActive).
		Order("id ASC").
		Scan(&rows).Error; err != nil {
		return 0, errors.WithStack(err)
	}
	contract.ReportStart(report, len(rows))
	defer contract.ReportFinish(report)
	if len(rows) == 0 {
		return 0, nil
	}
	for start := 0; start < len(rows); start += memoryReindexBatchSize {
		end := start + memoryReindexBatchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[start:end]
		batchStart := time.Now()
		texts := make([]string, len(batch))
		for i, row := range batch {
			texts[i] = MemorySearchText(row.Title, string(row.Content.Value()))
		}
		vectors, usage, err := port.EmbedStringsWithUsage(ctx, s.embed, texts)
		if err != nil {
			return start, errors.WithStack(err)
		}
		if len(vectors) != len(batch) {
			return start, errors.Newf("memory: embedder returned %d vectors for %d memories", len(vectors), len(rows))
		}
		for i, row := range batch {
			// 采用结构体 + Select 更新，保证 embedding 字段经 serializer 落库
			if err := s.db.WithContext(ctx).Model(&ai.AgentMemory{}).Where("id = ?", row.ID).
				Select("embedding").
				Updates(&ai.AgentMemory{Embedding: retrieval.ToFloat32(vectors[i])}).Error; err != nil {
				return start + i, errors.WithStack(err)
			}
		}
		contract.ReportStep(report, contract.SyncProgressItem{
			Action:    contract.SyncActionMemory,
			Count:     len(batch),
			ElapsedMs: time.Since(batchStart).Milliseconds(),
			Usage:     usage,
		})
	}
	return len(rows), nil
}

// Retrieve 执行长期记忆的混合检索：向量路 + 关键词路各取 memory.candidates 条 → RRF 融合 →
// 按 memory.dedupeThreshold 内容去重 → 取 memory.topK，再受 memory.maxItems 约束；限定
// namespace + agent_kind + session_id + status='active'，即记忆只在其来源会话内可见。
// memory.enabled=false、命名空间为空、会话为 0 或查询空白时返回 (nil, nil) 且不报错。
// 命中后异步回写使用统计（失败仅告警，不影响返回）。
func (s *Store) Retrieve(ctx context.Context, namespaceName string, agentKind ai.AgentKind, sessionID, userID uint, query string) ([]contract.MemoryHit, error) {
	if !s.params.Bool("memory.enabled") {
		return nil, nil
	}
	namespaceName = strings.TrimSpace(namespaceName)
	if namespaceName == "" || sessionID == 0 || strings.TrimSpace(query) == "" {
		return nil, nil
	}
	agentKind, err := ai.ParseAgentKind(agentKind)
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
	vecHits, vecKinds, err := s.vectorSearch(ctx, namespaceName, agentKind, sessionID, userID, vectors[0], candidates)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	kwHits, kwKinds, err := s.keywordSearch(ctx, namespaceName, agentKind, sessionID, userID, query, candidates)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	fused := retrieval.FuseRRF(vecHits, kwHits)
	fused = retrieval.DedupeSimilarResultsWithCutoff(fused, s.params.Float("memory.dedupeThreshold"))
	fused = retrieval.SelectTopK(fused, s.params.Int("memory.topK"))
	if maxItems := s.params.Int("memory.maxItems"); maxItems > 0 && len(fused) > maxItems {
		fused = fused[:maxItems]
	}
	maxBytes := s.params.Int("memory.maxItemBytes")
	hits := make([]contract.MemoryHit, 0, len(fused))
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
		hits = append(hits, contract.MemoryHit{
			ID:      uint(id64),
			Kind:    kind,
			Title:   hit.Title,
			Content: retrieval.TruncateBytes(hit.Content, maxBytes),
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
func (s *Store) TouchUsage(ctx context.Context, ids []uint) error {
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

// Get 读取指定命名空间下、属于该用户的一条记忆；不存在时返回 ErrMemoryNotFound。
func (s *Store) Get(ctx context.Context, namespaceName string, userID uint, id uint) (*ai.AgentMemory, error) {
	namespaceName = strings.TrimSpace(namespaceName)
	if namespaceName == "" {
		return nil, errors.New("memory: empty namespace name")
	}
	var mem ai.AgentMemory
	if err := s.db.WithContext(ctx).
		Where("namespace_name = ? AND user_id = ? AND id = ?", namespaceName, userID, id).
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
func (s *Store) List(ctx context.Context, q MemoryQuery) ([]*ai.AgentMemory, int64, error) {
	namespaceName := strings.TrimSpace(q.NamespaceName)
	if namespaceName == "" {
		return nil, 0, errors.New("memory: empty namespace name")
	}
	base := func() *gorm.DB {
		tx := s.db.WithContext(ctx).Model(&ai.AgentMemory{}).Where("namespace_name = ? AND user_id = ?", namespaceName, q.UserID)
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
			// 正文已加密存储，无法再做 ILIKE 明文匹配，改走 search 列（tsvector）的关键词命中；
			// 标题仍为明文，保留子串匹配。
			tx = tx.Where("title ILIKE ? OR search @@ plainto_tsquery('simple', ?)", like, keyword)
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

// Delete 软删除指定命名空间下、属于该用户的若干记忆；ids 为空直接返回 nil。
func (s *Store) Delete(ctx context.Context, namespaceName string, userID uint, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	namespaceName = strings.TrimSpace(namespaceName)
	if namespaceName == "" {
		return errors.New("memory: empty namespace name")
	}
	if err := s.db.WithContext(ctx).
		Where("namespace_name = ? AND user_id = ? AND id IN ?", namespaceName, userID, ids).
		Delete(&ai.AgentMemory{}).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// SetStatus 设置指定命名空间下、属于该用户的一条记忆的状态；status 不在取值空间内时返回英文错误。
func (s *Store) SetStatus(ctx context.Context, namespaceName string, userID uint, id uint, status ai.MemoryStatus) error {
	if _, err := ai.ParseMemoryStatus(status); err != nil {
		return errors.WithStack(err)
	}
	namespaceName = strings.TrimSpace(namespaceName)
	if namespaceName == "" {
		return errors.New("memory: empty namespace name")
	}
	res := s.db.WithContext(ctx).Model(&ai.AgentMemory{}).
		Where("namespace_name = ? AND user_id = ? AND id = ?", namespaceName, userID, id).
		Update("status", string(status))
	if res.Error != nil {
		return errors.WithStack(res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrMemoryNotFound
	}
	return nil
}

// memoryStoreWriter 把 Store 适配为 internal/ai/tools 的写入器接口：该包的
// memory_write 工具在工具循环中经 ctx 取回本适配器，把模型给出的内容落库。适配器的入参即
// tools.MemoryWriteInput，直接透传给 Store.Write，规范化、去重与语义合并全部由其承担。
// 适配器声明在本包内是因为 internal/ai/tools 不能引用本包（导入环），故由本包单向满足其接口。
type memoryStoreWriter struct {
	// store 底层长期记忆存储（nil 时 WriteMemory 返回英文错误）
	store *Store
}

// WriteMemory 落库一条由 memory_write 工具提交的记忆并返回其 ID：store 缺失、命名空间为空或
// 内容为空时返回英文错误，其余校验与落库语义完全委托 Store.Write。
func (w memoryStoreWriter) WriteMemory(ctx context.Context, in tools.MemoryWriteInput) (uint, error) {
	if w.store == nil {
		return 0, errors.New("memory: nil store")
	}
	mem, err := w.store.Write(ctx, in)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return mem.ID, nil
}

// vectorSearch 执行向量路检索：在「命名空间 + Agent 类型 + 会话」范围内按余弦距离升序取前
// limit 条 active 记忆，返回检索结果与 DocumentID → 记忆类别的映射。
func (s *Store) vectorSearch(ctx context.Context, namespaceName string, agentKind ai.AgentKind, sessionID, userID uint, vec []float64, limit int) ([]*retrieval.RetrievalResult, map[string]ai.MemoryKind, error) {
	if limit <= 0 || len(vec) == 0 {
		return nil, nil, nil
	}
	lit := retrieval.VectorLiteral(vec)
	var rows []memorySearchRow
	if err := s.db.WithContext(ctx).Table("agent_memories").
		Select("id, kind, title, content, (1 - (embedding <=> ?::vector)) AS score", lit).
		Where("namespace_name = ? AND user_id = ? AND agent_kind = ? AND session_id = ? AND status = ? AND deleted_at IS NULL",
			namespaceName, userID, agentKind, sessionID, ai.MemoryStatusActive).
		Order(clause.OrderBy{Expression: gorm.Expr("embedding <=> ?::vector", lit)}).
		Limit(limit).Scan(&rows).Error; err != nil {
		return nil, nil, errors.WithStack(err)
	}
	hits, kinds := memoryRowsToResults(rows)
	return hits, kinds, nil
}

// keywordSearch 执行关键词路检索：在「命名空间 + Agent 类型 + 会话」范围内以
// search @@ plainto_tsquery 过滤，按 ts_rank 降序取前 limit 条。
func (s *Store) keywordSearch(ctx context.Context, namespaceName string, agentKind ai.AgentKind, sessionID, userID uint, query string, limit int) ([]*retrieval.RetrievalResult, map[string]ai.MemoryKind, error) {
	if limit <= 0 {
		return nil, nil, nil
	}
	var rows []memorySearchRow
	if err := s.db.WithContext(ctx).Table("agent_memories").
		Select("id, kind, title, content, ts_rank(search, plainto_tsquery('simple', ?)) AS score", query).
		Where("namespace_name = ? AND user_id = ? AND agent_kind = ? AND session_id = ? AND status = ? AND deleted_at IS NULL",
			namespaceName, userID, agentKind, sessionID, ai.MemoryStatusActive).
		Where("search @@ plainto_tsquery('simple', ?)", query).
		Order(clause.OrderBy{Expression: gorm.Expr("ts_rank(search, plainto_tsquery('simple', ?)) DESC", query)}).
		Limit(limit).Scan(&rows).Error; err != nil {
		return nil, nil, errors.WithStack(err)
	}
	hits, kinds := memoryRowsToResults(rows)
	return hits, kinds, nil
}

// refreshMemory 刷新既有记忆行：以结构体 Select 更新给定字段并重写 search 列。采用结构体
// 而非 map 是为了触发 crypto 序列化器加密 content（map 更新走 clause.Assignment 直写，会把
// 明文落库）。unscoped 为 true 时同时复活被软删除的行（软删除行的唯一 hash 仍占用唯一索引，
// 复活可避免唯一约束冲突）。
func (s *Store) refreshMemory(tx *gorm.DB, id uint, userID uint, unscoped bool, values *ai.AgentMemory, searchText string) error {
	q := tx.Model(&ai.AgentMemory{}).Where("id = ? AND user_id = ?", id, userID)
	if unscoped {
		q = tx.Unscoped().Model(&ai.AgentMemory{}).Where("id = ? AND user_id = ?", id, userID)
	}
	if err := q.
		Select("session_id", "kind", "title", "content", "embedding", "hash", "status", "deleted_at").
		Updates(values).Error; err != nil {
		return errors.WithStack(err)
	}
	return s.writeMemorySearch(tx, id, searchText)
}

// writeMemorySearch 经 to_tsvector('simple', ...) SQL 表达式写入 search 列（tsvector 列不能经
// 模型直接赋值）。
func (s *Store) writeMemorySearch(tx *gorm.DB, id uint, searchText string) error {
	if err := tx.Model(&ai.AgentMemory{}).Where("id = ?", id).
		Update("search", gorm.Expr("to_tsvector('simple', ?)", searchText)).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// reload 按命名空间 + 属主用户 + ID 重新读取行到 out。
func (s *Store) reload(tx *gorm.DB, namespaceName string, userID uint, id uint, out **ai.AgentMemory) error {
	var got ai.AgentMemory
	if err := tx.Where("namespace_name = ? AND user_id = ? AND id = ?", namespaceName, userID, id).First(&got).Error; err != nil {
		return errors.WithStack(err)
	}
	*out = &got
	return nil
}

// warnf 在 logger 非空时输出告警。
func (s *Store) warnf(ctx context.Context, format string, args ...any) {
	if s.logger == nil {
		return
	}
	s.logger.Ctx(ctx).Warnf(format, args...)
}

// memoryRowsToResults 将检索 SQL 行转换为检索结果，并建立 DocumentID → 记忆类别的映射，
// 供融合后还原记忆类别（retrieval.RetrievalResult 本身不携带类别）。
func memoryRowsToResults(rows []memorySearchRow) ([]*retrieval.RetrievalResult, map[string]ai.MemoryKind) {
	hits := make([]*retrieval.RetrievalResult, 0, len(rows))
	kinds := make(map[string]ai.MemoryKind, len(rows))
	for _, row := range rows {
		id := strconv.FormatUint(uint64(row.ID), 10)
		hits = append(hits, &retrieval.RetrievalResult{
			DocumentID: id,
			Title:      row.Title,
			Content:    string(row.Content.Value()),
			Score:      row.Score,
		})
		kinds[id] = row.Kind
	}
	return hits, kinds
}

// MemorySearchText 计算记忆的全文检索文本：标题 + 正文（与知识库同一约定）。
func MemorySearchText(title, content string) string {
	return strings.TrimSpace(title + "\n" + content)
}

// NormalizeMemoryContent 规范化记忆正文：折叠全部空白串为单个空格并去除首尾空白，使排版差异
// 不影响带密钥 HMAC-SHA256 摘要的精确去重。
func NormalizeMemoryContent(content string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(content), " "))
}
