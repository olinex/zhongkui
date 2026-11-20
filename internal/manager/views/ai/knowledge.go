package ai

import (
	"zhongkuisec.com/zhongkui/internal/ai/llminfra"
	// 标准库
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	// 第三方
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	// 项目内部
	aiknowledge "zhongkuisec.com/zhongkui/internal/ai/knowledge"
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/manager/views"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/docconv"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/s3"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/pkg/utils"
	"zhongkuisec.com/zhongkui/internal/worker/handlers/knowledge"
)

// maxUploadBytes 文档导入请求体的上限（含 multipart 表单开销），64 MiB。
const maxUploadBytes = 64 << 20

// KnowledgeDocumentItemOut 定义知识库文档列表的输出项结构（独立于模型层）。
type KnowledgeDocumentItemOut struct {
	// ID 文档 ID
	ID uint `json:"id"`
	// Title 文档标题
	Title string `json:"title"`
	// NamespaceName 文档所属命名空间名（供前端按来源命名空间溯源跳转）
	NamespaceName string `json:"namespaceName"`
	// ChunkCount 文档被切分的知识分块数量
	ChunkCount int `json:"chunkCount"`
	// Format 文档来源格式（取值为 documentFormat 枚举类的值：markdown/txt/pdf/docx）
	Format string `json:"format"`
	// Status 文档处理状态（取值为 knowledgeDocStatus 枚举类的值：pending/parsing/ready/failed）
	Status string `json:"status"`
	// SizeBytes 原件字节数（手工录入的文档为 0）
	SizeBytes int64 `json:"sizeBytes"`
	// UseCount 该文档被检索命中的次数
	UseCount int `json:"useCount"`
	// LastUsedAt 最近一次被检索命中的时间，从未命中时为空
	LastUsedAt *time.Time `json:"lastUsedAt"`
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
	// Content 文档正文内容，可选：手动录入文档可修改（变更后同步重建索引），导入文档为只读
	// （携带非空内容会被显式拒绝），故不再强制必填
	Content string `json:"content"`
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
	// UseCount 该文档被检索命中的次数
	UseCount int `json:"useCount"`
	// LastUsedAt 最近一次被检索命中的时间，从未命中时为空
	LastUsedAt *time.Time `json:"lastUsedAt"`
	// Hash 文档内容的 HMAC-SHA256 十六进制摘要（带密钥，防离线爆破）
	Hash string `json:"hash"`
	// Format 文档来源格式（取值为 documentFormat 枚举类的值：markdown/txt/pdf/docx）
	Format string `json:"format"`
	// Status 文档处理状态（取值为 knowledgeDocStatus 枚举类的值：pending/parsing/ready/failed）
	Status string `json:"status"`
	// SizeBytes 原件字节数（手工录入的文档为 0）
	SizeBytes int64 `json:"sizeBytes"`
	// OriginalName 导入文档的原始文件名（手工录入的文档为空）
	OriginalName string `json:"originalName"`
	// ParseError 解析失败原因（解析成功或手工录入的文档为空）
	ParseError string `json:"parseError"`
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
	// Status 重新索引后的文档处理状态；导入文档异步重解析时为 pending，手动文档同步重建时为 ready
	Status string `json:"status"`
}

// KnowledgeDocumentUploadOut 定义导入知识库文档（multipart 上传）的输出结构。
// 客户端据此拿到新建文档 ID 与初始状态并开始轮询解析进度。
type KnowledgeDocumentUploadOut struct {
	// ID 新建文档 ID
	ID uint `json:"id"`
	// Status 文档初始处理状态（取值为 knowledgeDocStatus 枚举类的值，恒为 pending）
	Status string `json:"status"`
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

// KnowledgeSystemDocumentListGet 处理获取系统命名空间知识库文档列表的请求。
// 与 KnowledgeDocumentListGet 同构，唯一区别是数据范围恒为内置系统命名空间
// （base.SystemNamespace），不取自客户端命名空间，故为纯只读接口。
func KnowledgeSystemDocumentListGet(c *gin.Context) {
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
			Where("namespace_name = ?", base.SystemNamespace), &in); err != nil {
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
		// 正文经 crypto 序列化器加密落库（Create 走结构体路径，序列化器生效）
		Content: serializer.NewCrypto([]byte(in.Content)),
		// 手工录入文档固定为 markdown + ready；原件字段保持零值（无 S3 原件）
		Format: string(docconv.FormatMarkdown),
		Status: string(modelsai.KnowledgeDocStatusReady),
	}
	if err := middlewares.GetCoreDatabase(c).WithContext(c).Create(doc).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	comps := middlewares.GetAi(c)
	if comps.Knowledge == nil {
		response.CreateBadBodyMessageError("aiFeatureDisabled").AbortWithGinH(c)
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
		ID:           instance.ID,
		Title:        instance.Title,
		Content:      string(instance.Content.Value()),
		ChunkCount:   instance.ChunkCount,
		UseCount:     instance.UseCount,
		LastUsedAt:   instance.LastUsedAt,
		Hash:         instance.Hash.Digest(),
		Format:       instance.Format,
		Status:       instance.Status,
		SizeBytes:    instance.SizeBytes,
		OriginalName: instance.OriginalName,
		ParseError:   instance.ParseError,
		CreatedAt:    instance.CreatedAt,
		UpdatedAt:    instance.UpdatedAt,
	})
}

// KnowledgeSystemDocumentDetailGet 处理获取系统命名空间知识库文档详情的请求。
// 与 KnowledgeDocumentDetailGet 同构，数据范围恒为内置系统命名空间（base.SystemNamespace），
// 不存在时返回 404；纯只读接口。
func KnowledgeSystemDocumentDetailGet(c *gin.Context) {
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	instance := modelsai.KnowledgeDocument{}
	if err := middlewares.GetCoreDatabase(c).WithContext(c).
		Where("id = ? AND namespace_name = ?", id, base.SystemNamespace).
		First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, KnowledgeDocumentDetailOut{
		ID:           instance.ID,
		Title:        instance.Title,
		Content:      string(instance.Content.Value()),
		ChunkCount:   instance.ChunkCount,
		UseCount:     instance.UseCount,
		LastUsedAt:   instance.LastUsedAt,
		Hash:         instance.Hash.Digest(),
		Format:       instance.Format,
		Status:       instance.Status,
		SizeBytes:    instance.SizeBytes,
		OriginalName: instance.OriginalName,
		ParseError:   instance.ParseError,
		CreatedAt:    instance.CreatedAt,
		UpdatedAt:    instance.UpdatedAt,
	})
}

// KnowledgeDocumentUpdate 处理更新知识库文档的请求。
// 按 (id, namespace) 定位文档，不存在返回 404。按来源区分语义：
//   - 导入文档（ObjectKey 非空）：仅允许改标题，且标题变更不触发重新解析；请求携带非空
//     content 时显式返回 400（正文只读，不静默忽略）。
//   - 手动录入文档：标题/正文均可改，正文变更后同步重建索引（AI 未启用时仅更新文档）。
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
	// 导入文档：正文由解析流水线产生，只读；携带非空内容显式拒绝，标题变更不触发重新解析。
	if instance.ObjectKey.Key() != "" {
		if in.Content != "" {
			response.CreateBadBodyMessageError("knowledgeImportedContentReadOnly").AbortWithGinH(c)
			return
		}
		if err := db.WithContext(c).Model(&modelsai.KnowledgeDocument{}).
			Where("id = ? AND namespace_name = ?", in.ID, namespace).
			Update("title", in.Title).Error; err != nil {
			response.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
		response.SimpleOK.AbortWithGinH(c)
		return
	}
	// 采用结构体 Select 更新以触发 crypto 序列化器加密存储（map 更新不会走序列化器）
	if err := db.WithContext(c).Model(&modelsai.KnowledgeDocument{}).
		Where("id = ? AND namespace_name = ?", in.ID, namespace).
		Select("title", "content").
		Updates(&modelsai.KnowledgeDocument{Title: in.Title, Content: serializer.NewCrypto([]byte(in.Content))}).Error; err != nil {
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
		Content:       serializer.NewCrypto([]byte(in.Content)),
	}
	if err := comps.Knowledge.Reindex(c, doc); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// KnowledgeDocumentBatchDelete 批量删除知识库文档。
// 逐条按 (id, namespace) 确认文档存在（不存在记录失败明细），再经知识库索引删除
// 文档及其分块；AI 功能未启用时直接删除文档行（无索引可清理）。导入文档（含 ObjectKey）
// 在删除行/索引后同步清理 S3 原件，失败计入批量失败明细（不静默忽略），保证「DB 行 ↔ S3 原件」共存亡。
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
					return errors.New(response.LocalizeDetail(response.GetLocalizer(c), "knowledgeDocumentNotFound", id))
				}
				return errors.WithStack(err)
			}
			if comps.Knowledge == nil {
				if err := db.WithContext(c).
					Where("id = ? AND namespace_name = ?", id, namespace).
					Delete(&modelsai.KnowledgeDocument{}).Error; err != nil {
					return errors.WithStack(err)
				}
			} else if err := comps.Knowledge.DeleteDocument(c, id); err != nil {
				return errors.WithStack(err)
			}
			// 手动录入文档无原件（ObjectKey 为空），跳过 S3 清理
			if instance.ObjectKey.Key() == "" {
				return nil
			}
			docKey, err := instance.ObjectCryptoKey()
			if err != nil {
				return errors.WithStack(err)
			}
			if err := middlewares.GetObjectCache(c).Remove(c, instance.ObjectKey.Key(), docKey); err != nil {
				return errors.WithStack(err)
			}
			return nil
		},
	)
}

// KnowledgeDocumentReindex 处理重新索引知识库文档的请求。
// 按 (id, namespace) 定位文档（不存在 404）。按来源区分语义：
//   - 导入文档（ObjectKey 非空）：置 pending + 清空 parse_error，异步投递解析任务重新下载
//     原件解析并重建索引，接口立即返回（chunkCount/hash 为当前值）；文档已在 pending/parsing
//     时返回 400，避免重复投递。
//   - 手动录入文档：同步切块向量化并返回最新分块数与内容摘要（status 为 ready）。
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
		response.CreateBadBodyMessageError("aiFeatureDisabled").AbortWithGinH(c)
		return
	}
	db := middlewares.GetCoreDatabase(c)
	doc := modelsai.KnowledgeDocument{}
	if err := db.WithContext(c).
		Where("id = ? AND namespace_name = ?", id, namespace).
		First(&doc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if doc.ObjectKey.Key() != "" {
		if doc.Status == string(modelsai.KnowledgeDocStatusPending) || doc.Status == string(modelsai.KnowledgeDocStatusParsing) {
			response.CreateBadBodyMessageError("knowledgeDocumentParsing").AbortWithGinH(c)
			return
		}
		if err := db.WithContext(c).Model(&modelsai.KnowledgeDocument{}).
			Where("id = ? AND namespace_name = ?", id, namespace).
			Updates(map[string]any{
				"status":      string(modelsai.KnowledgeDocStatusPending),
				"parse_error": "",
			}).Error; err != nil {
			response.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
		task, err := knowledge.NewParseTask(doc.ID)
		if err != nil {
			response.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
		if _, err := middlewares.GetPipelineClient(c).EnqueueContext(c, task, asynq.MaxRetry(0)); err != nil {
			response.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
		response.SimpleOK.AbortWithGinW(c, KnowledgeDocumentReindexOut{
			ID:         doc.ID,
			ChunkCount: doc.ChunkCount,
			Hash:       doc.Hash.Digest(),
			Status:     string(modelsai.KnowledgeDocStatusPending),
		})
		return
	}
	if err := comps.Knowledge.Reindex(c, &doc); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, KnowledgeDocumentReindexOut{
		ID:         doc.ID,
		ChunkCount: doc.ChunkCount,
		Hash:       doc.Hash.Digest(),
		Status:     string(modelsai.KnowledgeDocStatusReady),
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
	// Hash 文档内容的 HMAC-SHA256 十六进制摘要（带密钥，防离线爆破）
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
	overview, err := aiknowledge.BuildChunkOverview(c, db, middlewares.GetAi(c).Params, &doc)
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
	if info, ok := middlewares.GetAiRuntime(c).Snapshot().Slots()[modelsai.SlotEmbedding]; ok {
		out.Summary.EmbeddingModel = info.Model
	}
	// 向量维度取 embedding 列的实际维度（表或列不可读时记 0），与状态接口共用同一读取来源
	if n, found, err := llminfra.ChunkEmbeddingDimension(c, db); err == nil && found {
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

// KnowledgeSystemDocumentChunksGet 处理获取系统命名空间知识库文档分块明细的请求。
// 与 KnowledgeDocumentChunksGet 同构，数据范围恒为内置系统命名空间（base.SystemNamespace），
// 不存在时返回 404；纯只读接口。
func KnowledgeSystemDocumentChunksGet(c *gin.Context) {
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	doc := modelsai.KnowledgeDocument{}
	if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, base.SystemNamespace).First(&doc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	overview, err := aiknowledge.BuildChunkOverview(c, db, middlewares.GetAi(c).Params, &doc)
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
	if info, ok := middlewares.GetAiRuntime(c).Snapshot().Slots()[modelsai.SlotEmbedding]; ok {
		out.Summary.EmbeddingModel = info.Model
	}
	// 向量维度取 embedding 列的实际维度（表或列不可读时记 0），与状态接口共用同一读取来源
	if n, found, err := llminfra.ChunkEmbeddingDimension(c, db); err == nil && found {
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

// knowledgeUploadCounter 包装上传文件读取流并统计已读字节数：ObjectCache.Store 流式加密上传
// 不返回字节数，故以计数 reader 在流式消费过程中统计，避免把整文件读进内存。
type knowledgeUploadCounter struct {
	// r 底层文件读取流
	r io.Reader
	// n 已读取的字节数
	n int64
}

// Read 读取底层流并累加字节数，实现 io.Reader。
func (c *knowledgeUploadCounter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, errors.WithStack(err)
}

// KnowledgeDocumentUpload 处理导入知识库文档（multipart/form-data）的请求。
// 字段：title（可选，缺省取文件名去扩展名）、file（必填，必须为最后一个表单部分）。
// 处理顺序：请求体限流 → 流式解析表单 → docconv.Detect 做唯一权威格式校验 → AI 启用校验 →
// 插入 pending 文档行 → 流式加密上传原件并统计字节数 → 回填 size_bytes/object_key →
// 投递解析任务。任一步失败按「DB 行 ↔ S3 原件共存亡」回滚已产生的副作用，并返回按请求语言本地化的错误。
func KnowledgeDocumentUpload(c *gin.Context) {
	// 请求体上限（含 multipart 表单开销）：先按 Content-Length 早拒，再用 MaxBytesReader
	// 兜底分块传输/长度未知的场景，二者共同保证超限请求以按请求语言本地化的错误返回 400。
	if c.Request.ContentLength > maxUploadBytes {
		response.CreateBadBodyMessageError("knowledgeUploadTooLarge", maxUploadBytes).AbortWithGinH(c)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes)
	var title string
	file, err := views.ReadMultipartFile(c, func(name string, body []byte) {
		if name == "title" {
			title = string(body)
		}
	})
	if err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	if file == nil {
		response.CreateBadBodyMessageError("knowledgeUploadFileRequired").AbortWithGinH(c)
		return
	}
	// 原始文件名取自 *multipart.Part（file 部分），Detect 以它做唯一权威的格式校验；
	// 前端 accept 只是选择过滤，不构成服务端契约。
	var originalName string
	if part, ok := file.(*multipart.Part); ok {
		originalName = part.FileName()
	}
	format, err := docconv.Detect(originalName)
	if err != nil {
		response.CreateBadBodyMessageError("knowledgeUnsupportedFormat", originalName).AbortWithGinH(c)
		return
	}
	// AI 未启用时导入无意义（正文必须由解析流水线产生）：直接 400，不落库、不上传。
	if !middlewares.GetAiRuntime(c).Enabled() {
		response.CreateBadBodyMessageError("aiFeatureDisabled").AbortWithGinH(c)
		return
	}
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	if title = strings.TrimSpace(title); title == "" {
		title = strings.TrimSuffix(filepath.Base(originalName), filepath.Ext(originalName))
	}
	db := middlewares.GetCoreDatabase(c)
	cache := middlewares.GetObjectCache(c)
	doc := &modelsai.KnowledgeDocument{
		NamespaceName: namespace,
		Title:         title,
		Format:        string(format),
		OriginalName:  originalName,
		Status:        string(modelsai.KnowledgeDocStatusPending),
	}
	// S3 原件密钥由 object_key 字段的 s3crypto 标签派生，上传/删除/下载共用同一密钥
	docKey, err := doc.ObjectCryptoKey()
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := db.WithContext(c).Create(doc).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	objectKey := fmt.Sprintf("knowledge/%s/%d.%s", namespace, doc.ID, doc.Format)
	// rollback 撤销已产生的副作用：删除 S3 原件（若已上传）与文档行，保证共存亡；
	// 回滚自身的失败只记日志，不覆盖原始错误（禁止静默）。
	rollback := func() {
		if err := cache.Remove(c, objectKey, docKey); err != nil {
			middlewares.GetLogger(c).Warnf(c, nil, "knowledge upload rollback: remove object %q failed, err=%v", objectKey, err)
		}
		if err := db.WithContext(c).Where("id = ?", doc.ID).Delete(&modelsai.KnowledgeDocument{}).Error; err != nil {
			middlewares.GetLogger(c).Warnf(c, nil, "knowledge upload rollback: delete document %d failed, err=%v", doc.ID, err)
		}
	}
	counter := &knowledgeUploadCounter{r: file}
	if err := cache.Store(c, objectKey, counter, docKey); err != nil {
		rollback()
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			response.CreateBadBodyMessageError("knowledgeUploadTooLarge", maxUploadBytes).AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 结构体 + Select 更新以触发 s3crypto 序列化器（map 更新不会走序列化器）
	if err := db.WithContext(c).Model(&modelsai.KnowledgeDocument{}).
		Where("id = ?", doc.ID).
		Select("size_bytes", "object_key").
		Updates(&modelsai.KnowledgeDocument{
			SizeBytes: counter.n,
			ObjectKey: serializer.NewS3Crypto(objectKey),
		}).Error; err != nil {
		rollback()
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	task, err := knowledge.NewParseTask(doc.ID)
	if err != nil {
		rollback()
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if _, err := middlewares.GetPipelineClient(c).EnqueueContext(c, task, asynq.MaxRetry(0)); err != nil {
		rollback()
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, KnowledgeDocumentUploadOut{
		ID:     doc.ID,
		Status: string(modelsai.KnowledgeDocStatusPending),
	})
}

// KnowledgeDocumentDownload 处理下载知识库文档原件的请求。
// 按 (id, namespace) 定位文档：不存在或非导入文档（ObjectKey 为空）返回 404。经
// ObjectCache.Open 取明文读取流并流式拷贝到响应，不整文件缓冲、不落地。响应设置
// Content-Disposition（RFC 5987 编码原始文件名）与按格式的 Content-Type。
// 该接口返回二进制流，不经 views.StructMetaGinHandler 的统一 JSON 响应封装：统一响应会把
// 结果包成 {code,data} JSON，破坏二进制内容与下载文件名，故照 plaintext.S3PlaintextGet 的
// 既有先例直接写响应体（不改变框架响应语义，因为此接口无 JSON 出参）。
func KnowledgeDocumentDownload(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
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
	// 非导入文档（手工录入，无 S3 原件）不可下载
	if doc.ObjectKey.Key() == "" {
		response.ErrDefaultPageNotFound.AbortWithGinH(c)
		return
	}
	docKey, err := doc.ObjectCryptoKey()
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	rc, err := middlewares.GetObjectCache(c).Open(c, doc.ObjectKey.Key(), docKey)
	if err != nil {
		if s3.IsNotExists(err) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	defer rc.Close()
	name := doc.OriginalName
	if name == "" {
		name = doc.Title
	}
	// RFC 5987：文件名可能含中文等非 ASCII 字符，须以 UTF-8'' + 百分号编码置于 filename*
	c.Header("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(name))
	c.Header("Content-Type", knowledgeDocumentContentType(doc.Format))
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, rc); err != nil {
		// 响应头已写出，无法再改状态；仅记录告警以便排查传输中断
		middlewares.GetLogger(c).Warnf(c, nil, "knowledge download: stream object %q to client failed, err=%v", doc.ObjectKey, err)
	}
}

// knowledgeDocumentContentType 按文档格式返回下载响应的 Content-Type，未知格式回退
// application/octet-stream。
func knowledgeDocumentContentType(format string) string {
	switch docconv.Format(format) {
	case docconv.FormatMarkdown, docconv.FormatTxt:
		return "text/plain; charset=utf-8"
	case docconv.FormatPDF:
		return "application/pdf"
	case docconv.FormatDocx:
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		return "application/octet-stream"
	}
}
