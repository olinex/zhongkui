package ai

import (
	// 标准库
	"context"
	"testing"

	// 第三方
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
)

// TestOptimizePrompt_Success 测试成功路径：flash 档位返回带首尾空白的文本时返回裁剪后的正文；系统
// 消息取自 agent.promptOptimizeSystemPrompt（本包已不持有出厂值，故须由参数读取器提供），用户消息
// 即原始诉求。
func TestOptimizePrompt_Success(t *testing.T) {
	chat := &stubChat{reply: "  改写后的提问  "}
	agent := newAuditTestAgent(t, slotChatResolver(map[ai.Slot]model.BaseChatModel{ai.SlotFlash: chat}))

	system := agent.params.Text("agent.promptOptimizeSystemPrompt")
	if system == "" {
		t.Fatal("test params do not provide agent.promptOptimizeSystemPrompt")
	}
	got, err := agent.OptimizePrompt(context.Background(), "帮我看看这个")
	if err != nil {
		t.Fatalf("OptimizePrompt() error = %v", err)
	}
	if got != "改写后的提问" {
		t.Errorf("OptimizePrompt() = %q, want the trimmed text", got)
	}
	if len(chat.gotInputs) != 1 {
		t.Fatalf("chat calls = %d, want 1", len(chat.gotInputs))
	}
	input := chat.gotInputs[0]
	if input[0].Role != schema.System || input[0].Content != system {
		t.Errorf("system message = %+v, want the configured prompt optimization prompt", input[0])
	}
	if input[1].Role != schema.User || input[1].Content != "帮我看看这个" {
		t.Errorf("user message = %+v, want the original query", input[1])
	}
}

// TestOptimizePrompt_EmptyQuery 测试空诉求（含纯空白）：返回英文错误且不调用模型（以替身收到的调用
// 次数断言）。
func TestOptimizePrompt_EmptyQuery(t *testing.T) {
	chat := &stubChat{reply: "unused"}
	agent := newAuditTestAgent(t, slotChatResolver(map[ai.Slot]model.BaseChatModel{ai.SlotFlash: chat}))

	if _, err := agent.OptimizePrompt(context.Background(), "   "); err == nil {
		t.Fatal("OptimizePrompt(\"   \") = nil error, want an error")
	}
	if len(chat.gotInputs) != 0 {
		t.Errorf("model calls = %d, want 0 for an empty prompt", len(chat.gotInputs))
	}
}

// TestOptimizePrompt_FlashResolveError 测试 flash 档位解析失败：返回英文错误（不回退到其它档位）。
func TestOptimizePrompt_FlashResolveError(t *testing.T) {
	// 空绑定表使任意档位解析都返回错误，无需真实模型。
	agent := newAuditTestAgent(t, slotChatResolver(map[ai.Slot]model.BaseChatModel{}))

	if _, err := agent.OptimizePrompt(context.Background(), "hello"); err == nil {
		t.Fatal("OptimizePrompt() with an unresolvable flash slot = nil error, want an error")
	}
}
