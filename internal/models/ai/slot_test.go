package ai

import (
	// 标准库
	"strings"
	"testing"
)

// TestParseChatSlot 测试对话档位解析：flash/standard/advanced 合法（容忍空白与大小写），
// embedding 与未知取值非法。
func TestParseChatSlot(t *testing.T) {
	cases := []struct {
		value Slot
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
