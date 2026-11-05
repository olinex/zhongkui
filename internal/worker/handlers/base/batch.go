// Package base 提供通用的批量数据流水线任务处理 Handler 实现（BatchHandler）。
package base

import (
	"context"
	"time"

	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/etl"
	"zhongkuisec.com/zhongkui/internal/worker/totel"
)

// 编译期断言：确保 BatchHandler 实现了 etl.Handler 接口
var _ etl.Handler = (*BatchHandler)(nil)

func emptyLiveFunc(ctx context.Context, task etl.Task) error {
	return nil
}

// BatchHandler 批量任务处理器，用于处理流水线式的批量数据处理任务。
// 持有 etl.Chain 作为数据处理内核，在 Do 中编排任务级生命周期（指标、前后置钩子）。
type BatchHandler struct {
	chain *etl.Chain[any] // 数据处理链（extract → transforms → buffer → loads）
	preE  etl.LiveFunc    // 数据处理前
	postE etl.LiveFunc    // 数据处理后
}

// NewBatchHandler 创建一个新的批量任务处理器。
// bufferCount 指定缓冲区大小，bufferTimeout 指定缓冲区超时时间，
// extract 是数据提取函数，transforms 是数据转换函数列表，loads 是数据加载函数列表。
func NewBatchHandler(
	bufferCount int,
	bufferTimeout time.Duration,
	extract etl.TaskExtractFunc[any],
	transforms []etl.TaskTransformFunc[any],
	loads []etl.TaskLoadFunc[any],
	preE etl.LiveFunc,
	postE etl.LiveFunc,
) *BatchHandler {
	totel.InitMetrics()
	if preE == nil {
		preE = emptyLiveFunc
	}
	if postE == nil {
		postE = emptyLiveFunc
	}
	return &BatchHandler{
		chain: etl.NewChain(bufferCount, bufferTimeout, extract, transforms, loads),
		preE:  preE,
		postE: postE,
	}
}

// Do 执行批量任务处理。
// 依次执行 preE 前置钩子、数据处理链（etl.Chain）与 postE 后置钩子。
// 数据处理链内部使用 etl.Pipeline 以原生 channel + errgroup 编排三个阶段，
// 不再依赖 rxgo 响应式算子。
func (w *BatchHandler) Do(ctx context.Context, task etl.Task) (err error) {
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

	loadedCount, err := w.chain.Run(ctx, task)
	if err != nil {
		return errors.WithStack(err)
	}
	totel.RecordItemsLoaded(ctx, taskType, loadedCount)
	return nil
}
