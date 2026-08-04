// Package flow 实现基于 TCP 流的分布式抓包处理。
// 包含从 ClickHouse 轮询数据、TCP 流重组、批量输出到 ClickHouse 的完整流水线。
package flow

import (
	"context"
	"sync"
	"time"

	"zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errgroupx"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// BatchWriter 批量写入 HttpTransport 到 ClickHouse。
//
// 多 Shard 的输出通过 input channel 汇聚，由内部 flush loop 统一批量写入，
// 避免多个 Shard 各自写 DB 导致小事务过多。
type BatchWriter struct {
	mu          sync.Mutex
	db          *database.DB
	logger      *logging.Logger
	param       *Param
	httpInput   chan *network.HttpTransport
	httpBuffer  []*network.HttpTransport
	stackInput  chan *network.NetworkStack
	stackBuffer []*network.NetworkStack
}

// NewBatchWriter 创建批量写入器。
//
// 参数:
//   - db: ClickHouse GORM 连接
//   - logger: 结构化日志实例
func NewBatchWriter(db *database.DB, logger *logging.Logger, param *Param) *BatchWriter {
	return &BatchWriter{
		db:          db,
		logger:      logger,
		param:       param,
		httpInput:   make(chan *network.HttpTransport, param.BatchSize*3),
		httpBuffer:  make([]*network.HttpTransport, 0, param.BatchSize),
		stackInput:  make(chan *network.NetworkStack, param.BatchSize*3),
		stackBuffer: make([]*network.NetworkStack, 0, param.BatchSize),
	}
}

// WriteHttpTransport 提交一条 HttpTransport 到批量写入队列。
// 非阻塞：通道满时丢弃（极端情况，避免阻塞 Router 输出流）。
func (w *BatchWriter) WriteHttpTransport(ctx context.Context, t *network.HttpTransport) error {
	timer := time.NewTimer(w.param.FlushInterval * 2)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil
	case w.httpInput <- t:
		return nil
	case <-timer.C:
		return errors.New("write http transport timeout")
	}
}

func (w *BatchWriter) WriteStack(ctx context.Context, t *network.NetworkStack) error {
	timer := time.NewTimer(w.param.FlushInterval * 2)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil
	case w.stackInput <- t:
		return nil
	case <-timer.C:
		return errors.New("write stack timeout")
	}
}

// Run 启动批量写入主循环，阻塞直到 ctx 取消。
//
// 同时监听 input 通道和定时器：
//   - input 有新数据时追加到 buffer，buffer 满则 flush
//   - 定时器触发时 flush（低流量兜底）
//   - ctx 取消时执行最终 flush
func (w *BatchWriter) Start(ctx context.Context) error {
	w.logger.Ctx(ctx).Infof("batch writer started, batchSize=%d, flushInterval=%s",
		w.param.BatchSize, w.param.FlushInterval)

	ticker := time.NewTicker(w.param.FlushInterval)
	defer ticker.Stop()

	var totalFlushed int64

LOOP:
	for {
		select {
		case <-ctx.Done():
			if err := w.flush(ctx); err != nil {
				w.logger.Ctx(ctx).Errorf("batch writer final flush failed: %v", err)
				return errors.WithStack(err)
			}
			w.logger.Ctx(ctx).Infof("batch writer shutting down, totalFlushed=%d", totalFlushed)
			return nil
		case model := <-w.httpInput:
			w.mu.Lock()
			w.httpBuffer = append(w.httpBuffer, model)
			shouldFlush := len(w.httpBuffer) >= w.param.BatchSize
			w.mu.Unlock()
			if !shouldFlush {
				continue LOOP
			}
		case model := <-w.stackInput:
			w.mu.Lock()
			w.stackBuffer = append(w.stackBuffer, model)
			shouldFlush := len(w.stackBuffer) >= w.param.BatchSize
			w.mu.Unlock()
			if !shouldFlush {
				continue LOOP
			}
		case <-ticker.C:
		}
		if err := w.flush(ctx); err != nil {
			w.logger.Ctx(ctx).Errorf("batch writer flush failed: %v", err)
			return errors.WithStack(err)
		}
		totalFlushed++
	}
}

func (w *BatchWriter) flushStack(ctx context.Context) error {
	w.mu.Lock()
	if len(w.stackBuffer) == 0 {
		w.mu.Unlock()
		return nil
	}
	batch := w.stackBuffer
	batchSize := len(batch)
	w.stackBuffer = make([]*network.NetworkStack, 0, w.param.BatchSize)
	w.mu.Unlock()

	if err := w.db.WithContext(ctx).Create(batch).Error; err != nil {
		w.logger.Ctx(ctx).Errorf("batch writer stack DB create failed, batchSize=%d, err=%v", batchSize, err)
		return errors.WithStack(err)
	}
	w.logger.Ctx(ctx).Debugf("batch writer stack flushed, batchSize=%d", batchSize)
	return nil
}

// flush 将当前缓冲区数据批量写入 ClickHouse。
func (w *BatchWriter) flushHttp(ctx context.Context) error {
	w.mu.Lock()
	if len(w.httpBuffer) == 0 {
		w.mu.Unlock()
		return nil
	}
	batch := w.httpBuffer
	batchSize := len(batch)
	w.httpBuffer = make([]*network.HttpTransport, 0, w.param.BatchSize)
	w.mu.Unlock()

	if err := w.db.WithContext(ctx).Create(batch).Error; err != nil {
		w.logger.Ctx(ctx).Errorf("batch writer http DB create failed, batchSize=%d, err=%v", batchSize, err)
		return errors.WithStack(err)
	}
	w.logger.Ctx(ctx).Debugf("batch writer http flushed, batchSize=%d", batchSize)
	return nil
}

func (w *BatchWriter) flush(ctx context.Context) error {
	wg, ctx := errgroupx.WithContext(ctx)
	wg.GoNamed("http", func() error {
		return w.flushHttp(ctx)
	})
	wg.GoNamed("stack", func() error {
		return w.flushStack(ctx)
	})
	if err := wg.Wait(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (w *BatchWriter) Shutdown() {
	close(w.httpInput)
}
