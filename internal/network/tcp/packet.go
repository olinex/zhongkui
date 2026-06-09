package tcp

import (
	"fmt"
	"net"

	"github.com/gopacket/gopacket/layers"
	n "zhongkui.com/zhongkui/internal/models/network"
)

// TCPPacket 是TCP数据包的通用接口，封装了网络层捕获的TCP数据包信息。
// 该接口支持泛型设计，可适配不同的TCP数据包实现，如IPv4 TCP和IPv6 TCP。
// 接口定义了TCP数据包的核心属性和方法，用于在网络数据包处理流水线中进行
// 数据包解析、流重组、应用层协议识别等操作。
type TCPPacket interface {
	// Meta 返回数据包的基础元数据信息。
	// 包含命名空间、捕获器名称、版本号和会话ID等元数据字段。
	// 这些信息用于数据包溯源和系统级管理。
	Meta() *n.BaseMeta

	// StreamID 返回TCP流的唯一标识符。
	// 该标识符基于源IP、目的IP、源端口和目的端口计算得出，
	// 用于将同一TCP连接的数据包关联起来进行流重组。
	StreamID() string

	// FrameInfo 返回数据包的帧信息。
	// 包含时间戳和网络层堆栈信息，用于记录数据包的捕获上下文。
	FrameInfo() *n.FrameInfo

	// SrcAddr 返回数据包的源IP地址。
	// 支持IPv4和IPv6地址格式。
	SrcAddr() net.IP

	// DstAddr 返回数据包的目的IP地址。
	// 支持IPv4和IPv6地址格式。
	DstAddr() net.IP

	// SrcPort 返回数据包的源端口号。
	// 使用layers.TCPPort类型表示TCP端口。
	SrcPort() layers.TCPPort

	// DstPort 返回数据包的目的端口号。
	// 使用layers.TCPPort类型表示TCP端口。
	DstPort() layers.TCPPort

	// SeqNum 返回TCP序列号。
	// 序列号用于标识TCP数据段在数据流中的位置，
	// 确保数据包按正确顺序重组。
	SeqNum() uint32

	// AckNum 返回TCP确认号。
	// 确认号表示期望接收的下一个字节的序列号，
	// 用于TCP的可靠传输机制。
	AckNum() uint32

	// IsSyn 检查TCP数据包是否设置了SYN标志位。
	// SYN标志表示连接建立请求，用于TCP三次握手的第一步。
	// 返回true表示该数据包是SYN包。
	IsSyn() bool

	// IsSynAck 检查TCP数据包是否同时设置了SYN和ACK标志位。
	// SYN-ACK标志表示连接建立确认，用于TCP三次握手的第二步。
	// 返回true表示该数据包是SYN-ACK包。
	IsSynAck() bool

	// IsAck 检查TCP数据包是否设置了ACK标志位。
	// ACK标志表示确认收到数据，用于TCP的确认机制。
	// 返回true表示该数据包是ACK包。
	IsAck() bool

	// IsFin 检查TCP数据包是否设置了FIN标志位。
	// FIN标志表示连接终止请求，用于TCP四次挥手。
	// 返回true表示该数据包是FIN包。
	IsFin() bool

	// IsRst 检查TCP数据包是否设置了RST标志位。
	// RST标志表示连接重置，用于异常终止TCP连接。
	// 返回true表示该数据包是RST包。
	IsRst() bool

	// GetPayload 返回TCP数据包的载荷数据。
	// 载荷是TCP数据包中传输的实际应用层数据，
	// 可能包含HTTP、TLS、DNS等应用层协议数据。
	// 返回字节切片，如果数据包没有载荷则返回空切片。
	GetPayload() []byte
}

// tcpPacketSeq 返回TCP数据包的序列信息字符串，用于调试和日志输出。
// 该函数格式化TCP数据包的关键连接信息，包括源地址、目的地址、端口和序列号，
// 生成易于阅读的字符串表示形式，便于在日志、调试信息和监控指标中使用。
//
// 参数:
//
//	packet - 实现了TCPPacket接口的数据包实例
//
// 返回值:
//
//	格式化的字符串，格式为: "src: <源IP>:<源端口>, dst: <目的IP>:<目的端口>, seq: <序列号>"
//
// 示例:
//
//	"src: 192.168.1.100:8080, dst: 10.0.0.1:443, seq: 1234567890"
func tcpPacketSeq(packet TCPPacket) string {
	return fmt.Sprintf(
		`src: %s:%d, dst: %s:%d, seq: %d`,
		packet.SrcAddr().String(),
		packet.SrcPort(),
		packet.DstAddr().String(),
		packet.DstPort(),
		packet.SeqNum(),
	)
}
