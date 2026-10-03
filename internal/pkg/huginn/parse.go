// Package huginn — p0f.fp 数据库解析器（huginn-net-db parse 模块的纯 Go 移植）。
//
// 逐行解析 p0f `*.fp` 格式，跟踪当前 `[module:direction]` section，
// 分发到 TCP / HTTP 各自的叶子解析器，复刻上游 nom 组合子的行为
// （含 separated_list0 的容错语义、parse_ua_os 的"遇不可解析片段即截断"等）。
package huginn

import (
	"strings"

	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// parsedSections p0f.fp 文本的中间解析结果（对应 Rust parse::ParsedSections）。
type parsedSections struct {
	classes      []string
	mtu          []mtuEntry
	uaOS         []uaOSEntry
	tcpRequest   []labeledTCP
	tcpResponse  []labeledTCP
	httpRequest  []labeledHTTP
	httpResponse []labeledHTTP
}

// mtuEntry [mtu] section 中的一条 MTU 映射（label + 一组 MTU 值）。
type mtuEntry struct {
	label  string
	values []uint16
}

// uaOSEntry `ua_os` 表条目：User-Agent 子串 → 操作系统族。
type uaOSEntry struct {
	ua     string
	family *string
}

// labeledTCP 一个 label 及其关联的 TCP 签名列表。
type labeledTCP struct {
	label label
	sigs  []tcpSignature
}

// labeledHTTP 一个 label 及其关联的 HTTP 签名列表。
type labeledHTTP struct {
	label label
	sigs  []httpSignature
}

// parseErr 包装解析错误，附上被解析的片段，便于定位数据库问题。
func parseErr(what, input string, err error) error {
	return errors.Newf("fail to parse `%s`: %s, %v", what, input, err)
}

// parseSections 解析 p0f.fp 格式字符串为中间结构（对应 Rust parse_sections）。
func parseSections(s string) (*parsedSections, error) {
	out := &parsedSections{}
	var (
		curModule    string
		curDirection string
		hasModule    bool
	)

	for _, rawLine := range strings.Split(s, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "classes") {
			classes, err := parseClasses(line)
			if err != nil {
				return nil, parseErr("classes", line, err)
			}
			out.classes = append(out.classes, classes...)
			continue
		}

		if strings.HasPrefix(line, "ua_os") {
			entries, err := parseUAOS(line)
			if err != nil {
				return nil, parseErr("ua_os", line, err)
			}
			out.uaOS = append(out.uaOS, entries...)
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			module, direction, err := parseModule(line)
			if err != nil {
				return nil, parseErr("module", line, err)
			}
			curModule, curDirection, hasModule = module, direction, true
			continue
		}

		if !hasModule {
			return nil, errors.Newf("unexpected line outside the module: %s", line)
		}

		name, value, err := parseNamedValue(line)
		if err != nil {
			return nil, parseErr("named value", line, err)
		}

		switch name {
		case "label":
			if curModule == "mtu" {
				out.mtu = append(out.mtu, mtuEntry{label: value})
				continue
			}
			lbl, err := parseLabel(value)
			if err != nil {
				return nil, parseErr("label", value, err)
			}
			switch {
			case curModule == "tcp" && curDirection == "request":
				out.tcpRequest = append(out.tcpRequest, labeledTCP{label: lbl})
			case curModule == "tcp" && curDirection == "response":
				out.tcpResponse = append(out.tcpResponse, labeledTCP{label: lbl})
			case curModule == "http" && curDirection == "request":
				out.httpRequest = append(out.httpRequest, labeledHTTP{label: lbl})
			case curModule == "http" && curDirection == "response":
				out.httpResponse = append(out.httpResponse, labeledHTTP{label: lbl})
			default:
				// 未知 module 的 label：跳过
			}
		case "sig":
			if curModule == "mtu" {
				if len(out.mtu) == 0 {
					return nil, errors.Newf("`mtu` value without `label`: %s", value)
				}
				v, err := parseUint16(value)
				if err != nil {
					return nil, parseErr("mtu value", value, err)
				}
				last := &out.mtu[len(out.mtu)-1]
				last.values = append(last.values, v)
				continue
			}
			switch {
			case curModule == "tcp" && curDirection == "request":
				if len(out.tcpRequest) == 0 {
					return nil, errors.Newf("tcp signature without `label`: %s", value)
				}
				sig, err := parseTCPSignature(value)
				if err != nil {
					return nil, parseErr("tcp signature", value, err)
				}
				last := &out.tcpRequest[len(out.tcpRequest)-1]
				last.sigs = append(last.sigs, sig)
			case curModule == "tcp" && curDirection == "response":
				if len(out.tcpResponse) == 0 {
					return nil, errors.Newf("tcp signature without `label`: %s", value)
				}
				sig, err := parseTCPSignature(value)
				if err != nil {
					return nil, parseErr("tcp signature", value, err)
				}
				last := &out.tcpResponse[len(out.tcpResponse)-1]
				last.sigs = append(last.sigs, sig)
			case curModule == "http" && curDirection == "request":
				if len(out.httpRequest) == 0 {
					return nil, errors.Newf("http signature without `label`: %s", value)
				}
				sig, err := parseHTTPSignature(value)
				if err != nil {
					return nil, parseErr("http signature", value, err)
				}
				last := &out.httpRequest[len(out.httpRequest)-1]
				last.sigs = append(last.sigs, sig)
			case curModule == "http" && curDirection == "response":
				if len(out.httpResponse) == 0 {
					return nil, errors.Newf("http signature without `label`: %s", value)
				}
				sig, err := parseHTTPSignature(value)
				if err != nil {
					return nil, parseErr("http signature", value, err)
				}
				last := &out.httpResponse[len(out.httpResponse)-1]
				last.sigs = append(last.sigs, sig)
			default:
				// 未知 module 的 sig：跳过
			}
		case "sys":
			// `sys` 行（mtu 之外）无指纹语义，直接忽略
		default:
			// 其它未知命名值：忽略
		}
	}

	return out, nil
}

// trimSpace 移除前导空格与制表符（对应 nom 的 space0）。
func trimSpace(s string) string {
	return strings.TrimLeft(s, " \t")
}

// isASCIIAlnum 判断字节是否为 ASCII 字母或数字。
func isASCIIAlnum(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// isASCIIAlpha 判断字节是否为 ASCII 字母。
func isASCIIAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// isASCIIDigit 判断字节是否为 ASCII 数字。
func isASCIIDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// parseNamedValue 解析 `name = value` 行：名字为字母数字，值取行尾剩余全部内容
// （对应 Rust parse_named_value）。
func parseNamedValue(line string) (name, value string, err error) {
	i := 0
	for i < len(line) && isASCIIAlnum(line[i]) {
		i++
	}
	if i == 0 {
		return "", "", errors.Newf("parse named value failed: %s", line)
	}
	name = line[:i]
	rest := trimSpace(line[i:])
	if !strings.HasPrefix(rest, "=") {
		return "", "", errors.Newf("parse named value failed: %s", line)
	}
	rest = trimSpace(rest[1:])
	return name, rest, nil
}

// parseClasses 解析 `classes = a,b,c` 行（对应 Rust parse_classes）。
func parseClasses(line string) ([]string, error) {
	rest := strings.TrimPrefix(line, "classes")
	rest = trimSpace(rest)
	if !strings.HasPrefix(rest, "=") {
		return nil, errors.Newf("parse classes failed: %s", line)
	}
	rest = trimSpace(rest[1:])

	var classes []string
	for {
		item, r, ok := takeAlphanumeric(rest)
		if !ok {
			break
		}
		classes = append(classes, item)
		rest = r
		if !strings.HasPrefix(rest, ",") {
			break
		}
		rest = rest[1:]
	}
	return classes, nil
}

// takeAlphanumeric 消费连续的 ASCII 字母数字，返回内容与剩余输入。
func takeAlphanumeric(s string) (token, rest string, ok bool) {
	i := 0
	for i < len(s) && isASCIIAlnum(s[i]) {
		i++
	}
	if i == 0 {
		return "", s, false
	}
	return s[:i], s[i:], true
}

// parseModule 解析 `[module:direction]` 或 `[module]` 头（对应 Rust parse_module）。
func parseModule(line string) (module, direction string, err error) {
	rest := strings.TrimPrefix(line, "[")
	i := 0
	for i < len(rest) && isASCIIAlpha(rest[i]) {
		i++
	}
	if i == 0 {
		return "", "", errors.Newf("parse module failed: %s", line)
	}
	module = rest[:i]
	rest = rest[i:]
	if strings.HasPrefix(rest, ":") {
		rest = rest[1:]
		j := 0
		for j < len(rest) && isASCIIAlpha(rest[j]) {
			j++
		}
		if j == 0 {
			return "", "", errors.Newf("parse module direction failed: %s", line)
		}
		direction = rest[:j]
		rest = rest[j:]
	}
	if rest != "]" {
		return "", "", errors.Newf("parse module failed: %s", line)
	}
	return module, direction, nil
}

// parseType 解析 label 类型标记 s / g（对应 Rust parse_type）。
func parseType(s string) (labelType, string, error) {
	switch {
	case strings.HasPrefix(s, "s"):
		return labelSpecified, s[1:], nil
	case strings.HasPrefix(s, "g"):
		return labelGeneric, s[1:], nil
	default:
		return 0, "", errors.Newf("parse type failed: %s", s)
	}
}

// parseLabel 解析 `ty:class:name:flavor` label（对应 Rust parse_label）。
// class 为 `!` 时视为无类别；flavor 为空时省略。
func parseLabel(value string) (label, error) {
	ty, rest, err := parseType(value)
	if err != nil {
		return label{}, err
	}
	if !strings.HasPrefix(rest, ":") {
		return label{}, errors.Newf("parse label failed: %s", value)
	}
	rest = rest[1:]

	var class *string
	if strings.HasPrefix(rest, "!") {
		rest = rest[1:]
	} else {
		idx := strings.Index(rest, ":")
		if idx < 0 {
			return label{}, errors.Newf("parse label class failed: %s", value)
		}
		c := rest[:idx]
		class = &c
		rest = rest[idx:]
	}
	if !strings.HasPrefix(rest, ":") {
		return label{}, errors.Newf("parse label failed: %s", value)
	}
	rest = rest[1:]

	idx := strings.Index(rest, ":")
	if idx < 0 {
		return label{}, errors.Newf("parse label name failed: %s", value)
	}
	name := rest[:idx]
	rest = rest[idx:]

	var flavor *string
	if strings.HasPrefix(rest, ":") {
		f := rest[1:]
		if f != "" {
			flavor = &f
		}
	}
	return label{ty: ty, class: class, name: name, flavor: flavor}, nil
}

// parseUAOS 解析 `ua_os = A,B=[C],...` 行（对应 Rust parse_ua_os）。
// 复刻 nom separated_list0 语义：解析到无法继续的片段即截断，不报错。
func parseUAOS(line string) ([]uaOSEntry, error) {
	rest := strings.TrimPrefix(line, "ua_os")
	rest = trimSpace(rest)
	if !strings.HasPrefix(rest, "=") {
		return nil, errors.Newf("parse ua_os failed: %s", line)
	}
	rest = trimSpace(rest[1:])

	var out []uaOSEntry
	for {
		name, value, r, ok := parseKeyValue(rest)
		if !ok {
			break
		}
		var fam *string
		if value != "" {
			fam = &value
		}
		out = append(out, uaOSEntry{ua: name, family: fam})
		rest = r
		if !strings.HasPrefix(rest, ",") {
			break
		}
		rest = rest[1:]
	}
	return out, nil
}

// parseKeyValue 解析 `名字` 或 `名字=值`（名字与值均为字母数字；对应 Rust parse_key_value）。
// ok=false 表示当前位置无法解析出名字（列表解析在此截断）。
func parseKeyValue(s string) (name, value, rest string, ok bool) {
	token, r, ok := takeAlphanumeric(s)
	if !ok {
		return "", "", s, false
	}
	name = token
	rest = trimSpace(r)
	if strings.HasPrefix(rest, "=") {
		afterEq := trimSpace(rest[1:])
		if v, r2, ok2 := takeAlphanumeric(afterEq); ok2 {
			value = v
			rest = r2
		}
	}
	return name, value, rest, true
}

// parseIPVersion 解析 IP 版本标记 4 / 6 / *。
func parseIPVersion(s string) (ipVersion, error) {
	switch s {
	case "4":
		return ipV4, nil
	case "6":
		return ipV6, nil
	case "*":
		return ipAny, nil
	default:
		return 0, errors.Newf("parse ip version failed: %s", s)
	}
}

// parseUint8 解析无符号 8 位整数。
func parseUint8(s string) (uint8, error) {
	var v uint64
	if len(s) == 0 || len(s) > 3 {
		return 0, errors.Newf("parse u8 failed: %s", s)
	}
	for i := 0; i < len(s); i++ {
		if !isASCIIDigit(s[i]) {
			return 0, errors.Newf("parse u8 failed: %s", s)
		}
		v = v*10 + uint64(s[i]-'0')
	}
	if v > 255 {
		return 0, errors.Newf("parse u8 failed: %s", s)
	}
	return uint8(v), nil
}

// parseUint16 解析无符号 16 位整数。
func parseUint16(s string) (uint16, error) {
	var v uint64
	if len(s) == 0 || len(s) > 5 {
		return 0, errors.Newf("parse u16 failed: %s", s)
	}
	for i := 0; i < len(s); i++ {
		if !isASCIIDigit(s[i]) {
			return 0, errors.Newf("parse u16 failed: %s", s)
		}
		v = v*10 + uint64(s[i]-'0')
	}
	if v > 65535 {
		return 0, errors.Newf("parse u16 failed: %s", s)
	}
	return uint16(v), nil
}

// takeDigits 消费连续数字，返回内容与剩余输入。
func takeDigits(s string) (token, rest string, ok bool) {
	i := 0
	for i < len(s) && isASCIIDigit(s[i]) {
		i++
	}
	if i == 0 {
		return "", s, false
	}
	return s[:i], s[i:], true
}

// parseTTL 解析 TTL 标记：`64-` Bad / `64+?` Guess / `64+0` Distance / `64` Value
// （对应 Rust parse_ttl，顺序敏感）。
func parseTTL(s string) (ttl, error) {
	token, rest, ok := takeDigits(s)
	if !ok {
		return ttl{}, errors.Newf("parse ttl failed: %s", s)
	}
	v, err := parseUint8(token)
	if err != nil {
		return ttl{}, errors.Newf("parse ttl failed: %s", s)
	}
	switch {
	case rest == "-":
		return ttl{kind: ttlBad, first: v}, nil
	case strings.HasPrefix(rest, "+?"):
		return ttl{kind: ttlGuess, first: v}, nil
	case strings.HasPrefix(rest, "+"):
		dToken, r, ok := takeDigits(rest[1:])
		if !ok {
			return ttl{}, errors.Newf("parse ttl distance failed: %s", s)
		}
		d, err := parseUint8(dToken)
		if err != nil {
			return ttl{}, errors.Newf("parse ttl distance failed: %s", s)
		}
		if r != "" {
			return ttl{}, errors.Newf("parse ttl failed: %s", s)
		}
		return ttl{kind: ttlDistance, first: v, second: d}, nil
	case rest == "":
		return ttl{kind: ttlValue, first: v}, nil
	default:
		return ttl{}, errors.Newf("parse ttl failed: %s", s)
	}
}

// parseWindowSize 解析窗口大小标记（对应 Rust parse_window_size）。
func parseWindowSize(s string) (windowSize, error) {
	switch {
	case s == "*":
		return windowSize{kind: winAny}, nil
	case strings.HasPrefix(s, "mss*"):
		token, rest, ok := takeDigits(s[len("mss*"):])
		if !ok || rest != "" {
			return windowSize{}, errors.Newf("parse window size failed: %s", s)
		}
		v, err := parseUint8(token)
		if err != nil {
			return windowSize{}, errors.Newf("parse window size failed: %s", s)
		}
		return windowSize{kind: winMss, value: uint16(v)}, nil
	case strings.HasPrefix(s, "mtu*"):
		token, rest, ok := takeDigits(s[len("mtu*"):])
		if !ok || rest != "" {
			return windowSize{}, errors.Newf("parse window size failed: %s", s)
		}
		v, err := parseUint8(token)
		if err != nil {
			return windowSize{}, errors.Newf("parse window size failed: %s", s)
		}
		return windowSize{kind: winMtu, value: uint16(v)}, nil
	case strings.HasPrefix(s, "%"):
		token, rest, ok := takeDigits(s[1:])
		if !ok || rest != "" {
			return windowSize{}, errors.Newf("parse window size failed: %s", s)
		}
		v, err := parseUint16(token)
		if err != nil {
			return windowSize{}, errors.Newf("parse window size failed: %s", s)
		}
		return windowSize{kind: winMod, value: v}, nil
	default:
		v, err := parseUint16(s)
		if err != nil {
			return windowSize{}, errors.Newf("parse window size failed: %s", s)
		}
		return windowSize{kind: winValue, value: v}, nil
	}
}

// parseTCPOption 解析单个 TCP 选项标记（对应 Rust parse_tcp_option，顺序敏感）。
func parseTCPOption(s string) (tcpOption, error) {
	switch {
	case strings.HasPrefix(s, "eol+"):
		token, rest, ok := takeDigits(s[len("eol+"):])
		if !ok || rest != "" {
			return tcpOption{}, errors.Newf("parse tcp option failed: %s", s)
		}
		v, err := parseUint8(token)
		if err != nil {
			return tcpOption{}, errors.Newf("parse tcp option failed: %s", s)
		}
		return tcpOption{kind: optEol, num: v}, nil
	case s == "nop":
		return tcpOption{kind: optNop}, nil
	case s == "mss":
		return tcpOption{kind: optMss}, nil
	case s == "ws":
		return tcpOption{kind: optWs}, nil
	case s == "sok":
		return tcpOption{kind: optSok}, nil
	case s == "sack":
		return tcpOption{kind: optSack}, nil
	case s == "ts":
		return tcpOption{kind: optTS}, nil
	case strings.HasPrefix(s, "?"):
		token, rest, ok := takeDigits(s[1:])
		if !ok || rest != "" {
			return tcpOption{}, errors.Newf("parse tcp option failed: %s", s)
		}
		v, err := parseUint8(token)
		if err != nil {
			return tcpOption{}, errors.Newf("parse tcp option failed: %s", s)
		}
		return tcpOption{kind: optUnknown, num: v}, nil
	default:
		return tcpOption{}, errors.Newf("parse tcp option failed: %s", s)
	}
}

// parseQuirk 解析单个协议怪癖标记（对应 Rust parse_quirk）。
func parseQuirk(s string) (quirk, error) {
	switch s {
	case "df":
		return quirkDf, nil
	case "id+":
		return quirkNonZeroID, nil
	case "id-":
		return quirkZeroID, nil
	case "ecn":
		return quirkEcn, nil
	case "0+":
		return quirkMustBeZero, nil
	case "flow":
		return quirkFlowID, nil
	case "seq-":
		return quirkSeqNumZero, nil
	case "ack+":
		return quirkAckNumNonZero, nil
	case "ack-":
		return quirkAckNumZero, nil
	case "uptr+":
		return quirkNonZeroURG, nil
	case "urgf+":
		return quirkUrg, nil
	case "pushf+":
		return quirkPush, nil
	case "ts1-":
		return quirkOwnTimestampZero, nil
	case "ts2+":
		return quirkPeerTimestampNonZero, nil
	case "opt+":
		return quirkTrailingNonZero, nil
	case "exws":
		return quirkExcessiveWindowScaling, nil
	case "bad":
		return quirkOptBad, nil
	default:
		return 0, errors.Newf("parse quirk failed: %s", s)
	}
}

// parsePayloadSize 解析载荷大小分类 0 / + / *。
func parsePayloadSize(s string) (payloadSize, error) {
	switch s {
	case "0":
		return payloadZero, nil
	case "+":
		return payloadNonZero, nil
	case "*":
		return payloadAny, nil
	default:
		return 0, errors.Newf("parse payload size failed: %s", s)
	}
}

// parseTCPSignature 解析 TCP 签名 `version:ittl:olen:mss:wsize,wscale:olayout:quirks:pclass`
// （对应 Rust parse_tcp_signature）。
func parseTCPSignature(value string) (tcpSignature, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 8 {
		return tcpSignature{}, errors.Newf("parse tcp signature failed: %s", value)
	}

	version, err := parseIPVersion(parts[0])
	if err != nil {
		return tcpSignature{}, err
	}
	ittl, err := parseTTL(parts[1])
	if err != nil {
		return tcpSignature{}, err
	}
	olen, err := parseUint8(parts[2])
	if err != nil {
		return tcpSignature{}, err
	}
	var mss *uint16
	if parts[3] != "*" {
		v, err := parseUint16(parts[3])
		if err != nil {
			return tcpSignature{}, err
		}
		mss = &v
	}
	winParts := strings.SplitN(parts[4], ",", 2)
	if len(winParts) != 2 {
		return tcpSignature{}, errors.Newf("parse window pair failed: %s", parts[4])
	}
	wsize, err := parseWindowSize(winParts[0])
	if err != nil {
		return tcpSignature{}, err
	}
	var wscale *uint8
	if winParts[1] != "*" {
		v, err := parseUint8(winParts[1])
		if err != nil {
			return tcpSignature{}, err
		}
		wscale = &v
	}

	olayout := make([]tcpOption, 0, 4)
	if parts[5] != "" {
		for _, token := range strings.Split(parts[5], ",") {
			opt, err := parseTCPOption(token)
			if err != nil {
				return tcpSignature{}, err
			}
			olayout = append(olayout, opt)
		}
	}
	if len(olayout) == 0 {
		return tcpSignature{}, errors.Newf("parse tcp signature olayout empty: %s", value)
	}

	quirks := make([]quirk, 0, 4)
	if parts[6] != "" {
		for _, token := range strings.Split(parts[6], ",") {
			q, err := parseQuirk(token)
			if err != nil {
				return tcpSignature{}, err
			}
			quirks = append(quirks, q)
		}
	}

	pclass, err := parsePayloadSize(parts[7])
	if err != nil {
		return tcpSignature{}, err
	}

	return tcpSignature{
		version: version,
		ittl:    ittl,
		olen:    olen,
		mss:     mss,
		wsize:   wsize,
		wscale:  wscale,
		olayout: olayout,
		quirks:  quirks,
		pclass:  pclass,
	}, nil
}

// parseHTTPVersion 解析 HTTP 版本标记 0 / 1 / *（对应 Rust parse_http_version）。
func parseHTTPVersion(s string) (httpVersion, error) {
	switch s {
	case "0":
		return httpV10, nil
	case "1":
		return httpV11, nil
	case "*":
		return httpAny, nil
	default:
		return 0, errors.Newf("parse http version failed: %s", s)
	}
}

// parseHeaderKeyValue 解析头名与可选 `=[值]`（对应 Rust parse_header_key_value）。
// 头名为 ASCII 字母数字或连字符；值以 `]` 终止，找不到 `]` 时不消费 `=[` 前缀。
func parseHeaderKeyValue(s string) (name string, value *string, rest string) {
	i := 0
	for i < len(s) && (isASCIIAlnum(s[i]) || s[i] == '-') {
		i++
	}
	name = s[:i]
	rest = s[i:]
	if strings.HasPrefix(rest, "=[") {
		if idx := strings.Index(rest[2:], "]"); idx >= 0 {
			v := rest[2 : 2+idx]
			value = &v
			rest = rest[2+idx+1:]
		}
	}
	return name, value, rest
}

// parseHTTPHeader 解析单个 HTTP 签名头（对应 Rust parse_http_header）。
// 返回解析出的头与剩余输入；头名可能为空（对应 take_while 空匹配，如紧随 `=` 的输入）。
func parseHTTPHeader(s string) (httpHeader, string) {
	rest := s
	optional := false
	if strings.HasPrefix(rest, "?") {
		optional = true
		rest = rest[1:]
	}
	name, value, r := parseHeaderKeyValue(rest)
	return httpHeader{optional: optional, name: name, value: value}, r
}

// parseHTTPSignature 解析 HTTP 签名 `version:horder:habsent:expsw`
// （对应 Rust parse_http_signature；habsent 中空名字被过滤）。
func parseHTTPSignature(value string) (httpSignature, error) {
	parts := strings.SplitN(value, ":", 4)
	if len(parts) != 4 {
		return httpSignature{}, errors.Newf("parse http signature failed: %s", value)
	}

	version, err := parseHTTPVersion(parts[0])
	if err != nil {
		return httpSignature{}, err
	}

	horder := make([]httpHeader, 0, 4)
	if parts[1] != "" {
		rest := parts[1]
		for {
			h, r := parseHTTPHeader(rest)
			horder = append(horder, h)
			rest = r
			if !strings.HasPrefix(rest, ",") {
				break
			}
			rest = rest[1:]
		}
	}
	if len(horder) == 0 {
		return httpSignature{}, errors.Newf("parse http signature horder empty: %s", value)
	}

	habsent := make([]httpHeader, 0, 4)
	if parts[2] != "" {
		rest := parts[2]
		for {
			h, r := parseHTTPHeader(rest)
			if h.name != "" {
				habsent = append(habsent, h)
			}
			rest = r
			if !strings.HasPrefix(rest, ",") {
				break
			}
			rest = rest[1:]
		}
	}

	return httpSignature{version: version, horder: horder, habsent: habsent, expsw: parts[3]}, nil
}
