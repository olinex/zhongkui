package huginn

import "strings"

// 距离打分：TCP 9 维 + HTTP 4 维。
//
// 对应 huginn-net-db 的 tcp::distances / tcp::matching 与 http::distances /
// http::matching。每维返回 *uint32：nil 表示维度不兼容（整体判为不匹配），
// 否则为 0（High）/ 1（Medium）/ 2（Low）/ 3（Bad）加权分值。

// tcpMaxDistance TCP 距离上限：9 维 × 每维最高 2 分。
const tcpMaxDistance = 18

// httpMaxDistance HTTP 距离上限：4 维 × 每维最高 3 分。
const httpMaxDistance = 12

// satAdd 无符号饱和加法（对应 Rust saturating_add）。
func satAdd(a, b uint32) uint32 {
	if a > ^uint32(0)-b {
		return ^uint32(0)
	}
	return a + b
}

// distanceIPVersion 观测版本与签名版本的距离（对应 distance_ip_version）。
func distanceIPVersion(observed, signature ipVersion) *uint32 {
	if signature == ipAny {
		return ptrU32(0)
	}
	if observed == signature {
		return ptrU32(0)
	}
	return nil
}

// distanceTTL 观测 TTL 与签名 TTL 的距离（对应 distance_ttl）。
// 支持 Value / Distance / Guess / Bad 四种表示间的交叉比较。
func distanceTTL(observed, signature ttl) *uint32 {
	switch {
	case observed.kind == ttlValue && signature.kind == ttlValue:
		if observed.first == signature.first {
			return ptrU32(0)
		}
		return ptrU32(2)
	case observed.kind == ttlDistance && signature.kind == ttlDistance:
		if observed.first == signature.first && observed.second == signature.second {
			return ptrU32(0)
		}
		return ptrU32(2)
	case observed.kind == ttlDistance && signature.kind == ttlValue:
		if uint16(observed.first)+uint16(observed.second) == uint16(signature.first) {
			return ptrU32(0)
		}
		return ptrU32(2)
	case observed.kind == ttlGuess && signature.kind == ttlGuess:
		if observed.first == signature.first {
			return ptrU32(0)
		}
		return ptrU32(2)
	case observed.kind == ttlBad && signature.kind == ttlBad:
		if observed.first == signature.first {
			return ptrU32(0)
		}
		return ptrU32(2)
	case observed.kind == ttlGuess && signature.kind == ttlValue:
		if observed.first == signature.first {
			return ptrU32(0)
		}
		return ptrU32(2)
	case observed.kind == ttlValue && signature.kind == ttlDistance:
		if uint16(observed.first) == uint16(signature.first)+uint16(signature.second) {
			return ptrU32(0)
		}
		return ptrU32(2)
	case observed.kind == ttlValue && signature.kind == ttlGuess:
		if observed.first == signature.first {
			return ptrU32(0)
		}
		return ptrU32(2)
	default:
		return nil
	}
}

// distanceOlen 观测 IPv4 选项 / IPv6 扩展头长度与签名的距离（对应 distance_olen）。
func distanceOlen(observed uint8, signature *tcpSignature) *uint32 {
	if observed == signature.olen {
		return ptrU32(0)
	}
	return ptrU32(2)
}

// distanceMSS 观测 MSS 与签名 MSS 的距离（对应 distance_mss）。
// 签名 MSS 为通配（nil）时恒为匹配。
func distanceMSS(observed *tcpObservation, signature *tcpSignature) *uint32 {
	if signature.mss == nil || (observed.mss != nil && *observed.mss == *signature.mss) {
		return ptrU32(0)
	}
	return ptrU32(2)
}

// distanceWindowSize 观测窗口大小与签名窗口大小的距离（对应 distance_window_size）。
// mss 为观测 MSS，用于把 Value 观测与 Mss 签名做整除比比较。
func distanceWindowSize(observed, signature windowSize, mss *uint16) *uint32 {
	switch {
	case observed.kind == winMss && signature.kind == winMss:
		if observed.value == signature.value {
			return ptrU32(0)
		}
		return ptrU32(2)
	case observed.kind == winMtu && signature.kind == winMtu:
		if observed.value == signature.value {
			return ptrU32(0)
		}
		return ptrU32(2)
	case observed.kind == winValue && signature.kind == winMss:
		if mss != nil && *mss != 0 {
			if uint16(observed.value) / *mss == signature.value {
				return ptrU32(0)
			}
			return ptrU32(2)
		}
		return ptrU32(2)
	case observed.kind == winMod && signature.kind == winMod:
		if observed.value == signature.value {
			return ptrU32(0)
		}
		return ptrU32(2)
	case observed.kind == winValue && signature.kind == winValue:
		if observed.value == signature.value {
			return ptrU32(0)
		}
		return ptrU32(2)
	case signature.kind == winAny:
		return ptrU32(0)
	default:
		return nil
	}
}

// distanceWscale 观测窗口缩放因子与签名的距离（对应 distance_wscale）。
// 签名 wscale 为通配（nil）时恒为匹配；不匹配仅计 Medium（1）。
func distanceWscale(observed *tcpObservation, signature *tcpSignature) *uint32 {
	if signature.wscale == nil || (observed.wscale != nil && *observed.wscale == *signature.wscale) {
		return ptrU32(0)
	}
	return ptrU32(1)
}

// distanceOlayout 观测选项布局与签名的距离（对应 distance_olayout）。
// 布局必须精确一致，否则整条签名判为不匹配。
func distanceOlayout(observed, signature []tcpOption) *uint32 {
	if len(observed) == len(signature) {
		equal := true
		for i := range observed {
			if observed[i] != signature[i] {
				equal = false
				break
			}
		}
		if equal {
			return ptrU32(0)
		}
	}
	return nil
}

// distanceQuirks 观测怪癖集与签名的距离（对应 distance_quirks）。
// 怪癖集必须精确一致，否则整条签名判为不匹配。
func distanceQuirks(observed, signature []quirk) *uint32 {
	if len(observed) == len(signature) {
		equal := true
		for i := range observed {
			if observed[i] != signature[i] {
				equal = false
				break
			}
		}
		if equal {
			return ptrU32(0)
		}
	}
	return nil
}

// distancePayloadSize 观测载荷分类与签名的距离（对应 distance_payload_size）。
func distancePayloadSize(observed, signature payloadSize) *uint32 {
	if signature == payloadAny || observed == signature {
		return ptrU32(0)
	}
	return nil
}

// calculateTCPDistance 计算观测 TCP 指纹与签名条目的九维总距离（对应 calculate_distance）。
// 任一维度不兼容（nil）时返回 nil。
func calculateTCPDistance(observed *tcpObservation, signature *tcpSignature) *uint32 {
	d := distanceIPVersion(observed.version, signature.version)
	if d == nil {
		return nil
	}
	t := distanceTTL(observed.ittl, signature.ittl)
	if t == nil {
		return nil
	}
	o := distanceOlen(observed.olen, signature)
	m := distanceMSS(observed, signature)
	w := distanceWindowSize(observed.wsize, signature.wsize, observed.mss)
	if w == nil {
		return nil
	}
	ws := distanceWscale(observed, signature)
	ol := distanceOlayout(observed.olayout, signature.olayout)
	if ol == nil {
		return nil
	}
	q := distanceQuirks(observed.quirks, signature.quirks)
	if q == nil {
		return nil
	}
	p := distancePayloadSize(observed.pclass, signature.pclass)
	if p == nil {
		return nil
	}
	total := uint32(0)
	for _, part := range []uint32{*d, *t, *o, *m, *w, *ws, *ol, *q, *p} {
		total = satAdd(total, part)
	}
	return &total
}

// tcpDistanceToScore 将 TCP 距离映射为 0.0~1.0 质量分（对应 TcpMatchQuality::distance_to_score）。
func tcpDistanceToScore(distance uint32) float64 {
	switch {
	case distance == 0:
		return 1.0
	case distance == 1:
		return 0.95
	case distance == 2:
		return 0.90
	case distance >= 3 && distance <= 4:
		return 0.80
	case distance >= 5 && distance <= 6:
		return 0.70
	case distance >= 7 && distance <= 9:
		return 0.60
	case distance >= 10 && distance <= 12:
		return 0.40
	case distance >= 13 && distance <= 15:
		return 0.20
	case distance <= tcpMaxDistance:
		return 0.10
	default:
		return 0.05
	}
}

// distanceHTTPVersion 观测 HTTP 版本与签名版本的距离（对应 distance_http_version）。
func distanceHTTPVersion(observed, signature httpVersion) *uint32 {
	if signature == httpAny || observed == signature {
		return ptrU32(0)
	}
	return nil
}

// httpHeaderValueEqual 比较两个可选头值是否相等（None 与 None 相等）。
func httpHeaderValueEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// distanceHeader 计算两个头列表的距离（对应 distance_header）。
// 双指针遍历：精确匹配推进双方；同名不同值仅在签名头可选时容忍；
// 签名头名不匹配时可选头跳过、否则计错；观测头或签名头多余部分计错。
// 错误数 >= 12 时返回 nil（不可匹配）。
func distanceHeader(observed, signature []httpHeader) *uint32 {
	obsIdx, sigIdx := 0, 0
	var errors uint32

	for obsIdx < len(observed) && sigIdx < len(signature) {
		obsHeader := observed[obsIdx]
		sigHeader := signature[sigIdx]
		switch {
		case obsHeader.name == sigHeader.name && httpHeaderValueEqual(obsHeader.value, sigHeader.value):
			obsIdx++
			sigIdx++
		case obsHeader.name == sigHeader.name:
			if !sigHeader.optional {
				errors = satAdd(errors, 1)
			}
			obsIdx++
			sigIdx++
		case sigHeader.optional:
			sigIdx++
		default:
			errors = satAdd(errors, 1)
			sigIdx++
		}
	}

	for obsIdx < len(observed) {
		errors = satAdd(errors, 1)
		obsIdx++
	}

	for sigIdx < len(signature) {
		if !signature[sigIdx].optional {
			errors = satAdd(errors, 1)
		}
		sigIdx++
	}

	switch {
	case errors >= 0 && errors <= 2:
		return ptrU32(0)
	case errors >= 3 && errors <= 5:
		return ptrU32(1)
	case errors >= 6 && errors <= 8:
		return ptrU32(2)
	case errors >= 9 && errors <= 11:
		return ptrU32(3)
	default:
		return nil
	}
}

// distanceExpsw 观测 expsw 串与签名期望子串的距离（对应 distance_expsw）。
// 本地补丁：按 p0f 语义判定 `observed.contains(signature)`（上游方向反转已修正）。
// 不匹配仍返回分值（Bad=3）而非 nil。
func distanceExpsw(observed, signature string) *uint32 {
	if strings.Contains(observed, signature) {
		return ptrU32(0)
	}
	return ptrU32(3)
}

// calculateHTTPDistance 计算观测 HTTP 指纹与签名条目的四维总距离
// （对应 HttpSignatureHelper::calculate_http_distance）。
// 任一维度不兼容（nil）时返回 nil。
func calculateHTTPDistance(observed *httpObservation, signature *httpSignature) *uint32 {
	v := distanceHTTPVersion(observed.version, signature.version)
	if v == nil {
		return nil
	}
	h := distanceHeader(observed.horder, signature.horder)
	if h == nil {
		return nil
	}
	a := distanceHeader(observed.habsent, signature.habsent)
	if a == nil {
		return nil
	}
	e := distanceExpsw(observed.expsw, signature.expsw)
	total := uint32(0)
	for _, part := range []uint32{*v, *h, *a, *e} {
		total = satAdd(total, part)
	}
	return &total
}

// httpDistanceToScore 将 HTTP 距离映射为 0.0~1.0 质量分
// （对应 HttpMatchQuality::distance_to_score）。
func httpDistanceToScore(distance uint32) float64 {
	switch {
	case distance == 0:
		return 1.0
	case distance == 1:
		return 0.95
	case distance == 2:
		return 0.90
	case distance == 3:
		return 0.80
	case distance >= 4 && distance <= 5:
		return 0.70
	case distance >= 6 && distance <= 7:
		return 0.60
	case distance >= 8 && distance <= 9:
		return 0.40
	case distance >= 10 && distance <= 11:
		return 0.20
	case distance <= httpMaxDistance:
		return 0.10
	default:
		return 0.05
	}
}

// ptrU32 返回 uint32 指针，用于距离函数返回值。
func ptrU32(v uint32) *uint32 {
	return &v
}
