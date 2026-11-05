package knowledge

import (
	// 标准库
	"context"
	"strings"
	"time"

	// 第三方
	"github.com/cloudwego/eino/components/retriever"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/port"
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// chatModelName 尽力取回 chat 客户端的模型名（客户端不支持时返回空串），用于
// dry-run 结果的调用明细。
func chatModelName(chat port.ChatGenerator) string {
	if namer, ok := chat.(port.ChatModelNamer); ok {
		return namer.ModelName()
	}
	return ""
}

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
	Calls []contract.CallMeta `json:"calls"`
	// TotalElapsedMs 整个 dry-run 的总耗时（毫秒）
	TotalElapsedMs int64 `json:"totalElapsedMs"`
	// Usage 全部调用的 token 用量汇总
	Usage contract.CallUsage `json:"usage"`
}

// DryRunResult 是一次 dry-run 的完整输出（view 层直接序列化）。
type DryRunResult struct {
	// Query 检索查询文本
	Query string `json:"query"`
	// Results 检索命中的分块列表
	Results []retrieval.RetrievalResult `json:"results"`
	// Evaluation 量化评估结果（chat 不可用或结果为空时为 nil）
	Evaluation *DryRunEvaluation `json:"evaluation,omitempty"`
	// EvalError 评估被跳过的原因（英文），评估成功时为空
	EvalError string `json:"evalError,omitempty"`
	// Meta 调用元信息汇总（检索耗时、各次调用明细、总计与 token 汇总）
	Meta DryRunMeta `json:"meta"`
}

// DryRun 执行知识库 RAG dry-run：混合检索（参数可覆盖）→ 基于上下文生成回答 →
// RAGAS 风格量化评估 → 改进建议。全流程经 report 实时上报阶段状态与模型增量输出（为 nil 时
// 不上报）；report 返回错误（SSE 写失败/客户端断开）时立即中止并向上返回该错误。
// languageTag 为请求语言标签（BCP-47），用于要求模型以该语言输出改进建议正文，为空时不
// 下发语言指令。检索失败或 flash 档位未绑定（重排与评估的声明档位）时返回错误；评估客户端
// 缺失或检索结果为空时返回带检索结果与 evalError 的结果（evaluation 为空）。结果中的 Meta
// 汇总检索耗时、各次调用明细、总计耗时与 token。
func (r *Retriever) DryRun(ctx context.Context, query string, report contract.EvalReporter, languageTag string, opts ...retriever.Option) (*DryRunResult, error) {
	totalStart := time.Now()
	// reportErr 记录上报写失败（客户端断开），与「评估前置条件不满足」的降级错误区分开
	var reportErr error
	emit := func(ev contract.StageEvent) error {
		if report == nil || reportErr != nil {
			return errors.WithStack(reportErr)
		}
		reportErr = report(ev)
		return errors.WithStack(reportErr)
	}

	if err := emit(contract.StageEvent{Stage: contract.EvalStageRetrieve, Status: contract.EvalStatusStart}); err != nil {
		return nil, errors.WithStack(err)
	}
	retrieveStart := time.Now()
	docs, err := r.Retrieve(ctx, query, opts...)
	retrieveElapsed := time.Since(retrieveStart).Milliseconds()
	if err != nil {
		_ = emit(contract.StageEvent{Stage: contract.EvalStageRetrieve, Status: contract.EvalStatusFailed, ElapsedMs: retrieveElapsed, Err: err.Error()})
		return nil, errors.WithStack(err)
	}
	results := retrieval.ToRetrievalResults(docs)
	if err := emit(contract.StageEvent{Stage: contract.EvalStageRetrieve, Status: contract.EvalStatusDone, ElapsedMs: retrieveElapsed}); err != nil {
		return nil, errors.WithStack(err)
	}

	out := &DryRunResult{
		Query:   query,
		Results: results,
		Meta: DryRunMeta{
			RetrieveElapsedMs: retrieveElapsed,
			// 空 slice 而非 nil，保证 JSON 序列化为 [] 而不是 null
			Calls: []contract.CallMeta{},
		},
	}
	// 评估 judge 同样声明使用 flash 档位；档位未绑定时返回英文错误（不切换到其他档位），
	// 由调用方以 SSE error 事件明确告知用户。
	chat, err := r.resolveRerankChat()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// 改进建议需引用本次实际生效的检索参数（与 Retrieve 走上完全相同的默认值与归一化），
	// 并需要请求语言标签
	diag := diagnosisContext{Options: *r.retrieveOptions(opts...), Language: languageTag}
	ev, calls, evalErr := evaluate(ctx, r.params, chat, r.embed, query, results, diag, emit)
	if reportErr != nil {
		return nil, errors.WithStack(reportErr)
	}
	if evalErr != nil {
		out.EvalError = evalErr.Error()
	} else {
		out.Evaluation = ev
	}
	if calls != nil {
		out.Meta.Calls = calls
	}
	modelName := chatModelName(chat)
	for i := range out.Meta.Calls {
		out.Meta.Calls[i].Model = modelName
		out.Meta.Usage.PromptTokens += out.Meta.Calls[i].Usage.PromptTokens
		out.Meta.Usage.CompletionTokens += out.Meta.Calls[i].Usage.CompletionTokens
		out.Meta.Usage.TotalTokens += out.Meta.Calls[i].Usage.TotalTokens
	}
	out.Meta.TotalElapsedMs = time.Since(totalStart).Milliseconds()
	return out, nil
}
