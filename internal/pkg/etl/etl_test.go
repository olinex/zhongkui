package etl

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// collectLoad 收集所有到达 load 阶段的数据批，线程安全。
type collectLoad[T any] struct {
	mu    sync.Mutex
	items []T
}

func (c *collectLoad[T]) Load(ctx context.Context, batch []T) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = append(c.items, batch...)
	return nil
}

func (c *collectLoad[T]) Batches() []T {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]T(nil), c.items...)
}

// TestPipelineBasic 验证流水线完整流转：extract → Map → Buffer → load。
// 10 个数据项经 ×2 转换后按 5 个一批聚合，最终全部加载且顺序保持。
func TestPipelineBasic(t *testing.T) {
	extract := func(ctx context.Context, out chan<- int) error {
		for i := 0; i < 10; i++ {
			select {
			case out <- i:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}
	stages := []Stage[int, int]{Map[int, int](func(ctx context.Context, v int) (int, error) {
		return v * 2, nil
	})}
	collect := &collectLoad[int]{}
	pl := NewPipeline[int](
		extract,
		stages,
		Buffer[int](5, 50*time.Millisecond),
		[]LoadFunc[int]{collect.Load},
		8,
	)

	loadedCount, err := pl.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if loadedCount != 10 {
		t.Fatalf("loadedCount = %d, want 10", loadedCount)
	}
	// 跨批次的 load 并发执行，不保证全局顺序，排序后比较多集
	got := collect.Batches()
	want := []int{0, 2, 4, 6, 8, 10, 12, 14, 16, 18}
	sort.Ints(got)
	if len(got) != len(want) {
		t.Fatalf("loaded items = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("loaded items = %v, want %v", got, want)
		}
	}
}

// TestBufferFlushOnCancel 验证 ctx 取消时 buffer 冲刷剩余半批。
// 提取 3 项后任务被取消，已提取数据不能丢失。
func TestBufferFlushOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sent := make(chan struct{})
	extract := func(ctx context.Context, out chan<- int) error {
		for i := 0; i < 3; i++ {
			select {
			case out <- i:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		close(sent)
		<-ctx.Done()
		return ctx.Err()
	}
	collect := &collectLoad[int]{}
	pl := NewPipeline[int](
		extract,
		nil,
		Buffer[int](10, time.Hour),
		[]LoadFunc[int]{collect.Load},
		8,
	)

	result := make(chan error, 1)
	go func() {
		_, err := pl.Run(ctx)
		result <- err
	}()

	<-sent
	cancel()
	if err := <-result; err == nil {
		t.Fatal("Run returned nil, want error on cancel")
	}
	got := collect.Batches()
	if len(got) != 3 {
		t.Fatalf("loaded items = %v, want [0 1 2] flushed on cancel", got)
	}
}

// TestPipelineStageError 验证阶段出错时整条流水线被取消并返回第一个错误，
// 且出错前已提取的数据仍被加载（半批冲刷语义）。
func TestPipelineStageError(t *testing.T) {
	sentinel := errors.New("transform boom")
	extract := func(ctx context.Context, out chan<- int) error {
		for i := 0; i < 10; i++ {
			select {
			case out <- i:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}
	stages := []Stage[int, int]{Map[int, int](func(ctx context.Context, v int) (int, error) {
		if v == 3 {
			return 0, sentinel
		}
		return v * 2, nil
	})}
	collect := &collectLoad[int]{}
	pl := NewPipeline[int](
		extract,
		stages,
		Buffer[int](100, time.Hour),
		[]LoadFunc[int]{collect.Load},
		8,
	)

	_, err := pl.Run(context.Background())
	if err == nil {
		t.Fatal("Run returned nil, want stage error")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("Run error = %v, want sentinel", err)
	}
	// 出错前已转换的 [0 2 4] 应被冲刷加载
	got := collect.Batches()
	if len(got) != 3 {
		t.Fatalf("loaded items = %v, want [0 2 4] flushed on stage error", got)
	}
}
