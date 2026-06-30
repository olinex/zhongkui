package tcp

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/gopacket/gopacket/layers"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// Rectifier 表示一个TCP整流器，用于管理TCP连接的数据包流重组和状态转换。
// 该结构体使用泛型设计，支持不同类型的TCP数据包实现，如IPv4 TCP和IPv6 TCP。
// Rectifier负责跟踪TCP连接的两端（客户端和服务端），处理三次握手、四次挥手、
// 乱序包重组和数据包转发到目标管道等核心功能。
//
// 类型参数:
//   - P: 实现了TCPPacket接口的数据包类型，用于支持不同的TCP数据包实现
//
// 主要功能:
//   - TCP连接状态管理: 跟踪客户端和服务端的TCP状态机
//   - 数据包流重组: 处理乱序到达的数据包，确保按序交付
//   - 连接生命周期管理: 处理连接建立、数据传输和连接终止全过程
//   - 错误检测和恢复: 检测和处理TCP协议违规、缓冲区溢出等异常情况
type Rectifier[P TCPPacket] struct {
	// ClientAddr 客户端IP地址
	// 表示TCP连接中客户端的源IP地址，用于识别连接方向和流关联
	ClientAddr net.IP `json:"client_addr"`

	// ClientPort 客户端端口号
	// 表示TCP连接中客户端的源端口，与ClientAddr共同唯一标识客户端端点
	ClientPort layers.TCPPort `json:"client_port"`

	// Client 客户端侧的TCP流管理
	// 管理客户端到服务端方向的数据包流，包括状态跟踪、序列号管理和乱序包缓冲
	Client *Side[P] `json:"client"`

	// ServerAddr 服务端IP地址
	// 表示TCP连接中服务端的源IP地址，用于识别连接方向和流关联
	ServerAddr net.IP `json:"server_addr"`

	// ServerPort 服务端端口号
	// 表示TCP连接中服务端的源端口，与ServerAddr共同唯一标识服务端端点
	ServerPort layers.TCPPort `json:"server_port"`

	// Server 服务端侧的TCP流管理
	// 管理服务端到客户端方向的数据包流，包括状态跟踪、序列号管理和乱序包缓冲
	Server *Side[P] `json:"server"`
}

// newRectifierWithHandshakeFirstSyn 根据TCP三次握手的第一个SYN包创建一个新的TCP Rectifier。
// 该函数是TCP整流器的工厂函数，用于初始化一个新的TCP连接管理实例。
// 它从TCP三次握手的第一个SYN包中提取连接信息，并初始化客户端和服务端的状态。
//
// 参数:
//   - first: TCP三次握手的第一个SYN包，必须设置了SYN标志位
//
// 返回值:
//   - *Rectifier[P]: 成功时返回初始化的Rectifier实例
//   - error: 如果第一个包不是SYN包，返回错误
//
// 初始化过程:
//  1. 验证数据包是否为SYN包
//  2. 从SYN包中提取客户端和服务端的地址和端口信息
//  3. 创建客户端Side实例，状态设置为StateSynSent
//  4. 创建服务端Side实例，状态设置为StateListen
//  5. 设置最大乱序包缓冲区大小
//
// 注意:
//   - 客户端ISN（初始序列号）从第一个SYN包的序列号确定
//   - 服务端ISN初始为0，在收到SYN包并发送SYN-ACK响应后确定
//   - 该函数假设SYN包来自客户端，方向为客户端到服务端
func newRectifierWithHandshakeFirstSyn[P TCPPacket](
	ctx context.Context,
	first P,
	maxOutOrderPacketSize int,
	rwBufferTimeout time.Duration,
) (*Rectifier[P], error) {
	if clientSide, serverSide, err := newSides(
		ctx,
		first,
		maxOutOrderPacketSize,
		rwBufferTimeout,
	); err != nil {
		return nil, err
	} else {
		return &Rectifier[P]{
			ClientAddr: first.SrcAddr(),
			ClientPort: first.SrcPort(),
			ServerAddr: first.DstAddr(),
			ServerPort: first.DstPort(),
			Client:     clientSide,
			Server:     serverSide,
		}, nil
	}
}

// String 返回TCP Rectifier的字符串表示，包含客户端和服务端的地址信息。
// 该方法实现了fmt.Stringer接口，提供人类可读的TCP连接标识符。
// 字符串格式为: "TCPRectifier(客户端IP:客户端端口->服务端IP:服务端端口)"
//
// 返回值:
//   - string: 格式化的TCP连接标识符字符串
//
// 示例:
//   - "TCPRectifier(192.168.1.100:8080->10.0.0.1:443)"
//   - "TCPRectifier(2001:db8::1:8080->2001:db8::2:443)"
//
// 使用场景:
//   - 调试日志输出
//   - 监控指标标签
//   - 错误信息格式化
func (r *Rectifier[P]) String() string {
	return fmt.Sprintf(
		`TCPRectifier(%s:%d->%s:%d)`,
		r.ClientAddr.String(),
		r.ClientPort,
		r.ServerAddr.String(),
		r.ServerPort,
	)
}

// isSameFlow 检查给定的数据包是否属于当前TCP流。
// 该方法通过比较数据包的源/目的地址和端口与Rectifier记录的客户端/服务端信息，
// 确定数据包是否属于同一个TCP连接以及数据包的方向。
//
// 参数:
//   - packet: 要检查的TCP数据包
//
// 返回值:
//   - same: true表示数据包属于当前TCP流，false表示不属于
//   - c2s: 当same为true时，表示数据包方向（true: 客户端到服务端, false: 服务端到客户端）
//
// 匹配逻辑:
//  1. 客户端到服务端方向匹配:
//     - 数据包源地址/端口 == 客户端地址/端口
//     - 数据包目的地址/端口 == 服务端地址/端口
//  2. 服务端到客户端方向匹配:
//     - 数据包源地址/端口 == 服务端地址/端口
//     - 数据包目的地址/端口 == 客户端地址/端口
//
// 注意:
//   - 该方法支持IPv4和IPv6地址
//   - 使用net.IP.Equal方法进行地址比较，确保正确比较IP地址
//   - 如果两个方向都不匹配，返回same=false，c2s的值无意义
func (r *Rectifier[P]) isSameFlow(packet P) (same, c2s bool) {
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
// 这是Rectifier的核心方法，负责处理TCP连接的完整生命周期，包括：
// 1. 数据包流归属验证
// 2. 缓冲区容量检查
// 3. TCP状态机转换
// 4. 数据包处理和转发
// 5. 乱序包重组尝试
//
// 参数:
//   - ctx: 上下文，用于控制操作取消和超时
//   - packet: 要添加的TCP数据包
//   - clientDst: 客户端到服务端方向的数据包目标管道通道
//   - serverDst: 服务端到客户端方向的数据包目标管道通道
//
// 返回值:
//   - error: 处理过程中发生的错误，包括：
//   - 数据包不属于当前流
//   - 乱序包缓冲区已满
//   - TCP状态转换错误
//   - 数据包处理错误
//   - 上下文被取消
//
// 处理流程:
//  1. 验证数据包是否属于当前TCP流
//  2. 确定数据包方向和对应的Side实例
//  3. 检查乱序包缓冲区容量
//  4. 处理RST包（强制连接重置）
//  5. 处理TCP三次握手过程
//  6. 正常数据包处理
//  7. 尝试乱序包重组
//
// 状态转换处理:
//   - RST包: 立即重置连接，忽略所有未处理数据
//   - 三次握手:
//   - 第二个SYN-ACK包: 服务端从Listen状态转换到SynReceived
//   - 第三个ACK包: 客户端从SynSent状态转换到Established，服务端从SynReceived转换到Established
//   - 正常数据包: 通过Side.addPacket方法处理
//   - FIN包: 通过Side.handwaveWithFin方法处理连接终止
//
// 注意:
//   - 该方法会同时尝试重组当前端和对端的乱序包
//   - 错误处理使用errors.WithStack添加调用栈信息
//   - 通道操作是阻塞的，调用者应确保通道有足够的缓冲区
//   - 如果上下文被取消，方法将返回错误而不是阻塞
func (r *Rectifier[P]) addPacket(ctx context.Context, packet P) error {
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

func (r *Rectifier[P]) isCompleted() bool {
	return r.Client.isCompleted() && r.Server.isCompleted()
}

func (r *Rectifier[P]) release() {
	r.Client.release()
	r.Server.release()
}
