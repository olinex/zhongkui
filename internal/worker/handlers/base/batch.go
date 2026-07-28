// Package base 提供分布式版本化任务处理的基础 Handler 实现。
// 包含基于 etcd 版本 CAS 机制的版本化 Handler（versionHandler）
// 和通用的批量数据流水线 Handler（BatchHandler）。
package base

import (
	"context"
	"time"

	"github.com/reactivex/rxgo/v2"
	"zhongkui.com/zhongkui/internal/pkg/errgroupx"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	"zhongkui.com/zhongkui/internal/worker/totel"
)

// 编译期断言：确保 BatchHandler 实现了 pipeline.Handler 接口
var _ pipeline.Handler = (*BatchHandler)(nil)

func emptyLiveFunc(ctx context.Context, task *pipeline.Task) error {
	return nil
}

// BatchHandler 批量任务处理器，用于处理流水线式的批量数据处理任务。
// 支持数据提取、转换和加载三个阶段的流水线处理。
type BatchHandler struct {
	bufferCount   int                      // 缓冲区大小
	bufferTimeout time.Duration            // 缓冲区超时时间
	preE          pipeline.LiveFunc        // 数据处理前
	postE         pipeline.LiveFunc        // 数据处理后
	extract       pipeline.ExtractFunc     // 数据提取函数
	transforms    []pipeline.TransformFunc // 数据转换函数列表
	loads         []pipeline.LoadFunc      // 数据加载函数列表
}

// NewBatchHandler 创建一个新的批量任务处理器。
// bufferCount 指定缓冲区大小，bufferTimeout 指定缓冲区超时时间，
// extract 是数据提取函数，transforms 是数据转换函数列表，loads 是数据加载函数列表。
func NewBatchHandler(
	bufferCount int,
	bufferTimeout time.Duration,
	extract pipeline.ExtractFunc,
	transforms []pipeline.TransformFunc,
	loads []pipeline.LoadFunc,
	preE pipeline.LiveFunc,
	postE pipeline.LiveFunc,
) *BatchHandler {
	totel.InitMetrics()
	if preE == nil {
		preE = emptyLiveFunc
	}
	if postE == nil {
		postE = emptyLiveFunc
	}
	return &BatchHandler{
		bufferCount:   bufferCount,
		bufferTimeout: bufferTimeout,
		preE:          preE,
		postE:         postE,
		extract:       extract,
		transforms:    transforms,
		loads:         loads,
	}
}

// Do 执行批量任务处理。
// 启动数据提取、数据转换和数据加载的流水线处理流程。
func (w *BatchHandler) Do(ctx context.Context, task *pipeline.Task) (err error) {
	taskType := task.Type()
	ctx, span := totel.StartBatchTaskSpan(ctx, taskType)
	defer totel.EndSpan(span, &err)
	defer func() {
		if err != nil {
			totel.RecordTaskError(ctx, taskType)
		} else {
			totel.RecordTask(ctx, taskType)
		}
	}()

	err = w.preE(ctx, task)
	if err != nil {
		return errors.WithStack(err)
	}
	defer func() {
		if pe := w.postE(ctx, task); pe != nil {
			panic(pe)
		}
	}()
	channel := make(chan rxgo.Item, w.bufferCount)
	var loadedCount int
	outterWg, outterCtx := errgroupx.WithContext(ctx)
	outterWg.GoNamed("transform", func() error {
		observe := rxgo.FromChannel(channel)
		for _, transform := range w.transforms {
			observe = observe.Map(transform)
		}
		observe = observe.BufferWithTimeOrCount(rxgo.WithDuration(w.bufferTimeout), w.bufferCount)
		innerWG, innerCtx := errgroupx.WithContext(outterCtx)
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
			loadedCount += len(items)
			for _, load := range w.loads {
				func(load pipeline.LoadFunc) {
					innerWG.GoNamed("load", func() error {
						return load(innerCtx, items)
					})
				}(load)
			}
		}
		if ierr := innerWG.Wait(); ierr != nil {
			return errors.WithStack(ierr)
		}
		return nil
	})
	outterWg.GoNamed("extract", func() error {
		defer close(channel)
		return w.extract(outterCtx, task, channel)
	})
	err = outterWg.Wait()
	if err != nil {
		return err
	}
	totel.RecordItemsLoaded(ctx, taskType, loadedCount)
	return nil
}
