package ai

import (
	// 标准库
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/llm"
)

// llmToolsInsertPrefix 与 llmToolsConflictClause 界定 002_fixtures.sql 中 llm_tools 出厂行
// 的 INSERT 段（到 ON CONFLICT 结束），使行匹配不受文件其余内容影响。
const (
	llmToolsInsertPrefix   = "INSERT INTO llm_tools"
	llmToolsConflictClause = "ON CONFLICT"
)

// builtinSourceLiteral 是 llm_tools 中内置工具来源的字面量（source='builtin'）。
const builtinSourceLiteral = "'builtin'"

// llmToolRowPattern 匹配 llm_tools 的 VALUES 行：首个单引号字面量为工具名，其余列内容由
// 同一括号内（不含嵌套括号）的文本承载，据其是否含 source='builtin' 字面量判定来源。
var llmToolRowPattern = regexp.MustCompile(`(?s)\(\s*'([^']+)'\s*,([^()]*)\)`)

// TestBuiltinToolsFixtureMatchesRegistry 守卫「代码内内置工具元信息注册表」与「migrate fixture」
// 不漂移：fixture 必须为注册表声明的每个内置工具（且仅为这些工具）种子化出厂行，二者名字集合
// 双向一致。首版两边皆空，本测试在此情形下通过（与 params_fixture_test.go 对空集合 fatal 的
// 判断不同——内置工具清单允许为空）；提取逻辑的有效性由 TestBuiltinToolNamesParsing 以一段
// 内联的伪 fixture 正向证明，避免「集合为空而永远通过」。
func TestBuiltinToolsFixtureMatchesRegistry(t *testing.T) {
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read %s error = %v", fixturePath, err)
	}
	// 002_fixtures.sql 的 llm_tools INSERT 段仅在「内置工具清单非空」时出现：注册表为空时
	// 该段有意缺席，解析器随即返回空集合，故本测试在两边皆空时平凡通过（与 params_fixture_test.go
	// 对空集合 fatal 的处理不同）。注册表一旦声明内置工具，段即必须随之出现，双向断言便会强制
	// 二者与 source='builtin' 精确一致。
	inFixture := make(map[string]struct{})
	for _, name := range builtinToolNames(string(raw)) {
		if _, dup := inFixture[name]; dup {
			t.Errorf("fixture declares builtin tool %q more than once", name)
		}
		inFixture[name] = struct{}{}
	}
	inRegistry := make(map[string]struct{}, len(llm.ToolSpecs))
	for _, spec := range llm.ToolSpecs {
		inRegistry[spec.Name] = struct{}{}
	}
	for name := range inRegistry {
		if _, ok := inFixture[name]; !ok {
			t.Errorf("fixture misses the builtin tool %q declared by the registry", name)
		}
	}
	for name := range inFixture {
		if _, ok := inRegistry[name]; !ok {
			t.Errorf("fixture seeds the builtin tool %q which the registry does not declare", name)
		}
	}
	t.Logf("builtin tools: registry=%d fixture=%d", len(inRegistry), len(inFixture))
}

// TestBuiltinToolNamesParsing 以表驱动子测试正向证明「解析 fixture 文本 → 提取 builtin 工具名」
// 的纯函数有效：段缺失时返回空、段存在时只取 source='builtin' 行（多行与 SELECT ... FROM (
// VALUES ...) 两种写法皆覆盖），从而保证上一条守卫测试不是因为集合为空而永远通过。
func TestBuiltinToolNamesParsing(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "no llm_tools section",
			text: "INSERT INTO agent_params (key, value) VALUES ('a', 'b') ON CONFLICT (key) DO NOTHING;",
			want: nil,
		},
		{
			name: "only builtin rows are extracted",
			text: "INSERT INTO llm_tools (name, source, mcp_server_id, remote_name, description, enabled) VALUES\n" +
				"    ('alpha', 'builtin', NULL, '', '', false),\n" +
				"    ('beta', 'mcp', 3, 'remote-beta', 'desc', true)\n" +
				"ON CONFLICT (name) DO NOTHING;",
			want: []string{"alpha"},
		},
		{
			name: "select-from-values form",
			text: "INSERT INTO llm_tools (name, source) SELECT v.name, v.source FROM (VALUES\n" +
				"    ('gamma', 'builtin'),\n" +
				"    ('delta', 'mcp')\n" +
				") AS v(name, source) ON CONFLICT (name) DO NOTHING;",
			want: []string{"gamma"},
		},
	}
	for _, c := range cases {
		got := builtinToolNames(c.text)
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%s: builtinToolNames(...) = %v, want %v", c.name, got, c.want)
		}
	}
}

// builtinToolNames 从 fixture 文本的 llm_tools 出厂行中提取 source='builtin' 的工具名（升序）。
// 段不存在时返回空集合而非 fatal：该段仅在「内置工具清单非空」时出现，故注册表为空时本就
// 缺席；段存在后按 source='builtin' 字面量判定来源。
func builtinToolNames(raw string) []string {
	section := llmToolsSection(raw)
	if section == "" {
		return nil
	}
	names := make([]string, 0)
	for _, m := range llmToolRowPattern.FindAllStringSubmatch(section, -1) {
		if !strings.Contains(m[2], builtinSourceLiteral) {
			continue
		}
		names = append(names, m[1])
	}
	sort.Strings(names)
	return names
}

// llmToolsSection 截取 fixture 中 llm_tools 的 INSERT 段（到 ON CONFLICT 结束）；不存在时
// 返回空串。
func llmToolsSection(raw string) string {
	start := strings.Index(raw, llmToolsInsertPrefix)
	if start < 0 {
		return ""
	}
	rest := raw[start:]
	end := strings.Index(rest, llmToolsConflictClause)
	if end < 0 {
		return rest
	}
	return rest[:end]
}
