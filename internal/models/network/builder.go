package network

import (
	"fmt"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	reporter_v1 "zhongkuisec.com/zhongkui/internal/pkg/protobuf/reporter/v1"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// ExtractLayerInfos 从数据包中提取网络层信息
// 该函数按层解码注册表逐层解析网络数据包，构建层堆栈并提取载荷数据
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
//  1. 已注册协议由 LayerDecoder 给出层信息与推进方式（继续、递归、终止），
//     本函数只负责层级推进、递归解析与终止编排
//  2. 未注册协议走统一默认规则：按下一层级记录该层（Headers 为 nil），
//     提取剩余载荷后终止，后续可通过注册新解码器改变行为
//  3. 解码器返回错误时终止解析并保留当前已构建的堆栈，不 panic
func ExtractLayerInfos(packet gopacket.Packet, baseLevel LayerLevel) (currentLevel LayerLevel, stack LayerStack, payload []byte) {
	currentLevel = baseLevel
	nextLevel := currentLevel
	mainLayers := packet.Layers()
	stack = make(LayerStack, 0, 4)
	for _, layer := range mainLayers {
		decoder, ok := lookupLayerDecoder(layer.LayerType())
		if !ok {
			// 未注册协议层，记录到堆栈中并提取载荷数据
			currentLevel = nextLevel
			stack = append(stack, &LayerInfo{
				Level:   currentLevel,
				Proto:   layer.LayerType(),
				Headers: nil,
			})
			payload = layer.LayerPayload()
			return
		}
		result, err := decoder.Decode(DecodeContext{CurrentLevel: currentLevel, NextLevel: nextLevel}, layer)
		if err != nil {
			return
		}
		switch result.Kind {
		case DecodeRecurse:
			if result.Info != nil {
				stack = append(stack, result.Info)
			}
			innerLevel, innerInfos, innerPayload := ExtractLayerInfos(result.Inner, result.InnerBase)
			stack = append(stack, innerInfos...)
			payload = innerPayload
			currentLevel = innerLevel
			return
		case DecodeStop:
			if result.Info != nil {
				stack = append(stack, result.Info)
			}
			payload = result.Payload
			return
		default:
			if result.Info == nil {
				// 该层不进入层堆栈（如 TLS），但仍需按原语义推进下一层级
				nextLevel = currentLevel.Next()
				continue
			}
			stack = append(stack, result.Info)
			currentLevel = result.Info.Level
			nextLevel = currentLevel.Next()
		}
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
	LinkType      layers.LinkType         // pcap 链路类型，决定解析起始层级与链路槽位含义
	Level         LayerLevel              // 数据包的最高解析层级
	Stack         LayerStack              // 网络层堆栈信息
	Payload       []byte                  // 应用层载荷数据
}

// BuildAsPacketLinkModel 构建链路层数据包模型
// 将原始数据包转换为链路层（OSI第2层）的数据模型
//
// 返回值:
//   - any: 返回具体的链路层数据包模型，支持 PacketEthernet、PacketLinuxSLL、
//     PacketLinuxSLL2、PacketPPP、PacketDot11；无链路层或协议未注册装配器时返回 nil
//
// 功能说明:
//  1. 按层级查找层堆栈中的第一个链路层
//  2. 按层级与协议类型查找已注册的模型装配器
//  3. 由装配器提取链路层头部并构建包含完整元数据的模型
//  4. 返回可用于数据库存储的结构化数据
func (b *ModelBuilder) BuildAsPacketLinkModel() any {
	info := firstLayerOfLevel(b.Stack, LayerLevelLink)
	if info == nil || info.Headers == nil {
		return nil
	}
	assembler, ok := lookupModelAssembler(LayerLevelLink, info.Proto)
	if !ok {
		return nil
	}
	return assembler.Build(b)
}

// linkSlots 返回当前数据包的链路槽位
// 由链路层模型提供槽位映射；无链路层（Raw IP）或模型未实现 LinkSlotted 时返回空槽位
func (b *ModelBuilder) linkSlots() LinkSlots {
	if slotted, ok := b.BuildAsPacketLinkModel().(LinkSlotted); ok {
		return slotted.LinkSlotValues()
	}
	return LinkSlots{}
}

// BuildAsPacketNetworkModel 构建网络层数据包模型
// 将原始数据包转换为网络层（OSI第3层）的数据模型
// 支持IPv4和IPv6协议，构建包含链路槽位和网络层信息的完整数据包模型
//
// 返回值:
//   - any: 返回具体的网络层数据包模型，支持 PacketIPv4 或 PacketIPv6；
//     协议未注册装配器时返回 nil
//
// 异常:
//   - 如果层堆栈中不存在网络层，会触发panic
//   - 如果网络层头部信息为nil，会触发panic
//
// 功能说明:
//  1. 按层级查找层堆栈中的第一个网络层
//  2. 按层级与协议类型查找已注册的模型装配器
//  3. 由装配器组合链路槽位与网络层头部
//  4. 返回可用于数据库存储的结构化数据
func (b *ModelBuilder) BuildAsPacketNetworkModel() any {
	info := firstLayerOfLevel(b.Stack, LayerLevelNetwork)
	if info == nil {
		panic("network layer stack have not enough layers")
	}
	if info.Headers == nil {
		panic("network layer stack headers is nil")
	}
	assembler, ok := lookupModelAssembler(LayerLevelNetwork, info.Proto)
	if !ok {
		return nil
	}
	return assembler.Build(b)
}

// BuildAsPacketTransportModel 构建传输层数据包模型
// 将原始数据包转换为传输层（OSI第4层）的数据模型
// 支持TCP和UDP协议，构建包含链路槽位、网络层和传输层信息的完整数据包模型
//
// 返回值:
//   - any: 返回具体的传输层数据包模型，支持 PacketIPv4TCP、PacketIPv4UDP、PacketIPv6TCP 或 PacketIPv6UDP；
//     协议未注册装配器时返回 nil
//
// 异常:
//   - 如果层堆栈中不存在传输层，会触发panic
//
// 功能说明:
//  1. 按层级查找层堆栈中的最后一个传输层
//  2. 按层级与协议类型查找已注册的模型装配器
//  3. 由装配器复用网络层模型并组合传输层头部
//  4. 返回可用于数据库存储的结构化数据
func (b *ModelBuilder) BuildAsPacketTransportModel() any {
	info := lastLayerOfLevel(b.Stack, LayerLevelTransport)
	if info == nil {
		panic("transport layer stack have not enough layers")
	}
	if info.Headers == nil {
		return nil
	}
	assembler, ok := lookupModelAssembler(LayerLevelTransport, info.Proto)
	if !ok {
		return nil
	}
	return assembler.Build(b)
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

// NewPacketModelBuilderWithLinkType 创建带链路类型的数据包模型构建器
// 按 pcap 真实链路类型决定解析起始层级：Raw IP（Raw/IPv4/IPv6）从网络层开始，
// 其余链路类型从链路层开始；链路类型同时写入模型元数据。
//
// 参数:
//   - packet: gopacket.Packet 类型，原始网络数据包
//   - sessionID: serializer.HexUUID 类型，数据捕获会话的唯一标识
//   - source: reporter_v1.ResourceRaw 类型，数据源信息（命名空间、捕获器、版本）
//   - linkType: layers.LinkType 类型，pcap 文件真实链路类型
//
// 返回值:
//   - *ModelBuilder: 初始化完成的模型构建器实例
//
// 异常:
//   - 如果数据包元数据为nil，会触发panic
func NewPacketModelBuilderWithLinkType(packet gopacket.Packet, sessionID serializer.HexUUID, source reporter_v1.ResourceRaw, linkType layers.LinkType) *ModelBuilder {
	metadata := packet.Metadata()
	if metadata == nil {
		panic("packet metadata is nil")
	}
	level, stack, payload := ExtractLayerInfos(packet, startLayerLevel(linkType))
	return &ModelBuilder{
		Timestamp:     metadata.Timestamp,
		Truncated:     metadata.Truncated,
		CaptureLength: uint32(metadata.CaptureLength),
		SessionID:     sessionID,
		Source:        source,
		LinkType:      linkType,
		Level:         level,
		Stack:         stack,
		Payload:       payload,
	}
}

// startLayerLevel 根据链路类型确定解析起始层级
// Raw IP（Raw/IPv4/IPv6）数据包直接以网络层开始，其余链路类型从链路层开始。
func startLayerLevel(linkType layers.LinkType) LayerLevel {
	switch linkType {
	case layers.LinkTypeRaw, layers.LinkTypeIPv4, layers.LinkTypeIPv6:
		return LayerLevelNetwork
	default:
		return LayerLevelLink
	}
}
