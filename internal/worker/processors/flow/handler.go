// Package flow 实现基于 TCP 流的分布式抓包处理。
// 包含从 ClickHouse 轮询数据、TCP 流重组、批量输出到 ClickHouse 的完整流水线。
package flow

import (
	"context"
	"time"

	"zhongkuisec.com/zhongkui/internal/models/network"
	"zhongkuisec.com/zhongkui/internal/network/tcp"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errgroupx"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/etl"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/tagengine"
)

// flowClusterHandler 分片级别的 TCP 流处理器，实现 distributed.ClusterHandler 接口。
// 负责从 ClickHouse 轮询数据包，经 TCP Router 重组 TCP 流后，
// 将解析出的 HttpTransport / NetworkStack 批量写入 ClickHouse。
// 内部处理链（提取 → 重组 → 打标签 → 批量加载）由 etl.Pipeline 统一编排，
// 取代原先手写的 Router + output 分发 + BatchWriter 双缓冲循环。
type flowClusterHandler[P tcp.TCPPacket] struct {
	db        *database.DB
	logger    *logging.Logger
	param     *Param
	tagEngine *tagengine.Engine
}

func newFlowClusterHandler[P tcp.TCPPacket](
	db *database.DB,
	logger *logging.Logger,
	param *Param,
	tagEngine *tagengine.Engine,
) *flowClusterHandler[P] {
	return &flowClusterHandler[P]{
		db:        db,
		logger:    logger,
		param:     param,
		tagEngine: tagEngine,
	}
}

// Poll 按水位区间从 ClickHouse 轮询数据包并投递到 items 通道。
// 仅查询 created_at 落在 (prevWatermark, curWatermark] 且哈希命中 vnodeIndices 的分片，
// 按批次边查询边发送，ctx 取消时提前返回。
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
	// 水位边界必须转换为微秒精度的 UTC 字符串传参：gorm clickhouse 驱动对 time.Time
	// 参数按秒级精度插值（toDateTime），与 DateTime64(6) 列比较时边界判定异常，
	// 导致重复消费或数据丢失（详见 database.ToUTCDatetime64）。
	prevBoundary := database.ToUTCDatetime64(prevWatermark, 6)
	curBoundary := database.ToUTCDatetime64(curWatermark, 6)
	if len(vnodeIndices) == 0 || len(vnodeIndices) == h.param.VNodeCount {
		query = query.Where("created_at > ? AND created_at <= ?", prevBoundary, curBoundary)
	} else {
		query = h.db.WithContext(ctx).
			Where(
				"created_at > ? AND created_at <= ? AND (intHash64(tcp_stream_id) % ?) IN ?",
				prevBoundary,
				curBoundary,
				h.param.VNodeCount,
				vnodeIndices,
			)
	}
	query = query.Order("created_at ASC").
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
		// 仅 ctx 已取消时按关机路径提前返回 nil：返回 nil 会推进水位，非关机场景下的
		// 取消（如驱动/连接中断）必须按错误上报，避免静默跳过数据。
		if ctx.Err() != nil {
			h.logger.Ctx(ctx).Infof("flow handler poll interrupted (context canceled), duration=%s, sent=%d",
				time.Since(pollStart), totalSent)
			return nil
		}
		h.logger.Ctx(ctx).Errorf("flow handler poll failed, duration=%s, sent=%d, err=%v",
			time.Since(pollStart), totalSent, err)
		return errors.WithStack(err)
	}
	h.logger.Ctx(ctx).Debugf("flow handler poll completed, duration=%s, sent=%d",
		time.Since(pollStart), totalSent)
	return nil
}

// Watermark 从数据包中提取水位时间戳。
// 必须返回与 Poll 查询过滤字段（created_at，即入库时间）一致的字段，
// 否则水位与查询下界不匹配会导致重复拉取或遗漏数据。
func (h *flowClusterHandler[P]) Watermark(item P) time.Time {
	return item.IngestedAt()
}

// Start 运行处理管道，阻塞直到 ctx 取消或 items 通道关闭。
// 将 orchestrator 逐窗口投递的 items 通道适配为 etl.Pipeline 的数据源，
// 依次经过 TCP 流重组（routerStage）、标签识别（tagStage）、批量聚合（etl.Buffer）
// 与批量加载（loadBatch）。
// 与原先 hand-rolled 实现相比，聚合/flush/ctx 取消排空的语义由 Pipeline 统一兜底；
// 背压策略为阻塞式：Router 或 Load 慢速时上游 Poll 同步退避。
func (h *flowClusterHandler[P]) Start(ctx context.Context, items <-chan P) error {
	h.logger.Ctx(ctx).Infof("flow handler pipeline starting, maxReassemblerSize=%d, maxOutOrderPacketSize=%d, noticeTimeout=%s",
		h.param.MaxReassemblerSize, h.param.MaxOutOrderPacketSize, h.param.NoticeTimeout)

	// source 适配：编排器逐窗口投递 items，桥接为流水线 ExtractFunc[any]。
	// 泛型流水线以 any 承载产物（HttpTransport / NetworkStack），
	// 上游先做 P → any 的宽化（零成本），窄化回 P 在 routerStage 内完成。
	extract := func(ctx context.Context, out chan<- any) error {
		for {
			select {
			case <-ctx.Done():
				return nil
			case v, ok := <-items:
				if !ok {
					return nil
				}
				select {
				case out <- v:
				case <-ctx.Done():
					return nil
				}
			}
		}
	}

	pipeline := etl.NewPipeline(
		extract,
		[]etl.Stage[any, any]{
			&routerStage[P]{logger: h.logger, param: h.param},
			&tagStage{engine: h.tagEngine, logger: h.logger},
		},
		etl.Buffer[any](h.param.BatchSize, h.param.FlushInterval),
		[]etl.LoadFunc[any]{h.loadBatch},
		h.param.MaxReassemblerSize,
	)
	// context.Canceled 是正常关机路径：外部取消会触发 buffer 冲刷后返回该错误
	//（etl 库语义，见 TestBufferFlushOnCancel），此处按干净停止处理，不向上冒泡为错误。
	if _, err := pipeline.Run(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			h.logger.Ctx(ctx).Infof("flow handler pipeline stopped (context canceled)")
			return nil
		}
		h.logger.Ctx(ctx).Errorf("flow handler pipeline exited with error: %v", err)
		return errors.WithStack(err)
	}
	h.logger.Ctx(ctx).Infof("flow handler pipeline stopped")
	return nil
}

// Shutdown 优雅关闭处理管道。
// 流水线的退出完全由 ctx 取消驱动，无需额外资源释放。
func (h *flowClusterHandler[P]) Shutdown() {}

// routerStage 将 tcp.Router 适配为 etl.Stage[any, any]。
// Pipeline 上游产出宽化的 any，此处断言窄化回 P 后送入 Router 完成 TCP 流重组，
// 重组结果（HttpTransport / NetworkStack）写入 out 交给下游聚合阶段。
type routerStage[P tcp.TCPPacket] struct {
	logger *logging.Logger
	param  *Param
}

// Process 实现 etl.Stage[any, any] 接口。
// 内部以 errgroup 并行运行「窄化转发」与「Router 重组」两个协程：
// 任一返回错误即通过 errgroup 取消对方，避免 Router 提前退出后
// 转发协程阻塞在发送上导致 goroutine 泄漏。
func (s *routerStage[P]) Process(ctx context.Context, in <-chan any, out chan<- any) error {
	defer close(out)
	router := tcp.NewRouter[P](
		s.param.MaxReassemblerSize,
		s.param.MaxOutOrderPacketSize,
		s.param.NoticeTimeout,
	)
	packets := make(chan P, s.param.MaxReassemblerSize)

	wg, ctx := errgroupx.WithContext(ctx)
	wg.GoNamed("downcast", func() error {
		defer close(packets)
		for {
			select {
			case <-ctx.Done():
				return nil
			case v, ok := <-in:
				if !ok {
					return nil
				}
				if p, ok := v.(P); ok {
					select {
					case packets <- p:
					case <-ctx.Done():
						return nil
					}
				}
			}
		}
	})
	wg.GoNamed("router", func() error {
		return router.Run(ctx, s.logger, packets, out)
	})
	if err := wg.Wait(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// loadBatch 将聚合批次按类型分发后并行批量写入 ClickHouse。
// 对应原 BatchWriter 的 flush 语义：http / stack 两类产物并行落库。
func (h *flowClusterHandler[P]) loadBatch(ctx context.Context, batch []any) error {
	var transports []*network.HttpTransport
	var stacks []*network.NetworkStack
	for _, item := range batch {
		switch v := item.(type) {
		case *network.HttpTransport:
			transports = append(transports, v)
		case *network.NetworkStack:
			stacks = append(stacks, v)
		}
	}

	wg, ctx := errgroupx.WithContext(ctx)
	if len(transports) > 0 {
		wg.GoNamed("http", func() error {
			if err := h.db.WithContext(ctx).Create(transports).Error; err != nil {
				h.logger.Ctx(ctx).Errorf("flow load http DB create failed, batchSize=%d, err=%v", len(transports), err)
				return errors.WithStack(err)
			}
			h.logger.Ctx(ctx).Debugf("flow load http flushed, batchSize=%d", len(transports))
			return nil
		})
	}
	if len(stacks) > 0 {
		wg.GoNamed("stack", func() error {
			if err := h.db.WithContext(ctx).Create(stacks).Error; err != nil {
				h.logger.Ctx(ctx).Errorf("flow load stack DB create failed, batchSize=%d, err=%v", len(stacks), err)
				return errors.WithStack(err)
			}
			h.logger.Ctx(ctx).Debugf("flow load stack flushed, batchSize=%d", len(stacks))
			return nil
		})
	}
	if err := wg.Wait(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
