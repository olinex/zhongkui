package views

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
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	modelsai "zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/errors"
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
	Ids FlexList[uint] `json:"ids"`
}

// KnowledgeDocumentListGet 处理获取知识库文档列表的请求。
// 基于统一搜索基座 BuildSearchCountQuery 支持分页、排序与过滤（标题/创建/更新时间），
// 仅返回当前命名空间内的文档。
func KnowledgeDocumentListGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in KnowledgeDocumentListGetSearchIn
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
	id, ok := getUint64Param(c, "id")
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
	if !validFlexLen(c, ids) {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	comps := middlewares.GetAi(c)
	batchExecute(c, ids,
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
	id, ok := getUint64Param(c, "id")
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
