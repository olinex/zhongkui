package agentkit

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/adk"
	einoprompt "github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/ai/config"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/msgops"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
)

// citationMarkerRE 匹配正文中的来源标注 [n]；codeSegmentRE 切出代码围栏与行内代码，
// 其中的数组下标（如 arr[1]）不得被当作引用标注处理。
var (
	citationMarkerRE = regexp.MustCompile(`\[[0-9]+\]`)
	codeSegmentRE    = regexp.MustCompile("(?s)```.*?```|~~~.*?~~~|`[^`\\n]*`")
)

// stripCitationMarkers 移除文本中的 [n] 来源标注（保留代码段内的下标）：来源编号只在本轮运行内
// 有效，历史回答与摘要里的旧编号不得带入新一轮，否则模型会把旧 [n] 误当成本轮可引用的来源。
func stripCitationMarkers(text string) string {
	if text == "" {
		return text
	}
	matches := codeSegmentRE.FindAllStringIndex(text, -1)
	var b strings.Builder
	last := 0
	for _, match := range matches {
		b.WriteString(citationMarkerRE.ReplaceAllString(text[last:match[0]], ""))
		b.WriteString(text[match[0]:match[1]])
		last = match[1]
	}
	b.WriteString(citationMarkerRE.ReplaceAllString(text[last:], ""))
	return b.String()
}

// stripHistoryCitationMarkers 移除历史助手消息正文里的旧来源标注：新提问重新开始编号，
// 历史 [n] 与本轮 citations 无对应关系，保留会让模型抄用而出现跨轮次的假引用。
func stripHistoryCitationMarkers[M adk.MessageType](history []M) []M {
	out := make([]M, 0, len(history))
	for _, message := range history {
		if msgops.RoleLabel[M](message) != "Agent" {
			out = append(out, message)
			continue
		}
		text := msgops.AssistantText[M](message)
		stripped := stripCitationMarkers(text)
		if stripped == text {
			out = append(out, message)
			continue
		}
		out = append(out, msgops.ReplaceAssistantText(message, stripped))
	}
	return out
}

// newGenModelInput 为单个槽位构建 TypedGenModelInput。全新运行时会读取
// turns 注入的 RunContext[M]。恢复的运行不携带运行上下文（ADK 会从
// 检查点还原会话），因此还原后的消息会原样返回——尤其是恢复时不会
// 再次清洗业务数据。
func newGenModelInput[M adk.MessageType](rt *Runtime) adk.TypedGenModelInput[M] {
	return func(ctx context.Context, _ string, input *adk.TypedAgentInput[M]) ([]M, error) {
		rc, ok := RunContextFrom[M](ctx)
		if !ok {
			if input == nil {
				return nil, nil
			}
			return input.Messages, nil
		}
		// RunContext.Memories 允许调用方预先召回；未提供时按与旧 Agent 一致的口径召回
		//（服务缺失 / 记忆关闭 / 无会话 / 检索失败都降级为空）。
		if rc.Memories == nil {
			rc.Memories = rt.RecallMemories(ctx, rc.Request)
		}
		return buildModelInput(ctx, rt.Params(), rc)
	}
}

// buildModelInput 为全新运行组装完整的模型输入：系统提示词（基础提示词、
// 能力提示、引用提示、可选的内联下一步提示）、摘要段、记忆段、
// 规范化历史，以及携带业务数据和查询的最终用户消息。
func buildModelInput[M adk.MessageType](ctx context.Context, params *config.Params, rc RunContext[M]) ([]M, error) {
	vars := map[string]any{
		"BasePrompt":           params.Text(promptKeyForAgentKind(rc.Request.AgentKind)),
		"CapabilityHint":       agentCapabilityHint,
		"CitationHint":         agentCitationHint,
		"InlineNextSteps":      inlineNextStepsEnabled(params, rc.Request),
		"NextStepsHint":        agentNextStepsHint,
		"BusinessSectionTitle": agentBusinessSectionTitle,
		"BusinessData":         "",
		"QuestionSectionTitle": agentQuestionSectionTitle,
		"Query":                strings.TrimSpace(rc.Request.Query),
		"SkillName":            strings.TrimSpace(rc.Request.SkillName),
		"SkillDescription":     strings.TrimSpace(rc.Request.SkillDescription),
		"SkillContent":         strings.TrimSpace(rc.Request.SkillContent),
		"SystemHint":           strings.TrimSpace(rc.Request.SystemHint),
		"history":              msgops.NormalizeMessagesForModelInput(stripHistoryCitationMarkers(rc.History)),
	}

	if len(rc.Request.BusinessData) > 0 {
		sanitized, err := SanitizeBusinessData(rc.Request.BusinessData, params)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		formatted, err := formatBusinessData(ctx, sanitized)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		vars["BusinessData"] = formatted
	}

	if trimmed := strings.TrimSpace(rc.Summary); trimmed != "" {
		message, err := renderTemplateMessage[M](ctx, true, agentSummaryPromptTemplate, map[string]any{
			"Title":   agentSummarySectionTitle,
			"Summary": stripCitationMarkers(trimmed),
		})
		if err != nil {
			return nil, errors.WithStack(err)
		}
		vars["summaryMessages"] = []M{message}
	}

	if items := memoryPromptItems(rc.Memories); len(items) > 0 {
		message, err := renderTemplateMessage[M](ctx, true, agentMemoryPromptTemplate, map[string]any{
			"Title": agentMemorySectionTitle,
			"Items": items,
		})
		if err != nil {
			return nil, errors.WithStack(err)
		}
		vars["memoryMessages"] = []M{message}
	}

	return assembleMessages[M](ctx, vars)
}

// assembleMessages 为 M 渲染消息骨架。Chat 使用 eino 的 prompt 组件；
// agentic 消息逐模板渲染，因为此版本的 eino 未提供 agentic 的
// chat-template 构造器。
func assembleMessages[M adk.MessageType](ctx context.Context, vars map[string]any) ([]M, error) {
	if msgops.APIModeOf[M]() == llm.APIModeChat {
		template := einoprompt.FromMessages(schema.GoTemplate,
			schema.SystemMessage(agentSystemPromptTemplate),
			schema.MessagesPlaceholder("summaryMessages", true),
			schema.MessagesPlaceholder("memoryMessages", true),
			schema.MessagesPlaceholder("history", true),
			schema.UserMessage(agentUserPromptTemplate),
		)
		rendered, err := template.Format(ctx, vars)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		out := make([]M, 0, len(rendered))
		for _, message := range rendered {
			out = append(out, any(message).(M))
		}
		return out, nil
	}

	templates := []schema.AgenticMessagesTemplate{
		schema.SystemAgenticMessage(agentSystemPromptTemplate),
		schema.AgenticMessagesPlaceholder("summaryMessages", true),
		schema.AgenticMessagesPlaceholder("memoryMessages", true),
		schema.AgenticMessagesPlaceholder("history", true),
		schema.UserAgenticMessage(agentUserPromptTemplate),
	}
	var out []M
	for _, template := range templates {
		rendered, err := template.Format(ctx, vars, schema.GoTemplate)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		for _, message := range rendered {
			out = append(out, any(message).(M))
		}
	}
	return out, nil
}

// renderTemplateMessage 使用 GoTemplate 语义为 M 渲染一条 system 或 user 消息：chat 与 agentic
// 两种消息表示各走对应的 eino 模板构造器，返回第一条渲染结果。
func renderTemplateMessage[M adk.MessageType](ctx context.Context, system bool, template string, vars map[string]any) (M, error) {
	var zero M
	if msgops.APIModeOf[M]() == llm.APIModeResponses {
		var (
			rendered []*schema.AgenticMessage
			err      error
		)
		if system {
			rendered, err = schema.SystemAgenticMessage(template).Format(ctx, vars, schema.GoTemplate)
		} else {
			rendered, err = schema.UserAgenticMessage(template).Format(ctx, vars, schema.GoTemplate)
		}
		if err != nil {
			return zero, errors.WithStack(err)
		}
		if len(rendered) == 0 {
			return zero, errors.New("agentkit: empty agentic template output")
		}
		return any(rendered[0]).(M), nil
	}

	var (
		rendered []*schema.Message
		err      error
	)
	if system {
		rendered, err = schema.SystemMessage(template).Format(ctx, vars, schema.GoTemplate)
	} else {
		rendered, err = schema.UserMessage(template).Format(ctx, vars, schema.GoTemplate)
	}
	if err != nil {
		return zero, errors.WithStack(err)
	}
	if len(rendered) == 0 {
		return zero, errors.New("agentkit: empty chat template output")
	}
	return any(rendered[0]).(M), nil
}

// formatBusinessData 把脱敏后的业务数据序列化为 JSON 并包进
// 围栏代码块。输入已经过脱敏，序列化必然成功；仍防御性地回退为
// "{}"。
func formatBusinessData(ctx context.Context, data contract.BusinessData) (string, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		payload = []byte("{}")
	}
	message, err := schema.UserMessage(agentBusinessDataTemplate).Format(ctx, map[string]any{
		"JSON": string(payload),
	}, schema.GoTemplate)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return message[0].Content, nil
}

// RecallMemories 实现全新运行使用的长期记忆召回语义。服务缺失、记忆
// 关闭、无会话或 agent 类型无效时均返回 nil；检索失败会降级为 nil
// 并记录警告，绝不中断生成。
func (r *Runtime) RecallMemories(ctx context.Context, req Request) []contract.MemoryHit {
	if r == nil || r.memory == nil || !r.params.Bool("memory.enabled") || req.SessionID == 0 {
		return nil
	}
	agentKind, err := modelsai.ParseAgentKind(req.AgentKind)
	if err != nil {
		r.warnf(ctx, "agentkit: long-term memory retrieval skipped, invalid agent kind: %v", err)
		return nil
	}
	hits, err := r.memory.Recall(ctx, req.NamespaceName, agentKind, req.SessionID, req.UserID, req.Query)
	if err != nil {
		r.warnf(ctx, "agentkit: long-term memory retrieval failed, skipping the memory section: %v", err)
		return nil
	}
	return hits
}
