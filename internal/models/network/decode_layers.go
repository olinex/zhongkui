package network

import (
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// llcSnapHeaderLength 是 LLC/SNAP 头部长度：3 字节 LLC + 3 字节 OUI + 2 字节以太网类型
const llcSnapHeaderLength = 8

// decodeOptions 递归解析内层数据包时复用的解码选项，避免复制载荷并按需惰性解析
var decodeOptions = gopacket.DecodeOptions{NoCopy: true, Lazy: true}

// decodeEthernet 解析以太网链路层，层信息固定在链路层
// 链路层之后可继续承载 VLAN、PPPoE 等链路子层，因此显式使用 LayerLevelLink，
// 避免后续子层被逐层推进到网络层或传输层。
func decodeEthernet(_ DecodeContext, eth *layers.Ethernet) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level:   LayerLevelLink,
			Proto:   eth.LayerType(),
			Headers: NewEthernetHeaders(eth),
		},
		Kind: DecodeContinue,
	}, nil
}

// decodeLoopback 解析回环链路层
// Loopback（pcap DLT_NULL/DLT_LOOP）不包含 MAC 地址，内层数据包从网络层开始解析
func decodeLoopback(_ DecodeContext, loop *layers.Loopback) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level:   LayerLevelLink,
			Proto:   loop.LayerType(),
			Headers: NewLoopbackHeaders(),
		},
		Kind:      DecodeRecurse,
		Inner:     gopacket.NewPacket(loop.LayerPayload(), loop.NextLayerType(), decodeOptions),
		InnerBase: LayerLevelNetwork,
	}, nil
}

// decodeLinuxSLL 解析 Linux SLL 链路层，内层协议由 gopacket 按以太网类型继续解码
func decodeLinuxSLL(_ DecodeContext, sll *layers.LinuxSLL) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level:   LayerLevelLink,
			Proto:   sll.LayerType(),
			Headers: NewLinuxSLLHeaders(sll),
		},
		Kind: DecodeContinue,
	}, nil
}

// decodeLinuxSLL2 解析 Linux SLL2 链路层，内层协议由 gopacket 按协议类型继续解码
func decodeLinuxSLL2(_ DecodeContext, sll2 *layers.LinuxSLL2) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level:   LayerLevelLink,
			Proto:   sll2.LayerType(),
			Headers: NewLinuxSLL2Headers(sll2),
		},
		Kind: DecodeContinue,
	}, nil
}

// decodePPP 解析 PPP 链路层，内层协议由 gopacket 按 PPP 协议类型继续解码
// PPP 既可能是 pcap 首层，也可能位于 PPPoE 之后，层级始终为链路层
func decodePPP(_ DecodeContext, ppp *layers.PPP) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level:   LayerLevelLink,
			Proto:   ppp.LayerType(),
			Headers: NewPPPHeaders(ppp),
		},
		Kind: DecodeContinue,
	}, nil
}

// decodeDot1Q 解析 802.1Q / QinQ VLAN 标签
// VLAN 标签属于链路层子层，内层协议由 gopacket 按标签类型继续解码
func decodeDot1Q(_ DecodeContext, dot1q *layers.Dot1Q) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level:   LayerLevelLink,
			Proto:   dot1q.LayerType(),
			Headers: NewDot1QHeaders(dot1q),
		},
		Kind: DecodeContinue,
	}, nil
}

// decodePPPoE 解析 PPPoE 链路层封装
// 会话报文继续解析内层 PPP；发现阶段报文没有 PPP 载荷，停留在链路层并保留剩余字节
func decodePPPoE(_ DecodeContext, pppoe *layers.PPPoE) (DecodeResult, error) {
	info := &LayerInfo{
		Level:   LayerLevelLink,
		Proto:   pppoe.LayerType(),
		Headers: NewPPPoEHeaders(pppoe),
	}
	if pppoe.Code != layers.PPPoECodeSession {
		return DecodeResult{
			Info:    info,
			Kind:    DecodeStop,
			Payload: pppoe.Payload,
		}, nil
	}
	return DecodeResult{
		Info: info,
		Kind: DecodeContinue,
	}, nil
}

// decodeRadioTap 解析 RadioTap 无线抓包元数据层
func decodeRadioTap(_ DecodeContext, radioTap *layers.RadioTap) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level:   LayerLevelLink,
			Proto:   radioTap.LayerType(),
			Headers: NewRadioTapHeaders(radioTap),
		},
		Kind: DecodeContinue,
	}, nil
}

// decodePrism 解析 Prism 无线抓包元数据层
func decodePrism(_ DecodeContext, prism *layers.PrismHeader) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level:   LayerLevelLink,
			Proto:   prism.LayerType(),
			Headers: NewPrismHeaders(prism),
		},
		Kind: DecodeContinue,
	}, nil
}

// decodeDot11 解析 IEEE 802.11 链路层
// 数据帧继续解析 Dot11Data、LLC/SNAP 与内层协议；
// 管理帧、控制帧与加密帧没有可继续解析的明文载荷，停留在链路层
func decodeDot11(_ DecodeContext, dot11 *layers.Dot11) (DecodeResult, error) {
	info := &LayerInfo{
		Level:   LayerLevelLink,
		Proto:   dot11.LayerType(),
		Headers: NewDot11Headers(dot11),
	}
	if dot11.Flags.WEP() {
		// Protected 位可能仍然保留，但载荷已由抓包工具或网卡驱动解密；
		// 此时绕过 gopacket 的 Dot11WEP 层，直接按 LLC 继续解析
		if isDecryptedDot11Payload(dot11.Payload) {
			return DecodeResult{
				Info:      info,
				Kind:      DecodeRecurse,
				Inner:     gopacket.NewPacket(dot11.Payload, layers.LayerTypeLLC, decodeOptions),
				InnerBase: LayerLevelLink,
			}, nil
		}
		return DecodeResult{
			Info:    info,
			Kind:    DecodeStop,
			Payload: dot11.Payload,
		}, nil
	}
	if dot11.DataLayer == nil {
		return DecodeResult{
			Info:    info,
			Kind:    DecodeStop,
			Payload: dot11.Payload,
		}, nil
	}
	return DecodeResult{
		Info: info,
		Kind: DecodeContinue,
	}, nil
}

// isDecryptedDot11Payload 判断 Protected 位置位的 802.11 帧载荷是否已是明文
// 已解密的载荷以 LLC/SNAP 开头（DSAP=SSAP=0xAA、Control=0x03、OUI=0x000000），
// 随机密文命中该 6 字节前缀的概率为 2^-48，可安全用于识别「已解密但 Protected 位未清」的帧
func isDecryptedDot11Payload(payload []byte) bool {
	if len(payload) < llcSnapHeaderLength {
		return false
	}
	return payload[0] == 0xAA && payload[1] == 0xAA && payload[2] == 0x03 &&
		payload[3] == 0x00 && payload[4] == 0x00 && payload[5] == 0x00
}

// decodeDot11DataLayer 解析 802.11 数据帧变体（含 CF / QoS 系列）
// 数据帧变体本身不携带额外头部字段，仅作为链路层标记进入层堆栈并继续解析 LLC
func decodeDot11DataLayer(_ DecodeContext, layer gopacket.Layer) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level: LayerLevelLink,
			Proto: layer.LayerType(),
		},
		Kind: DecodeContinue,
	}, nil
}

// decodeLLC 解析 IEEE 802.2 LLC 链路层子层
func decodeLLC(_ DecodeContext, llc *layers.LLC) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level:   LayerLevelLink,
			Proto:   llc.LayerType(),
			Headers: NewLLCHeaders(llc),
		},
		Kind: DecodeContinue,
	}, nil
}

// decodeSNAP 解析 LLC/SNAP 扩展头部
func decodeSNAP(_ DecodeContext, snap *layers.SNAP) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level:   LayerLevelLink,
			Proto:   snap.LayerType(),
			Headers: NewSNAPHeaders(snap),
		},
		Kind: DecodeContinue,
	}, nil
}

// decodeIPv4 解析 IPv4 网络层，IP-in-IP 封装递归解析内层 IPv4
func decodeIPv4(ctx DecodeContext, ip *layers.IPv4) (DecodeResult, error) {
	result := DecodeResult{
		Info: &LayerInfo{
			Level:   ctx.NextLevel,
			Proto:   ip.LayerType(),
			Headers: NewIPv4Headers(ip),
		},
		Kind: DecodeContinue,
	}
	if ip.Protocol == layers.IPProtocolIPIP {
		result.Kind = DecodeRecurse
		result.Inner = gopacket.NewPacket(ip.LayerPayload(), layers.LayerTypeIPv4, decodeOptions)
		result.InnerBase = ctx.NextLevel
	}
	return result, nil
}

// decodeIPv6 解析 IPv6 网络层
func decodeIPv6(ctx DecodeContext, ip *layers.IPv6) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level:   ctx.NextLevel,
			Proto:   ip.LayerType(),
			Headers: NewIPv6Headers(ip),
		},
		Kind: DecodeContinue,
	}, nil
}

// decodeIPv6Destination 解析 IPv6 目标选项扩展头，扩展头不推进层级
func decodeIPv6Destination(ctx DecodeContext, destination *layers.IPv6Destination) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level:   ctx.CurrentLevel,
			Proto:   destination.LayerType(),
			Headers: NewIPv6Destination(destination),
		},
		Kind: DecodeContinue,
	}, nil
}

// decodeIPv6Fragment 解析 IPv6 分片扩展头，扩展头不推进层级
func decodeIPv6Fragment(ctx DecodeContext, fragment *layers.IPv6Fragment) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level:   ctx.CurrentLevel,
			Proto:   fragment.LayerType(),
			Headers: NewIPv6Fragment(fragment),
		},
		Kind: DecodeContinue,
	}, nil
}

// decodeIPv6HopByHop 解析 IPv6 逐跳选项扩展头，扩展头不推进层级
func decodeIPv6HopByHop(ctx DecodeContext, hopByHop *layers.IPv6HopByHop) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level:   ctx.CurrentLevel,
			Proto:   hopByHop.LayerType(),
			Headers: NewIPv6HopByHop(hopByHop),
		},
		Kind: DecodeContinue,
	}, nil
}

// decodeIPv6Routing 解析 IPv6 路由扩展头，扩展头不推进层级
func decodeIPv6Routing(ctx DecodeContext, routing *layers.IPv6Routing) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level:   ctx.CurrentLevel,
			Proto:   routing.LayerType(),
			Headers: NewIPv6Routing(routing),
		},
		Kind: DecodeContinue,
	}, nil
}

// decodeTCP 解析 TCP 传输层
func decodeTCP(ctx DecodeContext, tcp *layers.TCP) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level:   ctx.NextLevel,
			Proto:   tcp.LayerType(),
			Headers: NewTCPHeaders(tcp),
		},
		Kind: DecodeContinue,
	}, nil
}

// decodeUDP 解析 UDP 传输层
func decodeUDP(ctx DecodeContext, udp *layers.UDP) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level:   ctx.NextLevel,
			Proto:   udp.LayerType(),
			Headers: NewUDPHeaders(udp),
		},
		Kind: DecodeContinue,
	}, nil
}

// decodeTLS 跳过 TLS 层：暂未实现详细解析，不进入层堆栈并继续解析后续层
func decodeTLS(_ DecodeContext, _ *layers.TLS) (DecodeResult, error) {
	return DecodeResult{Kind: DecodeContinue}, nil
}

// decodeVXLAN 解析 VXLAN 隧道，内层为完整以太网帧，从链路层开始递归解析
func decodeVXLAN(ctx DecodeContext, vxlan *layers.VXLAN) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level:   ctx.NextLevel,
			Proto:   vxlan.LayerType(),
			Headers: NewVXLANChannel(vxlan),
		},
		Kind:      DecodeRecurse,
		Inner:     gopacket.NewPacket(vxlan.LayerPayload(), layers.LayerTypeEthernet, decodeOptions),
		InnerBase: LayerLevelLink,
	}, nil
}

// decodeGRE 解析 GRE 隧道，内层起始层级由封装协议决定
func decodeGRE(ctx DecodeContext, gre *layers.GRE) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level:   ctx.NextLevel,
			Proto:   gre.LayerType(),
			Headers: NewGREChannel(gre),
		},
		Kind:      DecodeRecurse,
		Inner:     gopacket.NewPacket(gre.LayerPayload(), gre.Protocol, decodeOptions),
		InnerBase: getTunnelInnerBaseLevel(gre.Protocol),
	}, nil
}

// decodeGeneve 解析 Geneve 隧道，内层起始层级由封装协议决定
func decodeGeneve(ctx DecodeContext, geneve *layers.Geneve) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level:   ctx.NextLevel,
			Proto:   geneve.LayerType(),
			Headers: NewGeneveChannel(geneve),
		},
		Kind:      DecodeRecurse,
		Inner:     gopacket.NewPacket(geneve.LayerPayload(), geneve.Protocol, decodeOptions),
		InnerBase: getTunnelInnerBaseLevel(geneve.Protocol),
	}, nil
}

// decodeMPLS 解析 MPLS 标签栈，内层从当前标签层级的下一层级开始递归解析
func decodeMPLS(ctx DecodeContext, mpls *layers.MPLS) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level:   ctx.NextLevel,
			Proto:   mpls.LayerType(),
			Headers: NewMPLSChannel(mpls),
		},
		Kind:      DecodeRecurse,
		Inner:     gopacket.NewPacket(mpls.LayerPayload(), layers.LayerTypeMPLS, decodeOptions),
		InnerBase: ctx.NextLevel.Next(),
	}, nil
}

// decodePayload 到达载荷层，终止解析并返回原始载荷
func decodePayload(_ DecodeContext, payload *gopacket.Payload) (DecodeResult, error) {
	return DecodeResult{
		Kind:    DecodeStop,
		Payload: payload.Payload(),
	}, nil
}

// decodeDecodeFailure 解码失败，记录失败层并终止解析
func decodeDecodeFailure(ctx DecodeContext, failure *gopacket.DecodeFailure) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level:   ctx.NextLevel,
			Proto:   failure.LayerType(),
			Headers: nil,
		},
		Kind:    DecodeStop,
		Payload: failure.LayerContents(),
	}, nil
}

// dot11DataLayerTypes 是 802.11 数据帧的全部具体层类型
// gopacket 会按帧控制字段选择 CF / QoS 等变体层，这里统一注册为链路层数据帧
var dot11DataLayerTypes = []gopacket.LayerType{
	layers.LayerTypeDot11Data,
	layers.LayerTypeDot11DataCFAck,
	layers.LayerTypeDot11DataCFPoll,
	layers.LayerTypeDot11DataCFAckPoll,
	layers.LayerTypeDot11DataNull,
	layers.LayerTypeDot11DataCFAckNoData,
	layers.LayerTypeDot11DataCFPollNoData,
	layers.LayerTypeDot11DataCFAckPollNoData,
	layers.LayerTypeDot11DataQOSData,
	layers.LayerTypeDot11DataQOSDataCFAck,
	layers.LayerTypeDot11DataQOSDataCFPoll,
	layers.LayerTypeDot11DataQOSDataCFAckPoll,
	layers.LayerTypeDot11DataQOSNull,
	layers.LayerTypeDot11DataQOSCFPollNoData,
	layers.LayerTypeDot11DataQOSCFAckPollNoData,
}

// init 注册内置协议层的解码器
func init() {
	RegisterTypedDecoder(decodeEthernet)
	RegisterTypedDecoder(decodeLoopback)
	RegisterTypedDecoder(decodeLinuxSLL)
	RegisterTypedDecoder(decodeLinuxSLL2)
	RegisterTypedDecoder(decodePPP)
	RegisterTypedDecoder(decodeDot1Q)
	RegisterTypedDecoder(decodePPPoE)
	RegisterTypedDecoder(decodeRadioTap)
	RegisterTypedDecoder(decodePrism)
	RegisterTypedDecoder(decodeDot11)
	RegisterTypedFamilyLayerDecoders(dot11DataLayerTypes, decodeDot11DataLayer)
	RegisterTypedDecoder(decodeLLC)
	RegisterTypedDecoder(decodeSNAP)
	RegisterTypedDecoder(decodeIPv4)
	RegisterTypedDecoder(decodeIPv6)
	RegisterTypedDecoder(decodeIPv6Destination)
	RegisterTypedDecoder(decodeIPv6Fragment)
	RegisterTypedDecoder(decodeIPv6HopByHop)
	RegisterTypedDecoder(decodeIPv6Routing)
	RegisterTypedDecoder(decodeTCP)
	RegisterTypedDecoder(decodeUDP)
	RegisterTypedDecoder(decodeTLS)
	RegisterTypedDecoder(decodeVXLAN)
	RegisterTypedDecoder(decodeGRE)
	RegisterTypedDecoder(decodeGeneve)
	RegisterTypedDecoder(decodeMPLS)
	RegisterTypedDecoder(decodePayload)
	RegisterTypedDecoder(decodeDecodeFailure)
}
