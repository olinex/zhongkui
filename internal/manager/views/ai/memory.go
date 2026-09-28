package ai

import (
	// stdlib
	"fmt"
	"strconv"
	"strings"
	"time"

	// third-party
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"

	// internal
	ai "zhongkui.com/zhongkui/internal/ai"
	"zhongkui.com/zhongkui/internal/ai/tools"
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/manager/views"
	modelsai "zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	pkgllm "zhongkui.com/zhongkui/internal/pkg/llm"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// AgentMemoryItemOut 定义长期记忆列表项的输出结构（独立于内部模型层，逐字段映射）。
// AgentMemoryItemOut defines the output item of the long-term memory list (independent of the
// internal model layer, mapped field by field).
type AgentMemoryItemOut struct {
	// ID 记忆 ID
	// ID the memory id
	ID uint `json:"id"`
	// AgentKind 记忆所属 Agent 类型（knowledge/rego）
	// AgentKind the agent kind the memory belongs to (knowledge/rego)
	AgentKind pkgllm.AgentKind `json:"agentKind"`
	// Kind 记忆类别（fact/preference/decision/artifact）
	// Kind the memory kind (fact/preference/decision/artifact)
	Kind pkgllm.MemoryKind `json:"kind"`
	// Title 记忆标题
	// Title the memory title
	Title string `json:"title"`
	// Content 记忆正文
	// Content the memory body
	Content string `json:"content"`
	// Status 记忆状态（active/archived）
	// Status the memory status (active/archived)
	Status pkgllm.MemoryStatus `json:"status"`
	// UseCount 被检索命中的次数
	// UseCount the number of times the memory has been recalled by retrieval
	UseCount int `json:"useCount"`
	// LastUsedAt 最近一次被检索召回的时间，可空
	// LastUsedAt the last time the memory was recalled, nullable
	LastUsedAt *time.Time `json:"lastUsedAt"`
	// CreatedAt 创建时间
	// CreatedAt the creation time
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt 更新时间
	// UpdatedAt the update time
	UpdatedAt time.Time `json:"updatedAt"`
}

// AgentMemoryListOut 定义长期记忆列表输出（嵌入 BaseTotalOut 支持分页总数）。
// AgentMemoryListOut defines the long-term memory list output (embedding BaseTotalOut for the
// paginated total).
type AgentMemoryListOut struct {
	AgentMemoryItemOut
	utils.BaseTotalOut
}

// AgentMemoryDetailOut 定义长期记忆详情的输出结构。
// AgentMemoryDetailOut defines the output of the long-term memory detail.
type AgentMemoryDetailOut struct {
	// ID 记忆 ID
	// ID the memory id
	ID uint `json:"id"`
	// NamespaceName 所属命名空间
	// NamespaceName the owning namespace
	NamespaceName string `json:"namespaceName"`
	// AgentKind 记忆所属 Agent 类型（knowledge/rego）
	// AgentKind the agent kind the memory belongs to (knowledge/rego)
	AgentKind pkgllm.AgentKind `json:"agentKind"`
	// SessionID 来源会话 ID（手动新增为 0）
	// SessionID the originating session id (0 for manually added memories)
	SessionID uint `json:"sessionId"`
	// Kind 记忆类别（fact/preference/decision/artifact）
	// Kind the memory kind (fact/preference/decision/artifact)
	Kind pkgllm.MemoryKind `json:"kind"`
	// Title 记忆标题
	// Title the memory title
	Title string `json:"title"`
	// Content 记忆正文
	// Content the memory body
	Content string `json:"content"`
	// Status 记忆状态（active/archived）
	// Status the memory status (active/archived)
	Status pkgllm.MemoryStatus `json:"status"`
	// UseCount 被检索命中的次数
	// UseCount the number of times the memory has been recalled by retrieval
	UseCount int `json:"useCount"`
	// LastUsedAt 最近一次被检索召回的时间，可空
	// LastUsedAt the last time the memory was recalled, nullable
	LastUsedAt *time.Time `json:"lastUsedAt"`
	// CreatedAt 创建时间
	// CreatedAt the creation time
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt 更新时间
	// UpdatedAt the update time
	UpdatedAt time.Time `json:"updatedAt"`
}

// AgentMemoryCreateIn 定义新增长期记忆的请求输入（命名空间取自请求中间件，不在此声明）。
// AgentMemoryCreateIn defines the request input of creating a long-term memory (the namespace comes
// from the request middleware and is not declared here).
type AgentMemoryCreateIn struct {
	// AgentKind 记忆所属 Agent 类型（knowledge/rego），可选，缺省 knowledge；非法取值或首尾空白以 400 拒绝
	// AgentKind the agent kind the memory belongs to (knowledge/rego), optional, defaulting to
	// knowledge; an invalid value or surrounding whitespace is rejected with 400
	AgentKind pkgllm.AgentKind `json:"agentKind" binding:"omitempty,trimmed,oneof=knowledge rego"`
	// Kind 记忆类别（fact/preference/decision/artifact），可选，缺省 fact；非法取值或首尾空白以 400 拒绝
	// Kind the memory kind (fact/preference/decision/artifact), optional, defaulting to fact; an
	// invalid value or surrounding whitespace is rejected with 400
	Kind pkgllm.MemoryKind `json:"kind" binding:"omitempty,trimmed,oneof=fact preference decision artifact"`
	// Title 记忆标题，可选
	// Title the memory title, optional
	Title string `json:"title"`
	// Content 记忆正文，必填
	// Content the memory body, required
	Content string `json:"content" binding:"required"`
}

// AgentMemoryUpdateIn 定义更新长期记忆的请求输入：各字段可选，空值表示不修改对应字段。
// AgentMemoryUpdateIn defines the request input of updating a long-term memory: every field is
// optional and an empty value leaves the corresponding field unchanged.
type AgentMemoryUpdateIn struct {
	// Kind 记忆类别（fact/preference/decision/artifact），可选，缺省不修改；非法取值或首尾空白以 400 拒绝
	// Kind the memory kind (fact/preference/decision/artifact), optional, unchanged when empty; an
	// invalid value or surrounding whitespace is rejected with 400
	Kind pkgllm.MemoryKind `json:"kind" binding:"omitempty,trimmed,oneof=fact preference decision artifact"`
	// Title 记忆标题，可选，空值不修改
	// Title the memory title, optional, unchanged when empty
	Title string `json:"title"`
	// Content 记忆正文，可选，空值不修改；非空时重新向量化
	// Content the memory body, optional, unchanged when empty; a non-empty value is re-embedded
	Content string `json:"content"`
}

// AgentMemoryBatchIn 定义批量删除长期记忆的请求输入；Ids 接受标量或数组。
// AgentMemoryBatchIn defines the request input of batch-deleting long-term memories; Ids accepts a
// scalar or an array.
type AgentMemoryBatchIn struct {
	// Ids 待批量删除的记忆 ID 列表
	// Ids the memory ids to batch-delete
	Ids views.FlexList[uint] `json:"ids"`
}

// AgentMemoryStatusIn 定义启停长期记忆的请求输入。
// AgentMemoryStatusIn defines the request input of activating/archiving a long-term memory.
type AgentMemoryStatusIn struct {
	// Status 目标状态（active/archived），必填且须为合法取值
	// Status the target status (active/archived), required and must be a valid value
	Status pkgllm.MemoryStatus `json:"status" binding:"required,oneof=active archived"`
}

// newAgentMemoryItemOut 将记忆模型映射为列表项输出。
// newAgentMemoryItemOut maps a memory model into a list item output.
func newAgentMemoryItemOut(m *modelsai.AgentMemory) AgentMemoryItemOut {
	return AgentMemoryItemOut{
		ID:         m.ID,
		AgentKind:  m.AgentKind,
		Kind:       m.Kind,
		Title:      m.Title,
		Content:    m.Content,
		Status:     m.Status,
		UseCount:   m.UseCount,
		LastUsedAt: m.LastUsedAt,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
	}
}

// newAgentMemoryDetailOut 将记忆模型映射为详情输出。
// newAgentMemoryDetailOut maps a memory model into a detail output.
func newAgentMemoryDetailOut(m *modelsai.AgentMemory) AgentMemoryDetailOut {
	return AgentMemoryDetailOut{
		ID:            m.ID,
		NamespaceName: m.NamespaceName,
		AgentKind:     m.AgentKind,
		SessionID:     m.SessionID,
		Kind:          m.Kind,
		Title:         m.Title,
		Content:       m.Content,
		Status:        m.Status,
		UseCount:      m.UseCount,
		LastUsedAt:    m.LastUsedAt,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
}

// AgentMemoryListGet 处理长期记忆列表查询的请求。
// 仅返回当前命名空间内的记忆，支持分页、按 keyword/agentKind/kind/status 过滤与字段投影；
// 过滤与落库语义复用 ai.Memory.List（MemoryStore），视图层不自写 SQL。
// AgentMemoryListGet handles the long-term memory list request. It returns only the memories of the
// current namespace and supports pagination, filtering by keyword/agentKind/kind/status and field
// projection; the filtering and persistence semantics reuse ai.Memory.List (MemoryStore), so the
// view layer writes no SQL itself.
func AgentMemoryListGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	var in views.AgentMemoryListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	query := ai.MemoryQuery{
		NamespaceName: namespace,
		Offset:        int((in.Page - 1) * in.PageSize),
		Limit:         int(in.PageSize),
	}
	if fb := in.FilterBy; fb != nil {
		if fb.Keyword != nil {
			query.Keyword = strings.TrimSpace(fb.Keyword.Value)
		}
		if fb.AgentKind != nil {
			query.AgentKind = pkgllm.AgentKind(strings.TrimSpace(fb.AgentKind.Value))
		}
		if fb.Kind != nil {
			query.Kind = pkgllm.MemoryKind(strings.TrimSpace(fb.Kind.Value))
		}
		if fb.Status != nil {
			query.Status = pkgllm.MemoryStatus(strings.TrimSpace(fb.Status.Value))
		}
	}
	items, total, err := comps.Memory.List(c, query)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	outs := make([]AgentMemoryListOut, 0, len(items))
	for _, m := range items {
		outs = append(outs, AgentMemoryListOut{AgentMemoryItemOut: newAgentMemoryItemOut(m)})
	}
	projected := utils.ProjectSearchItems(outs, &in, func(o *AgentMemoryListOut) any { return *o })
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(uint(total), projected))
}

// AgentMemoryCreate 处理新增长期记忆的请求。
// 记忆归属当前请求命名空间，来源会话为 0（手动新增）；Agent 类型与类别经枚举校验（缺省
// knowledge / fact），正文经向量化与去重落库后返回详情。
// AgentMemoryCreate handles the long-term memory creation request. The memory belongs to the current
// namespace and its originating session is 0 (manually added); the agent kind and kind are validated
// against their enums (defaulting to knowledge / fact) and the body is embedded and persisted with
// dedupe, then the detail is returned.
func AgentMemoryCreate(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	var in AgentMemoryCreateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	mem, err := comps.Memory.Write(c, tools.MemoryWriteInput{
		NamespaceName: namespace,
		AgentKind:     in.AgentKind,
		Kind:          in.Kind,
		Title:         in.Title,
		Content:       in.Content,
	})
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, newAgentMemoryDetailOut(mem))
}

// AgentMemoryDetailGet 处理长期记忆详情查询的请求。
// 记忆不存在或不属于当前命名空间时返回 404。
// AgentMemoryDetailGet handles the long-term memory detail request. A missing memory or one outside
// the current namespace returns 404.
func AgentMemoryDetailGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	mem, err := comps.Memory.Get(c, namespace, uint(id))
	if err != nil {
		if errors.Is(err, ai.ErrMemoryNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, newAgentMemoryDetailOut(mem))
}

// AgentMemoryUpdate 处理更新长期记忆的请求。
// 记忆不存在或不属于当前命名空间时返回 404；仅更新入参中的非空字段，正文变更时重新向量化。
// AgentMemoryUpdate handles the long-term memory update request. A missing memory or one outside the
// current namespace returns 404; only the non-empty input fields are updated and a body change is
// re-embedded.
func AgentMemoryUpdate(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	var in AgentMemoryUpdateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	mem, err := comps.Memory.Update(c, namespace, uint(id), ai.MemoryUpdateInput{
		Kind:    in.Kind,
		Title:   in.Title,
		Content: in.Content,
	})
	if err != nil {
		if errors.Is(err, ai.ErrMemoryNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, newAgentMemoryDetailOut(mem))
}

// AgentMemoryBatchDelete 批量软删除长期记忆。
// 逐条按 (id, namespace) 确认记忆存在（不存在记录失败明细），再软删；返回批量结果汇总。
// AgentMemoryBatchDelete batch soft-deletes long-term memories. Each id is checked for existence
// under (id, namespace) first (a missing one is recorded as a failure) and then soft-deleted; the
// batch result summary is returned.
func AgentMemoryBatchDelete(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	var in AgentMemoryBatchIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	ids := []uint(in.Ids)
	if !views.ValidFlexLen(c, ids) {
		return
	}
	views.BatchExecute(c, ids,
		func(id uint) string { return strconv.FormatUint(uint64(id), 10) },
		func(id uint) error {
			if _, err := comps.Memory.Get(c, namespace, id); err != nil {
				if errors.Is(err, ai.ErrMemoryNotFound) {
					return fmt.Errorf("memory %d not found", id)
				}
				return errors.WithStack(err)
			}
			return comps.Memory.Delete(c, namespace, []uint{id})
		},
	)
}

// AgentMemorySetStatus 处理启停长期记忆的请求。
// 记忆不存在或不属于当前命名空间时返回 404；目标状态经枚举校验（active/archived）。
// AgentMemorySetStatus handles the long-term memory status request. A missing memory or one outside
// the current namespace returns 404; the target status is validated against its enum
// (active/archived).
func AgentMemorySetStatus(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	var in AgentMemoryStatusIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	if err := comps.Memory.SetStatus(c, namespace, uint(id), in.Status); err != nil {
		if errors.Is(err, ai.ErrMemoryNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}
