package huginn

// tcpObservation 从一次 TCP 握手包（SYN / SYN-ACK）中提取的观测值。
// 对应 huginn-net-tcp 的 TcpObservation。
type tcpObservation struct {
	version ipVersion
	ittl    ttl
	olen    uint8
	mss     *uint16
	wsize   windowSize
	wscale  *uint8
	olayout []tcpOption
	quirks  []quirk
	pclass  payloadSize
}

// String 渲染为 p0f 数据库文本形式（与 tcpSignature 相同的 8 段格式）。
func (o tcpObservation) String() string {
	return tcpSignature{
		version: o.version,
		ittl:    o.ittl,
		olen:    o.olen,
		mss:     o.mss,
		wsize:   o.wsize,
		wscale:  o.wscale,
		olayout: o.olayout,
		quirks:  o.quirks,
		pclass:  o.pclass,
	}.String()
}

// httpObservation 从 HTTP/1.x 请求或响应头中提取的观测值。
// 对应 huginn-net-http 的 HttpRequestObservation / HttpResponseObservation。
type httpObservation struct {
	version httpVersion
	horder  []httpHeader
	habsent []httpHeader
	expsw   string
}
