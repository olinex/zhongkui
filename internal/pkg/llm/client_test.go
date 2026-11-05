package llm

import (
	// 标准库
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	// 第三方
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// fakeEmbedder 记录每次收到的批次文本，并返回与入参等长的占位向量（第 i 个向量的唯一元素为
// 文本长度），用于验证 EmbedClient 的切批与顺序对齐行为。
type fakeEmbedder struct {
	batches [][]string
}

// EmbedStrings 记录本次收到的批次文本，并返回与入参等长的占位向量（元素为文本长度）。
func (f *fakeEmbedder) EmbedStrings(_ context.Context, texts []string, _ ...embedding.Option) ([][]float64, error) {
	f.batches = append(f.batches, append([]string(nil), texts...))
	out := make([][]float64, len(texts))
	for i, s := range texts {
		out[i] = []float64{float64(len(s))}
	}
	return out, nil
}

// callbackEmbedder 在每次调用时按批发送 eino 回调并上报固定的 token 用量，用于验证
// EmbedClient 的用量采集路径。
type callbackEmbedder struct{ promptPerBatch int }

// EmbedStrings 发送 eino 起止回调并上报固定 token 用量，向量元素为文本长度。
func (c *callbackEmbedder) EmbedStrings(ctx context.Context, texts []string, _ ...embedding.Option) ([][]float64, error) {
	ctx = callbacks.EnsureRunInfo(ctx, "fake", components.ComponentOfEmbedding)
	ctx = callbacks.OnStart(ctx, &embedding.CallbackInput{Texts: texts})
	out := make([][]float64, len(texts))
	for i, s := range texts {
		out[i] = []float64{float64(len(s))}
	}
	_ = callbacks.OnEnd(ctx, &embedding.CallbackOutput{
		Embeddings: out,
		TokenUsage: &embedding.TokenUsage{PromptTokens: c.promptPerBatch, TotalTokens: c.promptPerBatch},
	})
	return out, nil
}

// TestNewChatClient_Validation 测试 Chat 客户端缺 baseURL/apiKey/model 时返回英文错误。
func TestNewChatClient_Validation(t *testing.T) {
	cases := []struct {
		name string
		cfg  ModelConfig
	}{
		{"no baseURL", ModelConfig{APIKey: "k", Model: "m"}},
		{"no apiKey", ModelConfig{BaseURL: "http://x", Model: "m"}},
		{"no model", ModelConfig{BaseURL: "http://x", APIKey: "k"}},
	}
	for _, tt := range cases {
		if _, err := NewChatClient(context.Background(), tt.cfg); err == nil {
			t.Errorf("NewChatClient(%s) error = nil, want error", tt.name)
		} else if !strings.Contains(err.Error(), "incomplete chat config") {
			t.Errorf("NewChatClient(%s) error = %v, want incomplete-chat-config message", tt.name, err)
		}
	}
}

// TestNewEmbedClient_Validation 测试 Embedding 客户端缺 baseURL/apiKey/model 时返回英文错误。
func TestNewEmbedClient_Validation(t *testing.T) {
	cases := []struct {
		name string
		cfg  ModelConfig
	}{
		{"no baseURL", ModelConfig{APIKey: "k", Model: "m"}},
		{"no apiKey", ModelConfig{BaseURL: "http://x", Model: "m"}},
		{"no model", ModelConfig{BaseURL: "http://x", APIKey: "k"}},
	}
	for _, tt := range cases {
		if _, err := NewEmbedClient(context.Background(), tt.cfg); err == nil {
			t.Errorf("NewEmbedClient(%s) error = nil, want error", tt.name)
		} else if !strings.Contains(err.Error(), "incomplete embedding config") {
			t.Errorf("NewEmbedClient(%s) error = %v, want incomplete-embedding-config message", tt.name, err)
		}
	}
}

// TestClient_ModelName 测试客户端回传底层供应商模型名（供状态与 dry-run 元信息展示）。
func TestClient_ModelName(t *testing.T) {
	// eino 构造器不发起网络请求，仅组装配置，可使用不可达地址
	chat, err := NewChatClient(context.Background(), ModelConfig{
		Kind:        ModelKindChat,
		BaseURL:     "http://127.0.0.1:1/v1",
		APIKey:      "sk-test",
		Model:       "gpt-test",
		Timeout:     time.Second,
		Temperature: 0.5,
		MaxTokens:   1024,
	})
	if err != nil {
		t.Fatalf("NewChatClient() error = %v", err)
	}
	if chat.ModelName() != "gpt-test" {
		t.Errorf("ChatClient.ModelName() = %q, want %q", chat.ModelName(), "gpt-test")
	}
	embed, err := NewEmbedClient(context.Background(), ModelConfig{
		Kind:       ModelKindEmbedding,
		BaseURL:    "http://127.0.0.1:1/v1",
		APIKey:     "sk-test",
		Model:      "embed-test",
		Dimensions: 128,
	})
	if err != nil {
		t.Fatalf("NewEmbedClient() error = %v", err)
	}
	if embed.ModelName() != "embed-test" {
		t.Errorf("EmbedClient.ModelName() = %q, want %q", embed.ModelName(), "embed-test")
	}
}

// TestEmbedClient_EmbedStringsBatching 验证 EmbedClient 在入参超过单请求上限时按 embedBatchSize
// 切批提交，结果按入参顺序拼接；未超过上限时只提交一批。
func TestEmbedClient_EmbedStringsBatching(t *testing.T) {
	ctx := context.Background()

	t.Run("within cap sends a single batch", func(t *testing.T) {
		fake := &fakeEmbedder{}
		client := &EmbedClient{embedder: fake}
		texts := []string{"a", "bb", "ccc"}
		vectors, err := client.EmbedStrings(ctx, texts)
		if err != nil {
			t.Fatalf("EmbedStrings() error = %v", err)
		}
		if len(fake.batches) != 1 || len(fake.batches[0]) != len(texts) {
			t.Fatalf("batches = %v, want a single batch of %d", fake.batches, len(texts))
		}
		if len(vectors) != len(texts) {
			t.Fatalf("len(vectors) = %d, want %d", len(vectors), len(texts))
		}
	})

	t.Run("over cap is split and concatenated in order", func(t *testing.T) {
		fake := &fakeEmbedder{}
		client := &EmbedClient{embedder: fake}
		texts := make([]string, embedBatchSize*2+3)
		for i := range texts {
			texts[i] = strings.Repeat("x", i+1)
		}
		vectors, err := client.EmbedStrings(ctx, texts)
		if err != nil {
			t.Fatalf("EmbedStrings() error = %v", err)
		}
		if len(fake.batches) != 3 {
			t.Fatalf("len(batches) = %d, want 3", len(fake.batches))
		}
		for i, want := range []int{embedBatchSize, embedBatchSize, 3} {
			if len(fake.batches[i]) != want {
				t.Errorf("batch %d size = %d, want %d", i, len(fake.batches[i]), want)
			}
		}
		if len(vectors) != len(texts) {
			t.Fatalf("len(vectors) = %d, want %d", len(vectors), len(texts))
		}
		for i, v := range vectors {
			if len(v) != 1 || v[0] != float64(i+1) {
				t.Fatalf("vectors[%d] = %v, want [%d]", i, v, i+1)
			}
		}
	})
}

// TestEmbedClient_EmbedStringsWithUsage 验证 EmbedStringsWithUsage 采集底层向量化组件经 eino
// 回调上报的 token 用量：单批时等于该批上报值，超批时等于各批之和，且向量顺序与入参一致。
func TestEmbedClient_EmbedStringsWithUsage(t *testing.T) {
	ctx := context.Background()

	t.Run("within cap reports single batch usage", func(t *testing.T) {
		client := &EmbedClient{embedder: &callbackEmbedder{promptPerBatch: 7}}
		texts := []string{"a", "bb", "ccc"}
		vectors, usage, err := client.EmbedStringsWithUsage(ctx, texts)
		if err != nil {
			t.Fatalf("EmbedStringsWithUsage() error = %v", err)
		}
		if usage.PromptTokens != 7 || usage.TotalTokens != 7 {
			t.Errorf("usage = %+v, want prompt/total = 7", usage)
		}
		if len(vectors) != len(texts) {
			t.Fatalf("len(vectors) = %d, want %d", len(vectors), len(texts))
		}
		for i, v := range vectors {
			if len(v) != 1 || v[0] != float64(i+1) {
				t.Fatalf("vectors[%d] = %v, want [%d]", i, v, i+1)
			}
		}
	})

	t.Run("over cap accumulates usage across batches", func(t *testing.T) {
		client := &EmbedClient{embedder: &callbackEmbedder{promptPerBatch: 7}}
		texts := make([]string, embedBatchSize*2+3)
		for i := range texts {
			texts[i] = strings.Repeat("x", i+1)
		}
		vectors, usage, err := client.EmbedStringsWithUsage(ctx, texts)
		if err != nil {
			t.Fatalf("EmbedStringsWithUsage() error = %v", err)
		}
		if want := 7 * 3; usage.PromptTokens != want || usage.TotalTokens != want {
			t.Errorf("usage = %+v, want prompt/total = %d", usage, want)
		}
		if len(vectors) != len(texts) {
			t.Fatalf("len(vectors) = %d, want %d", len(vectors), len(texts))
		}
		for i, v := range vectors {
			if len(v) != 1 || v[0] != float64(i+1) {
				t.Fatalf("vectors[%d] = %v, want [%d]", i, v, i+1)
			}
		}
	})
}

// TestRequestExtraFields 验证依据思考模式开关合并请求字段：仅 ExtraBody、仅思考开关（on/off
// 分别写入正确取值）、思考字段覆盖 ExtraBody 同名键、未知 thinking 不注入、以及两者皆空返回 nil。
func TestRequestExtraFields(t *testing.T) {
	thinkingCfg := ModelConfig{ThinkingField: "enable_thinking", ThinkingOnValue: true, ThinkingOffValue: false}
	cases := []struct {
		name     string
		cfg      ModelConfig
		thinking string
		want     map[string]any
	}{
		{
			name:     "extra body only",
			cfg:      ModelConfig{ExtraBody: map[string]any{"top_k": 20}},
			thinking: "",
			want:     map[string]any{"top_k": 20},
		},
		{
			name:     "thinking on only",
			cfg:      thinkingCfg,
			thinking: "on",
			want:     map[string]any{"enable_thinking": true},
		},
		{
			name:     "thinking off only",
			cfg:      thinkingCfg,
			thinking: "off",
			want:     map[string]any{"enable_thinking": false},
		},
		{
			name: "thinking on overrides extra body key",
			cfg: ModelConfig{
				ExtraBody:        map[string]any{"enable_thinking": "legacy", "top_k": 20},
				ThinkingField:    "enable_thinking",
				ThinkingOnValue:  true,
				ThinkingOffValue: false,
			},
			thinking: "on",
			want:     map[string]any{"enable_thinking": true, "top_k": 20},
		},
		{
			name: "unknown thinking leaves extra body untouched",
			cfg: ModelConfig{
				ExtraBody:        map[string]any{"enable_thinking": "legacy"},
				ThinkingField:    "enable_thinking",
				ThinkingOnValue:  true,
				ThinkingOffValue: false,
			},
			thinking: "auto",
			want:     map[string]any{"enable_thinking": "legacy"},
		},
		{
			name:     "empty extra body and no thinking returns nil",
			cfg:      ModelConfig{},
			thinking: "on",
			want:     nil,
		},
	}
	for _, tt := range cases {
		if got := requestExtraFields(tt.cfg, tt.thinking); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("requestExtraFields(%s) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// TestChatClient_RequestOptions 验证 RequestOptions 在合并结果非空时返回单个选项、为空时返回空切片，
// 且合并过程不写回配置快照的 ExtraBody（拷贝语义）。
func TestChatClient_RequestOptions(t *testing.T) {
	extra := map[string]any{"top_k": 20}
	client := &ChatClient{cfg: ModelConfig{
		ExtraBody:       extra,
		ThinkingField:   "enable_thinking",
		ThinkingOnValue: true,
	}}
	if opts := client.RequestOptions(""); len(opts) != 1 {
		t.Errorf("RequestOptions(\"\") returned %d options, want 1", len(opts))
	}
	if opts := client.RequestOptions("on"); len(opts) != 1 {
		t.Errorf("RequestOptions(\"on\") returned %d options, want 1", len(opts))
	}
	if opts := (&ChatClient{}).RequestOptions("on"); len(opts) != 0 {
		t.Errorf("RequestOptions with empty config returned %d options, want 0", len(opts))
	}
	if _, ok := extra["enable_thinking"]; ok {
		t.Errorf("cfg.ExtraBody was mutated with the thinking field: %v", extra)
	}
}

// TestNewChatHTTPClient_NoTotalTimeout 验证对话专用 HTTP 客户端不设整请求超时
// （http.Client.Timeout），仅以响应头超时约束首字节，避免流式长回答被整体时长中断。
func TestNewChatHTTPClient_NoTotalTimeout(t *testing.T) {
	client := newChatHTTPClient(30 * time.Second)
	if client.Timeout != 0 {
		t.Errorf("client.Timeout = %v, want 0 (no whole-request timeout)", client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client.Transport = %T, want *http.Transport", client.Transport)
	}
	if transport.ResponseHeaderTimeout != 30*time.Second {
		t.Errorf("ResponseHeaderTimeout = %v, want 30s", transport.ResponseHeaderTimeout)
	}
	if zero := newChatHTTPClient(0); zero.Timeout != 0 {
		t.Errorf("newChatHTTPClient(0).Timeout = %v, want 0", zero.Timeout)
	}
}

// TestChatClient_StreamExceedsModelTimeout 验证流式生成的整体时长可以超过模型超时而不被中断：
// 服务端先发首块，再停顿远超模型超时的时长，客户端仍应读完全部增量。这是对「整请求超时
// （http.Client.Timeout）中断流式响应、报 context deadline exceeded ... while reading body」
// 的回归测试。
func TestChatClient_StreamExceedsModelTimeout(t *testing.T) {
	const modelTimeout = 150 * time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Errorf("response writer does not support flushing")
			return
		}
		writeChunk := func(content string) {
			_, _ = io.WriteString(w, `data: {"id":"1","object":"chat.completion.chunk","created":0,"model":"m","choices":[{"index":0,"delta":{"content":"`+content+`"},"finish_reason":null}]}`+"\n\n")
			flusher.Flush()
		}
		writeChunk("hel")
		// 停顿远超模型超时：若按 http.Client.Timeout 实现会在此中断
		time.Sleep(4 * modelTimeout)
		writeChunk("lo")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	chat, err := NewChatClient(context.Background(), ModelConfig{
		Kind:        ModelKindChat,
		BaseURL:     srv.URL + "/v1",
		APIKey:      "sk-test",
		Model:       "gpt-test",
		Timeout:     modelTimeout,
		Temperature: 0.5,
		MaxTokens:   16,
	})
	if err != nil {
		t.Fatalf("NewChatClient() error = %v", err)
	}
	start := time.Now()
	reader, err := chat.Stream(context.Background(), []*schema.Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer reader.Close()
	var sb strings.Builder
	for {
		chunk, recvErr := reader.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			t.Fatalf("Recv() error = %v", recvErr)
		}
		if chunk != nil {
			sb.WriteString(chunk.Content)
		}
	}
	if sb.String() != "hello" {
		t.Errorf("streamed content = %q, want %q", sb.String(), "hello")
	}
	if elapsed := time.Since(start); elapsed <= modelTimeout {
		t.Errorf("elapsed = %v, want > model timeout %v (stream should outlive the model timeout)", elapsed, modelTimeout)
	}
}

// TestChatClient_StreamCanceled 验证流式路径尊重传入 ctx 的取消：服务端持续慢速产出 SSE，客户端
// 读到首块后取消 ctx，Recv 必须迅速以 context.Canceled（errors.Is）结束、且不再收到后续增量——
// 这正是「用户中止即断流止损」的底座：取消经 ctx 传播到上游 HTTP 请求并关闭连接，从根本上停止
// token 消耗。
func TestChatClient_StreamCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	firstChunkSent := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Errorf("response writer does not support flushing")
			return
		}
		writeChunk := func(content string) {
			_, _ = io.WriteString(w, `data: {"id":"1","object":"chat.completion.chunk","created":0,"model":"m","choices":[{"index":0,"delta":{"content":"`+content+`"},"finish_reason":null}]}`+"\n\n")
			flusher.Flush()
		}
		writeChunk("he")
		close(firstChunkSent)
		// 持续慢速产出，直到客户端断开（ctx 取消会中断本次请求、进而停止本循环）。
		for {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(10 * time.Millisecond):
				writeChunk("x")
			}
		}
	}))
	defer srv.Close()

	chat, err := NewChatClient(context.Background(), ModelConfig{
		Kind:        ModelKindChat,
		BaseURL:     srv.URL + "/v1",
		APIKey:      "sk-test",
		Model:       "gpt-test",
		Timeout:     5 * time.Second,
		Temperature: 0.5,
		MaxTokens:   16,
	})
	if err != nil {
		t.Fatalf("NewChatClient() error = %v", err)
	}
	reader, err := chat.Stream(ctx, []*schema.Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer reader.Close()

	chunk, err := reader.Recv()
	if err != nil {
		t.Fatalf("Recv() first chunk error = %v", err)
	}
	if chunk == nil || chunk.Content != "he" {
		t.Fatalf("first chunk = %+v, want content %q", chunk, "he")
	}
	<-firstChunkSent

	// 读到首块后取消：后续 Recv 必须迅速收敛到 context.Canceled，且不再有有效增量。
	cancel()
	start := time.Now()
	for {
		type recvResult struct {
			chunk *schema.Message
			err   error
		}
		resCh := make(chan recvResult, 1)
		go func() {
			c, e := reader.Recv()
			resCh <- recvResult{chunk: c, err: e}
		}()
		var res recvResult
		select {
		case res = <-resCh:
		case <-time.After(3 * time.Second):
			t.Fatal("Recv() did not observe the cancellation in time")
		}
		if res.err != nil {
			if !errors.Is(res.err, context.Canceled) {
				t.Fatalf("Recv() error = %v, want context.Canceled", res.err)
			}
			break
		}
		if res.chunk != nil && res.chunk.Content != "" {
			t.Errorf("received content %q after cancellation, want no further increments", res.chunk.Content)
		}
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("cancellation took %v to be observed, want it to be prompt", elapsed)
	}
}
