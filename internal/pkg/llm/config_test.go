package llm

import (
	"testing"
	"time"

	uv "github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

// testValidate 测试用验证器实例（无业务校验标签，仅用于满足 ExtractConfig 签名）。
var testValidate = uv.New()

// TestExtractConfig 测试从 Viper 提取 [llm] 配置并映射到各字段（含 time.Duration 字符串解析）。
// TestExtractConfig verifies extracting the [llm] config from Viper into fields (including
// the string-to-time.Duration parsing).
func TestExtractConfig(t *testing.T) {
	v := viper.New()
	v.Set("llm.enabled", true)
	v.Set("llm.chat.baseURL", "https://api.example.com/v1")
	v.Set("llm.chat.apiKey", "sk-test")
	v.Set("llm.chat.model", "gpt-4o")
	v.Set("llm.chat.timeout", "30s")
	v.Set("llm.chat.temperature", 0.5)
	v.Set("llm.chat.maxTokens", 2048)
	v.Set("llm.embedding.baseURL", "https://api.example.com/v1")
	v.Set("llm.embedding.apiKey", "sk-test")
	v.Set("llm.embedding.model", "text-embedding-3-small")
	v.Set("llm.embedding.dimensions", 1024)
	v.Set("llm.rerank.topK", 8)
	v.Set("llm.rerank.candidates", 40)
	v.Set("llm.rerank.fusion", "weighted")
	v.Set("llm.rerank.weights.vector", 0.7)
	v.Set("llm.rerank.weights.keyword", 0.3)
	v.Set("llm.session.maxContextMessages", 30)
	v.Set("llm.session.maxContextBytes", 65536)
	v.Set("llm.chunking.size", 1000)
	v.Set("llm.chunking.overlap", 150)

	cfg, err := ExtractConfig(v, testValidate)
	if err != nil {
		t.Fatalf("ExtractConfig() error = %v", err)
	}
	if !cfg.EnabledSwitch {
		t.Errorf("EnabledSwitch = false, want true")
	}
	if cfg.Chat.BaseURL != "https://api.example.com/v1" {
		t.Errorf("Chat.BaseURL = %q, want %q", cfg.Chat.BaseURL, "https://api.example.com/v1")
	}
	if cfg.Chat.APIKey != "sk-test" {
		t.Errorf("Chat.APIKey = %q, want %q", cfg.Chat.APIKey, "sk-test")
	}
	if cfg.Chat.Model != "gpt-4o" {
		t.Errorf("Chat.Model = %q, want %q", cfg.Chat.Model, "gpt-4o")
	}
	if cfg.Chat.Timeout != 30*time.Second {
		t.Errorf("Chat.Timeout = %v, want %v", cfg.Chat.Timeout, 30*time.Second)
	}
	if cfg.Chat.Temperature != 0.5 {
		t.Errorf("Chat.Temperature = %v, want 0.5", cfg.Chat.Temperature)
	}
	if cfg.Chat.MaxTokens != 2048 {
		t.Errorf("Chat.MaxTokens = %d, want 2048", cfg.Chat.MaxTokens)
	}
	if cfg.Embedding.BaseURL != "https://api.example.com/v1" {
		t.Errorf("Embedding.BaseURL = %q, want %q", cfg.Embedding.BaseURL, "https://api.example.com/v1")
	}
	if cfg.Embedding.APIKey != "sk-test" {
		t.Errorf("Embedding.APIKey = %q, want %q", cfg.Embedding.APIKey, "sk-test")
	}
	if cfg.Embedding.Model != "text-embedding-3-small" {
		t.Errorf("Embedding.Model = %q, want %q", cfg.Embedding.Model, "text-embedding-3-small")
	}
	if cfg.Embedding.Dimensions != 1024 {
		t.Errorf("Embedding.Dimensions = %d, want 1024", cfg.Embedding.Dimensions)
	}
	if cfg.Rerank.TopK != 8 {
		t.Errorf("Rerank.TopK = %d, want 8", cfg.Rerank.TopK)
	}
	if cfg.Rerank.Candidates != 40 {
		t.Errorf("Rerank.Candidates = %d, want 40", cfg.Rerank.Candidates)
	}
	if cfg.Rerank.Fusion != "weighted" {
		t.Errorf("Rerank.Fusion = %q, want %q", cfg.Rerank.Fusion, "weighted")
	}
	if cfg.Rerank.Weights.Vector != 0.7 {
		t.Errorf("Rerank.Weights.Vector = %v, want 0.7", cfg.Rerank.Weights.Vector)
	}
	if cfg.Rerank.Weights.Keyword != 0.3 {
		t.Errorf("Rerank.Weights.Keyword = %v, want 0.3", cfg.Rerank.Weights.Keyword)
	}
	if cfg.Session.MaxContextMessages != 30 {
		t.Errorf("Session.MaxContextMessages = %d, want 30", cfg.Session.MaxContextMessages)
	}
	if cfg.Session.MaxContextBytes != 65536 {
		t.Errorf("Session.MaxContextBytes = %d, want 65536", cfg.Session.MaxContextBytes)
	}
	if cfg.Chunking.Size != 1000 {
		t.Errorf("Chunking.Size = %d, want 1000", cfg.Chunking.Size)
	}
	if cfg.Chunking.Overlap != 150 {
		t.Errorf("Chunking.Overlap = %d, want 150", cfg.Chunking.Overlap)
	}
}

// TestExtractConfig_DefaultsBackfill 测试 [llm] 段存在但未显式配置的字段由 default 标签回填默认值。
// TestExtractConfig_DefaultsBackfill verifies unspecified fields fall back to their
// default-tag values when the [llm] section exists.
func TestExtractConfig_DefaultsBackfill(t *testing.T) {
	v := viper.New()
	v.Set("llm.enabled", false)

	cfg, err := ExtractConfig(v, testValidate)
	if err != nil {
		t.Fatalf("ExtractConfig() error = %v", err)
	}
	if cfg.EnabledSwitch {
		t.Errorf("EnabledSwitch = true, want false (default)")
	}
	if cfg.Chat.Timeout != 60*time.Second {
		t.Errorf("Chat.Timeout = %v, want 60s (default)", cfg.Chat.Timeout)
	}
	if cfg.Chat.Temperature != 0.2 {
		t.Errorf("Chat.Temperature = %v, want 0.2 (default)", cfg.Chat.Temperature)
	}
	if cfg.Chat.MaxTokens != 4096 {
		t.Errorf("Chat.MaxTokens = %d, want 4096 (default)", cfg.Chat.MaxTokens)
	}
	if cfg.Embedding.Dimensions != 1536 {
		t.Errorf("Embedding.Dimensions = %d, want 1536 (default)", cfg.Embedding.Dimensions)
	}
	if cfg.Rerank.TopK != 5 {
		t.Errorf("Rerank.TopK = %d, want 5 (default)", cfg.Rerank.TopK)
	}
	if cfg.Rerank.Candidates != 30 {
		t.Errorf("Rerank.Candidates = %d, want 30 (default)", cfg.Rerank.Candidates)
	}
	if cfg.Rerank.Fusion != "rrf" {
		t.Errorf("Rerank.Fusion = %q, want %q (default)", cfg.Rerank.Fusion, "rrf")
	}
	if cfg.Rerank.Weights.Vector != 0.6 {
		t.Errorf("Rerank.Weights.Vector = %v, want 0.6 (default)", cfg.Rerank.Weights.Vector)
	}
	if cfg.Rerank.Weights.Keyword != 0.4 {
		t.Errorf("Rerank.Weights.Keyword = %v, want 0.4 (default)", cfg.Rerank.Weights.Keyword)
	}
	if cfg.Session.MaxContextMessages != 20 {
		t.Errorf("Session.MaxContextMessages = %d, want 20 (default)", cfg.Session.MaxContextMessages)
	}
	if cfg.Session.MaxContextBytes != 32768 {
		t.Errorf("Session.MaxContextBytes = %d, want 32768 (default)", cfg.Session.MaxContextBytes)
	}
	if cfg.Chunking.Size != 800 {
		t.Errorf("Chunking.Size = %d, want 800 (default)", cfg.Chunking.Size)
	}
	if cfg.Chunking.Overlap != 100 {
		t.Errorf("Chunking.Overlap = %d, want 100 (default)", cfg.Chunking.Overlap)
	}
}

// TestExtractConfigOrEmpty 测试 [llm] 段完全缺省时返回填充默认值的空配置且不报错。
// TestExtractConfigOrEmpty verifies a fully absent [llm] section yields a defaulted empty
// config without an error.
func TestExtractConfigOrEmpty(t *testing.T) {
	v := viper.New()
	cfg, err := ExtractConfigOrEmpty(v, testValidate)
	if err != nil {
		t.Fatalf("ExtractConfigOrEmpty() error = %v, want nil for absent section", err)
	}
	if cfg == nil {
		t.Fatal("ExtractConfigOrEmpty() = nil config")
	}
	if cfg.EnabledSwitch {
		t.Errorf("EnabledSwitch = true, want false (default)")
	}
	if cfg.Embedding.Dimensions != 1536 {
		t.Errorf("Embedding.Dimensions = %d, want 1536 (default)", cfg.Embedding.Dimensions)
	}
	if cfg.Chat.Timeout != 60*time.Second {
		t.Errorf("Chat.Timeout = %v, want 60s (default)", cfg.Chat.Timeout)
	}
	if cfg.Enabled() {
		t.Errorf("Enabled() = true, want false when chat/embedding configs are empty")
	}
}

// TestExtractConfigOrEmpty_PreservesErrors 测试 [llm] 段存在但解析失败时错误不被吞掉。
// TestExtractConfigOrEmpty_PreservesErrors verifies parse errors of an existing [llm]
// section are propagated rather than swallowed.
func TestExtractConfigOrEmpty_PreservesErrors(t *testing.T) {
	v := viper.New()
	v.Set("llm.chat", "not-a-map")
	if _, err := ExtractConfigOrEmpty(v, testValidate); err == nil {
		t.Fatal("ExtractConfigOrEmpty() error = nil, want error when [llm] section is malformed")
	}
}

// TestLLMConfig_Enabled 测试 Enabled 在配置完整/不完整时的判定。
// TestLLMConfig_Enabled verifies Enabled with complete / incomplete configs.
func TestLLMConfig_Enabled(t *testing.T) {
	full := &LLMConfig{
		EnabledSwitch: true,
		Chat: ChatConfig{
			BaseURL: "https://api.example.com/v1",
			APIKey:  "sk-test",
			Model:   "gpt-4o",
		},
		Embedding: EmbeddingConfig{
			BaseURL: "https://api.example.com/v1",
			APIKey:  "sk-test",
			Model:   "text-embedding-3-small",
		},
	}
	if !full.Enabled() {
		t.Errorf("Enabled() = false, want true for complete config")
	}

	// 开关关闭：不可用
	disabled := *full
	disabled.EnabledSwitch = false
	if disabled.Enabled() {
		t.Errorf("Enabled() = true, want false when the enabled switch is off")
	}

	// chat 配置缺失：不可用
	noChat := *full
	noChat.Chat.APIKey = ""
	if noChat.Enabled() {
		t.Errorf("Enabled() = true, want false when the chat apiKey is empty")
	}

	// embedding 配置缺失：不可用
	noEmbedding := *full
	noEmbedding.Embedding.Model = ""
	if noEmbedding.Enabled() {
		t.Errorf("Enabled() = true, want false when the embedding model is empty")
	}

	// 空配置：不可用
	empty := &LLMConfig{}
	if empty.Enabled() {
		t.Errorf("Enabled() = true, want false for empty config")
	}
}

// TestDescribeConfig 测试 DescribeConfig 输出结构化描述且前缀为 ai。
// TestDescribeConfig verifies DescribeConfig returns descriptions under the "llm" prefix.
func TestDescribeConfig(t *testing.T) {
	descs := DescribeConfig()
	if len(descs) == 0 {
		t.Fatalf("DescribeConfig() returned empty descriptions")
	}
	found := map[string]bool{}
	for _, d := range descs {
		found[d.Path] = true
	}
	for _, key := range []string{
		"llm.enabled",
		"llm.chat.baseURL",
		"llm.chat.timeout",
		"llm.chat.temperature",
		"llm.chat.maxTokens",
		"llm.embedding.dimensions",
		"llm.rerank.topK",
		"llm.rerank.candidates",
		"llm.rerank.fusion",
		"llm.rerank.weights.vector",
		"llm.rerank.weights.keyword",
		"llm.session.maxContextMessages",
		"llm.session.maxContextBytes",
		"llm.chunking.size",
		"llm.chunking.overlap",
	} {
		if !found[key] {
			t.Errorf("DescribeConfig() missing path %q", key)
		}
	}
}
