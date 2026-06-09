package tcp

import (
	"context"
	"time"

	n "zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/test"
)

// Reassembler 表示一个TCP流重组器，用于从TCP数据包流中重组完整的应用层消息。
// 该结构体使用泛型设计，支持不同类型的TCP数据包实现，如IPv4 TCP和IPv6 TCP。
// Reassembler继承自Rectifier，增加了应用层数据缓冲、协议识别和消息重组功能。
//
// 类型参数:
//   - P: 实现了TCPPacket接口的数据包类型，用于支持不同的TCP数据包实现
//
// 主要功能:
//   - 应用层数据缓冲: 使用环形缓冲区存储TCP数据流
//   - 协议自动识别: 自动识别HTTP、TLS等应用层协议
//   - 消息边界检测: 检测应用层消息的完整边界
//   - 双向流重组: 同时处理客户端到服务端和服务端到客户端的双向数据流
//   - 应用层消息构建: 将重组的数据组装成应用层协议对象
type Reassembler[P TCPPacket] struct {
	Rectifier[P]
	signalChan     chan any
	lastNoticeTime time.Time
}

// newReassembler 根据TCP三次握手的第一个SYN包创建一个新的TCP Reassembler。
// 该函数是TCP重组器的工厂函数，用于初始化一个新的TCP连接重组实例。
// 它从TCP三次握手的第一个SYN包中提取连接信息，并初始化客户端和服务端的数据缓冲区。
//
// 参数:
//   - first: TCP三次握手的第一个SYN包，必须设置了SYN标志位
//   - maxOutOrderPacketSize: 最大乱序包缓冲区大小，限制每个方向可以缓冲的乱序包数量
//   - maxAppBufferSize: 最大应用层数据缓冲区大小，限制每个方向的应用层数据缓冲容量
//
// 返回值:
//   - *Reassembler[P]: 成功时返回初始化的Reassembler实例
//   - error: 如果第一个包不是SYN包，返回错误
//
// 初始化过程:
//  1. 验证数据包是否为SYN包
//  2. 创建Rectifier实例管理TCP连接状态
//  3. 初始化请求侧和响应侧的帧堆栈（空切片）
//  4. 创建请求侧和响应侧的环形数据缓冲区
//  5. 设置请求侧和响应侧的Payload为nil（等待数据填充）
//
// 注意:
//   - 客户端ISN（初始序列号）从第一个SYN包的序列号确定
//   - 服务端ISN初始为0，在收到SYN包并发送SYN-ACK响应后确定
//   - 该函数假设SYN包来自客户端，方向为客户端到服务端
//   - 应用层数据缓冲区大小应根据预期处理的数据量合理设置，避免内存溢出
func newReassembler[P TCPPacket](
	ctx context.Context,
	first P,
	maxOutOrderPacketSize int,
	rwBufferTimeout time.Duration,
) (*Reassembler[P], error) {
	rectifier, err := newRectifierWithHandshakeFirstSyn(ctx, first, maxOutOrderPacketSize, rwBufferTimeout)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &Reassembler[P]{
		Rectifier:  *rectifier,
		signalChan: make(chan any, maxOutOrderPacketSize),
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
func (r *Reassembler[P]) extractApplication(ctx context.Context, meta *n.BaseMeta, reqPeek, rspPeek []byte) (any, error) {
	if len(reqPeek) == 0 || len(rspPeek) == 0 {
		return nil, nil
	}
	if reqPeekTmp, err := r.Client.buffer.PeekAll(reqPeek); err != nil {
		return nil, errors.WithStack(err)
	} else {
		reqPeek = reqPeekTmp
	}
	if rspPeekTmp, err := r.Server.buffer.PeekAll(rspPeek); err != nil {
		return nil, errors.WithStack(err)
	} else {
		rspPeek = rspPeekTmp
	}
	for _, supposer := range detectors {
		test.Tracer.Begin("detector.Detect")
		if ok, err := supposer.Detect(ctx, reqPeek, true); err != nil {
			test.Tracer.End()
			return nil, errors.WithStack(err)
		} else if !ok {
			test.Tracer.End()
			continue
		}
		if ok, err := supposer.Detect(ctx, rspPeek, false); err != nil {
			test.Tracer.End()
			return nil, errors.WithStack(err)
		} else if !ok {
			test.Tracer.End()
			continue
		}
		test.Tracer.End()

		test.Tracer.Begin("detector.Extract")
		var clientPayload, serverPayload any
		if clientTmp, serverTmp, err := supposer.Extract(ctx, r.Client.buffer, r.Server.buffer); err != nil {
			test.Tracer.End()
			return nil, errors.WithStack(err)
		} else {
			clientPayload = clientTmp
			serverPayload = serverTmp
		}
		test.Tracer.End()

		test.Tracer.Begin("detector.Build")
		var app any
		if tmp, err := supposer.Build(
			ctx,
			meta,
			r.ClientAddr,
			r.ServerAddr,
			r.ClientPort,
			r.ServerPort,
			r.Client.frameStack,
			r.Server.frameStack,
			clientPayload,
			serverPayload,
		); err != nil {
			test.Tracer.End()
			return nil, errors.WithStack(err)
		} else {
			app = tmp
		}
		test.Tracer.End()
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
//   - signalChan: 信号通道，当有新的TCP数据包到达时发送信号触发解析
//   - dstChan: 目标通道，用于发送成功提取的应用层对象
//
// 返回值:
//   - error: 处理过程中发生错误时返回错误信息，上下文取消时返回nil
//
// 处理流程:
//  1. 进入无限循环，监听信号和上下文取消
//  2. 当收到信号通道的信号时，触发应用层数据提取
//  3. 调用extractApplication方法提取应用层对象
//  4. 如果提取成功，将应用层对象发送到目标通道
//  5. 如果提取过程中发生错误，返回错误
//  6. 当上下文被取消时，优雅地退出循环
//
// 注意:
//   - 该方法在单独的goroutine中运行，监听信号通道和上下文取消
//   - 信号通道用于通知解析器有新的数据到达，需要重新尝试协议识别
//   - 目标通道应该是缓冲通道，避免阻塞解析器的运行
//   - 每次收到信号都会尝试提取应用层数据，即使数据可能不完整
//   - 应用层协议推测器可能需要在多个数据包到达后才能成功识别协议
func (r *Reassembler[P]) parse(ctx context.Context, meta *n.BaseMeta, dstChan chan<- any) error {
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
	select {
	case <-ctx.Done():
	case r.signalChan <- nil:
		r.lastNoticeTime = time.Now()
	default:
	}
}

func (r *Reassembler[P]) stop() {
	close(r.signalChan)
}
