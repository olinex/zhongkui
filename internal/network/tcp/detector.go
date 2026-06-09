package tcp

import (
	"context"
	"net"

	"github.com/gopacket/gopacket/layers"
	n "zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// AppProtocol 表示应用层协议的枚举类型。
// 该类型用于标识TCP数据流中承载的应用层协议类型，支持协议识别和协议特定的数据处理。
// 在TCP流重组过程中，系统会根据数据内容自动识别协议类型，并将识别结果存储在此字段中。
type AppProtocol string

// detectors 存储所有已注册的应用层协议推测器。
var detectors map[AppProtocol]AppProtocolDetector = make(map[AppProtocol]AppProtocolDetector)

// RegisterAppProtocolDetector 注册一个应用层协议推测器。
// protocol: 要注册的协议类型
// detector: 协议推测器实现
func RegisterAppProtocolDetector(protocol AppProtocol, detector AppProtocolDetector) {
	detectors[protocol] = detector
}

// AppProtocolDetector 应用层协议推测器接口。
// 用于推测数据包中的应用层协议类型，并判断数据是否完整。
type AppProtocolDetector interface {
	Detect(
		ctx context.Context,
		peek []byte,
		c2s bool,
	) (ok bool, err error)

	Extract(
		ctx context.Context,
		clientRb, serverRb *serializer.RingBuffer,
	) (clientPayload, serverPayload any, err error)

	Build(
		ctx context.Context,
		meta *n.BaseMeta,
		clientAddr, serverAddr net.IP,
		clientPort, serverPort layers.TCPPort,
		clientFrameStack, serverFrameStack n.FrameStack,
		clientPayload, serverPayload any,
	) (app any, err error)
}
