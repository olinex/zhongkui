package llm

import (
	// stdlib

	"context"
	"strings"
	"testing"
	"time"
)

// TestNewChatClient_Validation 测试 Chat 客户端缺 baseURL/apiKey/model 时返回英文错误。
// TestNewChatClient_Validation verifies a missing baseURL/apiKey/model yields an English
// error.
func TestNewChatClient_Validation(t *testing.T) {
	cases := []struct {
		name string
		cfg  ChatConfig
	}{
		{"no baseURL", ChatConfig{APIKey: "k", Model: "m"}},
		{"no apiKey", ChatConfig{BaseURL: "http://x", Model: "m"}},
		{"no model", ChatConfig{BaseURL: "http://x", APIKey: "k"}},
	}
	for _, tt := range cases {
		if _, err := NewChatClient(context.Background(), tt.cfg); err == nil {
			t.Errorf("NewChatClient(%s) error = nil, want error", tt.name)
		} else if !strings.Contains(err.Error(), "incomplete chat config") {
			t.Errorf("NewChatClient(%s) error = %v, want incomplete-chat-config message", tt.name, err)
		}
	}
}

// TestNewEmbedClient_Validation 测试 Embedding 客户端缺 baseURL/apiKey/model 时返回英文错误。
// TestNewEmbedClient_Validation verifies a missing baseURL/apiKey/model yields an English
// error.
func TestNewEmbedClient_Validation(t *testing.T) {
	cases := []struct {
		name string
		cfg  EmbeddingConfig
	}{
		{"no baseURL", EmbeddingConfig{APIKey: "k", Model: "m"}},
		{"no apiKey", EmbeddingConfig{BaseURL: "http://x", Model: "m"}},
		{"no model", EmbeddingConfig{BaseURL: "http://x", APIKey: "k"}},
	}
	for _, tt := range cases {
		if _, err := NewEmbedClient(context.Background(), tt.cfg); err == nil {
			t.Errorf("NewEmbedClient(%s) error = nil, want error", tt.name)
		} else if !strings.Contains(err.Error(), "incomplete embedding config") {
			t.Errorf("NewEmbedClient(%s) error = %v, want incomplete-embedding-config message", tt.name, err)
		}
	}
}

// TestNewClients_Validation 测试聚合构造：nil 配置或缺任一端点配置时报错并指明缺失部分。
// TestNewClients_Validation verifies the aggregate constructor: a nil config or a missing
// endpoint config errors out naming the missing part.
func TestNewClients_Validation(t *testing.T) {
	if _, err := NewClients(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "nil") {
		t.Errorf("NewClients(nil) error = %v, want nil-config error", err)
	}
	noChat := &LLMConfig{Embedding: EmbeddingConfig{BaseURL: "http://x", APIKey: "k", Model: "m"}}
	if _, err := NewClients(context.Background(), noChat); err == nil || !strings.Contains(err.Error(), "chat") {
		t.Errorf("NewClients(missing chat) error = %v, want chat error", err)
	}
	noEmbed := &LLMConfig{Chat: ChatConfig{BaseURL: "http://x", APIKey: "k", Model: "m"}}
	if _, err := NewClients(context.Background(), noEmbed); err == nil || !strings.Contains(err.Error(), "embedding") {
		t.Errorf("NewClients(missing embedding) error = %v, want embedding error", err)
	}
}

// TestNewClients_Success 测试完整配置可一次性构造出对话与向量化客户端
// （eino 构造器不发起网络请求，仅组装配置）。
// TestNewClients_Success verifies a complete config constructs both the chat and the
// embedding clients in one call (the eino constructors only assemble configs and never
// perform network I/O).
func TestNewClients_Success(t *testing.T) {
	cfg := &LLMConfig{
		Chat: ChatConfig{
			BaseURL:     "http://127.0.0.1:1/v1",
			APIKey:      "sk-test",
			Model:       "gpt-test",
			Timeout:     time.Second,
			Temperature: 0.5,
			MaxTokens:   1024,
		},
		Embedding: EmbeddingConfig{
			BaseURL:    "http://127.0.0.1:1/v1",
			APIKey:     "sk-test",
			Model:      "embed-test",
			Dimensions: 128,
		},
	}
	clients, err := NewClients(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewClients() error = %v", err)
	}
	if clients.Chat == nil {
		t.Error("NewClients() chat client is nil")
	}
	if clients.Embed == nil {
		t.Error("NewClients() embedding client is nil")
	}
}
