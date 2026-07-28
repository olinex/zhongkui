package tcp

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/gopacket/gopacket/layers"
	"github.com/smallnest/ringbuffer"
	"zhongkui.com/zhongkui/internal/pkg/errors"

	n "zhongkui.com/zhongkui/internal/models/network"
)

// Reassembler 表示一个TCP流重组器，用于从TCP数据包流中重组完整的应用层消息。
// 该结构体使用泛型设计，支持不同类型的TCP数据包实现，如IPv4 TCP和IPv6 TCP。
// Reassembler 负责管理 TCP 连接的两端（客户端和服务端），处理三次握手、四次挥手、
// 乱序包重组及应用层协议识别等功能。
//
// 类型参数:
//   - P: 实现了TCPPacket接口的数据包类型，用于支持不同的TCP数据包实现
type Reassembler[P TCPPacket] struct {
	// ClientAddr 客户端IP地址
	ClientAddr net.IP

	// ClientPort 客户端端口号
	ClientPort layers.TCPPort

	// Client 客户端侧的TCP流管理
	Client *Side[P]

	// ServerAddr 服务端IP地址
	ServerAddr net.IP

	// ServerPort 服务端端口号
	ServerPort layers.TCPPort

	// Server 服务端侧的TCP流管理
	Server *Side[P]

	signalChan     chan any
	lastNoticeNano atomic.Int64
	// cancel 用于取消当前流的上下文，触发环形缓冲区 WithCancel 释放阻塞的读写操作
	cancel context.CancelFunc
}

// newReassembler 根据TCP三次握手的第一个SYN包创建一个新的TCP Reassembler。
func newReassembler[P TCPPacket](
	ctx context.Context,
	first P,
	maxOutOrderPacketSize int,
) (*Reassembler[P], error) {
	// 创建可取消的子上下文，用于 stop() 时解除环形缓冲区的阻塞读取
	flowCtx, cancel := context.WithCancel(ctx)
	clientSide, serverSide, err := newSides(flowCtx, first, maxOutOrderPacketSize)
	if err != nil {
		cancel()
		return nil, errors.WithStack(err)
	}
	recordReassemblerCreated(ctx)
	return &Reassembler[P]{
		ClientAddr: first.SrcAddr(),
		ClientPort: first.SrcPort(),
		ServerAddr: first.DstAddr(),
		ServerPort: first.DstPort(),
		Client:     clientSide,
		Server:     serverSide,
		signalChan: make(chan any, maxOutOrderPacketSize),
		cancel:     cancel,
	}, nil
}

// String 返回TCP Reassembler的字符串表示。
func (r *Reassembler[P]) String() string {
	return fmt.Sprintf(
		`TCPReassembler(%s:%d->%s:%d)`,
		r.ClientAddr.String(),
		r.ClientPort,
		r.ServerAddr.String(),
		r.ServerPort,
	)
}

// isSameFlow 检查给定的数据包是否属于当前TCP流。
func (r *Reassembler[P]) isSameFlow(packet P) (same, c2s bool) {
	if r.ClientPort == packet.SrcPort() &&
		r.ServerPort == packet.DstPort() &&
		r.ClientAddr.Equal(packet.SrcAddr()) &&
		r.ServerAddr.Equal(packet.DstAddr()) {
		same = true
		c2s = true
	} else if r.ServerPort == packet.SrcPort() &&
		r.ClientPort == packet.DstPort() &&
		r.ServerAddr.Equal(packet.SrcAddr()) &&
		r.ClientAddr.Equal(packet.DstAddr()) {
		same = true
		c2s = false
	}
	return
}

// addPacket 将一个TCP数据包添加到流中，处理TCP状态转换和乱序包重组。
func (r *Reassembler[P]) addPacket(ctx context.Context, packet P) error {
	var currentSide, otherSide *Side[P]
	var c2s bool
	if same, dir := r.isSameFlow(packet); !same {
		return errors.Newf(`%s is not same flow`, tcpPacketSeq(packet))
	} else if dir {
		currentSide = r.Client
		otherSide = r.Server
		c2s = true
	} else {
		currentSide = r.Server
		otherSide = r.Client
		c2s = false
	}

	ctx, span := startRectifierSpan(ctx, packet, c2s)
	defer endSpan(span, nil)

	if err := currentSide.addPacket(ctx, packet, otherSide); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (r *Reassembler[P]) isCompleted() bool {
	return r.Client.isCompleted() && r.Server.isCompleted()
}

func (r *Reassembler[P]) release() {
	r.Client.release()
	r.Server.release()
	// 取消流上下文，确保环形缓冲区 WithCancel 的后台 goroutine 退出
	if r.cancel != nil {
		r.cancel()
	}
}

// setLastNotice 原子性地更新最后通知时间。
func (r *Reassembler[P]) setLastNotice(t time.Time) {
	r.lastNoticeNano.Store(t.UnixNano())
}

// lastNotice 原子性地获取最后通知时间。
func (r *Reassembler[P]) lastNotice() time.Time {
	return time.Unix(0, r.lastNoticeNano.Load())
}

// extractApplication 从当前TCP流中提取应用层协议数据，构建完整的应用层对象。
// 该方法遍历所有已注册的应用层协议推测器(supposers)，尝试识别当前TCP流中传输的应用层协议。
// 它首先使用Detect方法检查请求侧和响应侧的数据是否匹配某种应用层协议，如果匹配成功，
// 则使用Extract方法从环形缓冲区中提取协议特定的数据，最后使用Build方法构建完整的应用层对象。
//
// 参数:
//   - ctx: 上下文对象，用于传递取消信号和超时控制
//   - meta: 基础元数据，包含命名空间、捕获器名称、版本号等系统信息
//
// 返回值:
//   - any: 成功时返回构建的应用层对象，失败时返回AppUnknown或nil
//   - error: 处理过程中发生错误时返回错误信息
//
// 处理流程:
//  1. 获取互斥锁，确保并发安全
//  2. 遍历所有已注册的应用层协议推测器
//  3. 对每个推测器:
//     a. 使用Detect方法检查请求侧数据是否匹配协议
//     b. 使用Detect方法检查响应侧数据是否匹配协议
//     c. 如果两侧都匹配，使用Extract方法提取请求侧和响应侧的应用层数据
//     d. 使用Build方法构建完整的应用层对象
//     e. 如果构建成功，返回应用层对象
//  4. 如果没有匹配的协议或构建失败，返回nil
//
// 注意:
//   - 该方法假设TCP流已经包含了足够的数据来识别应用层协议
//   - 不同的应用层协议推测器可能返回不同类型的应用层对象
//   - 如果提取过程中发生错误，返回AppUnknown作为错误指示
//   - 该方法会同时处理请求侧和响应侧的数据，确保双向数据都匹配协议特征
func (r *Reassembler[P]) extractApplication(ctx context.Context, meta *n.BaseMeta, reqPeekCap, rspPeekCap []byte) (any, error) {
	// 拼接 bufio.Reader 已有缓存 + RingBuffer 剩余数据用于协议检测
	// bufio.Reader 中可能已缓存从 RingBuffer 读取但尚未被应用层消费的数据，
	// 仅从 RingBuffer peek 会漏掉这部分导致数据不全。
	// 使用 Peek(n) n≤Buffered() 只取缓存不触发 RingBuffer 读取，
	// PeekAll 从 RingBuffer 做非破坏性查看，最终拼接两者。
	reqPeek, err := peekCombined(r.Client.reader, r.Client.buffer, reqPeekCap)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	rspPeek, err := peekCombined(r.Server.reader, r.Server.buffer, rspPeekCap)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if len(reqPeek) == 0 && len(rspPeek) == 0 {
		return nil, nil
	}
	for protocol, supposer := range detectors {
		innerCtx, detectSpan := startDetectSpan(ctx, protocol)
		var dErr error
		var ok, ok2 bool
		ok, dErr = supposer.Detect(innerCtx, reqPeek, true)
		if dErr != nil {
			endSpan(detectSpan, &dErr)
			return nil, errors.WithStack(dErr)
		} else if !ok {
			endSpan(detectSpan, nil)
			continue
		}
		ok2, dErr = supposer.Detect(innerCtx, rspPeek, false)
		if dErr != nil {
			endSpan(detectSpan, &dErr)
			return nil, errors.WithStack(dErr)
		} else if !ok2 {
			endSpan(detectSpan, nil)
			continue
		}
		endSpan(detectSpan, nil)

		innerCtx, extractSpan := startExtractSpan(ctx, protocol)
		var clientPayload, serverPayload any
		clientPayload, serverPayload, dErr = supposer.Extract(innerCtx, r.Client.reader, r.Server.reader)
		if dErr != nil {
			endSpan(extractSpan, &dErr)
			recordAppExtractionError(innerCtx)
			return nil, errors.WithStack(dErr)
		}
		endSpan(extractSpan, nil)

		innerCtx, buildSpan := startBuildSpan(ctx, protocol)
		var app any
		app, dErr = supposer.Build(
			innerCtx,
			meta,
			r.ClientAddr,
			r.ServerAddr,
			r.ClientPort,
			r.ServerPort,
			r.Client.frameStack,
			r.Server.frameStack,
			clientPayload,
			serverPayload,
		)
		if dErr != nil {
			endSpan(buildSpan, &dErr)
			recordAppExtractionError(innerCtx)
			return nil, errors.WithStack(dErr)
		}
		endSpan(buildSpan, nil)
		recordAppExtracted(innerCtx, string(protocol))
		return app, nil
	}
	return nil, nil
}

// peekCombined 合并 bufio.Reader 缓存与 RingBuffer 数据用于协议检测
// bufio.Reader 可能已从 RingBuffer 读取但未被消费的数据，仅 Peek RingBuffer 会遗漏这部分。
// 参数 cap 为预分配缓冲区，用于 RingBuffer.PeekAll 避免额外分配。
func peekCombined(reader *bufio.Reader, buffer *ringbuffer.RingBuffer, cap []byte) ([]byte, error) {
	var bufPeek []byte
	if n := reader.Buffered(); n > 0 {
		// 仅 Peek 缓冲区内已有数据，不触发底层 RingBuffer 读取
		var err error
		if bufPeek, err = reader.Peek(n); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	rbPeek := make([]byte, buffer.Length())
	if n, err := buffer.Peek(rbPeek); err != nil {
		return nil, errors.WithStack(err)
	} else {
		rbPeek = rbPeek[:n]
	}
	if len(bufPeek) == 0 {
		return rbPeek, nil
	}
	if len(rbPeek) == 0 {
		return bufPeek, nil
	}
	combined := make([]byte, len(bufPeek)+len(rbPeek))
	copy(combined, bufPeek)
	copy(combined[len(bufPeek):], rbPeek)
	return combined, nil
}

// isRetryableError 判断错误是否为可重试的非致命错误。
// 当 Extract 阶段因数据不足或上下文超时失败时，不应终止整个 parse 循环，
// 而应等待后续数据包到达后重试。
func isRetryableError(err error) bool {
	return errors.Is(err, ErrDataIncomplete) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled)
}

// parse 解析TCP流中的应用层数据，将提取的应用层对象发送到目标通道。
// 该方法是一个事件循环，监听信号通道和上下文取消信号，当收到信号时尝试从TCP流中
// 提取应用层数据。它使用extractApplication方法进行实际的协议识别和数据提取，
// 然后将成功的应用层对象发送到目标通道供后续处理使用。
//
// 参数:
//   - ctx: 上下文对象，用于传递取消信号和超时控制
//   - meta: 基础元数据，包含命名空间、捕获器名称、版本号等系统信息
//   - dstChan: 目标通道，用于发送成功提取的应用层对象
//
// 返回值:
//   - error: 致命错误时返回错误信息；上下文取消、流僵死或正常关闭时返回nil
//
// 错误处理策略:
//   - 可重试错误（ErrDataIncomplete、DeadlineExceeded、Canceled）：continue 等待下次信号重试
//   - 可重试错误 + 流僵死（lastNoticeTime 超过 rwBufferTimeout）：return nil 释放资源
//   - 其他致命错误：return error 向上传播
//
// 注意:
//   - 该方法在单独的goroutine中运行，监听信号通道和上下文取消
//   - 信号通道用于通知解析器有新的数据到达，需要重新尝试协议识别
//   - 目标通道应该是缓冲通道，避免阻塞解析器的运行
//   - 每次收到信号都会尝试提取应用层数据，即使数据可能不完整
//   - 应用层协议推测器可能需要在多个数据包到达后才能成功识别协议
func (r *Reassembler[P]) parse(ctx context.Context, meta *n.BaseMeta, dstChan chan<- any) error {
	ctx, span := startParseSpan(ctx)
	defer endSpan(span, nil)
	defer recordReassemblerCompleted(ctx)
	defer r.release()
	reqPeek := make([]byte, r.Client.buffer.Capacity())
	rspPeek := make([]byte, r.Server.buffer.Capacity())
	for {
		select {
		case <-ctx.Done():
			return nil
		case _, ok := <-r.signalChan:
			if !ok {
				return nil
			}
			app, err := r.extractApplication(ctx, meta, reqPeek, rspPeek)
			if err != nil {
				if isRetryableError(err) {
					// 数据不足或上下文超时，不是致命错误，等待后续数据包重试
					continue
				}
				return errors.WithStack(err)
			}
			if app != nil {
				select {
				case <-ctx.Done():
					return nil
				case dstChan <- app:
				}
			}
		}
	}
}

func (r *Reassembler[P]) notice(ctx context.Context) {
	r.setLastNotice(time.Now())
	select {
	case <-ctx.Done():
	case r.signalChan <- nil:
	default:
	}
}

func (r *Reassembler[P]) stop() {
	close(r.signalChan)
	// 取消流上下文以打断僵死在 ringbuffer.Read 上的协程
	// cancel 只影响缓冲区为空、正在等待的 Read，不会中断缓冲区有数据时的正常消费
	if r.cancel != nil {
		r.cancel()
	}
}
