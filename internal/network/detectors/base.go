// Package detectors 定义应用层协议推测器接口与注册表，并内置 HTTP 协议的检测、提取与组装实现。
package detectors

import (
	"bufio"
	"context"
	"net"
	"time"

	"github.com/gopacket/gopacket/layers"
	n "zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// ErrDataIncomplete 表示应用层数据尚未完整到达。
// 当 Extract 阶段发现数据不足以构成完整消息时，Detector 实现应返回此错误，
// 以通知 parse 循环等待后续数据包到达后重试，而非将流视为失败终止。
var ErrDataIncomplete = errors.New("application data incomplete, retry later")

// AppProtocol 表示应用层协议的枚举类型。
// 该类型用于标识TCP数据流中承载的应用层协议类型，支持协议识别和协议特定的数据处理。
// 在TCP流重组过程中，系统会根据数据内容自动识别协议类型，并将识别结果存储在此字段中。
type AppProtocol string

// Detectors 存储所有已注册的应用层协议推测器。
var Detectors map[AppProtocol]AppProtocolDetector = make(map[AppProtocol]AppProtocolDetector)

// RegisterAppProtocolDetector 注册一个应用层协议推测器。
// protocol: 要注册的协议类型
// detector: 协议推测器实现
func RegisterAppProtocolDetector(protocol AppProtocol, detector AppProtocolDetector) {
	Detectors[protocol] = detector
}

// StreamMeta 描述一条已重组 TCP 流的元数据：地址、端口、ISN、被动指纹与时间戳。
type StreamMeta struct {
	// TraceID 流跟踪ID，用于关联不同数据包的元数据
	TraceID serializer.HexUUID
	// ClientAddr 客户端IP地址
	ClientAddr net.IP
	// ClientPort 客户端端口号
	ClientPort layers.TCPPort
	// ClientISN 客户端ISN
	ClientISN uint32
	// ClientFingerprint 客户端被动 TCP 指纹（pcap 阶段对 SYN 包识别，组包时透传），无指纹为 nil
	ClientFingerprint *n.PacketTCPFingerprint
	// ServerAddr 服务端IP地址
	ServerAddr net.IP
	// ServerPort 服务端端口号
	ServerPort layers.TCPPort
	// ServerISN 服务端ISN
	ServerISN uint32
	// ServerFingerprint 服务端被动 TCP 指纹（pcap 阶段对 SYN-ACK 包识别，组包时透传），无指纹为 nil
	ServerFingerprint *n.PacketTCPFingerprint
	// Timestamp 重组器第一个 syn 包的时间戳
	Timestamp time.Time
}

// AppTailer 记录一条流在提取阶段消费的客户端/服务端数据量、偏移与时间戳。
type AppTailer struct {
	// ClientConsumed、ServerConsumed 分别为客户端、服务端方向已消费的字节数。
	ClientConsumed, ServerConsumed uint64
	// ClientStartPayloadOffset、ClientEndPayloadOffset 为客户端方向的起始/结束负载偏移。
	ClientStartPayloadOffset, ClientEndPayloadOffset uint64
	// ClientStartTimestamp、ClientEndTimestamp 为客户端方向的起始/结束时间戳。
	ClientStartTimestamp, ClientEndTimestamp time.Time
	// ServerStartPayloadOffset、ServerEndPayloadOffset 为服务端方向的起始/结束负载偏移。
	ServerStartPayloadOffset, ServerEndPayloadOffset uint64
	// ServerStartTimestamp、ServerEndTimestamp 为服务端方向的起始/结束时间戳。
	ServerStartTimestamp, ServerEndTimestamp time.Time
}

// BuildTailer 是构建 AppTailer 的回调，供组装阶段取用流的消费偏移信息。
type BuildTailer func(ctx context.Context) (tailer *AppTailer, err error)

// AppProtocolDetector 应用层协议推测器接口。
// 用于推测数据包中的应用层协议类型，并判断数据是否完整。
type AppProtocolDetector interface {
	// Detect 判断当前方向的数据是否匹配本协议。
	Detect(
		ctx context.Context,
		peek []byte,
		c2s bool,
	) (ok bool, err error)

	// Extract 从双向缓冲区中提取完整的请求与响应对象。
	Extract(
		ctx context.Context,
		clientReader, serverReader *bufio.Reader,
	) (clientPayload, serverPayload any, err error)

	// Build 将提取到的请求/响应组装为应用层对象。
	Build(
		ctx context.Context,
		baseMeta *n.BaseMeta,
		streamMeta *StreamMeta,
		streamIndex uint32,
		clientPayload, serverPayload any,
		buildTailer BuildTailer,
	) (app any, err error)
}
