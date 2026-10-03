package ai

import (
	// 标准库
	"context"
	"strconv"
	"strings"
	"testing"

	// 第三方
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	// 项目内部
	"zhongkui.com/zhongkui/internal/ai/tools/mcp"
	"zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// fakeTool 是测试用 eino 工具：Info 直接返回给定工具名（模拟内置/MCP 工具的 schema 来源）。
type fakeTool struct {
	name string
}

// Info 返回仅携带工具名的 schema。
func (t *fakeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: t.name}, nil
}

// fakeMCPConn 是可观测的假 MCP 会话：记录 Close 调用次数与最近一次取工具的名单。
type fakeMCPConn struct {
	tools      map[string]tool.BaseTool // remote name → tool
	loadErr    error                    // 非 nil 时取工具失败（模拟 tools/list 失败）
	closeErr   error                    // 非 nil 时 Close 返回该错误（模拟单条会话关闭失败）
	closeCount int
	lastNames  []string
}

// LoadTools 按 remoteNames 从预置工具表取回存在的工具。
func (c *fakeMCPConn) LoadTools(_ context.Context, remoteNames []string, _ map[string]string) ([]tool.BaseTool, error) {
	c.lastNames = remoteNames
	if c.loadErr != nil {
		return nil, c.loadErr
	}
	out := make([]tool.BaseTool, 0, len(remoteNames))
	for _, name := range remoteNames {
		if built, ok := c.tools[name]; ok {
			out = append(out, built)
		}
	}
	return out, nil
}

// Close 记录一次关闭并返回预置的错误（默认 nil）。
func (c *fakeMCPConn) Close() error {
	c.closeCount++
	return c.closeErr
}

// fakeDialer 是假 MCP 连接器：按 URL 返回预置会话或预置错误，并记录被拨号的 URL。
type fakeDialer struct {
	conns    map[string]*fakeMCPConn
	failures map[string]error
	dialed   []string
}

// Dial 按 URL 返回预置结果（失败优先于成功）。
func (d *fakeDialer) Dial(_ context.Context, cfg mcp.ConnConfig) (mcpConn, error) {
	d.dialed = append(d.dialed, cfg.URL)
	if err, ok := d.failures[cfg.URL]; ok {
		return nil, err
	}
	conn, ok := d.conns[cfg.URL]
	if !ok {
		return nil, errors.Newf("fake dialer: no connection configured for %q", cfg.URL)
	}
	return conn, nil
}

// newTestToolDB 构造工具装配测试用的 sqlite 内存库并迁移 mcp_servers / llm_tools 两表。
func newTestToolDB(t *testing.T) *database.DB {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("gorm.Open(sqlite) error = %v", err)
	}
	if err := gdb.AutoMigrate(&ai.McpServer{}, &ai.LlmTool{}); err != nil {
		t.Fatalf("AutoMigrate error = %v", err)
	}
	return gdb
}

// seedMCPServer 插入一台 MCP 服务器并返回其记录（HTTP headers 留空以避开加解密链路）。
func seedMCPServer(t *testing.T, db *database.DB, name, url string, enabled bool) *ai.McpServer {
	t.Helper()
	server := &ai.McpServer{
		Name:           name,
		Transport:      string(mcp.TransportStreamableHTTP),
		URL:            url,
		TimeoutSeconds: 5,
		Enabled:        enabled,
	}
	if err := db.Create(server).Error; err != nil {
		t.Fatalf("seed mcp server %q error = %v", name, err)
	}
	// Enabled 列声明了 default:true，Create 会因零值（false）回落到列默认值，故显式回写一次，
	// 使「禁用服务器」的测试夹具真的落库为 false。
	if err := db.Model(server).Update("enabled", enabled).Error; err != nil {
		t.Fatalf("seed mcp server %q enabled error = %v", name, err)
	}
	return server
}

// seedTool 插入一行 llm_tools 并返回其记录。
func seedTool(t *testing.T, db *database.DB, name string, source ai.ToolSource, serverID *uint, remoteName string, enabled bool) *ai.LlmTool {
	t.Helper()
	row := &ai.LlmTool{
		Name:        name,
		Source:      source,
		McpServerID: serverID,
		RemoteName:  remoteName,
		Enabled:     enabled,
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("seed tool %q error = %v", name, err)
	}
	return row
}

// toolNames 取出装配结果中全部工具的 Info 名称（顺序）。
func toolNames(t *testing.T, set *ToolSet) []string {
	t.Helper()
	names := make([]string, 0, len(set.Tools))
	for _, built := range set.Tools {
		info, err := built.Info(context.Background())
		if err != nil {
			t.Fatalf("tool Info error = %v", err)
		}
		names = append(names, info.Name)
	}
	return names
}

// warningsContain 报告是否有告警包含子串。
func warningsContain(warnings []string, substr string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, substr) {
			return true
		}
	}
	return false
}

// containsName 报告名称清单是否精确包含给定名称。
func containsName(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

// TestLoadToolsEnabledAndDegrade 验证工具装配的两个关键契约：只装配 enabled 的行（禁用的工具
// 不出现、禁用的服务器其工具整体跳过且不告警），以及 MCP 端点故障按服务器粒度降级——不可达的
// 服务器只产生英文告警、其余可达服务器的工具照常装配、整体返回 nil 错误。同时覆盖内置工具
// 解析失败降级为告警、远端已不提供某工具时告警跳过，以及空清单非 nil 的顺序与关闭语义。
func TestLoadToolsEnabledAndDegrade(t *testing.T) {
	db := newTestToolDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	a1, a2, a3 := "tool-a1", "tool-a2", "tool-a3"
	remoteA1, remoteA2, remoteA3 := "remote-a1", "remote-a2", "remote-a3"
	// 真实 MCP 工具经 eino 暴露后其 Info.Name 即远端工具名，故假工具的 Info.Name 取 remote_name。
	upstreamA := &fakeMCPConn{tools: map[string]tool.BaseTool{
		remoteA1: &fakeTool{name: remoteA1},
		remoteA2: &fakeTool{name: remoteA2},
	}}
	upstreamD := &fakeMCPConn{tools: map[string]tool.BaseTool{
		"remote-d1": &fakeTool{name: "remote-d1"},
	}}
	store.dialer = &fakeDialer{
		conns: map[string]*fakeMCPConn{
			"http://a.test/mcp": upstreamA,
			"http://d.test/mcp": upstreamD,
		},
		failures: map[string]error{
			"http://c.test/mcp": errors.New("dial timeout"),
		},
	}

	// 行序（id 升序）即装配顺序：builtin（未注册实现，应降级为告警）、a1（启用）、a2（禁用）、
	// b1（所属服务器禁用，静默跳过）、c1（服务器不可达，告警跳过）、a3（远端已不提供，告警跳过）、
	// d1（可达，正常装配）。
	seedTool(t, db, "tool-unregistered-builtin", ai.ToolSourceBuiltin, nil, "", true)
	serverA := seedMCPServer(t, db, "srv-a", "http://a.test/mcp", true)
	seedTool(t, db, a1, ai.ToolSourceMCP, &serverA.ID, remoteA1, true)
	seedTool(t, db, a2, ai.ToolSourceMCP, &serverA.ID, remoteA2, false)
	serverB := seedMCPServer(t, db, "srv-b", "http://b.test/mcp", false)
	seedTool(t, db, "tool-b1", ai.ToolSourceMCP, &serverB.ID, "remote-b1", true)
	serverC := seedMCPServer(t, db, "srv-c", "http://c.test/mcp", true)
	seedTool(t, db, "tool-c1", ai.ToolSourceMCP, &serverC.ID, "remote-c1", true)
	seedTool(t, db, a3, ai.ToolSourceMCP, &serverA.ID, remoteA3, true)
	serverD := seedMCPServer(t, db, "srv-d", "http://d.test/mcp", true)
	seedTool(t, db, "tool-d1", ai.ToolSourceMCP, &serverD.ID, "remote-d1", true)

	set, err := store.LoadTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("LoadTools error = %v, want nil (a degraded MCP endpoint must not fail)", err)
	}
	if set == nil {
		t.Fatal("LoadTools returned a nil ToolSet")
	}
	if set.Tools == nil || set.Infos == nil {
		t.Fatalf("LoadTools returned nil slices: Tools=%v Infos=%v", set.Tools, set.Infos)
	}

	got := toolNames(t, set)
	want := []string{remoteA1, "remote-d1"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("assembled tools = %v, want %v", got, want)
	}
	if len(set.Infos) != len(set.Tools) {
		t.Errorf("Infos length = %d, want %d (aligned with Tools)", len(set.Infos), len(set.Tools))
	}
	for _, absent := range []string{remoteA2, "remote-b1", "remote-c1", remoteA3} {
		if containsName(got, absent) {
			t.Errorf("disabled/unavailable tool %q leaked into the assembled tools", absent)
		}
	}

	// 服务器 b 未被启用：整体跳过且不产生告警；服务器 c 不可达：告警含其名且工具缺席；
	// 内置工具未注册实现：告警降级；a3 远端已不提供：告警。
	if warningsContain(set.Warnings, "srv-b") {
		t.Errorf("a disabled server must be skipped without a warning, warnings = %v", set.Warnings)
	}
	if !warningsContain(set.Warnings, "srv-c") {
		t.Errorf("the unreachable server warning is missing, warnings = %v", set.Warnings)
	}
	if !warningsContain(set.Warnings, "tool-unregistered-builtin") {
		t.Errorf("the builtin resolution warning is missing, warnings = %v", set.Warnings)
	}
	if !warningsContain(set.Warnings, "remote-a3") {
		t.Errorf("the missing remote tool warning is missing, warnings = %v", set.Warnings)
	}
	for _, warning := range set.Warnings {
		if warning == "" || !strings.HasPrefix(warning, "ai: ") {
			t.Errorf("warning %q must be a non-empty English message prefixed by \"ai: \"", warning)
		}
	}

	// 同一服务器只拨号一次；禁用的服务器 b 从不拨号。
	dialer := store.dialer.(*fakeDialer)
	perURL := map[string]int{}
	for _, url := range dialer.dialed {
		perURL[url]++
	}
	if perURL["http://a.test/mcp"] != 1 || perURL["http://d.test/mcp"] != 1 {
		t.Errorf("dial counts = %v, want each reachable server dialed exactly once", perURL)
	}
	if perURL["http://b.test/mcp"] != 0 {
		t.Errorf("the disabled server was dialed %d times, want 0", perURL["http://b.test/mcp"])
	}

	// 可达服务器 a 与 d 的会话句柄由 ToolSet 持有，Close 幂等。
	if err := set.Close(); err != nil {
		t.Errorf("ToolSet.Close error = %v", err)
	}
	if upstreamA.closeCount != 1 || upstreamD.closeCount != 1 {
		t.Errorf("close counts = (%d, %d), want (1, 1)", upstreamA.closeCount, upstreamD.closeCount)
	}
	if err := set.Close(); err != nil {
		t.Errorf("ToolSet.Close (second) error = %v", err)
	}
	if upstreamA.closeCount != 1 {
		t.Errorf("ToolSet.Close is not idempotent: close count = %d, want 1", upstreamA.closeCount)
	}
}

// TestLoadToolsRealDialerUnreachableDegrades 用真实 MCP 适配器（realMCPDialer，经
// internal/ai/tools/mcp 走真实网络）驱动装配层的降级契约：注册一台不可达的启用服务器及其启用工具
// 后，LoadTools 返回 nil 错误、工具清单为空，但 Warnings 含该服务器的英文告警——证明「服务器粒度
// 降级」在真实传输栈上成立，而非仅对假 dialer 成立。
func TestLoadToolsRealDialerUnreachableDegrades(t *testing.T) {
	db := newTestToolDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	store.dialer = realMCPDialer{}

	// 127.0.0.1:1 上无监听进程：握手必然失败（连接被拒），按服务器粒度降级。
	server := seedMCPServer(t, db, "srv-dead", "http://127.0.0.1:1/mcp", true)
	seedTool(t, db, "tool-dead", ai.ToolSourceMCP, &server.ID, "remote-dead", true)

	set, err := store.LoadTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("LoadTools error = %v, want nil (an unreachable MCP server must not fail the assembly)", err)
	}
	if set == nil {
		t.Fatal("LoadTools returned a nil ToolSet")
	}
	if len(set.Tools) != 0 || len(set.Infos) != 0 {
		t.Errorf("assembled %d tools / %d infos, want none (the only server is unreachable)", len(set.Tools), len(set.Infos))
	}
	if !warningsContain(set.Warnings, "srv-dead") {
		t.Errorf("the unreachable-server warning is missing, warnings = %v", set.Warnings)
	}
	if !warningsContain(set.Warnings, "unreachable") {
		t.Errorf("the warning must name the server as unreachable, warnings = %v", set.Warnings)
	}
	for _, warning := range set.Warnings {
		if warning == "" || !strings.HasPrefix(warning, "ai: ") {
			t.Errorf("warning %q must be a non-empty English message prefixed by \"ai: \"", warning)
		}
	}
	if err := set.Close(); err != nil {
		t.Errorf("ToolSet.Close error = %v, want nil", err)
	}
}

// TestLoadToolsNoEnabledRows 验证空清单是正常结果：没有任何启用工具时返回非 nil 且各切片为
// 非 nil 空切片、无告警、Close 安全返回 nil。
func TestLoadToolsNoEnabledRows(t *testing.T) {
	db := newTestToolDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	server := seedMCPServer(t, db, "srv-off", "http://off.test/mcp", true)
	seedTool(t, db, "tool-off", ai.ToolSourceMCP, &server.ID, "remote-off", false)

	set, err := store.LoadTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("LoadTools error = %v", err)
	}
	if set == nil || set.Tools == nil || set.Infos == nil {
		t.Fatalf("LoadTools must return non-nil empty slices, got %+v", set)
	}
	if len(set.Tools) != 0 || len(set.Infos) != 0 || len(set.Warnings) != 0 {
		t.Errorf("LoadTools returned %d tools / %d infos / %d warnings, want all empty", len(set.Tools), len(set.Infos), len(set.Warnings))
	}
	if err := set.Close(); err != nil {
		t.Errorf("ToolSet.Close error = %v, want nil", err)
	}
}

// TestCloseToolsMultiServerClosesAllSessions 固化多 MCP 服务器场景下快照关闭会话的行为：装配
// 建立的句柄按服务器一条累积，CloseTools 逐条关闭全部句柄（不因某条失败而中断后续），仅把失败
// 错误合并返回；其中「拨号成功但取工具失败」的服务器其句柄已登记、同样被关闭（无泄漏），而拨号
// 失败的服务器没有句柄可关。同时固化该函数的幂等契约：首次关闭的错误不会在重复调用时重现。
func TestCloseToolsMultiServerClosesAllSessions(t *testing.T) {
	db := newTestToolDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	// 四台可达服务器各一条句柄，其中两台关闭失败、一台取工具失败（句柄仍应被关闭）；
	// 另有一台拨号失败（不产生句柄）。
	connOK := &fakeMCPConn{tools: map[string]tool.BaseTool{"remote-1": &fakeTool{name: "remote-1"}}}
	connCloseErr2 := &fakeMCPConn{
		tools:    map[string]tool.BaseTool{"remote-2": &fakeTool{name: "remote-2"}},
		closeErr: errors.New("close failure of srv-2"),
	}
	connCloseErr3 := &fakeMCPConn{
		tools:    map[string]tool.BaseTool{"remote-3": &fakeTool{name: "remote-3"}},
		closeErr: errors.New("close failure of srv-3"),
	}
	connLoadErr := &fakeMCPConn{loadErr: errors.New("tools/list failed")}
	store.dialer = &fakeDialer{
		conns: map[string]*fakeMCPConn{
			"http://s1.test/mcp": connOK,
			"http://s2.test/mcp": connCloseErr2,
			"http://s3.test/mcp": connCloseErr3,
			"http://s4.test/mcp": connLoadErr,
		},
		failures: map[string]error{
			"http://s5.test/mcp": errors.New("dial refused"),
		},
	}

	servers := []struct{ name, url string }{
		{"srv-1", "http://s1.test/mcp"},
		{"srv-2", "http://s2.test/mcp"},
		{"srv-3", "http://s3.test/mcp"},
		{"srv-4", "http://s4.test/mcp"},
		{"srv-5", "http://s5.test/mcp"},
	}
	for i, s := range servers {
		server := seedMCPServer(t, db, s.name, s.url, true)
		seedTool(t, db, "tool-"+s.name, ai.ToolSourceMCP, &server.ID, "remote-"+strconv.Itoa(i+1), true)
	}

	set, err := store.LoadTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("LoadTools error = %v, want nil", err)
	}
	// 句柄按服务器一条累积：四台可达服务器 → 四条（拨号失败的 srv-5 不产生句柄）。
	if got := len(set.conns); got != 4 {
		t.Fatalf("assembled %d session handles, want 4 (one per reachable server)", got)
	}
	// srv-4 取工具失败 → 其工具被跳过，但仍持有句柄；srv-5 不可达 → 告警。
	got := toolNames(t, set)
	want := []string{"remote-1", "remote-2", "remote-3"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("assembled tools = %v, want %v (srv-4 failed tools/list, srv-5 is unreachable)", got, want)
	}
	if !warningsContain(set.Warnings, "srv-4") || !warningsContain(set.Warnings, "srv-5") {
		t.Errorf("warnings = %v, want both srv-4 (tools/list) and srv-5 (unreachable)", set.Warnings)
	}

	// 经 Snapshot 关闭（生产路径）：全部四条句柄都被关闭一次。
	snap := emptySnapshot()
	snap.mountTools(set)
	err = snap.CloseTools()
	if err != nil {
		// 两条关闭失败的错误应被合并返回，且互不吞没。
		for _, want := range []string{"close failure of srv-2", "close failure of srv-3"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("CloseTools error = %v, want it to carry %q", err, want)
			}
		}
	} else {
		t.Error("CloseTools error = nil, want the joined close failures of srv-2 and srv-3")
	}
	for name, conn := range map[string]*fakeMCPConn{
		"srv-1 (ok)":          connOK,
		"srv-2 (close error)": connCloseErr2,
		"srv-3 (close error)": connCloseErr3,
		"srv-4 (load error)":  connLoadErr,
	} {
		if conn.closeCount != 1 {
			t.Errorf("%s session close count = %d, want 1 (a failing close must not spare the others)", name, conn.closeCount)
		}
	}

	// 幂等：重复关闭返回 nil（首次的错误不再重现）且不再触发任何会话关闭。
	if err := snap.CloseTools(); err != nil {
		t.Errorf("CloseTools (second) error = %v, want nil (idempotent)", err)
	}
	if connOK.closeCount != 1 || connCloseErr2.closeCount != 1 || connCloseErr3.closeCount != 1 || connLoadErr.closeCount != 1 {
		t.Errorf("close counts after the second call = (%d, %d, %d, %d), want all 1",
			connOK.closeCount, connCloseErr2.closeCount, connCloseErr3.closeCount, connLoadErr.closeCount)
	}
}

// TestCloseToolsNilSafeAndIdempotent 验证 Snapshot.CloseTools 的零值安全：nil 接收者、未挂载
// 工具的空快照、以及无连接的工具集都安全返回 nil。
func TestCloseToolsNilSafeAndIdempotent(t *testing.T) {
	var nilSnapshot *Snapshot
	if err := nilSnapshot.CloseTools(); err != nil {
		t.Errorf("(*Snapshot)(nil).CloseTools() error = %v, want nil", err)
	}
	if err := emptySnapshot().CloseTools(); err != nil {
		t.Errorf("emptySnapshot().CloseTools() error = %v, want nil", err)
	}
	if err := (&ToolSet{}).Close(); err != nil {
		t.Errorf("(&ToolSet{}).Close() error = %v, want nil", err)
	}
	if tools := nilSnapshot.Tools(); tools == nil || len(tools) != 0 {
		t.Errorf("(*Snapshot)(nil).Tools() = %v, want a non-nil empty slice", tools)
	}
	if infos := emptySnapshot().ToolInfos(); infos == nil || len(infos) != 0 {
		t.Errorf("emptySnapshot().ToolInfos() = %v, want a non-nil empty slice", infos)
	}
}

// TestSwapStateClosesReplacedConnections 验证连接生命周期：swapState 返回被替换掉的旧装配态，
// 关闭旧快照即关闭其持有的 MCP 会话且幂等，同时确认 mountTools 暴露的工具与 schema 只读副本。
func TestSwapStateClosesReplacedConnections(t *testing.T) {
	conn := &fakeMCPConn{tools: map[string]tool.BaseTool{}}
	oldSet := &ToolSet{
		Tools: []tool.BaseTool{&fakeTool{name: "t1"}},
		Infos: []*schema.ToolInfo{{Name: "t1"}},
		conns: []mcpConn{conn},
	}
	oldSnap := emptySnapshot()
	oldSnap.mountTools(oldSet)

	if got := oldSnap.Tools(); len(got) != 1 {
		t.Fatalf("mountTools exposed %d tools, want 1", len(got))
	}
	if got := oldSnap.ToolInfos(); len(got) != 1 || got[0].Name != "t1" {
		t.Fatalf("mountTools exposed infos = %v, want one named t1", got)
	}
	// 访问器返回副本：改动返回值不影响后续读取。
	exposed := oldSnap.Tools()
	exposed[0] = nil
	if again := oldSnap.Tools(); again[0] == nil {
		t.Error("Snapshot.Tools() leaks its internal slice (a caller's mutation was observed)")
	}

	oldState := &runtimeState{snapshot: oldSnap, comps: &Components{}}
	nextState := &runtimeState{snapshot: emptySnapshot(), comps: &Components{}}
	rt := &Runtime{}
	if got := rt.swapState(oldState); got != nil {
		t.Errorf("first swapState returned %v, want nil", got)
	}
	replaced := rt.swapState(nextState)
	if replaced != oldState {
		t.Fatalf("swapState returned %p, want the replaced old state %p", replaced, oldState)
	}
	if err := replaced.snapshot.CloseTools(); err != nil {
		t.Errorf("CloseTools on the replaced snapshot error = %v", err)
	}
	if conn.closeCount != 1 {
		t.Errorf("replaced session close count = %d, want 1", conn.closeCount)
	}
	if err := replaced.snapshot.CloseTools(); err != nil {
		t.Errorf("CloseTools (second) error = %v", err)
	}
	if conn.closeCount != 1 {
		t.Errorf("CloseTools is not idempotent: close count = %d, want 1", conn.closeCount)
	}
}
