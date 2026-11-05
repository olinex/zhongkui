// 本文件承载 AI 助手领域事件协议下的流式接口：chat / render / action / events / abort。
// 所有服务端输出统一为领域事件（SSE 行），运行与连接解耦：轮次由 turns.Manager 在独立
// goroutine 中执行并写入内存缓冲，HTTP 连接断开只会 detach 订阅而不中止回合。

package ai

import (
	// 标准库
	"net/http"
	"strconv"
	"strings"

	// 第三方
	"github.com/gin-gonic/gin"

	// 项目内部
	ai "zhongkuisec.com/zhongkui/internal/ai"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	interaction "zhongkuisec.com/zhongkui/internal/ai/interaction"
	sessionstore "zhongkuisec.com/zhongkui/internal/ai/session"
	skillstore "zhongkuisec.com/zhongkui/internal/ai/skill"
	"zhongkuisec.com/zhongkui/internal/ai/turns"
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/manager/views"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// agentSessionFromParam 解析 :sessionID 并校验会话归属（命名空间 + 属主）；失败时已中断请求并返回 nil。
func agentSessionFromParam(c *gin.Context, comps *ai.Components, namespace string, userID uint) *modelsai.AgentSession {
	sessionID, ok := views.GetUint64Param(c, "sessionID")
	if !ok {
		return nil
	}
	return getOwnedAgentSession(c, comps, uint(sessionID), namespace, userID)
}

// AgentChat 处理启动（或抢占）一轮对话的请求（SSE，路由 POST /agent/:sessionID/chat）。
// 校验命名空间 / AI 功能 / 属主 / 会话归属后，按会话锁定状态拒绝已达轮次上限的会话；请求可选指定
// 本轮技能（SkillID），非 0 时须存在、属于当前命名空间且已启用，否则以 400 拒绝（绝不静默忽略）；
// 随后把请求装配为 turns.ChatInput 交给 turns.Manager.Chat，运行在独立 goroutine 中执行，本接口只
// 负责订阅并转发事件。客户端断开只 detach 订阅，回合继续执行并写入会话缓冲。
func AgentChat(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	if comps.Turns == nil {
		response.CreateAccidentError(errors.New("ai: the turn manager is not assembled")).AbortWithGinH(c)
		return
	}
	userID, ok := middlewares.RequireUserID(c)
	if !ok {
		return
	}
	session := agentSessionFromParam(c, comps, namespace, userID)
	if session == nil {
		return
	}
	var in AgentChatIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	// 仅轮次上限是硬拒绝；运行中 / 等待中允许发起（抢占旧回合或作废挂起现场）。
	if rejectIfSessionLocked(c, interactionSnapshot(c, comps, session)) {
		return
	}
	// 本轮显式选用技能时先解析引用并校验可用性（内置技能查代码注册表，自定义技能查当前命名
	// 空间且须启用）；任一条不满足即 400 拒绝，绝不静默忽略或降级为「未选技能」。
	chatInput := newChatInput(session, in)
	selection, ok := resolveChatSkill(c, comps, namespace, in)
	if !ok {
		return
	}
	if selection != nil {
		// 技能内容以代码注册表或库中最新值为准，覆盖请求透传的占位字段。
		chatInput.SkillID = selection.ID
		chatInput.SkillName = selection.Name
		chatInput.SkillDescription = selection.Description
		chatInput.SkillContent = selection.Content
	}
	stream, err := comps.Turns.Chat(c.Request.Context(), chatInput)
	if err != nil {
		mapAgentTurnError(c, err)
		return
	}
	streamAssistantEvents(c, stream)
}

// AgentRender 处理会话历史回放的请求（SSE，路由 GET /agent/:sessionID/render）。
// 输出历史快照、已作答交互与当前挂起卡片后结束连接；前端随后可经 /events 订阅后续增量。
func AgentRender(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	if comps.Turns == nil {
		response.CreateAccidentError(errors.New("ai: the turn manager is not assembled")).AbortWithGinH(c)
		return
	}
	userID, ok := middlewares.RequireUserID(c)
	if !ok {
		return
	}
	session := agentSessionFromParam(c, comps, namespace, userID)
	if session == nil {
		return
	}
	stream, err := comps.Turns.RenderAssistant(c.Request.Context(), session.ID, userID, namespace)
	if err != nil {
		mapAgentTurnError(c, err)
		return
	}
	streamAssistantEvents(c, stream)
}

// AgentAction 处理审批 / 问卷动作的请求（SSE，路由 POST /agent/:sessionID/action）。
// 请求体直接绑定 contract.ActionRequest；动作到 ResumeParams 的映射、校验与 pending 现场认领全部由
// turns.Manager.Action 完成，本接口只负责续跑与转发。
func AgentAction(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	if comps.Turns == nil {
		response.CreateAccidentError(errors.New("ai: the turn manager is not assembled")).AbortWithGinH(c)
		return
	}
	userID, ok := middlewares.RequireUserID(c)
	if !ok {
		return
	}
	session := agentSessionFromParam(c, comps, namespace, userID)
	if session == nil {
		return
	}
	var in contract.ActionRequest
	if !bindActionRequest(c, &in) {
		return
	}
	actionInput := newActionInput(session, in)
	stream, err := comps.Turns.Action(c.Request.Context(), actionInput)
	if err != nil {
		mapAgentTurnError(c, err)
		return
	}
	streamAssistantEvents(c, stream)
}

// AgentInteractionReopenIn 定义「重新审批 / 重新回答」的请求输入：审批回退按调用 ID 定位，问卷回退
// 按根中断 ID 定位，二者至少提供其一。
type AgentInteractionReopenIn struct {
	// CallIDs 审批卡片上的工具调用 ID（审批回退使用）：与 InterruptIDs 至少提供其一
	CallIDs []string `json:"callIDs" binding:"required_without=InterruptIDs,dive,notBlank"`
	// InterruptIDs 问卷卡片上的根中断 ID（问卷回退使用）：与 CallIDs 至少提供其一
	InterruptIDs []string `json:"interruptIDs" binding:"required_without=CallIDs,dive,notBlank"`
}

// AgentInteractionReopen 处理「重新审批 / 重新回答」请求（路由 POST /agent/:sessionID/interaction/reopen）。
// 与「重新提问」不同，这里不重跑整轮回答：把会话回退到该次交互挂起的状态——删除该交互之后的回答与
// 更晚交互、把交互重置为待处理、并以挂起时的图现场重建现场，使前端刷新后重新展示批准 / 拒绝按钮
// 或待作答问卷，用户可重选决策 / 重新作答。依次校验命名空间 / AI 功能 / 属主 / 会话归属（404 语义同
// 详情接口）→ 交互状态守卫（仅 `运行中` 以 409 + 1306 拒绝；`暂停` / `已锁定` / `等待中` 放行）→
// 绑定请求体 → 执行回退。目标交互不可回退（快照缺失 / 回退点已被摘要覆盖）返回 404，避免泄露现场
// 存在性。
func AgentInteractionReopen(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	if comps.Turns == nil {
		response.CreateAccidentError(errors.New("ai: the turn manager is not assembled")).AbortWithGinH(c)
		return
	}
	userID, ok := middlewares.RequireUserID(c)
	if !ok {
		return
	}
	session := agentSessionFromParam(c, comps, namespace, userID)
	if session == nil {
		return
	}
	// 回退守卫：仅 `运行中` 拒绝；回退会作废当前更晚的挂起现场（口径与截断一致）。
	if rejectGuardError(c, interaction.CanRerunUserInput(agentInteractionFacts(c, comps, session))) {
		return
	}
	var in AgentInteractionReopenIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	if err := comps.Turns.ReopenInteraction(c.Request.Context(), turns.ReopenInteractionInput{
		SessionID:     session.ID,
		UserID:        userID,
		NamespaceName: session.NamespaceName,
		CallIDs:       in.CallIDs,
		InterruptIDs:  in.InterruptIDs,
	}); err != nil {
		mapAgentTurnError(c, err)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// AgentEvents 处理运行中重连订阅的请求（SSE，路由 GET /agent/:sessionID/events?after=）。
// 先回放缓冲中序号 > after 的消息，再转发后续实时消息；无在途回合时回放后即结束。
func AgentEvents(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	if comps.Turns == nil {
		response.CreateAccidentError(errors.New("ai: the turn manager is not assembled")).AbortWithGinH(c)
		return
	}
	userID, ok := middlewares.RequireUserID(c)
	if !ok {
		return
	}
	session := agentSessionFromParam(c, comps, namespace, userID)
	if session == nil {
		return
	}
	after := 0
	if raw := strings.TrimSpace(c.Query("after")); raw != "" {
		parsed := 0
		if !parseAfterIndex(c, raw, &parsed) {
			return
		}
		after = parsed
	}
	stream, err := comps.Turns.Events(c.Request.Context(), session.ID, userID, namespace, after)
	if err != nil {
		mapAgentTurnError(c, err)
		return
	}
	if stream == nil {
		// 无在途运行：客户端以 /render 的结果为准。
		c.Status(http.StatusNoContent)
		return
	}
	streamAssistantEvents(c, stream)
}

// AgentAbort 处理中止当前回合的请求（路由 POST /agent/:sessionID/abort）。
// turns.Manager.Abort 立即停止 TurnLoop、以 turn.completed(cancelled) 收敛缓冲并作废挂起现场；
// 无在途回合时幂等。
func AgentAbort(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	comps := requireAiEnabled(c)
	if comps == nil {
		return
	}
	if comps.Turns == nil {
		response.CreateAccidentError(errors.New("ai: the turn manager is not assembled")).AbortWithGinH(c)
		return
	}
	userID, ok := middlewares.RequireUserID(c)
	if !ok {
		return
	}
	session := agentSessionFromParam(c, comps, namespace, userID)
	if session == nil {
		return
	}
	if err := comps.Turns.Abort(c.Request.Context(), session.ID, userID, namespace); err != nil {
		mapAgentTurnError(c, err)
		return
	}
	response.SimpleOK.AbortWithGinW(c, AgentCancelOut{Cancelled: true})
}

// newChatInput 把请求与会话身份装配为 turns.ChatInput。
func newChatInput(session *modelsai.AgentSession, in AgentChatIn) turns.ChatInput {
	return turns.ChatInput{
		SessionID:     session.ID,
		UserID:        session.UserID,
		NamespaceName: session.NamespaceName,
		Query:         in.Query,
		DocIDs:        in.DocIDs,
		BusinessData:  contract.BusinessData(in.BusinessData),
		Thinking:      in.Thinking,
		Slot:          modelsai.Slot(strings.TrimSpace(in.Slot)),
		SkillID:       in.SkillID,
	}
}

// chatSkillSelection 是一轮对话最终采用的技能内容：内置技能 ID 为 0，其余三项由代码注册表
// 或库中记录填入，随后覆盖到 turns.ChatInput 的同名字段。
type chatSkillSelection struct {
	// ID 自定义技能 ID；内置技能为 0
	ID uint
	// Name 技能名称（内置取注册表 Title）
	Name string
	// Description 技能说明
	Description string
	// Content 技能正文（写入系统提示词「当前技能」段）
	Content string
}

// resolveChatSkill 解析并校验本轮选用的技能：SkillID 为 0 时返回 (nil, true) 表示未选技能；
// 技能不存在 / 已停用 / 自定义技能不属于当前命名空间 / 内置技能代码正文缺失 / 技能存储缺失时
// 以 400 + 本地化错误中断请求并返回 (nil, false)；其余读取失败以意外错误中断。内置与自定义技能
// 统一按 agent_skills 行 ID 选择，内置正文按行名称取自代码注册表。
func resolveChatSkill(c *gin.Context, comps *ai.Components, namespace string, in AgentChatIn) (*chatSkillSelection, bool) {
	if in.SkillID == 0 {
		return nil, true
	}
	if comps.Skills == nil {
		response.CreateBadBodyMessageError("aiSkillUnavailable").AbortWithGinH(c)
		return nil, false
	}
	item, getErr := comps.Skills.GetByID(c.Request.Context(), in.SkillID)
	if getErr != nil {
		if errors.Is(getErr, skillstore.ErrSkillNotFound) {
			response.CreateBadBodyMessageError("aiSkillUnavailable").AbortWithGinH(c)
			return nil, false
		}
		response.CreateAccidentError(getErr).AbortWithGinH(c)
		return nil, false
	}
	if !item.Enabled {
		response.CreateBadBodyMessageError("aiSkillUnavailable").AbortWithGinH(c)
		return nil, false
	}
	switch item.Source {
	case modelsai.SkillSourceBuiltin:
		spec, declared := skillstore.LookupBuiltin(item.Name)
		if !declared {
			// 注册行存在但代码正文缺失属于编程缺陷（守卫测试拦截）；运行期按不可用处理。
			response.CreateBadBodyMessageError("aiSkillUnavailable").AbortWithGinH(c)
			return nil, false
		}
		return &chatSkillSelection{
			ID:          item.ID,
			Name:        item.Name,
			Description: item.Description,
			Content:     spec.Content,
		}, true
	case modelsai.SkillSourceCustom:
		if item.NamespaceName != strings.TrimSpace(namespace) {
			response.CreateBadBodyMessageError("aiSkillUnavailable").AbortWithGinH(c)
			return nil, false
		}
		return &chatSkillSelection{
			ID:          item.ID,
			Name:        item.Name,
			Description: item.Description,
			Content:     string(item.Content.Value()),
		}, true
	default:
		response.CreateBadBodyMessageError("aiSkillUnavailable").AbortWithGinH(c)
		return nil, false
	}
}

// newActionInput 把审批 / 问卷动作与会话身份装配为 turns.ActionInput。
func newActionInput(session *modelsai.AgentSession, action contract.ActionRequest) turns.ActionInput {
	return turns.ActionInput{
		SessionID:     session.ID,
		UserID:        session.UserID,
		NamespaceName: session.NamespaceName,
		Action:        action,
	}
}

// bindActionRequest 绑定动作请求体；绑定失败时以本地化 400 中断并返回 false。
func bindActionRequest(c *gin.Context, out *contract.ActionRequest) bool {
	if err := c.ShouldBindBodyWithJSON(out); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return false
	}
	return true
}

// parseAfterIndex 解析 events 的 after 查询参数（非负整数）；非法时以本地化 400 中断并返回 false。
func parseAfterIndex(c *gin.Context, raw string, out *int) bool {
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		response.CreateBadQueryError("badUrlParam", "after", err).AbortWithGinH(c)
		return false
	}
	if parsed < 0 {
		response.CreateBadQueryError("badUrlParam", "after", raw).AbortWithGinH(c)
		return false
	}
	*out = parsed
	return true
}

// mapAgentTurnError 把回落 / 动作 / 中止失败映射为本地化业务错误：
//   - 空提问与非法动作 → 400 请求体错误；
//   - 无挂起现场 / checkpoint 缺失 / 动作无可续跑目标 → 404（避免泄露现场存在性）；
//   - 会话不存在 → 404；其余归为意外错误。
func mapAgentTurnError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, turns.ErrNoQuery), errors.Is(err, contract.ErrInvalidAction):
		response.CreateBadBodyError(err).AbortWithGinH(c)
	case errors.Is(err, turns.ErrNoPendingRun),
		errors.Is(err, turns.ErrCheckpointMissing),
		errors.Is(err, turns.ErrRedoUnavailable),
		errors.Is(err, turns.ErrNoResumeTargets):
		response.CreateCodeError(http.StatusNotFound, response.CodeAgentRunNotFound, nil).AbortWithGinH(c)
	case errors.Is(err, sessionstore.ErrSessionNotFound):
		response.ErrDefaultPageNotFound.AbortWithGinH(c)
	default:
		response.CreateAccidentError(err).AbortWithGinH(c)
	}
}
