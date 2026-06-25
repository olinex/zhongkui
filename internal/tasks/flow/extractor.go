package flow

import (
	"context"
	"time"

	"github.com/reactivex/rxgo/v2"
	"github.com/samber/do/v2"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
	"zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	"zhongkui.com/zhongkui/internal/provider"
)

func ExtractIPv4HttpTransportFromTable(ctx context.Context, task *pipeline.Task, channel chan<- rxgo.Item) error {
	scope := pipeline.GetScope(ctx)
	router, err := do.Invoke[*IPv4TCPRouter](scope)
	if err != nil {
		return errors.WithStack(err)
	}
	logger, err := provider.InvokePipelineLogger(scope)
	if err != nil {
		return errors.WithStack(err)
	}
	db, err := provider.InvokeRawDatabase(scope)
	if err != nil {
		return errors.WithStack(err)
	}
	param, err := pipeline.GetTaskParam[Param](ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	wg, ctx := errgroup.WithContext(ctx)
	srcChan := make(chan *network.PacketIPv4TCP, param.MaxReassemblerSize)
	dstChan := make(chan any, param.MaxReassemblerSize)
	// 启动转换任务
	wg.Go(func() error {
		for tmp := range dstChan {
			transport := tmp.(*network.HttpTransport)
			channel <- rxgo.Of(transport)
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
		end := time.Now().Truncate(param.WindowRange)
		start := end.Add(-param.WindowRange)
		err = db.
			WithContext(ctx).
			Where("timestamp >= ? AND timestamp < ?", start, end).
			FindInBatches(&packets, param.MaxReassemblerSize, func(tx *gorm.DB, batch int) error {
				for _, packet := range packets {
					srcChan <- &packet
				}
				return nil
			}).Error
		if err != nil {
			return errors.WithStack(err)
		}
		return nil
	})
	return wg.Wait()
}

func ExtractIPv6HttpTransportFromTable(ctx context.Context, task *pipeline.Task, channel chan<- rxgo.Item) error {
	return nil
}
