package ai

import (
	// 标准库
	"testing"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/llm"
)

// testParams 构造测试用参数读取器：出厂值由 migrate fixture 注入数据库，测试不复制生产
// 提示词正文，故提示词类键给可辨识的短占位串（agent.systemPrompt 保留 "ZhongKui" 以便
// 断言系统提示已注入），其余键给与出厂值等价的字面量；overrides 覆盖其上。
func testParams(t *testing.T, overrides map[string]string) *llm.Params {
	t.Helper()
	values := map[string]string{
		"agent.systemPrompt":                "test system prompt of ZhongKui AI",
		"agent.retrievalSectionTitle":       "## Retrieved Knowledge",
		"agent.businessSectionTitle":        "## Current Business Scene",
		"agent.questionSectionTitle":        "## User Question",
		"agent.retrievedDocSnippetLimit":    "500",
		"agent.defaultSlot":                 "standard",
		"agent.maxToolRounds":               "3",
		"agent.maxToolResultBytes":          "8192",
		"retrieval.topK":                    "5",
		"retrieval.candidates":              "30",
		"retrieval.fusion":                  "rrf",
		"retrieval.weightVector":            "0.6",
		"retrieval.weightKeyword":           "0.4",
		"retrieval.rerankSystemPrompt":      "test rerank system prompt",
		"retrieval.rerankSnippetLimit":      "500",
		"retrieval.rerankTemperature":       "0.2",
		"session.maxContextMessages":        "20",
		"session.maxContextBytes":           "32768",
		"session.topicSystemPrompt":         "test session topic system prompt",
		"memory.enabled":                    "false",
		"memory.sectionTitle":               "## Long-term Memory",
		"memory.topK":                       "5",
		"memory.candidates":                 "20",
		"memory.maxItems":                   "8",
		"memory.maxItemBytes":               "1024",
		"memory.dedupeThreshold":            "0.92",
		"memory.summarySystemPrompt":        "test memory summary system prompt",
		"memory.summaryMinEvictedMessages":  "6",
		"memory.extractSystemPrompt":        "test memory extract system prompt",
		"memory.extractMaxItems":            "5",
		"chunking.size":                     "800",
		"chunking.overlap":                  "100",
		"eval.answerSystemPrompt":           "test eval answer system prompt",
		"eval.chunkRelevanceSystemPrompt":   "test eval chunk relevance system prompt",
		"eval.claimsSystemPrompt":           "test eval claims system prompt",
		"eval.reverseQuestionsSystemPrompt": "test eval reverse questions system prompt",
		"eval.suggestionsSystemPrompt":      "test eval suggestions system prompt",
		"eval.reverseQuestionCount":         "3",
		"eval.snippetLimit":                 "500",
		"sanitize.maxItems":                 "0",
		"sanitize.maxBytes":                 "32768",
	}
	for key, value := range overrides {
		values[key] = value
	}
	return llm.NewParams(values)
}
