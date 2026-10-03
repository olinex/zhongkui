package ai

import (
	// 标准库
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	// 项目内部
)

// llmToolsInsertPrefix 与 llmToolsConflictClause 界定 002_fixtures.sql 中 llm_tools 出厂行
// 的 INSERT 段（到 ON CONFLICT 结束），使行匹配不受文件其余内容影响。
const (
	llmToolsInsertPrefix   = "INSERT INTO llm_tools"
	llmToolsConflictClause = "ON CONFLICT"
)

// 工具来源在 llm_tools 出厂行中的字面量（source='builtin' / source='client'）。
const (
	// builtinSourceLiteral 内置工具来源字面量
	builtinSourceLiteral = "'builtin'"
	// clientSourceLiteral 客户端工具来源字面量
	clientSourceLiteral = "'client'"
)

// llmToolRowPattern 匹配 llm_tools 的 VALUES 行：首个单引号字面量为工具名，其余列内容由
// 同一括号内（不含嵌套括号）的文本承载，据其是否含目标来源字面量判定来源。
var llmToolRowPattern = regexp.MustCompile(`(?s)\(\s*'([^']+)'\s*,([^()]*)\)`)

// TestBuiltinToolsFixtureMatchesRegistry 守卫「代码内工具元信息注册表」「migrate fixture」与
// 「client schema 注册表」三者不漂移，按注册表声明的分组分别校验：
//   - builtin 分组：fixture 的 source='builtin' 行与注册表声明双向一致（既有校验不放宽）；
//   - client 分组：fixture 的 source='client' 行与注册表声明双向一致，且每个工具在 client schema
//     注册表（client_tools.go）中有对应条目——client 工具无本地执行器，故以 schema 注册表承担与
//     builtin 实现注册表等价的一致性校验。
//
// 提取逻辑的有效性由 TestBuiltinToolNamesParsing 以一段内联的伪 fixture 正向证明，避免「集合为空
// 而永远通过」。
func TestBuiltinToolsFixtureMatchesRegistry(t *testing.T) {
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read %s error = %v", fixturePath, err)
	}
	builtinFixture := nameSet(t, builtinToolNames(string(raw)))
	clientFixture := nameSet(t, clientToolNames(string(raw)))
	builtinRegistry := registryNamesByGroup("builtin")
	clientRegistry := registryNamesByGroup("client")

	assertNameSetsEqual(t, "builtin", builtinRegistry, builtinFixture)
	assertNameSetsEqual(t, "client", clientRegistry, clientFixture)

	for name := range clientRegistry {
		if _, ok := clientToolInfo(name); !ok {
			t.Errorf("client tool %q declared by the registry has no schema in the client schema registry", name)
		}
	}
	t.Logf("tools: builtin registry=%d fixture=%d, client registry=%d fixture=%d",
		len(builtinRegistry), len(builtinFixture), len(clientRegistry), len(clientFixture))
}

// registryNamesByGroup 返回工具元信息注册表中指定分组的工具名集合。
func registryNamesByGroup(group string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, spec := range ToolSpecs {
		if spec.Group == group {
			out[spec.Name] = struct{}{}
		}
	}
	return out
}

// nameSet 把工具名清单转为集合并校验无重复。
func nameSet(t *testing.T, names []string) map[string]struct{} {
	t.Helper()
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, dup := out[name]; dup {
			t.Errorf("fixture declares tool %q more than once", name)
		}
		out[name] = struct{}{}
	}
	return out
}

// assertNameSetsEqual 断言注册表名称集合与 fixture 名称集合双向一致。
func assertNameSetsEqual(t *testing.T, group string, registry, fixture map[string]struct{}) {
	t.Helper()
	for name := range registry {
		if _, ok := fixture[name]; !ok {
			t.Errorf("fixture misses the %s tool %q declared by the registry", group, name)
		}
	}
	for name := range fixture {
		if _, ok := registry[name]; !ok {
			t.Errorf("fixture seeds the %s tool %q which the registry does not declare", group, name)
		}
	}
}

// TestBuiltinToolNamesParsing 以表驱动子测试正向证明「解析 fixture 文本 → 按来源提取工具名」的
// 纯函数有效：段缺失时返回空、段存在时只取目标来源的行（多行与 SELECT ... FROM (VALUES ...) 两种
// 写法皆覆盖），从而保证上一条守卫测试不是因为集合为空而永远通过。
func TestBuiltinToolNamesParsing(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		extract func(string) []string
		want    []string
	}{
		{
			name:    "no llm_tools section",
			text:    "INSERT INTO agent_params (key, value) VALUES ('a', 'b') ON CONFLICT (key) DO NOTHING;",
			extract: builtinToolNames,
			want:    nil,
		},
		{
			name: "only builtin rows are extracted",
			text: "INSERT INTO llm_tools (name, source, mcp_server_id, remote_name, description, enabled) VALUES\n" +
				"    ('alpha', 'builtin', NULL, '', '', false),\n" +
				"    ('beta', 'mcp', 3, 'remote-beta', 'desc', true)\n" +
				"ON CONFLICT (name) DO NOTHING;",
			extract: builtinToolNames,
			want:    []string{"alpha"},
		},
		{
			name: "select-from-values form",
			text: "INSERT INTO llm_tools (name, source) SELECT v.name, v.source FROM (VALUES\n" +
				"    ('gamma', 'builtin'),\n" +
				"    ('delta', 'mcp')\n" +
				") AS v(name, source) ON CONFLICT (name) DO NOTHING;",
			extract: builtinToolNames,
			want:    []string{"gamma"},
		},
		{
			name: "only client rows are extracted",
			text: "INSERT INTO llm_tools (name, source, mcp_server_id, remote_name, description, enabled) VALUES\n" +
				"    ('alpha', 'builtin', NULL, '', '', true),\n" +
				"    ('epsilon', 'client', NULL, '', '', true)\n" +
				"ON CONFLICT (name) DO NOTHING;",
			extract: clientToolNames,
			want:    []string{"epsilon"},
		},
	}
	for _, c := range cases {
		got := c.extract(c.text)
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%s: extract(...) = %v, want %v", c.name, got, c.want)
		}
	}
}

// builtinToolNames 从 fixture 文本的 llm_tools 出厂行中提取 source='builtin' 的工具名（升序）。
func builtinToolNames(raw string) []string {
	return toolNamesBySource(raw, builtinSourceLiteral)
}

// clientToolNames 从 fixture 文本的 llm_tools 出厂行中提取 source='client' 的工具名（升序）。
func clientToolNames(raw string) []string {
	return toolNamesBySource(raw, clientSourceLiteral)
}

// toolNamesBySource 从 fixture 文本的 llm_tools 出厂行中提取指定来源的工具名（升序）。段不存在时
// 返回空集合而非 fatal：该段仅在「工具清单非空」时出现，故注册表为空时本就缺席；段存在后按来源
// 字面量判定来源。
func toolNamesBySource(raw, sourceLiteral string) []string {
	section := llmToolsSection(raw)
	if section == "" {
		return nil
	}
	names := make([]string, 0)
	for _, m := range llmToolRowPattern.FindAllStringSubmatch(section, -1) {
		if !strings.Contains(m[2], sourceLiteral) {
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
