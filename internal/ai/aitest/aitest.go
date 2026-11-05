// Package aitest 提供 internal/ai 各子包单测共用的测试装配：
// 全局密钥派生初始化、内存库构造、行为参数与常用替身。本包只被测试代码引用，不进入生产装配。
package aitest

import (
	"context"
	"testing"

	// 第三方

	"github.com/cloudwego/eino/components/embedding"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	// 项目内部

	"zhongkuisec.com/zhongkui/internal/ai/config"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto/key"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
)

// InitKeys 初始化全局密钥派生：生成文档的派生摘要与落库摘要共用 hash 字段的 HMAC 密钥，
// 纯逻辑测试也要能计算摘要，故先初始化全局密钥派生。各包 TestMain 调用一次。
func InitKeys() {
	pb, err := key.LoadPasswordBook("")
	if err != nil {
		panic(err)
	}
	provider := key.NewConfigFileSecretKeyProvider("this-is-a-32-byte-secret-key!!")
	key.InitGlobalKeyDerivation(provider, pb)
}

// NewSessionDB 构造会话测试用的 sqlite 内存库并迁移会话/消息/持久化交互/长期记忆/挂起运行相关表
// （会话删除级联用例需要这些表同库；sqlite 不支持 pgvector/tsvector，用例只覆盖空嵌入的路径）。
func NewSessionDB(t testing.TB) *database.DB {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("gorm.Open(sqlite) error = %v", err)
	}
	if err := gdb.AutoMigrate(
		&modelsai.AgentSession{}, &modelsai.AgentMessage{}, &modelsai.AgentSessionCompaction{}, &modelsai.AgentMemory{}, &modelsai.AgentRun{},
		&modelsai.AgentInteraction{},
	); err != nil {
		t.Fatalf("AutoMigrate error = %v", err)
	}
	return gdb
}

// NewKnowledgeDB 构造知识库/长期记忆测试用的 sqlite 内存库并迁移知识文档、知识分块与长期记忆表
// （sqlite 不支持 pgvector/tsvector，用例只覆盖空命名空间与不触发向量写入的路径）。
func NewKnowledgeDB(t testing.TB) *database.DB {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("gorm.Open(sqlite) error = %v", err)
	}
	if err := gdb.AutoMigrate(&modelsai.KnowledgeDocument{}, &modelsai.KnowledgeChunk{}, &modelsai.AgentMemory{}); err != nil {
		t.Fatalf("AutoMigrate error = %v", err)
	}
	return gdb
}

// Params 构造测试用参数读取器：出厂值由 migrate fixture 注入数据库，测试不复制生产
// 提示词正文，故提示词类键给可辨识的短占位串（agent.systemPrompt 保留 "ZhongKui" 以便
// 断言系统提示已注入），其余键给与出厂值等价的字面量；overrides 覆盖其上。
func Params(t testing.TB, overrides map[string]string) *config.Params {
	t.Helper()
	values := map[string]string{
		"agent.systemPrompt":               "test system prompt of ZhongKui AI",
		"agent.generalSystemPrompt":        "test general system prompt of ZhongKui AI",
		"agent.auditLevelSystemPrompt":     "test audit level system prompt",
		"agent.promptOptimizeSystemPrompt": "test prompt optimize system prompt",
		"agent.nextStepsSystemPrompt":      "test next steps system prompt",
		// 推荐下一步默认关闭：既有用例不应额外发起辅助模型调用；专测显式覆盖为 "true"。
		"agent.nextStepsEnabled":            "false",
		"agent.retrievedDocSnippetLimit":    "500",
		"agent.defaultSlot":                 "standard",
		"agent.maxToolRounds":               "10",
		"agent.maxToolResultBytes":          "32768",
		"agent.runDetachGraceSeconds":       "90",
		"retrieval.topK":                    "5",
		"retrieval.candidates":              "30",
		"retrieval.expandDocLimit":          "26000",
		"retrieval.fusion":                  "rrf",
		"retrieval.weightVector":            "0.6",
		"retrieval.weightKeyword":           "0.4",
		"retrieval.rerankSystemPrompt":      "test rerank system prompt",
		"retrieval.rerankSnippetLimit":      "500",
		"retrieval.rerankTemperature":       "0.2",
		"session.maxContextMessages":        "20",
		"session.maxContextBytes":           "32768",
		"session.compactThresholdRatio":     "0.8",
		"session.topicSystemPrompt":         "test session topic system prompt",
		"memory.enabled":                    "false",
		"memory.topK":                       "5",
		"memory.candidates":                 "20",
		"memory.maxItems":                   "8",
		"memory.maxItemBytes":               "1024",
		"memory.dedupeThreshold":            "0.92",
		"memory.summarySystemPrompt":        "test memory summary system prompt",
		"memory.summaryMinEvictedMessages":  "6",
		"memory.evictedMessageByteLimit":    "2000",
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
	return config.NewParams(values)
}

// StubEmbedder 是满足 embedder 接口的最小桩实现：EmbedStrings 返回与入参等长的空向量表。
type StubEmbedder struct{}

// EmbedStrings 返回与入参等长的空向量表。
func (StubEmbedder) EmbedStrings(_ context.Context, texts []string, _ ...embedding.Option) ([][]float64, error) {
	out := make([][]float64, len(texts))
	return out, nil
}

// RecordingReporter 是 contract.SyncReporter 的测试替身：记录 Start/Step/Finish 的调用次数与参数。
type RecordingReporter struct {
	// Starts Start 收到的待处理总数序列。
	Starts []int
	// Steps Step 收到的进度明细序列。
	Steps []contract.SyncProgressItem
	// Finishs Finish 的调用次数。
	Finishs int
}

// Start 记录一次 Start 的待处理总数。
func (r *RecordingReporter) Start(total int) {
	r.Starts = append(r.Starts, total)
}

// Step 记录一条进度明细。
func (r *RecordingReporter) Step(item contract.SyncProgressItem) {
	r.Steps = append(r.Steps, item)
}

// Finish 累计一次 Finish 调用。
func (r *RecordingReporter) Finish() {
	r.Finishs++
}
