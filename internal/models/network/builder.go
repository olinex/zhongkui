package network

import (
	"fmt"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	reporter_v1 "zhongkui.com/zhongkui/internal/pkg/protobuf/reporter/v1"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// ExtractLayerInfos 从数据包中提取网络层信息
// 该函数递归地解析网络数据包的各个协议层，构建层堆栈并提取载荷数据
//
// 参数:
//   - packet: gopacket.Packet 类型，表示原始网络数据包
//   - baseLevel: LayerLevel 类型，表示解析的起始层级（通常为链路层 LayerLevelLink）
//
// 返回值:
//   - currentLevel: LayerLevel 类型，表示解析完成后的当前层级
//   - stack: LayerStack 类型，包含所有解析出的网络层信息堆栈
//   - payload: []byte 类型，表示应用层载荷数据（如果有）
//
// 功能说明:
//  1. 支持多种网络协议层解析，包括以太网、IPv4/IPv6、TCP/UDP、隧道协议等
//  2. 处理隧道封装（VXLAN、GRE、Geneve、MPLS等）的递归解析
//  3. 支持IP-in-IP封装（IPIP）的嵌套解析
//  4. 自动构建从链路层到传输层的完整协议栈
//  5. 提取最终的应用层载荷数据
//
// 支持的协议层类型:
//   - 链路层: Ethernet
//   - 网络层: IPv4, IPv6, IPv6扩展头（Destination, Fragment, HopByHop, Routing）
//   - 传输层: TCP, UDP
//   - 隧道协议: VXLAN, GRE, Geneve, MPLS
//   - 应用层: TLS（暂未实现详细解析）, 原始载荷
func ExtractLayerInfos(packet gopacket.Packet, baseLevel LayerLevel) (currentLevel LayerLevel, stack LayerStack, payload []byte) {
	currentLevel = baseLevel
	nextLevel := currentLevel
	mainLayers := packet.Layers()
	stack = make(LayerStack, 0, 4)
	opts := gopacket.DecodeOptions{NoCopy: true, Lazy: true}
	for _, layer := range mainLayers {
		switch layerType := layer.LayerType(); layerType {
		case layers.LayerTypeEthernet:
			currentLevel = nextLevel
			eth := layer.(*layers.Ethernet)
			headers := NewEthernetHeaders(eth)
			info := LayerInfo{
				Level:   currentLevel,
				Proto:   layerType,
				Headers: headers,
			}
			stack = append(stack, &info)
		case layers.LayerTypeLoopback:
			// Loopback 链路层（pcap DLT_NULL/DLT_LOOP），不包含 MAC 地址
			// 递归解析内层数据包，起始层级为网络层（无链路层头部）
			currentLevel = nextLevel
			loop := layer.(*layers.Loopback)
			info := LayerInfo{
				Level:   currentLevel,
				Proto:   layerType,
				Headers: NewLoopbackHeaders(),
			}
			stack = append(stack, &info)
			innerPacket := gopacket.NewPacket(layer.LayerPayload(), loop.NextLayerType(), opts)
			innerLevel, innerInfos, innerPayload := ExtractLayerInfos(innerPacket, LayerLevelNetwork)
			stack = append(stack, innerInfos...)
			payload = innerPayload
			currentLevel = innerLevel
			return
		case layers.LayerTypeIPv4:
			currentLevel = nextLevel
			ip := layer.(*layers.IPv4)
			headers := NewIPv4Headers(ip)
			info := LayerInfo{
				Level:   currentLevel,
				Proto:   layerType,
				Headers: headers,
			}
			stack = append(stack, &info)
			// 处理IP-in-IP封装（IPIP协议）
			if ip.Protocol == layers.IPProtocolIPIP {
				innerPacket := gopacket.NewPacket(ip.LayerPayload(), layers.LayerTypeIPv4, opts)
				innerLevel, innerInfos, innerPayload := ExtractLayerInfos(innerPacket, currentLevel)
				stack = append(stack, innerInfos...)
				payload = innerPayload
				currentLevel = innerLevel
				return
			}
		case layers.LayerTypeIPv6:
			currentLevel = nextLevel
			ip := layer.(*layers.IPv6)
			headers := NewIPv6Headers(ip)
			info := LayerInfo{
				Level:   currentLevel,
				Proto:   layerType,
				Headers: headers,
			}
			stack = append(stack, &info)
		case layers.LayerTypeIPv6Destination:
			dest := layer.(*layers.IPv6Destination)
			headers := NewIPv6Destination(dest)
			info := LayerInfo{
				Level:   currentLevel,
				Proto:   layerType,
				Headers: headers,
			}
			stack = append(stack, &info)
			continue
		case layers.LayerTypeIPv6Fragment:
			fragment := layer.(*layers.IPv6Fragment)
			headers := NewIPv6Fragment(fragment)
			info := LayerInfo{
				Level:   currentLevel,
				Proto:   layerType,
				Headers: headers,
			}
			stack = append(stack, &info)
			continue
		case layers.LayerTypeIPv6HopByHop:
			headers := NewIPv6HopByHop(layer.(*layers.IPv6HopByHop))
			info := LayerInfo{
				Level:   currentLevel,
				Proto:   layerType,
				Headers: headers,
			}
			stack = append(stack, &info)
			continue
		case layers.LayerTypeIPv6Routing:
			routing := layer.(*layers.IPv6Routing)
			headers := NewIPv6Routing(routing)
			info := LayerInfo{
				Level:   currentLevel,
				Proto:   layerType,
				Headers: headers,
			}
			stack = append(stack, &info)
			continue
		case layers.LayerTypeTCP:
			currentLevel = nextLevel
			tcp := layer.(*layers.TCP)
			headers := NewTCPHeaders(tcp)
			info := LayerInfo{
				Level:   currentLevel,
				Proto:   layerType,
				Headers: headers,
			}
			stack = append(stack, &info)
		case layers.LayerTypeUDP:
			currentLevel = nextLevel
			udp := layer.(*layers.UDP)
			headers := NewUDPHeaders(udp)
			info := LayerInfo{
				Level:   currentLevel,
				Proto:   layerType,
				Headers: headers,
			}
			stack = append(stack, &info)
		case layers.LayerTypeTLS:
			// TLS层暂未实现详细解析，保留占位符
		case layers.LayerTypeVXLAN:
			currentLevel = nextLevel
			vxlan := layer.(*layers.VXLAN)
			headers := NewVXLANChannel(vxlan)
			info := LayerInfo{
				Level:   currentLevel,
				Proto:   layerType,
				Headers: headers,
			}
			stack = append(stack, &info)
			// VXLAN隧道封装：解析内层以太网帧
			innerPacket := gopacket.NewPacket(layer.LayerPayload(), layers.LayerTypeEthernet, opts)
			innerLevel, innerInfos, innerPayload := ExtractLayerInfos(innerPacket, LayerLevelLink)
			stack = append(stack, innerInfos...)
			payload = innerPayload
			currentLevel = innerLevel
			return
		case layers.LayerTypeGRE:
			currentLevel = nextLevel
			gre := layer.(*layers.GRE)
			headers := NewGREChannel(gre)
			info := LayerInfo{
				Level:   currentLevel,
				Proto:   layerType,
				Headers: headers,
			}
			stack = append(stack, &info)
			// GRE隧道封装：根据协议类型解析内层数据包
			innerPacket := gopacket.NewPacket(layer.LayerPayload(), gre.Protocol, opts)
			innerBaseLevel := getTunnelInnerBaseLevel(gre.Protocol)
			innerLevel, innerInfos, innerPayload := ExtractLayerInfos(innerPacket, innerBaseLevel)
			stack = append(stack, innerInfos...)
			payload = innerPayload
			currentLevel = innerLevel
			return
		case layers.LayerTypeGeneve:
			currentLevel = nextLevel
			gnv := layer.(*layers.Geneve)
			headers := NewGeneveChannel(gnv)
			info := LayerInfo{
				Level:   currentLevel,
				Proto:   layerType,
				Headers: headers,
			}
			stack = append(stack, &info)
			// Geneve隧道封装：根据协议类型解析内层数据包
			innerPacket := gopacket.NewPacket(layer.LayerPayload(), gnv.Protocol, opts)
			innerBaseLevel := getTunnelInnerBaseLevel(gnv.Protocol)
			innerLevel, innerInfos, innerPayload := ExtractLayerInfos(innerPacket, innerBaseLevel)
			stack = append(stack, innerInfos...)
			payload = innerPayload
			currentLevel = innerLevel
			return
		case layers.LayerTypeMPLS:
			currentLevel = nextLevel
			mpls := layer.(*layers.MPLS)
			headers := NewMPLSChannel(mpls)
			info := LayerInfo{
				Level:   currentLevel,
				Proto:   layerType,
				Headers: headers,
			}
			stack = append(stack, &info)
			// MPLS标签栈：递归解析下一个MPLS标签或内层数据包
			innerPacket := gopacket.NewPacket(layer.LayerPayload(), layers.LayerTypeMPLS, opts)
			innerLevel, innerInfos, innerPayload := ExtractLayerInfos(innerPacket, currentLevel.Next())
			stack = append(stack, innerInfos...)
			payload = innerPayload
			currentLevel = innerLevel
			return
		case gopacket.LayerTypePayload:
			// 到达载荷层，提取原始数据
			raw := layer.(*gopacket.Payload)
			payload = raw.Payload()
			return
		case gopacket.LayerTypeDecodeFailure:
			// 解码失败，直接提取载荷数据
			raw := layer.(*gopacket.DecodeFailure)
			info := LayerInfo{
				Level:   nextLevel,
				Proto:   layerType,
				Headers: nil,
			}
			payload = raw.LayerContents()
			stack = append(stack, &info)
			return
		default:
			// 未知协议层，记录到堆栈中并提取载荷数据
			currentLevel = nextLevel
			info := LayerInfo{
				Level:   currentLevel,
				Proto:   layerType,
				Headers: nil,
			}
			stack = append(stack, &info)
			payload = layer.LayerPayload()
			return
		}
		nextLevel = currentLevel.Next()
	}
	return
}

// getTunnelInnerBaseLevel 根据隧道封装的协议类型确定内层数据包的起始层级。
//
// 对于透明以太网桥接（TransparentEthernetBridging），内层包含完整的以太网帧，
// 起始层级为链路层；对于 IPv4/IPv6 等网络层协议，内层直接以网络层开始。
// 其他未知协议默认从链路层开始解析。
func getTunnelInnerBaseLevel(proto layers.EthernetType) LayerLevel {
	switch proto {
	case layers.EthernetTypeIPv4, layers.EthernetTypeIPv6:
		return LayerLevelNetwork
	case layers.EthernetTypeTransparentEthernetBridging:
		return LayerLevelLink
	default:
		return LayerLevelLink
	}
}

// ModelBuilder 数据包模型构建器
// 用于将原始网络数据包转换为特定层级的结构化数据模型
// 支持从链路层到传输层的模型构建，便于后续存储、查询和分析
type ModelBuilder struct {
	Timestamp     time.Time               // 数据包捕获时间戳
	Truncated     bool                    // 数据包是否被截断
	CaptureLength uint32                  // 捕获的数据包长度
	SessionID     serializer.HexUUID      // 会话ID，唯一标识一次数据捕获会话
	Source        reporter_v1.ResourceRaw // 数据源信息（命名空间、捕获器、版本等）
	Level         LayerLevel              // 数据包的最高解析层级
	Stack         LayerStack              // 网络层堆栈信息
	Payload       []byte                  // 应用层载荷数据
}

// BuildAsPacketLinkModel 构建链路层数据包模型
// 将原始数据包转换为链路层（OSI第2层）的数据模型
//
// 返回值:
//   - any: 返回具体的链路层数据包模型，目前支持 PacketEthernet
//
// 异常:
//   - 如果层堆栈为空，会触发panic
//   - 如果第一层不是链路层，会触发panic
//   - 如果头部信息为nil，会触发panic
//   - 如果协议类型不支持，会触发panic
//
// 功能说明:
//  1. 验证层堆栈的有效性
//  2. 提取以太网头部信息
//  3. 构建包含完整元数据的以太网数据包模型
//  4. 返回可用于数据库存储的结构化数据
func (b *ModelBuilder) BuildAsPacketLinkModel() any {
	if len(b.Stack) < 1 {
		return nil
	}
	info := b.Stack[0]
	if info.Level != LayerLevelLink {
		return nil
	}
	if info.Headers == nil {
		return nil
	}
	switch info.Proto {
	case layers.LayerTypeEthernet, layers.LayerTypeLoopback:
		eth := info.Headers.(*EthernetHeaders)
		return &PacketEthernet{
			AsPacket: AsPacket{
				BaseMeta: BaseMeta{
					SessionID: b.SessionID,
					Namespace: b.Source.Namespace,
					Capture:   b.Source.Name,
					Version:   b.Source.Version,
				},
				TimestampMeta: TimestampMeta{
					Timestamp: b.Timestamp,
				},
				CaptureMeta: CaptureMeta{
					Truncated:     b.Truncated,
					CaptureLength: uint32(b.CaptureLength),
				},
				PacketMeta: PacketMeta{
					LayerStack: b.Stack,
				},
				BytesPayload: BytesPayload{
					Payload: b.Payload,
				},
			},
			EthernetHeaders: *eth,
		}
	default:
		return nil
	}
}

// BuildAsPacketNetworkModel 构建网络层数据包模型
// 将原始数据包转换为网络层（OSI第3层）的数据模型
// 支持IPv4和IPv6协议，构建包含链路层和网络层信息的完整数据包模型
//
// 返回值:
//   - any: 返回具体的网络层数据包模型，支持 PacketIPv4 或 PacketIPv6
//
// 异常:
//   - 如果层堆栈少于2层，会触发panic
//   - 如果第二层不是网络层，会触发panic
//   - 如果头部信息为nil，会触发panic
//   - 如果链路层模型构建失败，会触发panic
//   - 如果协议类型不支持，会触发panic
//
// 功能说明:
//  1. 验证网络层堆栈的有效性
//  2. 构建底层链路层模型作为基础
//  3. 提取IPv4或IPv6头部信息
//  4. 组合链路层和网络层信息构建完整模型
//  5. 返回可用于数据库存储的结构化数据
func (b *ModelBuilder) BuildAsPacketNetworkModel() any {
	if len(b.Stack) < 2 {
		panic("network layer stack have not enough layers")
	}
	info := b.Stack[1]
	if info.Level != LayerLevelNetwork {
		panic("second stack layer is not network layer")
	}
	if info.Headers == nil {
		panic("second stack layer headers is nil")
	}
	switch info.Proto {
	case layers.LayerTypeIPv4:
		net := info.Headers.(*IPv4Headers)
		model := b.BuildAsPacketLinkModel()
		if model == nil {
			return nil
		}
		return &PacketIPv4{
			PacketEthernet: *model.(*PacketEthernet),
			IPv4Headers:    *net,
		}
	case layers.LayerTypeIPv6:
		net := info.Headers.(*IPv6Headers)
		model := b.BuildAsPacketLinkModel()
		if model == nil {
			return nil
		}
		return &PacketIPv6{
			PacketEthernet: *model.(*PacketEthernet),
			IPv6Headers:    *net,
		}
	default:
		panic(fmt.Sprintf("unknown network layer protocol %s", info.Proto))
	}
}

// BuildAsPacketTransportModel 构建传输层数据包模型
// 将原始数据包转换为传输层（OSI第4层）的数据模型
// 支持TCP和UDP协议，构建包含链路层、网络层和传输层信息的完整数据包模型
//
// 返回值:
//   - any: 返回具体的传输层数据包模型，支持 PacketIPv4TCP、PacketIPv4UDP、PacketIPv6TCP 或 PacketIPv6UDP
//
// 异常:
//   - 如果层堆栈少于3层，会触发panic
//   - 如果最后一层不是传输层，会触发panic
//   - 如果头部信息为nil，会触发panic
//   - 如果网络层模型构建失败，会触发panic
//   - 如果协议类型不支持，会触发panic
//
// 功能说明:
//  1. 验证传输层堆栈的有效性（最后一层必须是传输层）
//  2. 构建底层网络层模型作为基础
//  3. 根据网络层协议类型（IPv4/IPv6）选择相应的传输层处理逻辑
//  4. 提取TCP或UDP头部信息
//  5. 组合链路层、网络层和传输层信息构建完整模型
//  6. 返回可用于数据库存储的结构化数据
//
// 注意:
//   - 该函数假设传输层是协议栈的最后一层（在应用层之前）
//   - 支持IPv4/TCP、IPv4/UDP、IPv6/TCP、IPv6/UDP四种组合
func (b *ModelBuilder) BuildAsPacketTransportModel() any {
	if len(b.Stack) < 3 {
		panic("transport layer stack have not enough layers")
	}
	info := b.Stack[len(b.Stack)-1]
	if info.Level != LayerLevelTransport {
		panic("last stack layer is not transport layer")
	}
	if info.Headers == nil {
		return nil
	}
	model := b.BuildAsPacketNetworkModel()
	if model == nil {
		return nil
	}
	switch model.(type) {
	case *PacketIPv4:
		switch info.Proto {
		case layers.LayerTypeTCP:
			tcp := info.Headers.(*TCPHeaders)
			return &PacketIPv4TCP{
				PacketIPv4: *model.(*PacketIPv4),
				TCPHeaders: *tcp,
			}
		case layers.LayerTypeUDP:
			udp := info.Headers.(*UDPHeaders)
			return &PacketIPv4UDP{
				PacketIPv4: *model.(*PacketIPv4),
				UDPHeaders: *udp,
			}
		default:
			panic(fmt.Sprintf("unknown transport layer protocol %s", info.Proto))
		}
	case *PacketIPv6:
		switch info.Proto {
		case layers.LayerTypeTCP:
			tcp := info.Headers.(*TCPHeaders)
			return &PacketIPv6TCP{
				PacketIPv6: *model.(*PacketIPv6),
				TCPHeaders: *tcp,
			}
		case layers.LayerTypeUDP:
			udp := info.Headers.(*UDPHeaders)
			return &PacketIPv6UDP{
				PacketIPv6: *model.(*PacketIPv6),
				UDPHeaders: *udp,
			}
		default:
			panic(fmt.Sprintf("unknown transport layer protocol %s", info.Proto))
		}
	default:
		panic(fmt.Sprintf("unknown transport layer protocol %s", info.Proto))
	}
}

// BuildAsPacketApplicationModel 构建应用层数据包模型
// 将原始数据包转换为应用层（OSI第7层）的数据模型
// 注意：当前版本暂未实现应用层模型的详细构建，返回nil作为占位符
//
// 返回值:
//   - any: 返回具体的应用层数据包模型，当前版本返回nil
//
// 功能说明:
//  1. 为应用层模型构建提供接口占位符
//  2. 未来可扩展支持HTTP、DNS、TLS等应用层协议解析
//  3. 保持接口一致性，便于后续功能扩展
func (b *ModelBuilder) BuildAsPacketApplicationModel() any {
	return nil
}

// BuildAsPacketModel 根据层级构建数据包模型
// 根据数据包的最高解析层级，自动选择相应的模型构建方法
// 提供统一的接口构建不同层级的网络数据包模型
//
// 返回值:
//   - any: 返回对应层级的数据包模型
//
// 异常:
//   - 如果层级未知，会触发panic
//
// 功能说明:
//  1. 根据Level字段判断数据包的解析深度
//  2. 自动调用相应层级的模型构建方法
//  3. 提供从链路层到应用层的统一构建接口
//  4. 简化上层代码调用，无需手动判断层级
func (b *ModelBuilder) BuildAsPacketModel() any {
	switch b.Level {
	case LayerLevelLink:
		return b.BuildAsPacketLinkModel()
	case LayerLevelNetwork:
		return b.BuildAsPacketNetworkModel()
	case LayerLevelTransport:
		return b.BuildAsPacketTransportModel()
	case LayerLevelApplication:
		return b.BuildAsPacketApplicationModel()
	default:
		panic(fmt.Sprintf("unknown layer level %d", b.Level))
	}
}

// NewPacketModelBuilder 创建数据包模型构建器
// 工厂函数，用于从原始网络数据包创建ModelBuilder实例
//
// 参数:
//   - packet: gopacket.Packet 类型，原始网络数据包
//   - sessionID: serializer.HexUUID 类型，数据捕获会话的唯一标识
//   - source: reporter_v1.ResourceRaw 类型，数据源信息（命名空间、捕获器、版本）
//
// 返回值:
//   - *ModelBuilder: 初始化完成的模型构建器实例
//
// 异常:
//   - 如果数据包元数据为nil，会触发panic
//
// 功能说明:
//  1. 提取数据包元数据（时间戳、截断状态、捕获长度）
//  2. 调用ExtractLayerInfos解析网络层信息
//  3. 初始化ModelBuilder所有字段
//  4. 返回可直接用于模型构建的实例
func NewPacketModelBuilder(packet gopacket.Packet, sessionID serializer.HexUUID, source reporter_v1.ResourceRaw) *ModelBuilder {
	metadata := packet.Metadata()
	if metadata == nil {
		panic("packet metadata is nil")
	}
	level, stack, payload := ExtractLayerInfos(packet, LayerLevelLink)
	return &ModelBuilder{
		Timestamp:     metadata.Timestamp,
		Truncated:     metadata.Truncated,
		CaptureLength: uint32(metadata.CaptureLength),
		SessionID:     sessionID,
		Source:        source,
		Level:         level,
		Stack:         stack,
		Payload:       payload,
	}
}
