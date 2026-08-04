package tcp

import (
	"context"
	"sync/atomic"
	"time"

	n "zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// ReassemblerNode 表示重组器节点，用于在双向链表中管理TCP流的重组器实例。
// 每个节点对应一个TCP连接流，包含重组器实例和LRU链表指针。
// 节点使用双向链表结构实现LRU（最近最少使用）缓存策略，提高流查找效率。
//
// 类型参数:
//   - P: 实现了TCPPacket接口的数据包类型
type ReassemblerNode[P TCPPacket] struct {
	ID          string // TCP流的唯一标识符
	ctx         context.Context
	cancel      context.CancelFunc
	signalChan  chan any
	reassembler *Reassembler[P] // TCP重组器实例
	prev        *ReassemblerNode[P]
	next        *ReassemblerNode[P]
	noticeTime  time.Time
	status      atomic.Int32
}

func newReassemblerNode[P TCPPacket](
	ctx context.Context,
	logger *logging.Logger,
	streamID string,
	first P,
	maxOutOrderPacketSize int,
	dstChan chan<- any,
) (*ReassemblerNode[P], error) {
	ctx, cancel := context.WithCancel(ctx)
	rsm, err := newReassembler(ctx, logger, first, maxOutOrderPacketSize, dstChan)
	if err != nil {
		cancel()
		return nil, errors.WithStack(err)
	}
	return &ReassemblerNode[P]{
		ID:          streamID,
		ctx:         ctx,
		cancel:      cancel,
		signalChan:  make(chan any, maxOutOrderPacketSize),
		reassembler: rsm,
		prev:        nil,
		next:        nil,
		noticeTime:  time.Now(), // 初始化为当前时间，避免刚创建的节点被clearable误判为过期
		status:      atomic.Int32{},
	}, nil
}

func (n *ReassemblerNode[P]) draft() bool {
	return n.status.Load() == 0
}

func (n *ReassemblerNode[P]) running() bool {
	return n.status.Load() == 1
}

func (n *ReassemblerNode[P]) asRunning() bool {
	return n.status.CompareAndSwap(0, 1)
}

func (n *ReassemblerNode[P]) completed() bool {
	return n.status.Load() == 2
}

func (n *ReassemblerNode[P]) asCompleted() {
	n.status.Store(2)
}

func (n *ReassemblerNode[P]) clearable(noticeTimeout time.Duration) bool {
	if n.draft() {
		return false
	}
	if n.noticeTime.Add(noticeTimeout).Before(time.Now()) {
		return true
	}
	return n.reassembler.isCompleted()
}

func (n *ReassemblerNode[P]) addPacket(packet P) error {
	if err := n.reassembler.addPacket(n.ctx, packet); err != nil {
		return errors.WithStack(err)
	}
	select {
	case <-n.ctx.Done():
	case n.signalChan <- nil:
		n.noticeTime = time.Now()
	default:
	}
	return nil
}

// run 启动重组器节点的重组器实例。
// 该方法初始化重组器控制器，并在新的goroutine中启动重组器的运行。
// 如果节点已经启动，则返回错误。
//
// 参数:
//   - wg: errgroup.Group实例，用于管理goroutine的错误和等待
//   - dstChan: 目标通道，用于输出重组后的应用层数据
//   - packetChannelSize: 数据包通道的缓冲区大小
//
// 返回值:
//   - error: 如果节点已经启动，返回错误；否则返回nil
//
// 启动流程:
//  1. 检查控制器是否已初始化，避免重复启动
//  2. 创建新的重组器控制器
//  3. 在新的goroutine中启动重组器
//  4. 设置defer确保节点关闭时资源正确释放
func (n *ReassemblerNode[P]) start(
	logger *logging.Logger,
	meta *n.BaseMeta,
	dstChan chan<- any,
) {
	defer func() {
		n.cancel()
		if err := recover(); err != nil {
			logger.Ctx(n.ctx).Errorf(`reassembler node %s panic: %v`, n.ID, err)
		}
	}()
	if !n.asRunning() {
		panic("reassembler node is running or completed")
	}
	err := n.reassembler.parse(
		n.ctx,
		meta,
		n.signalChan,
		dstChan,
	)
	if err != nil {
		logger.Ctx(n.ctx).Errorf(`reassembler node %s run error: %v`, n.ID, err)
	}
}

func (n *ReassemblerNode[P]) stop(kill bool) {
	n.asCompleted()
	if kill {
		// 先 cancel context，让 parse goroutine 的 select 走 ctx.Done() 安全退出，
		// 避免 parse 排空 signalChan 缓冲信号后向已关闭的 dstChan 发送数据。
		n.cancel()
	}
	// close(signalChan) 让 parse 排空缓冲信号后通过 ok=false 正常退出。
	// 非 kill 模式下不 cancel，context 由父级 errgroup 取消时一并释放。
	close(n.signalChan)
	n.reassembler.Close()
}
