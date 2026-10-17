// 本文件承载运行现场（agent_runs）的挂起持久化与恢复：当工具循环命中任一 client 来源工具、
// 或后端工具被评估为 must 级审核时，后端把挂起时刻的完整消息链、待执行调用列表、检索引用与档位
// 落库，结束本次流等待前端回传审批决定与执行结果；此后据该现场从中断处续跑——已批准的 backend
// 调用由后端自行执行，client 调用沿用前端回传结果。运行现场的读写方法挂在 SessionStore 上，
// 避免改动 Agent 的装配参数。

package ai

import (
	// 标准库
	"context"
	"encoding/json"
	"time"

	// 第三方
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// agentRunTTL 是运行现场的存活期限：挂起后超过该时限的现场不可恢复，前端据此提示本次调用已超时。
const agentRunTTL = 10 * time.Minute

// 包级错误哨兵：运行现场恢复的失败类别，供上层 errors.Is 映射业务码。
var (
	// ErrAgentRunNotFound 运行现场不存在
	ErrAgentRunNotFound = errors.New("agent run not found")
	// ErrAgentRunConsumed 运行现场已被消费（重复恢复）
	ErrAgentRunConsumed = errors.New("agent run already consumed")
	// ErrAgentRunExpired 运行现场已过期
	ErrAgentRunExpired = errors.New("agent run expired")
)

// ToolExecOwner 标识一次挂起工具调用的执行归属：client 由前端在用户自身凭据下执行，backend 由
// 后端在恢复时自行执行（builtin / MCP 工具，仅在审核重要级为 must 时挂起等审批）。
type ToolExecOwner string

// 执行归属常量。
const (
	// ToolExecOwnerClient 前端执行（既有 client 工具语义）
	ToolExecOwnerClient ToolExecOwner = "client"
	// ToolExecOwnerBackend 后端执行（builtin / MCP 工具，经 must 级审批后由后端执行）
	ToolExecOwnerBackend ToolExecOwner = "backend"
)

// pendingToolCall 是持久化的待执行工具调用：挂起时随运行现场落库，恢复时按原始顺序回灌为
// tool message。字段须保证 JSON 往返稳定（恢复时据此重建调用与结果关联）。
type pendingToolCall struct {
	// CallID 与模型 tool call id 一致
	CallID string `json:"callID"`
	// ToolName 待执行的工具名
	ToolName string `json:"toolName"`
	// Arguments 模型给出的调用入参（原始 JSON 文本）
	Arguments string `json:"arguments"`
	// ExecOwner 执行归属（client / backend）；兼容旧现场：空字符串一律按 client 处理
	ExecOwner ToolExecOwner `json:"execOwner"`
	// AuditLevel 本条调用的审核重要级（挂起前批量评估产出，随运行现场落库留痕）
	AuditLevel AuditLevel `json:"auditLevel"`
	// ApprovalRequired 由审核重要级与会话阀门合成得出的最终结论：true 表示需人工审核
	ApprovalRequired bool `json:"approvalRequired"`
	// Round 该次调用所属的轮次（从 1 起，挂起前由工具循环写入）；恢复续跑时据此让阶段事件沿用
	// 同一轮次，使同一轮内并发发起的调用在前端仍聚为一组
	Round int `json:"round,omitempty"`
}

// SaveAgentRun 创建一行运行现场（状态为 pending），承载挂起时刻的运行现场。
func (s *SessionStore) SaveAgentRun(ctx context.Context, run *ai.AgentRun) error {
	if run == nil {
		return errors.New("session store: nil agent run")
	}
	if err := s.db.WithContext(ctx).Create(run).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// ClaimAgentRun 认领一次恢复请求对应的运行现场：以（会话 ID、命名空间、属主用户）为查询条件，优先
// 取该会话最新的 pending 现场，无 pending 时再取最新一行用于区分「不存在 / 已消费 / 已过期」——回退
// （reanswer）会把较早的问卷现场重置为 pending 并把更晚的现场置为已消费，故必须按状态优先而非单纯
// 按 id 取最新，否则会认领到已消费的现场。归属不符的行在查询阶段即被过滤，表现为「不存在」；随后
// 依次校验状态与过期，通过后原子置为 consumed（并发双提交只有一方成功）并返回该现场。失败类别见
// 包级哨兵 ErrAgentRunNotFound / ErrAgentRunConsumed / ErrAgentRunExpired，供上层映射业务码。
func (s *SessionStore) ClaimAgentRun(ctx context.Context, sessionID uint, namespaceName string, userID uint) (*ai.AgentRun, error) {
	scope := "session_id = ? AND namespace_name = ? AND user_id = ?"
	var run ai.AgentRun
	err := s.db.WithContext(ctx).
		Where(scope+" AND status = ?", sessionID, namespaceName, userID, ai.AgentRunStatusPending).
		Order("id DESC").
		First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// 无 pending 现场：取最新一行只为区分「已消费 / 已过期 / 不存在」，不参与认领。
		err = s.db.WithContext(ctx).
			Where(scope, sessionID, namespaceName, userID).
			Order("id DESC").
			First(&run).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgentRunNotFound
		}
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if run.Status != ai.AgentRunStatusPending {
		return nil, ErrAgentRunConsumed
	}
	if time.Now().After(run.ExpiresAt) {
		// 过期现场尽力置为 expired，失败仅忽略（不影响「已过期」这一结论）。
		_ = s.db.WithContext(ctx).Model(&ai.AgentRun{}).
			Where("id = ?", run.ID).
			Update("status", ai.AgentRunStatusExpired).Error
		return nil, ErrAgentRunExpired
	}
	res := s.db.WithContext(ctx).Model(&ai.AgentRun{}).
		Where("id = ? AND status = ?", run.ID, ai.AgentRunStatusPending).
		Update("status", ai.AgentRunStatusConsumed)
	if res.Error != nil {
		return nil, errors.WithStack(res.Error)
	}
	if res.RowsAffected == 0 {
		return nil, ErrAgentRunConsumed
	}
	return &run, nil
}

// prepareClientToolCall 处理一次 client 来源工具调用：仅当本次运行有持久化会话且具备审批通道时，
// 原样登记待执行调用（工具名与入参文本，后端不做解释），待本轮结束统一挂起；否则返回回灌给模型的
// 英文错误文本，此时不产生任何 ToolRequest。
func prepareClientToolCall(tc schema.ToolCall, in AgentInput, round int, requests ToolRequestReporter) (*pendingToolCall, string) {
	if in.SessionID == 0 || requests == nil {
		return nil, "client tool calls require a persisted session and an approval channel; this call cannot be executed"
	}
	return &pendingToolCall{
		CallID:    tc.ID,
		ToolName:  tc.Function.Name,
		Arguments: tc.Function.Arguments,
		ExecOwner: ToolExecOwnerClient,
		Round:     round,
	}, ""
}

// persistSuspendedRun 把挂起时刻的运行现场落库：完整消息链、待执行调用列表、检索引用、档位、归属
// 与过期时间。任一序列化或落库失败返回错误，调用方据此中止挂起。返回值为新落库现场的 ID，供问卷
// 挂起在写好锚点消息行后把其 ID 回写现场（见 SetAgentRunAnchor）。
func (a *Agent) persistSuspendedRun(ctx context.Context, in AgentInput, slot ai.Slot, messages []*schema.Message, collector *knowledgeCollector, pending []pendingToolCall) (uint, error) {
	messagesJSON, err := json.Marshal(messages)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	pendingJSON, err := json.Marshal(pending)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	citationsJSON, err := json.Marshal(collector.snapshot())
	if err != nil {
		return 0, errors.WithStack(err)
	}
	run := &ai.AgentRun{
		NamespaceName:    in.NamespaceName,
		UserID:           in.UserID,
		SessionID:        in.SessionID,
		Status:           ai.AgentRunStatusPending,
		MessagesJSON:     serializer.NewCrypto(messagesJSON),
		PendingCallsJSON: serializer.NewCrypto(pendingJSON),
		CitationsJSON:    serializer.NewCrypto(citationsJSON),
		Slot:             slot,
		ExpiresAt:        time.Now().Add(agentRunTTL),
	}
	if err := a.store.SaveAgentRun(ctx, run); err != nil {
		return 0, err
	}
	return run.ID, nil
}

// SetAgentRunAnchor 把挂起锚点消息 ID 回写到运行现场：挂起时 wrapPersist 写好承载本回合最终回答（问卷
// 挂起还承载问卷载荷）的助手消息行后调用，使续跑能原地终态化该行、回退接口能据锚点截断其后消息。现场
// 不存在（含归属不符）时返回 ErrAgentRunNotFound。
func (s *SessionStore) SetAgentRunAnchor(ctx context.Context, runID, anchorMessageID uint) error {
	if runID == 0 {
		return errors.New("session store: unidentified agent run")
	}
	res := s.db.WithContext(ctx).Model(&ai.AgentRun{}).
		Where("id = ?", runID).
		Update("anchor_message_id", anchorMessageID)
	if res.Error != nil {
		return errors.WithStack(res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrAgentRunNotFound
	}
	return nil
}

// ResumeInput 是一次恢复运行的入参。
type ResumeInput struct {
	// SessionID 恢复请求对应的会话 ID
	SessionID uint
	// NamespaceName 恢复请求所属命名空间
	NamespaceName string
	// UserID 恢复请求的属主用户 ID（users.id），用于跨用户隔离
	UserID uint
	// Results 前端回传的执行结果，按 callID 与挂起时下发的调用关联
	Results []ToolResult
	// ToolReporter 工具调用阶段事件的上报回调（可空）
	ToolReporter EvalReporter
	// ToolRequests 再次挂起时的客户端工具调用请求上报回调（可空）
	ToolRequests ToolRequestReporter
}

// ResumeRun 恢复一次因客户端工具调用挂起的运行：认领运行现场（校验归属/状态/过期并原子
// 消费），把前端回传结果按待执行调用的原始顺序回灌为 tool message，随后不经新的用户提问直接重入
// 生成节点续跑，并以同一套 SSE 事件继续输出；恢复后的最终 done 由 wrapPersist 落库为助手消息。
// 认领失败直接返回对应哨兵错误（不存在/归属不符/已消费/已过期），上层据此映射业务码，本函数不
// 额外包装以免破坏 errors.Is 语义。
func (a *Agent) ResumeRun(ctx context.Context, in ResumeInput, opts ...compose.Option) (*schema.StreamReader[AgentOutput], error) {
	// 续跑同样是一次完整回答，耗时自进入本函数起算，token 用量经回调累加，随最终 done 落库。
	metrics := newRunMetrics()
	run, err := a.store.ClaimAgentRun(ctx, in.SessionID, in.NamespaceName, in.UserID)
	if err != nil {
		return nil, err
	}
	var msgs []*schema.Message
	if err := json.Unmarshal(run.MessagesJSON.Value(), &msgs); err != nil {
		return nil, errors.WithStack(err)
	}
	var pending []pendingToolCall
	if err := json.Unmarshal(run.PendingCallsJSON.Value(), &pending); err != nil {
		return nil, errors.WithStack(err)
	}
	var citations []RetrievalResult
	if err := json.Unmarshal(run.CitationsJSON.Value(), &citations); err != nil {
		return nil, errors.WithStack(err)
	}
	// 运行现场未持久化 Agent 类型，尽力从会话回填（失败留空，按缺省类型处理）；同一会话的
	// 审核阀门也在此取回，使续跑新产生的调用与首次挂起使用同一阀门。
	var agentKind ai.AgentKind
	var reviewValve ai.ReviewValve
	if session, serr := a.store.GetSession(ctx, run.SessionID, run.NamespaceName, run.UserID); serr == nil {
		agentKind = session.AgentKind
		reviewValve = session.ReviewValve
	}
	input := AgentInput{
		SessionID:           run.SessionID,
		NamespaceName:       run.NamespaceName,
		UserID:              run.UserID,
		AgentKind:           agentKind,
		Slot:                run.Slot,
		ToolReporter:        in.ToolReporter,
		ToolRequestReporter: in.ToolRequests,
		ReviewValve:         reviewValve,
	}
	// 挂起续跑：本次运行的助手消息行就是挂起时写下的锚点行（run.AnchorMessageID），终态化须原地写回该行
	// 而非新增。问卷挂起时取回锚点行上的问卷载荷（题目 + 上次答案）并把本次回传的答案并入，使终态化后该行
	// 仍保留问卷卡片数据；非问卷挂起（锚点行无问卷载荷）时仅为复用该行，不并入任何问卷数据。
	input.Questionnaire = &questionnaireState{anchorID: run.AnchorMessageID}
	if run.AnchorMessageID != 0 {
		if payload := a.loadAnchorQuestionnaire(ctx, run.AnchorMessageID, run.UserID); payload != nil {
			if answers := questionnaireAnswersFromResults(pending, in.Results); answers != nil {
				payload.Answers = answers
			}
			input.Questionnaire.payload = payload
		}
	}
	// 恢复时后端须能执行已批准的 backend 调用：注入与 generate 相同的工具上下文（记忆 + 知识
	// 检索），并以采集器承接 backend knowledge_search 的新命中（执行后并入 citations 续跑产出）。
	collector := &knowledgeCollector{results: citations}
	toolCtx := a.memoryToolContext(ctx, input)
	toolCtx = a.knowledgeToolContext(toolCtx, input, collector)
	msgs = append(msgs, a.buildResumeToolMessages(ctx, toolCtx, pending, in.Results, a.params.Int("agent.maxToolResultBytes"), in.ToolReporter)...)
	citations = collector.snapshot()
	input.ResumeMessages = msgs
	input.ResumeCitations = citations
	// 本轮没有新的用户提问，须走 Stream（不落用户消息）而非 StreamRun。
	reader, err := a.Stream(ctx, input, append(opts, compose.WithCallbacks(metrics.handler()))...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return a.wrapPersist(ctx, reader, input, metrics), nil
}

// buildResumeToolMessages 按待执行调用的原始顺序，把恢复现场组装为 tool message 列表：
//   - client（owner 为空或 client）：既有逻辑——未提供结果、用户拒绝、前端执行出错分别回灌固定的
//     英文文案或错误文本，其余回灌执行结果；文本一律按 maxBytes 截断（客户端工具结果按不可信外部
//     输入处理）。
//   - backend：已批准（Approved=true）时由后端用 a.runTool 执行该工具，成功取截断结果、失败取
//     "tool execution failed: "+err；未批准或无结果时回灌「用户拒绝」固定英文文案（客户端工具拒绝
//     文案同一风格）。执行经 reporter 上报 toolResult 阶段的 start / done / failed（reporter 为 nil
//     时不上报，上报失败不影响回灌）。
//
// 执行错误只作为工具结果回灌，不作为流错误。
func (a *Agent) buildResumeToolMessages(ctx, toolCtx context.Context, pending []pendingToolCall, results []ToolResult, maxBytes int, reporter EvalReporter) []*schema.Message {
	byCallID := make(map[string]ToolResult, len(results))
	for _, r := range results {
		byCallID[r.CallID] = r
	}
	out := make([]*schema.Message, 0, len(pending))
	for _, call := range pending {
		result, ok := byCallID[call.CallID]
		if call.execOwner() == ToolExecOwnerBackend {
			var text string
			if ok && result.Approved {
				text = a.execBackendTool(toolCtx, call, reporter)
			} else {
				text = declinedToolCallText
			}
			out = append(out, schema.ToolMessage(truncateBytes(text, maxBytes), call.CallID))
			continue
		}
		var text string
		switch {
		case !ok:
			text = "no result was provided for this call"
		case !result.Approved:
			text = declinedToolCallText
		case result.Error != "":
			text = result.Error
		default:
			text = result.Result
		}
		// client 分支同 backend 分支一样上报 toolResult 终态，使前端该次调用行从「发起」收敛
		// 为 done / failed（而非停在「发起」态）。
		if reporter != nil {
			if result.Error != "" {
				_ = reporter(StageEvent{Stage: EvalStageToolResult, Status: EvalStatusFailed, Tool: call.ToolName, CallID: call.CallID, Round: call.Round, Err: result.Error})
			} else {
				_ = reporter(StageEvent{Stage: EvalStageToolResult, Status: EvalStatusDone, Tool: call.ToolName, CallID: call.CallID, Round: call.Round})
			}
		}
		out = append(out, schema.ToolMessage(truncateBytes(text, maxBytes), call.CallID))
	}
	return out
}

// execBackendTool 由后端执行一次已批准的 backend 工具调用：成功返回其结果文本、失败返回
// "tool execution failed: "+err（与 generate 的行内执行一致）；执行前后经 reporter 上报
// toolResult 阶段的 start / done / failed（reporter 为 nil 时跳过，上报失败不改变执行结果）。
func (a *Agent) execBackendTool(toolCtx context.Context, call pendingToolCall, reporter EvalReporter) string {
	name := call.ToolName
	if reporter != nil {
		_ = reporter(StageEvent{Stage: EvalStageToolResult, Status: EvalStatusStart, Tool: name, CallID: call.CallID, Round: call.Round})
	}
	start := time.Now()
	result, err := a.runTool(toolCtx, schema.ToolCall{ID: call.CallID, Function: schema.FunctionCall{Name: call.ToolName, Arguments: call.Arguments}})
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		if reporter != nil {
			_ = reporter(StageEvent{Stage: EvalStageToolResult, Status: EvalStatusFailed, Tool: name, CallID: call.CallID, Round: call.Round, ElapsedMs: elapsed, Err: err.Error()})
		}
		return "tool execution failed: " + err.Error()
	}
	if reporter != nil {
		_ = reporter(StageEvent{Stage: EvalStageToolResult, Status: EvalStatusDone, Tool: name, CallID: call.CallID, Round: call.Round, ElapsedMs: elapsed})
	}
	return result
}

// execOwner 返回调用的执行归属：兼容旧运行现场（无该字段）——空字符串一律按 client 处理。
func (c pendingToolCall) execOwner() ToolExecOwner {
	if c.ExecOwner == "" {
		return ToolExecOwnerClient
	}
	return c.ExecOwner
}

// loadAnchorQuestionnaire 取回问卷锚点消息行上的问卷载荷（题目与上次答案）；行不存在、未携带问卷载荷
// 或载荷无法解码时返回 nil（此时续跑按「无问卷载荷」处理，不写回问卷）。
func (a *Agent) loadAnchorQuestionnaire(ctx context.Context, messageID, userID uint) *QuestionnairePayload {
	msg, err := a.store.GetMessage(ctx, messageID, userID)
	if err != nil || msg.QuestionnaireJSON == nil {
		return nil
	}
	var payload QuestionnairePayload
	if err := json.Unmarshal(msg.QuestionnaireJSON.Value(), &payload); err != nil {
		a.warnf(ctx, "agent: decoding the questionnaire anchor payload failed: %v", err)
		return nil
	}
	return &payload
}

// questionnaireAnswersFromResults 从恢复现场回传的结果中取出问卷答案：按 pending 原序遍历**全部**问卷类
// 调用（一轮内可有多条 ask_user 调用，其题目在挂起时已合并为一张问卷），逐条取回其 Result（前端回传的
// {"answers":[...]} JSON）并把答案依序合并。未回传结果、用户取消（Approved=false）、执行出错、结果为空
// 或载荷非法的调用被跳过；全部调用都取不到答案时返回 nil——此时锚点行只保留题目（无答案），与「取消
// 回答」的语义一致。
func questionnaireAnswersFromResults(pending []pendingToolCall, results []ToolResult) []QuestionnaireAnswer {
	byCallID := make(map[string]ToolResult, len(results))
	for _, r := range results {
		byCallID[r.CallID] = r
	}
	var answers []QuestionnaireAnswer
	for _, call := range pending {
		if !isQuestionnaireTool(call.ToolName) {
			continue
		}
		result, ok := byCallID[call.CallID]
		if !ok || !result.Approved || result.Error != "" || result.Result == "" {
			continue
		}
		var payload struct {
			Answers []QuestionnaireAnswer `json:"answers"`
		}
		if err := json.Unmarshal([]byte(result.Result), &payload); err != nil {
			continue
		}
		answers = append(answers, payload.Answers...)
	}
	if len(answers) == 0 {
		return nil
	}
	return answers
}

// declinedToolCallText 是用户拒绝执行一次挂起调用时回灌给模型的固定英文文案（client / backend 共用）。
const declinedToolCallText = "the user declined to execute this call; do not retry it without a clear reason"
