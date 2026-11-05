package etl

import (
	"context"
	"time"
)

// Chain 任务型批量数据处理链。
// 将任务提取（TaskExtractFunc）、逐项转换（transforms）、批量聚合（buffer）与
// 批量加载（loads）编排为可复用的数据处理链，是批量任务 Handler 的流水线内核。
// 复用 Pipeline 的 channel + errgroup 编排语义，仅将数据源从通用 ExtractFunc[T]
// 收敛为绑定任务（Task）的 TaskExtractFunc[T]，并在 Chain.Run 中完成适配。
type Chain[T any] struct {
	bufferCount   int                    // 缓冲区大小
	bufferTimeout time.Duration          // 缓冲区超时时间
	extract       TaskExtractFunc[T]     // 数据提取函数
	transforms    []TaskTransformFunc[T] // 数据转换函数列表
	loads         []TaskLoadFunc[T]      // 数据加载函数列表
}

// NewChain 创建任务型批量数据处理链。
// bufferCount 指定缓冲区大小，bufferTimeout 指定缓冲区超时时间，
// extract 是数据提取函数，transforms 是数据转换函数列表，loads 是数据加载函数列表。
func NewChain[T any](
	bufferCount int,
	bufferTimeout time.Duration,
	extract TaskExtractFunc[T],
	transforms []TaskTransformFunc[T],
	loads []TaskLoadFunc[T],
) *Chain[T] {
	return &Chain[T]{
		bufferCount:   bufferCount,
		bufferTimeout: bufferTimeout,
		extract:       extract,
		transforms:    transforms,
		loads:         loads,
	}
}

// Run 执行数据处理链，阻塞直到所有阶段完成或任一阶段失败。
// task 透传给提取函数，供其读取任务类型与载荷；
// 返回成功派发给 load 阶段的数据项总数。
//
// 错误语义与 Pipeline.Run 一致：任一阶段返回 error 时，链内 ctx 被取消，
// 已冲刷的半批与已派发的批量仍会被加载完成（保证已提取数据不丢）。
func (c *Chain[T]) Run(ctx context.Context, task Task) (int, error) {
	// 绑定任务上下文：将任务型提取函数适配为通用流水线 ExtractFunc[T]
	adaptExtract := func(ctx context.Context, out chan<- T) error {
		return c.extract(ctx, task, out)
	}

	// 转换阶段：业务 TaskTransformFunc 包装为 etl.Map 阶段
	stages := make([]Stage[T, T], 0, len(c.transforms))
	for _, transform := range c.transforms {
		stages = append(stages, Map(transform))
	}

	pl := NewPipeline(
		adaptExtract,
		stages,
		Buffer[T](c.bufferCount, c.bufferTimeout),
		c.loads,
		c.bufferCount,
	)
	return pl.Run(ctx)
}
