package llm

import (
	// stdlib
	"context"
	"strings"
	"testing"
	"time"
)

// TestParseChatSlot 测试对话档位解析：flash/standard/advanced 合法（容忍空白与大小写），
// embedding 与未知取值非法。
// TestParseChatSlot verifies chat-slot parsing: flash/standard/advanced are valid (whitespace
// and case are tolerated), while embedding and unknown values are invalid.
func TestParseChatSlot(t *testing.T) {
	cases := []struct {
		value string
		want  Slot
		ok    bool
	}{
		{"flash", SlotFlash, true},
		{" standard ", SlotStandard, true},
		{"ADVANCED", SlotAdvanced, true},
		{"embedding", "", false},
		{"", "", false},
		{"huge", "", false},
	}
	for _, c := range cases {
		slot, ok := ParseChatSlot(c.value)
		if ok != c.ok || slot != c.want {
			t.Errorf("ParseChatSlot(%q) = (%q, %v), want (%q, %v)", c.value, slot, ok, c.want, c.ok)
		}
	}
}

// TestResolveSlot 测试档位优先级解析：显式优先于调用点声明，声明优先于默认；
// 非法取值与全空输入均返回英文错误。
// TestResolveSlot verifies the slot precedence resolution: the explicit value wins over the
// declared one, which wins over the default; invalid values and an all-empty input yield
// English errors.
func TestResolveSlot(t *testing.T) {
	slot, err := ResolveSlot(SlotAdvanced, SlotFlash, SlotStandard)
	if err != nil || slot != SlotAdvanced {
		t.Errorf("ResolveSlot(explicit) = (%q, %v), want (%q, nil)", slot, err, SlotAdvanced)
	}
	slot, err = ResolveSlot("", SlotFlash, SlotStandard)
	if err != nil || slot != SlotFlash {
		t.Errorf("ResolveSlot(declared) = (%q, %v), want (%q, nil)", slot, err, SlotFlash)
	}
	slot, err = ResolveSlot("", "", SlotStandard)
	if err != nil || slot != SlotStandard {
		t.Errorf("ResolveSlot(fallback) = (%q, %v), want (%q, nil)", slot, err, SlotStandard)
	}
	if _, err = ResolveSlot(SlotEmbedding, "", SlotStandard); err == nil || !strings.Contains(err.Error(), "invalid chat slot") {
		t.Errorf("ResolveSlot(embedding) error = %v, want invalid-chat-slot error", err)
	}
	if _, err = ResolveSlot("", "", ""); err == nil || !strings.Contains(err.Error(), "invalid default chat slot") {
		t.Errorf("ResolveSlot(empty) error = %v, want invalid-default-slot error", err)
	}
}

// TestNewChatClient_Validation 测试 Chat 客户端缺 baseURL/apiKey/model 时返回英文错误。
// TestNewChatClient_Validation verifies a missing baseURL/apiKey/model yields an English
// error.
func TestNewChatClient_Validation(t *testing.T) {
	cases := []struct {
		name string
		cfg  ModelConfig
	}{
		{"no baseURL", ModelConfig{APIKey: "k", Model: "m"}},
		{"no apiKey", ModelConfig{BaseURL: "http://x", Model: "m"}},
		{"no model", ModelConfig{BaseURL: "http://x", APIKey: "k"}},
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
		cfg  ModelConfig
	}{
		{"no baseURL", ModelConfig{APIKey: "k", Model: "m"}},
		{"no apiKey", ModelConfig{BaseURL: "http://x", Model: "m"}},
		{"no model", ModelConfig{BaseURL: "http://x", APIKey: "k"}},
	}
	for _, tt := range cases {
		if _, err := NewEmbedClient(context.Background(), tt.cfg); err == nil {
			t.Errorf("NewEmbedClient(%s) error = nil, want error", tt.name)
		} else if !strings.Contains(err.Error(), "incomplete embedding config") {
			t.Errorf("NewEmbedClient(%s) error = %v, want incomplete-embedding-config message", tt.name, err)
		}
	}
}

// TestClient_ModelName 测试客户端回传底层供应商模型名（供状态与 dry-run 元信息展示）。
// TestClient_ModelName verifies the clients report the vendor-side model name (for status
// and dry-run metadata display).
func TestClient_ModelName(t *testing.T) {
	// eino 构造器不发起网络请求，仅组装配置，可使用不可达地址
	// The eino constructors only assemble configs and never perform network I/O, so an
	// unreachable address is fine here
	chat, err := NewChatClient(context.Background(), ModelConfig{
		Kind:        ModelKindChat,
		BaseURL:     "http://127.0.0.1:1/v1",
		APIKey:      "sk-test",
		Model:       "gpt-test",
		Timeout:     time.Second,
		Temperature: 0.5,
		MaxTokens:   1024,
	})
	if err != nil {
		t.Fatalf("NewChatClient() error = %v", err)
	}
	if chat.ModelName() != "gpt-test" {
		t.Errorf("ChatClient.ModelName() = %q, want %q", chat.ModelName(), "gpt-test")
	}
	embed, err := NewEmbedClient(context.Background(), ModelConfig{
		Kind:       ModelKindEmbedding,
		BaseURL:    "http://127.0.0.1:1/v1",
		APIKey:     "sk-test",
		Model:      "embed-test",
		Dimensions: 128,
	})
	if err != nil {
		t.Fatalf("NewEmbedClient() error = %v", err)
	}
	if embed.ModelName() != "embed-test" {
		t.Errorf("EmbedClient.ModelName() = %q, want %q", embed.ModelName(), "embed-test")
	}
}
