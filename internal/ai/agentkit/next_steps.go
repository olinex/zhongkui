package agentkit

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/msgops"
	"zhongkuisec.com/zhongkui/internal/ai/port"
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// 推荐下一步的行为上限（非提示词正文；内联路径的输出约定见 agentNextStepsHint，兜底辅助调用的
// 提示词由 agent.nextStepsSystemPrompt 提供）。
const (
	// nextStepsMaxItems 单条回答最多保留的推荐项数量
	nextStepsMaxItems = 3
	// nextStepsHistoryLimit 拼入推荐上下文的最近对话条数上限
	nextStepsHistoryLimit = 10
	// nextStepsMessageSnippetLimit 单条历史消息 / 摘要的截断长度（字符）
	nextStepsMessageSnippetLimit = 1500
	// nextStepsAnswerSnippetLimit 本轮回答的截断长度（字符）
	nextStepsAnswerSnippetLimit = 6000
	// nextStepsMaxAttempts 推荐解析 / 校验失败时允许的最大生成次数（含首次）：失败时把具体
	// 校验错误回传给模型要求重新生成，不在本地截断或改写模型输出
	nextStepsMaxAttempts = 2
	// nextStepsTimeout 推荐调用的独立超时：辅助能力，超时即静默跳过，不拖住主回答收尾
	nextStepsTimeout = 10 * time.Second
)

// 内联下一步段在正文流中的起止标记：general 助手在最终回答末尾输出由该标记包裹的 JSON，
// 过滤器据此把该段从正文增量中剥离。标记是解析契约，不随语言变化。
const (
	// nextStepsInlineOpen 内联下一步段的起始标记
	nextStepsInlineOpen = "<!--next_steps"
	// nextStepsInlineClose 内联下一步段的结束标记
	nextStepsInlineClose = "-->"
)

// 推荐下一步解析的固定错误。
var (
	// errNextStepsEmpty 推荐响应为空
	errNextStepsEmpty = errors.New("agentkit: next step response is empty")
	// errNextStepsParse 推荐响应无法解析为 JSON
	errNextStepsParse = errors.New("agentkit: cannot parse the next step response as JSON")
)

// InlineNextStepsFilter 从正文流中剥离内联下一步段：起始标记之前的正文即时外发，标记内只累积、
// 不下发。标记可能跨 chunk 边界，故两个状态各保留「标记长度 - 1」的尾部缓冲；结束标记之后的内容
// 一律丢弃（约定中该段位于整段回答最末尾）。body 累积已外发的正文全文，供 done 覆盖时使用。
//
// 尾部缓冲不能按固定字节长度硬切：一个多字节字符（中文 / emoji）若被切在中间，残缺的半 rune 会
// 立刻作为正文外发，前端按 UTF-8 解码得到临时乱码（U+FFFD），直到后续片段补齐才恢复。故裁切点
// 一律先校正到合法字符边界（见 cutTailBytes）。
type InlineNextStepsFilter struct {
	body     strings.Builder // 已外发的正文全量
	payload  strings.Builder // 标记内的原始 JSON 文本
	pending  string          // 尚未判定归属的尾部缓冲
	inSteps  bool            // 已进入下一步段
	finished bool            // 已遇到结束标记（或已判定为载荷），后续内容丢弃
	seen     bool            // 本次流中是否出现过起始标记
}

// NewInlineNextStepsFilter 创建过滤器。
func NewInlineNextStepsFilter() *InlineNextStepsFilter {
	return &InlineNextStepsFilter{}
}

// cutTailBytes 把 pending 按「尾部保留 keep 字节」切成 emit 与 rest，并把切点向前校正到 UTF-8
// 字符边界：rest 以合法 rune 起始，emit 才不会以半截多字节字符结尾。
func cutTailBytes(pending string, keep int) (string, string) {
	split := len(pending) - keep
	for split > 0 && !utf8.RuneStart(pending[split]) {
		split--
	}
	return pending[:split], pending[split:]
}

// Write 接收一个正文增量，返回本次应外发的正文（可能为空）。返回值只含起始标记之前的文本。
func (f *InlineNextStepsFilter) Write(chunk string) string {
	if chunk == "" || f.finished {
		return ""
	}
	f.pending += chunk
	var out strings.Builder
	for {
		if f.inSteps {
			idx := strings.Index(f.pending, nextStepsInlineClose)
			if idx < 0 {
				// 保留可能被切断的结束标记前缀，其余全部计入载荷（切点按字符边界校正）。
				if keep := len(nextStepsInlineClose) - 1; len(f.pending) > keep {
					emit, rest := cutTailBytes(f.pending, keep)
					f.payload.WriteString(emit)
					f.pending = rest
				}
				return out.String()
			}
			f.payload.WriteString(f.pending[:idx])
			f.pending = ""
			f.inSteps = false
			f.finished = true
			return out.String()
		}
		idx := strings.Index(f.pending, nextStepsInlineOpen)
		if idx < 0 {
			// 无起始标记：仅保留可能被切断的标记前缀，其余即正文（切点按字符边界校正，
			// 避免把多字节字符截成残缺 rune 外发）。
			if keep := len(nextStepsInlineOpen) - 1; len(f.pending) > keep {
				emit, rest := cutTailBytes(f.pending, keep)
				f.pending = rest
				out.WriteString(emit)
				f.body.WriteString(emit)
			}
			return out.String()
		}
		if idx > 0 {
			emit := f.pending[:idx]
			out.WriteString(emit)
			f.body.WriteString(emit)
		}
		f.pending = f.pending[idx+len(nextStepsInlineOpen):]
		f.inSteps = true
		f.seen = true
	}
}

// Close 处理流结束：正文态把残留缓冲作为正文补发；下一步段未闭合时把残留并入载荷（容忍模型
// 漏写结束标记）。返回需要补发的正文字符串。
func (f *InlineNextStepsFilter) Close() string {
	if f.finished {
		return ""
	}
	if f.inSteps {
		f.payload.WriteString(f.pending)
		f.pending = ""
		f.finished = true
		return ""
	}
	emit := f.pending
	f.pending = ""
	f.body.WriteString(emit)
	return emit
}

// ResetSection 在一个工具轮结束时复位：丢弃该轮累积的载荷与标记状态，避免中间轮误输出的
// 下一步段吞掉后续轮次的正文；正文全量保持不变。返回需要在轮次边界补发的正文尾缓冲（标记不会
// 跨轮次，残留缓冲必属正文）。
func (f *InlineNextStepsFilter) ResetSection() string {
	var emit string
	if !f.inSteps && !f.finished {
		emit = f.pending
		f.body.WriteString(emit)
	}
	f.pending = ""
	f.payload.Reset()
	f.inSteps = false
	f.finished = false
	f.seen = false
	return emit
}

// Body 返回已外发的正文全文。
func (f *InlineNextStepsFilter) Body() string {
	return f.body.String()
}

// Seen 表示本次流是否出现过起始标记：投影层据此判断是否需要把累积正文替换为剥离后的全文。
func (f *InlineNextStepsFilter) Seen() bool {
	return f.seen
}

// Steps 返回标记内解析出的推荐项：seen 表示本次流是否出现过起始标记（false 时调用方应回退到
// 独立推荐调用）；seen 为 true 但解析或契约校验失败时返回错误，调用方记录告警后同样回退。
func (f *InlineNextStepsFilter) Steps() (steps []contract.NextStep, seen bool, err error) {
	if !f.seen {
		return nil, false, nil
	}
	payload := strings.TrimSpace(f.payload.String())
	if payload == "" {
		return nil, true, errNextStepsEmpty
	}
	items, err := ParseNextSteps(payload)
	if err != nil {
		return nil, true, errors.WithStack(err)
	}
	steps, err = NormalizeNextSteps(items)
	if err != nil {
		return nil, true, errors.WithStack(err)
	}
	return steps, true, nil
}

// RecommendNextSteps 是基于整体上下文的独立推荐调用，现作为内联路径的兜底：仅当模型未按约定输出
// 内联下一步段（或该段无法解析 / 不满足契约）时执行，用于非 general 助手、或提示词未遵循约定的模型。
// 仅在真实会话（SessionID 非零）、开关开启且回答非空时执行。输出解析或长度校验失败时，把具体错误
// 回传给模型要求重新生成（最多 nextStepsMaxAttempts 次），不在本地截断模型输出；模型 / 超时失败或
// 次数用尽时只告警并返回 nil，绝不中断本轮收尾与落库。返回项已按推荐程度排序，首条 Recommended 为 true。
func RecommendNextSteps[M adk.MessageType](ctx context.Context, rt *Runtime, req Request, summary string, history []M, answer string) []contract.NextStep {
	if rt == nil || req.SessionID == 0 || !rt.params.Bool("agent.nextStepsEnabled") {
		return nil
	}
	if strings.TrimSpace(answer) == "" {
		return nil
	}
	chat, err := rt.resolveFlashChat(req.Slot)
	if err != nil {
		rt.warnf(ctx, "agentkit: next step recommendation skipped: %v", err)
		return nil
	}
	callCtx, cancel := context.WithTimeout(ctx, nextStepsTimeout)
	defer cancel()
	userMessage, err := buildNextStepsPrompt(callCtx, summary, history, req.Query, answer)
	if err != nil {
		rt.warnf(ctx, "agentkit: next step prompt build failed: %v", err)
		return nil
	}
	messages := []*schema.Message{
		schema.SystemMessage(rt.params.Text("agent.nextStepsSystemPrompt")),
		userMessage,
	}
	for attempt := 0; attempt < nextStepsMaxAttempts; attempt++ {
		resp, callErr := chat.Generate(callCtx, messages)
		if callErr != nil {
			rt.warnf(ctx, "agentkit: next step recommendation failed: %v", callErr)
			return nil
		}
		if resp == nil || strings.TrimSpace(resp.Content) == "" {
			rt.warnf(ctx, "agentkit: next step recommendation returned an empty response")
			return nil
		}
		steps, err := nextStepsFromContent(resp.Content)
		if err == nil {
			if len(steps) == 0 {
				return nil
			}
			return steps
		}
		// 解析或契约校验失败：把具体错误回传给模型要求重新生成，不在本地截断 / 改写输出。
		if attempt == nextStepsMaxAttempts-1 {
			rt.warnf(ctx, "agentkit: next step recommendation rejected after %d attempts: %v", attempt+1, err)
			return nil
		}
		retry, renderErr := renderTemplateMessage[*schema.Message](ctx, false, nextStepsRetryPromptTemplate, map[string]any{
			"Cause":     err.Error(),
			"TitleMax":  contract.NextStepTitleMaxLength,
			"ReasonMax": contract.NextStepReasonMaxLength,
			"TextMax":   contract.NextStepTextMaxLength,
		})
		if renderErr != nil {
			rt.warnf(ctx, "agentkit: next step retry prompt build failed: %v", renderErr)
			return nil
		}
		messages = append(messages, schema.AssistantMessage(resp.Content, nil), retry)
	}
	return nil
}

// nextStepsFromContent 解析并归一化一次推荐输出：解析失败与契约校验失败都返回英文错误，由调用方
// 回传给模型重新生成。
func nextStepsFromContent(content string) ([]contract.NextStep, error) {
	items, err := ParseNextSteps(content)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return NormalizeNextSteps(items)
}

// resolveFlashChat 经 resolver 解析 flash 档位的对话客户端：优先 flash，失败回退本次运行档位；
// 两者都失败返回英文错误。用于审核评估与推荐下一步等辅助模型调用。
func (r *Runtime) resolveFlashChat(runSlot modelsai.Slot) (model.BaseChatModel, error) {
	return port.FlashOrFallbackChat(r.deps.ChatResolver, runSlot)
}

// nextStepsPromptMessage 是推荐下一步模板中的单条对话数据。
type nextStepsPromptMessage struct {
	// Role 展示用角色标签（用户 / 助手）
	Role string
	// Content 按 nextStepsMessageSnippetLimit 截断后的正文
	Content string
}

// buildNextStepsPrompt 按模板构造推荐请求的用户消息：依次给出会话摘要（首次运行）、最近对话、
// 本轮提问与本轮回答，使模型能结合整体上下文判断「最可能 / 最重要 / 最急迫」的下一步。
func buildNextStepsPrompt[M adk.MessageType](ctx context.Context, summary string, history []M, query, answer string) (*schema.Message, error) {
	var trimmedSummary string
	if trimmed := strings.TrimSpace(summary); trimmed != "" {
		trimmedSummary = retrieval.TruncateText(trimmed, nextStepsMessageSnippetLimit)
	}
	messages := nextStepsContextMessages(history)
	var trimmedQuery string
	if trimmed := strings.TrimSpace(query); trimmed != "" {
		trimmedQuery = retrieval.TruncateText(trimmed, nextStepsMessageSnippetLimit)
	}
	return renderTemplateMessage[*schema.Message](ctx, false, nextStepsPromptTemplate, map[string]any{
		"Summary":  trimmedSummary,
		"Messages": messages,
		"Query":    trimmedQuery,
		"Answer":   retrieval.TruncateText(strings.TrimSpace(answer), nextStepsAnswerSnippetLimit),
	})
}

// nextStepsContextMessages 取最近对话：只保留 user / assistant，裁到最近 nextStepsHistoryLimit 条，
// 跳过工具结果消息，单条按 nextStepsMessageSnippetLimit 截断。
func nextStepsContextMessages[M adk.MessageType](history []M) []nextStepsPromptMessage {
	conversation := make([]nextStepsPromptMessage, 0, len(history))
	for _, message := range history {
		role, ok := nextStepsRole(message)
		if !ok {
			continue
		}
		content := strings.TrimSpace(msgops.Text(message))
		if content == "" {
			continue
		}
		conversation = append(conversation, nextStepsPromptMessage{
			Role:    role,
			Content: retrieval.TruncateText(content, nextStepsMessageSnippetLimit),
		})
	}
	if len(conversation) > nextStepsHistoryLimit {
		conversation = conversation[len(conversation)-nextStepsHistoryLimit:]
	}
	return conversation
}

// nextStepsRole 返回可用于推荐提示词的角色标签，仅 user / assistant 参与。
func nextStepsRole[M adk.MessageType](message M) (string, bool) {
	switch m := any(message).(type) {
	case *schema.Message:
		if m == nil {
			return "", false
		}
		switch m.Role {
		case schema.User:
			return "用户", true
		case schema.Assistant:
			return "助手", true
		}
	case *schema.AgenticMessage:
		if m == nil {
			return "", false
		}
		if len(msgops.ToolResults(message)) > 0 {
			return "", false
		}
		switch m.Role {
		case schema.AgenticRoleTypeUser:
			return "用户", true
		case schema.AgenticRoleTypeAssistant:
			return "助手", true
		}
	}
	return "", false
}

// nextStepsPayload 是推荐模型输出的 JSON 形状：
// {"steps":[{"title":"...","reason":"...","text":"..."}]}。
type nextStepsPayload struct {
	Steps []json.RawMessage `json:"steps"`
}

// ParseNextSteps 容错解析推荐模型输出：先去掉 ```json 代码围栏，再按 {"steps":[...]} 解析；
// 失败时退回裸数组解析。条目既接受 {"title","reason","text"} 对象，也接受纯字符串。
func ParseNextSteps(content string) ([]contract.NextStep, error) {
	trimmed := contract.StripJSONFence(content)
	if strings.TrimSpace(trimmed) == "" {
		return nil, errNextStepsEmpty
	}
	var payload nextStepsPayload
	if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
		return nextStepsFromRaw(payload.Steps), nil
	}
	var bare []json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &bare); err == nil {
		return nextStepsFromRaw(bare), nil
	}
	return nil, errNextStepsParse
}

// nextStepsFromRaw 把原始 JSON 条目解码为推荐项：字符串直接作为文本，对象取 title / reason /
// text 字段；无法识别的条目跳过，不使整份响应失败。
func nextStepsFromRaw(raw []json.RawMessage) []contract.NextStep {
	out := make([]contract.NextStep, 0, len(raw))
	for _, item := range raw {
		var text string
		if err := json.Unmarshal(item, &text); err == nil {
			out = append(out, contract.NextStep{Text: text})
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
		out = append(out, contract.NextStep{Title: obj.Title, Text: obj.Text, Reason: obj.Reason})
	}
	return out
}

// NormalizeNextSteps 归一化推荐项：标题 / 文本 / 理由分别去首尾空白与包裹引号、折叠换行与连续空白，
// 文本为空时回退标题，按文本精确去重，最多保留 nextStepsMaxItems 条。归一化**不做长度截断**：任一
// 非空条目不满足 contract.NextStep 的 binding 标签（Text / Reason 必填，Title ≤ 24、Reason ≤ 64、
// Text ≤ 200 个字符）时返回英文错误，由调用方把错误回传给模型重新生成。首条标记为最推荐。
func NormalizeNextSteps(items []contract.NextStep) ([]contract.NextStep, error) {
	out := make([]contract.NextStep, 0, nextStepsMaxItems)
	seen := make(map[string]bool, len(items))
	for i, item := range items {
		text := normalizeNextStepField(item.Text)
		title := normalizeNextStepField(item.Title)
		reason := normalizeNextStepField(item.Reason)
		if text == "" {
			text = title
		}
		// 完全空的条目是模型输出噪声（如空字符串），直接跳过；其余不合法取值一律报错。
		if text == "" && reason == "" && title == "" {
			continue
		}
		candidate := contract.NextStep{Title: title, Text: text, Reason: reason}
		if err := candidate.Validate(); err != nil {
			return nil, errors.Wrapf(err, "agentkit: next step #%d violates the contract", i+1)
		}
		if seen[text] {
			continue
		}
		seen[text] = true
		out = append(out, candidate)
		if len(out) >= nextStepsMaxItems {
			break
		}
	}
	if len(out) > 0 {
		out[0].Recommended = true
	}
	return out, nil
}

// normalizeNextStepField 规范化标题 / 文本 / 理由：去首尾空白、去包裹引号 / 反引号，折叠所有空白为
// 单空格；不做长度截断，长度约束由 contract.NextStep.Validate 判定。
func normalizeNextStepField(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"'`")
	value = strings.Join(strings.Fields(value), " ")
	return strings.TrimSpace(value)
}
