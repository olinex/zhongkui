package tcp

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/gopacket/gopacket/layers"
	"zhongkui.com/zhongkui/internal/network/detectors"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/serializer"

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
	// TraceID 流跟踪ID，用于关联不同数据包的元数据
	TraceID serializer.HexUUID
	// startTime 流的起始时间戳（首个SYN包的捕获时间），
	// 用于Meta()中填充流的起始时间。
	startTime time.Time
	// logger 日志记录器，用于记录TCP Reassembler的运行时信息。
	logger *logging.Logger
	// ClientAddr 客户端IP地址
	ClientAddr net.IP
	// ClientPort 客户端端口号
	ClientPort layers.TCPPort
	// client 客户端侧的TCP流管理
	client *Side[P]
	// ServerAddr 服务端IP地址
	ServerAddr net.IP
	// ServerPort 服务端端口号
	ServerPort layers.TCPPort
	// server 服务端侧的TCP流管理
	server *Side[P]
}

// newReassembler 根据TCP三次握手的第一个SYN包创建一个新的TCP Reassembler。
func newReassembler[P TCPPacket](
	ctx context.Context,
	logger *logging.Logger,
	first P,
	maxOutOrderPacketSize int,
	dstChan chan<- any,
) (*Reassembler[P], error) {
	traceID := serializer.NewHexUUIDv4()
	clientSide, serverSide, err := newSides(ctx, logger, first, maxOutOrderPacketSize, traceID, dstChan)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	recordReassemblerCreated(ctx)
	return &Reassembler[P]{
		TraceID:    traceID,
		startTime:  first.CaptureTimestamp(),
		logger:     logger,
		ClientAddr: first.SrcAddr(),
		ClientPort: first.SrcPort(),
		ServerAddr: first.DstAddr(),
		ServerPort: first.DstPort(),
		client:     clientSide,
		server:     serverSide,
	}, nil
}

func (r *Reassembler[P]) Meta() *detectors.StreamMeta {
	return &detectors.StreamMeta{
		TraceID:    r.TraceID,
		ClientAddr: r.ClientAddr,
		ClientPort: r.ClientPort,
		ClientISN:  r.client.isn,
		ServerAddr: r.ServerAddr,
		ServerPort: r.ServerPort,
		ServerISN:  r.server.isn,
		Timestamp:  r.startTime,
	}
}

func (r *Reassembler[P]) Close() {
	r.client.bufferClose()
	r.server.bufferClose()
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
		currentSide = r.client
		otherSide = r.server
		c2s = true
	} else {
		currentSide = r.server
		otherSide = r.client
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
	return r.client.isCompleted() && r.server.isCompleted()
}

func (r *Reassembler[P]) buildTailer(ctx context.Context) (tailer *detectors.AppTailer, err error) {
	ClientConsumed, clientStartPayloadOffset, clientEndPayloadOffset, clientStartTimestamp, clientEndTimestamp := r.client.UpdateComsumed()
	serverConsumed, serverStartPayloadOffset, serverEndPayloadOffset, serverStartTimestamp, serverEndTimestamp := r.server.UpdateComsumed()
	return &detectors.AppTailer{
		ClientConsumed:           ClientConsumed,
		ServerConsumed:           serverConsumed,
		ClientStartPayloadOffset: clientStartPayloadOffset,
		ClientEndPayloadOffset:   clientEndPayloadOffset,
		ClientStartTimestamp:     clientStartTimestamp,
		ClientEndTimestamp:       clientEndTimestamp,
		ServerStartPayloadOffset: serverStartPayloadOffset,
		ServerEndPayloadOffset:   serverEndPayloadOffset,
		ServerStartTimestamp:     serverStartTimestamp,
		ServerEndTimestamp:       serverEndTimestamp,
	}, nil
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
func (r *Reassembler[P]) extractApplication(ctx context.Context, meta *n.BaseMeta, streamIndex uint32) (any, error) {
	// 无阻塞检查双方是否有数据可读取：bufio.Reader 内部缓存 + RingBuffer 剩余数据。
	// 仅检查是否有数据，不做实际读取，避免阻塞等待不够的数据量。
	reqPeek, err := r.client.peekAll()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	rspPeek, err := r.server.peekAll()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	for protocol, supposer := range detectors.Detectors {
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
		clientPayload, serverPayload, dErr = supposer.Extract(
			innerCtx,
			r.client.brw.Reader,
			r.server.brw.Reader,
		)
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
			r.Meta(),
			streamIndex,
			clientPayload,
			serverPayload,
			r.buildTailer,
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
//   - NetworkStack帧记录已在Side.appendPacket中即时写入dstChan，无需在此刷新
func (r *Reassembler[P]) parse(ctx context.Context, meta *n.BaseMeta, signalChan <-chan any, dstChan chan<- any) error {
	ctx, span := startParseSpan(ctx)
	defer endSpan(span, nil)
	defer recordReassemblerCompleted(ctx)
	streamIndex := uint32(0)
	for {
		select {
		case <-ctx.Done():
			return nil
		case _, ok := <-signalChan:
			if !ok {
				// 流结束（signalChan被关闭），正常退出
				return nil
			}
			app, err := r.extractApplication(ctx, meta, streamIndex)
			if err != nil {
				if errors.Is(err, detectors.ErrDataIncomplete) {
					continue
				}
				return errors.WithStack(err)
			}
			if app != nil {
				select {
				case <-ctx.Done():
					return nil
				case dstChan <- app:
					streamIndex++
				}
			}
		}
	}
}
