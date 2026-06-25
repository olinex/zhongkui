package tcp

import (
	"context"
	"encoding/json"
	"time"

	"zhongkui.com/zhongkui/internal/pkg/errors"

	n "zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/test"
)

// TCPState 表示TCP连接的状态，遵循标准TCP状态机。
// 该枚举类型定义了TCP连接生命周期中的关键状态，用于跟踪和管理TCP连接的建立、数据传输和终止过程。
// 状态转换遵循RFC 793定义的TCP状态机规范，确保TCP协议的可靠性和正确性。
type TCPState int

const (
	// StateClosed 表示连接已关闭
	// 这是TCP连接的初始状态和最终状态，表示连接不存在或已完全终止。
	// 在此状态下，不接受任何数据包，所有资源已释放。
	StateClosed TCPState = iota

	// StateListen 表示服务端监听状态
	// 服务端在此状态下等待客户端的连接请求（SYN包）。
	// 这是服务端被动打开连接的初始状态，准备接受来自客户端的连接建立请求。
	StateListen

	// StateSynSent 表示客户端已发送SYN包
	// 客户端在此状态下已发送SYN包，正在等待服务端的SYN-ACK响应。
	// 这是TCP三次握手的第一步，表示客户端主动发起连接建立请求。
	StateSynSent

	// StateSynReceived 表示服务端已接收SYN包
	// 服务端在此状态下已收到客户端的SYN包并发送了SYN-ACK响应，正在等待客户端的ACK确认。
	// 这是TCP三次握手的第二步，表示服务端已确认连接请求并等待最终确认。
	StateSynReceived

	// StateEstablished 表示连接已建立
	// 在此状态下，TCP连接已成功建立，双方可以开始正常的数据传输。
	// 这是TCP连接的主要工作状态，支持双向可靠数据传输。
	StateEstablished

	// StateCloseWait 表示等待关闭状态
	// 当一端收到对方的FIN包并发送ACK确认后进入此状态，表示本地应用已收到连接关闭请求，
	// 正在等待本地应用发出FIN包已完成连接终止过程。
	StateCloseWait
)

// Side 表示TCP流的一端（客户端或服务端），包含该端的连接状态、缓冲区和数据。
// 该结构体使用泛型设计，支持不同类型的TCP数据包实现，如IPv4 TCP和IPv6 TCP。
// Side负责管理TCP连接的状态转换、序列号跟踪、乱序包缓冲和数据包重组等功能，
// 是TCP流重组和状态管理的核心组件。
//
// 类型参数:
//   - P: 实现了TCPPacket接口的数据包类型，用于支持不同的TCP数据包实现
type Side[P TCPPacket] struct {
	// buffer 单端应用层数据环形缓冲区，存储该端方向的TCP数据流。
	// 使用环形缓冲区设计，支持高效的数据读写和滑动窗口管理。
	buffer *serializer.RingBuffer

	// frameStack 单端帧堆栈，记录构成当前请求数据的所有TCP帧信息。
	// 每个FrameInfo包含时间戳和网络层堆栈信息，用于追踪数据的来源。
	// 在数据溯源、调试和分析场景中，frameStack提供了原始数据包的完整上下文信息。
	frameStack n.FrameStack

	// cli2Srv 表示数据包方向是否为客户端到服务端
	// true表示客户端到服务端方向，false表示服务端到客户端方向
	// 该字段用于区分TCP连接的两端，确保正确的状态转换和数据处理
	cli2Srv bool

	// state 表示当前TCP连接状态
	// 遵循TCP状态机规范，跟踪连接的生命周期状态变化
	// 状态转换由TCP协议事件（如SYN、ACK、FIN、RST包）触发
	state TCPState

	// isn 表示初始序列号（Initial Sequence Number）
	// 在TCP连接建立时确定的随机起始序列号，用于后续序列号计算
	// 客户端isn由第一个SYN包确定，服务端isn由SYN-ACK包确定
	isn uint32

	// expectedSeq 表示期望接收的下一个数据包的序列号
	// 用于检测数据包顺序、处理乱序包和确认数据包完整性
	// 根据接收到的数据包载荷长度动态更新，确保序列号连续性
	expectedSeq uint32

	// outOrderPackets 存储乱序到达的数据包
	// 键为数据包序列号，值为对应的TCP数据包
	// 当数据包乱序到达时临时存储，待期望序列号的数据包到达后进行重组
	outOrderPackets map[uint32]P

	// MaxOutOrderPacketSize 最大乱序包缓冲区大小
	// 限制每个方向（客户端或服务端）可以缓冲的乱序数据包数量，防止内存溢出
	// 当缓冲区达到此限制时，新的乱序包将被拒绝，避免资源耗尽
	maxOutOrderPacketSize int
}

// seqLess 比较两个TCP序列号，正确处理序列号回绕情况。
// TCP序列号是32位无符号整数，当达到最大值（2^32-1）后会回绕到0。
// 该函数实现TCP序列号的标准比较算法，考虑回绕情况。
//
// 参数:
//   - a: 第一个序列号
//   - b: 第二个序列号
//
// 返回值:
//   - bool: 如果a在b之前（考虑回绕），返回true
//
// 算法说明:
//   - 如果|a-b| < 2^31，则使用正常的整数比较
//   - 否则，a < b 当且仅当 a > b（因为回绕）
//
// 示例:
//   - seqLess(1, 10) = true
//   - seqLess(10, 1) = false
//   - seqLess(4294967290, 5) = false（因为4294967290回绕后大于5）
//   - seqLess(5, 4294967290) = true（因为5回绕后小于4294967290）
func seqLess(a, b uint32) bool {
	// TCP序列号比较的标准算法：考虑32位回转
	// 如果两个序列号的差值的绝对值小于2^31，则正常比较
	// 否则，反转比较结果
	const maxSeq = 1 << 31 // 2^31
	diff := int32(a - b)
	if diff < 0 {
		diff = -diff
	}
	if uint32(diff) < maxSeq {
		return a < b
	}
	// 差值大于等于2^31，说明发生了回绕
	return a > b
}

// isSynSent 检查当前是否为客户端且处于SynSent状态。
// 该方法用于判断当前Side实例是否代表客户端且已发送SYN包，正在等待SYN-ACK响应。
//
// 返回值:
//   - bool: true表示是客户端且处于SynSent状态，false表示不是
//
// 使用场景:
//   - TCP三次握手过程中验证客户端状态
//   - 确定是否可以处理SYN-ACK响应包
func (s *Side[P]) isSynSent() bool {
	return s.cli2Srv && s.state == StateSynSent
}

// isListen 检查当前是否为服务端且处于Listen状态。
// 该方法用于判断当前Side实例是否代表服务端且正在监听连接请求。
//
// 返回值:
//   - bool: true表示是服务端且处于Listen状态，false表示不是
//
// 使用场景:
//   - TCP连接建立前验证服务端状态
//   - 确定是否可以接受客户端的SYN包
func (s *Side[P]) isListen() bool {
	return !s.cli2Srv && s.state == StateListen
}

// isSynReceived 检查当前是否为服务端且处于SynReceived状态。
// 该方法用于判断当前Side实例是否代表服务端且已收到SYN包并发送了SYN-ACK响应。
//
// 返回值:
//   - bool: true表示是服务端且处于SynReceived状态，false表示不是
//
// 使用场景:
//   - TCP三次握手过程中验证服务端状态
//   - 确定是否可以处理客户端的ACK确认包
func (s *Side[P]) isSynReceived() bool {
	return !s.cli2Srv && s.state == StateSynReceived
}

// isEstablished 检查当前是否处于Established状态。
// 该方法用于判断TCP连接是否已成功建立，可以开始正常的数据传输。
//
// 返回值:
//   - bool: true表示连接已建立，false表示未建立
//
// 使用场景:
//   - 验证连接是否可以进行数据传输
//   - 确定是否可以处理数据包和FIN包
func (s *Side[P]) isEstablished() bool {
	return s.state == StateEstablished
}

// isCloseWait 检查当前是否处于CloseWait状态。
// 该方法用于判断是否已收到对方的FIN包并发送了ACK确认，正在等待本地应用发出FIN包。
//
// 返回值:
//   - bool: true表示处于CloseWait状态，false表示不是
//
// 使用场景:
//   - TCP四次挥手过程中验证连接关闭状态
//   - 确定是否可以处理对方的ACK确认
func (s *Side[P]) isCloseWait() bool {
	return s.state == StateCloseWait
}

// isClosed 检查当前是否处于Closed状态。
// 该方法用于判断TCP连接是否已完全关闭，所有资源已释放。
//
// 返回值:
//   - bool: true表示连接已关闭，false表示未关闭
//
// 使用场景:
//   - 验证连接是否已终止
//   - 确定是否可以清理连接相关资源
func (s *Side[P]) isClosed() bool {
	return s.state == StateClosed
}

func (s *Side[P]) isCompleted() bool {
	return s.isClosed() || s.isCloseWait()
}

// appendPacket 将数据包添加到该端，更新期望序列号并记录帧信息。
// 该方法处理按序到达的数据包，更新期望序列号并将数据包发送到目标管道。
// 期望序列号根据数据包载荷长度递增，确保序列号的连续性。
//
// 参数:
//   - ctx: 上下文，用于控制操作取消和超时
//   - packet: 要添加的TCP数据包，必须按序到达（seq == ExpectedSeq）
//   - dst: 目标管道通道，用于发送处理后的数据包进行后续处理
//
// 返回值:
//   - error: 如果上下文被取消或发送数据包时发生错误，返回错误
//
// 注意:
//   - 该方法不验证数据包顺序，调用前应确保seq == ExpectedSeq
//   - 数据包载荷长度可能为0（如纯ACK包），此时期望序列号不变
//   - 该方法不处理乱序包，乱序包应由addPacket方法处理
//   - 如果上下文被取消，方法将返回错误而不是阻塞
func (s *Side[P]) appendPacket(packet P) error {
	payload := packet.GetPayload()
	s.expectedSeq += uint32(len(payload))
	s.frameStack = append(s.frameStack, packet.FrameInfo())
	// 将新进入的TCP包的有效载荷写入到应用层数据缓冲区中
	if n, err := s.buffer.Write(payload); err != nil {
		return errors.WithStack(err)
	} else if n != len(payload) {
		return errors.New("payload not fully written to buffer")
	}
	return nil
}

// newClientSide 根据第一个SYN包创建客户端流的一端。
// 该函数用于初始化TCP连接中客户端方向的Side实例，设置初始状态和序列号。
// 客户端ISN由第一个SYN包的序列号确定，期望序列号为ISN+1，表示已发送SYN包。
//
// 参数:
//   - first: 第一个SYN包，包含客户端的初始序列号
//
// 返回值:
//   - *Side[P]: 初始化完成的客户端Side实例，状态为StateSynSent
//
// 注意:
//   - 该函数假设传入的数据包是有效的SYN包
//   - 客户端ISN应具有随机性，避免序列号预测攻击
func newSides[P TCPPacket](ctx context.Context, first P, maxOutOrderPacketSize int, rwBufferTimeout time.Duration) (clientSide, serverSide *Side[P], err error) {
	if !first.IsSyn() {
		panic("first packet must be SYN")
	}
	seq := first.SeqNum()
	clientSide = &Side[P]{
		buffer: serializer.
			NewRingBuffer(serializer.MiddleBufferSize).
			WithCancel(ctx).
			WithTimeout(rwBufferTimeout).
			SetBlocking(true),
		frameStack:            make(n.FrameStack, 0, 32),
		cli2Srv:               true,
		state:                 StateSynSent,
		isn:                   seq,
		expectedSeq:           seq + 1,
		maxOutOrderPacketSize: maxOutOrderPacketSize,
	}
	if err := clientSide.appendPacket(first); err != nil {
		return nil, nil, errors.WithStack(err)
	}
	serverSide = &Side[P]{
		buffer: serializer.
			NewRingBuffer(serializer.MiddleBufferSize).
			WithCancel(ctx).
			WithTimeout(rwBufferTimeout).
			SetBlocking(true),
		frameStack:            make(n.FrameStack, 0, 32),
		cli2Srv:               false,
		state:                 StateListen,
		isn:                   0,
		expectedSeq:           0,
		maxOutOrderPacketSize: maxOutOrderPacketSize,
	}
	return
}

// handshakeWithSecondSynAck 处理TCP三次握手的第二个SYN-ACK包。
// 该方法处理服务端对客户端SYN包的响应，验证SYN-ACK包的有效性并更新服务端状态。
// 服务端进入SynReceived状态，设置ISN为SYN-ACK包的序列号，期望序列号为ISN+1。
//
// 参数:
//   - ctx: 上下文，用于控制操作取消和超时
//   - second: 第二个SYN-ACK包，必须是有效的SYN-ACK包且来自服务端
//   - dst: 目标管道通道，用于发送处理后的数据包
//
// 返回值:
//   - error: 如果数据包不是有效的SYN-ACK包或方向错误，返回错误；否则返回nil
//
// TCP三次握手流程:
//  1. 客户端发送SYN (seq=x)
//  2. 服务端发送SYN-ACK (seq=y, ack=x+1) ← 此方法处理此包
//  3. 客户端发送ACK (seq=x+1, ack=y+1)
func (s *Side[P]) handshakeWithSecondSynAck(second P) error {
	if !second.IsSynAck() || s.cli2Srv {
		return errors.Newf(`%s is not second syn-ack packet`, tcpPacketSeq(second))
	}
	s.state = StateSynReceived
	s.isn = second.SeqNum()
	s.expectedSeq = s.isn + 1
	if err := s.appendPacket(second); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// handshakeWithThirdAck 处理TCP三次握手的第三个ACK包，完成握手过程。
// 该方法处理客户端对服务端SYN-ACK包的确认，验证ACK包的有效性并完成连接建立。
// 双方都进入Established状态，表示TCP连接已成功建立，可以开始数据传输。
//
// 参数:
//   - ctx: 上下文，用于控制操作取消和超时
//   - third: 第三个ACK包，必须是有效的ACK包且来自客户端
//   - otherSide: 对端的Side实例，用于同时更新对端状态
//   - dst: 目标管道通道，用于发送处理后的数据包
//
// 返回值:
//   - error: 如果数据包不是有效的ACK包、方向错误或序列号不正确，返回错误；否则返回nil
//
// 注意:
//   - 客户端期望序列号必须与ACK包的序列号匹配
//   - 该方法同时更新客户端和服务端状态为Established
//   - 连接建立后，双方可以开始正常的数据传输
func (s *Side[P]) handshakeWithThirdAck(third P, otherSide *Side[P]) error {
	if !third.IsAck() || !s.cli2Srv {
		return errors.Newf(`%s is not third ack packet`, tcpPacketSeq(third))
	}
	seq := third.SeqNum()
	if seq != s.expectedSeq {
		return errors.Newf(`%s third ack packet's seq is not in order`, tcpPacketSeq(third))
	}
	s.state = StateEstablished
	otherSide.state = StateEstablished
	if err := s.appendPacket(third); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// reset 重置连接状态，处理RST包导致的强制关闭。
// 该方法处理TCP RST（重置）包，立即终止连接并将双方状态设置为Closed。
// RST包用于异常终止连接，如连接错误、协议违规或应用程序请求。
//
// 参数:
//   - ctx: 上下文，用于控制操作取消和超时
//   - packet: RST数据包，必须设置了RST标志位
//   - otherSide: 对端的Side实例，用于同时终止对端连接
//   - dst: 目标管道通道，用于发送处理后的数据包
//
// 返回值:
//   - error: 如果上下文被取消或发送数据包时发生错误，返回错误；否则返回nil
//
// 注意:
//   - RST包立即终止连接，不进行正常的四次挥手过程
//   - 期望序列号递增1，表示已处理RST包
//   - 连接资源应立即释放，不再接受任何数据包
func (s *Side[P]) reset(packet P, otherSide *Side[P]) error {
	s.state = StateClosed
	otherSide.state = StateClosed
	s.expectedSeq += 1
	if err := s.appendPacket(packet); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// handwaveWithFin 处理FIN包，进入CloseWait状态。
// 该方法处理TCP FIN（结束）包，表示对方希望终止连接。
// 收到FIN包后，发送ACK确认并进入CloseWait状态，等待本地应用发出FIN包。
//
// 参数:
//   - ctx: 上下文，用于控制操作取消和超时
//   - first: FIN数据包，必须设置了FIN标志位
//   - dst: 目标管道通道，用于发送处理后的数据包
//
// 返回值:
//   - error: 如果上下文被取消或发送数据包时发生错误，返回错误；否则返回nil
//
// TCP四次挥手流程（被动关闭方）:
//  1. 收到对方的FIN包
//  2. 发送ACK确认 (此方法处理FIN包并进入CloseWait)
//  3. 等待本地应用发出FIN包
//  4. 收到对方的ACK确认，进入Closed状态
func (s *Side[P]) handwaveWithFin(first P) error {
	s.state = StateCloseWait
	s.expectedSeq += 1
	if err := s.appendPacket(first); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// handwaveWithAck 处理FIN包的ACK确认，进入Closed状态。
// 该方法在发送FIN包后收到对方的ACK确认时调用，完成连接终止过程。
// 进入Closed状态表示连接已完全终止，所有资源可以释放。
//
// 注意:
//   - 该方法不接收参数，仅更新本地状态
//   - 应在确认对方已收到FIN包的ACK后调用
//   - 进入Closed状态后，不再处理任何数据包
func (s *Side[P]) handwaveWithAck() {
	s.state = StateClosed
}

// tryReassembly 尝试重组乱序包，当期望序列号的包到达时进行处理。
// 该方法检查乱序包缓冲区中是否存在期望序列号的数据包，如果存在则进行处理。
// 处理完成后递归调用自身，继续检查下一个期望序列号的数据包，实现连续重组。
//
// 参数:
//   - ctx: 上下文，用于控制操作取消和超时
//   - otherSide: 对端的Side实例，用于处理可能的状态更新
//   - dst: 目标管道通道，用于发送重组后的数据包
//
// 返回值:
//   - error: 如果处理数据包时发生错误，返回错误；否则返回nil
//
// 算法流程:
//  1. 检查OutOrderPackets中是否存在ExpectedSeq对应的数据包
//  2. 如果存在，调用addPacket处理该数据包
//  3. 从缓冲区中删除已处理的数据包
//  4. 递归调用自身，继续处理下一个期望序列号的数据包
//  5. 如果不存在，返回nil表示重组完成
func (s *Side[P]) tryReassembly(otherSide *Side[P]) error {
	for {
		packet, ok := s.outOrderPackets[s.expectedSeq]
		if !ok {
			return nil
		}
		delete(s.outOrderPackets, s.expectedSeq)
		err := s.addPacket(packet, otherSide)
		if err != nil {
			return errors.WithStack(err)
		}
	}
}

// addPacket 添加数据包到该端，处理重传包、乱序包和FIN包。
// 这是Side的核心数据处理方法，根据数据包序列号决定处理策略：
// 1. 重传包 (seq < ExpectedSeq): 忽略，已处理过
// 2. 乱序包 (seq > ExpectedSeq): 存入缓冲区，等待后续重组
// 3. 按序包 (seq == ExpectedSeq): 立即处理，可能触发状态更新
//
// 参数:
//   - ctx: 上下文，用于控制操作取消和超时
//   - packet: 要添加的TCP数据包
//   - otherSide: 对端的Side实例，用于处理连接关闭确认
//   - dst: 目标管道通道，用于发送处理后的数据包
//
// 返回值:
//   - error: 如果处理数据包时发生错误，返回错误；否则返回nil
//
// 处理逻辑:
//   - 重传包: 序列号小于期望序列号，表示已处理过，直接忽略
//   - 乱序包: 序列号大于期望序列号，存入OutOrderPackets缓冲区
//   - 按序包: 序列号等于期望序列号，进一步处理：
//     1. 如果对端处于CloseWait状态且确认号匹配，确认连接关闭
//     2. 如果当前处于Established状态且是FIN包，处理连接终止
//     3. 否则，按正常数据包处理
func (s *Side[P]) addPacket(packet P, otherSide *Side[P]) error {
	test.Tracer.Begin("side.addPacket")
	defer test.Tracer.End()
	// 若连接已关闭，直接忽略
	if s.isCompleted() {
		return nil
	}

	// 强制短连, 并关闭两侧的管道
	if packet.IsRst() {
		if err := s.reset(packet, otherSide); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}
	// 完成三次握手前，tcp 包不会进入缓冲区
	if s.isListen() && otherSide.isSynSent() {
		return s.handshakeWithSecondSynAck(packet)
	}
	if s.isSynSent() && otherSide.isSynReceived() {
		return s.handshakeWithThirdAck(packet, otherSide)
	}
	// 处理乱序包
	seq := packet.SeqNum()
	if seqLess(seq, s.expectedSeq) {
		return nil
	} else if seqLess(s.expectedSeq, seq) {
		if s.outOrderPackets == nil {
			s.outOrderPackets = make(map[uint32]P)
		} else if len(s.outOrderPackets) >= s.maxOutOrderPacketSize {
			return errors.Newf(`Out order packet size is over max size %d`, s.maxOutOrderPacketSize)
		}
		s.outOrderPackets[seq] = packet
		return nil
	}
	// 处理按序包
	if otherSide.isCloseWait() && packet.AckNum() == otherSide.expectedSeq {
		otherSide.handwaveWithAck()
	}
	if s.isEstablished() && packet.IsFin() {
		return s.handwaveWithFin(packet)
	}
	if err := s.appendPacket(packet); err != nil {
		return errors.WithStack(err)
	}
	if err := s.tryReassembly(otherSide); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (s *Side[P]) release() {
	s.buffer.Release()
}

// sideAux 是 Side 序列化的辅助结构体，导出 TCP 状态字段和乱序包数据。
// 运行时字段（buffer、frameStack）不参与序列化。
type sideAux struct {
	Cli2Srv               bool                       `json:"cli2srv"`
	State                 TCPState                   `json:"state"`
	Isn                   uint32                     `json:"isn"`
	ExpectedSeq           uint32                     `json:"expected_seq"`
	MaxOutOrderPacketSize int                        `json:"max_out_order_packet_size"`
	OutOrderPackets       map[uint32]json.RawMessage `json:"out_order_packets,omitempty"`
}

// MarshalJSON 实现 Side[P] 的 JSON 序列化。
// 序列化 TCP 状态字段和乱序包数据，跳过运行时缓冲区、帧堆栈。
func (s Side[P]) MarshalJSON() ([]byte, error) {
	ooPackets := make(map[uint32]json.RawMessage, len(s.outOrderPackets))
	for seq, pkt := range s.outOrderPackets {
		raw, err := json.Marshal(pkt)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		ooPackets[seq] = raw
	}
	return json.Marshal(sideAux{
		Cli2Srv:               s.cli2Srv,
		State:                 s.state,
		Isn:                   s.isn,
		ExpectedSeq:           s.expectedSeq,
		MaxOutOrderPacketSize: s.maxOutOrderPacketSize,
		OutOrderPackets:       ooPackets,
	})
}

// UnmarshalJSON 实现 Side[P] 的 JSON 反序列化。
// 恢复 TCP 状态字段和乱序包数据。运行时缓冲区、帧堆栈保持零值。
func (s *Side[P]) UnmarshalJSON(data []byte) error {
	var aux sideAux
	if err := json.Unmarshal(data, &aux); err != nil {
		return errors.WithStack(err)
	}
	s.cli2Srv = aux.Cli2Srv
	s.state = aux.State
	s.isn = aux.Isn
	s.expectedSeq = aux.ExpectedSeq
	s.maxOutOrderPacketSize = aux.MaxOutOrderPacketSize
	if len(aux.OutOrderPackets) > 0 {
		s.outOrderPackets = make(map[uint32]P, len(aux.OutOrderPackets))
		for seq, raw := range aux.OutOrderPackets {
			var p P
			if err := json.Unmarshal(raw, &p); err != nil {
				return errors.WithStack(err)
			}
			s.outOrderPackets[seq] = p
		}
	}
	return nil
}
