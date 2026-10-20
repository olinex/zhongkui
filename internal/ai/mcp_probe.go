package ai

import (
	// 标准库
	"context"
	"time"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/tools/mcp"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// MCPRemoteTool 是远端 MCP 服务器 tools/list 返回的一个工具的展示信息：服务器侧的工具名与
// 可读描述。它是 internal/ai/tools/mcp.RemoteTool 的类型别名（二者为同一类型），避免同一结构
// 在同模块内重复声明；视图层（连通性测试、工具清单同步）经本包使用该类型，只依赖 internal/ai，
// 不直接引用 MCP SDK。
type MCPRemoteTool = mcp.RemoteTool

// ProbeMCPServer 连接远端 MCP 服务器（initialize 握手 + tools/list）取回全部工具的名称与描述，
// 随即关闭会话；失败返回英文错误。transport 取 streamable_http / sse，url 非空，headers 为已由
// 调用方从密文列解出的明文请求头（可为 nil），timeout 为单次请求与流的最大时长（非正数时交由
// ctx 约束）。本函数不落库、不产生持久化副作用，供连通性测试与工具清单同步共用。
func ProbeMCPServer(ctx context.Context, transport, url string, headers map[string]string, timeout time.Duration) ([]MCPRemoteTool, error) {
	conn, err := mcp.DialConn(ctx, mcp.ConnConfig{
		Transport: mcp.Transport(transport),
		URL:       url,
		Headers:   headers,
		Timeout:   timeout,
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer func() {
		_ = conn.Close()
	}()
	tools, err := conn.ListAllTools(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return tools, nil
}
