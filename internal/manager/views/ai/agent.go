package ai

import (
	// stdlib
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	// third-party
	"github.com/gin-gonic/gin"

	// internal
	ai "zhongkui.com/zhongkui/internal/ai"
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/manager/views"
	modelsai "zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	pkgllm "zhongkui.com/zhongkui/internal/pkg/llm"
	"zhongkui.com/zhongkui/internal/pkg/sse"
)

// requireAiEnabled 获取已启用的 AI 组件；功能未启用时以 400 + 英文错误中断请求并返回 nil。
// requireAiEnabled fetches the enabled AI components; when the feature is disabled it
// aborts the request with 400 + an English error and returns nil.
func requireAiEnabled(c *gin.Context) *ai.Components {
	comps := middlewares.GetAi(c)
	if !comps.Enabled() {
		response.CreateCodeError(http.StatusBadRequest, response.CodeBadBody, errors.New("ai feature is disabled")).AbortWithGinH(c)
		return nil
	}
	return comps
}

// getOwnedAgentSession 按 ID 查询 AI 助手会话并校验其属于给定命名空间：
// 会话不存在（ErrSessionNotFound）或归属不符时以 404 中断请求并返回 nil。
// getOwnedAgentSession fetches an agent session by ID and validates that it belongs
// to the given namespace: a missing session (ErrSessionNotFound) or an ownership
// mismatch aborts the request with 404 and returns nil.
func getOwnedAgentSession(c *gin.Context, comps *ai.Components, sessionID uint, namespace string) *modelsai.AgentSession {
	session, err := comps.Sessions.GetSession(c, sessionID)
	if err != nil {
		if errors.Is(err, ai.ErrSessionNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return nil
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return nil
	}
	if session.NamespaceName != namespace {
		response.ErrDefaultPageNotFound.AbortWithGinH(c)
		return nil
	}
	return session
}

// AgentSessionItemOut 定义 AI 助手会话的输出项结构。
type AgentSessionItemOut struct {
	// ID 会话 ID
	ID uint `json:"id"`
	// Title 会话标题
	Title string `json:"title"`
	// Slot 会话缺省档位（flash/standard/advanced），空表示按 agent.defaultSlot 参数解析
	Slot string `json:"slot"`
	// Status 会话状态（active/archived）
	Status string `json:"status"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updatedAt"`
}

// AgentSessionListOut 定义 AI 助手会话列表输出。
type AgentSessionListOut []AgentSessionItemOut

// AgentSessionCreateIn 定义创建 AI 助手会话的请求输入。
type AgentSessionCreateIn struct {
	// Title 会话标题，必填
	Title string `json:"title" binding:"required"`
	// Slot 会话缺省档位（flash/standard/advanced），可选；空表示按 agent.defaultSlot 参数解析
	Slot string `json:"slot"`
}

// AgentSessionRenameIn 定义重命名 AI 助手会话的请求输入。
type AgentSessionRenameIn struct {
	// ID 会话 ID，必填
	ID uint `json:"id" binding:"required"`
	// Title 会话新标题，必填
	Title string `json:"title" binding:"required"`
}

// AgentMessageItemOut 定义 AI 助手会话消息的输出项结构。
type AgentMessageItemOut struct {
	// ID 消息 ID
	ID uint `json:"id"`
	// Role 消息角色（user/assistant/system）
	Role string `json:"role"`
	// Content 消息正文内容
	Content string `json:"content"`
	// Draft 助手回复的结构化草稿载荷（JSON 对象），无草稿时省略
	Draft json.RawMessage `json:"draft,omitempty"`
	// Citations 助手回复的检索引用来源列表，无引用时省略
	Citations []ai.RetrievalResult `json:"citations,omitempty"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"createdAt"`
}

// AgentMessageListOut 定义 AI 助手会话消息列表输出。
type AgentMessageListOut []AgentMessageItemOut

// AgentStreamIn 定义 AI 助手流式生成的请求输入。
type AgentStreamIn struct {
	// Query 用户本次提问文本，必填
	Query string `json:"query" binding:"required"`
	// DocIDs 检索限定的文档 ID 列表，为空表示不限定
	DocIDs []uint `json:"docIDs"`
	// BusinessData 前端透传的当前业务现场（注入前经 SanitizeBusinessData 清洗限界）
	BusinessData pkgllm.BusinessData `json:"businessData"`
	// RegoRule 可选的结构化草稿 Rego 校验规则源码，为空跳过规则校验
	RegoRule string `json:"regoRule"`
	// Slot 本次生成的显式档位（flash/standard/advanced），可选；为空时依次回退到会话档位与
	// agent.defaultSlot 参数
	Slot string `json:"slot"`
}

// AgentSlotStatusOut 定义单个档位的绑定模型与可用性输出项。
// AgentSlotStatusOut defines the output item of one slot: its bound model and availability.
type AgentSlotStatusOut struct {
	// Slot 档位标识（flash/standard/advanced/embedding）
	Slot string `json:"slot"`
	// ModelName 绑定模型在注册表中的本地标识（无绑定时为空）
	ModelName string `json:"modelName"`
	// ProviderModel 供应商侧的模型名（无绑定时为空）
	ProviderModel string `json:"providerModel"`
	// Kind 绑定模型类型（chat/embedding；无绑定时为空）
	Kind string `json:"kind"`
	// Enabled 绑定模型是否启用
	Enabled bool `json:"enabled"`
	// Bound 绑定是否可用（模型已启用且类型与档位匹配）
	Bound bool `json:"bound"`
	// Available 该档位当前是否可解析到已构建的客户端
	Available bool `json:"available"`
}

// AgentStatusOut 定义 AI 助手状态输出。
type AgentStatusOut struct {
	// Enabled AI 能力是否已完整装配启用
	Enabled bool `json:"enabled"`
	// ChatModel 默认对话档位（standard）绑定的模型名称
	ChatModel string `json:"chatModel"`
	// EmbeddingModel Embedding 模型名称
	EmbeddingModel string `json:"embeddingModel"`
	// EmbeddingDimensions 配置的向量维度
	EmbeddingDimensions int `json:"embeddingDimensions"`
	// DimensionMismatch 数据库 embedding 列实际维度与配置是否不一致
	DimensionMismatch bool `json:"dimensionMismatch"`
	// DimensionDetail 维度不一致的详细说明（一致时为空）
	DimensionDetail string `json:"dimensionDetail"`
	// Slots 各档位绑定的模型与可用性（含不可用绑定），按 pkgllm.AllSlots 顺序
	Slots []AgentSlotStatusOut `json:"slots"`
	// DefaultSlot 未显式指定档位且会话未记录档位时使用的默认档位（参数 agent.defaultSlot 的
	// 生效值），供前端档位下拉取初值，避免前端自持档位字面量
	DefaultSlot string `json:"defaultSlot"`
	// ParamOverrideCount 当前值与出厂值不同的行为参数条数（0 表示全部参数仍为出厂值）
	ParamOverrideCount int `json:"paramOverrideCount"`
	// Reason AI 能力不可用的原因说明（按请求语言本地化；可用时为空）
	Reason string `json:"reason"`
}

// newAgentSessionItemOut 将会话模型映射为视图层输出项。
// newAgentSessionItemOut maps the session model into a view-layer output item.
func newAgentSessionItemOut(s *modelsai.AgentSession) AgentSessionItemOut {
	return AgentSessionItemOut{
		ID:        s.ID,
		Title:     s.Title,
		Slot:      s.TargetModel,
		Status:    s.Status,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}
}

// newAgentMessageItemOut 将消息模型映射为视图层输出项：
// 草稿与引用载荷经 JSON 反序列化暴露，空或非法时留空（omitempty 省略）。
// newAgentMessageItemOut maps the message model into a view-layer output item: the
// draft and citation payloads are exposed via JSON unmarshal, left empty when blank
// or invalid (omitted by omitempty).
func newAgentMessageItemOut(m *modelsai.AgentMessage) AgentMessageItemOut {
	out := AgentMessageItemOut{
		ID:        m.ID,
		Role:      m.Role,
		Content:   m.Content,
		CreatedAt: m.CreatedAt,
	}
	if len(m.DraftJSON) > 0 {
		out.Draft = json.RawMessage(m.DraftJSON)
	}
	if len(m.CitationsJSON) > 0 {
		var citations []ai.RetrievalResult
		if err := json.Unmarshal(m.CitationsJSON, &citations); err == nil {
			out.Citations = citations
		}
	}
	return out
}

// AgentSessionCreate 处理创建 AI 助手会话的请求。
// 新会话归属当前请求命名空间，初始状态为 active；可选档位字段经校验后作为该会话的
// 缺省档位落库，非法取值以 400 + 英文错误拒绝（不静默回落）。
func AgentSessionCreate(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	var in AgentSessionCreateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	slot := ""
	if value := strings.TrimSpace(in.Slot); value != "" {
		parsed, ok := pkgllm.ParseChatSlot(value)
		if !ok {
			response.CreateCodeError(http.StatusBadRequest, response.CodeBadBody,
				pkgllm.InvalidChatSlotError(value)).AbortWithGinH(c)
			return
		}
		slot = string(parsed)
	}
	session, err := comps.Sessions.CreateSession(c, namespace, in.Title, slot)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, newAgentSessionItemOut(session))
}

// AgentSessionListGet 处理获取 AI 助手会话列表的请求。
// 仅返回当前命名空间内的会话，按更新时间倒序（最近活跃在前）。
func AgentSessionListGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	sessions, err := comps.Sessions.ListSessions(c, namespace, 0, 0)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	out := make(AgentSessionListOut, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, newAgentSessionItemOut(s))
	}
	response.SimpleOK.AbortWithGinW(c, out)
}

// AgentSessionDetailGet 处理获取 AI 助手会话详情的请求。
// 会话不存在或不属于当前命名空间时返回 404。
func AgentSessionDetailGet(c *gin.Context) {
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
	session := getOwnedAgentSession(c, comps, uint(id), namespace)
	if session == nil {
		return
	}
	response.SimpleOK.AbortWithGinW(c, newAgentSessionItemOut(session))
}

// AgentSessionRename 处理重命名 AI 助手会话的请求。
// 会话不存在或不属于当前命名空间时返回 404。
func AgentSessionRename(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	var in AgentSessionRenameIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	if session := getOwnedAgentSession(c, comps, in.ID, namespace); session == nil {
		return
	}
	if _, err := comps.Sessions.RenameSession(c, in.ID, in.Title); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// AgentSessionDelete 处理删除 AI 助手会话的请求。
// 会话不存在或不属于当前命名空间时返回 404；删除会话时其消息在同一事务内级联删除。
func AgentSessionDelete(c *gin.Context) {
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
	if session := getOwnedAgentSession(c, comps, uint(id), namespace); session == nil {
		return
	}
	if err := comps.Sessions.DeleteSession(c, uint(id)); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// AgentMessageListGet 处理获取 AI 助手会话消息列表的请求。
// 会话不存在或不属于当前命名空间时返回 404；消息按创建时间升序返回（与对话顺序一致）。
func AgentMessageListGet(c *gin.Context) {
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
	if session := getOwnedAgentSession(c, comps, uint(id), namespace); session == nil {
		return
	}
	messages, err := comps.Sessions.ListMessages(c, uint(id), 0, 0)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	out := make(AgentMessageListOut, 0, len(messages))
	for _, m := range messages {
		out = append(out, newAgentMessageItemOut(m))
	}
	response.SimpleOK.AbortWithGinW(c, out)
}

// AgentStream 处理 AI 助手流式生成的请求（SSE，路由 /agent/:sessionID/stream）。
// 先校验会话归属（404 语义同详情接口），再经 Agent.StreamRun 执行带会话持久化的
// 流式图；流上依次透传 delta 增量事件，最终以 done 完成事件（含结构化产物与引用）
// 收尾；任一环节错误以 error 事件发送并结束流。本次生成的档位由 Agent 按「请求 slot →
// 会话档位 → agent.defaultSlot」解析，指定档位未绑定时返回英文错误，不静默回落。
func AgentStream(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	sessionID, ok := views.GetUint64Param(c, "sessionID")
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	session := getOwnedAgentSession(c, comps, uint(sessionID), namespace)
	if session == nil {
		return
	}
	var in AgentStreamIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	sseWriter := sse.NewSSEWriter(c.Writer)
	// 工具调用阶段事件映射：带增量输出的事件走 delta 事件（携带阶段标识），状态事件走
	// stage 事件（沿用 dryrun.go 的 report 写法）；写失败（如客户端断开）由 Agent 节点 4
	// 捕获并中止工具循环，避免无谓的模型与工具调用。
	// Tool-call stage event mapping: events carrying a delta become delta events (tagged with
	// the stage), status events become stage events (following the report style of dryrun.go);
	// a write failure (e.g. a disconnected client) is caught by Agent node 4 and aborts the tool
	// loop, avoiding needless model and tool calls.
	report := func(ev ai.StageEvent) error {
		if ev.Delta != "" {
			return sseWriter.SendStageDelta(string(ev.Stage), ev.Delta)
		}
		return sseWriter.SendStage(ev)
	}
	reader, err := comps.Agent.StreamRun(c, ai.AgentInput{
		Query:         in.Query,
		NamespaceName: namespace,
		DocIDs:        in.DocIDs,
		SessionID:     uint(sessionID),
		BusinessData:  in.BusinessData,
		RegoRule:      in.RegoRule,
		Slot:          pkgllm.Slot(strings.TrimSpace(in.Slot)),
		DeclaredSlot:  pkgllm.Slot(session.TargetModel),
		ToolReporter:  report,
	})
	if err != nil {
		_ = sseWriter.SendError(err)
		return
	}
	defer reader.Close()
	for {
		item, err := reader.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			_ = sseWriter.SendError(err)
			return
		}
		if item.Delta != "" {
			_ = sseWriter.SendDelta(item.Delta)
		}
		if item.Done {
			_ = sseWriter.SendDone(item)
			return
		}
	}
}

// AgentStatusGet 处理获取 AI 助手状态的请求（不强制启用，始终 200）。
// 从运行时快照读取 AI 能力是否启用、各档位绑定的模型与可用性、生效的参数覆盖数量；
// 未启用时给出明确原因（按请求语言本地化）。启用时额外查询数据库 knowledge_chunks.embedding
// 列的实际维度并与绑定模型维度对比，不一致时提示风险。
// AgentStatusGet handles the AI assistant status request (no forced enable, always 200).
// It reads from the runtime snapshot whether the AI capability is enabled, the model bound
// to each slot with its availability and the number of effective parameter overrides, and
// gives an explicit reason (localized to the request language) when disabled. When enabled it
// additionally queries the actual dimension of the knowledge_chunks.embedding column and compares
// it with the bound model dimension, warning about a mismatch.
func AgentStatusGet(c *gin.Context) {
	rt := middlewares.GetAiRuntime(c)
	snap := rt.Snapshot()
	out := AgentStatusOut{Enabled: rt.Enabled(), Reason: snap.DisabledReason(middlewares.GetLanguage(c))}
	out.DefaultSlot = string(snap.DefaultSlot())
	slots := snap.Slots()
	out.Slots = make([]AgentSlotStatusOut, 0, len(pkgllm.AllSlots))
	for _, slot := range pkgllm.AllSlots {
		item := AgentSlotStatusOut{Slot: string(slot)}
		if info, ok := slots[slot]; ok {
			item.ModelName = info.Name
			item.ProviderModel = info.Model
			item.Kind = string(info.Kind)
			item.Enabled = info.Enabled
			item.Bound = info.Bound
		}
		// 可用性取运行时实际构建结果：对话档位查客户端表，向量化槽位查向量化客户端
		if slot == pkgllm.SlotEmbedding {
			_, err := rt.Embed()
			item.Available = err == nil
		} else {
			_, err := rt.Chat(slot)
			item.Available = err == nil
		}
		out.Slots = append(out.Slots, item)
	}
	if info, ok := slots[pkgllm.SlotStandard]; ok {
		out.ChatModel = info.Model
	}
	if info, ok := slots[pkgllm.SlotEmbedding]; ok {
		out.EmbeddingModel = info.Model
	}
	out.EmbeddingDimensions = snap.EmbeddingDimensions()
	out.ParamOverrideCount = snap.ParamOverrideCount()
	if out.Enabled {
		if n, found, err := pkgllm.ChunkEmbeddingDimension(c, middlewares.GetCoreDatabase(c)); err == nil && found && n != out.EmbeddingDimensions {
			out.DimensionMismatch = true
			out.DimensionDetail = fmt.Sprintf("embedding dimension mismatch: column is %d, bound model is %d", n, out.EmbeddingDimensions)
		}
	}
	response.SimpleOK.AbortWithGinW(c, out)
}
