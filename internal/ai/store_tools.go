package ai

import (
	// stdlib
	"context"
	"encoding/json"
	"fmt"
	"time"

	// third-party
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	// internal
	"zhongkui.com/zhongkui/internal/ai/tools"
	"zhongkui.com/zhongkui/internal/ai/tools/mcp"
	coremodel "zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/llm"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// mcpConn 是一次已建立的 MCP 会话在装配层的抽象：按名取回远端工具并关闭会话。生产实现是
// internal/ai/tools/mcp.Connection 的薄包装，测试注入可观测的假实现。
// mcpConn is the assembly-layer abstraction of one established MCP session: it fetches remote
// tools by name and closes the session. The production implementation thinly wraps
// internal/ai/tools/mcp.Connection, while tests inject an observable fake.
type mcpConn interface {
	LoadTools(ctx context.Context, remoteNames []string, headers map[string]string) ([]tool.BaseTool, error)
	Close() error
}

// mcpDialer 是 MCP 连接器的装配层抽象：生产实现委托 internal/ai/tools/mcp，测试注入假实现，
// 使 Store.LoadTools 可在不触碰真实网络的前提下被单测覆盖。
// mcpDialer is the assembly-layer abstraction of the MCP connector: the production implementation
// delegates to internal/ai/tools/mcp and tests inject a fake, letting Store.LoadTools be unit
// tested without touching the real network.
type mcpDialer interface {
	Dial(ctx context.Context, cfg mcp.ConnConfig) (mcpConn, error)
}

// realMCPDialer 是 mcpDialer 的生产实现，委托 internal/ai/tools/mcp（MCP SDK 在本仓库内的唯一
// 落点）；本包因此不直接引用 MCP SDK。失败时返回英文错误且不泄漏已建立的连接。
// realMCPDialer is the production implementation of mcpDialer, delegating to
// internal/ai/tools/mcp (the only place in the repository where the MCP SDK appears); this package
// therefore never references the MCP SDK directly. A failure yields an English error and never
// leaks an already established connection.
type realMCPDialer struct{}

// Dial 建立一条 MCP 会话。
// Dial establishes one MCP session.
func (realMCPDialer) Dial(ctx context.Context, cfg mcp.ConnConfig) (mcpConn, error) {
	conn, err := mcp.DialConn(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// ToolSet 是一次工具装配的产出：已构建的工具、与之一一对应的 schema 清单、本次拨号建立的
// MCP 会话关闭句柄，以及按服务器粒度降级时收集的英文告警。Tools 与 Infos 顺序一致且恒为非
// nil（空清单是正常结果），Warnings 交由调用方记录。会话句柄由 ToolSet 持有，调用方在快照
// 不再使用时经 Close 释放。
// ToolSet is the output of one tool assembly pass: the built tools, the schema list aligned with
// them one to one, the close handles of the MCP sessions dialed this time, and the English
// warnings collected while degrading per server. Tools and Infos share the same order and are
// always non-nil (an empty list is a normal result), and Warnings is for the caller to log. The
// session handles are held by the ToolSet and released by the caller through Close once the
// snapshot is no longer used.
type ToolSet struct {
	// Tools 已构建的 eino 工具（顺序与 Infos 一致，恒非 nil）
	// Tools the built eino tools (same order as Infos, always non-nil)
	Tools []tool.BaseTool
	// Infos 与 Tools 一一对应的 schema 清单（顺序一致，恒非 nil）
	// Infos the schema list aligned one to one with Tools (same order, always non-nil)
	Infos []*schema.ToolInfo
	// Warnings 按服务器/工具粒度降级时收集的英文告警，供调用方记录（可为 nil）
	// Warnings the English warnings collected while degrading per server/tool, for the caller to
	// log (may be nil)
	Warnings []string

	// conns 本次拨号建立的 MCP 会话句柄（凭据材料不在此处，仅关闭句柄）
	// conns the MCP session handles dialed this time (no credential material, only close handles)
	conns []mcpConn
	// closed 标记是否已关闭，保证 Close 幂等
	// closed marks whether it is already closed, keeping Close idempotent
	closed bool
}

// Close 关闭本次装配建立的全部 MCP 会话并把错误合并返回；幂等（重复调用或空清单均返回 nil）。
// Close closes every MCP session established by this assembly and returns the joined errors; it is
// idempotent (a repeated call or an empty list returns nil).
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
// warnf appends one formatted English degradation warning.
func (ts *ToolSet) warnf(format string, args ...any) {
	ts.Warnings = append(ts.Warnings, fmt.Sprintf(format, args...))
}

// LoadTools 把 llm_tools 中已启用的行装配为运行时可消费的 eino 工具集：内置工具经
// internal/ai/tools 按名解析，MCP 工具经其所属服务器连接按 remote_name 取回。装配按服务器
// 粒度降级——某服务器的连接、握手或工具解析失败只跳过该服务器的全部工具并记入 Warnings，
// 绝不上抛；仅「读 llm_tools 表失败」这类数据库错误才返回 error。同服务器只拨号一次，本次
// 建立的会话句柄由返回的 ToolSet 持有，调用方（Runtime）在快照被替换后关闭。行按 id 升序处理，
// 保证工具与 schema 顺序确定（避免 prompt/schema 抖动）。logger 注入给内置工具的实现。
// LoadTools assembles the enabled rows of llm_tools into a runtime-consumable eino tool set:
// builtin tools are resolved by name through internal/ai/tools and MCP tools are fetched by
// remote_name through the connection of their owning server. The assembly degrades per server — a
// connection, handshake or tool-resolution failure of one server only skips that server's tools
// and lands in Warnings, never being returned; only a database error such as a failed llm_tools
// read yields an error. Each server is dialed once, and the sessions established this time are held
// by the returned ToolSet for the caller (Runtime) to close after the snapshot is replaced. Rows
// are processed in ascending id order, keeping the tools and schema deterministic (no prompt/schema
// churn). logger is injected into the builtin tool implementations.
func (s *Store) LoadTools(ctx context.Context, logger *logging.Logger) (*ToolSet, error) {
	rows, err := s.loadEnabledTools(ctx)
	if err != nil {
		return nil, err
	}
	set := &ToolSet{Tools: []tool.BaseTool{}, Infos: []*schema.ToolInfo{}}
	ordered, byServer := s.groupMCPRows(set, rows)
	for _, group := range ordered {
		s.dialMCPGroup(ctx, set, group)
	}
	for i := range rows {
		row := &rows[i]
		switch llm.ToolSource(row.Source) {
		case llm.ToolSourceBuiltin:
			s.appendBuiltinTool(ctx, set, logger, row)
		case llm.ToolSourceMCP:
			s.appendMCPTool(ctx, set, row, byServer)
		default:
			set.warnf("ai: tool %q has an unknown source %q and is skipped", row.Name, row.Source)
		}
	}
	return set, nil
}

// loadEnabledTools 读取 llm_tools 中 enabled=true 的行，预加载所属 MCP 服务器并按 id 升序。
// 这是装配流程中唯一可能返回数据库错误的一步。
// loadEnabledTools reads the enabled=true rows of llm_tools, preloads the owning MCP server and
// orders by id ascending. It is the only step of the assembly that may return a database error.
func (s *Store) loadEnabledTools(ctx context.Context) ([]coremodel.LlmTool, error) {
	var rows []coremodel.LlmTool
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
// mcpGroup is one enabled MCP server with its pending tool rows.
type mcpGroup struct {
	server *coremodel.McpServer
	rows   []*coremodel.LlmTool
	tools  map[string]tool.BaseTool // remote_name → 已取回的工具（拨号/取工具失败时为空）
	failed bool                     // 该服务器的连接或取工具整体失败（其工具整体跳过）
}

// groupMCPRows 预扫一遍行：把 source=mcp 的行按所属服务器分组（同服务器只拨号一次，返回按首次
// 出现顺序排列的分组），并在分组阶段就地剔除非法行——服务器指针/外键为空或 remote_name 为空
// 视为数据非法，记英文告警并跳过该行；服务器未启用是管理员的显式选择，整体跳过且不告警。
// groupMCPRows pre-scans the rows: it groups the source=mcp rows by their owning server (each
// server dialed once; the groups are returned in first-appearance order) and prunes the invalid
// rows in place — a nil server pointer/foreign key or an empty remote_name counts as invalid data
// and is warned and skipped, while a disabled server is an operator's explicit choice and is
// skipped wholesale without a warning.
func (s *Store) groupMCPRows(set *ToolSet, rows []coremodel.LlmTool) ([]*mcpGroup, map[uint]*mcpGroup) {
	ordered := make([]*mcpGroup, 0)
	byServer := make(map[uint]*mcpGroup)
	for i := range rows {
		row := &rows[i]
		if llm.ToolSource(row.Source) != llm.ToolSourceMCP {
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
// dialMCPGroup dials one server and fetches all its tools, dialing each server once: it first
// decodes the plaintext request headers (empty or invalid JSON never blocks — invalid is warned
// and continued without headers, and the plaintext is never written to the log), then connects by
// transport/URL/timeout; a failure in either connecting or fetching tools logs an English warning
// (carrying the server name and the error, never the headers) and marks the group failed so all
// its tools are skipped in the second pass. A successfully established session is appended to the
// ToolSet for closing with the snapshot.
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
// appendBuiltinTool resolves a builtin tool by name through internal/ai/tools and appends its
// schema; a resolution or schema failure only logs an English warning and skips the tool without
// propagating (a missing implementation is a programming defect but is degraded here so the other
// tools are not blocked).
func (s *Store) appendBuiltinTool(ctx context.Context, set *ToolSet, logger *logging.Logger, row *coremodel.LlmTool) {
	built, err := tools.Build(ctx, row.Name, tools.Deps{DB: s.db, Logger: logger})
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
// appendMCPTool resolves the tool fetched for the owning server in row order: it skips silently
// when the foreign key is nil (invalid row), the server is disabled, or the server's
// connect/fetch failed as a whole (the first two were handled in the grouping pass or are an
// explicit choice), while a tool the remote no longer provides or a schema failure logs an English
// warning and skips the row.
func (s *Store) appendMCPTool(ctx context.Context, set *ToolSet, row *coremodel.LlmTool, byServer map[uint]*mcpGroup) {
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

// decodeHeaders 把密文列解出的明文请求头解析为 map[string]string；为空返回 nil（表示无请求头），
// JSON 非法返回固定的英文错误——错误文本不携带原始明文，避免凭据经错误日志泄漏。
// decodeHeaders parses the plaintext request headers (decrypted from the encrypted column) into a
// map[string]string; empty yields nil (meaning no headers) and invalid JSON yields a fixed English
// error whose text never carries the raw plaintext, so credentials can never leak into the log
// through the error text.
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
