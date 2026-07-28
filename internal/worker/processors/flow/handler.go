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
	db            *database.DB
	logger        *logging.Logger
	writer        *BatchWriter
	param         *Param
	consumedTotal int64 // 累计消费的数据包数量，用于诊断生产-消费速率差异
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

func (h *flowClusterHandler[P]) Poll(ctx context.Context, prevWatermark, curWatermark time.Time, vnodeIndices []int, items chan<- P) error {
	pollStart := time.Now()
	h.logger.Ctx(ctx).Debugf("flow handler poll started, prev=%s, end=%s, vnodeIndices=%v",
		prevWatermark.Format(time.RFC3339Nano),
		curWatermark.Format(time.RFC3339Nano),
		vnodeIndices,
	)

	var packets []P
	totalSent := 0
	query := h.db.WithContext(ctx)
	if len(vnodeIndices) == 0 || len(vnodeIndices) == h.param.VNodeCount {
		query = query.Where("timestamp > ? AND timestamp <= ?", prevWatermark, curWatermark)
	} else {
		query = h.db.WithContext(ctx).
			Where(
				"timestamp > ? AND timestamp <= ? AND (intHash64(stream_id) % ?) IN ?",
				prevWatermark,
				curWatermark,
				h.param.VNodeCount,
				vnodeIndices,
			)
	}
	query = query.Order("timestamp ASC").
		FindInBatches(&packets, h.param.BatchSize, func(tx *database.DB, batch int) error {
			batchSent := 0
			for _, packet := range packets {
				select {
				case <-ctx.Done():
					return nil
				case items <- packet:
					batchSent++
				}
			}
			totalSent += batchSent
			return nil
		})
	if err := query.Error; err != nil {
		h.logger.Ctx(ctx).Errorf("flow handler poll failed, duration=%s, sent=%d, err=%v",
			time.Since(pollStart), totalSent, err)
		return errors.WithStack(err)
	}
	h.logger.Ctx(ctx).Debugf("flow handler poll completed, duration=%s, sent=%d",
		time.Since(pollStart), totalSent)
	return nil
}

func (h *flowClusterHandler[P]) Watermark(item P) time.Time {
	return item.FrameInfo().Timestamp
}

func (h *flowClusterHandler[P]) Start(ctx context.Context, items <-chan P) error {
	h.logger.Ctx(ctx).Infof("flow handler router starting, maxReassemblerSize=%d, maxOutOrderPacketSize=%d, noticeTimeout=%s",
		h.param.MaxReassemblerSize, h.param.MaxOutOrderPacketSize, h.param.NoticeTimeout)

	router := tcp.NewRouter[P](
		h.param.MaxReassemblerSize,
		h.param.MaxOutOrderPacketSize,
		h.param.NoticeTimeout,
	)
	dstChan := make(chan any, h.param.MaxReassemblerSize)

	var transportCount int64
	wg, shardCtx := errgroupx.WithContext(ctx)
	wg.GoNamed("router", func() error {
		defer close(dstChan)
		h.logger.Ctx(shardCtx).Debugf("flow handler router goroutine started")
		err := router.Run(shardCtx, h.logger, items, dstChan)
		if err != nil {
			h.logger.Ctx(shardCtx).Errorf("flow handler router goroutine exited with error: %v", err)
		} else {
			h.logger.Ctx(shardCtx).Infof("flow handler router goroutine exited cleanly")
		}
		return err
	})
	wg.GoNamed("output", func() error {
		h.logger.Ctx(shardCtx).Debugf("flow handler output goroutine started")
		for {
			select {
			case <-shardCtx.Done():
				h.logger.Ctx(shardCtx).Infof("flow handler output goroutine exiting, totalTransport=%d", transportCount)
				return nil
			case transport, ok := <-dstChan:
				if !ok {
					h.logger.Ctx(shardCtx).Infof("flow handler dstChan closed, totalTransport=%d", transportCount)
					return nil
				}
				switch t := transport.(type) {
				case *network.HttpTransport:
					if err := h.writer.Write(shardCtx, t); err != nil {
						h.logger.Ctx(shardCtx).Errorf("flow handler write transport failed: %v", err)
						return errors.WithStack(err)
					}
					transportCount++
				default:
					h.logger.Ctx(shardCtx).Warnf("flow handler unexpected transport type: %T", transport)
				}
			}
		}
	})

	waitErr := wg.Wait()
	h.logger.Ctx(ctx).Infof("flow handler Start exited, totalTransport=%d, err=%v", transportCount, waitErr)
	return waitErr
}

func (h *flowClusterHandler[P]) Shutdown() {
	h.writer.Shutdown()
}
