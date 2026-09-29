// Package detectors 定义应用层协议推测器接口与注册表，并内置 HTTP 协议的检测、提取与组装实现。
//
// 本文解释 zhongkui 如何从原始数据包里"看懂"一次应用层会话：先基于 TCP/IP 头做被动指纹，
// 再把同一会话的多段数据重组为一次请求/响应，最终组装成可打标、可展示的业务模型。
//
// ## 协议推测器
//
// base.go 定义了三件事：
//
//   - AppProtocolDetector 接口：Detect 判断某段数据是否像该协议；Extract 从字节流中解析出协议对象；
//     Build 把解析结果与流元信息组装为目标模型。三步分离，便于按协议渐进实现。
//   - AppTailer 与 StreamMeta：StreamMeta 携带会话级元信息（TraceID、客户端/服务端地址与端口、
//     ISN、被动 TCP 指纹 PacketTCPFingerprint、时间戳），供 Build 阶段组装模型使用。
//   - Detectors 注册表与 RegisterAppProtocolDetector：各协议在包初始化时登记，供重组器按数据特征选择。
//
// 目前内置 AppHttp（HTTP/1.0、HTTP/1.1）实现（http.go）：HttpDetector.Detect 先做最小长度判断，
// 再区分请求/响应方向；Extract 用 http.ReadRequest / http.ReadResponse 解析；Build 生成
// network.HttpTransport，并回填 TCP 指纹与 HTTP 指纹。
//
// ## 被动 TCP 指纹从哪来
//
// 指纹能力来自 internal/pkg/huginn——一个 p0f 风格被动指纹识别的纯 Go 移植。
// 它把 p0f 签名库 p0f.fp 与本地扩展 p0f.local.fp 以内嵌方式（go:embed）加载、拼接并建立索引，
// 对外提供 AnalyzeTCPIPv4 / AnalyzeTCPIPv6 / AnalyzeHTTPRequest / AnalyzeHTTPResponse，
// 分别返回 TCPFingerprint（OS、OSQuality、Sig、MTU）与 HTTPFingerprint（Browser、Server、Language、Quality）。
//
// 指纹的原始输入是 TCP 握手包。流水线在 internal/worker/handlers/pcap/transformer.go 的
// enrichTCPFingerprint 中，仅对 SYN / SYN-ACK 包拼接网络层与传输层的原始头字节：
// IPv4 走 AnalyzeTCPIPv4、IPv6 走 AnalyzeTCPIPv6，把结果回填到 PacketTCPFingerprint，
// 随数据包模型写入 `packet_ipv4_tcps` / `packet_ipv6_tcps`。
//
// 之所以只看握手包，是因为操作系统的 TCP 栈特征（TTL、窗口大小、选项布局、MSS、各类 quirk）
// 集中在三次握手的前两个包里——它们是内核协议栈"自报家门"的独特组合，稳定且可与已知指纹库比对；
// 连接一旦建立，这些字段便不再出现。识别失败（未匹配到签名或解析失败）只记录指标，不阻断导入流水线。
//
// ## 从指纹到标签输入
//
// 组包阶段把数据包透传为 HttpTransport：TCP 指纹落入 fpTcpClientSig / fpTcpServerSig，
// HTTP 指纹落入浏览器/服务器等字段。它们既是展示信息，也是标签引擎的输入——这正是
// internal/tagengine 能按"客户端/服务端指纹签名"维度做窗口统计与告警的基础。
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
