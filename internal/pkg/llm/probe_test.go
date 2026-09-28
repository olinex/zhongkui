package llm

import (
	// stdlib
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newEmbeddingMockServer 启动一个模拟 OpenAI 兼容 embedding 端点的 httptest 服务器：
// 校验请求为 POST {baseURL}/v1/embeddings 并解析 JSON 请求体，交由 respond 决定状态码与
// 响应体；返回的第二个函数用于读取最近一次请求体（供断言 dimensions 参数）。
// newEmbeddingMockServer starts an httptest server mimicking an OpenAI-compatible embedding
// endpoint: it verifies the request is POST {baseURL}/v1/embeddings and decodes the JSON body,
// then lets respond decide the status code and the response body; the second returned function
// reads the latest request body (for asserting the dimensions parameter).
func newEmbeddingMockServer(t *testing.T, respond func(req map[string]any) (int, map[string]any)) (*httptest.Server, func() map[string]any) {
	t.Helper()
	var lastRequest map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("mock endpoint method = %q, want %q", r.Method, http.MethodPost)
		}
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("mock endpoint path = %q, want %q", r.URL.Path, "/v1/embeddings")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("mock endpoint decode request body: %v", err)
		}
		lastRequest = body
		status, payload := respond(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(payload)
	}))
	t.Cleanup(server.Close)
	return server, func() map[string]any { return lastRequest }
}

// mockEmbeddingResponse 构造符合 OpenAI 兼容响应结构的单条向量响应
// （data[].embedding 为浮点向量），dimensions 为 0 时返回空向量。
// mockEmbeddingResponse builds a single-vector response matching the OpenAI-compatible
// response structure (data[].embedding holds the float vector); a dimensions of 0 yields an
// empty vector.
func mockEmbeddingResponse(dimensions int) map[string]any {
	vector := make([]float32, dimensions)
	for i := range vector {
		vector[i] = float32(i+1) / 10
	}
	return map[string]any{
		"object": "list",
		"data": []map[string]any{
			{"object": "embedding", "embedding": vector, "index": 0},
		},
		"model": "embed-test",
		"usage": map[string]int{"prompt_tokens": 1, "total_tokens": 1},
	}
}

// mockEmbedConfig 构造指向 mock 端点（{baseURL}/v1）的 embedding 模型配置。
// mockEmbedConfig builds an embedding model config pointing at the mock endpoint
// ({baseURL}/v1).
func mockEmbedConfig(baseURL string, dimensions int) ModelConfig {
	return ModelConfig{
		Name:       "embed-test",
		Kind:       ModelKindEmbedding,
		BaseURL:    baseURL + "/v1",
		APIKey:     "sk-test",
		Model:      "embed-test",
		Timeout:    5 * time.Second,
		Dimensions: dimensions,
	}
}

// TestProbeEmbeddingDimensions_Native 测试未声明维度时按模型原生维度探测：请求体不得携带
// dimensions 字段，探测结果等于端点返回的向量长度。
// TestProbeEmbeddingDimensions_Native verifies the probe without a declared dimension follows
// the native dimension of the model: the request body must carry no dimensions field and the
// result equals the vector length returned by the endpoint.
func TestProbeEmbeddingDimensions_Native(t *testing.T) {
	const native = 8
	server, lastRequest := newEmbeddingMockServer(t, func(req map[string]any) (int, map[string]any) {
		return http.StatusOK, mockEmbeddingResponse(native)
	})
	got, err := ProbeEmbeddingDimensions(context.Background(), mockEmbedConfig(server.URL, 0))
	if err != nil {
		t.Fatalf("ProbeEmbeddingDimensions(native) error = %v, want nil", err)
	}
	if got != native {
		t.Errorf("ProbeEmbeddingDimensions(native) = %d, want %d", got, native)
	}
	if _, ok := lastRequest()["dimensions"]; ok {
		t.Errorf("request body must not declare dimensions when cfg.Dimensions is 0, got %v", lastRequest())
	}
}

// TestProbeEmbeddingDimensions_Declared 测试声明维度时随请求下发 dimensions 参数：端点按
// 该维度返回向量，探测结果等于声明的维度。
// TestProbeEmbeddingDimensions_Declared verifies a declared dimension is sent with the request
// as the dimensions parameter: the endpoint answers with that dimension and the result equals
// the declared one.
func TestProbeEmbeddingDimensions_Declared(t *testing.T) {
	const declared = 4
	server, lastRequest := newEmbeddingMockServer(t, func(req map[string]any) (int, map[string]any) {
		sent, _ := req["dimensions"].(float64)
		return http.StatusOK, mockEmbeddingResponse(int(sent))
	})
	got, err := ProbeEmbeddingDimensions(context.Background(), mockEmbedConfig(server.URL, declared))
	if err != nil {
		t.Fatalf("ProbeEmbeddingDimensions(declared) error = %v, want nil", err)
	}
	if got != declared {
		t.Errorf("ProbeEmbeddingDimensions(declared) = %d, want %d", got, declared)
	}
	sent, ok := lastRequest()["dimensions"].(float64)
	if !ok {
		t.Fatalf("request body must declare dimensions when cfg.Dimensions is %d, got %v", declared, lastRequest())
	}
	if int(sent) != declared {
		t.Errorf("request dimensions = %d, want %d", int(sent), declared)
	}
}

// TestProbeEmbeddingDimensions_EmptyVector 测试端点在数据为空或向量为空时返回英文错误。
// TestProbeEmbeddingDimensions_EmptyVector verifies an English error is returned when the
// endpoint answers with no data or with an empty vector.
func TestProbeEmbeddingDimensions_EmptyVector(t *testing.T) {
	t.Run("empty data", func(t *testing.T) {
		server, _ := newEmbeddingMockServer(t, func(req map[string]any) (int, map[string]any) {
			return http.StatusOK, map[string]any{"object": "list", "data": []any{}, "model": "embed-test"}
		})
		if _, err := ProbeEmbeddingDimensions(context.Background(), mockEmbedConfig(server.URL, 0)); err == nil {
			t.Errorf("ProbeEmbeddingDimensions(empty data) error = nil, want non-nil")
		}
	})
	t.Run("empty vector", func(t *testing.T) {
		server, _ := newEmbeddingMockServer(t, func(req map[string]any) (int, map[string]any) {
			return http.StatusOK, mockEmbeddingResponse(0)
		})
		_, err := ProbeEmbeddingDimensions(context.Background(), mockEmbedConfig(server.URL, 0))
		if err == nil {
			t.Fatalf("ProbeEmbeddingDimensions(empty vector) error = nil, want non-nil")
		}
		if !strings.Contains(err.Error(), "returned no vector") {
			t.Errorf("ProbeEmbeddingDimensions(empty vector) error = %v, want no-vector message", err)
		}
	})
}

// TestProbeEmbeddingDimensions_ServerError 测试端点返回 5xx 时底层向量化错误被正确传出。
// TestProbeEmbeddingDimensions_ServerError verifies the underlying embedding error propagates
// when the endpoint answers with 5xx.
func TestProbeEmbeddingDimensions_ServerError(t *testing.T) {
	server, _ := newEmbeddingMockServer(t, func(req map[string]any) (int, map[string]any) {
		return http.StatusInternalServerError, map[string]any{
			"error": map[string]any{"message": "boom", "type": "server_error"},
		}
	})
	if _, err := ProbeEmbeddingDimensions(context.Background(), mockEmbedConfig(server.URL, 0)); err == nil {
		t.Errorf("ProbeEmbeddingDimensions(server error) error = nil, want non-nil")
	} else if !strings.Contains(err.Error(), "boom") {
		t.Errorf("ProbeEmbeddingDimensions(server error) error = %v, want the endpoint error message", err)
	}
}
