package flow

import (
	"context"
	"sync"
	"time"

	"zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// BatchWriter 批量写入 HttpTransport 到 ClickHouse。
//
// 多 Shard 的输出通过 input channel 汇聚，由内部 flush loop 统一批量写入，
// 避免多个 Shard 各自写 DB 导致小事务过多。
type BatchWriter struct {
	mu     sync.Mutex
	db     *database.DB
	param  *Param
	input  chan *network.HttpTransport
	buffer []*network.HttpTransport
}

// NewBatchWriter 创建批量写入器。
//
// 参数:
//   - db: ClickHouse GORM 连接
func NewBatchWriter(db *database.DB, param *Param) *BatchWriter {
	return &BatchWriter{
		db:     db,
		param:  param,
		input:  make(chan *network.HttpTransport, param.BatchSize*3),
		buffer: make([]*network.HttpTransport, 0, param.BatchSize),
	}
}

// Write 提交一条 HttpTransport 到批量写入队列。
// 非阻塞：通道满时丢弃（极端情况，避免阻塞 Router 输出流）。
func (w *BatchWriter) Write(ctx context.Context, t *network.HttpTransport) error {
	timer := time.NewTimer(w.param.FlushInterval * 2)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil
	case w.input <- t:
		return nil
	case <-timer.C:
		return errors.New("write http transport timeout")
	}
}

// Input 返回 input 通道，供外部直接发送数据。
func (w *BatchWriter) Input() chan<- *network.HttpTransport {
	return w.input
}

// Run 启动批量写入主循环，阻塞直到 ctx 取消。
//
// 同时监听 input 通道和定时器：
//   - input 有新数据时追加到 buffer，buffer 满则 flush
//   - 定时器触发时 flush（低流量兜底）
//   - ctx 取消时执行最终 flush
func (w *BatchWriter) Start(ctx context.Context) error {
	ticker := time.NewTicker(w.param.FlushInterval)
	defer ticker.Stop()

LOOP:
	for {
		select {
		case <-ctx.Done():
			if err := w.flush(ctx); err != nil {
				return errors.WithStack(err)
			}
			return nil
		case t := <-w.input:
			w.mu.Lock()
			w.buffer = append(w.buffer, t)
			shouldFlush := len(w.buffer) >= w.param.BatchSize
			w.mu.Unlock()
			if !shouldFlush {
				continue LOOP
			}
		case <-ticker.C:
		}
		if err := w.flush(ctx); err != nil {
			return errors.WithStack(err)
		}
	}
}

// flush 将当前缓冲区数据批量写入 ClickHouse。
func (w *BatchWriter) flush(ctx context.Context) error {
	w.mu.Lock()
	if len(w.buffer) == 0 {
		w.mu.Unlock()
		return nil
	}
	batch := w.buffer
	w.buffer = make([]*network.HttpTransport, 0, w.param.BatchSize)
	w.mu.Unlock()

	if err := w.db.WithContext(ctx).Create(batch).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (w *BatchWriter) Shutdown() {
	close(w.input)
}
