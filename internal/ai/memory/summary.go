// 本文件实现长期记忆（agent_memories）的存储与检索：按「命名空间 + Agent 类型 + 会话」归属与
// 隔离（记忆只在其来源会话内可见），写入时做「规范化正文带密钥 HMAC-SHA256 精确去重 + pgvector
// 余弦相似度语义合并」并同步向量列与全文检索列，检索时做「向量路 + 关键词路」混合召回、RRF
// 融合、按参数阈值内容去重后取 topK，命中异步回写使用统计。记忆正文经 crypto 序列化器加密存储
// （列类型 BYTEA）。检索结果的去重复用知识库同包的文本 n-gram Jaccard 逻辑（与写入的余弦去重
// 是两回事，勿混用）。

package memory

import (
	// 标准库
	"context"
	"strings"

	// 第三方
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/port"
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// SummarizeStream 与 Summarize 语义一致，但经 onDelta 逐块回调摘要生成的增量文本：
// onDelta 非空时用 flash 档位 Stream 逐块读取并回调（回调返回错误即中止并透传）；
// 为空时退回一次性 Generate。返回（新摘要, 新游标, 本次摘要模型调用的 token 用量, 错误）；
// 用量取自响应元信息，供应商未返回时该值为 nil（调用方据此跳过任何依赖用量的估算）。
func (m *Service) SummarizeStream(ctx context.Context, session *ai.AgentSession, evicted []*ai.AgentMessage, onDelta func(string) error) (string, uint, *schema.TokenUsage, error) {
	if session == nil {
		return "", 0, nil, errors.New("memory: nil session")
	}
	if len(evicted) == 0 {
		return string(session.Summary.Value()), session.SummaryUptoSeq, nil, nil
	}
	chat, err := m.resolveFlashChat()
	if err != nil {
		return "", 0, nil, errors.WithStack(err)
	}
	userMessage, err := buildSummaryPrompt(ctx, string(session.Summary.Value()), evicted, m.params.Int("memory.evictedMessageByteLimit"))
	if err != nil {
		return "", 0, nil, errors.WithStack(err)
	}
	messages := []*schema.Message{
		schema.SystemMessage(m.params.Text("memory.summarySystemPrompt")),
		userMessage,
	}
	var content string
	var usage *schema.TokenUsage
	if onDelta == nil {
		resp, gerr := chat.Generate(ctx, messages)
		if gerr != nil {
			return "", 0, nil, errors.WithStack(gerr)
		}
		if resp != nil {
			content = resp.Content
			usage = responseUsage(resp)
		}
	} else {
		streamed, streamUsage, _, serr := port.StreamChat(ctx, chat, messages, onDelta)
		if serr != nil {
			return "", 0, nil, errors.WithStack(serr)
		}
		content, usage = streamed, streamUsage
	}
	if strings.TrimSpace(content) == "" {
		return "", 0, nil, errors.New("memory: summary model returned an empty response")
	}
	summary := retrieval.TruncateBytes(strings.TrimSpace(content), m.params.Int("session.maxContextBytes"))
	return summary, uint(evicted[len(evicted)-1].Seq), usage, nil
}

// responseUsage 从一条模型响应帧（非流式整帧或流式任一帧）取回 token 用量：响应元信息缺失或
// 未携带用量时返回 nil。
func responseUsage(msg *schema.Message) *schema.TokenUsage {
	if msg == nil || msg.ResponseMeta == nil {
		return nil
	}
	return msg.ResponseMeta.Usage
}

// Summarize 用 flash 档位把「既有摘要 + 被挤出消息」重写为新的滚动摘要：返回新摘要与被挤出
// 消息的最后一条会话内 seq（即新的 summary_upto_seq）。evicted 为空时原样返回既有摘要与游标。
// resolver 为 nil、档位未绑定或解析失败、模型返回空内容时均返回英文错误（由调用方降级为不推进
// 游标）。摘要自身按 session.maxContextBytes 截断，避免摘要膨胀。
func (m *Service) Summarize(ctx context.Context, session *ai.AgentSession, evicted []*ai.AgentMessage) (string, uint, error) {
	summary, upto, _, err := m.SummarizeStream(ctx, session, evicted, nil)
	return summary, upto, errors.WithStack(err)
}

// SessionTopicFallback 是会话主题生成失败时的兜底：把首个用户问题按 SessionTopicByteLimit 截断
// 作为主题，保证会话在任何情况下都有一个可辨识的标题。
func SessionTopicFallback(query string) string {
	return retrieval.TruncateBytes(strings.TrimSpace(query), SessionTopicByteLimit)
}

// GenerateTopic 用 flash 档位据首个用户问题生成会话短主题，结果按 SessionTopicByteLimit 截断。
// query 为空时返回空串；resolver 缺失、档位未绑定或解析失败、模型调用失败、空响应一律告警并回退
// SessionTopicFallback，绝不返回错误，以免阻断会话创建。
func (m *Service) GenerateTopic(ctx context.Context, query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	fallback := SessionTopicFallback(query)
	chat, err := m.resolveFlashChat()
	if err != nil {
		m.warnf(ctx, "memory: session topic generation skipped, using the query as the topic: %v", err)
		return fallback
	}
	messages := []*schema.Message{
		schema.SystemMessage(m.params.Text("session.topicSystemPrompt")),
		schema.UserMessage(query),
	}
	resp, err := chat.Generate(ctx, messages)
	if err != nil {
		m.warnf(ctx, "memory: session topic generation failed, using the query as the topic: %v", err)
		return fallback
	}
	if resp == nil || strings.TrimSpace(resp.Content) == "" {
		m.warnf(ctx, "memory: session topic model returned an empty response, using the query as the topic")
		return fallback
	}
	return retrieval.TruncateBytes(strings.TrimSpace(resp.Content), SessionTopicByteLimit)
}

// CompactStream 折叠给定 evicted（seq 升序、均 > 当前游标）进滚动摘要并落库，再按 memory.enabled
// 抽取长期记忆；onDelta 经 SummarizeStream 逐块回调摘要增量。语义与 Compact 等价，区别是折叠集合
// 由调用方给定且摘要可流式产出。reason 为本次推进的原因（rolling / auto / manual），metrics 为该次
// 推进的真实度量（压缩前占用与折叠条数），二者随摘要落库一并写入压缩检查点——reason 供回滚恢复更早
// 的上下文起点、并供读取侧判定是否回显，metrics 供刷新后回显历史压缩记录。
func (m *Service) CompactStream(ctx context.Context, session *ai.AgentSession, evicted []*ai.AgentMessage, reason string, metrics contract.CompactionMetrics, onDelta func(string) error) (*contract.CompactResult, error) {
	if session == nil {
		return nil, errors.New("memory: nil session")
	}
	result := &contract.CompactResult{Summary: string(session.Summary.Value())}
	if len(evicted) == 0 {
		return result, nil
	}
	summary, upto, usage, err := m.SummarizeStream(ctx, session, evicted, onDelta)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// 压缩后占用的即时估算：优先取摘要模型用量算出的估值，用量缺失或该估值不合理时回落为按字节折算
	// 的估值（见 EstimatedCompactedTokens），使压缩完成即刻总有可展示的占用值。该估值随检查点一并落库，
	// 故刷新 / 重进会话后仍可回显前后对比；后续生成由供应商测得的真实 prompt tokens 到达后即被覆盖。
	metrics.TokensAfter = EstimatedCompactedTokens(metrics.TokensBefore, usage, evicted, summary, m.params.Int("memory.evictedMessageByteLimit"))
	if err := m.sessions.UpdateSummaryAndCheckpoint(ctx, session, summary, upto, reason, metrics); err != nil {
		return nil, errors.WithStack(err)
	}
	session.Summary = serializer.NewCrypto([]byte(summary))
	session.SummaryUptoSeq = upto
	result.Summary = summary
	result.TokensAfter = metrics.TokensAfter
	if !m.params.Bool("memory.enabled") {
		result.Warnings = append(result.Warnings, "memory: long-term memory is disabled, extraction skipped")
		return result, nil
	}
	extracted, warnings := m.extract(ctx, session, evicted)
	result.Extracted = extracted
	result.Warnings = append(result.Warnings, warnings...)
	return result, nil
}

// EstimatedCompactedTokens 估算一次压缩后会话的上下文占用：优先取摘要模型用量算出的估值
// （见 EstimatedTokensAfter），用量缺失或该估值不合理时回落为按字节折算的估值（见
// EstimatedTokensByBytes），使压缩完成即刻总有可展示的占用估算。两者都无解时返回 0，
// 调用方据此不携带 TokensAfter，回落为等待真实测量回填。
func EstimatedCompactedTokens(before int, usage *schema.TokenUsage, evicted []*ai.AgentMessage, summary string, perMessageByteLimit int) int {
	if after := EstimatedTokensAfter(before, usage); after > 0 {
		return after
	}
	return EstimatedTokensByBytes(before, evicted, summary, perMessageByteLimit)
}

// EstimatedTokensByBytes 在没有可用的摘要模型用量时按字节折算压缩后占用：压缩前占用加上「新摘要 −
// 被折叠内容」的字节差（按 approxBytesPerToken 折算成 token）。被折叠内容的字节数取
// buildMessageTranscript 的产物长度——即摘要提示词中真正呈现的那份文本（单条按 perMessageByteLimit
// 截断），与实际入参口径一致。新摘要替换的是「旧摘要 + 被折叠内容」，此处只减被折叠内容，故该估值
// 偏保守（偏高）。压缩前占用为 0（无基线）或折算结果不构成缩减（≤0 或 ≥压缩前占用）时返回 0，
// 不给出无意义的估值。
func EstimatedTokensByBytes(before int, evicted []*ai.AgentMessage, summary string, perMessageByteLimit int) int {
	if before <= 0 {
		return 0
	}
	folded := messageTranscriptBytes(evicted, perMessageByteLimit)
	after := before + (len(summary)-folded)/approxBytesPerToken
	if after <= 0 || after >= before {
		return 0
	}
	return after
}

// EstimatedTokensAfter 依据摘要调用的真实用量估算压缩后的上下文占用：压缩前占用减去交给摘要
// 模型的 prompt tokens（即被折叠内容在提示词中的占用），再加上 completion tokens（新摘要自身）。
// 该值只是估算——摘要提示词的系统提示与逐条截断都会引入偏差——故仅用于压缩完成后立即展示；
// 后续生成由供应商测得的真实 prompt tokens 到达后由前端校准为精确值。
//
// 估算只在输入可信且结果合理时给出，否则返回 0（调用方据此改用字节折算的估值，见
// EstimatedCompactedTokens）：压缩前占用必须为正——为 0 时本就没有可用的基线（如会话最近一条助手
// 消息被取消 / 失败，从未记下用量），此时公式会因「新摘要 completion 大于摘要 prompt」而得出比压缩前
// 更大的无意义正值；且估算结果必须为正、必须严格小于压缩前占用——压缩不可能抬高占用，凡不满足者一律
// 视为估算失效。
func EstimatedTokensAfter(before int, usage *schema.TokenUsage) int {
	if usage == nil || before <= 0 {
		return 0
	}
	after := before - usage.PromptTokens + usage.CompletionTokens
	if after <= 0 || after >= before {
		return 0
	}
	return after
}

// Compact 压缩一个会话：先把 summary_upto_seq 之后的全部消息折叠进滚动摘要并落库，再
// 用 flash 从中抽取不超过 memory.extractMaxItems 条长期记忆写入记忆库。摘要失败（flash 不可用
// 或模型空响应）返回 (nil, err)；长期记忆抽取或落库失败只追加进 Warnings、不影响摘要结果，函数
// 仍返回 (result, nil)。memory.enabled=false 时仍刷新摘要，但跳过长期记忆抽取（Extracted=0）。
func (m *Service) Compact(ctx context.Context, session *ai.AgentSession) (*contract.CompactResult, error) {
	if session == nil {
		return nil, errors.New("memory: nil session")
	}
	msgs, err := m.sessions.ListMessagesAfter(ctx, session.ID, session.NamespaceName, session.UserID, session.SummaryUptoSeq)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// 度量口径见 ContextTokens（末次真实测量；会话尚无任何测量时为 0，表示「尚无测量」而非
	// 「占用为 0」）：与是否展示无关，三种推进原因均落值，读取侧再按 reason 决定是否回显。
	contextTokens, err := m.sessions.ContextTokens(ctx, session.ID, session.NamespaceName, session.UserID)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return m.CompactStream(ctx, session, msgs, "manual", contract.CompactionMetrics{TokensBefore: contextTokens, FoldedCount: len(msgs)}, nil)
}
