// TCP 观测值提取：huginn-net-tcp 的 process/flow、tcp/ttl、tcp/window_size、
// tcp/ip_options 与 mtu/extractor 模块的纯 Go 移植。
//
// 输入为完整的 IP 数据包（IPv4 或 IPv6）+ TCP 头字节，输出为 p0f 风格的
// TCP 观测结构（版本、TTL、选项、窗口、怪癖、载荷分类）与可选 MTU。
package huginn

import (
	"encoding/binary"

	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// TCP 标志位（对应 pnet 的 TcpFlags）。
const (
	tcpFlagFIN uint8 = 0x01
	tcpFlagSYN uint8 = 0x02
	tcpFlagRST uint8 = 0x04
	tcpFlagPSH uint8 = 0x08
	tcpFlagACK uint8 = 0x10
	tcpFlagURG uint8 = 0x20
	tcpFlagECE uint8 = 0x40
	tcpFlagCWR uint8 = 0x80
)

// TCP 选项号（对应 pnet 的 TcpOptionNumbers）。
const (
	tcpOptEOL  uint8 = 0
	tcpOptNOP  uint8 = 1
	tcpOptMSS  uint8 = 2
	tcpOptWS   uint8 = 3
	tcpOptSOK  uint8 = 4
	tcpOptSACK uint8 = 5
	tcpOptTS   uint8 = 8
)

// IP 协议号（对应 pnet 的 IpNextHeaderProtocols）。
const (
	ipProtoTCP      uint8 = 6
	ipProtoIPv6Frag uint8 = 44
)

// IP 头中的 ECN 位（对应 flow.rs 的 IP_TOS_CE / IP_TOS_ECT）。
const (
	ipTosCE  uint8 = 0x01
	ipTosECT uint8 = 0x02
)

// maxHopsAcceptable TTL 距离可接受的最大跳数（对应 ttl.rs 的 MAX_HOPS_ACCEPTABLE）。
const maxHopsAcceptable uint8 = 30

// 分析失败的错误哨兵（对应 HuginnNetTcpError 的各变体；FFI 层统一映射为 NULL）。
var (
	// ErrPacketTooShort 数据包长度不足以容纳完整头。
	ErrPacketTooShort = errors.New("huginn: packet too short")
	// ErrNotIPVersion 数据包版本号非法（非 4 / 非 6）。
	ErrNotIPVersion = errors.New("huginn: invalid IP version")
	// ErrUnsupportedProtocol 上层协议不是 TCP。
	ErrUnsupportedProtocol = errors.New("huginn: unsupported protocol")
	// ErrUnexpectedPacket 分片或其它无法处理的异常数据包。
	ErrUnexpectedPacket = errors.New("huginn: unexpected packet (fragmented)")
	// ErrInvalidTCPFlags TCP 标志组合非法。
	ErrInvalidTCPFlags = errors.New("huginn: invalid TCP flags")
)

// tcpObservedPackage 一次 TCP 分析的结果：客户端（SYN）/ 服务端（SYN-ACK）观测
// 与可选 MTU（仅客户端 SYN）。
// 对应 huginn-net-tcp 的 ObservableTCPPackage。
type tcpObservedPackage struct {
	tcpRequest  *tcpObservation
	tcpResponse *tcpObservation
	mtu         *uint16
}

// tcpData 从原始字节解析出的 TCP 头字段。
type tcpData struct {
	source      uint16
	destination uint16
	sequence    uint32
	ack         uint32
	dataOffset  uint8 // 32 位字单位
	flags       uint8
	window      uint16
	urgentPtr   uint16
	options     []byte // 选项区原始字节（EOL 之后仍可能含填充）
	payload     []byte // TCP 载荷（选项区之后）
}

// parseTCPPacket 解析 TCP 头（至少 20 字节；对应 pnet TcpPacket::new）。
func parseTCPPacket(raw []byte) (*tcpData, error) {
	if len(raw) < 20 {
		return nil, ErrPacketTooShort
	}
	t := &tcpData{
		source:      binary.BigEndian.Uint16(raw[0:2]),
		destination: binary.BigEndian.Uint16(raw[2:4]),
		sequence:    binary.BigEndian.Uint32(raw[4:8]),
		ack:         binary.BigEndian.Uint32(raw[8:12]),
		dataOffset:  raw[12] >> 4,
		flags:       raw[13],
		window:      binary.BigEndian.Uint16(raw[14:16]),
		urgentPtr:   binary.BigEndian.Uint16(raw[18:20]),
	}
	if t.dataOffset < 5 {
		return nil, ErrUnexpectedPacket
	}
	headerLen := int(t.dataOffset) * 4
	if headerLen > len(raw) {
		headerLen = len(raw)
	}
	// 对应 pnet get_options_raw：data_offset > 5 时取 data_offset*4-20 字节，否则为空
	if t.dataOffset > 5 {
		t.options = raw[20:headerLen]
	}
	t.payload = raw[headerLen:]
	return t, nil
}

// fromClient 判断是否为客户端方向（SYN 置位且未置 ACK；对应 flow.rs from_client）。
func fromClient(flags uint8) bool {
	return flags&tcpFlagSYN != 0 && flags&tcpFlagACK == 0
}

// isTcpValid 校验 TCP 标志组合（对应 flow.rs is_valid）。
// 拒绝：SYN 与 FIN/RST 同时置位、FIN 与 RST 同时置位、或 type 为 0。
func isTcpValid(flags, tcpType uint8) bool {
	return !((flags&tcpFlagSYN == tcpFlagSYN && flags&(tcpFlagFIN|tcpFlagRST) != 0) ||
		flags&(tcpFlagFIN|tcpFlagRST) == tcpFlagFIN|tcpFlagRST ||
		tcpType == 0)
}

// guessDistance 根据观测 TTL 猜测初始 TTL 与观测值之差（对应 ttl.rs guess_distance）。
func guessDistance(ttl uint8) uint8 {
	switch {
	case ttl > 128:
		return 255 - ttl
	case ttl > 64:
		return 128 - ttl
	case ttl > 32:
		return 64 - ttl
	default:
		return 32 - ttl
	}
}

// calculateTTL 由观测 TTL 计算指纹 TTL 表示（对应 ttl.rs calculate_ttl）。
// 0 → Bad；距离不超过 MAX_HOPS_ACCEPTABLE → Distance；否则保留原始值 Value。
func calculateTTL(ttlObserved uint8) ttl {
	if ttlObserved == 0 {
		return ttl{kind: ttlBad, first: ttlObserved}
	}
	distance := guessDistance(ttlObserved)
	if distance <= maxHopsAcceptable {
		return ttl{kind: ttlDistance, first: ttlObserved, second: distance}
	}
	return ttl{kind: ttlValue, first: ttlObserved}
}

// detectWinMultiplicator 识别窗口大小的倍数模式（对应 tcp/window_size.rs）。
// 依次检查：MSS 倍数（含时间戳调整）、常见模数、MTU 倍数、mss+头长，最后兜底原始值。
func detectWinMultiplicator(observed, mss, totalHeader uint16, hasTS bool, ver ipVersion) windowSize {
	const (
		minTCP4       uint16 = 40 // 20 IP + 20 TCP
		minTCP6       uint16 = 60 // 40 IP + 20 TCP
		ethMTU        uint16 = 1500
		tsSize        uint16 = 12
		maxMultiplier uint16 = 255
	)

	if observed == 0 || mss < 100 {
		return windowSize{kind: winValue, value: observed}
	}

	checkDiv := func(div uint16, kind windowSizeKind) (windowSize, bool) {
		if div != 0 && observed%div == 0 {
			mult := observed / div
			if mult <= maxMultiplier {
				return windowSize{kind: kind, value: mult}, true
			}
		}
		return windowSize{}, false
	}

	// 1. MSS 倍数
	if mss > 0 {
		if w, ok := checkDiv(mss, winMss); ok {
			return w
		}
		if hasTS && mss > tsSize {
			if w, ok := checkDiv(mss-tsSize, winMss); ok {
				return w
			}
		}
	}

	// 2. 常见模数（逆序，取最大可整除模数）
	modulos := [...]uint16{256, 512, 1024, 2048, 4096}
	for i := len(modulos) - 1; i >= 0; i-- {
		if observed%modulos[i] == 0 {
			return windowSize{kind: winMod, value: modulos[i]}
		}
	}

	// 3. MTU 倍数
	if w, ok := checkDiv(ethMTU, winMtu); ok {
		return w
	}
	switch ver {
	case ipV4:
		if w, ok := checkDiv(ethMTU-minTCP4, winMtu); ok {
			return w
		}
		if hasTS {
			if w, ok := checkDiv(ethMTU-minTCP4-tsSize, winMtu); ok {
				return w
			}
		}
	case ipV6:
		if w, ok := checkDiv(ethMTU-minTCP6, winMtu); ok {
			return w
		}
		if hasTS {
			if w, ok := checkDiv(ethMTU-minTCP6-tsSize, winMtu); ok {
				return w
			}
		}
	}

	// 4. mss + 头长特例
	if mss > 0 {
		if totalHeader > 0 {
			if w, ok := checkDiv(mss+totalHeader, winMtu); ok {
				return w
			}
		} else {
			switch ver {
			case ipV4:
				if w, ok := checkDiv(mss+minTCP4, winMtu); ok {
					return w
				}
			case ipV6:
				if w, ok := checkDiv(mss+minTCP6, winMtu); ok {
					return w
				}
			}
		}
	}

	return windowSize{kind: winValue, value: observed}
}

// extractMTU 由客户端 SYN 的 MSS 反推链路 MTU（对应 mtu/extractor.rs）。
// ipHeaderLen：IPv4 为 IHL（字，内部乘 4），IPv6 为固定 40 字节。
func extractMTU(t *tcpData, ver ipVersion, ipHeaderLen uint8, mss uint16) *uint16 {
	// 提取器仅要求 SYN 置位（对应 extractor.rs from_client）；是否保留由 visitTCP 决定
	if t.flags&tcpFlagSYN != tcpFlagSYN {
		return nil
	}
	ipHeaderLenBytes := uint16(ipHeaderLen)
	if ver == ipV4 {
		ipHeaderLenBytes *= 4
	}
	tcpHeaderLen := uint16(t.dataOffset) * 4
	if tcpHeaderLen > 20 {
		tcpHeaderLen -= 20
	}
	mtu := mss + ipHeaderLenBytes + tcpHeaderLen
	return &mtu
}

// calculateIPv4Olen 计算 IPv4 选项区长度（对应 IpOptions::calculate_ipv4_length）。
func calculateIPv4Olen(ihl uint8) uint8 {
	if ihl > 5 {
		return (ihl - 5) * 4
	}
	return 0
}

// calculateIPv6Olen 计算 IPv6 扩展头长度（对应 IpOptions::calculate_ipv6_length）。
// next_header 直接为 TCP → 0；分片头 → 8；其它按扩展头长度字段推算。
func calculateIPv6Olen(raw []byte) uint8 {
	nextHeader := raw[6]
	if nextHeader == ipProtoTCP {
		return 0
	}
	payload := raw[40:]
	if len(payload) == 0 {
		return 0
	}
	switch nextHeader {
	case ipProtoIPv6Frag:
		return 8
	default:
		if len(payload) >= 2 {
			return uint8((uint16(payload[1]) + 1) * 8)
		}
		return 0
	}
}

// analyzeTCPIPv4 分析完整 IPv4 数据包（对应 process_tcp_ipv4）。
func analyzeTCPIPv4(raw []byte) (*tcpObservedPackage, error) {
	if len(raw) < 20 {
		return nil, ErrPacketTooShort
	}
	if raw[0]>>4 != 4 {
		return nil, ErrNotIPVersion
	}
	if raw[9] != ipProtoTCP {
		return nil, ErrUnsupportedProtocol
	}

	fragmentOffset := (uint16(raw[6]&0x1F) << 8) | uint16(raw[7])
	if fragmentOffset > 0 || raw[6]&0x20 != 0 { // 分片偏移非零或 MF 置位
		return nil, ErrUnexpectedPacket
	}

	ihl := raw[0] & 0x0F
	headerLen := int(ihl) * 4
	if headerLen > len(raw) {
		return nil, ErrPacketTooShort
	}

	ittl := calculateTTL(raw[8])
	olen := calculateIPv4Olen(ihl)

	quirks := make([]quirk, 0, 8)
	ecn := raw[1] & 0x03
	if ecn&(ipTosCE|ipTosECT) != 0 {
		quirks = append(quirks, quirkEcn)
	}
	if raw[6]&0x80 != 0 { // "必须为零"保留位（对应 IP4_MBZ）
		quirks = append(quirks, quirkMustBeZero)
	}
	identification := binary.BigEndian.Uint16(raw[4:6])
	if raw[6]&0x40 != 0 { // DF 置位
		quirks = append(quirks, quirkDf)
		if identification != 0 {
			quirks = append(quirks, quirkNonZeroID)
		}
	} else if identification == 0 {
		quirks = append(quirks, quirkZeroID)
	}

	tcp, err := parseTCPPacket(raw[headerLen:])
	if err != nil {
		return nil, err
	}
	return visitTCP(tcp, ipV4, ittl, ihl, olen, quirks)
}

// analyzeTCPIPv6 分析完整 IPv6 数据包（对应 process_tcp_ipv6）。
func analyzeTCPIPv6(raw []byte) (*tcpObservedPackage, error) {
	if len(raw) < 40 {
		return nil, ErrPacketTooShort
	}
	if raw[0]>>4 != 6 {
		return nil, ErrNotIPVersion
	}
	if raw[6] != ipProtoTCP {
		return nil, ErrUnsupportedProtocol
	}

	ittl := calculateTTL(raw[7])
	olen := calculateIPv6Olen(raw)

	quirks := make([]quirk, 0, 8)
	trafficClass := (uint16(raw[0]&0x0F) << 4) | uint16(raw[1]>>4)
	flowLabel := (uint32(raw[1]&0x0F) << 16) | (uint32(raw[2]) << 8) | uint32(raw[3])
	if flowLabel != 0 {
		quirks = append(quirks, quirkFlowID)
	}
	if trafficClass&(uint16(ipTosCE)|uint16(ipTosECT)) != 0 {
		quirks = append(quirks, quirkEcn)
	}

	tcp, err := parseTCPPacket(raw[40:])
	if err != nil {
		return nil, err
	}
	return visitTCP(tcp, ipV6, ittl, 40, olen, quirks)
}

// visitTCP 由解析出的 TCP 头组装观测结果（对应 visit_tcp）。
// ipHeaderLen：IPv4 为 IHL 字，IPv6 为 40。
func visitTCP(t *tcpData, ver ipVersion, ittl ttl, ipHeaderLen uint8, olen uint8, ipQuirks []quirk) (*tcpObservedPackage, error) {
	flags := t.flags
	client := fromClient(flags)
	tcpType := flags & (tcpFlagSYN | tcpFlagACK | tcpFlagFIN | tcpFlagRST)
	if !isTcpValid(flags, tcpType) {
		return nil, ErrInvalidTCPFlags
	}

	quirks := make([]quirk, len(ipQuirks), len(ipQuirks)+8)
	copy(quirks, ipQuirks)

	if flags&(tcpFlagECE|tcpFlagCWR) != 0 {
		quirks = append(quirks, quirkEcn)
	}
	if t.sequence == 0 {
		quirks = append(quirks, quirkSeqNumZero)
	}
	if flags&tcpFlagACK == tcpFlagACK {
		if t.ack == 0 {
			quirks = append(quirks, quirkAckNumZero)
		}
	} else if t.ack != 0 && flags&tcpFlagRST == 0 {
		quirks = append(quirks, quirkAckNumNonZero)
	}
	if flags&tcpFlagURG == tcpFlagURG {
		quirks = append(quirks, quirkUrg)
	} else if t.urgentPtr != 0 {
		quirks = append(quirks, quirkNonZeroURG)
	}
	if flags&tcpFlagPSH == tcpFlagPSH {
		quirks = append(quirks, quirkPush)
	}

	// 解析 TCP 选项（复刻 pnet TcpOptionPacket 语义）
	var (
		mss     *uint16
		wscale  *uint8
		hasTS   bool
		olayout []tcpOption
	)
	buf := t.options
	for len(buf) > 0 {
		kind := buf[0]
		packetSize := 1
		if kind != tcpOptEOL && kind != tcpOptNOP {
			packetSize = 2
			if len(buf) >= 2 && buf[1] >= 2 {
				packetSize = int(buf[1])
			}
		}

		switch kind {
		case tcpOptEOL:
			// 选项结束后剩余字节数记为 Eol(n)，且存在非零填充时记为 opt+ 怪癖
			olayout = append(olayout, tcpOption{kind: optEol, num: uint8(len(buf) - 1)})
			for _, b := range buf[1:] {
				if b != 0 {
					quirks = append(quirks, quirkTrailingNonZero)
					break
				}
			}
		case tcpOptNOP:
			olayout = append(olayout, tcpOption{kind: optNop})
		case tcpOptMSS:
			olayout = append(olayout, tcpOption{kind: optMss})
			if len(buf) >= 4 {
				v := binary.BigEndian.Uint16(buf[2:4])
				mss = &v
			}
		case tcpOptWS:
			olayout = append(olayout, tcpOption{kind: optWs})
			if len(buf) >= 3 {
				v := buf[2]
				wscale = &v
				if v > 14 {
					quirks = append(quirks, quirkExcessiveWindowScaling)
				}
			}
		case tcpOptSOK:
			olayout = append(olayout, tcpOption{kind: optSok})
		case tcpOptSACK:
			olayout = append(olayout, tcpOption{kind: optSack})
		case tcpOptTS:
			olayout = append(olayout, tcpOption{kind: optTS})
			hasTS = true
			if len(buf) >= 6 {
				ts1 := binary.BigEndian.Uint32(buf[2:6])
				if ts1 == 0 {
					quirks = append(quirks, quirkOwnTimestampZero)
				}
			}
			if len(buf) >= 10 && tcpType == tcpFlagSYN {
				ts2 := binary.BigEndian.Uint32(buf[6:10])
				if ts2 != 0 {
					quirks = append(quirks, quirkPeerTimestampNonZero)
				}
			}
		default:
			olayout = append(olayout, tcpOption{kind: optUnknown, num: kind})
		}

		if packetSize > len(buf) {
			packetSize = len(buf)
		}
		buf = buf[packetSize:]
		if kind == tcpOptEOL {
			break
		}
	}

	// MTU：仅客户端 SYN 输出（对应 ObservableTCPPackage.mtu 的 from_client 过滤）
	var mtu *uint16
	if mss != nil {
		mtu = extractMTU(t, ver, ipHeaderLen, *mss)
	}
	if !client {
		mtu = nil
	}

	wsize := detectWinMultiplicator(t.window, mssValue(mss), uint16(ipHeaderLen), hasTS, ver)

	pclass := payloadZero
	if len(t.payload) > 0 {
		pclass = payloadNonZero
	}

	obs := &tcpObservation{
		version: ver,
		ittl:    ittl,
		olen:    olen,
		mss:     mss,
		wsize:   wsize,
		wscale:  wscale,
		olayout: olayout,
		quirks:  quirks,
		pclass:  pclass,
	}

	pkg := &tcpObservedPackage{mtu: mtu}
	if client {
		pkg.tcpRequest = obs
	} else {
		pkg.tcpResponse = obs
	}
	return pkg, nil
}

// mssValue 展开可选的 MSS 值（对应 Rust mss.unwrap_or(0)）。
func mssValue(mss *uint16) uint16 {
	if mss == nil {
		return 0
	}
	return *mss
}
