package audit

import (
	// 标准库
	"context"
	"encoding/json"
	"strings"

	// 第三方
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/config"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/port"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
)

// ChatResolver 按档位解析对话模型。
type ChatResolver = func(slot modelsai.Slot) (model.BaseChatModel, error)

// Deps 是评估器的装配依赖。
type Deps struct {
	// Chats 按档位解析对话模型（可空：为空时全部调用回落 must）
	Chats ChatResolver
	// Params 行为参数读取器（取评估提示词）
	Params *config.Params
	// Logger 日志记录器（可空）
	Logger *logging.Logger
	// ToolInfos 本次装配的 builtin / MCP 工具 schema 索引（按名取用途说明）
	ToolInfos map[string]*schema.ToolInfo
}

// Evaluator 对一批待执行工具调用做独立的重要性分级：优先使用 flash 档位模型，失败时一律保守
// 回落 must，绝不中断调用方。
type Evaluator struct {
	// chats 按档位解析对话客户端的回调
	chats ChatResolver
	// params 行为参数读取器
	params *config.Params
	// logger 日志记录器（可空）
	logger *logging.Logger
	// toolInfos 工具 schema 索引（按名取用途说明）
	toolInfos map[string]*schema.ToolInfo
}

// New 构造评估器。
func New(deps Deps) *Evaluator {
	return &Evaluator{chats: deps.Chats, params: deps.Params, logger: deps.Logger, toolInfos: deps.ToolInfos}
}

// Levels 在一次运行命中待挂起工具、准备挂起之前，对本次全部待执行调用做一次独立的批量分类，
// 产出与 calls 顺序对齐、长度相等的审核重要级。评估用对话模型优先取 flash 档位，解析失败回退
// 本次运行档位；两者都失败、模型调用报错、响应为空、解析失败、条数不匹配或档位非法时，对应调用
// 一律保守回落 must，并记录告警，绝不中断本次挂起。返回长度恒等于 len(calls)。
func (e *Evaluator) Levels(ctx context.Context, runSlot modelsai.Slot, calls []contract.PendingCall) []contract.AuditLevel {
	levels := make([]contract.AuditLevel, len(calls))
	for i := range levels {
		levels[i] = contract.AuditLevelMust
	}
	if len(calls) == 0 {
		return levels
	}
	if e == nil || e.params == nil {
		return levels
	}
	chat, err := port.FlashOrFallbackChat(e.chats, runSlot)
	if err != nil {
		e.warnf(ctx, "agent: audit level evaluation skipped, falling back to must for all calls: %v", err)
		return levels
	}
	userMessage, err := e.buildPrompt(ctx, calls)
	if err != nil {
		e.warnf(ctx, "agent: audit level prompt build failed, falling back to must for all calls: %v", err)
		return levels
	}
	messages := []*schema.Message{
		schema.SystemMessage(e.params.Text("agent.auditLevelSystemPrompt")),
		userMessage,
	}
	resp, err := chat.Generate(ctx, messages)
	if err != nil {
		e.warnf(ctx, "agent: audit level evaluation failed, falling back to must for all calls: %v", err)
		return levels
	}
	if resp == nil || strings.TrimSpace(resp.Content) == "" {
		e.warnf(ctx, "agent: audit level evaluation returned an empty response, falling back to must for all calls")
		return levels
	}
	items, err := parseLevels(resp.Content)
	if err != nil {
		e.warnf(ctx, "agent: cannot parse the audit level response, falling back to must for all calls: %v", err)
		return levels
	}
	if len(items) != len(calls) {
		e.warnf(ctx, "agent: audit level response carries %d entries for %d calls, falling back to must for unmatched calls", len(items), len(calls))
	}
	byCallID := make(map[string]contract.AuditLevel, len(items))
	for _, item := range items {
		level, ok := parseLevelLiteral(item.Level)
		if !ok {
			e.warnf(ctx, "agent: audit level response carries an invalid level %q for call %q, falling back to must", item.Level, item.CallID)
			continue
		}
		byCallID[item.CallID] = level
	}
	for i := range calls {
		level, ok := byCallID[calls[i].CallID]
		if !ok {
			e.warnf(ctx, "agent: no valid audit level was resolved for call %q, falling back to must", calls[i].CallID)
			continue
		}
		levels[i] = level
	}
	return levels
}

// toolDescription 按工具名解析用途说明：先查 client 工具 schema 注册表，未命中再查本次装配的
// builtin / MCP 工具 schema 索引；两处均未命中或说明为空时返回空串。
func (e *Evaluator) toolDescription(name string) string {
	if info, ok := contract.ClientToolInfo(name); ok && strings.TrimSpace(info.Desc) != "" {
		return info.Desc
	}
	if info, ok := e.toolInfos[name]; ok && info != nil && strings.TrimSpace(info.Desc) != "" {
		return info.Desc
	}
	return ""
}

// warnf 在 logger 非空时输出告警。
func (e *Evaluator) warnf(ctx context.Context, format string, args ...any) {
	if e == nil || e.logger == nil {
		return
	}
	e.logger.Ctx(ctx).Warnf(format, args...)
}

// levelItem 是评估模型输出的单条档位（JSON 形状）。
type levelItem struct {
	// CallID 与待执行调用的 callID 一致，用于对齐回填
	CallID string `json:"callID"`
	// Level 审核重要级字面量（must / suggest / auto / none）
	Level string `json:"level"`
}

// levelPayload 是评估模型输出的 JSON 形状：{"levels":[{"callID":"...","level":"..."}]}。
type levelPayload struct {
	Levels []levelItem `json:"levels"`
}

// parseLevels 容错解析评估模型输出：先去掉 ```json 代码围栏，再按 {"levels":[...]} 解析，
// 失败时退回裸数组解析（容忍仅有 levels 数组的形式）。空白响应返回英文错误。
func parseLevels(content string) ([]levelItem, error) {
	trimmed := contract.StripJSONFence(content)
	if strings.TrimSpace(trimmed) == "" {
		return nil, errors.New("agent: audit level response is empty")
	}
	var payload levelPayload
	if err := json.Unmarshal([]byte(trimmed), &payload); err == nil && payload.Levels != nil {
		return payload.Levels, nil
	}
	var items []levelItem
	if err := json.Unmarshal([]byte(trimmed), &items); err != nil {
		return nil, errors.WithStack(err)
	}
	return items, nil
}

// parseLevelLiteral 校验字面量是否属于四档重要级取值空间：命中返回对应常量与 true，
// 否则返回 (must, false)，由调用方回落 must 并告警。
func parseLevelLiteral(literal string) (contract.AuditLevel, bool) {
	switch contract.AuditLevel(strings.TrimSpace(literal)) {
	case contract.AuditLevelMust:
		return contract.AuditLevelMust, true
	case contract.AuditLevelSuggest:
		return contract.AuditLevelSuggest, true
	case contract.AuditLevelAuto:
		return contract.AuditLevelAuto, true
	case contract.AuditLevelNone:
		return contract.AuditLevelNone, true
	default:
		return contract.AuditLevelMust, false
	}
}
