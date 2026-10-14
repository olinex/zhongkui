package ai

import (
	// 标准库
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/tools/mcp"
)

// jsonrpcRequest 是 MCP 客户端发来的 JSON-RPC 请求的最小视图；id 以原始 JSON 承载以便原样回显。
type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// jsonrpcResponse 是回给 MCP 客户端的 JSON-RPC 响应。
type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
}

// stubRemoteTool 是桩服务远端 tools/list 返回的一项。
type stubRemoteTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// newMCPJSONRPCStub 起一个最小的 streamable_http MCP 桩服务：实现 initialize 握手、initialized
// 通知（202）与 tools/list（返回给定工具清单），其余方法返回 -32601。它不含任何 MCP SDK 依赖，
// 因此可直接在 internal/ai 的测试中使用而不越过「SDK 只在 internal/ai/tools/mcp」的边界。
func newMCPJSONRPCStub(t *testing.T, tools []stubRemoteTool) *httptest.Server {
	t.Helper()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete { // 会话关闭通知：无会话 id，直接成功
			w.WriteHeader(http.StatusOK)
			return
		}
		var req jsonrpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		// 无 id 的是通知（notifications/initialized）：按协议回 202 且无正文。
		if len(req.ID) == 0 || string(req.ID) == "null" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{
				"protocolVersion": "2025-03-26",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "zki-jsonrpc-stub", "version": "1.0.0"},
			}
		case "tools/list":
			result = map[string]any{"tools": tools}
		default:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jsonrpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result})
	})
	return httptest.NewServer(handler)
}

// TestProbeMCPServer_RealHandshakeAndToolList 端到端驱动 ai.ProbeMCPServer：对最小 streamable_http
// 桩服务执行 initialize 握手 + tools/list，断言取回远端工具的名称与描述清单（连通性测试与工具
// 清单同步共用的核心路径）。
func TestProbeMCPServer_RealHandshakeAndToolList(t *testing.T) {
	stub := newMCPJSONRPCStub(t, []stubRemoteTool{
		{Name: "search", Description: "searches the web", InputSchema: map[string]any{"type": "object"}},
		{Name: "fetch", Description: "fetches a url", InputSchema: map[string]any{"type": "object"}},
	})
	defer stub.Close()

	tools, err := ProbeMCPServer(context.Background(), string(mcp.TransportStreamableHTTP), stub.URL, nil, 10*time.Second)
	if err != nil {
		t.Fatalf("ProbeMCPServer error = %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("probe returned %d tools, want 2", len(tools))
	}
	got := map[string]string{}
	for _, item := range tools {
		got[item.Name] = item.Description
	}
	if got["search"] != "searches the web" || got["fetch"] != "fetches a url" {
		t.Errorf("probe tools = %v, want the remote names and descriptions", got)
	}
}

// TestProbeMCPServer_UnreachableReturnsEnglishError 校验不可达端点返回英文错误（视图层据此返回
// 明确的连通性失败提示）。
func TestProbeMCPServer_UnreachableReturnsEnglishError(t *testing.T) {
	if _, err := ProbeMCPServer(context.Background(), string(mcp.TransportStreamableHTTP), "http://127.0.0.1:1/mcp", nil, 2*time.Second); err == nil {
		t.Fatal("ProbeMCPServer error = nil, want a handshake failure on an unreachable endpoint")
	} else if err.Error() == "" {
		t.Errorf("ProbeMCPServer error = %v, want a non-empty English error", err)
	}
}
