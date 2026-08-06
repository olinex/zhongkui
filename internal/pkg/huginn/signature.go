package huginn

// tcpSignature TCP 指纹签名（p0f 数据库条目）。
// 对应 huginn-net-db 的 tcp::Signature。
type tcpSignature struct {
	// version IP 协议版本。
	version ipVersion
	// ittl 操作系统使用的初始 TTL。
	ittl ttl
	// olen IPv4 选项或 IPv6 扩展头的长度。
	olen uint8
	// mss TCP 选项中指定的最大分段大小；nil 表示通配。
	mss *uint16
	// wsize 窗口大小。
	wsize windowSize
	// wscale TCP 选项中指定的窗口缩放因子；nil 表示通配。
	wscale *uint8
	// olayout TCP 选项的布局与顺序。
	olayout []tcpOption
	// quirks 在 IP 或 TCP 头中观察到的属性与怪癖。
	quirks []quirk
	// pclass 载荷大小分类。
	pclass payloadSize
}

// String 渲染为 p0f 数据库文本形式：
// `version:ittl:olen:mss:wsize,wscale:olayout:quirks:pclass`。
func (s tcpSignature) String() string {
	buf := make([]byte, 0, 48)
	buf = append(buf, s.version.String()...)
	buf = append(buf, ':')
	buf = append(buf, s.ittl.String()...)
	buf = append(buf, ':')
	buf = append(buf, uint8String(s.olen)...)
	buf = append(buf, ':')
	if s.mss != nil {
		buf = append(buf, uint16String(*s.mss)...)
	} else {
		buf = append(buf, '*')
	}
	buf = append(buf, ':')
	buf = append(buf, s.wsize.String()...)
	buf = append(buf, ',')
	if s.wscale != nil {
		buf = append(buf, uint8String(*s.wscale)...)
	} else {
		buf = append(buf, '*')
	}
	buf = append(buf, ':')
	for i, o := range s.olayout {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, o.String()...)
	}
	buf = append(buf, ':')
	for i, q := range s.quirks {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, q.String()...)
	}
	buf = append(buf, ':')
	buf = append(buf, s.pclass.String()...)
	return string(buf)
}

// httpSignature HTTP 指纹签名（p0f 数据库条目）。
// 对应 huginn-net-db 的 http::Signature。
type httpSignature struct {
	// version HTTP 版本。
	version httpVersion
	// horder 匹配流量中应出现的头的有序列表。
	horder []httpHeader
	// habsent 匹配流量中必须不出现的头列表。
	habsent []httpHeader
	// expsw User-Agent 或 Server 中的期望子串。
	expsw string
}

// String 渲染为 p0f 数据库文本形式：
// `version:horder:habsent:expsw`。
func (s httpSignature) String() string {
	buf := make([]byte, 0, 64)
	buf = append(buf, s.version.String()...)
	buf = append(buf, ':')
	for i, h := range s.horder {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, h.String()...)
	}
	buf = append(buf, ':')
	for i, h := range s.habsent {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, h.String()...)
	}
	buf = append(buf, ':')
	buf = append(buf, s.expsw...)
	return string(buf)
}
