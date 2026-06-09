package workers

import (
	"context"
	"time"

	"github.com/reactivex/rxgo/v2"
	"golang.org/x/sync/errgroup"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
)

// BatchTaskWorker 批量任务 worker，用于处理流水线式的批量数据处理任务。
// 支持数据提取、转换和加载三个阶段的流水线处理。
type BatchTaskWorker struct {
	bufferCount   int             // 缓冲区大小
	bufferTimeout time.Duration   // 缓冲区超时时间
	live          []LiveFunc      // 数据处理的函数列表
	extract       ExtractFunc     // 数据提取函数
	transforms    []TransformFunc // 数据转换函数列表
	loads         []LoadFunc      // 数据加载函数列表
}

// NewBatchTaskWorker 创建一个新的批量任务 worker。
// bufferCount 指定缓冲区大小，bufferTimeout 指定缓冲区超时时间，
// extract 是数据提取函数，transforms 是数据转换函数列表，loads 是数据加载函数列表。
func NewBatchTaskWorker(
	bufferCount int,
	bufferTimeout time.Duration,
	extract ExtractFunc,
	transforms []TransformFunc, // 数据转换函数列表
	loads []LoadFunc, // 数据加载函数列表
	live ...LiveFunc, // 数据处理前的函数列表
) *BatchTaskWorker {
	return &BatchTaskWorker{
		bufferCount:   bufferCount,
		bufferTimeout: bufferTimeout,
		live:          live,
		extract:       extract,
		transforms:    transforms,
		loads:         loads,
	}
}

// Do 执行批量任务处理。
// 启动数据提取、数据转换和数据加载的流水线处理流程。
func (w *BatchTaskWorker) Do(ctx context.Context, task *pipeline.Task) error {
	callbacks := make([]func(), 0, len(w.live))
	defer func() {
		for i := len(callbacks) - 1; i >= 0; i-- {
			callbacks[i]()
		}
	}()
	for _, before := range w.live {
		if callback, err := before(ctx, task); err != nil {
			callbacks = append(callbacks, callback)
			return errors.WithStack(err)
		}
	}
	channel := make(chan rxgo.Item, w.bufferCount)
	outterWg, outterCtx := errgroup.WithContext(ctx)
	outterWg.Go(func() error {
		observe := rxgo.FromChannel(channel)
		for _, transform := range w.transforms {
			observe = observe.Map(transform)
		}
		observe = observe.BufferWithTimeOrCount(rxgo.WithDuration(w.bufferTimeout), w.bufferCount)
		innerWG, innerCtx := errgroup.WithContext(ctx)
		for batch := range observe.Observe() {
			if batch.E != nil {
				return batch.E
			}
			items, ok := batch.V.([]any)
			if !ok {
				return errors.Newf("Batch value type error, expect []any, got %T", batch.V)
			}
			if len(items) == 0 {
				continue
			}
			for _, load := range w.loads {
				func(load LoadFunc) {
					innerWG.Go(func() error {
						return load(innerCtx, items)
					})
				}(load)
			}
		}
		if err := innerWG.Wait(); err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
	outterWg.Go(func() error {
		defer close(channel)
		return w.extract(outterCtx, task, channel)
	})
	return outterWg.Wait()
}
