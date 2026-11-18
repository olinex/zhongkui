package network

import (
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// decodeOptions 递归解析内层数据包时复用的解码选项，避免复制载荷并按需惰性解析
var decodeOptions = gopacket.DecodeOptions{NoCopy: true, Lazy: true}

// decodeEthernet 解析以太网链路层，层信息取下一层级
func decodeEthernet(ctx DecodeContext, eth *layers.Ethernet) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level:   ctx.NextLevel,
			Proto:   eth.LayerType(),
			Headers: NewEthernetHeaders(eth),
		},
		Kind: DecodeContinue,
	}, nil
}

// decodeLoopback 解析回环链路层
// Loopback（pcap DLT_NULL/DLT_LOOP）不包含 MAC 地址，内层数据包从网络层开始解析
func decodeLoopback(ctx DecodeContext, loop *layers.Loopback) (DecodeResult, error) {
	return DecodeResult{
		Info: &LayerInfo{
			Level:   ctx.NextLevel,
			Proto:   loop.LayerType(),
			Headers: NewLoopbackHeaders(),
		},
		Kind:      DecodeRecurse,
		Inner:     gopacket.NewPacket(loop.LayerPayload(), loop.NextLayerType(), decodeOptions),
		InnerBase: LayerLevelNetwork,
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

// init 注册内置协议层的解码器
func init() {
	RegisterTypedDecoder(decodeEthernet)
	RegisterTypedDecoder(decodeLoopback)
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
