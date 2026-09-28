// Package mcp 是远程 MCP（Model Context Protocol）服务器的薄适配层：把「连接一个远程
// 端点并按工具名取回工具」收敛为两个函数，供运行时装配（Task 4）按服务器粒度调用。本包
// 是 MCP SDK 在本仓库内的唯一落点，远端工具一律以 eino 的 tool.BaseTool 暴露，使消费侧
// 无需感知工具来源（内置 / MCP）的差异。
package mcp

import (
	// 标准库
	"context"
	"net/http"
	"strings"
	"time"

	// 第三方
	einomcp "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	// 项目内部

	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/version"
)

// ConnConfig 是连接一个远程 MCP 服务器所需的全部信息；Headers 承载 Authorization 等凭据，
// 由调用方从密文列解出后在内存中传入，绝不落日志。
type ConnConfig struct {
	// Transport 传输方式（仅 streamable_http / sse，取值空间唯一声明于本包 Transport）
	Transport Transport
	// URL 远端 MCP 端点地址（http 或 https）
	URL string
	// Headers 建连与请求时附带的 HTTP 请求头（凭据明文，内存中传递）
	Headers map[string]string
	// Timeout 单次 HTTP 请求与流的最大时长；非正数时交由调用方 ctx 约束
	Timeout time.Duration
}

// Dial 连接指定远程 MCP 服务器并完成 initialize 握手，返回一个已可用的客户端句柄；调用方
// 负责在连接不再需要时调用其 Close 释放资源（SSE 为有状态长连接）。传输方式非法、URL 为
// 空或握手失败时返回英文错误，且不会泄漏已建立的连接。
func Dial(ctx context.Context, cfg ConnConfig) (client.MCPClient, error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, errors.New("mcp: server url must be non-empty")
	}
	cli, err := newClient(cfg)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if err := cli.Start(ctx); err != nil {
		_ = cli.Close()
		return nil, errors.WithStack(err)
	}
	// 向远端声明的客户端身份取自项目源码级常量：名称用全局前缀（base.GlobalPrefix，与
	// otel scope 名、etcd 键前缀同源），版本用构建版本（version.Short()，与健康检查、
	// otel resource 同源），避免本地字面量随项目改名或发版而漂移。
	if _, err := cli.Initialize(ctx, mcpgo.InitializeRequest{
		Params: mcpgo.InitializeParams{
			ProtocolVersion: mcpgo.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcpgo.Implementation{
				Name:    base.GlobalPrefix,
				Version: version.Short(),
			},
		},
	}); err != nil {
		_ = cli.Close()
		return nil, errors.WithStack(err)
	}
	return cli, nil
}

// newClient 按传输方式构造尚未启动的 MCP 客户端；传输方式非法时返回英文错误。
func newClient(cfg ConnConfig) (*client.Client, error) {
	switch cfg.Transport {
	case TransportStreamableHTTP:
		options := []transport.StreamableHTTPCOption{}
		if len(cfg.Headers) > 0 {
			options = append(options, transport.WithHTTPHeaders(cfg.Headers))
		}
		if cfg.Timeout > 0 {
			options = append(options, transport.WithHTTPTimeout(cfg.Timeout))
		}
		return client.NewStreamableHttpClient(cfg.URL, options...)
	case TransportSSE:
		options := []transport.ClientOption{}
		if len(cfg.Headers) > 0 {
			options = append(options, transport.WithHeaders(cfg.Headers))
		}
		if cfg.Timeout > 0 {
			options = append(options, transport.WithHTTPClient(&http.Client{Timeout: cfg.Timeout}))
		}
		return client.NewSSEMCPClient(cfg.URL, options...)
	default:
		return nil, errors.Newf("mcp: unsupported transport %q, only streamable_http and sse are supported", string(cfg.Transport))
	}
}

// LoadTools 从已完成握手的客户端按 remoteNames 取回远端工具，统一以 eino tool.BaseTool
// 暴露（其 Info 携带远端 tools/list 返回的 JSON schema），使节点 4 对内置与 MCP 工具无差别
// 消费。remoteNames 为空时返回空切片而不请求远端：eino 的取工具语义把空清单视为「取全部」，
// 与本处「按名精确取用」的契约相反，故在适配层显式收敛。存在的名字远端不存在时不报错、
// 直接缺席，由调用方按名单核对。
func LoadTools(ctx context.Context, cli client.MCPClient, remoteNames []string, headers map[string]string) ([]tool.BaseTool, error) {
	if len(remoteNames) == 0 {
		return []tool.BaseTool{}, nil
	}
	tools, err := einomcp.GetTools(ctx, &einomcp.Config{
		Cli:           cli,
		ToolNameList:  remoteNames,
		CustomHeaders: headers,
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return tools, nil
}
