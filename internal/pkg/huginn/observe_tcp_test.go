// TCP 观测值提取单元测试：observe_tcp.go。
//
// 通过手工构造 IP/TCP 字节序列，覆盖 TTL 计算、窗口倍数识别、选项解析、
// 怪癖提取、MTU 反推与非法包拒绝路径。
package huginn

import (
	"encoding/binary"
	"testing"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// tcpOptBytes 构造单个 TCP 选项字节（kind + 长度 + 数据；EOL/NOP 无长度字段）。
func tcpOptBytes(kind byte, data ...byte) []byte {
	opt := []byte{kind}
	if kind == 0 || kind == 1 {
		return opt
	}
	opt = append(opt, byte(2+len(data)))
	return append(opt, data...)
}

// buildTCP 构造 TCP 段（选项 + 载荷）。
func buildTCP(srcPort, dstPort uint16, seq, ack uint32, flags byte, window uint16, options, payload []byte) []byte {
	headerLen := 20 + len(options)
	buf := make([]byte, headerLen+len(payload))
	binary.BigEndian.PutUint16(buf[0:2], srcPort)
	binary.BigEndian.PutUint16(buf[2:4], dstPort)
	binary.BigEndian.PutUint32(buf[4:8], seq)
	binary.BigEndian.PutUint32(buf[8:12], ack)
	buf[12] = byte(headerLen/4) << 4
	buf[13] = flags
	binary.BigEndian.PutUint16(buf[14:16], window)
	binary.BigEndian.PutUint16(buf[18:20], 0) // urgent ptr
	copy(buf[20:], options)
	copy(buf[headerLen:], payload)
	return buf
}

// buildIPv4 构造 IPv4 数据包。flagsBits 为字节 6 的高 3 位（0x80 MBZ / 0x40 DF / 0x20 MF）。
func buildIPv4(ihl, tos byte, id uint16, flagsBits byte, fragOff uint16, ttl, proto byte, payload []byte) []byte {
	if ihl == 0 {
		ihl = 5
	}
	headerLen := int(ihl) * 4
	buf := make([]byte, headerLen+len(payload))
	buf[0] = 4<<4 | ihl
	buf[1] = tos
	binary.BigEndian.PutUint16(buf[2:4], uint16(len(buf)))
	binary.BigEndian.PutUint16(buf[4:6], id)
	buf[6] = flagsBits | byte(fragOff>>8)
	buf[7] = byte(fragOff)
	buf[8] = ttl
	buf[9] = proto
	copy(buf[headerLen:], payload)
	return buf
}

// buildIPv6 构造 IPv6 数据包。
func buildIPv6(trafficClass uint16, flowLabel uint32, nextHeader, hopLimit byte, payload []byte) []byte {
	buf := make([]byte, 40+len(payload))
	buf[0] = 6<<4 | byte(trafficClass>>4)
	buf[1] = byte(trafficClass<<4) | byte(flowLabel>>16)
	buf[2] = byte(flowLabel >> 8)
	buf[3] = byte(flowLabel)
	binary.BigEndian.PutUint16(buf[4:6], uint16(len(payload)))
	buf[6] = nextHeader
	buf[7] = hopLimit
	copy(buf[40:], payload)
	return buf
}

// linuxSYNPacket 构造典型的 Linux 3.1-3.10 IPv4 SYN（与 p0f.fp 签名完全对应）。
func linuxSYNPacket() []byte {
	opts := []byte{}
	opts = append(opts, tcpOptBytes(tcpOptMSS, 0x05, 0xb4)...) // mss 1460
	opts = append(opts, tcpOptBytes(tcpOptSOK)...)
	opts = append(opts, tcpOptBytes(tcpOptTS, 0x01, 0x02, 0x03, 0x04, 0, 0, 0, 0)...)
	opts = append(opts, tcpOptBytes(tcpOptNOP)...)
	opts = append(opts, tcpOptBytes(tcpOptWS, 6)...)
	tcp := buildTCP(12345, 80, 0x01020304, 0, tcpFlagSYN, 14600, opts, nil)
	return buildIPv4(5, 0, 0x1234, 0x40, 0, 64, ipProtoTCP, tcp)
}

// TestGuessDistance 验证由 TTL 值反推的初始跳数估算。
func TestGuessDistance(t *testing.T) {
	cases := []struct {
		in, want uint8
	}{
		{200, 55}, {129, 126}, {128, 0}, {100, 28}, {64, 0}, {32, 0}, {10, 22}, {255, 0},
	}
	for _, tc := range cases {
		if got := guessDistance(tc.in); got != tc.want {
			t.Errorf("guessDistance(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestCalculateTTL 验证由 IP TTL 字段推导 TTL 表示形式（距离 / 取值 / 非法）。
func TestCalculateTTL(t *testing.T) {
	cases := []struct {
		in   uint8
		want ttl
	}{
		{0, ttl{kind: ttlBad, first: 0}},
		{64, ttl{kind: ttlDistance, first: 64, second: 0}},
		{63, ttl{kind: ttlDistance, first: 63, second: 1}},
		{128, ttl{kind: ttlDistance, first: 128, second: 0}},
		{100, ttl{kind: ttlDistance, first: 100, second: 28}},
		{255, ttl{kind: ttlDistance, first: 255, second: 0}},
		{129, ttl{kind: ttlValue, first: 129}}, // 255-129=126 > 30
		{1, ttl{kind: ttlValue, first: 1}},     // 32-1=31 > 30
		{200, ttl{kind: ttlValue, first: 200}}, // 255-200=55 > 30
	}
	for _, tc := range cases {
		if got := calculateTTL(tc.in); got != tc.want {
			t.Errorf("calculateTTL(%d) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

// TestDetectWinMultiplicator 验证窗口大小倍数的识别：MSS 倍数、MTU 倍数、模数与原始值。
func TestDetectWinMultiplicator(t *testing.T) {
	cases := []struct {
		name         string
		win, mss, th uint16
		ts           bool
		ver          ipVersion
		want         windowSize
	}{
		{"mss multiple", 14600, 1460, 5, false, ipV4, windowSize{kind: winMss, value: 10}},
		{"mss ts adjusted", 5792, 1460, 5, true, ipV4, windowSize{kind: winMss, value: 4}}, // (1460-12)*4
		{"zero window", 0, 1460, 5, false, ipV4, windowSize{kind: winValue, value: 0}},
		{"small mss", 14600, 50, 5, false, ipV4, windowSize{kind: winValue, value: 14600}},
		{"mod 1024", 1024, 1460, 5, false, ipV4, windowSize{kind: winMod, value: 1024}},
		{"mod 4096 first", 4096, 1460, 5, false, ipV4, windowSize{kind: winMod, value: 4096}},
		{"mtu 1500", 3000, 1460, 5, false, ipV4, windowSize{kind: winMtu, value: 2}},
		{"mtu v4 1460", 2920, 1000, 5, false, ipV4, windowSize{kind: winMtu, value: 2}},
		{"mtu v4 1448 ts", 2896, 1000, 5, true, ipV4, windowSize{kind: winMtu, value: 2}},
		{"mtu v6 1440", 2880, 1000, 40, false, ipV6, windowSize{kind: winMtu, value: 2}},
		{"mtu v6 1428 ts", 2856, 1000, 40, true, ipV6, windowSize{kind: winMtu, value: 2}},
		{"mss+header", 2930, 1460, 5, false, ipV4, windowSize{kind: winMtu, value: 2}},   // (1460+5)*2
		{"mss+min tcp6", 1520, 1460, 0, false, ipV6, windowSize{kind: winMtu, value: 1}}, // 1460+60
		{"value fallback", 65535, 1460, 5, false, ipV4, windowSize{kind: winValue, value: 65535}},
	}
	for _, tc := range cases {
		got := detectWinMultiplicator(tc.win, tc.mss, tc.th, tc.ts, tc.ver)
		if got != tc.want {
			t.Errorf("%s: detectWinMultiplicator(%d,%d,%d,%v) = %+v, want %+v",
				tc.name, tc.win, tc.mss, tc.th, tc.ts, got, tc.want)
		}
	}
}

// TestParseTCPPacketErrors 验证 TCP 段解析对过短数据与非法偏移的拒绝。
func TestParseTCPPacketErrors(t *testing.T) {
	if _, err := parseTCPPacket(nil); !errors.Is(err, ErrPacketTooShort) {
		t.Errorf("empty: got %v, want ErrPacketTooShort", err)
	}
	// data_offset < 5 视为非法
	short := make([]byte, 20)
	if _, err := parseTCPPacket(short); !errors.Is(err, ErrUnexpectedPacket) {
		t.Errorf("data_offset=0: got %v, want ErrUnexpectedPacket", err)
	}
}

// TestParseTCPPacketOptions 验证 TCP 选项区与载荷的解析及数据偏移计算。
func TestParseTCPPacketOptions(t *testing.T) {
	// 选项区长度必须是 4 的倍数（data_offset 以 32 位字为单位）
	opts := tcpOptBytes(tcpOptMSS, 0x05, 0xb4)
	opts = append(opts, tcpOptBytes(tcpOptNOP)...)
	opts = append(opts, tcpOptBytes(tcpOptNOP)...)
	opts = append(opts, tcpOptBytes(tcpOptNOP)...)
	opts = append(opts, tcpOptBytes(tcpOptNOP)...) // 4+4=8 字节
	raw := buildTCP(1, 2, 0, 0, tcpFlagSYN, 100, opts, []byte{0xde, 0xad})
	tcp, err := parseTCPPacket(raw)
	if err != nil {
		t.Fatalf("parseTCPPacket: %v", err)
	}
	if len(tcp.options) != len(opts) {
		t.Errorf("options len = %d, want %d", len(tcp.options), len(opts))
	}
	if len(tcp.payload) != 2 {
		t.Errorf("payload len = %d, want 2", len(tcp.payload))
	}
	if tcp.dataOffset != 7 {
		t.Errorf("dataOffset = %d, want 7", tcp.dataOffset)
	}
}

// TestAnalyzeTCPIPv4Errors 验证 IPv4 包分析对版本、协议与分片的拒绝。
func TestAnalyzeTCPIPv4Errors(t *testing.T) {
	if _, err := analyzeTCPIPv4(nil); !errors.Is(err, ErrPacketTooShort) {
		t.Errorf("empty: got %v", err)
	}
	bad := make([]byte, 20)
	bad[0] = 6 << 4 // version 6
	if _, err := analyzeTCPIPv4(bad); !errors.Is(err, ErrNotIPVersion) {
		t.Errorf("version 6: got %v", err)
	}
	noTCP := buildIPv4(5, 0, 0, 0, 0, 64, 17, nil) // UDP
	if _, err := analyzeTCPIPv4(noTCP); !errors.Is(err, ErrUnsupportedProtocol) {
		t.Errorf("udp: got %v", err)
	}
	// 分片：fragment offset > 0
	frag := buildIPv4(5, 0, 0x1234, 0, 100, 64, ipProtoTCP, nil)
	if _, err := analyzeTCPIPv4(frag); !errors.Is(err, ErrUnexpectedPacket) {
		t.Errorf("fragment: got %v", err)
	}
	// 分片：MF 置位
	mf := buildIPv4(5, 0, 0x1234, 0x20, 0, 64, ipProtoTCP, nil)
	if _, err := analyzeTCPIPv4(mf); !errors.Is(err, ErrUnexpectedPacket) {
		t.Errorf("mf: got %v", err)
	}
}

// TestAnalyzeTCPIPv4LinuxSYN 验证典型 Linux SYN 包可提取全部观测维度并渲染出 p0f 签名串。
func TestAnalyzeTCPIPv4LinuxSYN(t *testing.T) {
	pkg, err := analyzeTCPIPv4(linuxSYNPacket())
	if err != nil {
		t.Fatalf("analyzeTCPIPv4: %v", err)
	}
	if pkg.tcpRequest == nil || pkg.tcpResponse != nil {
		t.Fatalf("expected tcpRequest only, got req=%v resp=%v", pkg.tcpRequest != nil, pkg.tcpResponse != nil)
	}
	obs := pkg.tcpRequest
	if obs.version != ipV4 {
		t.Errorf("version = %v", obs.version)
	}
	if want := (ttl{kind: ttlDistance, first: 64, second: 0}); obs.ittl != want {
		t.Errorf("ittl = %+v, want %+v", obs.ittl, want)
	}
	if obs.olen != 0 {
		t.Errorf("olen = %d, want 0", obs.olen)
	}
	if obs.mss == nil || *obs.mss != 1460 {
		t.Errorf("mss = %v, want 1460", obs.mss)
	}
	if want := (windowSize{kind: winMss, value: 10}); obs.wsize != want {
		t.Errorf("wsize = %+v, want %+v", obs.wsize, want)
	}
	if obs.wscale == nil || *obs.wscale != 6 {
		t.Errorf("wscale = %v, want 6", obs.wscale)
	}
	wantLayout := []tcpOption{{kind: optMss}, {kind: optSok}, {kind: optTS}, {kind: optNop}, {kind: optWs}}
	if len(obs.olayout) != len(wantLayout) {
		t.Fatalf("olayout = %v, want %v", obs.olayout, wantLayout)
	}
	for i := range wantLayout {
		if obs.olayout[i] != wantLayout[i] {
			t.Errorf("olayout[%d] = %+v, want %+v", i, obs.olayout[i], wantLayout[i])
		}
	}
	wantQuirks := []quirk{quirkDf, quirkNonZeroID}
	if len(obs.quirks) != len(wantQuirks) {
		t.Fatalf("quirks = %v, want %v", obs.quirks, wantQuirks)
	}
	for i := range wantQuirks {
		if obs.quirks[i] != wantQuirks[i] {
			t.Errorf("quirks[%d] = %v, want %v", i, obs.quirks[i], wantQuirks[i])
		}
	}
	if obs.pclass != payloadZero {
		t.Errorf("pclass = %v, want zero", obs.pclass)
	}
	if pkg.mtu == nil || *pkg.mtu != 1500 {
		t.Errorf("mtu = %v, want 1500", pkg.mtu)
	}
	// 签名串应可渲染为 p0f 文本
	if want := "4:64+0:0:1460:mss*10,6:mss,sok,ts,nop,ws:df,id+:0"; obs.String() != want {
		t.Errorf("String() = %s, want %s", obs.String(), want)
	}
}

// TestAnalyzeTCPIPv4SynAck 验证 SYN-ACK 包被识别为响应方向且不输出 MTU。
func TestAnalyzeTCPIPv4SynAck(t *testing.T) {
	tcp := buildTCP(80, 12345, 0x02020202, 0x01020304, tcpFlagSYN|tcpFlagACK, 14600, nil, nil)
	raw := buildIPv4(5, 0, 0x1234, 0x40, 0, 64, ipProtoTCP, tcp)
	pkg, err := analyzeTCPIPv4(raw)
	if err != nil {
		t.Fatalf("analyzeTCPIPv4: %v", err)
	}
	if pkg.tcpResponse == nil || pkg.tcpRequest != nil {
		t.Fatalf("expected tcpResponse only, got req=%v resp=%v", pkg.tcpRequest != nil, pkg.tcpResponse != nil)
	}
	// 无选项：wsize 应为原始值；MTU 不输出
	if pkg.mtu != nil {
		t.Errorf("mtu = %v, want nil (server side)", pkg.mtu)
	}
	if pkg.tcpResponse.wsize != (windowSize{kind: winValue, value: 14600}) {
		t.Errorf("wsize = %+v", pkg.tcpResponse.wsize)
	}
}

// TestAnalyzeTCPIPv4InvalidFlags 验证非法标志位组合被拒绝。
func TestAnalyzeTCPIPv4InvalidFlags(t *testing.T) {
	// SYN + FIN 组合非法
	tcp := buildTCP(1, 2, 0, 0, tcpFlagSYN|tcpFlagFIN, 100, nil, nil)
	raw := buildIPv4(5, 0, 0, 0, 0, 64, ipProtoTCP, tcp)
	if _, err := analyzeTCPIPv4(raw); !errors.Is(err, ErrInvalidTCPFlags) {
		t.Errorf("syn|fin: got %v, want ErrInvalidTCPFlags", err)
	}
	// 无任何握手标志（type 0）
	tcp = buildTCP(1, 2, 0, 0, 0, 100, nil, nil)
	raw = buildIPv4(5, 0, 0, 0, 0, 64, ipProtoTCP, tcp)
	if _, err := analyzeTCPIPv4(raw); !errors.Is(err, ErrInvalidTCPFlags) {
		t.Errorf("no flags: got %v, want ErrInvalidTCPFlags", err)
	}
}

// TestTCPQuirksExtraction 验证各类 TCP 怪癖标记的提取。
func TestTCPQuirksExtraction(t *testing.T) {
	// ECN：IP TOS 低 2 位 + TCP ECE/CWR；seq-、ack-、uptr+、pushf+
	tcp := buildTCP(1, 2, 0, 0, tcpFlagSYN|tcpFlagECE|tcpFlagPSH, 100, nil, nil)
	raw := buildIPv4(5, 0x03, 0, 0, 0, 64, ipProtoTCP, tcp)
	pkg, err := analyzeTCPIPv4(raw)
	if err != nil {
		t.Fatalf("analyzeTCPIPv4: %v", err)
	}
	want := []quirk{quirkEcn, quirkZeroID, quirkEcn, quirkSeqNumZero, quirkPush}
	got := pkg.tcpRequest.quirks
	if len(got) != len(want) {
		t.Fatalf("quirks = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("quirks[%d] = %v, want %v", i, got[i], want[i])
		}
	}

	// ack+（ACK 字段非零但未置 ACK）、urgf+
	tcp = buildTCP(1, 2, 1, 5, tcpFlagSYN|tcpFlagURG, 100, nil, nil)
	raw = buildIPv4(5, 0, 0, 0, 0, 64, ipProtoTCP, tcp)
	pkg, err = analyzeTCPIPv4(raw)
	if err != nil {
		t.Fatalf("analyzeTCPIPv4: %v", err)
	}
	got = pkg.tcpRequest.quirks
	want = []quirk{quirkZeroID, quirkAckNumNonZero, quirkUrg}
	if len(got) != len(want) {
		t.Fatalf("quirks = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("quirks[%d] = %v, want %v", i, got[i], want[i])
		}
	}

	// ACK 置位但 ACK 字段为零 → ack-
	tcp = buildTCP(1, 2, 1, 0, tcpFlagACK, 100, nil, nil)
	raw = buildIPv4(5, 0, 0x10, 0, 0, 64, ipProtoTCP, tcp)
	pkg, err = analyzeTCPIPv4(raw)
	if err != nil {
		t.Fatalf("analyzeTCPIPv4: %v", err)
	}
	if pkg.tcpResponse == nil {
		t.Fatal("expected tcpResponse")
	}
	if !containsQuirk(pkg.tcpResponse.quirks, quirkAckNumZero) {
		t.Errorf("quirks = %v, want ack-", pkg.tcpResponse.quirks)
	}
}

func containsQuirk(quirks []quirk, target quirk) bool {
	for _, q := range quirks {
		if q == target {
			return true
		}
	}
	return false
}

// TestTCPOptionEdgeCases 验证 TCP 选项的边界处理：EOL 填充、未知选项、SACK 与时间戳等。
func TestTCPOptionEdgeCases(t *testing.T) {
	// EOL 后零填充 → eol+1，无 opt+ 怪癖
	// 选项区 4 字节对齐：NOP,NOP,EOL,pad
	opts := []byte{tcpOptNOP, tcpOptNOP, tcpOptEOL, 0}
	tcp := buildTCP(1, 2, 0, 0, tcpFlagSYN, 100, opts, nil)
	pkg, err := analyzeTCPIPv4(buildIPv4(5, 0, 0, 0, 0, 64, ipProtoTCP, tcp))
	if err != nil {
		t.Fatalf("analyzeTCPIPv4: %v", err)
	}
	if len(pkg.tcpRequest.olayout) != 3 || pkg.tcpRequest.olayout[2] != (tcpOption{kind: optEol, num: 1}) {
		t.Errorf("olayout = %v, want [nop nop eol+1]", pkg.tcpRequest.olayout)
	}
	if containsQuirk(pkg.tcpRequest.quirks, quirkTrailingNonZero) {
		t.Errorf("quirks = %v, unexpected opt+", pkg.tcpRequest.quirks)
	}

	// EOL 后非零填充 → opt+ 怪癖
	opts = []byte{2, 4, 0x05, 0xb4, tcpOptEOL, 0x05, 0, 0}
	tcp = buildTCP(1, 2, 0, 0, tcpFlagSYN, 100, opts, nil)
	pkg, err = analyzeTCPIPv4(buildIPv4(5, 0, 0, 0, 0, 64, ipProtoTCP, tcp))
	if err != nil {
		t.Fatalf("analyzeTCPIPv4: %v", err)
	}
	if !containsQuirk(pkg.tcpRequest.quirks, quirkTrailingNonZero) {
		t.Errorf("quirks = %v, want opt+", pkg.tcpRequest.quirks)
	}

	// ws > 14 → exws；ts1=0 → ts1-；ts2≠0（SYN）→ ts2+；未知选项 → ?29
	opts = append(tcpOptBytes(tcpOptWS, 15), tcpOptBytes(tcpOptTS, 0, 0, 0, 0, 0, 0, 0, 1)...)
	opts = append(opts, tcpOptBytes(29, 1)...)
	tcp = buildTCP(1, 2, 0, 0, tcpFlagSYN, 100, opts, nil)
	pkg, err = analyzeTCPIPv4(buildIPv4(5, 0, 0, 0, 0, 64, ipProtoTCP, tcp))
	if err != nil {
		t.Fatalf("analyzeTCPIPv4: %v", err)
	}
	for _, want := range []quirk{quirkExcessiveWindowScaling, quirkOwnTimestampZero, quirkPeerTimestampNonZero} {
		if !containsQuirk(pkg.tcpRequest.quirks, want) {
			t.Errorf("quirks = %v, want %v", pkg.tcpRequest.quirks, want)
		}
	}
	if !containsOption(pkg.tcpRequest.olayout, tcpOption{kind: optUnknown, num: 29}) {
		t.Errorf("olayout = %v, want ?29", pkg.tcpRequest.olayout)
	}

	// SACK 与 SACK_PERMITTED
	opts = append(tcpOptBytes(tcpOptSOK), tcpOptBytes(tcpOptSACK, 1, 2, 3, 4, 5, 6, 7, 8)...)
	tcp = buildTCP(1, 2, 0, 0, tcpFlagSYN, 100, opts, nil)
	pkg, err = analyzeTCPIPv4(buildIPv4(5, 0, 0, 0, 0, 64, ipProtoTCP, tcp))
	if err != nil {
		t.Fatalf("analyzeTCPIPv4: %v", err)
	}
	if len(pkg.tcpRequest.olayout) != 2 || pkg.tcpRequest.olayout[0].kind != optSok || pkg.tcpRequest.olayout[1].kind != optSack {
		t.Errorf("olayout = %v, want [sok sack]", pkg.tcpRequest.olayout)
	}
}

func containsOption(opts []tcpOption, target tcpOption) bool {
	for _, o := range opts {
		if o == target {
			return true
		}
	}
	return false
}

// TestAnalyzeTCPIPv6 验证 IPv6 SYN 包分析：版本、窗口倍数、flow/ecn 怪癖与 MTU。
func TestAnalyzeTCPIPv6(t *testing.T) {
	// 非法版本
	bad := make([]byte, 40)
	bad[0] = 4 << 4
	if _, err := analyzeTCPIPv6(bad); !errors.Is(err, ErrNotIPVersion) {
		t.Errorf("version 4: got %v", err)
	}
	// 非 TCP next header
	udp := buildIPv6(0, 0, 17, 64, []byte{})
	if _, err := analyzeTCPIPv6(udp); !errors.Is(err, ErrUnsupportedProtocol) {
		t.Errorf("udp: got %v", err)
	}
	// 过短
	if _, err := analyzeTCPIPv6(nil); !errors.Is(err, ErrPacketTooShort) {
		t.Errorf("short: got %v", err)
	}

	// 合法 IPv6 SYN：hop limit 64、traffic class 低 2 位 ECN、flow label 非零
	opts := append(tcpOptBytes(tcpOptMSS, 0x05, 0xa0), tcpOptBytes(tcpOptSOK)...) // mss 1440
	opts = append(opts, tcpOptBytes(tcpOptNOP)...)
	opts = append(opts, tcpOptBytes(tcpOptNOP)...) // 4+2+1+1=8 字节，4 对齐
	tcp := buildTCP(12345, 80, 1, 0, tcpFlagSYN, 2880, opts, nil)
	raw := buildIPv6(0x02, 0xabcde, ipProtoTCP, 64, tcp)
	pkg, err := analyzeTCPIPv6(raw)
	if err != nil {
		t.Fatalf("analyzeTCPIPv6: %v", err)
	}
	obs := pkg.tcpRequest
	if obs.version != ipV6 {
		t.Errorf("version = %v", obs.version)
	}
	if obs.olen != 0 {
		t.Errorf("olen = %d, want 0", obs.olen)
	}
	// mss 1440，窗口 2880 → mss*2
	if obs.wsize != (windowSize{kind: winMss, value: 2}) {
		t.Errorf("wsize = %+v", obs.wsize)
	}
	// flow + ecn 怪癖
	if !containsQuirk(obs.quirks, quirkFlowID) || !containsQuirk(obs.quirks, quirkEcn) {
		t.Errorf("quirks = %v, want flow and ecn", obs.quirks)
	}
	// MTU = 1440 + 40（IPv6 头）+ 8（TCP 选项区 = 28-20）
	if pkg.mtu == nil || *pkg.mtu != 1488 {
		t.Errorf("mtu = %v, want 1488", pkg.mtu)
	}
	if want := "6:64+0:0:1440:mss*2,*:mss,sok,nop,nop:flow,ecn:0"; obs.String() != want {
		t.Errorf("String() = %s, want %s", obs.String(), want)
	}
}

// TestCalculateIPv6Olen 验证 IPv6 扩展头长度的计算。
func TestCalculateIPv6Olen(t *testing.T) {
	// next header 直接为 TCP → 0
	if got := calculateIPv6Olen(buildIPv6(0, 0, ipProtoTCP, 64, nil)); got != 0 {
		t.Errorf("tcp: got %d", got)
	}
	// 分片头 → 8
	if got := calculateIPv6Olen(buildIPv6(0, 0, ipProtoIPv6Frag, 64, make([]byte, 8))); got != 8 {
		t.Errorf("frag: got %d", got)
	}
	// 其它扩展头：payload[1]=1 → (1+1)*8=16
	if got := calculateIPv6Olen(buildIPv6(0, 0, 60, 64, []byte{0, 1})); got != 16 {
		t.Errorf("ext: got %d", got)
	}
}

// TestMTUExtraction 验证 MTU 提取：方向过滤与 IP/TCP 选项长度累加。
func TestMTUExtraction(t *testing.T) {
	// 服务端方向不输出 MTU（extract 仅在 SYN 置位时计算，但输出被 from_client 过滤）
	tcp := buildTCP(80, 12345, 0, 0, tcpFlagSYN|tcpFlagACK, 100, tcpOptBytes(tcpOptMSS, 0x05, 0xb4), nil)
	pkg, err := analyzeTCPIPv4(buildIPv4(5, 0, 0, 0, 0, 64, ipProtoTCP, tcp))
	if err != nil {
		t.Fatalf("analyzeTCPIPv4: %v", err)
	}
	if pkg.mtu != nil {
		t.Errorf("syn-ack mtu = %v, want nil", pkg.mtu)
	}
	// IPv4 带 IP 选项（IHL=6 → 24 字节）时 MTU 包含 IP 选项长度
	opts := tcpOptBytes(tcpOptMSS, 0x05, 0xb4)
	opts = append(opts, tcpOptBytes(tcpOptNOP)...)
	opts = append(opts, tcpOptBytes(tcpOptNOP)...)
	opts = append(opts, tcpOptBytes(tcpOptNOP)...)
	opts = append(opts, tcpOptBytes(tcpOptNOP)...) // 4+1+1+1+1=8
	tcp = buildTCP(12345, 80, 0, 0, tcpFlagSYN, 100, opts, nil)
	raw := make([]byte, 24+len(tcp))
	raw[0] = 4<<4 | 6
	raw[8] = 64
	raw[9] = ipProtoTCP
	copy(raw[24:], tcp)
	pkg, err = analyzeTCPIPv4(raw)
	if err != nil {
		t.Fatalf("analyzeTCPIPv4: %v", err)
	}
	if pkg.mtu == nil || *pkg.mtu != 1460+24+8 {
		t.Errorf("mtu = %v, want %d", pkg.mtu, 1460+24+8)
	}
}
