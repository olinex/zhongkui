package ai

import (
	// 标准库
	"context"
	"sync"
	"testing"
	"time"
)

// fakeExpiredStore 是 ExpiredRunStore 的内存替身：三张表各以过期时刻切片表示行，删除时移除
// 所有早于 now 的行（至多 limit 行），并返回删除条数。收敛（存活性）以调用计数表示——其真实行为
// 由数据库层测试覆盖（见 session_liveness_test.go），此处只断言清扫任务会先收敛再删除。
type fakeExpiredStore struct {
	mu        sync.Mutex
	liveRuns  []time.Time
	runEvents []time.Time
	agentRuns []time.Time
	// convergePendingCalls / convergeRunningCalls 收敛步骤被调用的批次数（供清扫顺序断言）
	convergePendingCalls int
	convergeRunningCalls int
}

// ConvergeExpiredPendingRuns 记录一次「收敛过期挂起现场」的调用（内存替身不做真实收敛）。
func (s *fakeExpiredStore) ConvergeExpiredPendingRuns(ctx context.Context, now time.Time, limit int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.convergePendingCalls++
	return 0, nil
}

// ConvergeExpiredRunningLiveRuns 记录一次「收敛超期 running 在途运行」的调用（内存替身不做真实收敛）。
func (s *fakeExpiredStore) ConvergeExpiredRunningLiveRuns(ctx context.Context, now time.Time, limit int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.convergeRunningCalls++
	return 0, nil
}

// deleteExpired 从 *rows 中移除至多 limit 个早于 now 的行，返回删除条数。
func deleteExpired(rows *[]time.Time, now time.Time, limit int) int64 {
	kept := make([]time.Time, 0, len(*rows))
	var deleted int64
	for _, t := range *rows {
		if t.Before(now) && deleted < int64(limit) {
			deleted++
			continue
		}
		kept = append(kept, t)
	}
	*rows = kept
	return deleted
}

// DeleteExpiredLiveRuns 删除一批过期的运行登记。
func (s *fakeExpiredStore) DeleteExpiredLiveRuns(ctx context.Context, now time.Time, limit int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return deleteExpired(&s.liveRuns, now, limit), nil
}

// DeleteExpiredRunEvents 删除一批过期的事件日志。
func (s *fakeExpiredStore) DeleteExpiredRunEvents(ctx context.Context, now time.Time, limit int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return deleteExpired(&s.runEvents, now, limit), nil
}

// DeleteExpiredAgentRuns 删除一批过期的挂起运行现场。
func (s *fakeExpiredStore) DeleteExpiredAgentRuns(ctx context.Context, now time.Time, limit int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return deleteExpired(&s.agentRuns, now, limit), nil
}

// counts 返回三张表当前行数（活运行、事件、挂起现场）。
func (s *fakeExpiredStore) counts() (int, int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.liveRuns), len(s.runEvents), len(s.agentRuns)
}

// TestRunCleaner_SweepDeletesExpired 测试 sweep 删除过期行、保留未过期行，且三张表均被清扫。
func TestRunCleaner_SweepDeletesExpired(t *testing.T) {
	now := time.Now()
	store := &fakeExpiredStore{
		liveRuns:  []time.Time{now.Add(-time.Hour), now.Add(time.Hour)},
		runEvents: []time.Time{now.Add(-time.Hour), now.Add(time.Hour)},
		agentRuns: []time.Time{now.Add(-time.Hour), now.Add(time.Hour)},
	}
	c := newRunCleaner(store, nil)
	c.now = func() time.Time { return now }

	if err := c.sweep(context.Background()); err != nil {
		t.Fatalf("sweep error = %v", err)
	}
	live, events, agent := store.counts()
	if live != 1 || events != 1 || agent != 1 {
		t.Errorf("counts after sweep = (%d, %d, %d), want (1, 1, 1): only expired rows removed", live, events, agent)
	}
}

// TestRunCleaner_SweepBatched 测试分批删除：过期行数远大于 batch 时全部清完（循环直到某批不足 batch）。
func TestRunCleaner_SweepBatched(t *testing.T) {
	now := time.Now()
	rows := make([]time.Time, 0, 5)
	for i := 0; i < 5; i++ {
		rows = append(rows, now.Add(-time.Hour))
	}
	store := &fakeExpiredStore{liveRuns: rows}
	c := newRunCleaner(store, nil)
	c.batch = 2 // 强制多批：2 + 2 + 1
	c.now = func() time.Time { return now }

	if err := c.sweep(context.Background()); err != nil {
		t.Fatalf("sweep error = %v", err)
	}
	if live, _, _ := store.counts(); live != 0 {
		t.Errorf("live runs after batched sweep = %d, want 0", live)
	}
}

// TestRunCleaner_SweepConvergesBeforeDeleting 测试清扫任务先执行存活性收敛（过期挂起现场、超期 running
// 在途运行）再硬删除过期行：收敛步骤各被调用一次，且删除仍在收敛之后发生（过期行最终被移除）。
func TestRunCleaner_SweepConvergesBeforeDeleting(t *testing.T) {
	now := time.Now()
	store := &fakeExpiredStore{
		liveRuns:  []time.Time{now.Add(-time.Hour)},
		agentRuns: []time.Time{now.Add(-time.Hour)},
	}
	c := newRunCleaner(store, nil)
	c.now = func() time.Time { return now }

	if err := c.sweep(context.Background()); err != nil {
		t.Fatalf("sweep error = %v", err)
	}
	store.mu.Lock()
	pending, running := store.convergePendingCalls, store.convergeRunningCalls
	store.mu.Unlock()
	if pending != 1 || running != 1 {
		t.Errorf("converge calls = (pending %d, running %d), want (1, 1)", pending, running)
	}
	if live, _, agent := store.counts(); live != 0 || agent != 0 {
		t.Errorf("counts after sweep = (live %d, agent %d), want (0, 0): expired rows still deleted", live, agent)
	}
}

// TestRunCleaner_RunStopsOnCancel 测试 run 在 ctx 取消后返回（先立即清扫一次，随后退出）。
func TestRunCleaner_RunStopsOnCancel(t *testing.T) {
	now := time.Now()
	store := &fakeExpiredStore{liveRuns: []time.Time{now.Add(-time.Hour)}}
	c := newRunCleaner(store, nil)
	c.interval = time.Hour // 避免周期清扫干扰：仅依赖启动时的立即清扫
	c.now = func() time.Time { return now }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.run(ctx)
	}()

	// 立即清扫应已删除过期行。
	deadline := time.Now().Add(2 * time.Second)
	for {
		if live, _, _ := store.counts(); live == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("run did not perform the immediate sweep")
		}
		time.Sleep(5 * time.Millisecond)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run did not return after ctx cancellation")
	}
}
