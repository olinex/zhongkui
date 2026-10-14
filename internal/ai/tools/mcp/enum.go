// 本文件把远程 MCP 服务器的传输方式注册为枚举类 mcpTransport，并声明其取值类型 Transport（远端
// 建连本文件即消费方，故取值空间与建连逻辑同包，不再向外声明重复常量）。类名须与视图层绑定标签
// enum=mcpTransport 及 search.toml 字段绑定的引用一致。
package mcp

import (
	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/enum"
)

// Transport 是远程 MCP 服务器传输方式的取值类型（mcp_servers.transport 与视图层 transport 字段）。
type Transport string

// 传输方式常量。
const (
	// TransportStreamableHTTP 可流式 HTTP 传输（MCP 现行推荐传输）
	TransportStreamableHTTP Transport = "streamable_http"
	// TransportSSE Server-Sent Events 传输（兼容旧版远程服务器）
	TransportSSE Transport = "sse"
)

// transports 是传输方式的取值空间，声明顺序即枚举类条目顺序，也是前端下拉的展示顺序。
var transports = []Transport{TransportStreamableHTTP, TransportSSE}

// init 注册枚举类 mcpTransport（取值空间固定，条目在注册时快照）。
func init() {
	enum.Register(enum.Type{Name: "mcpTransport", Entries: enum.EntriesOf(transports)})
}
