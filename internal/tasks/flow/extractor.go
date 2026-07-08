package flow

import (
	"context"
	"time"

	"sync/atomic"

	"github.com/reactivex/rxgo/v2"
	"github.com/samber/do/v2"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
	"zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	"zhongkui.com/zhongkui/internal/provider"
	"zhongkui.com/zhongkui/internal/tasks/totel"
)

func ExtractIPv4HttpTransportFromTable(ctx context.Context, task *pipeline.Task, channel chan<- rxgo.Item) error {
	taskType := task.Type()
	ctx, span := totel.StartExtractSpan(ctx, taskType)
	var extractedCount int64
	var err error
	defer func() {
		totel.RecordItemsExtracted(ctx, taskType, int(extractedCount))
		totel.EndSpan(span, &err)
		if err != nil {
			totel.RecordTaskError(ctx, taskType)
		}
	}()

	scope := pipeline.GetScope(ctx)
	router, e := do.Invoke[*IPv4TCPRouter](scope)
	if e != nil {
		err = e
		return errors.WithStack(e)
	}
	logger, e := provider.InvokePipelineLogger(scope)
	if e != nil {
		err = e
		return errors.WithStack(e)
	}
	db, e := provider.InvokeRawDatabase(scope)
	if e != nil {
		err = e
		return errors.WithStack(e)
	}
	param, e := pipeline.GetTaskParam[Param](ctx)
	if e != nil {
		err = e
		return errors.WithStack(e)
	}
	end := time.Now().Truncate(param.WindowRange)
	start := end.Add(-param.WindowRange)
	logger.Ctx(ctx).Infof("start extract ipv4 http transport from table, window [%s, %s), maxReassemblerSize=%d",
		start.Format(time.RFC3339), end.Format(time.RFC3339), param.MaxReassemblerSize)
	var queriedCount int64

	wg, ctx := errgroup.WithContext(ctx)
	srcChan := make(chan *network.PacketIPv4TCP, param.MaxReassemblerSize)
	dstChan := make(chan any, param.MaxReassemblerSize)
	// 启动转换任务
	wg.Go(func() error {
		for tmp := range dstChan {
			transport := tmp.(*network.HttpTransport)
			atomic.AddInt64(&extractedCount, 1)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case channel <- rxgo.Of(transport):
			}
		}
		return nil
	})
	// 启动路由任务
	wg.Go(func() error {
		defer close(dstChan)
		return router.Run(ctx, logger, srcChan, dstChan)
	})
	// 启动查询任务
	wg.Go(func() error {
		defer close(srcChan)
		var packets = make([]network.PacketIPv4TCP, 0, param.MaxReassemblerSize)
		if e := db.
			WithContext(ctx).
			Where("timestamp >= ? AND timestamp < ?", start, end).
			FindInBatches(&packets, param.MaxReassemblerSize, func(tx *gorm.DB, batch int) error {
				for _, packet := range packets {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case srcChan <- &packet:
						atomic.AddInt64(&queriedCount, 1)
					}
				}
				return nil
			}).Error; e != nil {
			return errors.WithStack(e)
		}
		return nil
	})
	err = wg.Wait()
	logger.Ctx(ctx).Infof("finish extract ipv4 http transport from table, queried=%d, extracted=%d, err=%v",
		atomic.LoadInt64(&queriedCount), atomic.LoadInt64(&extractedCount), err)
	return err
}

func ExtractIPv6HttpTransportFromTable(ctx context.Context, task *pipeline.Task, channel chan<- rxgo.Item) error {
	return nil
}
