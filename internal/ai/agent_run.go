// 本文件承载运行现场（agent_runs）的挂起持久化与恢复：当工具循环命中任一 client 来源工具时，
// 后端把挂起时刻的完整消息链、待执行调用列表、检索引用与档位落库，结束本次流等待前端回传审批
// 决定与执行结果；此后据该现场从中断处续跑。运行现场的读写方法挂在 SessionStore 上，避免改动
// Agent 的装配参数。

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
	"zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
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
	// ErrAgentRunForbidden 运行现场归属不符（命名空间不一致）
	ErrAgentRunForbidden = errors.New("agent run forbidden")
)

// pendingToolCall 是持久化的待执行客户端工具调用：挂起时随运行现场落库，恢复时按原始顺序回灌为
// tool message。字段须保证 JSON 往返稳定（恢复时据此重建调用与结果关联）。
type pendingToolCall struct {
	// CallID 与模型 tool call id 一致
	CallID string `json:"callId"`
	// ToolName 待执行的客户端工具名
	ToolName string `json:"toolName"`
	// Arguments 模型给出的调用入参（原始 JSON 文本）
	Arguments string `json:"arguments"`
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

// ClaimAgentRun 认领一次恢复请求对应的运行现场：取该会话最近一行（不按状态过滤，以便区分「不存在」
// 与「已消费」），依次校验归属、状态与过期，通过后原子置为 consumed（并发双提交只有一方成功）并
// 返回该现场。失败类别见包级哨兵 ErrAgentRunNotFound / ErrAgentRunForbidden / ErrAgentRunConsumed /
// ErrAgentRunExpired，供上层映射业务码。
func (s *SessionStore) ClaimAgentRun(ctx context.Context, sessionID uint, namespaceName string) (*ai.AgentRun, error) {
	var run ai.AgentRun
	err := s.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("id DESC").
		First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAgentRunNotFound
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if run.NamespaceName != namespaceName {
		return nil, ErrAgentRunForbidden
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
func prepareClientToolCall(tc schema.ToolCall, in AgentInput, requests ToolRequestReporter) (*pendingToolCall, string) {
	if in.SessionID == 0 || requests == nil {
		return nil, "client tool calls require a persisted session and an approval channel; this call cannot be executed"
	}
	return &pendingToolCall{
		CallID:    tc.ID,
		ToolName:  tc.Function.Name,
		Arguments: tc.Function.Arguments,
	}, ""
}

// persistSuspendedRun 把挂起时刻的运行现场落库：完整消息链、待执行调用列表、检索引用、档位、归属
// 与过期时间。任一序列化或落库失败返回错误，调用方据此中止挂起。
func (a *Agent) persistSuspendedRun(ctx context.Context, in AgentInput, slot ai.Slot, messages []*schema.Message, collector *knowledgeCollector, pending []pendingToolCall) error {
	messagesJSON, err := json.Marshal(messages)
	if err != nil {
		return errors.WithStack(err)
	}
	pendingJSON, err := json.Marshal(pending)
	if err != nil {
		return errors.WithStack(err)
	}
	citationsJSON, err := json.Marshal(collector.snapshot())
	if err != nil {
		return errors.WithStack(err)
	}
	run := &ai.AgentRun{
		NamespaceName:    in.NamespaceName,
		SessionID:        in.SessionID,
		Status:           ai.AgentRunStatusPending,
		MessagesJSON:     serializer.NewCrypto(messagesJSON),
		PendingCallsJSON: serializer.NewCrypto(pendingJSON),
		CitationsJSON:    serializer.NewCrypto(citationsJSON),
		Slot:             slot,
		ExpiresAt:        time.Now().Add(agentRunTTL),
	}
	return a.store.SaveAgentRun(ctx, run)
}

// ResumeInput 是一次恢复运行的入参。
type ResumeInput struct {
	// SessionID 恢复请求对应的会话 ID
	SessionID uint
	// NamespaceName 恢复请求所属命名空间
	NamespaceName string
	// Results 前端回传的执行结果，按 callId 与挂起时下发的调用关联
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
	run, err := a.store.ClaimAgentRun(ctx, in.SessionID, in.NamespaceName)
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
	// 运行现场未持久化 Agent 类型，尽力从会话回填（失败留空，按缺省类型处理）。
	var agentKind ai.AgentKind
	if session, serr := a.store.GetSession(ctx, run.SessionID); serr == nil {
		agentKind = session.AgentKind
	}
	msgs = append(msgs, buildToolResultMessages(pending, in.Results, a.params.Int("agent.maxToolResultBytes"))...)
	input := AgentInput{
		SessionID:           run.SessionID,
		NamespaceName:       run.NamespaceName,
		AgentKind:           agentKind,
		Slot:                run.Slot,
		ResumeMessages:      msgs,
		ResumeCitations:     citations,
		ToolReporter:        in.ToolReporter,
		ToolRequestReporter: in.ToolRequests,
	}
	// 本轮没有新的用户提问，须走 Stream（不落用户消息）而非 StreamRun。
	reader, err := a.Stream(ctx, input, opts...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return a.wrapPersist(ctx, reader, input), nil
}

// buildToolResultMessages 按待执行调用的原始顺序，把前端回传结果组装为 tool message 列表：未提供
// 结果、用户拒绝、前端执行出错分别回灌固定的英文文案或错误文本，其余回灌执行结果；文本一律按
// maxBytes 截断（客户端工具结果按不可信外部输入处理）。执行错误只作为工具结果回灌，不作为流错误。
func buildToolResultMessages(pending []pendingToolCall, results []ToolResult, maxBytes int) []*schema.Message {
	byCallID := make(map[string]ToolResult, len(results))
	for _, r := range results {
		byCallID[r.CallID] = r
	}
	out := make([]*schema.Message, 0, len(pending))
	for _, call := range pending {
		result, ok := byCallID[call.CallID]
		var text string
		switch {
		case !ok:
			text = "no result was provided for this call"
		case !result.Approved:
			text = "the user declined to execute this call; do not retry it without a clear reason"
		case result.Error != "":
			text = result.Error
		default:
			text = result.Result
		}
		out = append(out, schema.ToolMessage(truncateBytes(text, maxBytes), call.CallID))
	}
	return out
}
