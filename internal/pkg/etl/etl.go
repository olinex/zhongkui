// Package etl 提供 golang-first 的泛型数据处理流水线模型。
// 使用原生 channel 与 errgroup 表达 Extract/Transform/Load 三个阶段，
// 替代响应式框架（rxgo）的算子与调度语义，goroutine 生命周期完全由调用方掌控。
package etl

import (
	"context"
	"fmt"
	"time"

	"zhongkui.com/zhongkui/internal/pkg/errgroupx"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// ExtractFunc 数据提取函数类型。
// 从数据源提取数据并通过 out 通道输出；返回后由 Pipeline 负责关闭 out。
type ExtractFunc[T any] = func(ctx context.Context, out chan<- T) error

// LoadFunc 数据加载函数类型。
// 消费一批聚合后的数据项，返回 error 表示加载失败。
type LoadFunc[T any] = func(ctx context.Context, batch []T) error

// Stage 数据处理阶段接口。
// 消费 in 通道的数据，处理后将结果写入 out 通道。
//
// 流转约定：
//   - 每个 Stage 负责关闭自己产出的 out 通道（Process 返回前应 close(out)）
//   - 消费方只通过 for range 读取 in，无需感知上游的关闭时机
//   - 实现必须监听 ctx.Done，发送数据必须使用 select 而非裸 send，避免取消时 goroutine 泄漏
type Stage[I, O any] interface {
	// Process 消费 in 中的数据，处理后写入 out；返回前负责关闭 out。
	Process(ctx context.Context, in <-chan I, out chan<- O) error
}

// mapStage 纯函数转换阶段，将 fn 应用到每个输入项。
type mapStage[I, O any] struct {
	fn func(ctx context.Context, in I) (O, error)
}

// Process 对每个输入项应用转换函数，逐项写入 out。
func (s *mapStage[I, O]) Process(ctx context.Context, in <-chan I, out chan<- O) error {
	defer close(out)
	for {
		select {
		case <-ctx.Done():
			return errors.WithStack(ctx.Err())
		case v, ok := <-in:
			if !ok {
				return nil
			}
			o, err := s.fn(ctx, v)
			if err != nil {
				return err
			}
			select {
			case out <- o:
			case <-ctx.Done():
				return errors.WithStack(ctx.Err())
			}
		}
	}
}

// Map 将纯函数包装为 Stage。
// fn 对每个输入项做一次转换，适用于无状态的逐项映射（如数据包到模型）。
func Map[I, O any](fn func(ctx context.Context, in I) (O, error)) Stage[I, O] {
	return &mapStage[I, O]{fn: fn}
}

// bufferStage 按数量或超时时间聚合数据项的 Stage。
// size 满或 timeout 到达时输出一批；ctx 取消时冲刷剩余数据后退出，不丢已提取数据。
type bufferStage[T any] struct {
	size    int
	timeout time.Duration
}

// Process 按数量或超时聚合输入项，满批后写入 out。
func (s *bufferStage[T]) Process(ctx context.Context, in <-chan T, out chan<- []T) error {
	defer close(out)

	batch := make([]T, 0, s.size)
	var timer *time.Timer
	var timerC <-chan time.Time
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	// flush 将当前批次发送到 out，发送前清空累计批次。
	// 使用阻塞发送而非 select：load 阶段在 batchCh 关闭前持续消费，发送必然被接收；
	// 若与 ctx.Done 竞争，select 会随机丢弃批次，违反"半批冲刷不丢"约定。
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		items := batch
		batch = make([]T, 0, s.size)
		out <- items
		return nil
	}
	// 复用单个 Timer，避免每次发批后重新分配（定时器堆操作 + GC 压力）。
	// Stop 后需非阻塞排空已触发未读取的陈旧值，防止 Reset 复用通道时残留值立即触发超时。
	stopTimer := func() {
		if timer != nil {
			timer.Stop()
			select {
			case <-timerC:
			default:
			}
		}
		timerC = nil
	}
	restartTimer := func() {
		if timer == nil {
			timer = time.NewTimer(s.timeout)
		} else {
			timer.Reset(s.timeout)
		}
		timerC = timer.C
	}

	for {
		select {
		case <-ctx.Done():
			// 排空 in 通道中的剩余数据后冲刷，保证已提取数据不丢。
			// 上游阶段退出前都会关闭 out，因此排空必然有界。
			for v := range in {
				batch = append(batch, v)
			}
			if err := flush(); err != nil {
				return err
			}
			return ctx.Err()
		case <-timerC:
			stopTimer()
			if err := flush(); err != nil {
				return err
			}
		case v, ok := <-in:
			if !ok {
				return flush()
			}
			batch = append(batch, v)
			if timerC == nil {
				restartTimer()
			}
			if len(batch) >= s.size {
				stopTimer()
				if err := flush(); err != nil {
					return err
				}
			}
		}
	}
}

// Buffer 创建按数量或超时聚合的 Stage。
// 聚合 T 为 []T 批量输出；size 满或 timeout 到达时发批，ctx 取消时冲刷剩余半批。
func Buffer[T any](size int, timeout time.Duration) Stage[T, []T] {
	return &bufferStage[T]{size: size, timeout: timeout}
}

// Pipeline 流水线执行器，串联 extract → stages → buffer → loads。
// 所有阶段由 errgroup 管理：任一阶段出错即取消整条流水线并返回第一个错误。
type Pipeline[T any] struct {
	extract  ExtractFunc[T]
	stages   []Stage[T, T]
	buffer   Stage[T, []T]
	loads    []LoadFunc[T]
	chanSize int
}

// NewPipeline 创建流水线执行器。
// extract 为数据提取函数，stages 为逐项转换阶段，buffer 为批量聚合阶段，
// loads 为批量加载函数列表，chanSize 为阶段间通道的缓冲大小。
func NewPipeline[T any](
	extract ExtractFunc[T],
	stages []Stage[T, T],
	buffer Stage[T, []T],
	loads []LoadFunc[T],
	chanSize int,
) *Pipeline[T] {
	return &Pipeline[T]{
		extract:  extract,
		stages:   stages,
		buffer:   buffer,
		loads:    loads,
		chanSize: chanSize,
	}
}

// Run 执行流水线，阻塞直到所有阶段完成或任一阶段失败。
//
// 错误语义：任一阶段返回 error 时，errgroup 取消 ctx，上游阶段立即退出；
// buffer 已冲刷的半批与已派发的批量仍会被加载完成（保证已提取数据不丢）。
// 返回的 loadedCount 为成功派发给 load 阶段的数据项总数。
func (p *Pipeline[T]) Run(ctx context.Context) (loadedCount int, err error) {
	wg, ctx := errgroupx.WithContext(ctx)

	// 数据提取阶段（生产者），返回后关闭 source
	source := make(chan T, p.chanSize)
	wg.GoNamed("extract", func() error {
		defer close(source)
		return p.extract(ctx, source)
	})

	// 逐项转换阶段链，前一个阶段的产出作为后一个阶段的输入
	prev := source
	for i, stage := range p.stages {
		in := prev
		out := make(chan T, p.chanSize)
		wg.GoNamed(
			fmt.Sprintf("stage-%d", i),
			func() error {
				return stage.Process(ctx, in, out)
			})
		prev = out
	}

	// 聚合阶段：T → []T，产出批量数据
	batchCh := make(chan []T, p.chanSize)
	wg.GoNamed("buffer", func() error {
		return p.buffer.Process(ctx, prev, batchCh)
	})

	// 加载阶段（消费者）：严格按序消费批量，每个批次内的 loads 并发执行。
	// 使用 for range 而非 select：既保证批次顺序，又能在 ctx 取消时
	// 排空 buffer 冲刷的半批（batchCh 在 buffer 退出时关闭）。
	wg.GoNamed("load", func() error {
		innerWG, innerCtx := errgroupx.WithContext(ctx)
		for batch := range batchCh {
			loadedCount += len(batch)
			for i, load := range p.loads {
				innerWG.GoNamed(
					fmt.Sprintf("load-%d", i),
					func() error {
						return load(innerCtx, batch)
					})
			}
		}
		if innerErr := innerWG.Wait(); innerErr != nil {
			return errors.WithStack(innerErr)
		}
		return nil
	})

	if err := wg.Wait(); err != nil {
		return loadedCount, errors.WithStack(err)
	}
	return loadedCount, nil
}
