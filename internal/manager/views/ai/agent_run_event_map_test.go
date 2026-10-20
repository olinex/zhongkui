package ai

import (
	// 标准库
	"testing"

	// 项目内部
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/sse"
)

// TestAgentRunEventTypeSSEMapping 守卫运行事件类型与 SSE 事件名的映射关系，防止两侧字面量漂移。
// AgentRunEventType 是落库 agent_run_events.type 的领域取值空间，sse.SSEEventValue 是传输层
// 事件名取值空间；二者各自独立声明，仅在视图层（writePublished）以 sse.SSEEventValue(ev.Type)
// 完成领域 → 传输的转换，故需在本边界锁定其对应关系：每个领域事件类型都必须有一个取值一致的
// SSE 事件名，且不得有遗漏或多余。
func TestAgentRunEventTypeSSEMapping(t *testing.T) {
	mapping := map[modelsai.AgentRunEventType]sse.SSEEventValue{
		modelsai.AgentRunEventDelta:         sse.SSEEventDelta,
		modelsai.AgentRunEventReasoning:     sse.SSEEventReasoning,
		modelsai.AgentRunEventStage:         sse.SSEEventStage,
		modelsai.AgentRunEventToolRequest:   sse.SSEEventToolRequest,
		modelsai.AgentRunEventCompressStart: sse.SSEEventCompressStart,
		modelsai.AgentRunEventCompressDelta: sse.SSEEventCompressDelta,
		modelsai.AgentRunEventCompressDone:  sse.SSEEventCompressDone,
		modelsai.AgentRunEventDone:          sse.SSEEventDone,
		modelsai.AgentRunEventError:         sse.SSEEventError,
		modelsai.AgentRunEventState:         sse.SSEEventState,
		modelsai.AgentRunEventBlock:         sse.SSEEventBlock,
	}
	types := modelsai.AllAgentRunEventTypes()
	if len(mapping) != len(types) {
		t.Fatalf("SSE 映射覆盖 %d 个事件类型，实际有 %d 个", len(mapping), len(types))
	}
	for _, typ := range types {
		want, ok := mapping[typ]
		if !ok {
			t.Errorf("运行事件类型 %q 缺少对应的 SSE 事件名", typ)
			continue
		}
		if got := sse.SSEEventValue(typ); got != want {
			t.Errorf("运行事件类型 %q 映射到 SSE 事件 %q，期望 %q", typ, got, want)
		}
	}
}
