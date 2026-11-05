package llmconfig

import (
	// 标准库
	"context"
	"encoding/json"
	"fmt"
	"time"

	// 第三方
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/ai/tools"
	"zhongkuisec.com/zhongkui/internal/ai/tools/mcp"
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
)

// mcpConn 是一次已建立的 MCP 会话在装配层的抽象：按名取回远端工具并关闭会话。生产实现是
// internal/ai/tools/mcp.Connection 的薄包装，测试注入可观测的假实现。
type mcpConn interface {
	LoadTools(ctx context.Context, remoteNames []string, headers map[string]string) ([]tool.BaseTool, error)
	Close() error
}

// mcpDialer 是 MCP 连接器的装配层抽象：生产实现委托 internal/ai/tools/mcp，测试注入假实现，
// 使 Store.LoadTools 可在不触碰真实网络的前提下被单测覆盖。
type mcpDialer interface {
	Dial(ctx context.Context, cfg mcp.ConnConfig) (mcpConn, error)
}

// realMCPDialer 是 mcpDialer 的生产实现，委托 internal/ai/tools/mcp（MCP SDK 在本仓库内的唯一
// 落点）；本包因此不直接引用 MCP SDK。失败时返回英文错误且不泄漏已建立的连接。
type realMCPDialer struct{}

// Dial 建立一条 MCP 会话。
func (realMCPDialer) Dial(ctx context.Context, cfg mcp.ConnConfig) (mcpConn, error) {
	conn, err := mcp.DialConn(ctx, cfg)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return conn, nil
}

// ToolSet 是一次工具装配的产出：已构建的工具、与之一一对应的 schema 清单、本次拨号建立的
// MCP 会话关闭句柄，以及按服务器粒度降级时收集的英文告警。Tools 与 Infos 顺序一致且恒为非
// nil（空清单是正常结果），Warnings 交由调用方记录。会话句柄由 ToolSet 持有，调用方在快照
// 不再使用时经 Close 释放。
type ToolSet struct {
	// Tools 已构建的 eino 工具（顺序与 Infos 相同，恒非 nil）
	Tools []tool.BaseTool
	// Infos 供模型选择的 schema 清单（顺序与 Tools 相同，恒非 nil）；client 来源工具只并入 schema、
	// 不构建执行器，故其条目多于 Tools——调用方须按来源区分，不得假定二者等长
	Infos []*schema.ToolInfo
	// Warnings 按服务器/工具粒度降级时收集的英文告警，供调用方记录（可为 nil）
	Warnings []string

	// conns 本次拨号建立的 MCP 会话句柄（凭据材料不在此处，仅关闭句柄）
	conns []mcpConn
	// closed 标记是否已关闭，保证 Close 幂等
	closed bool
}

// Close 关闭本次装配建立的全部 MCP 会话并把错误合并返回；幂等（重复调用或空清单均返回 nil）。
func (ts *ToolSet) Close() error {
	if ts == nil || ts.closed {
		return nil
	}
	ts.closed = true
	errs := make([]error, 0, len(ts.conns))
	for _, conn := range ts.conns {
		if conn == nil {
			continue
		}
		if err := conn.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// warnf 追加一条格式化后的英文降级告警。
func (ts *ToolSet) warnf(format string, args ...any) {
	ts.Warnings = append(ts.Warnings, fmt.Sprintf(format, args...))
}

// LoadTools 把 llm_tools 中已启用的行装配为运行时可消费的 eino 工具集：内置工具经
// internal/ai/tools 按名解析，MCP 工具经其所属服务器连接按 remote_name 取回。装配按服务器
// 粒度降级——某服务器的连接、握手或工具解析失败只跳过该服务器的全部工具并记入 Warnings，
// 绝不上抛；仅「读 llm_tools 表失败」这类数据库错误才返回 error。同服务器只拨号一次，本次
// 建立的会话句柄由返回的 ToolSet 持有，调用方（Runtime）在快照被替换后关闭。行按 id 升序处理，
// 保证工具与 schema 顺序确定（避免 prompt/schema 抖动）。logger 注入给内置工具的实现。
func (s *Store) LoadTools(ctx context.Context, logger *logging.Logger) (*ToolSet, error) {
	rows, err := s.loadEnabledTools(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	set := &ToolSet{Tools: []tool.BaseTool{}, Infos: []*schema.ToolInfo{}}
	ordered, byServer := s.groupMCPRows(set, rows)
	for _, group := range ordered {
		s.dialMCPGroup(ctx, set, group)
	}
	for i := range rows {
		row := &rows[i]
		switch row.Source {
		case ai.ToolSourceBuiltin:
			s.appendBuiltinTool(ctx, set, logger, row)
		case ai.ToolSourceMCP:
			s.appendMCPTool(ctx, set, row, byServer)
		case ai.ToolSourceClient:
			s.appendClientTool(ctx, set, row)
		default:
			set.warnf("ai: tool %q has an unknown source %q and is skipped", row.Name, row.Source)
		}
	}
	return set, nil
}

// loadEnabledTools 读取 llm_tools 中 enabled=true 的行，预加载所属 MCP 服务器并按 id 升序。
// 这是装配流程中唯一可能返回数据库错误的一步。
func (s *Store) loadEnabledTools(ctx context.Context) ([]ai.LlmTool, error) {
	var rows []ai.LlmTool
	if err := s.db.WithContext(ctx).
		Preload("McpServer").
		Where("enabled = ?", true).
		Order("id").
		Find(&rows).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return rows, nil
}

// mcpGroup 是一台已启用 MCP 服务器与其待装配的工具行。
type mcpGroup struct {
	server *ai.McpServer
	rows   []*ai.LlmTool
	tools  map[string]tool.BaseTool // remote_name → 已取回的工具（拨号/取工具失败时为空）
	failed bool                     // 该服务器的连接或取工具整体失败（其工具整体跳过）
}

// groupMCPRows 预扫一遍行：把 source=mcp 的行按所属服务器分组（同服务器只拨号一次，返回按首次
// 出现顺序排列的分组），并在分组阶段就地剔除非法行——服务器指针/外键为空或 remote_name 为空
// 视为数据非法，记英文告警并跳过该行；服务器未启用是管理员的显式选择，整体跳过且不告警。
func (s *Store) groupMCPRows(set *ToolSet, rows []ai.LlmTool) ([]*mcpGroup, map[uint]*mcpGroup) {
	ordered := make([]*mcpGroup, 0)
	byServer := make(map[uint]*mcpGroup)
	for i := range rows {
		row := &rows[i]
		if row.Source != ai.ToolSourceMCP {
			continue
		}
		if row.McpServerID == nil || row.McpServer == nil {
			set.warnf("ai: mcp tool %q has no valid owning server and is skipped", row.Name)
			continue
		}
		if !row.McpServer.Enabled {
			continue // 服务器未启用是显式选择：整体跳过且不告警
		}
		if row.RemoteName == "" {
			set.warnf("ai: mcp tool %q has an empty remote name and is skipped", row.Name)
			continue
		}
		id := *row.McpServerID
		group, ok := byServer[id]
		if !ok {
			group = &mcpGroup{server: row.McpServer}
			byServer[id] = group
			ordered = append(ordered, group)
		}
		group.rows = append(group.rows, row)
	}
	return ordered, byServer
}

// dialMCPGroup 拨号一台服务器并取回其全部工具（同服务器只拨号一次）：先解码请求头明文（为空或
// JSON 非法都不阻断，非法时告警并按「无请求头」继续，且绝不把明文写进日志），再按传输方式/URL/
// 超时建连；连接或取工具的任一步失败即记英文告警（含服务器名与错误、不含请求头）并标记该组失败，
// 其全部工具在第二遍整体跳过。成功建立的会话句柄追加到 ToolSet，随快照关闭。
func (s *Store) dialMCPGroup(ctx context.Context, set *ToolSet, group *mcpGroup) {
	server := group.server
	headers, err := decodeHeaders(server.HeadersData())
	if err != nil {
		set.warnf("ai: MCP server %q has invalid request headers and is used without headers: %v", server.Name, err)
	}
	remoteNames := make([]string, 0, len(group.rows))
	for _, row := range group.rows {
		remoteNames = append(remoteNames, row.RemoteName)
	}
	conn, err := s.dialer.Dial(ctx, mcp.ConnConfig{
		Transport: mcp.Transport(server.Transport),
		URL:       server.URL,
		Headers:   headers,
		Timeout:   time.Duration(server.TimeoutSeconds) * time.Second,
	})
	if err != nil {
		set.warnf("ai: MCP server %q is unreachable, its tools are skipped: %v", server.Name, err)
		group.failed = true
		return
	}
	set.conns = append(set.conns, conn)
	remoteTools, err := conn.LoadTools(ctx, remoteNames, headers)
	if err != nil {
		set.warnf("ai: MCP server %q failed to load tools, its tools are skipped: %v", server.Name, err)
		group.failed = true
		return
	}
	index := make(map[string]tool.BaseTool, len(remoteTools))
	for _, built := range remoteTools {
		if built == nil {
			continue
		}
		info, err := built.Info(ctx)
		if err != nil {
			set.warnf("ai: a tool of MCP server %q has no info and is ignored: %v", server.Name, err)
			continue
		}
		index[info.Name] = built
	}
	group.tools = index
}

// appendBuiltinTool 经 internal/ai/tools 按名解析内置工具并追加其 schema；解析或取 schema 失败
// 只记英文告警并跳过该工具，不上抛（内置实现的缺失属编程缺陷，但按降级处理以免阻断其余工具）。
func (s *Store) appendBuiltinTool(ctx context.Context, set *ToolSet, logger *logging.Logger, row *ai.LlmTool) {
	built, err := tools.Build(ctx, row.Name, tools.Deps{DB: s.db, Logger: logger, JS: s.jsEngine})
	if err != nil {
		set.warnf("ai: builtin tool %q could not be built and is skipped: %v", row.Name, err)
		return
	}
	info, err := built.Info(ctx)
	if err != nil {
		set.warnf("ai: builtin tool %q has no info and is skipped: %v", row.Name, err)
		return
	}
	set.Tools = append(set.Tools, built)
	set.Infos = append(set.Infos, info)
}

// appendMCPTool 按行序取用所属服务器已取回的工具：外键为空（非法行）、服务器未启用或该服务器
// 连接/取工具整体失败时直接跳过（前两者已在分组阶段处理或属显式选择）；远端已不提供该工具或取
// schema 失败则记英文告警并跳过该行。
func (s *Store) appendMCPTool(ctx context.Context, set *ToolSet, row *ai.LlmTool, byServer map[uint]*mcpGroup) {
	if row.McpServerID == nil {
		return // 非法行已在分组阶段告警
	}
	group, ok := byServer[*row.McpServerID]
	if !ok || group.failed {
		return // 服务器未启用、行非法或该服务器整体失败：已在分组阶段处理
	}
	built, ok := group.tools[row.RemoteName]
	if !ok {
		set.warnf("ai: mcp tool %q (remote name %q) is no longer provided by server %q and is skipped", row.Name, row.RemoteName, group.server.Name)
		return
	}
	info, err := built.Info(ctx)
	if err != nil {
		set.warnf("ai: mcp tool %q has no info and is skipped: %v", row.Name, err)
		return
	}
	set.Tools = append(set.Tools, built)
	set.Infos = append(set.Infos, info)
}

// appendClientTool 为 client 来源的工具并入合成 schema：client 工具由前端在用户自身凭据下执行，
// 后端不构建执行器、不拨号任何远端，故只把 client schema 注册表（client_tools.go）里的
// *schema.ToolInfo 追加进 Infos，不往 Tools 追加实现。取不到 schema 属编程缺陷（元信息注册表与
// client schema 注册表漂移），记英文告警并跳过，与 appendBuiltinTool 同风格。
func (s *Store) appendClientTool(_ context.Context, set *ToolSet, row *ai.LlmTool) {
	info, ok := contract.ClientToolInfo(row.Name)
	if !ok {
		set.warnf("ai: client tool %q has no registered schema and is skipped", row.Name)
		return
	}
	set.Infos = append(set.Infos, info)
}

// decodeHeaders 把密文列解出的明文请求头解析为 map[string]string；为空返回 nil（表示无请求头），
// JSON 非法返回固定的英文错误——错误文本不携带原始明文，避免凭据经错误日志泄漏。
func decodeHeaders(raw []byte) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var headers map[string]string
	if err := json.Unmarshal(raw, &headers); err != nil {
		return nil, errors.New("the request headers are not a valid JSON object")
	}
	if len(headers) == 0 {
		return nil, nil
	}
	return headers, nil
}
