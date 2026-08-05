package pcap

import (
	"context"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/pterm/pterm"
	huginn "zhongkui.com/zhongkui/internal/network/huginn"
	n "zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// TransFromPacketToModel 将数据包项转换为网络模型的数据转换函数。
// 从 PcapItem 中提取信息，使用 PacketModelBuilder 构建网络数据包模型。
// 对 TCP 握手包（SYN / SYN-ACK）额外计算被动指纹并回填到模型。
func TransFromPacketToModel(ctx context.Context, i any) (any, error) {
	item, ok := i.(*PcapItem)
	if !ok {
		return nil, errors.Newf("Expect pcap.PcapItem, got %T", i)
	}
	builder := n.NewPacketModelBuilder(item.Packet, item.SessionID, item.Source)
	model := builder.BuildAsPacketModel()
	if model == nil {
		// 调试：当模型构建失败时，打印层堆栈以定位原因
		// builder.Stack 和 builder.Level 由 ExtractLayerInfos 填充
		pterm.Debug.Printfln(
			"[nil model] level=%d stack_len=%d",
			builder.Level, len(builder.Stack),
		)
		for idx, info := range builder.Stack {
			pterm.Debug.Printfln(
				"[nil model]   stack[%d] level=%d proto=%s headers=%v",
				idx, info.Level, info.Proto, info.Headers != nil,
			)
		}
	}
	enrichTCPFingerprint(ctx, item.Packet, model)
	return model, nil
}

// enrichTCPFingerprint 对 TCP 数据包模型回填被动指纹（p0f 风格）。
// 仅 SYN / SYN-ACK 握手包包含足够的 TCP 选项与 IP 头特征用于识别，
// 识别结果记录到 PacketTCPFingerprint（写入 packet_ipv4_tcps / packet_ipv6_tcps 表），
// 供后续组包阶段透传至 HttpTransport 的 TcpFingerprint。
// 识别失败或 FFI 不可用时仅记录统计指标，不中断导入流水线。
func enrichTCPFingerprint(ctx context.Context, packet gopacket.Packet, model any) {
	if model == nil {
		return
	}
	netLayer := packet.NetworkLayer()
	transportLayer := packet.TransportLayer()
	tcp, ok := transportLayer.(*layers.TCP)
	if !ok || !tcp.SYN {
		// 仅握手包参与指纹识别；SYN 包（客户端）与 SYN-ACK 包（服务端）均可识别
		return
	}
	// 拼接网络层与传输层的原始头字节作为指纹分析输入
	netContents := netLayer.LayerContents()
	tcpContents := transportLayer.LayerContents()
	if len(netContents) == 0 || len(tcpContents) == 0 {
		return
	}
	raw := make([]byte, 0, len(netContents)+len(tcpContents))
	raw = append(raw, netContents...)
	raw = append(raw, tcpContents...)

	var fp *huginn.TCPFingerprint
	var err error
	switch netLayer.(type) {
	case *layers.IPv4:
		fp, err = huginn.AnalyzeTCPIPv4(raw)
	case *layers.IPv6:
		fp, err = huginn.AnalyzeTCPIPv6(raw)
	default:
		return
	}
	if err != nil {
		// FFI 不可用（未启用 huginn build tag）或解析失败：仅记录指标
		if stats := GetPcapStats(ctx); stats != nil {
			stats.RecordTCPFingerprintError()
		}
		return
	}
	if fp == nil {
		// 未匹配到已知签名（非标准握手包等），无指纹可回填
		return
	}
	if stats := GetPcapStats(ctx); stats != nil {
		stats.RecordTCPFingerprint()
	}
	modelFP := n.PacketTCPFingerprint{
		OS:        fp.OS,
		OSQuality: fp.OSQuality,
		Sig:       fp.Sig,
		MTU:       fp.MTU,
	}
	switch m := model.(type) {
	case *n.PacketIPv4TCP:
		m.PacketTCPFingerprint = modelFP
	case *n.PacketIPv6TCP:
		m.PacketTCPFingerprint = modelFP
	}
}
