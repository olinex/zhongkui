package knowledge

import (
	// 标准库
	"context"
	"strings"

	// 第三方
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/config"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/port"
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

// streamChat 以流式方式执行一次大模型调用：逐块累积正文并上报增量，记录整体耗时与
// 末个分块携带的 token 用量（eino 流式请求自动带 include_usage）。report 返回错误
// （客户端断开）时立即中断并向上返回该错误。
func streamChat(ctx context.Context, chat port.ChatGenerator, stage contract.EvalStage, messages []*schema.Message, report contract.EvalReporter) (content string, meta contract.CallMeta, err error) {
	meta = contract.CallMeta{Stage: stage}
	var onDelta func(string) error
	if report != nil {
		onDelta = func(delta string) error {
			return report(contract.StageEvent{Stage: stage, Delta: delta})
		}
	}
	content, usage, elapsed, err := port.StreamChat(ctx, chat, messages, onDelta)
	meta.ElapsedMs = elapsed.Milliseconds()
	if usage != nil {
		meta.Usage = contract.CallUsage{
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
		}
	}
	if err != nil {
		return content, meta, errors.WithStack(err)
	}
	return content, meta, nil
}

// answerResponse 是 dry-run 回答输出的结构化校验视图：answer 必填且不能为空白。
type answerResponse struct {
	// Answer 模型给出的回答正文
	Answer string `json:"answer" binding:"notBlank"`
}

// generateAnswer 基于检索上下文生成回答：system prompt 要求仅依据上下文回答、
// 上下文不含答案时承认不知道，且仅输出 JSON。Answer 为空时返回英文错误。
func generateAnswer(ctx context.Context, params *config.Params, chat port.ChatGenerator, query string, results []retrieval.RetrievalResult, report contract.EvalReporter) (string, contract.CallMeta, error) {
	userMessage, err := buildEvalChunkPrompt(ctx, query, results, params.Int("eval.snippetLimit"))
	if err != nil {
		return "", contract.CallMeta{Stage: contract.EvalStageAnswer}, errors.WithStack(err)
	}
	messages := []*schema.Message{
		schema.SystemMessage(params.Text("eval.answerSystemPrompt")),
		userMessage,
	}
	content, meta, err := streamChat(ctx, chat, contract.EvalStageAnswer, messages, report)
	if err != nil {
		return "", meta, errors.WithStack(err)
	}
	var parsed answerResponse
	if err := validator.DecodeObjectLenient(content, &parsed); err != nil {
		return "", meta, errors.Newf("dry-run: cannot parse answer response: %v", err)
	}
	if err := validator.Check(&parsed); err != nil {
		return "", meta, errors.Newf("dry-run: answer response is invalid: %s", validator.Describe(err))
	}
	return parsed.Answer, meta, nil
}

// judgeChunkRelevance 让模型逐块判断与查询的相关性并输出 JSON 判断数组：
// 丢弃 index 越界的条目，score 钳制到 0..1，reason 原样保留。
func judgeChunkRelevance(ctx context.Context, params *config.Params, chat port.ChatGenerator, query string, results []retrieval.RetrievalResult, report contract.EvalReporter) ([]ChunkJudgment, contract.CallMeta, error) {
	userMessage, err := buildEvalChunkPrompt(ctx, query, results, params.Int("eval.snippetLimit"))
	if err != nil {
		return nil, contract.CallMeta{Stage: contract.EvalStageChunkRelevance}, errors.WithStack(err)
	}
	messages := []*schema.Message{
		schema.SystemMessage(params.Text("eval.chunkRelevanceSystemPrompt")),
		userMessage,
	}
	content, meta, err := streamChat(ctx, chat, contract.EvalStageChunkRelevance, messages, report)
	if err != nil {
		return nil, meta, errors.WithStack(err)
	}
	parsed, perr := contract.ParseJSONResponse[struct {
		Judgments []struct {
			Index    int     `json:"index"`
			Relevant bool    `json:"relevant"`
			Score    float64 `json:"score"`
			Reason   string  `json:"reason"`
		} `json:"judgments"`
	}](content, "dry-run: cannot parse chunk relevance response")
	if perr != nil {
		return nil, meta, perr
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
func judgeClaims(ctx context.Context, params *config.Params, chat port.ChatGenerator, answer string, results []retrieval.RetrievalResult, report contract.EvalReporter) ([]FaithfulnessClaim, contract.CallMeta, error) {
	userMessage, err := buildClaimsPrompt(ctx, answer, results, params.Int("eval.snippetLimit"))
	if err != nil {
		return nil, contract.CallMeta{Stage: contract.EvalStageClaims}, errors.WithStack(err)
	}
	messages := []*schema.Message{
		schema.SystemMessage(params.Text("eval.claimsSystemPrompt")),
		userMessage,
	}
	content, meta, err := streamChat(ctx, chat, contract.EvalStageClaims, messages, report)
	if err != nil {
		return nil, meta, errors.WithStack(err)
	}
	parsed, perr := contract.ParseJSONResponse[struct {
		Claims []struct {
			Statement    string `json:"statement"`
			Supported    bool   `json:"supported"`
			ChunkIndexes []int  `json:"chunkIndexes"`
		} `json:"claims"`
	}](content, "dry-run: cannot parse claim faithfulness response")
	if perr != nil {
		return nil, meta, perr
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
func judgeReverseQuestions(ctx context.Context, params *config.Params, chat port.ChatGenerator, answer string, report contract.EvalReporter) ([]string, contract.CallMeta, error) {
	userMessage, err := buildReverseQuestionsPrompt(ctx, answer)
	if err != nil {
		return nil, contract.CallMeta{Stage: contract.EvalStageReverseQuestions}, errors.WithStack(err)
	}
	messages := []*schema.Message{
		schema.SystemMessage(params.Text("eval.reverseQuestionsSystemPrompt")),
		userMessage,
	}
	content, meta, err := streamChat(ctx, chat, contract.EvalStageReverseQuestions, messages, report)
	if err != nil {
		return nil, meta, errors.WithStack(err)
	}
	parsed, perr := contract.ParseJSONResponse[struct {
		Questions []string `json:"questions"`
	}](content, "dry-run: cannot parse reverse question response")
	if perr != nil {
		return nil, meta, perr
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
func judgeSuggestions(ctx context.Context, params *config.Params, chat port.ChatGenerator, query string, ev *DryRunEvaluation, results []retrieval.RetrievalResult, diag diagnosisContext, report contract.EvalReporter) ([]DiagnosisSuggestion, contract.CallMeta, error) {
	systemMessage, err := suggestionsSystemPrompt(ctx, params, diag.Language)
	if err != nil {
		return nil, contract.CallMeta{Stage: contract.EvalStageSuggestions}, errors.WithStack(err)
	}
	userMessage, err := buildSuggestionsPrompt(ctx, query, ev, results, diag.Options, params.Int("eval.snippetLimit"))
	if err != nil {
		return nil, contract.CallMeta{Stage: contract.EvalStageSuggestions}, errors.WithStack(err)
	}
	messages := []*schema.Message{
		systemMessage,
		userMessage,
	}
	content, meta, err := streamChat(ctx, chat, contract.EvalStageSuggestions, messages, report)
	if err != nil {
		return nil, meta, errors.WithStack(err)
	}
	parsed, perr := contract.ParseJSONResponse[struct {
		Suggestions []struct {
			Category string `json:"category"`
			Detail   string `json:"detail"`
		} `json:"suggestions"`
	}](content, "dry-run: cannot parse suggestion response")
	if perr != nil {
		return nil, meta, perr
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

// evaluate 对给定检索结果执行完整评估：生成回答 → 分块相关性 → 声明抽取 → 反推问题 →
// 改进建议。编排由 evalGraph 承载（answer/chunkRelevance 与 claims/reverseQuestions 各自并行），
// 每步经 report 上报 start / done / failed 事件并实时透传模型增量输出；单步失败不中断整体流程，
// 而是记录 warning 并将对应指标置 0；chat 为 nil 或 results 为空时返回英文错误（上层转为
// evalError，不产出 evaluation）。report 返回错误（客户端断开）时评估图短路后续阶段并原样返回
// 该错误。返回的 []contract.CallMeta 按固定阶段顺序排列。
func evaluate(ctx context.Context, params *config.Params, chat port.ChatGenerator, embed port.Embedder, query string, results []retrieval.RetrievalResult, diag diagnosisContext, report contract.EvalReporter) (*DryRunEvaluation, []contract.CallMeta, error) {
	if chat == nil {
		return nil, nil, errors.New("dry-run: chat client unavailable, evaluation skipped")
	}
	if len(results) == 0 {
		return nil, nil, errors.New("dry-run: no retrieval results to evaluate")
	}
	out, err := evalGraph.Invoke(ctx, evalInput{
		params:   params,
		chat:     chat,
		embed:    embed,
		query:    query,
		results:  results,
		diag:     diag,
		reporter: report,
	})
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	if out.ReportErr != nil {
		return out.Evaluation, out.Calls, errors.WithStack(out.ReportErr)
	}
	return out.Evaluation, out.Calls, nil
}

// runStage 执行一个评估阶段：先上报 start 事件，再执行 call；emit 返回错误（客户端
// 断开）时不再执行 call，直接将错误上抛。
func runStage[T any](stage contract.EvalStage, emit func(contract.StageEvent) error, call func() (T, contract.CallMeta, error)) (T, contract.CallMeta, error) {
	var zero T
	if err := emit(contract.StageEvent{Stage: stage, Status: contract.EvalStatusStart}); err != nil {
		return zero, contract.CallMeta{Stage: stage}, errors.WithStack(err)
	}
	return call()
}
