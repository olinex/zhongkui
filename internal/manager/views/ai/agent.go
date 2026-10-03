// Package ai 提供 AI 知识域（Agent / 知识库 / 会话）的 HTTP 处理器与视图 In/Out 结构体，
// 向内封装 internal/ai 的组件能力。
package ai

import (
	// 标准库
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	// 第三方
	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"

	// 项目内部
	ai "zhongkui.com/zhongkui/internal/ai"
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/manager/views"
	modelsai "zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/llm"
	"zhongkui.com/zhongkui/internal/pkg/sse"
)

// requireAiEnabled 获取已启用的 AI 组件；功能未启用时以 400 + 英文错误中断请求并返回 nil。
func requireAiEnabled(c *gin.Context) *ai.Components {
	comps := middlewares.GetAi(c)
	if !comps.Enabled() {
		response.CreateBadBodyError(errors.New("ai feature is disabled")).AbortWithGinH(c)
		return nil
	}
	return comps
}

// getOwnedAgentSession 按 ID 查询 AI 助手会话并校验其属于给定命名空间：
// 会话不存在（ErrSessionNotFound）或归属不符时以 404 中断请求并返回 nil。
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
	Slot modelsai.Slot `json:"slot"`
	// Status 会话状态（active/archived）
	Status modelsai.SessionStatus `json:"status"`
	// AgentKind 会话所属 Agent 类型（knowledge/rego）
	AgentKind modelsai.AgentKind `json:"agentKind"`
	// Summary 会话滚动摘要，无摘要时为空串
	Summary string `json:"summary"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updatedAt"`
}

// AgentSessionListOut 定义 AI 助手会话列表输出。
type AgentSessionListOut []AgentSessionItemOut

// AgentSessionCreateIn 定义创建 AI 助手会话的请求输入。
type AgentSessionCreateIn struct {
	// Query 首个用户问题，必填；会话主题据此经 LLM 生成（失败时按其截断兜底）
	Query string `json:"query" binding:"required"`
	// Slot 会话缺省档位（flash/standard/advanced），可选；空表示按 agent.defaultSlot 参数解析
	Slot modelsai.Slot `json:"slot"`
	// AgentKind 会话所属 Agent 类型（knowledge/rego），可选；缺省 knowledge，非法取值以 400 +
	// 英文错误拒绝
	AgentKind modelsai.AgentKind `json:"agentKind"`
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
	Role modelsai.MessageRole `json:"role"`
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

// AgentSessionCompactIn 定义会话压缩的请求输入：压缩仅需会话 ID（路径参数），无请求体字段。
type AgentSessionCompactIn struct{}

// AgentSessionCompactOut 定义会话压缩的输出：刷新后的摘要、成功落库的长期记忆条数与非致命告警。
type AgentSessionCompactOut struct {
	// Summary 刷新后的会话摘要
	Summary string `json:"summary"`
	// Extracted 成功落库的长期记忆条数
	Extracted int `json:"extracted"`
	// Warnings 非致命告警（长期记忆抽取/落库失败等），无告警时为空数组
	Warnings []string `json:"warnings"`
}

// AgentStreamIn 定义 AI 助手流式生成的请求输入。
type AgentStreamIn struct {
	// Query 用户本次提问文本，必填
	Query string `json:"query" binding:"required"`
	// DocIDs 检索限定的文档 ID 列表，为空表示不限定
	DocIDs []uint `json:"docIDs"`
	// BusinessData 前端透传的当前业务现场（注入前经 SanitizeBusinessData 清洗限界）
	BusinessData ai.BusinessData `json:"businessData"`
	// RegoRule 可选的结构化草稿 Rego 校验规则源码，为空跳过规则校验
	RegoRule string `json:"regoRule"`
	// Slot 本次生成的显式档位（flash/standard/advanced），可选；为空时依次回退到会话档位与
	// agent.defaultSlot 参数
	Slot modelsai.Slot `json:"slot"`
}

// AgentToolResultItemIn 定义单条客户端工具调用结果：与挂起时下发的 tool_request 按 callId 对应。
type AgentToolResultItemIn struct {
	// CallID 挂起时下发的调用 ID，必填
	CallID string `json:"callId" binding:"required"`
	// Approved 用户是否批准执行该调用
	Approved bool `json:"approved"`
	// Result 前端以用户自身凭据执行成功后的结果文本
	Result string `json:"result"`
	// Error 前端执行失败时的错误文本
	Error string `json:"error"`
}

// AgentToolResultIn 定义恢复挂起运行、回传客户端工具调用结果的请求输入。
type AgentToolResultIn struct {
	// Results 本次回传的执行结果列表；须覆盖挂起时下发的全部 callId（缺失者按「未提供结果」回灌）
	Results []AgentToolResultItemIn `json:"results" binding:"required,dive"`
}

// AgentSlotStatusOut 定义单个档位的绑定模型与可用性输出项。
type AgentSlotStatusOut struct {
	// Slot 档位标识（flash/standard/advanced/embedding）
	Slot modelsai.Slot `json:"slot"`
	// ModelName 绑定模型在注册表中的本地标识（无绑定时为空）
	ModelName string `json:"modelName"`
	// ProviderModel 供应商侧的模型名（无绑定时为空）
	ProviderModel string `json:"providerModel"`
	// Kind 绑定模型类型（chat/embedding；无绑定时为空）
	Kind llm.ModelKind `json:"kind"`
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
	// Slots 各档位绑定的模型与可用性（含不可用绑定），按 modelsai.AllSlots 顺序
	Slots []AgentSlotStatusOut `json:"slots"`
	// DefaultSlot 未显式指定档位且会话未记录档位时使用的默认档位（参数 agent.defaultSlot 的
	// 生效值），供前端档位下拉取初值，避免前端自持档位字面量
	DefaultSlot modelsai.Slot `json:"defaultSlot"`
	// ParamOverrideCount 当前值与出厂值不同的行为参数条数（0 表示全部参数仍为出厂值）
	ParamOverrideCount int `json:"paramOverrideCount"`
	// Reason AI 能力不可用的原因说明（按请求语言本地化；可用时为空）
	Reason string `json:"reason"`
}

// newAgentSessionItemOut 将会话模型映射为视图层输出项。
func newAgentSessionItemOut(s *modelsai.AgentSession) AgentSessionItemOut {
	return AgentSessionItemOut{
		ID:        s.ID,
		Title:     s.Title,
		Slot:      s.TargetModel,
		Status:    s.Status,
		AgentKind: s.AgentKind,
		Summary:   string(s.Summary.Value()),
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}
}

// newAgentMessageItemOut 将消息模型映射为视图层输出项：
// 草稿与引用载荷经 JSON 反序列化暴露，空或非法时留空（omitempty 省略）。
func newAgentMessageItemOut(m *modelsai.AgentMessage) AgentMessageItemOut {
	out := AgentMessageItemOut{
		ID:        m.ID,
		Role:      m.Role,
		Content:   string(m.Content.Value()),
		CreatedAt: m.CreatedAt,
	}
	if draft := m.DraftJSON.Value(); len(draft) > 0 {
		out.Draft = json.RawMessage(draft)
	}
	if citationsJSON := m.CitationsJSON.Value(); len(citationsJSON) > 0 {
		var citations []ai.RetrievalResult
		if err := json.Unmarshal(citationsJSON, &citations); err == nil {
			out.Citations = citations
		}
	}
	return out
}

// AgentSessionCreate 处理创建 AI 助手会话的请求。
// 新会话归属当前请求命名空间，初始状态为 active；会话主题由首个用户问题经 LLM 生成，失败时
// 按其截断兜底，绝不阻断创建。可选档位字段经校验后作为该会话的缺省档位落库，可选 Agent 类型
// 字段经校验后作为该会话的归属落库（缺省 knowledge）；两类字段的非法取值均以 400 + 英文错误
// 拒绝（不静默回落）。
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
	slot := modelsai.Slot("")
	if value := modelsai.Slot(strings.TrimSpace(string(in.Slot))); value != "" {
		parsed, ok := modelsai.ParseChatSlot(value)
		if !ok {
			response.CreateBadBodyError(modelsai.InvalidChatSlotError(value)).AbortWithGinH(c)
			return
		}
		slot = parsed
	}
	agentKind, err := modelsai.ParseAgentKind(in.AgentKind)
	if err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	query := strings.TrimSpace(in.Query)
	topic := ai.SessionTopicFallback(query)
	if comps.Memory != nil {
		topic = comps.Memory.GenerateTopic(c, query)
	}
	session, err := comps.Sessions.CreateSession(c, namespace, topic, slot, agentKind)
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

// AgentSessionCompact 处理压缩 AI 助手会话上下文并抽取长期记忆的请求。
// 先校验会话归属（404 语义同详情接口），再经 Memory.Compact 把未摘要消息折叠进滚动摘要并
// 抽取长期记忆；摘要失败以 500 + 英文错误返回，长期记忆抽取/落库失败仅作为 warnings 返回。
func AgentSessionCompact(c *gin.Context) {
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
	result, err := comps.Memory.Compact(c, session)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	warnings := result.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	response.SimpleOK.AbortWithGinW(c, AgentSessionCompactOut{
		Summary:   result.Summary,
		Extracted: result.Extracted,
		Warnings:  warnings,
	})
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
		Slot:          modelsai.Slot(strings.TrimSpace(string(in.Slot))),
		DeclaredSlot:  session.TargetModel,
		ToolReporter:  report,
		// 工具执行请求映射：命中 client 来源工具挂起时，把待执行调用下发给前端，由前端在用户
		// 自身凭据下经审批执行，随后经恢复端点回传结果续跑。
		ToolRequestReporter: func(req ai.ToolRequest) error {
			return sseWriter.SendToolRequest(req)
		},
	})
	if err != nil {
		_ = sseWriter.SendError(err)
		return
	}
	defer reader.Close()
	pumpAgentStream(sseWriter, reader)
}

// pumpAgentStream 消费 Agent 输出流并把事件透传为 SSE：带增量的输出项发 delta 事件，完成输出项
// 发 done 事件并结束，非 EOF 错误发 error 事件并结束；挂起（本次无 done）时流以 io.EOF 正常结束，
// 不发 error（此时前端已收到 tool_request，等待其回传结果后续跑）。供 AgentStream 与恢复端点复用，
// 保证两侧事件语义一致。
func pumpAgentStream(sseWriter *sse.SSEWriter, reader *schema.StreamReader[ai.AgentOutput]) {
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

// AgentToolResult 处理回传客户端工具调用结果并续跑挂起运行的请求（SSE，路由
// /agent/:sessionID/tool-result）。
// 挂起运行由 Agent 命中 client 来源工具时落库，本接口把前端在用户自身凭据下、经人工审批后得到的
// 执行结果回灌，从中断处续跑。流程：
//  1. 校验命名空间、会话 ID、AI 功能与会话归属（404 语义同其它接口，避免泄露资源存在性）；
//  2. 绑定请求体（须覆盖挂起时下发的全部 callId；同一轮多个调用一次性回传）；
//  3. 调用 ResumeRun 认领运行现场（校验归属/状态/过期并原子消费）并重建续跑流；
//  4. 仅当 ResumeRun 成功后才构造 SSE 写手：NewSSEWriter 一旦构造即写入响应头，此后无法再返回
//     JSON 错误，故失败时按 errors.Is 映射业务码以 JSON 中断（此时尚未构造 SSE 写手）；
//  5. 成功后以与流式生成一致的事件语义透传续跑输出（delta/stage/done），再次命中 client 工具时
//     再次下发 tool_request 并以 io.EOF 正常结束。
func AgentToolResult(c *gin.Context) {
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
	if session := getOwnedAgentSession(c, comps, uint(sessionID), namespace); session == nil {
		return
	}
	var in AgentToolResultIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	results := make([]ai.ToolResult, 0, len(in.Results))
	for _, item := range in.Results {
		results = append(results, ai.ToolResult{
			CallID:   item.CallID,
			Approved: item.Approved,
			Result:   item.Result,
			Error:    item.Error,
		})
	}
	// sseWriter 只能在 ResumeRun 成功后构造（构造即写响应头，之后无法再返回 JSON 错误），故先
	// 声明变量，成功后赋值；阶段事件/工具请求闭包内判空后调用，挂起下发与续跑输出均经此写手。
	var sseWriter *sse.SSEWriter
	report := func(ev ai.StageEvent) error {
		if sseWriter == nil {
			return nil
		}
		if ev.Delta != "" {
			return sseWriter.SendStageDelta(string(ev.Stage), ev.Delta)
		}
		return sseWriter.SendStage(ev)
	}
	reader, err := comps.Agent.ResumeRun(c, ai.ResumeInput{
		SessionID:     uint(sessionID),
		NamespaceName: namespace,
		Results:       results,
		ToolReporter:  report,
		ToolRequests: func(req ai.ToolRequest) error {
			if sseWriter == nil {
				return nil
			}
			return sseWriter.SendToolRequest(req)
		},
	})
	if err != nil {
		switch {
		case errors.Is(err, ai.ErrAgentRunNotFound):
			response.CreateCodeError(http.StatusNotFound, response.CodeAgentRunNotFound, nil).AbortWithGinH(c)
		case errors.Is(err, ai.ErrAgentRunForbidden):
			// 归属不符与「不存在」同样返回 404 + 对应业务码，避免泄露运行现场存在性。
			response.CreateCodeError(http.StatusNotFound, response.CodeAgentRunForbidden, nil).AbortWithGinH(c)
		case errors.Is(err, ai.ErrAgentRunConsumed):
			response.CreateCodeError(http.StatusBadRequest, response.CodeAgentRunConsumed, nil).AbortWithGinH(c)
		case errors.Is(err, ai.ErrAgentRunExpired):
			response.CreateCodeError(http.StatusBadRequest, response.CodeAgentRunExpired, nil).AbortWithGinH(c)
		default:
			response.CreateAccidentError(err).AbortWithGinH(c)
		}
		return
	}
	sseWriter = sse.NewSSEWriter(c.Writer)
	defer reader.Close()
	pumpAgentStream(sseWriter, reader)
}

// AgentStatusGet 处理获取 AI 助手状态的请求（不强制启用，始终 200）。
// 从运行时快照读取 AI 能力是否启用、各档位绑定的模型与可用性、生效的参数覆盖数量；
// 未启用时给出明确原因（按请求语言本地化）。启用时额外查询数据库 knowledge_chunks.embedding 与
// agent_memories.embedding 两列的实际维度并与绑定模型维度对比，不一致时提示风险。
func AgentStatusGet(c *gin.Context) {
	rt := middlewares.GetAiRuntime(c)
	snap := rt.Snapshot()
	out := AgentStatusOut{Enabled: rt.Enabled(), Reason: snap.DisabledReason(middlewares.GetLanguage(c))}
	out.DefaultSlot = snap.DefaultSlot()
	slots := snap.Slots()
	out.Slots = make([]AgentSlotStatusOut, 0, len(modelsai.AllSlots))
	for _, slot := range modelsai.AllSlots {
		item := AgentSlotStatusOut{Slot: slot}
		if info, ok := slots[slot]; ok {
			item.ModelName = info.Name
			item.ProviderModel = info.Model
			item.Kind = info.Kind
			item.Enabled = info.Enabled
			item.Bound = info.Bound
		}
		// 可用性取运行时实际构建结果：对话档位查客户端表，向量化槽位查向量化客户端
		if slot == modelsai.SlotEmbedding {
			_, err := rt.Embed()
			item.Available = err == nil
		} else {
			_, err := rt.Chat(slot)
			item.Available = err == nil
		}
		out.Slots = append(out.Slots, item)
	}
	if info, ok := slots[modelsai.SlotStandard]; ok {
		out.ChatModel = info.Model
	}
	if info, ok := slots[modelsai.SlotEmbedding]; ok {
		out.EmbeddingModel = info.Model
	}
	out.EmbeddingDimensions = snap.EmbeddingDimensions()
	out.ParamOverrideCount = snap.ParamOverrideCount()
	if out.Enabled {
		db := middlewares.GetCoreDatabase(c)
		// 维度自检覆盖两列受治理的向量列：knowledge_chunks.embedding 与 agent_memories.embedding；
		// 任一列可读且与绑定模型维度不一致即报出 dimensionMismatch（读不出时交由迁移建列）。
		if n, found, err := ai.ChunkEmbeddingDimension(c, db); err == nil && found && n != out.EmbeddingDimensions {
			out.DimensionMismatch = true
			out.DimensionDetail = fmt.Sprintf("embedding dimension mismatch: knowledge_chunks.embedding column is %d, bound model is %d", n, out.EmbeddingDimensions)
		} else if n, found, err := ai.MemoryEmbeddingDimension(c, db); err == nil && found && n != out.EmbeddingDimensions {
			out.DimensionMismatch = true
			out.DimensionDetail = fmt.Sprintf("embedding dimension mismatch: agent_memories.embedding column is %d, bound model is %d", n, out.EmbeddingDimensions)
		}
	}
	response.SimpleOK.AbortWithGinW(c, out)
}
