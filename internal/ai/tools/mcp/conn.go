// Package mcp 提供远程 MCP（Model Context Protocol）服务器的薄适配层：连接远端端点并按工具名
// 取回工具，统一以 eino tool.BaseTool 暴露，使消费侧无需区分工具来源（内置 / MCP）。
package mcp

import (
	// 标准库
	"context"

	// 第三方
	"github.com/cloudwego/eino/components/tool"
	"github.com/mark3labs/mcp-go/client"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// RemoteTool 是远端 MCP 服务器 tools/list 返回的一个工具的展示信息：服务器侧的工具名与
// 可读描述。与 eino 的 tool.BaseTool 不同，它只承载展示所需的元信息，供连通性测试与工具
// 清单同步使用。
type RemoteTool struct {
	// Name 远端服务器侧的工具名
	Name string
	// Description 远端提供的工具描述（可能为空）
	Description string
}

// Connection 是已完成握手的 MCP 会话在运行时装配层的句柄：它把「取工具」与「关闭会话」收敛为
// 两个方法，使 internal/ai 的装配代码无需引用 MCP SDK（SDK 只出现在本包内）。
type Connection struct {
	cli client.MCPClient
}

// DialConn 连接指定远程 MCP 服务器并完成 initialize 握手，返回本包对外的会话句柄；调用方负责在
// 会话不再需要时调用 Close 释放资源（SSE 为有状态长连接）。与 Dial 的差别仅在于返回类型：本函数
// 返回可被非 SDK 代码持有的 Connection，Dial 保留给需要直接操作 SDK 客户端的调用方。
func DialConn(ctx context.Context, cfg ConnConfig) (*Connection, error) {
	cli, err := Dial(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &Connection{cli: cli}, nil
}

// LoadTools 经本会话按 remoteNames 取回远端工具，语义与包级 LoadTools 一致。
func (c *Connection) LoadTools(ctx context.Context, remoteNames []string, headers map[string]string) ([]tool.BaseTool, error) {
	if c == nil || c.cli == nil {
		return nil, errors.New("mcp: nil connection")
	}
	return LoadTools(ctx, c.cli, remoteNames, headers)
}

// ListAllTools 列出本会话远端服务器的全部工具（tools/list 全量，SDK 自动翻页），返回工具名与
// 描述。与 LoadTools 的「按名精确取用」语义相反，本方法语义是「取全部」，且空结果不会被当作
// 「不请求远端」处理：连通性测试与工具清单同步正是要拿到全部工具（含空清单）这一事实。失败
// 返回英文错误。
func (c *Connection) ListAllTools(ctx context.Context) ([]RemoteTool, error) {
	if c == nil || c.cli == nil {
		return nil, errors.New("mcp: nil connection")
	}
	result, err := c.cli.ListTools(ctx, mcpgo.ListToolsRequest{})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	tools := make([]RemoteTool, 0, len(result.Tools))
	for _, t := range result.Tools {
		tools = append(tools, RemoteTool{Name: t.Name, Description: t.Description})
	}
	return tools, nil
}

// Close 关闭本会话并释放底层连接；nil 安全（未连接的句柄返回 nil）。
func (c *Connection) Close() error {
	if c == nil || c.cli == nil {
		return nil
	}
	return c.cli.Close()
}
