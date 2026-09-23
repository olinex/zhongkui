package ai

import (
	// stdlib
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	// third-party
	"github.com/cloudwego/eino/schema"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/llm"
)

// EvalStage 是 dry-run 评估流程的阶段标识（检索 + 4 次评估调用）。
// EvalStage is the stage identifier of the dry-run evaluation flow (retrieval plus the
// four evaluation calls).
type EvalStage string

// 评估阶段常量：SSE 流上 stage / delta 事件的 stage 字段取值。
// Evaluation stage constants: the stage field values of the stage / delta events on the
// SSE stream.
const (
	// EvalStageRetrieve 混合检索阶段
	// EvalStageRetrieve the hybrid retrieval stage
	EvalStageRetrieve EvalStage = "retrieve"
	// EvalStageAnswer 基于上下文生成回答阶段
	// EvalStageAnswer the context-based answer generation stage
	EvalStageAnswer EvalStage = "answer"
	// EvalStageChunkRelevance 分块相关性判定阶段
	// EvalStageChunkRelevance the chunk relevance judgment stage
	EvalStageChunkRelevance EvalStage = "chunkRelevance"
	// EvalStageClaims 答案声明抽取与支撑判定阶段
	// EvalStageClaims the answer claim extraction and support judgment stage
	EvalStageClaims EvalStage = "claims"
	// EvalStageReverseQuestions 反推问题生成阶段
	// EvalStageReverseQuestions the reverse question generation stage
	EvalStageReverseQuestions EvalStage = "reverseQuestions"
)

// EvalStageStatus 是阶段状态标识；带 Delta 的增量事件不携带状态。
// EvalStageStatus is the stage status identifier; delta events carrying a Delta have no
// status.
type EvalStageStatus string

// 阶段状态常量。
// Stage status constants.
const (
	// EvalStatusStart 阶段开始
	// EvalStatusStart the stage started
	EvalStatusStart EvalStageStatus = "start"
	// EvalStatusDone 阶段完成
	// EvalStatusDone the stage completed
	EvalStatusDone EvalStageStatus = "done"
	// EvalStatusFailed 阶段失败（已降级为 warning）
	// EvalStatusFailed the stage failed (degraded to a warning)
	EvalStatusFailed EvalStageStatus = "failed"
)

// CallUsage 是一次大模型调用的 token 用量。
// CallUsage is the token usage of a single LLM call.
type CallUsage struct {
	// PromptTokens 输入 token 数
	// PromptTokens the number of input tokens
	PromptTokens int `json:"promptTokens"`
	// CompletionTokens 输出 token 数
	// CompletionTokens the number of output tokens
	CompletionTokens int `json:"completionTokens"`
	// TotalTokens 合计 token 数
	// TotalTokens the total number of tokens
	TotalTokens int `json:"totalTokens"`
}

// CallMeta 是一次大模型调用的元信息（阶段、模型名、耗时与 token 用量）。
// CallMeta is the metadata of a single LLM call (stage, model name, elapsed time and
// token usage).
type CallMeta struct {
	// Stage 所属评估阶段
	// Stage the owning evaluation stage
	Stage EvalStage `json:"stage"`
	// Model 调用的模型名
	// Model the name of the model called
	Model string `json:"model"`
	// ElapsedMs 调用耗时（毫秒）
	// ElapsedMs the call elapsed time in milliseconds
	ElapsedMs int64 `json:"elapsedMs"`
	// Usage token 用量（模型未返回用量时为零值）
	// Usage the token usage (zero when the model reports none)
	Usage CallUsage `json:"usage"`
}

// StageEvent 是阶段上报事件：Status 非空时为阶段状态事件（start/done/failed），
// Delta 非空时为该阶段的增量输出事件。
// StageEvent is a stage report event: a non-empty Status makes it a stage status event
// (start/done/failed), a non-empty Delta makes it an incremental output event of that
// stage.
type StageEvent struct {
	// Stage 阶段标识
	// Stage the stage identifier
	Stage EvalStage `json:"stage"`
	// Status 阶段状态（start/done/failed）；增量事件为空
	// Status the stage status (start/done/failed); empty for delta events
	Status EvalStageStatus `json:"status,omitempty"`
	// Delta 该阶段的模型增量原始输出；状态事件为空
	// Delta the model's incremental raw output of the stage; empty for status events
	Delta string `json:"delta,omitempty"`
	// ElapsedMs 阶段耗时（毫秒，仅 done/failed 有意义）
	// ElapsedMs the stage elapsed time in milliseconds (meaningful for done/failed only)
	ElapsedMs int64 `json:"elapsedMs"`
	// Usage token 用量（仅 done 有意义，模型未返回用量时为 nil）
	// Usage the token usage (meaningful for done only; nil when the model reports none)
	Usage *CallUsage `json:"usage,omitempty"`
	// Err 阶段失败原因（仅 failed 有值）
	// Err the stage failure reason (failed only)
	Err string `json:"error,omitempty"`
}

// EvalReporter 是评估阶段事件的实时上报回调：返回错误（如 SSE 写失败/客户端断开）
// 会中止后续阶段，避免产生无谓的大模型调用开销；为 nil 时不上报。
// EvalReporter is the real-time report callback of evaluation stage events: returning an
// error (e.g. an SSE write failure or a disconnected client) aborts the remaining stages,
// avoiding needless LLM call costs; a nil reporter reports nothing.
type EvalReporter = func(StageEvent) error

// DryRunMetrics 是四项 RAGAS 风格量化指标（均 0..1，1 最优）。
// DryRunMetrics holds the four RAGAS-style quantitative metrics (all in 0..1, 1 is best).
type DryRunMetrics struct {
	// ContextPrecision 上下文精确率：相关分块在检索结果中的前置分布质量
	// ContextPrecision the context precision: how well the relevant chunks lead the ranking
	ContextPrecision float64 `json:"contextPrecision"`
	// Faithfulness 忠实度：答案声明可由检索上下文支撑的比例
	// Faithfulness the faithfulness: the ratio of answer claims supported by the context
	Faithfulness float64 `json:"faithfulness"`
	// AnswerRelevance 答案相关度：反推问题与原始查询的嵌入余弦相似度均值
	// AnswerRelevance the answer relevance: mean embedding cosine similarity between the
	// reverse-engineered questions and the original query
	AnswerRelevance float64 `json:"answerRelevance"`
	// ContextUtilization 上下文利用率：相关分块中被答案声明引用的比例
	// ContextUtilization the context utilization: the ratio of relevant chunks cited by the
	// answer claims
	ContextUtilization float64 `json:"contextUtilization"`
}

// ChunkJudgment 是单个检索分块的相关性判断（index 为分块在检索结果中的下标）。
// ChunkJudgment is the relevance judgment of a single retrieved chunk (index is the
// chunk's position in the retrieval results).
type ChunkJudgment struct {
	// Index 分块在检索结果中的下标
	// Index the chunk index within the retrieval results
	Index int `json:"index"`
	// Relevant 是否与查询相关
	// Relevant whether the chunk is relevant to the query
	Relevant bool `json:"relevant"`
	// Score 相关度分数（0..1）
	// Score the relevance score (0..1)
	Score float64 `json:"score"`
	// Reason 相关/不相关的英文简短说明
	// Reason a short English explanation of the judgment
	Reason string `json:"reason"`
}

// FaithfulnessClaim 是答案中的单条声明及支撑判定（chunkIndexes 为支撑该声明的分块下标）。
// FaithfulnessClaim is a single claim extracted from the answer plus its support judgment
// (chunkIndexes are the chunk indices supporting the claim).
type FaithfulnessClaim struct {
	// Statement 声明文本
	// Statement the claim text
	Statement string `json:"statement"`
	// Supported 该声明是否可由检索上下文推断
	// Supported whether the claim is inferable from the retrieved context
	Supported bool `json:"supported"`
	// ChunkIndexes 支撑该声明的分块下标（不支持时为空数组）
	// ChunkIndexes the chunk indices supporting the claim (an empty array when unsupported)
	ChunkIndexes []int `json:"chunkIndexes"`
}

// DryRunEvaluation 是一次 dry-run 的评估结果。
// DryRunEvaluation is the evaluation result of a single dry-run.
type DryRunEvaluation struct {
	// Answer 基于检索上下文生成的回答（生成失败时为空）
	// Answer the answer generated from the retrieved context (empty when generation failed)
	Answer string `json:"answer"`
	// Metrics 四项 RAGAS 风格量化指标
	// Metrics the four RAGAS-style quantitative metrics
	Metrics DryRunMetrics `json:"metrics"`
	// ChunkJudgments 各分块的相关性判断（判断失败时为空）
	// ChunkJudgments the per-chunk relevance judgments (empty when the judgment failed)
	ChunkJudgments []ChunkJudgment `json:"chunkJudgments"`
	// Claims 答案声明与支撑判定（无答案或抽取失败时为空）
	// Claims the answer claims with support judgments (empty when there is no answer or the
	// extraction failed)
	Claims []FaithfulnessClaim `json:"claims"`
	// ReverseQuestions 反推的问题（无答案或生成失败时为空）
	// ReverseQuestions the reverse-engineered questions (empty when there is no answer or
	// the generation failed)
	ReverseQuestions []string `json:"reverseQuestions"`
	// RelevantCount 被判定为相关的分块数
	// RelevantCount the number of chunks judged relevant
	RelevantCount int `json:"relevantCount"`
	// TotalChunks 参与评估的检索分块总数
	// TotalChunks the total number of retrieved chunks under evaluation
	TotalChunks int `json:"totalChunks"`
	// Warnings 评估过程中的降级/失败说明（为空表示全部步骤成功）
	// Warnings the degradation or failure notes during the evaluation (empty means every
	// step succeeded)
	Warnings []string `json:"warnings"`
}

// DryRunMeta 是一次 dry-run 的调用元信息汇总（检索耗时、各次调用明细、总计与 token 汇总）。
// DryRunMeta is the call-metadata summary of a single dry-run (retrieval elapsed time,
// per-call details, total elapsed time and token totals).
type DryRunMeta struct {
	// RetrieveElapsedMs 检索阶段耗时（毫秒）
	// RetrieveElapsedMs the retrieval stage elapsed time in milliseconds
	RetrieveElapsedMs int64 `json:"retrieveElapsedMs"`
	// Calls 各次大模型调用的明细（失败阶段同样计入）
	// Calls the details of every LLM call (failed stages are counted too)
	Calls []CallMeta `json:"calls"`
	// TotalElapsedMs 整个 dry-run 的总耗时（毫秒）
	// TotalElapsedMs the total elapsed time of the whole dry-run in milliseconds
	TotalElapsedMs int64 `json:"totalElapsedMs"`
	// Usage 全部调用的 token 用量汇总
	// Usage the aggregated token usage of all calls
	Usage CallUsage `json:"usage"`
}

// DryRunResult 是一次 dry-run 的完整输出（view 层直接序列化）。
// DryRunResult is the complete output of a single dry-run (serialized by the view layer
// directly).
type DryRunResult struct {
	// Query 检索查询文本
	// Query the retrieval query text
	Query string `json:"query"`
	// Results 检索命中的分块列表
	// Results the retrieved chunk hits
	Results []RetrievalResult `json:"results"`
	// Evaluation 量化评估结果（chat 不可用或结果为空时为 nil）
	// Evaluation the quantitative evaluation (nil when the chat client is unavailable or no
	// results are retrieved)
	Evaluation *DryRunEvaluation `json:"evaluation,omitempty"`
	// EvalError 评估被跳过的原因（英文），评估成功时为空
	// EvalError the reason the evaluation was skipped (English), empty when the evaluation
	// succeeded
	EvalError string `json:"evalError,omitempty"`
	// Meta 调用元信息汇总（检索耗时、各次调用明细、总计与 token 汇总）
	// Meta the call-metadata summary (retrieval elapsed time, per-call details, total
	// elapsed time and token totals)
	Meta DryRunMeta `json:"meta"`
}

// streamChat 以流式方式执行一次大模型调用：逐块累积正文并上报增量，记录整体耗时与
// 末个分块携带的 token 用量（eino 流式请求自动带 include_usage）。report 返回错误
// （客户端断开）时立即中断并向上返回该错误。
// streamChat performs a single LLM call in streaming mode: it accumulates the content
// chunk by chunk, reports the deltas, and records the overall elapsed time plus the token
// usage carried by the last chunk (eino streaming requests include the usage
// automatically). When report returns an error (a disconnected client) the call aborts
// immediately and the error is propagated.
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
		// The last chunk (with empty content) carries the token usage; overwrite per chunk
		// so the final value wins
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
// generateAnswer generates an answer from the retrieved context: the system prompt asks
// to answer only from the context, admit ignorance when the context lacks the answer, and
// output JSON only. An empty answer yields an English error.
func generateAnswer(ctx context.Context, params *llm.Params, chat chatGenerator, query string, results []RetrievalResult, report EvalReporter) (string, CallMeta, error) {
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
// judgeChunkRelevance asks the model to judge each chunk's relevance to the query and
// output a JSON judgment array: out-of-range index entries are dropped, score is clamped
// to 0..1 and reason is kept as-is.
func judgeChunkRelevance(ctx context.Context, params *llm.Params, chat chatGenerator, query string, results []RetrievalResult, report EvalReporter) ([]ChunkJudgment, CallMeta, error) {
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
// judgeClaims extracts every factual claim of the answer and judges whether each claim
// is inferable from the retrieved context: empty statements are dropped and
// out-of-range chunk indexes are discarded.
func judgeClaims(ctx context.Context, params *llm.Params, chat chatGenerator, answer string, results []RetrievalResult, report EvalReporter) ([]FaithfulnessClaim, CallMeta, error) {
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
// judgeReverseQuestions reverse-engineers the questions the answer could plausibly answer:
// empty strings are dropped and at most the number given by the parameter
// eval.reverseQuestionCount is kept (a non-positive value means no limit).
func judgeReverseQuestions(ctx context.Context, params *llm.Params, chat chatGenerator, answer string, report EvalReporter) ([]string, CallMeta, error) {
	messages := []*schema.Message{
		schema.SystemMessage(params.Text("eval.reverseQuestionsSystemPrompt")),
		schema.UserMessage("Answer:\n" + answer),
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

// buildEvalChunkPrompt 构造评估请求的用户消息：列出 query 与带序号的检索分块
// （正文经 truncateText 截断到 snippetLimit 字符）。
// buildEvalChunkPrompt builds the user message of an evaluation request, listing the
// query and the numbered retrieved chunks (bodies truncated to snippetLimit characters).
func buildEvalChunkPrompt(query string, results []RetrievalResult, snippetLimit int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Query: %s\n\nContext chunks:\n", query)
	for i, r := range results {
		fmt.Fprintf(&sb, "[%d] %s\n%s\n\n", i, r.Title, truncateText(r.Content, snippetLimit))
	}
	return sb.String()
}

// buildClaimsPrompt 构造声明判定请求的用户消息：列出 answer 与带序号的检索分块
// （正文经 truncateText 截断到 snippetLimit 字符）。
// buildClaimsPrompt builds the user message of the claim judgment request, listing the
// answer and the numbered retrieved chunks (bodies truncated to snippetLimit characters).
func buildClaimsPrompt(answer string, results []RetrievalResult, snippetLimit int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Answer:\n%s\n\nContext chunks:\n", answer)
	for i, r := range results {
		fmt.Fprintf(&sb, "[%d] %s\n%s\n\n", i, r.Title, truncateText(r.Content, snippetLimit))
	}
	return sb.String()
}

// contextPrecision 计算 RAGAS 上下文精确率：相关分块的 1-based 位置集合 R 上各
// precision@k（前 k 个位置中相关数 / k）的均值；无相关分块时返回 0。
// contextPrecision computes the RAGAS context precision: the mean of precision@k
// (relevant hits among the first k positions divided by k) over the 1-based relevant
// positions R; 0 when no chunk is relevant.
func contextPrecision(judgments []ChunkJudgment) float64 {
	// 模型可能不按检索顺序返回判断，先按分块下标排序，使前 k 位的分母与检索顺序一致
	// The model may return judgments out of retrieval order; sort by chunk index first so
	// the precision@k denominator follows the retrieval order
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
// faithfulness computes the RAGAS faithfulness: supported claims divided by the total
// claims; 0 when there are no claims.
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
// contextUtilization computes the RAGAS context utilization: |R ∩ C| / |R| where R is the
// set of relevant chunk indices and C the chunk indices cited by supported claims;
// 0 when no chunk is relevant.
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
// answerRelevance computes the RAGAS answer relevance: the mean embedding cosine
// similarity between the query and each reverse-engineered question; empty questions
// yield (0, nil) and a vector-count mismatch yields an English error.
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
// cosineSimilarity computes the cosine similarity of two vectors (0 for zero vectors or
// a length mismatch).
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

// evaluate 对给定检索结果执行完整评估：生成回答 → 分块相关性 → 声明抽取 → 反推问题。
// 每步经 report 上报 start / done / failed 事件并实时透传模型增量输出；单步失败不中断
// 整体流程，而是记录 warning 并将对应指标置 0；chat 为 nil 或 results 为空时返回英文
// 错误（上层转为 evalError，不产出 evaluation）。report 返回错误（客户端断开）时立即
// 中止后续阶段并向上返回该错误。返回的 []CallMeta 为各次调用的耗时与 token 明细。
// evaluate runs the full evaluation over the given retrieval results: answer generation,
// chunk relevance, claim extraction and reverse question generation. Each step reports
// start / done / failed events through report and streams the model deltas in real time; a
// step failure does not abort the flow, it records a warning and zeroes the related
// metrics. A nil chat or empty results yield an English error (the caller turns it into
// evalError without an evaluation). When report returns an error (a disconnected client)
// the remaining stages abort immediately and the error is propagated. The returned
// []CallMeta carries the elapsed time and token usage of every call.
func evaluate(ctx context.Context, params *llm.Params, chat chatGenerator, embed embedder, query string, results []RetrievalResult, report EvalReporter) (*DryRunEvaluation, []CallMeta, error) {
	if chat == nil {
		return nil, nil, errors.New("dry-run: chat client unavailable, evaluation skipped")
	}
	if len(results) == 0 {
		return nil, nil, errors.New("dry-run: no retrieval results to evaluate")
	}
	ev := &DryRunEvaluation{
		// 空 slice 而非 nil，保证 JSON 序列化为 [] 而不是 null，避免前端对数组字段做
		// .length 等读取时崩溃
		// Use empty slices instead of nil so JSON serializes as [] rather than null,
		// keeping array fields safe for frontend reads such as .length
		ChunkJudgments:   []ChunkJudgment{},
		Claims:           []FaithfulnessClaim{},
		ReverseQuestions: []string{},
		Warnings:         []string{},
		TotalChunks:      len(results),
	}
	calls := make([]CallMeta, 0, 4)

	// emit 上报阶段事件；首次写失败（客户端断开）被记录后，后续 emit 直接返回该错误，
	// 使剩余阶段立即中止，避免继续产生无谓的大模型调用开销
	// emit reports a stage event; after the first write failure (a disconnected client) is
	// recorded, later emits return that error directly so the remaining stages abort at
	// once, avoiding needless LLM call costs
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
	// Claim extraction and reverse questions depend on the answer and only run when it exists
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
	return ev, calls, nil
}

// runStage 执行一个评估阶段：先上报 start 事件，再执行 call；emit 返回错误（客户端
// 断开）时不再执行 call，直接将错误上抛。
// runStage runs a single evaluation stage: it reports the start event first, then invokes
// call; when emit returns an error (a disconnected client) call is not invoked and the
// error is propagated directly.
func runStage[T any](stage EvalStage, emit func(StageEvent) error, call func() (T, CallMeta, error)) (T, CallMeta, error) {
	var zero T
	if err := emit(StageEvent{Stage: stage, Status: EvalStatusStart}); err != nil {
		return zero, CallMeta{Stage: stage}, err
	}
	return call()
}
