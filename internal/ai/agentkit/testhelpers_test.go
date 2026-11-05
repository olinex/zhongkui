package agentkit

import (
	"testing"

	"zhongkuisec.com/zhongkui/internal/ai/config"
)

// testParams 构造单元测试用的行为参数读取器。agentkit 只读取这些键，故用固定值即可，
// 需要覆盖阈值时用 overrides 覆盖。
func testParams(overrides map[string]string) *config.Params {
	values := map[string]string{
		"agent.systemPrompt":               "base system prompt",
		"agent.generalSystemPrompt":        "general system prompt",
		"agent.auditLevelSystemPrompt":     "audit system prompt",
		"agent.nextStepsSystemPrompt":      "next steps system prompt",
		"agent.nextStepsEnabled":           "true",
		"agent.maxToolRounds":              "3",
		"agent.maxToolResultBytes":         "4096",
		"agent.defaultSlot":                "standard",
		"sanitize.maxItems":                "0",
		"sanitize.maxBytes":                "0",
		"session.maxContextMessages":       "10",
		"session.maxContextBytes":          "8192",
		"session.compactThresholdRatio":    "0.8",
		"memory.enabled":                   "false",
		"memory.summaryMinEvictedMessages": "2",
	}
	for key, value := range overrides {
		values[key] = value
	}
	return config.NewParams(values)
}

// mustParams 是 testParams 的便捷包装，便于在表驱动测试里直接使用。
func mustParams(t *testing.T, overrides map[string]string) *config.Params {
	t.Helper()
	return testParams(overrides)
}
