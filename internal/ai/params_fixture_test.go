package ai

import (
	// 标准库
	"os"
	"regexp"
	"strings"
	"testing"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/llm"
)

// fixturePath 是 Agent 参数出厂值的种子文件路径（相对本包目录）。
const fixturePath = "../migrations/core/002_fixtures.sql"

// paramRowPattern 匹配 002_fixtures.sql 中 agent_params 的 VALUES 行：键 + 值，值既可为
// 美元引用（$$...$$，提示词正文）也可为单引号字面量。
var paramRowPattern = regexp.MustCompile(`(?s)\(\s*'([^']+)',\s*(?:\$\$(.*?)\$\$|'([^']*)')\s*\)`)

// TestAgentParamsFixtureMatchesRegistry 守卫「代码内参数注册表」与「migrate fixture」不漂移：
// fixture 必须为注册表声明的每个键（且仅为这些键）种子化出厂值，且字面量可按该键声明的类型
// 解析。新增参数若忘记补 fixture，启动期强校验会中止服务，此测试在更早的构建阶段暴露问题。
func TestAgentParamsFixtureMatchesRegistry(t *testing.T) {
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read %s error = %v", fixturePath, err)
	}
	section := agentParamsSection(t, string(raw))
	literals := make(map[string]string)
	for _, m := range paramRowPattern.FindAllStringSubmatch(section, -1) {
		key := m[1]
		value := m[2]
		if value == "" {
			value = m[3]
		}
		if _, dup := literals[key]; dup {
			t.Errorf("fixture declares key %q more than once", key)
		}
		literals[key] = value
	}
	if len(literals) == 0 {
		t.Fatal("no agent parameter row found in the fixture")
	}
	for _, spec := range llm.Specs {
		value, ok := literals[spec.Key]
		if !ok {
			t.Errorf("fixture misses the parameter %q declared by the registry", spec.Key)
			continue
		}
		if value == "" {
			t.Errorf("fixture seeds the parameter %q with an empty value", spec.Key)
			continue
		}
		if err := llm.ValidateValue(spec, value); err != nil {
			t.Errorf("fixture value of %q does not match its kind: %v", spec.Key, err)
		}
		delete(literals, spec.Key)
	}
	for key := range literals {
		t.Errorf("fixture seeds the parameter %q which the registry does not declare", key)
	}
}

// agentParamsSection 截取 fixture 中 agent_params 的 INSERT 段（到 ON CONFLICT 结束），
// 使行匹配不受文件其余内容影响。
func agentParamsSection(t *testing.T, raw string) string {
	t.Helper()
	start := strings.Index(raw, "INSERT INTO agent_params")
	if start < 0 {
		t.Fatalf("%s has no INSERT INTO agent_params section", fixturePath)
	}
	rest := raw[start:]
	end := strings.Index(rest, "ON CONFLICT (key) DO NOTHING")
	if end < 0 {
		t.Fatalf("%s: the agent_params INSERT has no ON CONFLICT (key) DO NOTHING clause", fixturePath)
	}
	return rest[:end]
}
