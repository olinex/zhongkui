package knowledge

import (
	// 标准库
	"context"
	"sync"
	"testing"
	"time"

	// 第三方
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/aitest"
	"zhongkuisec.com/zhongkui/internal/ai/config"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// countingChat 在 fakeChat 之上按阶段统计调用次数，用于断言「中止后不再发起后续调用」。
type countingChat struct {
	// inner 实际应答的替身
	inner *fakeChat
	// prompts system prompt → 阶段
	prompts map[string]contract.EvalStage
	// mu 保护 counts
	mu sync.Mutex
	// counts 各阶段调用次数
	counts map[contract.EvalStage]int
}

// newCountingChat 构造按阶段计数的对话替身。
func newCountingChat(params *config.Params, inner *fakeChat) *countingChat {
	return &countingChat{inner: inner, prompts: evalStagePrompts(params), counts: map[contract.EvalStage]int{}}
}

// Generate 记录阶段后转发非流式调用。
func (c *countingChat) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	c.note(messages)
	return c.inner.Generate(ctx, messages, opts...)
}

// Stream 记录阶段后转发流式调用。
func (c *countingChat) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	c.note(messages)
	return c.inner.Stream(ctx, messages, opts...)
}

// note 记录一次阶段调用。
func (c *countingChat) note(messages []*schema.Message) {
	stage, err := stageOf(c.prompts, messages)
	if err != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts[stage]++
}

// count 返回指定阶段的调用次数。
func (c *countingChat) count(stage contract.EvalStage) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts[stage]
}

// barrierChat 在 answer 与 chunkRelevance 阶段阻塞，直到测试放行；用于证明两者并行执行。
type barrierChat struct {
	// prompts system prompt → 阶段
	prompts map[string]contract.EvalStage
	// responses 各阶段响应正文
	responses map[contract.EvalStage]string
	// started 收到并行阶段启动通知
	started chan contract.EvalStage
	// release 放行并行阶段
	release chan struct{}
}

// newBarrierChat 构造并行屏障替身。
func newBarrierChat(params *config.Params, responses map[contract.EvalStage]string) *barrierChat {
	return &barrierChat{
		prompts:   evalStagePrompts(params),
		responses: responses,
		started:   make(chan contract.EvalStage, 2),
		release:   make(chan struct{}),
	}
}

// Generate 未使用；评估链路只走 Stream。
func (c *barrierChat) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return nil, errors.New("barrier chat: Generate not used")
}

// Stream 在并行阶段等待另一路到达后统一放行。
func (c *barrierChat) Stream(ctx context.Context, messages []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	stage, err := stageOf(c.prompts, messages)
	if err != nil {
		return nil, err
	}
	if stage == contract.EvalStageAnswer || stage == contract.EvalStageChunkRelevance {
		select {
		case c.started <- stage:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		select {
		case <-c.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return stageChunks(c.responses[stage]), nil
}

// evalGraphResponses 是成功路径的五段响应。
func evalGraphResponses() map[contract.EvalStage]string {
	return map[contract.EvalStage]string{
		contract.EvalStageAnswer:           `{"answer": "The sky is blue."}`,
		contract.EvalStageChunkRelevance:   `{"judgments": [{"index": 0, "relevant": true, "score": 0.9, "reason": "ok"}]}`,
		contract.EvalStageClaims:           `{"claims": [{"statement": "s", "supported": true, "chunkIndexes": [0]}]}`,
		contract.EvalStageReverseQuestions: `{"questions": ["What color is the sky?"]}`,
		contract.EvalStageSuggestions:      `{"suggestions": [{"category": "coverage", "detail": "Add more sky documents."}]}`,
	}
}

// TestEvalGraph_CallMetaOrder 校验调用明细按固定阶段顺序输出，与并行完成顺序无关。
func TestEvalGraph_CallMetaOrder(t *testing.T) {
	params := aitest.Params(t, nil)
	chat := &fakeChat{stream: stageStream(params, evalGraphResponses())}
	results := []retrieval.RetrievalResult{{DocumentID: "1", Title: "A", Content: "The sky is blue."}}
	if _, calls, err := evaluate(context.Background(), params, chat, uniformEmbed(), "q", results, diagnosisContext{}, nil); err != nil {
		t.Fatalf("evaluate() error = %v", err)
	} else {
		want := []contract.EvalStage{
			contract.EvalStageAnswer,
			contract.EvalStageChunkRelevance,
			contract.EvalStageClaims,
			contract.EvalStageReverseQuestions,
			contract.EvalStageSuggestions,
		}
		if len(calls) != len(want) {
			t.Fatalf("calls length = %d, want %d", len(calls), len(want))
		}
		for idx := range want {
			if calls[idx].Stage != want[idx] {
				t.Errorf("calls[%d].Stage = %q, want %q", idx, calls[idx].Stage, want[idx])
			}
		}
	}
}

// TestEvalGraph_SkipsDependentStagesWithoutAnswer 校验回答失败时不发起 claims / reverseQuestions，
// 但仍执行 suggestions。
func TestEvalGraph_SkipsDependentStagesWithoutAnswer(t *testing.T) {
	params := aitest.Params(t, nil)
	responses := map[contract.EvalStage]string{
		contract.EvalStageAnswer:         "not json",
		contract.EvalStageChunkRelevance: `{"judgments": []}`,
		contract.EvalStageSuggestions:    `{"suggestions": []}`,
	}
	chat := newCountingChat(params, &fakeChat{gen: stageGen(params, responses)})
	results := []retrieval.RetrievalResult{{DocumentID: "1", Title: "A", Content: "The sky is blue."}}
	events := make([]contract.StageEvent, 0, 8)
	report := func(ev contract.StageEvent) error {
		events = append(events, ev)
		return nil
	}
	ev, calls, err := evaluate(context.Background(), params, chat, uniformEmbed(), "q", results, diagnosisContext{}, report)
	if err != nil {
		t.Fatalf("evaluate() error = %v", err)
	}
	if chat.count(contract.EvalStageClaims) != 0 || chat.count(contract.EvalStageReverseQuestions) != 0 {
		t.Errorf("claims/reverse calls = %d/%d, want 0/0", chat.count(contract.EvalStageClaims), chat.count(contract.EvalStageReverseQuestions))
	}
	if chat.count(contract.EvalStageSuggestions) != 1 {
		t.Errorf("suggestions calls = %d, want 1", chat.count(contract.EvalStageSuggestions))
	}
	for _, e := range events {
		if e.Stage == contract.EvalStageClaims || e.Stage == contract.EvalStageReverseQuestions {
			t.Errorf("unexpected stage event for skipped stage: %+v", e)
		}
	}
	if len(ev.Warnings) != 1 {
		t.Errorf("Warnings = %v, want the answer failure warning", ev.Warnings)
	}
	if ev.ChunkJudgments == nil || ev.Claims == nil || ev.ReverseQuestions == nil || ev.Suggestions == nil {
		t.Errorf("evaluation slices = %+v, want non-nil empty slices", ev)
	}
	for _, call := range calls {
		if call.Stage == contract.EvalStageClaims || call.Stage == contract.EvalStageReverseQuestions {
			t.Errorf("unexpected call meta for skipped stage: %+v", call)
		}
	}
}

// TestEvalGraph_ReportErrorAborts 校验客户端断开时中止后续阶段：claims / reverseQuestions /
// suggestions 均不再发起调用，且返回已产生的部分结果与原始错误。
func TestEvalGraph_ReportErrorAborts(t *testing.T) {
	params := aitest.Params(t, nil)
	chat := newCountingChat(params, &fakeChat{stream: stageStream(params, evalGraphResponses())})
	results := []retrieval.RetrievalResult{{DocumentID: "1", Title: "A", Content: "The sky is blue."}}
	boom := errors.New("client gone")
	report := func(ev contract.StageEvent) error {
		if ev.Stage == contract.EvalStageChunkRelevance && ev.Status == contract.EvalStatusStart {
			return boom
		}
		return nil
	}
	ev, _, err := evaluate(context.Background(), params, chat, uniformEmbed(), "q", results, diagnosisContext{}, report)
	if !errors.Is(err, boom) {
		t.Fatalf("evaluate() error = %v, want the report error", err)
	}
	if ev == nil {
		t.Fatal("evaluate() returned nil partial evaluation on abort")
	}
	for _, stage := range []contract.EvalStage{contract.EvalStageClaims, contract.EvalStageReverseQuestions, contract.EvalStageSuggestions} {
		if got := chat.count(stage); got != 0 {
			t.Errorf("%s calls = %d, want 0 after abort", stage, got)
		}
	}
}

// TestEvalGraph_ParallelStagesOverlap 通过屏障证明 answer 与 chunkRelevance 并发执行：
// 串行实现只能到达其中一个阶段，测试会超时失败。
func TestEvalGraph_ParallelStagesOverlap(t *testing.T) {
	params := aitest.Params(t, nil)
	chat := newBarrierChat(params, evalGraphResponses())
	results := []retrieval.RetrievalResult{{DocumentID: "1", Title: "A", Content: "The sky is blue."}}
	done := make(chan error, 1)
	go func() {
		_, _, err := evaluate(context.Background(), params, chat, uniformEmbed(), "q", results, diagnosisContext{}, nil)
		done <- err
	}()
	released := false
	defer func() {
		if !released {
			close(chat.release)
		}
	}()
	seen := map[contract.EvalStage]bool{}
	for len(seen) < 2 {
		select {
		case stage := <-chat.started:
			seen[stage] = true
		case <-time.After(3 * time.Second):
			t.Fatalf("parallel stages did not start concurrently, seen = %v", seen)
		}
	}
	close(chat.release)
	released = true
	if err := <-done; err != nil {
		t.Fatalf("evaluate() error = %v", err)
	}
	if !seen[contract.EvalStageAnswer] || !seen[contract.EvalStageChunkRelevance] {
		t.Errorf("seen = %v, want both parallel stages", seen)
	}
}
