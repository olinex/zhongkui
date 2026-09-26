package ai

import (
	// stdlib
	"fmt"
	"net/http"
	"strconv"
	"time"

	// third-party
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	// internal
	ai "zhongkui.com/zhongkui/internal/ai"
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/manager/views"
	modelsai "zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/llm"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// KnowledgeDocumentItemOut 定义知识库文档列表的输出项结构（独立于模型层）。
type KnowledgeDocumentItemOut struct {
	// ID 文档 ID
	ID uint `json:"id"`
	// Title 文档标题
	Title string `json:"title"`
	// ChunkCount 文档被切分的知识分块数量
	ChunkCount int `json:"chunkCount"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updatedAt"`
}

// KnowledgeDocumentListOut 定义知识库文档列表的输出项，嵌入 BaseTotalOut 支持分页总数。
type KnowledgeDocumentListOut struct {
	KnowledgeDocumentItemOut
	utils.BaseTotalOut
}

// KnowledgeDocumentCreateIn 定义创建知识库文档的请求输入。
type KnowledgeDocumentCreateIn struct {
	// Title 文档标题，必填
	Title string `json:"title" binding:"required"`
	// Content 文档正文内容，必填
	Content string `json:"content" binding:"required"`
}

// KnowledgeDocumentUpdateIn 定义更新知识库文档的请求输入。
type KnowledgeDocumentUpdateIn struct {
	// ID 文档 ID，必填
	ID uint `json:"id" binding:"required"`
	// Title 文档标题，必填
	Title string `json:"title" binding:"required"`
	// Content 文档正文内容，必填
	Content string `json:"content" binding:"required"`
}

// KnowledgeDocumentDetailOut 定义知识库文档详情的输出结构。
type KnowledgeDocumentDetailOut struct {
	// ID 文档 ID
	ID uint `json:"id"`
	// Title 文档标题
	Title string `json:"title"`
	// Content 文档正文内容
	Content string `json:"content"`
	// ChunkCount 文档被切分的知识分块数量
	ChunkCount int `json:"chunkCount"`
	// Hash 文档内容的 sha256 十六进制摘要
	Hash string `json:"hash"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updatedAt"`
}

// KnowledgeDocumentReindexOut 定义重新索引的输出结构。
type KnowledgeDocumentReindexOut struct {
	// ID 文档 ID
	ID uint `json:"id"`
	// ChunkCount 重新索引后的知识分块数量
	ChunkCount int `json:"chunkCount"`
	// Hash 重新索引后的内容摘要
	Hash string `json:"hash"`
}

// KnowledgeDocumentBatchIn 批量删除知识库文档的请求输入结构。
// Ids 接受标量（单个 ID）或数组（多个 ID）。
type KnowledgeDocumentBatchIn struct {
	// Ids 待批量删除的文档 ID 列表
	Ids views.FlexList[uint] `json:"ids"`
}

// KnowledgeDocumentListGet 处理获取知识库文档列表的请求。
// 基于统一搜索基座 BuildSearchCountQuery 支持分页、排序与过滤（标题/创建/更新时间），
// 仅返回当前命名空间内的文档。
func KnowledgeDocumentListGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in views.KnowledgeDocumentListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	var items []*KnowledgeDocumentListOut
	if query, err := utils.BuildSearchCountQuery(
		middlewares.GetCoreDatabase(c).Model(&modelsai.KnowledgeDocument{}).
			Where("namespace_name = ?", namespace), &in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&items).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 字段投影：按请求 fields 投影，未指定时回退全部白名单字段（封装于 utils.ProjectSearchItems）
	var total uint
	if len(items) > 0 {
		total = items[0].GetTotal()
	}
	projected := utils.ProjectSearchItems(items, &in, func(o **KnowledgeDocumentListOut) any {
		return *o
	})
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(total, projected))
}

// KnowledgeDocumentCreate 处理创建知识库文档的请求。
// 先落库保存文档，随后调用知识库索引将其切块向量化；AI 功能未启用时返回 400，
// 文档保留（chunkCount=0），待启用后经 reindex 接口重建索引。
func KnowledgeDocumentCreate(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in KnowledgeDocumentCreateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	doc := &modelsai.KnowledgeDocument{
		NamespaceName: namespace,
		Title:         in.Title,
		Content:       in.Content,
	}
	if err := middlewares.GetCoreDatabase(c).WithContext(c).Create(doc).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	comps := middlewares.GetAi(c)
	if comps.Knowledge == nil {
		response.CreateCodeError(http.StatusBadRequest, response.CodeBadBody, errors.New("ai feature is disabled")).AbortWithGinH(c)
		return
	}
	if err := comps.Knowledge.IndexDocument(c, doc); err != nil {
		// 索引失败不删除文档：文档保留（chunkCount=0），可稍后重新索引
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// KnowledgeDocumentDetailGet 处理获取知识库文档详情的请求。
// 仅允许访问当前命名空间内的文档，不存在时返回 404。
func KnowledgeDocumentDetailGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	instance := modelsai.KnowledgeDocument{}
	if err := middlewares.GetCoreDatabase(c).WithContext(c).
		Where("id = ? AND namespace_name = ?", id, namespace).
		First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, KnowledgeDocumentDetailOut{
		ID:         instance.ID,
		Title:      instance.Title,
		Content:    instance.Content,
		ChunkCount: instance.ChunkCount,
		Hash:       instance.Hash,
		CreatedAt:  instance.CreatedAt,
		UpdatedAt:  instance.UpdatedAt,
	})
}

// KnowledgeDocumentUpdate 处理更新知识库文档的请求。
// 按 (id, namespace) 定位文档，不存在返回 404；更新标题/正文后触发重新索引，
// 使分块与向量随内容变更重建。AI 功能未启用时仅更新文档（索引保持原状）。
func KnowledgeDocumentUpdate(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in KnowledgeDocumentUpdateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	db := middlewares.GetCoreDatabase(c)
	instance := modelsai.KnowledgeDocument{}
	if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", in.ID, namespace).First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := db.WithContext(c).Model(&modelsai.KnowledgeDocument{}).
		Where("id = ? AND namespace_name = ?", in.ID, namespace).
		Updates(map[string]any{"title": in.Title, "content": in.Content}).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	comps := middlewares.GetAi(c)
	if comps.Knowledge == nil {
		response.SimpleOK.AbortWithGinH(c)
		return
	}
	// 直接构造带 ID 的文档指针触发重新索引（Reindex 仅使用 ID/NamespaceName/Title/Content）
	doc := &modelsai.KnowledgeDocument{
		Model:         gorm.Model{ID: in.ID},
		NamespaceName: namespace,
		Title:         in.Title,
		Content:       in.Content,
	}
	if err := comps.Knowledge.Reindex(c, doc); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// KnowledgeDocumentBatchDelete 批量删除知识库文档。
// 逐条按 (id, namespace) 确认文档存在（不存在记录失败明细），再经知识库索引删除
// 文档及其分块；AI 功能未启用时直接删除文档行（无索引可清理）。
func KnowledgeDocumentBatchDelete(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in KnowledgeDocumentBatchIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	ids := []uint(in.Ids)
	if !views.ValidFlexLen(c, ids) {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	comps := middlewares.GetAi(c)
	views.BatchExecute(c, ids,
		func(id uint) string { return strconv.FormatUint(uint64(id), 10) },
		func(id uint) error {
			instance := modelsai.KnowledgeDocument{}
			if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return fmt.Errorf("knowledge document %d not found", id)
				}
				return errors.WithStack(err)
			}
			if comps.Knowledge == nil {
				return errors.WithStack(db.WithContext(c).
					Where("id = ? AND namespace_name = ?", id, namespace).
					Delete(&modelsai.KnowledgeDocument{}).Error)
			}
			return errors.WithStack(comps.Knowledge.DeleteDocument(c, id))
		},
	)
}

// KnowledgeDocumentReindex 处理重新索引知识库文档的请求。
// 按 (id, namespace) 定位文档（不存在 404），重新切块向量化并返回最新分块数与内容摘要。
func KnowledgeDocumentReindex(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	comps := middlewares.GetAi(c)
	if comps.Knowledge == nil {
		response.CreateCodeError(http.StatusBadRequest, response.CodeBadBody, errors.New("ai feature is disabled")).AbortWithGinH(c)
		return
	}
	doc := modelsai.KnowledgeDocument{}
	if err := middlewares.GetCoreDatabase(c).WithContext(c).
		Where("id = ? AND namespace_name = ?", id, namespace).
		First(&doc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := comps.Knowledge.Reindex(c, &doc); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, KnowledgeDocumentReindexOut{
		ID:         doc.ID,
		ChunkCount: doc.ChunkCount,
		Hash:       doc.Hash,
	})
}

// KnowledgeDocumentIndexSummaryOut 定义知识库文档索引摘要的输出结构：
// 全部字段在读取时从文档与其分块聚合派生（含 overlap 造成的重复计数），不落库。
type KnowledgeDocumentIndexSummaryOut struct {
	// ChunkCount 分块数
	ChunkCount int `json:"chunkCount"`
	// ContentChars 文档正文字符数
	ContentChars int `json:"contentChars"`
	// ChunkCharsTotal 各分块正文字符数之和
	ChunkCharsTotal int `json:"chunkCharsTotal"`
	// ChunkCharsAvg 分块正文字符数均值
	ChunkCharsAvg int `json:"chunkCharsAvg"`
	// ChunkCharsMin 分块正文字符数最小值
	ChunkCharsMin int `json:"chunkCharsMin"`
	// ChunkCharsMax 分块正文字符数最大值
	ChunkCharsMax int `json:"chunkCharsMax"`
	// HeadingPathCount 去重后的标题路径数
	HeadingPathCount int `json:"headingPathCount"`
	// AtomicChunkCount 含代码块或表格的分块数（超长也不切分的原子块）
	AtomicChunkCount int `json:"atomicChunkCount"`
	// ChunkSize 生效参数 chunking.size
	ChunkSize int `json:"chunkSize"`
	// ChunkOverlap 生效参数 chunking.overlap
	ChunkOverlap int `json:"chunkOverlap"`
	// Indexed 是否已建立索引
	Indexed bool `json:"indexed"`
	// Stale 索引摘要与当前正文是否不一致（索引落后于正文）
	Stale bool `json:"stale"`
	// IndexedAt 最近一次索引写入时间（文档更新时间）
	IndexedAt time.Time `json:"indexedAt"`
	// Hash 文档内容 sha256 摘要
	Hash string `json:"hash"`
	// EmbeddingModel embedding 档位绑定的供应商模型名（未绑定时为空）
	EmbeddingModel string `json:"embeddingModel"`
	// EmbeddingDimensions embedding 列的实际向量维度（不可读时为 0）
	EmbeddingDimensions int `json:"embeddingDimensions"`
}

// KnowledgeDocumentChunkOut 定义单个知识分块的输出结构。
type KnowledgeDocumentChunkOut struct {
	// Index 分块在文档内的序号
	Index int `json:"index"`
	// HeadingPath 所属标题路径（空表示该块不在任何标题之下）
	HeadingPath string `json:"headingPath"`
	// Content 分块正文
	Content string `json:"content"`
	// Chars 正文字符数
	Chars int `json:"chars"`
	// Lines 正文行数
	Lines int `json:"lines"`
	// Kinds 结构单元类型（取值为 chunkUnitKind 枚举类的值）
	Kinds []string `json:"kinds"`
}

// KnowledgeDocumentChunksOut 定义知识库文档分块明细的输出结构（文档级摘要 + 逐块明细）。
type KnowledgeDocumentChunksOut struct {
	// Summary 文档级索引摘要
	Summary KnowledgeDocumentIndexSummaryOut `json:"summary"`
	// Chunks 逐块明细（按分块序号升序；无分块时为空数组）
	Chunks []KnowledgeDocumentChunkOut `json:"chunks"`
}

// KnowledgeDocumentChunksGet 处理获取知识库文档分块明细的请求。
// 按 (id, namespace) 定位文档（不存在返回 404），随后读取该文档分块并聚合构建摘要：
// 分块统计、切片参数、向量模型与维度、索引新鲜度均在此给出。读路径不依赖 AI 能力是否启用
// （未启用时文档通常未索引，返回空分块与 indexed=false）。
func KnowledgeDocumentChunksGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	doc := modelsai.KnowledgeDocument{}
	if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&doc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	overview, err := ai.BuildChunkOverview(c, db, middlewares.GetAi(c).Params, &doc)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	out := KnowledgeDocumentChunksOut{
		Summary: KnowledgeDocumentIndexSummaryOut{
			ChunkCount:       overview.Summary.ChunkCount,
			ContentChars:     overview.Summary.ContentChars,
			ChunkCharsTotal:  overview.Summary.ChunkCharsTotal,
			ChunkCharsAvg:    overview.Summary.ChunkCharsAvg,
			ChunkCharsMin:    overview.Summary.ChunkCharsMin,
			ChunkCharsMax:    overview.Summary.ChunkCharsMax,
			HeadingPathCount: overview.Summary.HeadingPathCount,
			AtomicChunkCount: overview.Summary.AtomicChunkCount,
			ChunkSize:        overview.Summary.ChunkSize,
			ChunkOverlap:     overview.Summary.ChunkOverlap,
			Indexed:          overview.Summary.Indexed,
			Stale:            overview.Summary.Stale,
			IndexedAt:        overview.Summary.IndexedAt,
			Hash:             overview.Summary.Hash,
		},
		// 空数组而非 null，保证前端拿到稳定结构
		Chunks: make([]KnowledgeDocumentChunkOut, 0, len(overview.Chunks)),
	}
	// 向量模型取自运行时快照的 embedding 档位绑定（未绑定时留空）
	if info, ok := middlewares.GetAiRuntime(c).Snapshot().Slots()[llm.SlotEmbedding]; ok {
		out.Summary.EmbeddingModel = info.Model
	}
	// 向量维度取 embedding 列的实际维度（表或列不可读时记 0），与状态接口共用同一读取来源
	if n, found, err := llm.ChunkEmbeddingDimension(c, db); err == nil && found {
		out.Summary.EmbeddingDimensions = n
	}
	for _, chunk := range overview.Chunks {
		kinds := make([]string, 0, len(chunk.Kinds))
		for _, kind := range chunk.Kinds {
			kinds = append(kinds, string(kind))
		}
		out.Chunks = append(out.Chunks, KnowledgeDocumentChunkOut{
			Index:       chunk.Index,
			HeadingPath: chunk.HeadingPath,
			Content:     chunk.Content,
			Chars:       chunk.Chars,
			Lines:       chunk.Lines,
			Kinds:       kinds,
		})
	}
	response.SimpleOK.AbortWithGinW(c, out)
}
