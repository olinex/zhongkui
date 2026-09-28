package mcp

import (
	// 标准库
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	// 第三方
	"github.com/cloudwego/eino/components/tool"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// newStreamableStub 起一个真实的 streamable_http MCP 服务器（实现 initialize 握手与 tools/list，
// 且提供可被 tools/call 调用的工具），返回其地址与关闭函数。本测试包是 MCP SDK 在本仓库内的
// 唯一落点，故可在这里直接使用 SDK 的 server 包构造协议正确的对端。
func newStreamableStub(t *testing.T) (string, func()) {
	t.Helper()
	srv := server.NewMCPServer("zki-stub", "1.0.0")
	srv.AddTool(
		mcpgo.NewTool("echo", mcpgo.WithDescription("echoes the input back")),
		func(_ context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
			return mcpgo.NewToolResultText("echo:" + request.GetString("text", "")), nil
		},
	)
	srv.AddTool(
		mcpgo.NewTool("clock", mcpgo.WithDescription("reports a fixed time")),
		func(_ context.Context, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
			return mcpgo.NewToolResultText("12:00"), nil
		},
	)
	ts := server.NewTestStreamableHTTPServer(srv)
	return ts.URL, ts.Close
}

// TestDialConn_ListAndLoadTools_RealProtocol 端到端打通真实 MCP 协议：经适配层 DialConn 完成
// initialize 握手后，ListAllTools 取回远端全部工具的名单与描述，LoadTools 按远端工具名取回 eino
// tool.BaseTool（Info.Name 与远端一致）并能真正 invoke 远端工具；最后 Close 释放会话。
func TestDialConn_ListAndLoadTools_RealProtocol(t *testing.T) {
	url, stop := newStreamableStub(t)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, err := DialConn(ctx, ConnConfig{Transport: TransportStreamableHTTP, URL: url, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("DialConn error = %v", err)
	}
	defer func() { _ = conn.Close() }()

	all, err := conn.ListAllTools(ctx)
	if err != nil {
		t.Fatalf("ListAllTools error = %v", err)
	}
	names := make([]string, 0, len(all))
	descriptions := map[string]string{}
	for _, remote := range all {
		names = append(names, remote.Name)
		descriptions[remote.Name] = remote.Description
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "clock,echo" {
		t.Errorf("remote tool names = %v, want [clock echo]", names)
	}
	if descriptions["echo"] != "echoes the input back" {
		t.Errorf("echo description = %q, want the remote description", descriptions["echo"])
	}

	built, err := conn.LoadTools(ctx, []string{"echo"}, nil)
	if err != nil {
		t.Fatalf("LoadTools error = %v", err)
	}
	if len(built) != 1 {
		t.Fatalf("LoadTools returned %d tools, want 1", len(built))
	}
	info, err := built[0].Info(ctx)
	if err != nil {
		t.Fatalf("tool Info error = %v", err)
	}
	if info.Name != "echo" {
		t.Errorf("tool Info.Name = %q, want the remote name echo", info.Name)
	}
	invokable, ok := built[0].(tool.InvokableTool)
	if !ok {
		t.Fatalf("built tool %T does not implement tool.InvokableTool", built[0])
	}
	res, err := invokable.InvokableRun(ctx, `{"text":"hi"}`)
	if err != nil {
		t.Fatalf("InvokableRun error = %v", err)
	}
	if !strings.Contains(res, "echo:hi") {
		t.Errorf("InvokableRun result = %q, want the remote tool output", res)
	}

	// 空名单按契约不发远端请求并返回空切片。
	empty, err := conn.LoadTools(ctx, nil, nil)
	if err != nil {
		t.Fatalf("LoadTools(nil) error = %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("LoadTools(nil) = %v, want a non-nil empty slice", empty)
	}
}

// TestDial_RejectsUnsupportedTransportAndUnreachable 校验适配层的失败契约：非法传输方式与不可达
// 端点均返回英文错误。
func TestDial_RejectsUnsupportedTransportAndUnreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := Dial(ctx, ConnConfig{Transport: Transport("stdio"), URL: "http://x.test/mcp"}); err == nil {
		t.Error("Dial stdio error = nil, want an unsupported-transport error")
	} else if !strings.Contains(err.Error(), "unsupported transport") {
		t.Errorf("Dial stdio error = %q, want the English unsupported-transport message", err)
	}

	if _, err := Dial(ctx, ConnConfig{Transport: TransportStreamableHTTP, URL: ""}); err == nil {
		t.Error("Dial empty URL error = nil, want a non-empty-url error")
	}

	// 127.0.0.1:1 上无监听进程，握手必然失败且不得泄漏连接。
	if _, err := Dial(ctx, ConnConfig{Transport: TransportStreamableHTTP, URL: "http://127.0.0.1:1/mcp", Timeout: 2 * time.Second}); err == nil {
		t.Error("Dial unreachable error = nil, want a handshake failure")
	} else if !strings.Contains(err.Error(), "transport error") {
		t.Errorf("Dial unreachable error = %q, want the English transport failure message", err)
	}
}
