// Package huginn 提供 p0f 风格的被动指纹识别能力（huginn-net 的纯 Go 移植）。
//
// 本包将上游 huginn-net（Rust）的签名库解析、索引匹配引擎与观测值提取逻辑
// 以纯 Go 重写，对外保持与原 cgo 绑定完全一致的 API（TCPFingerprint /
// HTTPFingerprint 与 AnalyzeTCPIPv4/AnalyzeTCPIPv6/AnalyzeHTTPRequest/
// AnalyzeHTTPResponse），不再依赖 Rust 工具链与 FFI 静态库。
//
// 数据文件 p0f.fp（上游基础签名库）与 p0f.local.fp（本地定制签名）随包内嵌，
// 两者拼接后加载，与上游 ffi 的 include_str 行为一致。
package huginn

// ipVersion IP 协议版本（或通配符）。
// 对应 huginn-net-tcp 的 IpVersion。
type ipVersion uint8

const (
	// ipV4 IPv4。
	ipV4 ipVersion = iota
	// ipV6 IPv6。
	ipV6
	// ipAny 通配符（签名库中使用）。
	ipAny
)

// String 渲染为 p0f 文本形式：4 / 6 / *。
func (v ipVersion) String() string {
	switch v {
	case ipV4:
		return "4"
	case ipV6:
		return "6"
	default:
		return "*"
	}
}

// ttlKind TTL 表示形式的类型标签。
type ttlKind uint8

const (
	// ttlValue 原始 TTL 值（无足够上下文确定初始 TTL）。
	ttlValue ttlKind = iota
	// ttlDistance 携带网络距离的 TTL（first=观测值，second=跳数）。
	ttlDistance
	// ttlGuess 基于常见初始 TTL 猜测的值。
	ttlGuess
	// ttlBad 无效或异常的 TTL（如 0）。
	ttlBad
)

// ttl 生存时间（TTL）表示，用于 OS 指纹与网络距离估计。
// 对应 huginn-net-tcp 的 Ttl 枚举。
type ttl struct {
	kind   ttlKind
	first  uint8 // Value/Guess/Bad 为值本身；Distance 为观测 TTL
	second uint8 // 仅 Distance 使用：估计跳数
}

// String 渲染为 p0f 文本形式，如 64 / 64+0 / 64+? / 0-。
func (t ttl) String() string {
	switch t.kind {
	case ttlDistance:
		return uint8String(t.first) + "+" + uint8String(t.second)
	case ttlGuess:
		return uint8String(t.first) + "+?"
	case ttlBad:
		return uint8String(t.first) + "-"
	default:
		return uint8String(t.first)
	}
}

// windowSizeKind 窗口大小表示形式的类型标签。
type windowSizeKind uint8

const (
	// winMss 窗口大小为 MSS 的整数倍（value 为倍数）。
	winMss windowSizeKind = iota
	// winMtu 窗口大小为 MTU 的整数倍（value 为倍数）。
	winMtu
	// winValue 原始窗口大小值。
	winValue
	// winMod 窗口大小符合取模模式（value 为模数）。
	winMod
	// winAny 通配符。
	winAny
)

// windowSize TCP 窗口大小表示，用于指纹识别不同 TCP 协议栈。
// 对应 huginn-net-tcp 的 WindowSize 枚举。
type windowSize struct {
	kind  windowSizeKind
	value uint16
}

// String 渲染为 p0f 文本形式，如 mss*10 / mtu*2 / 65535 / %1024 / *。
func (w windowSize) String() string {
	switch w.kind {
	case winMss:
		return "mss*" + uint16String(w.value)
	case winMtu:
		return "mtu*" + uint16String(w.value)
	case winMod:
		return "%" + uint16String(w.value)
	case winAny:
		return "*"
	default:
		return uint16String(w.value)
	}
}

// tcpOptionKind TCP 选项类型的标签。
type tcpOptionKind uint8

const (
	// optEol 显式选项结束，后随 n 字节填充（num 为填充字节数）。
	optEol tcpOptionKind = iota
	// optNop 空操作选项。
	optNop
	// optMss 最大分段大小选项。
	optMss
	// optWs 窗口缩放选项。
	optWs
	// optSok 选择性确认允许选项。
	optSok
	// optSack 选择性确认选项（正常不应出现）。
	optSack
	// optTS 时间戳选项。
	optTS
	// optUnknown 未知选项（num 为选项号）。
	optUnknown
)

// tcpOption 一个 TCP 选项，按线上出现顺序排列。
// 对应 huginn-net-tcp 的 TcpOption 枚举。
type tcpOption struct {
	kind tcpOptionKind
	num  uint8 // optEol 为填充字节数；optUnknown 为选项号
}

// String 渲染为 p0f 文本形式，如 eol+1 / nop / mss / ws / sok / sack / ts / ?29。
func (o tcpOption) String() string {
	switch o.kind {
	case optEol:
		return "eol+" + uint8String(o.num)
	case optNop:
		return "nop"
	case optMss:
		return "mss"
	case optWs:
		return "ws"
	case optSok:
		return "sok"
	case optSack:
		return "sack"
	case optTS:
		return "ts"
	default:
		return "?" + uint8String(o.num)
	}
}

// quirk 在 IP 或 TCP 头中观察到的协议怪癖。
// 对应 huginn-net-tcp 的 Quirk 枚举。
type quirk uint8

const (
	// quirkDf df - 设置"不分片"位（疑似 PMTUD）；IPv6 忽略。
	quirkDf quirk = iota
	// quirkNonZeroID id+ - 设置 DF 且 IP 标识非零；IPv6 忽略。
	quirkNonZeroID
	// quirkZeroID id- - 未设置 DF 但 IP 标识为零；IPv6 忽略。
	quirkZeroID
	// quirkEcn ecn - 显式拥塞通知支持。
	quirkEcn
	// quirkMustBeZero 0+ - "必须为零"字段非零；IPv6 忽略。
	quirkMustBeZero
	// quirkFlowID flow - 非零 IPv6 流标签；IPv4 忽略。
	quirkFlowID
	// quirkSeqNumZero seq- - 序列号为零。
	quirkSeqNumZero
	// quirkAckNumNonZero ack+ - ACK 字段非零但未设置 ACK 标志。
	quirkAckNumNonZero
	// quirkAckNumZero ack- - 设置 ACK 标志但 ACK 字段为零。
	quirkAckNumZero
	// quirkNonZeroURG uptr+ - 未设置 URG 标志但紧急指针非零。
	quirkNonZeroURG
	// quirkUrg urgf+ - 使用 URG 标志。
	quirkUrg
	// quirkPush pushf+ - 使用 PUSH 标志。
	quirkPush
	// quirkOwnTimestampZero ts1- - 自身时间戳指定为零。
	quirkOwnTimestampZero
	// quirkPeerTimestampNonZero ts2+ - 初始 SYN 上对端时间戳非零。
	quirkPeerTimestampNonZero
	// quirkTrailingNonZero opt+ - 选项段中存在尾随非零数据。
	quirkTrailingNonZero
	// quirkExcessiveWindowScaling exws - 过大的窗口缩放因子（> 14）。
	quirkExcessiveWindowScaling
	// quirkOptBad bad - 畸形 TCP 选项。
	quirkOptBad
)

// String 渲染为 p0f 文本形式。
func (q quirk) String() string {
	switch q {
	case quirkDf:
		return "df"
	case quirkNonZeroID:
		return "id+"
	case quirkZeroID:
		return "id-"
	case quirkEcn:
		return "ecn"
	case quirkMustBeZero:
		return "0+"
	case quirkFlowID:
		return "flow"
	case quirkSeqNumZero:
		return "seq-"
	case quirkAckNumNonZero:
		return "ack+"
	case quirkAckNumZero:
		return "ack-"
	case quirkNonZeroURG:
		return "uptr+"
	case quirkUrg:
		return "urgf+"
	case quirkPush:
		return "pushf+"
	case quirkOwnTimestampZero:
		return "ts1-"
	case quirkPeerTimestampNonZero:
		return "ts2+"
	case quirkTrailingNonZero:
		return "opt+"
	case quirkExcessiveWindowScaling:
		return "exws"
	default:
		return "bad"
	}
}

// payloadSize TCP 载荷大小分类。
// 对应 huginn-net-tcp 的 PayloadSize 枚举。
type payloadSize uint8

const (
	// payloadZero 无载荷（空）。
	payloadZero payloadSize = iota
	// payloadNonZero 数据包包含数据。
	payloadNonZero
	// payloadAny 通配符（签名库中使用）。
	payloadAny
)

// String 渲染为 p0f 文本形式：0 / + / *。
func (p payloadSize) String() string {
	switch p {
	case payloadZero:
		return "0"
	case payloadNonZero:
		return "+"
	default:
		return "*"
	}
}

// httpVersion HTTP 版本。
// 对应 huginn-net-http 的 Version 枚举。
type httpVersion uint8

const (
	// httpV10 HTTP/1.0。
	httpV10 httpVersion = iota
	// httpV11 HTTP/1.1。
	httpV11
	// httpV20 HTTP/2。
	httpV20
	// httpV30 HTTP/3。
	httpV30
	// httpAny 通配符（签名库中使用）。
	httpAny
)

// String 渲染为 p0f 数据库文本形式：0 / 1 / 2 / 3 / *。
func (v httpVersion) String() string {
	switch v {
	case httpV10:
		return "0"
	case httpV11:
		return "1"
	case httpV20:
		return "2"
	case httpV30:
		return "3"
	default:
		return "*"
	}
}

// parseHTTPVersionToken 将 HTTP 版本字符串（如 "HTTP/1.1"）解析为版本枚举。
func parseHTTPVersionToken(s string) (httpVersion, bool) {
	switch s {
	case "HTTP/1.0":
		return httpV10, true
	case "HTTP/1.1":
		return httpV11, true
	case "HTTP/2", "HTTP/2.0":
		return httpV20, true
	case "HTTP/3", "HTTP/3.0":
		return httpV30, true
	default:
		return 0, false
	}
}

// httpHeader p0f 风格 HTTP 签名中使用的头名（及可选值）。
// 头可标记为可选（?）："存在则匹配，缺失则忽略"。
// 对应 huginn-net-http 的 Header。
type httpHeader struct {
	optional bool
	name     string
	value    *string
}

// String 渲染为 p0f 文本形式，如 ?Accept-Language 或 Server=[nginx/]。
func (h httpHeader) String() string {
	var sb []byte
	if h.optional {
		sb = append(sb, '?')
	}
	sb = append(sb, h.name...)
	if h.value != nil {
		sb = append(sb, "=["...)
		sb = append(sb, *h.value...)
		sb = append(sb, ']')
	}
	return string(sb)
}

// labelType 签名的类型：具体定义或通用兜底。
// 对应 huginn-net-db 的 Type。
type labelType uint8

const (
	// labelSpecified s - 具体定义。
	labelSpecified labelType = iota
	// labelGeneric g - 通用兜底。
	labelGeneric
)

// String 渲染为 p0f 文本形式：s / g。
func (t labelType) String() string {
	if t == labelGeneric {
		return "g"
	}
	return "s"
}

// label 与签名关联的标签，提供签名元数据（类型、类别、名称与可选的细节）。
// 对应 huginn-net-db 的 Label。
type label struct {
	ty     labelType
	class  *string
	name   string
	flavor *string
}

// String 渲染为 p0f 文本形式，如 s:unix:Linux:3.1-3.10。
func (l label) String() string {
	class := ""
	if l.class != nil {
		class = *l.class
	}
	flavor := ""
	if l.flavor != nil {
		flavor = *l.flavor
	}
	return l.ty.String() + ":" + class + ":" + l.name + ":" + flavor
}

// formatGuess 拼接猜测串：优先 name，其次 name variant，兜底 family。
// 与上游 ffi 的 format_guess 一致。
func formatGuess(name string, family, variant *string) string {
	guess := name
	if variant != nil && *variant != "" {
		if guess != "" {
			guess += " "
		}
		guess += *variant
	}
	if guess == "" && family != nil {
		guess = *family
	}
	return guess
}

func uint8String(v uint8) string {
	if v == 0 {
		return "0"
	}
	buf := [3]byte{}
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = '0' + v%10
		v /= 10
	}
	return string(buf[i:])
}

func uint16String(v uint16) string {
	if v == 0 {
		return "0"
	}
	buf := [5]byte{}
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = '0' + byte(v%10)
		v /= 10
	}
	return string(buf[i:])
}
