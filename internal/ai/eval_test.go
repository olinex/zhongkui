package ai

import (
	// 标准库
	"context"
	"strings"
	"testing"

	// 第三方
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// fakeChat 是测试用 chatGenerator 替身：Generate 与 Stream 分别转调配置的 gen/stream；
// stream 未配置时把 gen 的单次结果拆成两段正文分片并以一个携带用量的空分片收尾，
// 模拟 OpenAI 兼容流式响应的形状。
type fakeChat struct {
	gen    func(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error)
	stream func(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error)
}

func (f *fakeChat) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return f.gen(ctx, messages, opts...)
}

func (f *fakeChat) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if f.stream != nil {
		return f.stream(ctx, messages, opts...)
	}
	msg, err := f.gen(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}
	half := len(msg.Content) / 2
	chunks := []*schema.Message{
		{Role: schema.Assistant, Content: msg.Content[:half]},
		{Role: schema.Assistant, Content: msg.Content[half:]},
		{Role: schema.Assistant, ResponseMeta: &schema.ResponseMeta{
			FinishReason: "stop",
			Usage:        &schema.TokenUsage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30},
		}},
	}
	return schema.StreamReaderFromArray(chunks), nil
}

// fakeEmbed 是测试用 embedder 替身：EmbedStrings 转调配置的 embed 函数。
type fakeEmbed struct {
	embed func(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error)
}

func (f *fakeEmbed) EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	return f.embed(ctx, texts, opts...)
}

// TestContextPrecision 测试上下文精确率：相关位置集合 R 上 precision@k 的均值；
// 无相关分块时返回 0。
func TestContextPrecision(t *testing.T) {
	judgments := []ChunkJudgment{
		{Index: 0, Relevant: true},
		{Index: 1, Relevant: false},
		{Index: 2, Relevant: true},
	}
	// R = {1, 3}：precision@1 = 1/1，precision@3 = 2/3 → CP = (1+2/3)/2 = 5/6
	if got := contextPrecision(judgments); !almostEqual(got, 5.0/6.0) {
		t.Errorf("contextPrecision() = %v, want %v", got, 5.0/6.0)
	}
	if got := contextPrecision(nil); got != 0 {
		t.Errorf("contextPrecision(nil) = %v, want 0", got)
	}
	none := []ChunkJudgment{{Index: 0, Relevant: false}, {Index: 1, Relevant: false}}
	if got := contextPrecision(none); got != 0 {
		t.Errorf("contextPrecision(no relevant) = %v, want 0", got)
	}
}

// TestFaithfulness 测试忠实度：supported 声明数 / 声明总数；无声明时返回 0。
func TestFaithfulness(t *testing.T) {
	claims := []FaithfulnessClaim{
		{Statement: "a", Supported: true},
		{Statement: "b", Supported: true},
		{Statement: "c", Supported: false},
	}
	if got := faithfulness(claims); !almostEqual(got, 2.0/3.0) {
		t.Errorf("faithfulness() = %v, want %v", got, 2.0/3.0)
	}
	if got := faithfulness(nil); got != 0 {
		t.Errorf("faithfulness(nil) = %v, want 0", got)
	}
}

// TestContextUtilization 测试上下文利用率：相关分块与 supported 声明引用分块的
// 交集大小 / 相关分块数；无相关分块时返回 0。
func TestContextUtilization(t *testing.T) {
	judgments := []ChunkJudgment{
		{Index: 0, Relevant: true},
		{Index: 1, Relevant: true},
		{Index: 2, Relevant: false},
	}
	claims := []FaithfulnessClaim{
		{Statement: "s", Supported: true, ChunkIndexes: []int{0, 2}},
	}
	// R = {0, 1}，C = {0, 2} → |R ∩ C| / |R| = 1/2
	if got := contextUtilization(judgments, claims); !almostEqual(got, 0.5) {
		t.Errorf("contextUtilization() = %v, want 0.5", got)
	}
	if got := contextUtilization(nil, claims); got != 0 {
		t.Errorf("contextUtilization(nil) = %v, want 0", got)
	}
}

// TestAnswerRelevance 测试答案相关度：query 与反推问题的嵌入余弦相似度均值；
// questions 为空返回 (0, nil)，嵌入失败返回错误。
func TestAnswerRelevance(t *testing.T) {
	embed := &fakeEmbed{embed: func(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
		vecs := make([][]float64, len(texts))
		for i := range texts {
			switch i {
			case 0, 1:
				vecs[i] = []float64{1, 0, 0}
			default:
				vecs[i] = []float64{0, 1, 0}
			}
		}
		return vecs, nil
	}}
	// q=[1,0,0]，q1=[1,0,0]→cos=1，q2=[0,1,0]→cos=0 → AR = 0.5
	got, err := answerRelevance(context.Background(), embed, "q", []string{"q1", "q2"})
	if err != nil {
		t.Fatalf("answerRelevance() error = %v", err)
	}
	if !almostEqual(got, 0.5) {
		t.Errorf("answerRelevance() = %v, want 0.5", got)
	}
	if got, err := answerRelevance(context.Background(), embed, "q", nil); err != nil || got != 0 {
		t.Errorf("answerRelevance(empty questions) = (%v, %v), want (0, nil)", got, err)
	}
	failEmbed := &fakeEmbed{embed: func(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
		return nil, errors.New("embedding boom")
	}}
	if _, err := answerRelevance(context.Background(), failEmbed, "q", []string{"q1"}); err == nil {
		t.Errorf("answerRelevance(embed error) error = nil, want error")
	}
}

// TestParseJudges 测试分块相关性解析：容忍 ```json 围栏、丢弃 index 越界条目、
// score 钳制到 0..1；纯文本返回英文错误。
func TestParseJudges(t *testing.T) {
	results := []RetrievalResult{{DocumentID: "1"}, {DocumentID: "2"}}
	chat := &fakeChat{gen: func(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
		return &schema.Message{Role: schema.Assistant, Content: "```json\n{\"judgments\": [" +
			"{\"index\": 0, \"relevant\": true, \"score\": 0.9, \"reason\": \"directly answers\"}, " +
			"{\"index\": 1, \"relevant\": false, \"score\": 1.5, \"reason\": \"clamped\"}, " +
			"{\"index\": 5, \"relevant\": true, \"score\": 0.8, \"reason\": \"out of range\"}, " +
			"{\"index\": -1, \"relevant\": true, \"score\": 0.5, \"reason\": \"negative\"}]}\n```"}, nil
	}}
	judgments, _, err := judgeChunkRelevance(context.Background(), testParams(t, nil), chat, "q", results, nil)
	if err != nil {
		t.Fatalf("judgeChunkRelevance() error = %v", err)
	}
	if len(judgments) != 2 {
		t.Fatalf("judgments length = %d, want 2", len(judgments))
	}
	if judgments[0].Index != 0 || !judgments[0].Relevant || !almostEqual(judgments[0].Score, 0.9) {
		t.Errorf("judgments[0] = %+v, want index=0 relevant=true score=0.9", judgments[0])
	}
	if judgments[1].Index != 1 || judgments[1].Relevant || !almostEqual(judgments[1].Score, 1.0) {
		t.Errorf("judgments[1] = %+v, want index=1 relevant=false score=1.0 (clamped)", judgments[1])
	}
	badChat := &fakeChat{gen: func(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
		return &schema.Message{Role: schema.Assistant, Content: "not json"}, nil
	}}
	if _, _, err := judgeChunkRelevance(context.Background(), testParams(t, nil), badChat, "q", results, nil); err == nil || !strings.Contains(err.Error(), "dry-run: cannot parse chunk relevance") {
		t.Errorf("judgeChunkRelevance(plain text) error = %v, want a dry-run English error", err)
	}
}

// TestEvaluate 测试完整评估编排：错误前置、四步成功、单步失败降级为 warning 与指标置 0。
func TestEvaluate(t *testing.T) {
	results := []RetrievalResult{
		{DocumentID: "1", Title: "A", Content: "The sky is blue."},
		{DocumentID: "2", Title: "B", Content: "Unrelated content."},
	}
	okJudgments := `{"judgments": [{"index": 0, "relevant": true, "score": 0.9, "reason": "directly answers"}, {"index": 1, "relevant": false, "score": 0.2, "reason": "off topic"}]}`

	t.Run("nil chat", func(t *testing.T) {
		_, _, err := evaluate(context.Background(), testParams(t, nil), nil, nil, "q", results, diagnosisContext{}, nil)
		if err == nil || !strings.Contains(err.Error(), "chat client unavailable") {
			t.Errorf("evaluate(nil chat) error = %v, want chat-client error", err)
		}
	})

	t.Run("empty results", func(t *testing.T) {
		chat := &fakeChat{}
		_, _, err := evaluate(context.Background(), testParams(t, nil), chat, nil, "q", nil, diagnosisContext{}, nil)
		if err == nil || !strings.Contains(err.Error(), "no retrieval results") {
			t.Errorf("evaluate(empty results) error = %v, want empty-results error", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		responses := []string{
			`{"answer": "The sky is blue."}`,
			okJudgments,
			`{"claims": [{"statement": "The sky is blue.", "supported": true, "chunkIndexes": [0]}]}`,
			`{"questions": ["What color is the sky?"]}`,
			`{"suggestions": [{"category": "coverage", "detail": "Add a document about sky colors."}, {"category": "unknown-kind", "detail": "Fallback category."}]}`,
		}
		i := 0
		chat := &fakeChat{gen: func(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
			resp := responses[i]
			i++
			return &schema.Message{Role: schema.Assistant, Content: resp}, nil
		}}
		embed := &fakeEmbed{embed: func(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
			out := make([][]float64, len(texts))
			for j := range out {
				out[j] = []float64{1, 0, 0}
			}
			return out, nil
		}}
		ev, _, err := evaluate(context.Background(), testParams(t, nil), chat, embed, "What color is the sky?", results, diagnosisContext{}, nil)
		if err != nil {
			t.Fatalf("evaluate() error = %v", err)
		}
		if len(ev.Warnings) != 0 {
			t.Errorf("Warnings = %v, want none", ev.Warnings)
		}
		if ev.Answer != "The sky is blue." {
			t.Errorf("Answer = %q, want %q", ev.Answer, "The sky is blue.")
		}
		if ev.RelevantCount != 1 || ev.TotalChunks != 2 {
			t.Errorf("RelevantCount/TotalChunks = %d/%d, want 1/2", ev.RelevantCount, ev.TotalChunks)
		}
		if !almostEqual(ev.Metrics.ContextPrecision, 1) {
			t.Errorf("ContextPrecision = %v, want 1", ev.Metrics.ContextPrecision)
		}
		if !almostEqual(ev.Metrics.Faithfulness, 1) {
			t.Errorf("Faithfulness = %v, want 1", ev.Metrics.Faithfulness)
		}
		if !almostEqual(ev.Metrics.ContextUtilization, 1) {
			t.Errorf("ContextUtilization = %v, want 1", ev.Metrics.ContextUtilization)
		}
		if !almostEqual(ev.Metrics.AnswerRelevance, 1) {
			t.Errorf("AnswerRelevance = %v, want 1", ev.Metrics.AnswerRelevance)
		}
		if len(ev.ReverseQuestions) != 1 || ev.ReverseQuestions[0] != "What color is the sky?" {
			t.Errorf("ReverseQuestions = %v, want the single reverse question", ev.ReverseQuestions)
		}
		want := []DiagnosisSuggestion{
			{Category: DiagnosisCategoryCoverage, Detail: "Add a document about sky colors."},
			{Category: DiagnosisCategoryOther, Detail: "Fallback category."},
		}
		if len(ev.Suggestions) != len(want) {
			t.Fatalf("Suggestions = %+v, want %+v", ev.Suggestions, want)
		}
		for idx := range want {
			if ev.Suggestions[idx] != want[idx] {
				t.Errorf("Suggestions[%d] = %+v, want %+v", idx, ev.Suggestions[idx], want[idx])
			}
		}
	})

	t.Run("claims failure", func(t *testing.T) {
		responses := []string{
			`{"answer": "The sky is blue."}`,
			okJudgments,
			"not json",
			`{"questions": ["What color is the sky?"]}`,
			`{"suggestions": []}`,
		}
		i := 0
		chat := &fakeChat{gen: func(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
			resp := responses[i]
			i++
			return &schema.Message{Role: schema.Assistant, Content: resp}, nil
		}}
		embed := &fakeEmbed{embed: func(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
			out := make([][]float64, len(texts))
			for j := range out {
				out[j] = []float64{1, 0, 0}
			}
			return out, nil
		}}
		ev, _, err := evaluate(context.Background(), testParams(t, nil), chat, embed, "q", results, diagnosisContext{}, nil)
		if err != nil {
			t.Fatalf("evaluate() error = %v", err)
		}
		if len(ev.Warnings) != 1 || !strings.Contains(ev.Warnings[0], "claim faithfulness judgment failed") {
			t.Errorf("Warnings = %v, want the claim failure warning", ev.Warnings)
		}
		if ev.Metrics.Faithfulness != 0 || ev.Metrics.ContextUtilization != 0 {
			t.Errorf("Faithfulness/ContextUtilization = %v/%v, want 0/0", ev.Metrics.Faithfulness, ev.Metrics.ContextUtilization)
		}
		if !almostEqual(ev.Metrics.ContextPrecision, 1) {
			t.Errorf("ContextPrecision = %v, want 1", ev.Metrics.ContextPrecision)
		}
		if len(ev.Suggestions) != 0 {
			t.Errorf("Suggestions = %+v, want none", ev.Suggestions)
		}
	})

	t.Run("answer failure", func(t *testing.T) {
		responses := []string{
			"not json",
			okJudgments,
			`{"suggestions": [{"category": "generation", "detail": "The answer generator returned malformed JSON."}]}`,
		}
		i := 0
		chat := &fakeChat{gen: func(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
			resp := responses[i]
			i++
			return &schema.Message{Role: schema.Assistant, Content: resp}, nil
		}}
		embed := &fakeEmbed{embed: func(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
			return [][]float64{{1, 0, 0}}, nil
		}}
		ev, _, err := evaluate(context.Background(), testParams(t, nil), chat, embed, "q", results, diagnosisContext{}, nil)
		if err != nil {
			t.Fatalf("evaluate() error = %v", err)
		}
		if len(ev.Warnings) != 1 || !strings.Contains(ev.Warnings[0], "answer generation failed") {
			t.Errorf("Warnings = %v, want the answer failure warning", ev.Warnings)
		}
		if len(ev.ChunkJudgments) != 2 {
			t.Errorf("ChunkJudgments length = %d, want 2", len(ev.ChunkJudgments))
		}
		if ev.Metrics.Faithfulness != 0 || ev.Metrics.AnswerRelevance != 0 || ev.Metrics.ContextUtilization != 0 {
			t.Errorf("Faithfulness/AnswerRelevance/ContextUtilization = %v/%v/%v, want 0/0/0",
				ev.Metrics.Faithfulness, ev.Metrics.AnswerRelevance, ev.Metrics.ContextUtilization)
		}
		// 回答生成失败也要产出建议：失败本身是重要诊断线索
		if len(ev.Suggestions) != 1 || ev.Suggestions[0].Category != DiagnosisCategoryGeneration {
			t.Errorf("Suggestions = %+v, want one generation suggestion", ev.Suggestions)
		}
	})
}

// streamOf 构造一个把响应正文拆成两段分片、末段携带固定用量的假流式调用函数，
// 并按调用次序依次取用 responses（自增下标由调用方持有）。
func streamOf(responses []string, i *int) func(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return func(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
		body := responses[*i]
		*i++
		half := len(body) / 2
		return schema.StreamReaderFromArray([]*schema.Message{
			{Role: schema.Assistant, Content: body[:half]},
			{Role: schema.Assistant, Content: body[half:]},
			{Role: schema.Assistant, ResponseMeta: &schema.ResponseMeta{
				FinishReason: "stop",
				Usage:        &schema.TokenUsage{PromptTokens: 7, CompletionTokens: 9, TotalTokens: 16},
			}},
		}), nil
	}
}

// uniformEmbed 返回与输入文本数等长的同向单位向量，使余弦相似度恒为 1。
func uniformEmbed() *fakeEmbed {
	return &fakeEmbed{embed: func(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
		out := make([][]float64, len(texts))
		for i := range out {
			out[i] = []float64{1, 0, 0}
		}
		return out, nil
	}}
}

// TestEvaluateStreaming 测试流式评估的上报契约：各阶段 start/done 事件按序上报、
// 模型增量按阶段累计后与该阶段的原始响应一致、各次调用的耗时与 token 用量记入 CallMeta。
func TestEvaluateStreaming(t *testing.T) {
	results := []RetrievalResult{{DocumentID: "1", Title: "A", Content: "The sky is blue."}}
	responses := []string{
		`{"answer": "The sky is blue."}`,
		`{"judgments": [{"index": 0, "relevant": true, "score": 0.9, "reason": "ok"}]}`,
		`{"claims": [{"statement": "s", "supported": true, "chunkIndexes": [0]}]}`,
		`{"questions": ["What color is the sky?"]}`,
		`{"suggestions": [{"category": "coverage", "detail": "Add more sky documents."}]}`,
	}
	i := 0
	chat := &fakeChat{stream: streamOf(responses, &i)}
	events := make([]StageEvent, 0, 10)
	report := func(ev StageEvent) error {
		events = append(events, ev)
		return nil
	}
	ev, calls, err := evaluate(context.Background(), testParams(t, nil), chat, uniformEmbed(), "q", results, diagnosisContext{}, report)
	if err != nil {
		t.Fatalf("evaluate() error = %v", err)
	}
	if ev.Answer != "The sky is blue." {
		t.Errorf("Answer = %q, want %q", ev.Answer, "The sky is blue.")
	}
	if len(ev.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none", ev.Warnings)
	}
	stages := []EvalStage{EvalStageAnswer, EvalStageChunkRelevance, EvalStageClaims, EvalStageReverseQuestions, EvalStageSuggestions}
	deltas := make(map[EvalStage]string, len(stages))
	statuses := make(map[EvalStage][]EvalStageStatus, len(stages))
	for _, e := range events {
		if e.Delta != "" {
			deltas[e.Stage] += e.Delta
			continue
		}
		statuses[e.Stage] = append(statuses[e.Stage], e.Status)
	}
	for idx, stage := range stages {
		if deltas[stage] != responses[idx] {
			t.Errorf("deltas[%s] = %q, want %q", stage, deltas[stage], responses[idx])
		}
		if got := statuses[stage]; len(got) != 2 || got[0] != EvalStatusStart || got[1] != EvalStatusDone {
			t.Errorf("statuses[%s] = %v, want [start done]", stage, got)
		}
	}
	if len(calls) != len(stages) {
		t.Fatalf("calls length = %d, want %d", len(calls), len(stages))
	}
	for _, c := range calls {
		if c.Usage.PromptTokens != 7 || c.Usage.CompletionTokens != 9 || c.Usage.TotalTokens != 16 {
			t.Errorf("call %s usage = %+v, want prompt=7 completion=9 total=16", c.Stage, c.Usage)
		}
	}
}

// TestEvaluateStreamFailed 测试流式评估的阶段失败上报：解析失败时上报带错误信息的
// failed 事件，同时仍降级为 warning 并置 0 相关指标。
func TestEvaluateStreamFailed(t *testing.T) {
	results := []RetrievalResult{{DocumentID: "1", Title: "A", Content: "The sky is blue."}}
	responses := []string{
		`{"answer": "The sky is blue."}`,
		`{"judgments": [{"index": 0, "relevant": true, "score": 0.9, "reason": "ok"}]}`,
		"not json",
		`{"questions": ["What color is the sky?"]}`,
		`{"suggestions": []}`,
	}
	i := 0
	chat := &fakeChat{stream: streamOf(responses, &i)}
	events := make([]StageEvent, 0, 10)
	report := func(ev StageEvent) error {
		events = append(events, ev)
		return nil
	}
	ev, _, err := evaluate(context.Background(), testParams(t, nil), chat, uniformEmbed(), "q", results, diagnosisContext{}, report)
	if err != nil {
		t.Fatalf("evaluate() error = %v", err)
	}
	var failed *StageEvent
	for idx := range events {
		if events[idx].Status == EvalStatusFailed {
			failed = &events[idx]
		}
	}
	if failed == nil {
		t.Fatalf("no failed stage event reported, events = %+v", events)
	}
	if failed.Stage != EvalStageClaims || failed.Err == "" || failed.Usage != nil {
		t.Errorf("failed event = %+v, want stage=claims with an error and no usage", *failed)
	}
	if len(ev.Warnings) != 1 || !strings.Contains(ev.Warnings[0], "claim faithfulness judgment failed") {
		t.Errorf("Warnings = %v, want the claim failure warning", ev.Warnings)
	}
	if ev.Metrics.Faithfulness != 0 || ev.Metrics.ContextUtilization != 0 {
		t.Errorf("Faithfulness/ContextUtilization = %v/%v, want 0/0", ev.Metrics.Faithfulness, ev.Metrics.ContextUtilization)
	}
}

// TestStreamChatReportError 测试客户端断开：report 返回错误时立即中止流式读取并
// 原样上抛该错误，不再继续消费后续分片。
func TestStreamChatReportError(t *testing.T) {
	chat := &fakeChat{stream: func(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
		return schema.StreamReaderFromArray([]*schema.Message{
			{Role: schema.Assistant, Content: "a"},
			{Role: schema.Assistant, Content: "b"},
		}), nil
	}}
	boom := errors.New("client gone")
	reported := 0
	_, meta, err := streamChat(context.Background(), chat, EvalStageAnswer, nil, func(StageEvent) error {
		reported++
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("streamChat() error = %v, want the report error", err)
	}
	if reported != 1 {
		t.Errorf("report calls = %d, want 1 (abort on the first delta)", reported)
	}
	if meta.Stage != EvalStageAnswer {
		t.Errorf("meta.Stage = %q, want %q", meta.Stage, EvalStageAnswer)
	}
}
