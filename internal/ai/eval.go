package ai

import (
	// 标准库
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	// 第三方
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// EvalStage 是 dry-run 评估流程的阶段标识（检索 + 4 次评估调用）。
type EvalStage string

// 评估阶段常量：SSE 流上 stage / delta 事件的 stage 字段取值。
const (
	// EvalStageRetrieve 混合检索阶段
	EvalStageRetrieve EvalStage = "retrieve"
	// EvalStageAnswer 基于上下文生成回答阶段
	EvalStageAnswer EvalStage = "answer"
	// EvalStageChunkRelevance 分块相关性判定阶段
	EvalStageChunkRelevance EvalStage = "chunkRelevance"
	// EvalStageClaims 答案声明抽取与支撑判定阶段
	EvalStageClaims EvalStage = "claims"
	// EvalStageReverseQuestions 反推问题生成阶段
	EvalStageReverseQuestions EvalStage = "reverseQuestions"
	// EvalStageSuggestions 改进建议生成阶段（依据检索结果、指标与实际生效的检索参数给出）
	EvalStageSuggestions EvalStage = "suggestions"
	// EvalStageToolCall 工具调用阶段（Agent 节点 4 准备/发起一次工具调用）
	EvalStageToolCall EvalStage = "toolCall"
	// EvalStageToolResult 工具结果阶段（Agent 节点 4 收到一次工具执行的结果）
	EvalStageToolResult EvalStage = "toolResult"
)

// EvalStageStatus 是阶段状态标识；带 Delta 的增量事件不携带状态。
type EvalStageStatus string

// 阶段状态常量。
const (
	// EvalStatusStart 阶段开始
	EvalStatusStart EvalStageStatus = "start"
	// EvalStatusDone 阶段完成
	EvalStatusDone EvalStageStatus = "done"
	// EvalStatusFailed 阶段失败（已降级为 warning）
	EvalStatusFailed EvalStageStatus = "failed"
)

// CallUsage 是一次大模型调用的 token 用量。
type CallUsage struct {
	// PromptTokens 输入 token 数
	PromptTokens int `json:"promptTokens"`
	// CompletionTokens 输出 token 数
	CompletionTokens int `json:"completionTokens"`
	// TotalTokens 合计 token 数
	TotalTokens int `json:"totalTokens"`
}

// CallMeta 是一次大模型调用的元信息（阶段、模型名、耗时与 token 用量）。
type CallMeta struct {
	// Stage 所属评估阶段
	Stage EvalStage `json:"stage"`
	// Model 调用的模型名
	Model string `json:"model"`
	// ElapsedMs 调用耗时（毫秒）
	ElapsedMs int64 `json:"elapsedMs"`
	// Usage token 用量（模型未返回用量时为零值）
	Usage CallUsage `json:"usage"`
}

// StageEvent 是阶段上报事件：Status 非空时为阶段状态事件（start/done/failed），
// Delta 非空时为该阶段的增量输出事件。
type StageEvent struct {
	// Stage 阶段标识
	Stage EvalStage `json:"stage"`
	// Status 阶段状态（start/done/failed）；增量事件为空
	Status EvalStageStatus `json:"status,omitempty"`
	// Delta 该阶段的模型增量原始输出；状态事件为空
	Delta string `json:"delta,omitempty"`
	// ElapsedMs 阶段耗时（毫秒，仅 done/failed 有意义）
	ElapsedMs int64 `json:"elapsedMs"`
	// Usage token 用量（仅 done 有意义，模型未返回用量时为 nil）
	Usage *CallUsage `json:"usage,omitempty"`
	// Err 阶段失败原因（仅 failed 有值）
	Err string `json:"error,omitempty"`
	// Tool 本轮事件的工具名（仅 toolCall / toolResult 阶段有值，其他阶段为空）
	Tool string `json:"tool,omitempty"`
}

// EvalReporter 是评估阶段事件的实时上报回调：返回错误（如 SSE 写失败/客户端断开）
// 会中止后续阶段，避免产生无谓的大模型调用开销；为 nil 时不上报。
type EvalReporter = func(StageEvent) error

// DryRunMetrics 是四项 RAGAS 风格量化指标（均 0..1，1 最优）。
type DryRunMetrics struct {
	// ContextPrecision 上下文精确率：相关分块在检索结果中的前置分布质量
	ContextPrecision float64 `json:"contextPrecision"`
	// Faithfulness 忠实度：答案声明可由检索上下文支撑的比例
	Faithfulness float64 `json:"faithfulness"`
	// AnswerRelevance 答案相关度：反推问题与原始查询的嵌入余弦相似度均值
	AnswerRelevance float64 `json:"answerRelevance"`
	// ContextUtilization 上下文利用率：相关分块中被答案声明引用的比例
	ContextUtilization float64 `json:"contextUtilization"`
}

// ChunkJudgment 是单个检索分块的相关性判断（index 为分块在检索结果中的下标）。
type ChunkJudgment struct {
	// Index 分块在检索结果中的下标
	Index int `json:"index"`
	// Relevant 是否与查询相关
	Relevant bool `json:"relevant"`
	// Score 相关度分数（0..1）
	Score float64 `json:"score"`
	// Reason 相关/不相关的英文简短说明
	Reason string `json:"reason"`
}

// FaithfulnessClaim 是答案中的单条声明及支撑判定（chunkIndexes 为支撑该声明的分块下标）。
type FaithfulnessClaim struct {
	// Statement 声明文本
	Statement string `json:"statement"`
	// Supported 该声明是否可由检索上下文推断
	Supported bool `json:"supported"`
	// ChunkIndexes 支撑该声明的分块下标（不支持时为空数组）
	ChunkIndexes []int `json:"chunkIndexes"`
}

// DiagnosisCategory 是改进建议的封闭归类：模型只能从上列取值中选一个，未知取值由
// normalizeDiagnosisCategory 兜底为 DiagnosisCategoryOther，保证前端可以按类别稳定映射图标
// 与文案，而无需正则匹配自由文本。
type DiagnosisCategory string

// 改进建议归类常量。
const (
	// DiagnosisCategoryCoverage 知识覆盖不足：知识库缺少回答该问题所需的文档
	DiagnosisCategoryCoverage DiagnosisCategory = "coverage"
	// DiagnosisCategoryRanking 检索排序问题：融合策略、权重、条数等检索参数需要调整
	DiagnosisCategoryRanking DiagnosisCategory = "ranking"
	// DiagnosisCategoryGeneration 生成质量问题：回答本身偏离问题或忽略上下文
	DiagnosisCategoryGeneration DiagnosisCategory = "generation"
	// DiagnosisCategoryContext 上下文利用问题：检索到的上下文相关但未被回答充分使用
	DiagnosisCategoryContext DiagnosisCategory = "context"
	// DiagnosisCategoryOther 其他：不属于上述归类的建议
	DiagnosisCategoryOther DiagnosisCategory = "other"
)

// DiagnosisSuggestion 是一条改进建议：Category 为封闭归类，Detail 为按请求语言输出的
// 自由文本（说明问题与应采取的具体动作）。
type DiagnosisSuggestion struct {
	// Category 建议归类
	Category DiagnosisCategory `json:"category"`
	// Detail 建议正文（请求语言）
	Detail string `json:"detail"`
}

// diagnosisContext 是改进建议阶段所需的上下文：实际生效的检索参数（用于给出可执行的调参
// 建议）与请求语言标签（用于要求模型以该语言输出建议正文）。检索参数取自
// Retriever.retrieveOptions，即与本次检索完全一致的默认值与归一化结果。
type diagnosisContext struct {
	// Options 本次检索实际生效的参数
	Options RetrieveOptions
	// Language 请求语言标签（BCP-47，如 zh-Hans / en）；为空时不下发语言指令
	Language string
}

// normalizeDiagnosisCategory 把模型给出的归类字面量归一化到封闭取值：大小写与空白不敏感，
// 未知取值兜底为 DiagnosisCategoryOther。
func normalizeDiagnosisCategory(raw string) DiagnosisCategory {
	switch DiagnosisCategory(strings.ToLower(strings.TrimSpace(raw))) {
	case DiagnosisCategoryCoverage:
		return DiagnosisCategoryCoverage
	case DiagnosisCategoryRanking:
		return DiagnosisCategoryRanking
	case DiagnosisCategoryGeneration:
		return DiagnosisCategoryGeneration
	case DiagnosisCategoryContext:
		return DiagnosisCategoryContext
	default:
		return DiagnosisCategoryOther
	}
}

// DryRunEvaluation 是一次 dry-run 的评估结果。
type DryRunEvaluation struct {
	// Answer 基于检索上下文生成的回答（生成失败时为空）
	Answer string `json:"answer"`
	// Metrics 四项 RAGAS 风格量化指标
	Metrics DryRunMetrics `json:"metrics"`
	// ChunkJudgments 各分块的相关性判断（判断失败时为空）
	ChunkJudgments []ChunkJudgment `json:"chunkJudgments"`
	// Claims 答案声明与支撑判定（无答案或抽取失败时为空）
	Claims []FaithfulnessClaim `json:"claims"`
	// ReverseQuestions 反推的问题（无答案或生成失败时为空）
	ReverseQuestions []string `json:"reverseQuestions"`
	// Suggestions 依据检索结果与指标给出的改进建议（生成失败时为空）
	Suggestions []DiagnosisSuggestion `json:"suggestions"`
	// RelevantCount 被判定为相关的分块数
	RelevantCount int `json:"relevantCount"`
	// TotalChunks 参与评估的检索分块总数
	TotalChunks int `json:"totalChunks"`
	// Warnings 评估过程中的降级/失败说明（为空表示全部步骤成功）
	Warnings []string `json:"warnings"`
}

// DryRunMeta 是一次 dry-run 的调用元信息汇总（检索耗时、各次调用明细、总计与 token 汇总）。
type DryRunMeta struct {
	// RetrieveElapsedMs 检索阶段耗时（毫秒）
	RetrieveElapsedMs int64 `json:"retrieveElapsedMs"`
	// Calls 各次大模型调用的明细（失败阶段同样计入）
	Calls []CallMeta `json:"calls"`
	// TotalElapsedMs 整个 dry-run 的总耗时（毫秒）
	TotalElapsedMs int64 `json:"totalElapsedMs"`
	// Usage 全部调用的 token 用量汇总
	Usage CallUsage `json:"usage"`
}

// DryRunResult 是一次 dry-run 的完整输出（view 层直接序列化）。
type DryRunResult struct {
	// Query 检索查询文本
	Query string `json:"query"`
	// Results 检索命中的分块列表
	Results []RetrievalResult `json:"results"`
	// Evaluation 量化评估结果（chat 不可用或结果为空时为 nil）
	Evaluation *DryRunEvaluation `json:"evaluation,omitempty"`
	// EvalError 评估被跳过的原因（英文），评估成功时为空
	EvalError string `json:"evalError,omitempty"`
	// Meta 调用元信息汇总（检索耗时、各次调用明细、总计与 token 汇总）
	Meta DryRunMeta `json:"meta"`
}

// streamChat 以流式方式执行一次大模型调用：逐块累积正文并上报增量，记录整体耗时与
// 末个分块携带的 token 用量（eino 流式请求自动带 include_usage）。report 返回错误
// （客户端断开）时立即中断并向上返回该错误。
func streamChat(ctx context.Context, chat chatGenerator, stage EvalStage, messages []*schema.Message, report EvalReporter) (content string, meta CallMeta, err error) {
	start := time.Now()
	meta = CallMeta{Stage: stage}
	defer func() {
		meta.ElapsedMs = time.Since(start).Milliseconds()
	}()
	reader, err := chat.Stream(ctx, messages)
	if err != nil {
		return "", meta, errors.WithStack(err)
	}
	defer reader.Close()
	var sb strings.Builder
	for {
		chunk, recvErr := reader.Recv()
		if recvErr != nil {
			if errors.Is(recvErr, io.EOF) {
				break
			}
			return "", meta, errors.WithStack(recvErr)
		}
		if chunk == nil {
			continue
		}
		// 末个分块（内容为空）携带 token 用量，逐块覆盖保证取到最终值
		if chunk.ResponseMeta != nil && chunk.ResponseMeta.Usage != nil {
			meta.Usage = CallUsage{
				PromptTokens:     chunk.ResponseMeta.Usage.PromptTokens,
				CompletionTokens: chunk.ResponseMeta.Usage.CompletionTokens,
				TotalTokens:      chunk.ResponseMeta.Usage.TotalTokens,
			}
		}
		if chunk.Content == "" {
			continue
		}
		sb.WriteString(chunk.Content)
		if report != nil {
			if reportErr := report(StageEvent{Stage: stage, Delta: chunk.Content}); reportErr != nil {
				return sb.String(), meta, reportErr
			}
		}
	}
	return sb.String(), meta, nil
}

// generateAnswer 基于检索上下文生成回答：system prompt 要求仅依据上下文回答、
// 上下文不含答案时承认不知道，且仅输出 JSON。Answer 为空时返回英文错误。
func generateAnswer(ctx context.Context, params *Params, chat chatGenerator, query string, results []RetrievalResult, report EvalReporter) (string, CallMeta, error) {
	messages := []*schema.Message{
		schema.SystemMessage(params.Text("eval.answerSystemPrompt")),
		schema.UserMessage(buildEvalChunkPrompt(query, results, params.Int("eval.snippetLimit"))),
	}
	content, meta, err := streamChat(ctx, chat, EvalStageAnswer, messages, report)
	if err != nil {
		return "", meta, err
	}
	var parsed struct {
		Answer string `json:"answer"`
	}
	body := stripJSONFence(content)
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return "", meta, errors.Newf("dry-run: cannot parse answer response as JSON: %q", truncateText(body, 200))
	}
	if strings.TrimSpace(parsed.Answer) == "" {
		return "", meta, errors.New("dry-run: answer response contains an empty answer")
	}
	return parsed.Answer, meta, nil
}

// judgeChunkRelevance 让模型逐块判断与查询的相关性并输出 JSON 判断数组：
// 丢弃 index 越界的条目，score 钳制到 0..1，reason 原样保留。
func judgeChunkRelevance(ctx context.Context, params *Params, chat chatGenerator, query string, results []RetrievalResult, report EvalReporter) ([]ChunkJudgment, CallMeta, error) {
	messages := []*schema.Message{
		schema.SystemMessage(params.Text("eval.chunkRelevanceSystemPrompt")),
		schema.UserMessage(buildEvalChunkPrompt(query, results, params.Int("eval.snippetLimit"))),
	}
	content, meta, err := streamChat(ctx, chat, EvalStageChunkRelevance, messages, report)
	if err != nil {
		return nil, meta, err
	}
	var parsed struct {
		Judgments []struct {
			Index    int     `json:"index"`
			Relevant bool    `json:"relevant"`
			Score    float64 `json:"score"`
			Reason   string  `json:"reason"`
		} `json:"judgments"`
	}
	body := stripJSONFence(content)
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return nil, meta, errors.Newf("dry-run: cannot parse chunk relevance response as JSON: %q", truncateText(body, 200))
	}
	out := make([]ChunkJudgment, 0, len(parsed.Judgments))
	for _, j := range parsed.Judgments {
		if j.Index < 0 || j.Index >= len(results) {
			continue
		}
		if j.Score < 0 {
			j.Score = 0
		}
		if j.Score > 1 {
			j.Score = 1
		}
		out = append(out, ChunkJudgment{Index: j.Index, Relevant: j.Relevant, Score: j.Score, Reason: j.Reason})
	}
	return out, meta, nil
}

// judgeClaims 抽取答案的全部事实声明并逐条判断是否可由检索上下文推断：
// 丢弃 statement 为空的条目，chunkIndexes 中越界下标丢弃。
func judgeClaims(ctx context.Context, params *Params, chat chatGenerator, answer string, results []RetrievalResult, report EvalReporter) ([]FaithfulnessClaim, CallMeta, error) {
	messages := []*schema.Message{
		schema.SystemMessage(params.Text("eval.claimsSystemPrompt")),
		schema.UserMessage(buildClaimsPrompt(answer, results, params.Int("eval.snippetLimit"))),
	}
	content, meta, err := streamChat(ctx, chat, EvalStageClaims, messages, report)
	if err != nil {
		return nil, meta, err
	}
	var parsed struct {
		Claims []struct {
			Statement    string `json:"statement"`
			Supported    bool   `json:"supported"`
			ChunkIndexes []int  `json:"chunkIndexes"`
		} `json:"claims"`
	}
	body := stripJSONFence(content)
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return nil, meta, errors.Newf("dry-run: cannot parse claim faithfulness response as JSON: %q", truncateText(body, 200))
	}
	out := make([]FaithfulnessClaim, 0, len(parsed.Claims))
	for _, cl := range parsed.Claims {
		if strings.TrimSpace(cl.Statement) == "" {
			continue
		}
		indexes := make([]int, 0, len(cl.ChunkIndexes))
		for _, idx := range cl.ChunkIndexes {
			if idx >= 0 && idx < len(results) {
				indexes = append(indexes, idx)
			}
		}
		out = append(out, FaithfulnessClaim{Statement: cl.Statement, Supported: cl.Supported, ChunkIndexes: indexes})
	}
	return out, meta, nil
}

// judgeReverseQuestions 反推答案可能回答的问题：丢弃空串，最多保留参数
// eval.reverseQuestionCount 指定的条数（非正数表示不限制）。
func judgeReverseQuestions(ctx context.Context, params *Params, chat chatGenerator, answer string, report EvalReporter) ([]string, CallMeta, error) {
	messages := []*schema.Message{
		schema.SystemMessage(params.Text("eval.reverseQuestionsSystemPrompt")),
		schema.UserMessage("回答：\n" + answer),
	}
	content, meta, err := streamChat(ctx, chat, EvalStageReverseQuestions, messages, report)
	if err != nil {
		return nil, meta, err
	}
	var parsed struct {
		Questions []string `json:"questions"`
	}
	body := stripJSONFence(content)
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return nil, meta, errors.Newf("dry-run: cannot parse reverse question response as JSON: %q", truncateText(body, 200))
	}
	limit := params.Int("eval.reverseQuestionCount")
	out := make([]string, 0, len(parsed.Questions))
	for _, q := range parsed.Questions {
		if strings.TrimSpace(q) == "" {
			continue
		}
		out = append(out, q)
		if limit > 0 && len(out) == limit {
			break
		}
	}
	return out, meta, nil
}

// judgeSuggestions 依据检索结果、量化指标与本次实际生效的检索参数生成改进建议：
// 丢弃 detail 为空的条目并把归类归一化到封闭取值。
func judgeSuggestions(ctx context.Context, params *Params, chat chatGenerator, query string, ev *DryRunEvaluation, results []RetrievalResult, diag diagnosisContext, report EvalReporter) ([]DiagnosisSuggestion, CallMeta, error) {
	messages := []*schema.Message{
		schema.SystemMessage(suggestionsSystemPrompt(params, diag.Language)),
		schema.UserMessage(buildSuggestionsPrompt(query, ev, results, diag.Options, params.Int("eval.snippetLimit"))),
	}
	content, meta, err := streamChat(ctx, chat, EvalStageSuggestions, messages, report)
	if err != nil {
		return nil, meta, err
	}
	var parsed struct {
		Suggestions []struct {
			Category string `json:"category"`
			Detail   string `json:"detail"`
		} `json:"suggestions"`
	}
	body := stripJSONFence(content)
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return nil, meta, errors.Newf("dry-run: cannot parse suggestion response as JSON: %q", truncateText(body, 200))
	}
	out := make([]DiagnosisSuggestion, 0, len(parsed.Suggestions))
	for _, s := range parsed.Suggestions {
		if strings.TrimSpace(s.Detail) == "" {
			continue
		}
		out = append(out, DiagnosisSuggestion{Category: normalizeDiagnosisCategory(s.Category), Detail: s.Detail})
	}
	return out, meta, nil
}

// suggestionsSystemPrompt 拼装改进建议阶段的系统提示词：以管理员可编辑的
// eval.suggestionsSystemPrompt 出厂值为主体，追加以请求语言输出建议正文的指令。语言指令
// 由代码拼接而不是写进提示词，避免管理员编辑提示词时丢失语言约束。
func suggestionsSystemPrompt(params *Params, languageTag string) string {
	base := params.Text("eval.suggestionsSystemPrompt")
	if strings.TrimSpace(languageTag) == "" {
		return base
	}
	return base + "\n\n请用 BCP-47 语言标签 \"" + languageTag + "\" 指定的语言书写每一条 \"detail\"" +
		"；JSON 键与类别标识保持英文。"
}

// buildSuggestionsPrompt 构造改进建议请求的用户消息：列出查询、实际生效的检索参数、生成的
// 回答、量化指标、带相关性判定的检索分块与降级警告，使模型能区分「知识不全」与「排序/参数
// 不当」两类问题。answer 为空（生成失败）时省略回答段。
func buildSuggestionsPrompt(query string, ev *DryRunEvaluation, results []RetrievalResult, opts RetrieveOptions, snippetLimit int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "查询：%s\n\n", query)
	fmt.Fprintf(&sb, "实际生效的检索参数：topK=%d, candidates=%d, fusion=%s, weightVector=%g, weightKeyword=%g\n\n",
		opts.TopK, opts.Candidates, opts.Fusion, opts.Weights.Vector, opts.Weights.Keyword)
	if ev.Answer != "" {
		fmt.Fprintf(&sb, "回答：\n%s\n\n", ev.Answer)
	} else {
		sb.WriteString("回答：（生成失败，未产出回答）\n\n")
	}
	fmt.Fprintf(&sb, "指标：contextPrecision=%g, faithfulness=%g, answerRelevance=%g, contextUtilization=%g, relevantChunks=%d/%d\n\n",
		ev.Metrics.ContextPrecision, ev.Metrics.Faithfulness, ev.Metrics.AnswerRelevance,
		ev.Metrics.ContextUtilization, ev.RelevantCount, ev.TotalChunks)
	sb.WriteString("上下文分块：\n")
	judged := make(map[int]ChunkJudgment, len(ev.ChunkJudgments))
	for _, j := range ev.ChunkJudgments {
		judged[j.Index] = j
	}
	for i, r := range results {
		if j, ok := judged[i]; ok {
			fmt.Fprintf(&sb, "[%d] %s（relevant=%t, score=%g）\n%s\n\n", i, r.Title, j.Relevant, j.Score, truncateText(r.Content, snippetLimit))
			continue
		}
		fmt.Fprintf(&sb, "[%d] %s（未评判）\n%s\n\n", i, r.Title, truncateText(r.Content, snippetLimit))
	}
	if len(ev.Warnings) > 0 {
		fmt.Fprintf(&sb, "告警：\n- %s\n", strings.Join(ev.Warnings, "\n- "))
	}
	return sb.String()
}

// buildEvalChunkPrompt 构造评估请求的用户消息：列出 query 与带序号的检索分块
// （正文经 truncateText 截断到 snippetLimit 字符）。
func buildEvalChunkPrompt(query string, results []RetrievalResult, snippetLimit int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "查询：%s\n\n上下文分块：\n", query)
	for i, r := range results {
		fmt.Fprintf(&sb, "[%d] %s\n%s\n\n", i, r.Title, truncateText(r.Content, snippetLimit))
	}
	return sb.String()
}

// buildClaimsPrompt 构造声明判定请求的用户消息：列出 answer 与带序号的检索分块
// （正文经 truncateText 截断到 snippetLimit 字符）。
func buildClaimsPrompt(answer string, results []RetrievalResult, snippetLimit int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "回答：\n%s\n\n上下文分块：\n", answer)
	for i, r := range results {
		fmt.Fprintf(&sb, "[%d] %s\n%s\n\n", i, r.Title, truncateText(r.Content, snippetLimit))
	}
	return sb.String()
}

// contextPrecision 计算 RAGAS 上下文精确率：相关分块的 1-based 位置集合 R 上各
// precision@k（前 k 个位置中相关数 / k）的均值；无相关分块时返回 0。
func contextPrecision(judgments []ChunkJudgment) float64 {
	// 模型可能不按检索顺序返回判断，先按分块下标排序，使前 k 位的分母与检索顺序一致
	sorted := make([]ChunkJudgment, len(judgments))
	copy(sorted, judgments)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Index < sorted[j].Index })
	var relevant int
	var sum float64
	for k, j := range sorted {
		if j.Relevant {
			relevant++
			sum += float64(relevant) / float64(k+1)
		}
	}
	if relevant == 0 {
		return 0
	}
	return sum / float64(relevant)
}

// faithfulness 计算 RAGAS 忠实度：supported 声明数 / 声明总数；无声明时返回 0。
func faithfulness(claims []FaithfulnessClaim) float64 {
	if len(claims) == 0 {
		return 0
	}
	var supported int
	for _, c := range claims {
		if c.Supported {
			supported++
		}
	}
	return float64(supported) / float64(len(claims))
}

// contextUtilization 计算 RAGAS 上下文利用率：相关分块集合 R 与被 supported 声明引用的
// 分块集合 C 的交集大小 / |R|；无相关分块时返回 0。
func contextUtilization(judgments []ChunkJudgment, claims []FaithfulnessClaim) float64 {
	relevant := make(map[int]bool, len(judgments))
	for _, j := range judgments {
		if j.Relevant {
			relevant[j.Index] = true
		}
	}
	if len(relevant) == 0 {
		return 0
	}
	used := make(map[int]bool)
	for _, c := range claims {
		if !c.Supported {
			continue
		}
		for _, idx := range c.ChunkIndexes {
			used[idx] = true
		}
	}
	var overlap int
	for idx := range relevant {
		if used[idx] {
			overlap++
		}
	}
	return float64(overlap) / float64(len(relevant))
}

// answerRelevance 计算 RAGAS 答案相关度：query 与各反推问题的嵌入余弦相似度均值；
// questions 为空时返回 (0, nil)；嵌入返回的向量数与输入文本数不符时返回英文错误。
func answerRelevance(ctx context.Context, embed embedder, query string, questions []string) (float64, error) {
	if len(questions) == 0 {
		return 0, nil
	}
	texts := make([]string, 0, len(questions)+1)
	texts = append(texts, query)
	texts = append(texts, questions...)
	vecs, err := embed.EmbedStrings(ctx, texts)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	if len(vecs) != len(texts) {
		return 0, errors.Newf("dry-run: answer relevance embedding returned %d vectors for %d texts", len(vecs), len(texts))
	}
	var sum float64
	for _, v := range vecs[1:] {
		sum += cosineSimilarity(vecs[0], v)
	}
	return sum / float64(len(questions)), nil
}

// cosineSimilarity 计算两个向量的余弦相似度（零向量或长度不等时返回 0）。
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// evaluate 对给定检索结果执行完整评估：生成回答 → 分块相关性 → 声明抽取 → 反推问题 →
// 改进建议。每步经 report 上报 start / done / failed 事件并实时透传模型增量输出；单步失败
// 不中断整体流程，而是记录 warning 并将对应指标置 0；chat 为 nil 或 results 为空时返回英文
// 错误（上层转为 evalError，不产出 evaluation）。report 返回错误（客户端断开）时立即
// 中止后续阶段并向上返回该错误。返回的 []CallMeta 为各次调用的耗时与 token 明细。
func evaluate(ctx context.Context, params *Params, chat chatGenerator, embed embedder, query string, results []RetrievalResult, diag diagnosisContext, report EvalReporter) (*DryRunEvaluation, []CallMeta, error) {
	if chat == nil {
		return nil, nil, errors.New("dry-run: chat client unavailable, evaluation skipped")
	}
	if len(results) == 0 {
		return nil, nil, errors.New("dry-run: no retrieval results to evaluate")
	}
	ev := &DryRunEvaluation{
		// 空 slice 而非 nil，保证 JSON 序列化为 [] 而不是 null，避免前端对数组字段做
		// .length 等读取时崩溃
		ChunkJudgments:   []ChunkJudgment{},
		Claims:           []FaithfulnessClaim{},
		ReverseQuestions: []string{},
		Suggestions:      []DiagnosisSuggestion{},
		Warnings:         []string{},
		TotalChunks:      len(results),
	}
	calls := make([]CallMeta, 0, 5)

	// emit 上报阶段事件；首次写失败（客户端断开）被记录后，后续 emit 直接返回该错误，
	// 使剩余阶段立即中止，避免继续产生无谓的大模型调用开销
	var emitErr error
	emit := func(ev StageEvent) error {
		if emitErr != nil {
			return emitErr
		}
		if report == nil {
			return nil
		}
		emitErr = report(ev)
		return emitErr
	}
	fail := func(stage EvalStage, warnPrefix string, meta CallMeta, err error) {
		ev.Warnings = append(ev.Warnings, "dry-run: "+warnPrefix+": "+err.Error())
		_ = emit(StageEvent{Stage: stage, Status: EvalStatusFailed, ElapsedMs: meta.ElapsedMs, Err: err.Error()})
	}

	answer, call, err := runStage(EvalStageAnswer, emit, func() (string, CallMeta, error) {
		return generateAnswer(ctx, params, chat, query, results, emit)
	})
	calls = append(calls, call)
	if emitErr != nil {
		return ev, calls, emitErr
	}
	if err != nil {
		fail(EvalStageAnswer, "answer generation failed", call, err)
	} else {
		ev.Answer = answer
		_ = emit(StageEvent{Stage: EvalStageAnswer, Status: EvalStatusDone, ElapsedMs: call.ElapsedMs, Usage: &call.Usage})
	}
	if emitErr != nil {
		return ev, calls, emitErr
	}

	var judgments []ChunkJudgment
	judgments, call, err = runStage(EvalStageChunkRelevance, emit, func() ([]ChunkJudgment, CallMeta, error) {
		return judgeChunkRelevance(ctx, params, chat, query, results, emit)
	})
	calls = append(calls, call)
	if emitErr != nil {
		return ev, calls, emitErr
	}
	if err != nil {
		fail(EvalStageChunkRelevance, "chunk relevance judgment failed", call, err)
	} else {
		ev.ChunkJudgments = judgments
		for _, j := range judgments {
			if j.Relevant {
				ev.RelevantCount++
			}
		}
		ev.Metrics.ContextPrecision = contextPrecision(judgments)
		_ = emit(StageEvent{Stage: EvalStageChunkRelevance, Status: EvalStatusDone, ElapsedMs: call.ElapsedMs, Usage: &call.Usage})
	}
	if emitErr != nil {
		return ev, calls, emitErr
	}

	// 声明抽取与反推问题依赖答案，仅在生成成功时执行
	if ev.Answer != "" {
		var claims []FaithfulnessClaim
		claims, call, err = runStage(EvalStageClaims, emit, func() ([]FaithfulnessClaim, CallMeta, error) {
			return judgeClaims(ctx, params, chat, ev.Answer, results, emit)
		})
		calls = append(calls, call)
		if emitErr != nil {
			return ev, calls, emitErr
		}
		if err != nil {
			fail(EvalStageClaims, "claim faithfulness judgment failed", call, err)
		} else {
			ev.Claims = claims
			ev.Metrics.Faithfulness = faithfulness(claims)
			ev.Metrics.ContextUtilization = contextUtilization(judgments, claims)
			_ = emit(StageEvent{Stage: EvalStageClaims, Status: EvalStatusDone, ElapsedMs: call.ElapsedMs, Usage: &call.Usage})
		}
		if emitErr != nil {
			return ev, calls, emitErr
		}

		var questions []string
		questions, call, err = runStage(EvalStageReverseQuestions, emit, func() ([]string, CallMeta, error) {
			return judgeReverseQuestions(ctx, params, chat, ev.Answer, emit)
		})
		calls = append(calls, call)
		if emitErr != nil {
			return ev, calls, emitErr
		}
		if err != nil {
			fail(EvalStageReverseQuestions, "reverse question generation failed", call, err)
		} else {
			ev.ReverseQuestions = questions
			if ar, arErr := answerRelevance(ctx, embed, query, questions); arErr != nil {
				ev.Warnings = append(ev.Warnings, "dry-run: answer relevance embedding failed: "+arErr.Error())
			} else {
				ev.Metrics.AnswerRelevance = ar
			}
			_ = emit(StageEvent{Stage: EvalStageReverseQuestions, Status: EvalStatusDone, ElapsedMs: call.ElapsedMs, Usage: &call.Usage})
		}
	}
	if emitErr != nil {
		return ev, calls, emitErr
	}

	// 改进建议不依赖回答是否生成成功：回答失败本身即是重要诊断线索，故该阶段置于
	// ev.Answer 分支之外
	var suggestions []DiagnosisSuggestion
	suggestions, call, err = runStage(EvalStageSuggestions, emit, func() ([]DiagnosisSuggestion, CallMeta, error) {
		return judgeSuggestions(ctx, params, chat, query, ev, results, diag, emit)
	})
	calls = append(calls, call)
	if emitErr != nil {
		return ev, calls, emitErr
	}
	if err != nil {
		fail(EvalStageSuggestions, "suggestion generation failed", call, err)
	} else {
		ev.Suggestions = suggestions
		_ = emit(StageEvent{Stage: EvalStageSuggestions, Status: EvalStatusDone, ElapsedMs: call.ElapsedMs, Usage: &call.Usage})
	}
	if emitErr != nil {
		return ev, calls, emitErr
	}
	return ev, calls, nil
}

// runStage 执行一个评估阶段：先上报 start 事件，再执行 call；emit 返回错误（客户端
// 断开）时不再执行 call，直接将错误上抛。
func runStage[T any](stage EvalStage, emit func(StageEvent) error, call func() (T, CallMeta, error)) (T, CallMeta, error) {
	var zero T
	if err := emit(StageEvent{Stage: stage, Status: EvalStatusStart}); err != nil {
		return zero, CallMeta{Stage: stage}, err
	}
	return call()
}
