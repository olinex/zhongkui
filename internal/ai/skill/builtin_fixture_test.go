package skill

import (
	// 标准库
	"os"
	"regexp"
	"strings"
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/base"
)

// 内置技能出厂行在 002_fixtures.sql 中的定位与解析规则，与 llm_tools 出厂行的守卫方式同构。
const (
	// builtinSkillFixturePath 是内置技能出厂行的种子文件路径（相对本包目录）。
	builtinSkillFixturePath = "../../migrations/core/002_fixtures.sql"
	// builtinSkillInsertPrefix 内置技能出厂行所在的 agent_skills INSERT 段起始。
	builtinSkillInsertPrefix = "INSERT INTO agent_skills"
	// builtinSkillConflictClause 内置技能出厂行的段结束标记。
	builtinSkillConflictClause = "ON CONFLICT"
)

// builtinSkillRowPattern 匹配 agent_skills 出厂段中系统命名空间的 VALUES 行：列顺序固定为
// (namespace_name, name, description, content, enabled)，首个引号字面量为命名空间，第二个为技能名；
// 其余命名空间的自定义技能行不会被匹配。
var builtinSkillRowPattern = regexp.MustCompile(`\(\s*'` + base.SystemNamespace + `'\s*,\s*'([^']+)'`)

// TestBuiltinSkillFixtureMatchesRegistry 守卫「代码正文注册表」与「fixture 注册行」不漂移：
// agent_skills 系统命名空间出厂行的名称集合与 BuiltinSkills 的键集合双向一致，缺任一侧即失败。
func TestBuiltinSkillFixtureMatchesRegistry(t *testing.T) {
	raw, err := os.ReadFile(builtinSkillFixturePath)
	if err != nil {
		t.Fatalf("read %s error = %v", builtinSkillFixturePath, err)
	}
	fixture := builtinSkillFixtureNames(string(raw))
	registry := BuiltinSkillNames()

	seen := make(map[string]struct{}, len(registry))
	for _, name := range registry {
		if _, dup := seen[name]; dup {
			t.Errorf("code registry declares builtin skill %q more than once", name)
		}
		seen[name] = struct{}{}
	}
	fixtureSet := make(map[string]struct{}, len(fixture))
	for _, name := range fixture {
		if _, dup := fixtureSet[name]; dup {
			t.Errorf("fixture declares builtin skill %q more than once", name)
		}
		fixtureSet[name] = struct{}{}
	}
	for name := range seen {
		if _, ok := fixtureSet[name]; !ok {
			t.Errorf("fixture misses the builtin skill %q declared by the code registry", name)
		}
	}
	for name := range fixtureSet {
		if _, ok := seen[name]; !ok {
			t.Errorf("fixture seeds the builtin skill %q which the code registry does not declare", name)
		}
	}
	t.Logf("builtin skills: registry=%d fixture=%d", len(registry), len(fixture))
}

// TestBuiltinSkillFixtureParsing 以伪 fixture 正向证明解析有效：段缺失返回空；段存在时只取
// 系统命名空间的名称，普通命名空间的自定义技能行被忽略。
func TestBuiltinSkillFixtureParsing(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "no section",
			text: "INSERT INTO llm_tools (name, source) VALUES ('alpha', 'builtin');",
			want: nil,
		},
		{
			name: "only system namespace rows",
			text: "INSERT INTO agent_skills (namespace_name, name, description, content, enabled, source) VALUES\n" +
				"    ('system', 'alpha', '内置说明', NULL, true, 'builtin'),\n" +
				"    ('tenant', 'beta', '自定义说明', '', true, 'custom')\n" +
				"ON CONFLICT DO NOTHING;",
			want: []string{"alpha"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := builtinSkillFixtureNames(c.text)
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Fatalf("builtinSkillFixtureNames() = %v, want %v", got, c.want)
			}
		})
	}
}

// builtinSkillFixtureNames 从 fixture 文本的 agent_skills 出厂段中提取系统命名空间的内置技能
// 名称（行序）。段不存在时返回空集合而非 fatal：当前代码注册表为空时该段本就不存在。
func builtinSkillFixtureNames(raw string) []string {
	start := strings.Index(raw, builtinSkillInsertPrefix)
	if start < 0 {
		return nil
	}
	rest := raw[start:]
	if end := strings.Index(rest, builtinSkillConflictClause); end >= 0 {
		rest = rest[:end]
	}
	matches := builtinSkillRowPattern.FindAllStringSubmatch(rest, -1)
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, match[1])
	}
	return names
}
