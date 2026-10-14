// 本文件声明客户端工具调用的审核重要级（AuditLevel）取值空间，并实现其与会话级审核阀门
// （reviewValve）的合成判定。重要级由评估子系统在 client 工具挂起前产出，合成结果
// approvalRequired 是「这一次 client 工具调用是否需要人工审核」的唯一事实来源。取值空间的
// 唯一来源是本文件内的类型化常量，取值序列在 enum.go 复用为枚举类 auditLevel 的条目来源。

package ai

import (
	// 标准库
	"context"
	"encoding/json"
	"strings"

	// 第三方
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// AuditLevel 是客户端工具调用的审核重要级标识：由后端评估子系统在挂起前对本次全部待执行
// 调用分类产出，取值 must / suggest / auto / none，依次对应「必须审核 / 建议审核 / 可自动审核 /
// 无需审核」。与会话级审核阀门（ReviewValve）合成后决定调用是否需人工审核。
type AuditLevel string

// 审核重要级常量。
const (
	// AuditLevelMust 必须审核
	AuditLevelMust AuditLevel = "must"
	// AuditLevelSuggest 建议审核
	AuditLevelSuggest AuditLevel = "suggest"
	// AuditLevelAuto 可自动审核
	AuditLevelAuto AuditLevel = "auto"
	// AuditLevelNone 无需审核
	AuditLevelNone AuditLevel = "none"
)

// NeedsHumanReview 判定一次 client 工具调用是否需要人工审核：以审核重要级（level）与会话级
// 审核阀门（valve）合成，规则严格遵循 spec 判定矩阵（人工=需人工审核，自动=可自动执行）：
//
//	重要级 \ 阀门   each   auto   none
//	must            人工   人工   人工
//	suggest         人工   人工   自动
//	auto            人工   自动   自动
//	none            人工   自动   自动
//
// 即：must 为绝对下界，任何阀门下均需人工；each 为绝对下界，任何重要级下均需人工；
// auto 阀门下仅 auto / none 重要级可自动；none 阀门下仅 must 需人工。
func NeedsHumanReview(level AuditLevel, valve ai.ReviewValve) bool {
	return level == AuditLevelMust || valve == ai.ReviewValveEach ||
		(valve == ai.ReviewValveAuto && level != AuditLevelAuto && level != AuditLevelNone)
}

// evaluateAuditLevels 在一次运行命中 client 来源工具、准备挂起之前，对本次全部待执行调用做一次
// 独立的批量分类，产出与 calls 顺序对齐、长度相等的审核重要级。评估用对话模型优先取 flash 档位
// （a.resolver(ai.SlotFlash)），解析失败回退本次运行档位；两者都失败、模型调用报错、响应为空、
// 解析失败、条数不匹配或档位非法时，对应调用一律保守回落 AuditLevelMust（必须审核），并记录告警，
// 绝不中断本次挂起。返回长度恒等于 len(calls)。
func (a *Agent) evaluateAuditLevels(ctx context.Context, runSlot ai.Slot, calls []pendingToolCall) []AuditLevel {
	levels := make([]AuditLevel, len(calls))
	for i := range levels {
		levels[i] = AuditLevelMust
	}
	if len(calls) == 0 {
		return levels
	}
	chat, err := a.resolveAuditChat(runSlot)
	if err != nil {
		a.warnf(ctx, "agent: audit level evaluation skipped, falling back to must for all calls: %v", err)
		return levels
	}
	messages := []*schema.Message{
		schema.SystemMessage(a.params.Text("agent.auditLevelSystemPrompt")),
		schema.UserMessage(a.buildAuditLevelPrompt(calls)),
	}
	resp, err := chat.Generate(ctx, messages)
	if err != nil {
		a.warnf(ctx, "agent: audit level evaluation failed, falling back to must for all calls: %v", err)
		return levels
	}
	if resp == nil || strings.TrimSpace(resp.Content) == "" {
		a.warnf(ctx, "agent: audit level evaluation returned an empty response, falling back to must for all calls")
		return levels
	}
	items, err := parseAuditLevels(resp.Content)
	if err != nil {
		a.warnf(ctx, "agent: cannot parse the audit level response, falling back to must for all calls: %v", err)
		return levels
	}
	if len(items) != len(calls) {
		a.warnf(ctx, "agent: audit level response carries %d entries for %d calls, falling back to must for unmatched calls", len(items), len(calls))
	}
	byCallID := make(map[string]AuditLevel, len(items))
	for _, item := range items {
		level, ok := parseAuditLevelLiteral(item.Level)
		if !ok {
			a.warnf(ctx, "agent: audit level response carries an invalid level %q for call %q, falling back to must", item.Level, item.CallID)
			continue
		}
		byCallID[item.CallID] = level
	}
	for i := range calls {
		level, ok := byCallID[calls[i].CallID]
		if !ok {
			a.warnf(ctx, "agent: no valid audit level was resolved for call %q, falling back to must", calls[i].CallID)
			continue
		}
		levels[i] = level
	}
	return levels
}

// resolveAuditChat 解析评估用对话客户端：优先 flash 档位，失败回退本次运行档位；两者都失败返回
// 英文错误（由 evaluateAuditLevels 转为全量回落 must 并告警）。
func (a *Agent) resolveAuditChat(runSlot ai.Slot) (model.BaseChatModel, error) {
	if a.resolver == nil {
		return nil, errors.New("agent: nil chat resolver")
	}
	if chat, err := a.resolver(ai.SlotFlash); err == nil && chat != nil {
		return chat, nil
	}
	if chat, err := a.resolver(runSlot); err == nil && chat != nil {
		return chat, nil
	}
	return nil, errors.Newf("agent: no chat model resolved for slot %q", runSlot)
}

// buildAuditLevelPrompt 构造评估请求的用户消息：逐条列出本次全部待执行调用（client 与 backend
// 皆可），每条含 callID、工具名、工具用途说明与模型给出的入参原文 JSON（后端不做解释）。工具用途
// 说明先查 client 工具 schema 注册表（clientToolInfo 的 Desc），未命中再查本次装配的 builtin / MCP
// 工具 schema 索引（a.toolInfoIndex 的 Desc），供评估模型据以判断副作用——仅有工具名时模型只能凭字面猜测；
// 两处均未命中或说明为空时省略该行，不污染评估上下文。
func (a *Agent) buildAuditLevelPrompt(calls []pendingToolCall) string {
	var sb strings.Builder
	sb.WriteString("待评估的工具调用：\n")
	for _, call := range calls {
		sb.WriteString("- callID: ")
		sb.WriteString(call.CallID)
		sb.WriteString("\n  toolName: ")
		sb.WriteString(call.ToolName)
		if desc := a.toolDescription(call.ToolName); desc != "" {
			sb.WriteString("\n  description: ")
			sb.WriteString(desc)
		}
		sb.WriteString("\n  arguments: ")
		sb.WriteString(call.Arguments)
		sb.WriteString("\n")
	}
	return sb.String()
}

// toolDescription 按工具名解析用途说明：先查 client 工具 schema 注册表，未命中再查本次装配的
// builtin / MCP 工具 schema 索引（a.toolInfoIndex）；两处均未命中或说明为空时返回空串。
func (a *Agent) toolDescription(name string) string {
	if info, ok := clientToolInfo(name); ok && strings.TrimSpace(info.Desc) != "" {
		return info.Desc
	}
	if info, ok := a.toolInfoIndex[name]; ok && info != nil && strings.TrimSpace(info.Desc) != "" {
		return info.Desc
	}
	return ""
}

// auditLevelItem 是评估模型输出的单条档位（JSON 形状）。
type auditLevelItem struct {
	// CallID 与待执行调用的 callID 一致，用于对齐回填
	CallID string `json:"callID"`
	// Level 审核重要级字面量（must / suggest / auto / none）
	Level string `json:"level"`
}

// auditLevelPayload 是评估模型输出的 JSON 形状：{"levels":[{"callID":"...","level":"..."}]}。
type auditLevelPayload struct {
	Levels []auditLevelItem `json:"levels"`
}

// parseAuditLevels 容错解析评估模型输出：先去掉 ```json 代码围栏，再按 {"levels":[...]} 解析，
// 失败时退回裸数组解析（容忍仅有 levels 数组的形式）。空白响应返回英文错误。
func parseAuditLevels(content string) ([]auditLevelItem, error) {
	trimmed := stripJSONFence(content)
	if strings.TrimSpace(trimmed) == "" {
		return nil, errors.New("agent: audit level response is empty")
	}
	var payload auditLevelPayload
	if err := json.Unmarshal([]byte(trimmed), &payload); err == nil && payload.Levels != nil {
		return payload.Levels, nil
	}
	var items []auditLevelItem
	if err := json.Unmarshal([]byte(trimmed), &items); err != nil {
		return nil, errors.WithStack(err)
	}
	return items, nil
}

// parseAuditLevelLiteral 校验字面量是否属于四档重要级取值空间：命中返回对应常量与 true，
// 否则返回 (AuditLevelMust, false)，由调用方回落 must 并告警。
func parseAuditLevelLiteral(literal string) (AuditLevel, bool) {
	switch AuditLevel(strings.TrimSpace(literal)) {
	case AuditLevelMust:
		return AuditLevelMust, true
	case AuditLevelSuggest:
		return AuditLevelSuggest, true
	case AuditLevelAuto:
		return AuditLevelAuto, true
	case AuditLevelNone:
		return AuditLevelNone, true
	default:
		return AuditLevelMust, false
	}
}
