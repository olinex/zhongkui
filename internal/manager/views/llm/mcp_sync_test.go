package llm

import (
	// stdlib
	"testing"

	// third-party
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	// internal
	"zhongkui.com/zhongkui/internal/ai"
	mcptool "zhongkui.com/zhongkui/internal/ai/tools/mcp"
	core "zhongkui.com/zhongkui/internal/models/core"
	pkgllm "zhongkui.com/zhongkui/internal/pkg/llm"
)

// newSyncTestDB 构造工具同步对账测试用的 sqlite 内存库并迁移 mcp_servers / llm_tools 两表。
// newSyncTestDB builds an in-memory sqlite DB for the tool-sync reconciliation tests and migrates
// the mcp_servers / llm_tools tables.
func newSyncTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("gorm.Open(sqlite) error = %v", err)
	}
	if err := db.AutoMigrate(&core.McpServer{}, &core.LlmTool{}); err != nil {
		t.Fatalf("AutoMigrate error = %v", err)
	}
	return db
}

// seedSyncServer 插入一台 MCP 服务器并返回其记录。
// seedSyncServer inserts one MCP server and returns its record.
func seedSyncServer(t *testing.T, db *gorm.DB, name string) *core.McpServer {
	t.Helper()
	server := &core.McpServer{Name: name, Transport: string(mcptool.TransportStreamableHTTP), URL: "http://" + name + ".test/mcp", TimeoutSeconds: 5, Enabled: true}
	if err := db.Create(server).Error; err != nil {
		t.Fatalf("seed server %q error = %v", name, err)
	}
	return server
}

// seedSyncTool 插入一行 llm_tools 并返回其记录。
// seedSyncTool inserts one llm_tools row and returns its record.
func seedSyncTool(t *testing.T, db *gorm.DB, name string, source pkgllm.ToolSource, serverID *uint, remoteName, description string, enabled bool) *core.LlmTool {
	t.Helper()
	row := &core.LlmTool{Name: name, Source: source, McpServerID: serverID, RemoteName: remoteName, Description: description, Enabled: enabled}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("seed tool %q error = %v", name, err)
	}
	return row
}

// TestReconcileMCPTools_PreservesEnabledRefreshesAndCleans 覆盖 spec「重复同步保留启用状态并
// 清理失效工具」场景：远端既有工具 A 保留 enabled 并刷新 description，新增工具 B 以 enabled=false
// 落库，远端已移除的工具 C 其行被删除，返回的计数为 (created=1, updated=1, removed=1)。
// TestReconcileMCPTools_PreservesEnabledRefreshesAndCleans covers the spec scenario "repeated sync
// preserves the enabled flag and cleans up stale tools": the pre-existing remote tool A keeps its
// enabled flag and gets a refreshed description, the new tool B lands with enabled=false, and the
// row of the tool C the remote no longer provides is deleted, with counts (1, 1, 1).
func TestReconcileMCPTools_PreservesEnabledRefreshesAndCleans(t *testing.T) {
	db := newSyncTestDB(t)
	server := seedSyncServer(t, db, "srv-a")
	seedSyncTool(t, db, "tool-a", pkgllm.ToolSourceMCP, &server.ID, "tool-a", "old description", true)
	seedSyncTool(t, db, "tool-c", pkgllm.ToolSourceMCP, &server.ID, "tool-c", "stale description", true)

	out, err := reconcileMCPTools(db, server, []ai.MCPRemoteTool{
		{Name: "tool-a", Description: "new description"},
		{Name: "tool-b", Description: "brand new"},
	})
	if err != nil {
		t.Fatalf("reconcileMCPTools error = %v", err)
	}
	if out.Created != 1 || out.Updated != 1 || out.Removed != 1 {
		t.Errorf("counts = (created=%d, updated=%d, removed=%d), want (1, 1, 1)", out.Created, out.Updated, out.Removed)
	}

	var a core.LlmTool
	if err := db.Where("name = ?", "tool-a").First(&a).Error; err != nil {
		t.Fatalf("tool-a lookup error = %v", err)
	}
	if !a.Enabled {
		t.Error("tool-a enabled = false, want true (an existing tool keeps its enabled flag)")
	}
	if a.Description != "new description" {
		t.Errorf("tool-a description = %q, want the refreshed one", a.Description)
	}

	var b core.LlmTool
	if err := db.Where("name = ?", "tool-b").First(&b).Error; err != nil {
		t.Fatalf("tool-b lookup error = %v", err)
	}
	if b.Enabled {
		t.Error("tool-b enabled = true, want false (a newly discovered tool is disabled)")
	}
	if b.Source != pkgllm.ToolSourceMCP || b.McpServerID == nil || *b.McpServerID != server.ID || b.RemoteName != "tool-b" {
		t.Errorf("tool-b row = %+v, want source=mcp with the owning server and remote_name", b)
	}

	var count int64
	if err := db.Model(&core.LlmTool{}).Where("name = ?", "tool-c").Count(&count).Error; err != nil {
		t.Fatalf("tool-c count error = %v", err)
	}
	if count != 0 {
		t.Errorf("tool-c rows = %d, want 0 (a tool the remote no longer provides is removed)", count)
	}
}

// TestReconcileMCPTools_RejectsBuiltinNameCollision 覆盖内置工具不可被 MCP 同步占用：新工具名与
// 既有（内置）工具重名时返回英文错误并回滚，库内行数不变。
// TestReconcileMCPTools_RejectsBuiltinNameCollision covers that a builtin name cannot be taken over
// by an MCP sync: a colliding new tool name returns an English error, the transaction rolls back and
// the row count is unchanged.
func TestReconcileMCPTools_RejectsBuiltinNameCollision(t *testing.T) {
	db := newSyncTestDB(t)
	server := seedSyncServer(t, db, "srv-a")
	seedSyncTool(t, db, "shared", pkgllm.ToolSourceBuiltin, nil, "", "", true)

	if _, err := reconcileMCPTools(db, server, []ai.MCPRemoteTool{{Name: "shared"}}); err == nil {
		t.Fatal("reconcileMCPTools error = nil, want an English collision error")
	}
	var count int64
	if err := db.Model(&core.LlmTool{}).Count(&count).Error; err != nil {
		t.Fatalf("count error = %v", err)
	}
	if count != 1 {
		t.Errorf("llm_tools rows = %d, want 1 (the transaction must roll back)", count)
	}
}

// TestReconcileMCPTools_RejectsConsistencyViolation 覆盖一致性约束校验：库内存在 source='mcp' 但
// mcp_server_id 为空（或反之）的脏行时对账返回英文错误。
// TestReconcileMCPTools_RejectsConsistencyViolation covers the consistency invariant check: a dirty
// row with source='mcp' but a null mcp_server_id (or the reverse) makes the reconciliation return an
// English error.
func TestReconcileMCPTools_RejectsConsistencyViolation(t *testing.T) {
	db := newSyncTestDB(t)
	server := seedSyncServer(t, db, "srv-a")
	// 绕过创建期约束直接落一行违反不变量的脏数据（source=mcp 但无 mcp_server_id）。
	if err := db.Create(&core.LlmTool{Name: "dirty", Source: pkgllm.ToolSourceMCP, RemoteName: "dirty"}).Error; err != nil {
		t.Fatalf("seed dirty row error = %v", err)
	}
	if _, err := reconcileMCPTools(db, server, []ai.MCPRemoteTool{{Name: "clean"}}); err == nil {
		t.Fatal("reconcileMCPTools error = nil, want an English consistency error")
	}
}

// TestReconcileMCPTools_RejectsEmptyRemoteName 覆盖远端工具名为空时报英文错误并回滚。
// TestReconcileMCPTools_RejectsEmptyRemoteName covers that an empty remote tool name yields an
// English error and a rollback.
func TestReconcileMCPTools_RejectsEmptyRemoteName(t *testing.T) {
	db := newSyncTestDB(t)
	server := seedSyncServer(t, db, "srv-a")
	if _, err := reconcileMCPTools(db, server, []ai.MCPRemoteTool{{Name: "   "}}); err == nil {
		t.Fatal("reconcileMCPTools error = nil, want an English error for an empty remote name")
	}
}
