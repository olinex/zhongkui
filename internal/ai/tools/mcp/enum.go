// 本文件把远程 MCP 服务器的传输方式注册为枚举类 mcpTransport，并声明其取值类型 Transport（远端
// 建连本文件即消费方，故取值空间与建连逻辑同包，不再向外声明重复常量）。类名须与视图层绑定标签
// enum=mcpTransport 及 search.toml 字段绑定的引用一致。
//
// This file registers the transport of a remote MCP server as the enum class mcpTransport and declares
// its value type Transport (the dialing logic of this very file is the consumer, so the value space
// lives next to it instead of being declared elsewhere with duplicated constants). The class name must
// match the view-layer binding tag enum=mcpTransport and the references in the search.toml field
// bindings.
package mcp

import (
	// internal
	"zhongkui.com/zhongkui/internal/pkg/enum"
)

// Transport 是远程 MCP 服务器传输方式的取值类型（mcp_servers.transport 与视图层 transport 字段）。
// Transport is the value type of a remote MCP server transport (mcp_servers.transport and the
// view-layer transport field).
type Transport string

// 传输方式常量。
// Transport constants.
const (
	// TransportStreamableHTTP 可流式 HTTP 传输（MCP 现行推荐传输）
	// TransportStreamableHTTP the streamable HTTP transport (now recommended by MCP)
	TransportStreamableHTTP Transport = "streamable_http"
	// TransportSSE Server-Sent Events 传输（兼容旧版远程服务器）
	// TransportSSE Server-Sent Events, for older remote servers
	TransportSSE Transport = "sse"
)

// transports 是传输方式的取值空间，声明顺序即枚举类条目顺序，也是前端下拉的展示顺序。
// transports is the transport value space; the declaration order is the entry order of the enum class
// and therefore the display order of the frontend select.
var transports = []Transport{TransportStreamableHTTP, TransportSSE}

// init 注册枚举类 mcpTransport（取值空间固定，条目在注册时快照）。
// init registers the enum class mcpTransport (a fixed value space, its entries snapshotted at
// registration time).
func init() {
	enum.Register(enum.Type{Name: "mcpTransport", Entries: enum.EntriesOf(transports)})
}
