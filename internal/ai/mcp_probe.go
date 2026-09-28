package ai

import (
	// stdlib
	"context"
	"time"

	// internal
	"zhongkui.com/zhongkui/internal/ai/tools/mcp"
)

// MCPRemoteTool 是远端 MCP 服务器 tools/list 返回的一个工具的展示信息：服务器侧的工具名与
// 可读描述。它是 internal/ai/tools/mcp.RemoteTool 的类型别名（二者为同一类型），避免同一结构
// 在同模块内重复声明；视图层（连通性测试、工具清单同步）经本包使用该类型，只依赖 internal/ai，
// 不直接引用 MCP SDK。
// MCPRemoteTool is the display information of one tool returned by a remote MCP server's
// tools/list: the server-side tool name and its human-readable description. It is a type alias of
// internal/ai/tools/mcp.RemoteTool (the two are the same type), avoiding a duplicate declaration of
// the same struct within the module; the view layer (connectivity test, tool-list synchronization)
// uses it through this package and depends only on internal/ai, never referencing the MCP SDK.
type MCPRemoteTool = mcp.RemoteTool

// ProbeMCPServer 连接远端 MCP 服务器（initialize 握手 + tools/list）取回全部工具的名称与描述，
// 随即关闭会话；失败返回英文错误。transport 取 streamable_http / sse，url 非空，headers 为已由
// 调用方从密文列解出的明文请求头（可为 nil），timeout 为单次请求与流的最大时长（非正数时交由
// ctx 约束）。本函数不落库、不产生持久化副作用，供连通性测试与工具清单同步共用。
// ProbeMCPServer connects the remote MCP server (initialize handshake + tools/list) and fetches the
// names and descriptions of all its tools, closing the session right after; a failure yields an
// English error. transport is streamable_http / sse, url must be non-empty, headers are the
// plaintext request headers the caller has decrypted from the encrypted column (may be nil), and
// timeout bounds a single request and stream (a non-positive value leaves the bound to ctx). It
// persists nothing and has no side effect, shared by the connectivity test and the tool-list
// synchronization.
func ProbeMCPServer(ctx context.Context, transport, url string, headers map[string]string, timeout time.Duration) ([]MCPRemoteTool, error) {
	conn, err := mcp.DialConn(ctx, mcp.ConnConfig{
		Transport: mcp.Transport(transport),
		URL:       url,
		Headers:   headers,
		Timeout:   timeout,
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = conn.Close()
	}()
	tools, err := conn.ListAllTools(ctx)
	if err != nil {
		return nil, err
	}
	return tools, nil
}
