package knowledge

import (
	// 标准库
	"context"
	"sync"

	// 第三方
	"github.com/cloudwego/eino/compose"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/config"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/port"
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// 本文件把检索试运行的评估阶段编排为 compose.Graph：answer 与 chunkRelevance 并行、
// claims 与 reverseQuestions 并行，汇合节点按固定阶段顺序合并产物。共享输入与最终结果
// 放在 compose.ProcessState 的 local state 中，边上只传阶段产物，避免并发写同一结构。
// 阶段失败仍保持「记 warning、置零指标、继续后续阶段」的软失败语义；仅客户端断开
// （report 返回错误）会锁存中止标志并短路后续阶段。

const (
	// evalNodePrepare 图入口：把输入写入本地状态并初始化评估结果。
	evalNodePrepare = "eval_prepare"
	// evalNodeAnswer 基于检索上下文生成回答。
	evalNodeAnswer = "eval_answer"
	// evalNodeChunkRelevance 逐块判定相关性。
	evalNodeChunkRelevance = "eval_chunk_relevance"
	// evalNodeJoinStage1 汇合 answer 与 chunkRelevance，计算上下文精确率。
	evalNodeJoinStage1 = "eval_join_stage1"
	// evalNodeClaims 抽取答案声明并判定支撑关系。
	evalNodeClaims = "eval_claims"
	// evalNodeReverseQuestions 反推回答可能对应的问题并计算答案相关度。
	evalNodeReverseQuestions = "eval_reverse_questions"
	// evalNodeJoinStage2 汇合 claims 与 reverseQuestions，计算忠实度与上下文利用率。
	evalNodeJoinStage2 = "eval_join_stage2"
	// evalNodeSuggestions 生成改进建议。
	evalNodeSuggestions = "eval_suggestions"
	// evalNodeFinalize 从本地状态取出最终评估结果。
	evalNodeFinalize = "eval_finalize"
)

// evalInput 是评估图的输入：本次 dry-run 评估的全部依赖，由 evaluate 组装。
type evalInput struct {
	// params 行为参数解析器（提示词、片段截断长度）
	params *config.Params
	// chat 评估使用的对话客户端（flash 档位）
	chat port.ChatGenerator
	// embed 向量化客户端（答案相关度）
	embed port.Embedder
	// query 检索查询
	query string
	// results 检索命中的分块
	results []retrieval.RetrievalResult
	// diag 改进建议阶段需要的生效检索参数与语言标签
	diag diagnosisContext
	// reporter 阶段事件上报回调（可空）
	reporter contract.EvalReporter
}

// evalResult 是评估图的输出：评估结果、调用明细与中止原因。
type evalResult struct {
	// Evaluation 评估结果（恒非空，失败阶段对应字段保持空）
	Evaluation *DryRunEvaluation
	// Calls 各阶段调用明细，按固定阶段顺序排列
	Calls []contract.CallMeta
	// ReportErr 首个阶段事件上报错误（客户端断开）；非 nil 表示评估被中止
	ReportErr error
}

// evalGate 是控制边上的空载荷：只表达阶段先后，不携带业务数据。
type evalGate struct {
	// Aborted 为 true 表示上游已因客户端断开中止，后续节点应直接跳过
	Aborted bool
}

// answerOutcome 是回答阶段的产物：回答正文、调用明细与降级说明。
type answerOutcome struct {
	// Answer 模型生成的回答正文
	Answer string
	// Meta 本阶段调用明细（失败时同样保留耗时）
	Meta contract.CallMeta
	// Warnings 本阶段的降级说明（失败原因）
	Warnings []string
	// Ran 是否真正执行了阶段（跳过时为 false）
	Ran bool
}

// chunkOutcome 是相关性判定阶段的产物：逐块判定、汇总与指标。
type chunkOutcome struct {
	// Judgments 逐块相关性判定
	Judgments []ChunkJudgment
	// RelevantCount 判定为相关的分块数
	RelevantCount int
	// ContextPrecision 上下文精确率
	ContextPrecision float64
	// Meta 本阶段调用明细
	Meta contract.CallMeta
	// Warnings 本阶段的降级说明
	Warnings []string
	// Ran 是否真正执行了阶段
	Ran bool
}

// claimsOutcome 是声明判定阶段的产物。
type claimsOutcome struct {
	// Claims 答案声明与支撑判定
	Claims []FaithfulnessClaim
	// Meta 本阶段调用明细
	Meta contract.CallMeta
	// Warnings 本阶段的降级说明
	Warnings []string
	// Ran 是否真正执行了阶段（无回答时跳过）
	Ran bool
}

// reverseOutcome 是反推问题阶段的产物。
type reverseOutcome struct {
	// Questions 反推的问题
	Questions []string
	// AnswerRelevance 答案相关度（嵌入失败时保持 0）
	AnswerRelevance float64
	// Meta 本阶段调用明细
	Meta contract.CallMeta
	// Warnings 本阶段的降级说明
	Warnings []string
	// Ran 是否真正执行了阶段（无回答时跳过）
	Ran bool
}

// suggestionsOutcome 是改进建议阶段的产物。
type suggestionsOutcome struct {
	// Suggestions 改进建议
	Suggestions []DiagnosisSuggestion
	// Meta 本阶段调用明细
	Meta contract.CallMeta
	// Warnings 本阶段的降级说明
	Warnings []string
	// Ran 是否真正执行了阶段
	Ran bool
}

// evalRunState 是评估图的本地状态：输入由 prepare 写入，结果字段只在汇合节点按序写入；
// 并发分支共享的只有事件上报，故仅 reportErr 需要互斥保护。
type evalRunState struct {
	// input 评估依赖（prepare 写入后只读）
	input evalInput
	// modelName 评估模型名（prepare 写入后只读）
	modelName string

	// mu 保护 reportErr（并行分支会并发上报事件）
	mu sync.Mutex
	// reportErr 首个事件上报错误（客户端断开）
	reportErr error

	// evaluation 评估结果（只在串行汇合节点写入）
	evaluation *DryRunEvaluation
	// calls 调用明细（只在串行汇合节点按固定顺序追加）
	calls []contract.CallMeta
	// warnings 降级说明（只在串行汇合节点按固定顺序追加）
	warnings []string
}

// evalGraph 是编译后的评估图：拓扑静态、本地状态按次运行隔离，可并发调用。
var evalGraph = mustEvalGraph()

// mustEvalGraph 构造并编译评估图；拓扑错误属于编程缺陷，直接 panic（与注册表 fail-fast 一致）。
func mustEvalGraph() compose.Runnable[evalInput, evalResult] {
	g, err := newEvalGraph()
	if err != nil {
		panic("knowledge: build dry-run evaluation graph: " + err.Error())
	}
	runnable, err := g.Compile(context.Background(), compose.WithNodeTriggerMode(compose.AllPredecessor))
	if err != nil {
		panic("knowledge: compile dry-run evaluation graph: " + err.Error())
	}
	return runnable
}

// newEvalGraph 声明评估图的节点与边：扇出/扇入使用 OutputKey，使汇合节点按前驱 key 取产物。
func newEvalGraph() (*compose.Graph[evalInput, evalResult], error) {
	g := compose.NewGraph[evalInput, evalResult](
		compose.WithGenLocalState(func(context.Context) *evalRunState {
			return &evalRunState{}
		}),
	)
	if err := g.AddLambdaNode(evalNodePrepare, compose.InvokableLambda(evalPrepare)); err != nil {
		return nil, errors.WithStack(err)
	}
	// 并行组 1 的两个节点必须带 OutputKey：扇入节点以 map[string]any 接收，键为前驱的 OutputKey。
	if err := g.AddLambdaNode(evalNodeAnswer, compose.InvokableLambda(evalAnswer), compose.WithOutputKey(evalNodeAnswer)); err != nil {
		return nil, errors.WithStack(err)
	}
	if err := g.AddLambdaNode(evalNodeChunkRelevance, compose.InvokableLambda(evalChunkRelevance), compose.WithOutputKey(evalNodeChunkRelevance)); err != nil {
		return nil, errors.WithStack(err)
	}
	if err := g.AddLambdaNode(evalNodeJoinStage1, compose.InvokableLambda(evalJoinStage1)); err != nil {
		return nil, errors.WithStack(err)
	}
	// 并行组 2 同理。
	if err := g.AddLambdaNode(evalNodeClaims, compose.InvokableLambda(evalClaims), compose.WithOutputKey(evalNodeClaims)); err != nil {
		return nil, errors.WithStack(err)
	}
	if err := g.AddLambdaNode(evalNodeReverseQuestions, compose.InvokableLambda(evalReverseQuestions), compose.WithOutputKey(evalNodeReverseQuestions)); err != nil {
		return nil, errors.WithStack(err)
	}
	if err := g.AddLambdaNode(evalNodeJoinStage2, compose.InvokableLambda(evalJoinStage2)); err != nil {
		return nil, errors.WithStack(err)
	}
	if err := g.AddLambdaNode(evalNodeSuggestions, compose.InvokableLambda(evalSuggestions)); err != nil {
		return nil, errors.WithStack(err)
	}
	if err := g.AddLambdaNode(evalNodeFinalize, compose.InvokableLambda(evalFinalize)); err != nil {
		return nil, errors.WithStack(err)
	}

	edges := [][2]string{
		{compose.START, evalNodePrepare},
		{evalNodePrepare, evalNodeAnswer},
		{evalNodePrepare, evalNodeChunkRelevance},
		{evalNodeAnswer, evalNodeJoinStage1},
		{evalNodeChunkRelevance, evalNodeJoinStage1},
		{evalNodeJoinStage1, evalNodeClaims},
		{evalNodeJoinStage1, evalNodeReverseQuestions},
		{evalNodeClaims, evalNodeJoinStage2},
		{evalNodeReverseQuestions, evalNodeJoinStage2},
		{evalNodeJoinStage2, evalNodeSuggestions},
		{evalNodeSuggestions, evalNodeFinalize},
		{evalNodeFinalize, compose.END},
	}
	for _, edge := range edges {
		if err := g.AddEdge(edge[0], edge[1]); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	return g, nil
}

// evalPrepare 把图输入写入本地状态并初始化评估结果；后续节点只读取 input 与 evaluation 的
// 已写入字段。
func evalPrepare(ctx context.Context, in evalInput) (evalGate, error) {
	if err := withEvalState(ctx, func(st *evalRunState) error {
		st.input = in
		st.modelName = chatModelName(in.chat)
		st.evaluation = &DryRunEvaluation{
			// 空 slice 而非 nil，保证 JSON 序列化为 [] 而不是 null
			ChunkJudgments:   []ChunkJudgment{},
			Claims:           []FaithfulnessClaim{},
			ReverseQuestions: []string{},
			Suggestions:      []DiagnosisSuggestion{},
			Warnings:         []string{},
			TotalChunks:      len(in.results),
		}
		return nil
	}); err != nil {
		return evalGate{}, errors.WithStack(err)
	}
	return evalGate{}, nil
}

// evalAnswer 执行回答生成阶段：软失败记 warning 并继续；上报失败则标记中止。
func evalAnswer(ctx context.Context, _ evalGate) (answerOutcome, error) {
	st, err := currentEvalState(ctx)
	if err != nil {
		return answerOutcome{}, errors.WithStack(err)
	}
	if st.aborted() {
		return answerOutcome{}, nil
	}
	answer, meta, err := runStage(contract.EvalStageAnswer, st.report, func() (string, contract.CallMeta, error) {
		return generateAnswer(ctx, st.input.params, st.input.chat, st.input.query, st.input.results, st.report)
	})
	out := answerOutcome{Answer: answer, Meta: meta, Ran: true}
	if st.aborted() {
		return out, nil
	}
	if err != nil {
		out.Warnings = append(out.Warnings, "dry-run: answer generation failed: "+err.Error())
		_ = st.report(contract.StageEvent{Stage: contract.EvalStageAnswer, Status: contract.EvalStatusFailed, ElapsedMs: meta.ElapsedMs, Err: err.Error()})
		return out, nil
	}
	_ = st.report(contract.StageEvent{Stage: contract.EvalStageAnswer, Status: contract.EvalStatusDone, ElapsedMs: meta.ElapsedMs, Usage: &meta.Usage})
	return out, nil
}

// evalChunkRelevance 执行分块相关性判定阶段，并在本节点完成相关条数与精确率计算。
func evalChunkRelevance(ctx context.Context, _ evalGate) (chunkOutcome, error) {
	st, err := currentEvalState(ctx)
	if err != nil {
		return chunkOutcome{}, errors.WithStack(err)
	}
	if st.aborted() {
		return chunkOutcome{}, nil
	}
	judgments, meta, err := runStage(contract.EvalStageChunkRelevance, st.report, func() ([]ChunkJudgment, contract.CallMeta, error) {
		return judgeChunkRelevance(ctx, st.input.params, st.input.chat, st.input.query, st.input.results, st.report)
	})
	// 失败时保持空 slice 而非 nil，保证 JSON 序列化为 [] 而不是 null。
	out := chunkOutcome{Judgments: []ChunkJudgment{}, Meta: meta, Ran: true}
	if err == nil {
		if judgments != nil {
			out.Judgments = judgments
		}
		for _, judgment := range out.Judgments {
			if judgment.Relevant {
				out.RelevantCount++
			}
		}
		out.ContextPrecision = contextPrecision(out.Judgments)
	}
	if st.aborted() {
		return out, nil
	}
	if err != nil {
		out.Warnings = append(out.Warnings, "dry-run: chunk relevance judgment failed: "+err.Error())
		_ = st.report(contract.StageEvent{Stage: contract.EvalStageChunkRelevance, Status: contract.EvalStatusFailed, ElapsedMs: meta.ElapsedMs, Err: err.Error()})
		return out, nil
	}
	_ = st.report(contract.StageEvent{Stage: contract.EvalStageChunkRelevance, Status: contract.EvalStatusDone, ElapsedMs: meta.ElapsedMs, Usage: &meta.Usage})
	return out, nil
}

// evalJoinStage1 汇合并行组 1：按固定顺序（answer → chunkRelevance）写入结果与调用明细。
func evalJoinStage1(ctx context.Context, in map[string]any) (evalGate, error) {
	answer, ok := in[evalNodeAnswer].(answerOutcome)
	if !ok {
		return evalGate{}, errors.Newf("knowledge: eval graph missing answer outcome: %T", in[evalNodeAnswer])
	}
	chunk, ok := in[evalNodeChunkRelevance].(chunkOutcome)
	if !ok {
		return evalGate{}, errors.Newf("knowledge: eval graph missing chunk outcome: %T", in[evalNodeChunkRelevance])
	}
	if err := withEvalState(ctx, func(st *evalRunState) error {
		if answer.Ran {
			st.evaluation.Answer = answer.Answer
		}
		if chunk.Ran {
			st.evaluation.ChunkJudgments = chunk.Judgments
			st.evaluation.RelevantCount = chunk.RelevantCount
			st.evaluation.Metrics.ContextPrecision = chunk.ContextPrecision
		}
		st.appendStage(answer.Meta, answer.Ran, answer.Warnings)
		st.appendStage(chunk.Meta, chunk.Ran, chunk.Warnings)
		return nil
	}); err != nil {
		return evalGate{}, errors.WithStack(err)
	}
	return gateOf(ctx)
}

// evalClaims 执行声明判定阶段；无回答或已中止时直接跳过（不产生事件与调用明细）。
func evalClaims(ctx context.Context, gate evalGate) (claimsOutcome, error) {
	st, err := currentEvalState(ctx)
	if err != nil {
		return claimsOutcome{}, errors.WithStack(err)
	}
	if gate.Aborted || st.aborted() || st.evaluation.Answer == "" {
		return claimsOutcome{}, nil
	}
	claims, meta, err := runStage(contract.EvalStageClaims, st.report, func() ([]FaithfulnessClaim, contract.CallMeta, error) {
		return judgeClaims(ctx, st.input.params, st.input.chat, st.evaluation.Answer, st.input.results, st.report)
	})
	out := claimsOutcome{Claims: []FaithfulnessClaim{}, Meta: meta, Ran: true}
	if err == nil && claims != nil {
		out.Claims = claims
	}
	if st.aborted() {
		return out, nil
	}
	if err != nil {
		out.Warnings = append(out.Warnings, "dry-run: claim faithfulness judgment failed: "+err.Error())
		_ = st.report(contract.StageEvent{Stage: contract.EvalStageClaims, Status: contract.EvalStatusFailed, ElapsedMs: meta.ElapsedMs, Err: err.Error()})
		return out, nil
	}
	_ = st.report(contract.StageEvent{Stage: contract.EvalStageClaims, Status: contract.EvalStatusDone, ElapsedMs: meta.ElapsedMs, Usage: &meta.Usage})
	return out, nil
}

// evalReverseQuestions 执行反推问题阶段，并在本节点完成答案相关度嵌入计算。
func evalReverseQuestions(ctx context.Context, gate evalGate) (reverseOutcome, error) {
	st, err := currentEvalState(ctx)
	if err != nil {
		return reverseOutcome{}, errors.WithStack(err)
	}
	if gate.Aborted || st.aborted() || st.evaluation.Answer == "" {
		return reverseOutcome{}, nil
	}
	questions, meta, err := runStage(contract.EvalStageReverseQuestions, st.report, func() ([]string, contract.CallMeta, error) {
		return judgeReverseQuestions(ctx, st.input.params, st.input.chat, st.evaluation.Answer, st.report)
	})
	out := reverseOutcome{Questions: []string{}, Meta: meta, Ran: true}
	if err == nil && questions != nil {
		out.Questions = questions
	}
	if st.aborted() {
		return out, nil
	}
	if err != nil {
		out.Warnings = append(out.Warnings, "dry-run: reverse question generation failed: "+err.Error())
		_ = st.report(contract.StageEvent{Stage: contract.EvalStageReverseQuestions, Status: contract.EvalStatusFailed, ElapsedMs: meta.ElapsedMs, Err: err.Error()})
		return out, nil
	}
	if relevance, relevanceErr := answerRelevance(ctx, st.input.embed, st.input.query, questions); relevanceErr != nil {
		out.Warnings = append(out.Warnings, "dry-run: answer relevance embedding failed: "+relevanceErr.Error())
	} else {
		out.AnswerRelevance = relevance
	}
	_ = st.report(contract.StageEvent{Stage: contract.EvalStageReverseQuestions, Status: contract.EvalStatusDone, ElapsedMs: meta.ElapsedMs, Usage: &meta.Usage})
	return out, nil
}

// evalJoinStage2 汇合并行组 2：按固定顺序（claims → reverseQuestions）写入结果与调用明细，
// 并计算依赖两者与分块判定的忠实度、上下文利用率与答案相关度。
func evalJoinStage2(ctx context.Context, in map[string]any) (evalGate, error) {
	claims, ok := in[evalNodeClaims].(claimsOutcome)
	if !ok {
		return evalGate{}, errors.Newf("knowledge: eval graph missing claims outcome: %T", in[evalNodeClaims])
	}
	reverse, ok := in[evalNodeReverseQuestions].(reverseOutcome)
	if !ok {
		return evalGate{}, errors.Newf("knowledge: eval graph missing reverse outcome: %T", in[evalNodeReverseQuestions])
	}
	if err := withEvalState(ctx, func(st *evalRunState) error {
		if claims.Ran {
			st.evaluation.Claims = claims.Claims
			st.evaluation.Metrics.Faithfulness = faithfulness(claims.Claims)
			st.evaluation.Metrics.ContextUtilization = contextUtilization(st.evaluation.ChunkJudgments, claims.Claims)
		}
		if reverse.Ran {
			st.evaluation.ReverseQuestions = reverse.Questions
			st.evaluation.Metrics.AnswerRelevance = reverse.AnswerRelevance
		}
		st.appendStage(claims.Meta, claims.Ran, claims.Warnings)
		st.appendStage(reverse.Meta, reverse.Ran, reverse.Warnings)
		return nil
	}); err != nil {
		return evalGate{}, errors.WithStack(err)
	}
	return gateOf(ctx)
}

// evalSuggestions 执行改进建议阶段；回答失败本身是诊断线索，故不依赖 Answer 是否为空。
func evalSuggestions(ctx context.Context, gate evalGate) (suggestionsOutcome, error) {
	st, err := currentEvalState(ctx)
	if err != nil {
		return suggestionsOutcome{}, errors.WithStack(err)
	}
	if gate.Aborted || st.aborted() {
		return suggestionsOutcome{}, nil
	}
	suggestions, meta, err := runStage(contract.EvalStageSuggestions, st.report, func() ([]DiagnosisSuggestion, contract.CallMeta, error) {
		return judgeSuggestions(ctx, st.input.params, st.input.chat, st.input.query, st.evaluation, st.input.results, st.input.diag, st.report)
	})
	out := suggestionsOutcome{Suggestions: []DiagnosisSuggestion{}, Meta: meta, Ran: true}
	if err == nil && suggestions != nil {
		out.Suggestions = suggestions
	}
	if st.aborted() {
		return out, nil
	}
	if err != nil {
		out.Warnings = append(out.Warnings, "dry-run: suggestion generation failed: "+err.Error())
		_ = st.report(contract.StageEvent{Stage: contract.EvalStageSuggestions, Status: contract.EvalStatusFailed, ElapsedMs: meta.ElapsedMs, Err: err.Error()})
		return out, nil
	}
	_ = st.report(contract.StageEvent{Stage: contract.EvalStageSuggestions, Status: contract.EvalStatusDone, ElapsedMs: meta.ElapsedMs, Usage: &meta.Usage})
	return out, nil
}

// evalFinalize 汇合建议阶段并从本地状态产出最终结果；调用明细与 warning 已按固定阶段顺序排列。
func evalFinalize(ctx context.Context, in suggestionsOutcome) (evalResult, error) {
	var out evalResult
	if err := withEvalState(ctx, func(st *evalRunState) error {
		if in.Ran {
			st.evaluation.Suggestions = in.Suggestions
		}
		st.appendStage(in.Meta, in.Ran, in.Warnings)
		st.evaluation.Warnings = st.warnings
		out = evalResult{Evaluation: st.evaluation, Calls: st.calls, ReportErr: st.reportErr}
		return nil
	}); err != nil {
		return evalResult{}, errors.WithStack(err)
	}
	return out, nil
}

// withEvalState 读取图本地状态并执行 fn；ProcessState 失败属于框架/编程错误，原样上抛。
func withEvalState(ctx context.Context, fn func(*evalRunState) error) error {
	return compose.ProcessState[*evalRunState](ctx, func(_ context.Context, st *evalRunState) error {
		return fn(st)
	})
}

// currentEvalState 取回本次运行的本地状态。
func currentEvalState(ctx context.Context) (*evalRunState, error) {
	var st *evalRunState
	if err := withEvalState(ctx, func(s *evalRunState) error {
		st = s
		return nil
	}); err != nil {
		return nil, errors.WithStack(err)
	}
	return st, nil
}

// gateOf 读取中止标志并构造控制边载荷。
func gateOf(ctx context.Context) (evalGate, error) {
	st, err := currentEvalState(ctx)
	if err != nil {
		return evalGate{}, errors.WithStack(err)
	}
	return evalGate{Aborted: st.aborted()}, nil
}

// report 上报一条阶段事件：并发分支经同一把锁串行化写入，首个错误锁存后后续调用立即失败。
func (s *evalRunState) report(ev contract.StageEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reportErr != nil {
		return s.reportErr
	}
	if s.input.reporter == nil {
		return nil
	}
	ev.Model = s.modelName
	s.reportErr = s.input.reporter(ev)
	return s.reportErr
}

// aborted 报告本次评估是否已因客户端断开中止。
func (s *evalRunState) aborted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reportErr != nil
}

// appendStage 在串行汇合点按固定顺序追加一条调用明细与降级说明；未执行的阶段不追加明细。
func (s *evalRunState) appendStage(meta contract.CallMeta, ran bool, warnings []string) {
	if ran && meta.Stage != "" {
		s.calls = append(s.calls, meta)
	}
	s.warnings = append(s.warnings, warnings...)
}
