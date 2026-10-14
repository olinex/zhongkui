package ai

import (
	// 标准库
	"context"
	"strings"
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
)

// TestRunHub_RunSurvivesRequestCancel 测试驻留运行与连接解耦：请求上下文在启动后立即取消
// （模拟关抽屉 / 刷新），运行仍以脱管上下文执行到完成（done 收敛）；事件按序落库且负载加密
// 存储（不含明文）。
func TestRunHub_RunSurvivesRequestCancel(t *testing.T) {
	store, err := NewSessionStore(newTestSessionDB(t))
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	params := testParams(t, map[string]string{
		"session.maxContextMessages": "10",
		"session.maxContextBytes":    "4096",
	})
	chat := &stubChat{chunks: []string{"Hello", " world"}}
	agent, err := NewAgent(context.Background(), stubResolver(chat), &stubRetriever{}, store, params, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAgent error = %v", err)
	}
	hub := NewRunHub(RunHubDeps{
		Agent:  func() RunExecutor { return agent },
		Params: func() *Params { return params },
		Store:  func() RunStore { return store },
	})

	// 模拟浏览器刷新：启动驻留运行后立即取消请求上下文。
	reqCtx, cancelReq := context.WithCancel(context.Background())
	sessionID := uint(7)
	runID, err := hub.Start(NewDetachedContext(reqCtx), AgentInput{Query: "hi"}, sessionID, testUserID, "default", nil)
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}
	cancelReq()

	_, live, cancelSub, err := hub.Subscribe(context.Background(), sessionID, testUserID, 0)
	if err != nil {
		t.Fatalf("Subscribe error = %v", err)
	}
	defer cancelSub()
	var done *Published
	for ev := range live {
		if ev.Type == ai.AgentRunEventDone {
			cp := ev
			done = &cp
		}
	}
	if done == nil {
		t.Fatal("the run did not converge to done after the request context was cancelled")
	}
	if !strings.Contains(string(done.Payload), "Hello world") {
		t.Errorf("done payload = %s, want the aggregated content", done.Payload)
	}

	// 事件负载经 crypto 序列化器加密落库：原始列不含明文增量。
	var row struct{ Payload []byte }
	if err := store.db.Raw("SELECT payload FROM agent_run_events WHERE run_id = ? AND seq = 1", runID).Scan(&row).Error; err != nil {
		t.Fatalf("reading the raw run event payload error = %v", err)
	}
	assertEncryptedColumn(t, "agent_run_events.payload", row.Payload, "Hello")
}
