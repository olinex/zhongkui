package etl

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// fakeTask 测试用任务实现，满足 etl.Task 接口。
type fakeTask struct {
	typ     string
	payload []byte
}

// Type 返回任务类型标识。
func (t *fakeTask) Type() string { return t.typ }

// Payload 返回任务载荷。
func (t *fakeTask) Payload() []byte { return t.payload }

// TestChainBasic 验证数据处理链完整流转：任务透传 → extract → Map → Buffer → load。
// 10 个数据项经 ×2 转换后按 5 个一批聚合，最终全部加载且任务原样透传给提取函数。
func TestChainBasic(t *testing.T) {
	task := &fakeTask{typ: "test:chain", payload: []byte("payload")}
	gotTask := make(chan Task, 1)
	extract := func(ctx context.Context, tk Task, out chan<- any) error {
		gotTask <- tk
		if tk.Type() != "test:chain" {
			t.Errorf("task type = %s, want test:chain", tk.Type())
		}
		if string(tk.Payload()) != "payload" {
			t.Errorf("task payload = %s, want payload", tk.Payload())
		}
		for i := 0; i < 10; i++ {
			select {
			case out <- i:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}
	transforms := []TaskTransformFunc[any]{
		func(ctx context.Context, v any) (any, error) { return v.(int) * 2, nil },
	}
	collect := &collectLoad[any]{}
	chain := NewChain(
		5,
		50*time.Millisecond,
		extract,
		transforms,
		[]TaskLoadFunc[any]{collect.Load},
	)

	loadedCount, err := chain.Run(context.Background(), task)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if loadedCount != 10 {
		t.Fatalf("loadedCount = %d, want 10", loadedCount)
	}
	if got := <-gotTask; got != task {
		t.Fatal("task passed to extract is not the same instance")
	}
	// 跨批次的 load 并发执行，不保证全局顺序，排序后比较多集
	got := collect.Batches()
	sort.Slice(got, func(i, j int) bool {
		return got[i].(int) < got[j].(int)
	})
	want := []any{0, 2, 4, 6, 8, 10, 12, 14, 16, 18}
	if len(got) != len(want) {
		t.Fatalf("loaded items = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("loaded items = %v, want %v", got, want)
		}
	}
}

// TestChainExtractError 验证提取函数出错时链返回该错误。
func TestChainExtractError(t *testing.T) {
	sentinel := errors.New("extract boom")
	extract := func(ctx context.Context, task Task, out chan<- any) error {
		return sentinel
	}
	chain := NewChain(5, time.Hour, extract, nil, nil)

	_, err := chain.Run(context.Background(), &fakeTask{typ: "test:error"})
	if err == nil {
		t.Fatal("Run returned nil, want extract error")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("Run error = %v, want sentinel", err)
	}
}

// TestChainTransformError 验证转换阶段出错时整条链被取消并返回第一个错误，
// 且出错前已提取的数据仍被加载（半批冲刷语义）。
func TestChainTransformError(t *testing.T) {
	sentinel := errors.New("transform boom")
	extract := func(ctx context.Context, task Task, out chan<- any) error {
		for i := range 10 {
			select {
			case out <- i:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}
	transforms := []TaskTransformFunc[any]{
		func(ctx context.Context, v any) (any, error) {
			if v.(int) == 3 {
				return nil, sentinel
			}
			return v, nil
		},
	}
	collect := &collectLoad[any]{}
	chain := NewChain(
		100,
		time.Hour,
		extract,
		transforms,
		[]TaskLoadFunc[any]{collect.Load},
	)

	_, err := chain.Run(context.Background(), &fakeTask{typ: "test:transform-error"})
	if err == nil {
		t.Fatal("Run returned nil, want transform error")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("Run error = %v, want sentinel", err)
	}
	// 出错前已转换的 [0 1 2] 应被冲刷加载
	got := collect.Batches()
	if len(got) != 3 {
		t.Fatalf("loaded items = %v, want [0 1 2] flushed on stage error", got)
	}
}

// TestChainEmptyExtract 验证数据源无输出时链正常结束，加载数量为 0。
func TestChainEmptyExtract(t *testing.T) {
	extract := func(ctx context.Context, task Task, out chan<- any) error {
		return nil
	}
	collect := &collectLoad[any]{}
	chain := NewChain(5, time.Hour, extract, nil, []TaskLoadFunc[any]{collect.Load})

	loadedCount, err := chain.Run(context.Background(), &fakeTask{typ: "test:empty"})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if loadedCount != 0 {
		t.Fatalf("loadedCount = %d, want 0", loadedCount)
	}
	if got := collect.Batches(); len(got) != 0 {
		t.Fatalf("loaded items = %v, want none", got)
	}
}

// TestChainMultiTransform 验证多个转换阶段按声明顺序串联应用。
// 泛型类型使用 int，覆盖类型化链（非 any）的编译与流转路径。
func TestChainMultiTransform(t *testing.T) {
	extract := func(ctx context.Context, task Task, out chan<- int) error {
		for i := range 5 {
			select {
			case out <- i:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}
	transforms := []TaskTransformFunc[int]{
		func(ctx context.Context, v int) (int, error) { return v * 2, nil },
		func(ctx context.Context, v int) (int, error) { return v + 1, nil },
	}
	collect := &collectLoad[int]{}
	chain := NewChain(
		5,
		time.Hour,
		extract,
		transforms,
		[]TaskLoadFunc[int]{collect.Load},
	)

	loadedCount, err := chain.Run(context.Background(), &fakeTask{typ: "test:multi-transform"})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if loadedCount != 5 {
		t.Fatalf("loadedCount = %d, want 5", loadedCount)
	}
	got := collect.Batches()
	want := []int{1, 3, 5, 7, 9}
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

// TestChainMultiLoad 验证多个加载函数并行消费同一批次，每个加载函数都收到全部数据项。
func TestChainMultiLoad(t *testing.T) {
	extract := func(ctx context.Context, task Task, out chan<- any) error {
		for i := range 10 {
			select {
			case out <- i:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}
	collectA := &collectLoad[any]{}
	collectB := &collectLoad[any]{}
	chain := NewChain(
		5,
		time.Hour,
		extract,
		nil,
		[]TaskLoadFunc[any]{collectA.Load, collectB.Load},
	)

	loadedCount, err := chain.Run(context.Background(), &fakeTask{typ: "test:multi-load"})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if loadedCount != 10 {
		t.Fatalf("loadedCount = %d, want 10", loadedCount)
	}
	if got := collectA.Batches(); len(got) != 10 {
		t.Fatalf("load A items = %d, want 10", len(got))
	}
	if got := collectB.Batches(); len(got) != 10 {
		t.Fatalf("load B items = %d, want 10", len(got))
	}
}

// TestChainLoadError 验证加载阶段出错时链返回该错误，
// 且已派发给 load 阶段的数据项数量仍被返回（loadedCount 不丢失）。
func TestChainLoadError(t *testing.T) {
	sentinel := errors.New("load boom")
	extract := func(ctx context.Context, task Task, out chan<- any) error {
		for i := range 5 {
			select {
			case out <- i:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}
	chain := NewChain(
		10,
		time.Hour,
		extract,
		nil,
		[]TaskLoadFunc[any]{
			func(ctx context.Context, items []any) error { return sentinel },
		},
	)

	loadedCount, err := chain.Run(context.Background(), &fakeTask{typ: "test:load-error"})
	if err == nil {
		t.Fatal("Run returned nil, want load error")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("Run error = %v, want sentinel", err)
	}
	if loadedCount != 5 {
		t.Fatalf("loadedCount = %d, want 5 (dispatched items on error)", loadedCount)
	}
}

// TestChainCancelFlush 验证任务上下文取消时，已提取未达批量的数据被冲刷加载，不丢失。
func TestChainCancelFlush(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sent := make(chan struct{})
	extract := func(ctx context.Context, task Task, out chan<- any) error {
		for i := range 3 {
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
	collect := &collectLoad[any]{}
	chain := NewChain(10, time.Hour, extract, nil, []TaskLoadFunc[any]{collect.Load})

	result := make(chan error, 1)
	go func() {
		_, err := chain.Run(ctx, &fakeTask{typ: "test:cancel"})
		result <- err
	}()

	<-sent
	cancel()
	if err := <-result; err == nil {
		t.Fatal("Run returned nil, want error on cancel")
	}
	if got := collect.Batches(); len(got) != 3 {
		t.Fatalf("loaded items = %v, want [0 1 2] flushed on cancel", got)
	}
}

// TestChainBufferTimeoutFlush 验证未达批量大小时，超时触发冲刷（Timer 语义）。
// 提取函数在首项被冲刷前阻塞，确保中间批次只可能由超时触发，结果确定性。
func TestChainBufferTimeoutFlush(t *testing.T) {
	firstFlushed := make(chan struct{})
	extract := func(ctx context.Context, task Task, out chan<- any) error {
		send := func(v int) error {
			select {
			case out <- v:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if err := send(0); err != nil {
			return err
		}
		select {
		case <-firstFlushed:
		case <-ctx.Done():
			return ctx.Err()
		}
		if err := send(1); err != nil {
			return err
		}
		return send(2)
	}

	var mu sync.Mutex
	var got []any
	loadCalls := 0
	load := func(ctx context.Context, items []any) error {
		mu.Lock()
		got = append(got, items...)
		loadCalls++
		for _, v := range items {
			if v == 0 {
				select {
				case <-firstFlushed:
				default:
					close(firstFlushed)
				}
			}
		}
		mu.Unlock()
		return nil
	}
	chain := NewChain[any](10, 30*time.Millisecond, extract, nil, []TaskLoadFunc[any]{load})

	loadedCount, err := chain.Run(context.Background(), &fakeTask{typ: "test:timeout-flush"})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if loadedCount != 3 {
		t.Fatalf("loadedCount = %d, want 3", loadedCount)
	}
	mu.Lock()
	defer mu.Unlock()
	if loadCalls != 2 {
		t.Fatalf("load calls = %d, want 2 (timeout flush [0] + close flush [1 2])", loadCalls)
	}
	if len(got) != 3 {
		t.Fatalf("loaded items = %v, want 3 items", got)
	}
}

// TestChainBatchSizes 验证数量聚合边界：数据达到缓冲区大小时整批输出，
// 末尾不足一批的余量在数据源关闭时冲刷。
func TestChainBatchSizes(t *testing.T) {
	extract := func(ctx context.Context, task Task, out chan<- any) error {
		for i := 0; i < 10; i++ {
			select {
			case out <- i:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}
	var mu sync.Mutex
	var sizes []int
	load := func(ctx context.Context, items []any) error {
		mu.Lock()
		sizes = append(sizes, len(items))
		mu.Unlock()
		return nil
	}
	chain := NewChain[any](3, time.Hour, extract, nil, []TaskLoadFunc[any]{load})

	loadedCount, err := chain.Run(context.Background(), &fakeTask{typ: "test:batch-sizes"})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if loadedCount != 10 {
		t.Fatalf("loadedCount = %d, want 10", loadedCount)
	}
	mu.Lock()
	sort.Ints(sizes)
	mu.Unlock()
	want := []int{1, 3, 3, 3}
	if len(sizes) != len(want) {
		t.Fatalf("batch sizes = %v, want %v", sizes, want)
	}
	for i := range want {
		if sizes[i] != want[i] {
			t.Fatalf("batch sizes = %v, want %v", sizes, want)
		}
	}
}

// fakeHandler 测试用 Handler 实现，便于验证 HandlerRegister 与 HandlerRegistry 的注册/注入链路。
type fakeHandler struct {
	do HandlerFunc
}

// Do 转调配置的处理器函数执行任务。
func (f *fakeHandler) Do(ctx context.Context, task Task) error {
	return f.do(ctx, task)
}

// TestHandlerRegisterRegistry 验证 HandlerRegister 注册后，
// HandlerRegistry 能将同名配置注入上下文供 GetHandlerParam 读取。
func TestHandlerRegisterRegistry(t *testing.T) {
	// 保存并清空全局注册表，避免测试间相互污染
	backup := handlerRegistry
	handlerRegistry = newRegistry[HandlerFunc]()
	defer func() { handlerRegistry = backup }()

	type cfg struct {
		Name string `mapstructure:"name"`
	}
	var got string
	HandlerRegister("test:task", &fakeHandler{
		do: func(ctx context.Context, task Task) error {
			c, err := GetHandlerParam[cfg](ctx)
			if err != nil {
				return err
			}
			got = c.Name
			return nil
		},
	})

	reg := HandlerRegistry(map[string]map[string]any{
		"test:task": {"name": "wrapped"},
	})
	h := reg["test:task"]
	if h == nil {
		t.Fatal("handler not registered")
	}
	if err := h(context.Background(), &fakeTask{typ: "test:task"}); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if got != "wrapped" {
		t.Fatalf("injected param name = %s, want wrapped", got)
	}
}
