// Package flow 实现基于 TCP 流的分布式抓包处理。
// 包含从 ClickHouse 轮询数据、TCP 流重组、批量输出到 ClickHouse 的完整流水线。
package flow

import (
	"context"
	"time"

	"zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/network/tcp"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errgroupx"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// flowClusterHandler 分片级别的 TCP 流处理器，实现 distributed.ClusterHandler 接口。
// 负责从 ClickHouse 轮询数据包，通过 TCP Router 重组 TCP 流，
// 并将解析出的 HttpTransport 输出到 BatchWriter 进行批量写入。
type flowClusterHandler[P tcp.TCPPacket] struct {
	db     *database.DB
	logger *logging.Logger
	writer *BatchWriter
	param  *Param
}

func newFlowClusterHandler[P tcp.TCPPacket](
	db *database.DB,
	logger *logging.Logger,
	writer *BatchWriter,
	param *Param,
) *flowClusterHandler[P] {
	return &flowClusterHandler[P]{
		db:     db,
		logger: logger,
		writer: writer,
		param:  param,
	}
}

func (h *flowClusterHandler[P]) Poll(ctx context.Context, prevWatermark time.Time, vnodeIndices []int, items chan<- P) error {
	endWatermark := time.Now().Add(-h.param.PollOffset)
	var packets []P
	query := h.db.WithContext(ctx)
	if len(vnodeIndices) == 0 || len(vnodeIndices) == h.param.VNodeCount {
		query = query.Where("timestamp > ? AND timestamp <= ?", prevWatermark, endWatermark)
	} else {
		query = h.db.WithContext(ctx).
			Where(
				"timestamp > ? AND timestamp <= ? AND (intHash64(stream_id) % ?) IN ?",
				prevWatermark,
				endWatermark,
				h.param.VNodeCount,
				vnodeIndices,
			)
	}
	query = query.Order("timestamp ASC").
		FindInBatches(&packets, h.param.BatchSize, func(tx *database.DB, batch int) error {
			for _, packet := range packets {
				select {
				case <-ctx.Done():
					return nil
				case items <- packet:
				}
			}
			return nil
		})
	if err := query.Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (h *flowClusterHandler[P]) Watermark(item P) time.Time {
	return item.FrameInfo().Timestamp
}

func (h *flowClusterHandler[P]) Start(ctx context.Context, items <-chan P) error {
	router := tcp.NewRouter[P](
		h.param.MaxReassemblerSize,
		h.param.MaxOutOrderPacketSize,
		h.param.NoticeTimeout,
	)
	dstChan := make(chan any, h.param.MaxReassemblerSize)

	wg, shardCtx := errgroupx.WithContext(ctx)
	wg.GoNamed("router", func() error {
		defer close(dstChan)
		return router.Run(shardCtx, h.logger, items, dstChan)
	})
	wg.GoNamed("output", func() error {
		for {
			select {
			case <-shardCtx.Done():
				return nil
			case transport, ok := <-dstChan:
				if !ok {
					return nil
				}
				switch t := transport.(type) {
				case *network.HttpTransport:
					if err := h.writer.Write(shardCtx, t); err != nil {
						return errors.WithStack(err)
					}
				default:

				}
			}
		}
	})
	return wg.Wait()
}

func (h *flowClusterHandler[P]) Shutdown() {
	h.writer.Shutdown()
}
