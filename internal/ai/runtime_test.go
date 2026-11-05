package ai

import (
	// 标准库
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/llmconfig"
)

// TestSwapStateReturnsReplacedState 验证换装点语义：首次替换返回 nil，此后返回被替换掉的旧装配态
// （Invalidate / Shutdown 据此关闭旧快照持有的 MCP 会话）。
func TestSwapStateReturnsReplacedState(t *testing.T) {
	oldState := &runtimeState{snapshot: llmconfig.EmptySnapshot(), comps: &Components{}}
	nextState := &runtimeState{snapshot: llmconfig.EmptySnapshot(), comps: &Components{}}
	rt := &Runtime{}
	if got := rt.swapState(oldState); got != nil {
		t.Errorf("first swapState returned %v, want nil", got)
	}
	if replaced := rt.swapState(nextState); replaced != oldState {
		t.Fatalf("swapState returned %p, want the replaced old state %p", replaced, oldState)
	}
}
