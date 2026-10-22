package ai

import (
	// 标准库
	"context"
	"encoding/json"
	"strings"
	"time"

	// 第三方
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// 推荐下一步的行为上限（非提示词正文；提示词正文由 agent.nextStepsSystemPrompt 提供）。
const (
	// nextStepsMaxItems 单条回答最多保留的推荐项数量
	nextStepsMaxItems = 3
	// nextStepsHistoryLimit 拼入推荐上下文的最近对话条数上限
	nextStepsHistoryLimit = 10
	// nextStepsMessageSnippetLimit 单条历史消息 / 摘要的截断长度（字符）
	nextStepsMessageSnippetLimit = 1500
	// nextStepsAnswerSnippetLimit 本轮回答的截断长度（字符）
	nextStepsAnswerSnippetLimit = 6000
	// nextStepsTitleLimit 单条推荐标题的截断长度（字符）
	nextStepsTitleLimit = 24
	// nextStepsReasonLimit 单条推荐理由的截断长度（字符）
	nextStepsReasonLimit = 64
	// nextStepsItemSnippetLimit 单条推荐文本（用户可直接发送的诉求）的截断长度（字符）
	nextStepsItemSnippetLimit = 200
	// nextStepsTimeout 推荐调用的独立超时：辅助能力，超时即静默跳过，不拖住主回答收尾
	nextStepsTimeout = 10 * time.Second
)

// recommendNextSteps 在回答完成后基于整体上下文生成 1-3 条推荐的下一步。
// 仅在真实会话（SessionID 非零）、开关开启且回答非空时执行；模型 / 解析 / 超时失败一律只告警并
// 返回 nil，绝不中断本轮 done 的产出与落库。返回项已按推荐程度排序，首条 Recommended 为 true。
func (a *Agent) recommendNextSteps(ctx context.Context, in AgentInput, answer string) []NextStep {
	if in.SessionID == 0 || !a.params.Bool("agent.nextStepsEnabled") {
		return nil
	}
	if strings.TrimSpace(answer) == "" {
		return nil
	}
	chat, err := a.resolveAuditChat(in.Slot)
	if err != nil {
		a.warnf(ctx, "agent: next step recommendation skipped: %v", err)
		return nil
	}
	callCtx, cancel := context.WithTimeout(ctx, nextStepsTimeout)
	defer cancel()
	resp, err := chat.Generate(callCtx, []*schema.Message{
		schema.SystemMessage(a.params.Text("agent.nextStepsSystemPrompt")),
		schema.UserMessage(a.buildNextStepsPrompt(in, answer)),
	})
	if err != nil {
		a.warnf(ctx, "agent: next step recommendation failed: %v", err)
		return nil
	}
	if resp == nil || strings.TrimSpace(resp.Content) == "" {
		a.warnf(ctx, "agent: next step recommendation returned an empty response")
		return nil
	}
	items, err := parseNextSteps(resp.Content)
	if err != nil {
		a.warnf(ctx, "agent: cannot parse the next step recommendation response: %v", err)
		return nil
	}
	steps := normalizeNextSteps(items)
	if len(steps) == 0 {
		return nil
	}
	return steps
}

// buildNextStepsPrompt 构造推荐请求的用户消息：依次给出会话摘要（首次运行）、最近对话、本轮提问与
// 本轮回答，使模型能结合整体上下文判断「最可能 / 最重要 / 最急迫」的下一步。历史只保留 user /
// assistant 角色与摘要系统消息，跳过系统提示与 tool 消息；各段均有长度上限，避免辅助调用占用过多上下文。
func (a *Agent) buildNextStepsPrompt(in AgentInput, answer string) string {
	var sb strings.Builder
	if summary := strings.TrimSpace(in.Summary); summary != "" {
		sb.WriteString("## 会话摘要\n")
		sb.WriteString(truncateText(summary, nextStepsMessageSnippetLimit))
		sb.WriteString("\n\n")
	}
	if msgs := nextStepsContextMessages(in); len(msgs) > 0 {
		sb.WriteString("## 最近对话\n")
		for _, m := range msgs {
			switch m.Role {
			case schema.User:
				sb.WriteString("用户：")
			case schema.Assistant:
				sb.WriteString("助手：")
			}
			sb.WriteString(truncateText(strings.TrimSpace(m.Content), nextStepsMessageSnippetLimit))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	if query := strings.TrimSpace(in.Query); query != "" {
		sb.WriteString("## 本轮提问\n")
		sb.WriteString(truncateText(query, nextStepsMessageSnippetLimit))
		sb.WriteString("\n\n")
	}
	sb.WriteString("## 本轮回答\n")
	sb.WriteString(truncateText(strings.TrimSpace(answer), nextStepsAnswerSnippetLimit))
	return sb.String()
}

// nextStepsContextMessages 取推荐上下文可用的最近对话：首次运行取注入的历史消息，续跑取挂起现场
// 的消息链并截到本轮最后一条用户消息（其后已外发的助手正文由本轮回答承载，避免重复）；只保留
// user / assistant 与摘要系统消息，并裁到最近 nextStepsHistoryLimit 条。
func nextStepsContextMessages(in AgentInput) []*schema.Message {
	src := in.HistoryMessages
	if in.ResumeMessages != nil {
		src = in.ResumeMessages
		lastUser := -1
		for i := len(src) - 1; i >= 0; i-- {
			if src[i] != nil && src[i].Role == schema.User {
				lastUser = i
				break
			}
		}
		if lastUser >= 0 {
			src = src[:lastUser+1]
		}
	}
	out := make([]*schema.Message, 0, nextStepsHistoryLimit)
	var summary, conversation []*schema.Message
	for _, m := range src {
		if m == nil || strings.TrimSpace(m.Content) == "" {
			continue
		}
		switch m.Role {
		case schema.User, schema.Assistant:
			conversation = append(conversation, m)
		case schema.System:
			if strings.HasPrefix(strings.TrimSpace(m.Content), agentSummarySectionTitle) {
				summary = append(summary, m)
			}
		}
	}
	if len(conversation) > nextStepsHistoryLimit {
		conversation = conversation[len(conversation)-nextStepsHistoryLimit:]
	}
	out = append(out, summary...)
	out = append(out, conversation...)
	return out
}

// nextStepsPayload 是推荐模型输出的 JSON 形状：
// {"steps":[{"title":"...","reason":"...","text":"..."}]}。
type nextStepsPayload struct {
	Steps []json.RawMessage `json:"steps"`
}

// parseNextSteps 容错解析推荐模型输出：先去掉 ```json 代码围栏，再按 {"steps":[...]} 解析；
// 失败时退回裸数组解析。条目既接受 {"title","reason","text"} 对象，也接受纯字符串（旧形状，
// 由归一化从文本推导标题），便于模型偶发偏离形状时仍能给出可用结果。空白响应或完全无法解析时
// 返回英文错误（由调用方降级为不展示）。
func parseNextSteps(content string) ([]NextStep, error) {
	trimmed := stripJSONFence(content)
	if strings.TrimSpace(trimmed) == "" {
		return nil, errors.New("agent: next step response is empty")
	}
	var payload nextStepsPayload
	if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
		return nextStepsFromRaw(payload.Steps), nil
	}
	var bare []json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &bare); err == nil {
		return nextStepsFromRaw(bare), nil
	}
	return nil, errors.New("agent: cannot parse the next step response as JSON")
}

// nextStepsFromRaw 把原始 JSON 条目解码为推荐项：字符串直接作为文本，对象取 title / reason /
// text 字段；无法识别的条目跳过，不使整份响应失败。
func nextStepsFromRaw(raw []json.RawMessage) []NextStep {
	out := make([]NextStep, 0, len(raw))
	for _, item := range raw {
		var text string
		if err := json.Unmarshal(item, &text); err == nil {
			out = append(out, NextStep{Text: text})
			continue
		}
		var obj struct {
			Title  string `json:"title"`
			Text   string `json:"text"`
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal(item, &obj); err != nil {
			continue
		}
		out = append(out, NextStep{Title: obj.Title, Text: obj.Text, Reason: obj.Reason})
	}
	return out
}

// normalizeNextSteps 归一化推荐项：标题 / 文本 / 理由分别去首尾空白与包裹引号、折叠换行与连续空白，
// 并按各自上限截断（标题 24、理由 64、文本 200）；文本为空时回退标题，标题为空时从文本推导，丢弃
// 无文本项并按文本精确去重，最多保留 nextStepsMaxItems 条；首条标记为最推荐。
func normalizeNextSteps(items []NextStep) []NextStep {
	out := make([]NextStep, 0, nextStepsMaxItems)
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		text := normalizeNextStepField(item.Text, nextStepsItemSnippetLimit)
		title := normalizeNextStepField(item.Title, nextStepsTitleLimit)
		reason := normalizeNextStepField(item.Reason, nextStepsReasonLimit)
		if text == "" {
			text = title
		}
		if text == "" {
			continue
		}
		// 推荐理由为格式硬要求：缺失时丢弃该条，避免前端出现只有标题、没有推荐依据的推荐项。
		if reason == "" {
			continue
		}
		if title == "" {
			title = truncateNextStepField(text, nextStepsTitleLimit)
		}
		if seen[text] {
			continue
		}
		seen[text] = true
		out = append(out, NextStep{Title: title, Text: text, Reason: reason})
		if len(out) >= nextStepsMaxItems {
			break
		}
	}
	if len(out) > 0 {
		out[0].Recommended = true
	}
	return out
}

// normalizeNextStepField 规范化标题 / 文本 / 理由：去首尾空白、去包裹引号 / 反引号、折叠所有空白为
// 单空格，最后按给定上限截断。
func normalizeNextStepField(value string, limit int) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"'`")
	value = strings.Join(strings.Fields(value), " ")
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return truncateNextStepField(value, limit)
}

// truncateNextStepField 把推荐字段截断到不超过 limit 个字符（rune）：超长时保留 limit-1 个字符并追加
// 单个省略号，确保最终长度仍不超过上限（标题 24 / 理由 64 为硬约束）。
func truncateNextStepField(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit == 1 {
		return string(runes[:1])
	}
	return string(runes[:limit-1]) + "…"
}
