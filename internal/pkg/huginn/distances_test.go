// 距离打分单元测试：distances.go。
//
// 覆盖 TCP 9 维与 HTTP 4 维距离计算、各表示形式的交叉比较、错误带映射
// 与饱和加法。
package huginn

import "testing"

func ptr16(v uint16) *uint16 { return &v }
func ptr8(v uint8) *uint8    { return &v }

// u32Eq 比较 *uint32（nil 视为不匹配）。
func u32Eq(got *uint32, want uint32) bool {
	return got != nil && *got == want
}

func TestSatAdd(t *testing.T) {
	if got := satAdd(10, 5); got != 15 {
		t.Errorf("satAdd(10,5) = %d", got)
	}
	if got := satAdd(^uint32(0), 1); got != ^uint32(0) {
		t.Errorf("satAdd(max,1) = %d, want max", got)
	}
	if got := satAdd(^uint32(0), 0); got != ^uint32(0) {
		t.Errorf("satAdd(max,0) = %d", got)
	}
	if got := satAdd(0, 0); got != 0 {
		t.Errorf("satAdd(0,0) = %d", got)
	}
}

func TestPtrU32(t *testing.T) {
	p := ptrU32(7)
	if p == nil || *p != 7 {
		t.Errorf("ptrU32 = %v", p)
	}
}

func TestDistanceIPVersion(t *testing.T) {
	if d := distanceIPVersion(ipV4, ipV4); !u32Eq(d, 0) {
		t.Errorf("v4/v4 = %v", d)
	}
	if d := distanceIPVersion(ipV4, ipV6); d != nil {
		t.Errorf("v4/v6 = %v, want nil", d)
	}
	if d := distanceIPVersion(ipV4, ipAny); !u32Eq(d, 0) {
		t.Errorf("v4/any = %v", d)
	}
	if d := distanceIPVersion(ipV6, ipV6); !u32Eq(d, 0) {
		t.Errorf("v6/v6 = %v", d)
	}
}

func TestDistanceTTL(t *testing.T) {
	cases := []struct {
		name     string
		obs, sig ttl
		want     uint32
		nilMatch bool
	}{
		{"value equal", ttl{kind: ttlValue, first: 64}, ttl{kind: ttlValue, first: 64}, 0, false},
		{"value differ", ttl{kind: ttlValue, first: 64}, ttl{kind: ttlValue, first: 128}, 2, false},
		{"distance equal", ttl{kind: ttlDistance, first: 64}, ttl{kind: ttlDistance, first: 64}, 0, false},
		{"distance differ", ttl{kind: ttlDistance, first: 64}, ttl{kind: ttlDistance, first: 63, second: 1}, 2, false},
		{"distance vs value hit", ttl{kind: ttlDistance, first: 64}, ttl{kind: ttlValue, first: 64}, 0, false},
		{"distance vs value miss", ttl{kind: ttlDistance, first: 64, second: 1}, ttl{kind: ttlValue, first: 64}, 2, false},
		{"guess equal", ttl{kind: ttlGuess, first: 64}, ttl{kind: ttlGuess, first: 64}, 0, false},
		{"guess differ", ttl{kind: ttlGuess, first: 64}, ttl{kind: ttlGuess, first: 128}, 2, false},
		{"bad equal", ttl{kind: ttlBad, first: 0}, ttl{kind: ttlBad, first: 0}, 0, false},
		{"bad differ", ttl{kind: ttlBad, first: 0}, ttl{kind: ttlBad, first: 1}, 2, false},
		{"guess vs value hit", ttl{kind: ttlGuess, first: 64}, ttl{kind: ttlValue, first: 64}, 0, false},
		{"guess vs value miss", ttl{kind: ttlGuess, first: 64}, ttl{kind: ttlValue, first: 128}, 2, false},
		{"value vs distance hit", ttl{kind: ttlValue, first: 64}, ttl{kind: ttlDistance, first: 64}, 0, false},
		{"value vs distance miss", ttl{kind: ttlValue, first: 64}, ttl{kind: ttlDistance, first: 60, second: 1}, 2, false},
		{"value vs guess hit", ttl{kind: ttlValue, first: 64}, ttl{kind: ttlGuess, first: 64}, 0, false},
		{"value vs guess miss", ttl{kind: ttlValue, first: 64}, ttl{kind: ttlGuess, first: 128}, 2, false},
		{"value vs bad", ttl{kind: ttlValue, first: 64}, ttl{kind: ttlBad, first: 0}, 0, true},
		{"distance vs bad", ttl{kind: ttlDistance, first: 64}, ttl{kind: ttlBad, first: 0}, 0, true},
		{"guess vs bad", ttl{kind: ttlGuess, first: 64}, ttl{kind: ttlBad, first: 0}, 0, true},
	}
	for _, tc := range cases {
		got := distanceTTL(tc.obs, tc.sig)
		if tc.nilMatch {
			if got != nil {
				t.Errorf("%s: got %v, want nil", tc.name, got)
			}
			continue
		}
		if !u32Eq(got, tc.want) {
			t.Errorf("%s: got %v, want %d", tc.name, got, tc.want)
		}
	}
}

func TestDistanceOlen(t *testing.T) {
	sig := &tcpSignature{olen: 0}
	if d := distanceOlen(0, sig); !u32Eq(d, 0) {
		t.Errorf("0/0 = %v", d)
	}
	if d := distanceOlen(4, sig); !u32Eq(d, 2) {
		t.Errorf("4/0 = %v", d)
	}
}

func TestDistanceMSS(t *testing.T) {
	obs := &tcpObservation{mss: ptr16(1460)}
	// 签名 MSS 通配
	if d := distanceMSS(obs, &tcpSignature{}); !u32Eq(d, 0) {
		t.Errorf("wildcard = %v", d)
	}
	// 相等
	if d := distanceMSS(obs, &tcpSignature{mss: ptr16(1460)}); !u32Eq(d, 0) {
		t.Errorf("equal = %v", d)
	}
	// 不等
	if d := distanceMSS(obs, &tcpSignature{mss: ptr16(1000)}); !u32Eq(d, 2) {
		t.Errorf("differ = %v", d)
	}
	// 观测无 MSS 选项
	if d := distanceMSS(&tcpObservation{}, &tcpSignature{mss: ptr16(1460)}); !u32Eq(d, 2) {
		t.Errorf("obs nil = %v", d)
	}
}

func TestDistanceWindowSize(t *testing.T) {
	mss := ptr16(1460)
	cases := []struct {
		name     string
		obs, sig windowSize
		obsMSS   *uint16
		want     uint32
		wantNil  bool
	}{
		{"mss equal", windowSize{kind: winMss, value: 10}, windowSize{kind: winMss, value: 10}, nil, 0, false},
		{"mss differ", windowSize{kind: winMss, value: 10}, windowSize{kind: winMss, value: 5}, nil, 2, false},
		{"mtu equal", windowSize{kind: winMtu, value: 2}, windowSize{kind: winMtu, value: 2}, nil, 0, false},
		{"mtu differ", windowSize{kind: winMtu, value: 2}, windowSize{kind: winMtu, value: 3}, nil, 2, false},
		{"value vs mss hit", windowSize{kind: winValue, value: 14600}, windowSize{kind: winMss, value: 10}, mss, 0, false},
		{"value vs mss miss", windowSize{kind: winValue, value: 5000}, windowSize{kind: winMss, value: 10}, mss, 2, false},
		{"value vs mss no mss", windowSize{kind: winValue, value: 14600}, windowSize{kind: winMss, value: 10}, nil, 2, false},
		{"mod equal", windowSize{kind: winMod, value: 1024}, windowSize{kind: winMod, value: 1024}, nil, 0, false},
		{"mod differ", windowSize{kind: winMod, value: 1024}, windowSize{kind: winMod, value: 512}, nil, 2, false},
		{"value equal", windowSize{kind: winValue, value: 65535}, windowSize{kind: winValue, value: 65535}, nil, 0, false},
		{"value differ", windowSize{kind: winValue, value: 65535}, windowSize{kind: winValue, value: 1000}, nil, 2, false},
		{"sig any", windowSize{kind: winMss, value: 10}, windowSize{kind: winAny}, nil, 0, false},
		{"incompatible", windowSize{kind: winMss, value: 10}, windowSize{kind: winValue, value: 1000}, nil, 0, true},
	}
	for _, tc := range cases {
		got := distanceWindowSize(tc.obs, tc.sig, tc.obsMSS)
		if tc.wantNil {
			if got != nil {
				t.Errorf("%s: got %v, want nil", tc.name, got)
			}
			continue
		}
		if !u32Eq(got, tc.want) {
			t.Errorf("%s: got %v, want %d", tc.name, got, tc.want)
		}
	}
}

func TestDistanceWscale(t *testing.T) {
	obs := &tcpObservation{wscale: ptr8(6)}
	if d := distanceWscale(obs, &tcpSignature{}); !u32Eq(d, 0) {
		t.Errorf("wildcard = %v", d)
	}
	if d := distanceWscale(obs, &tcpSignature{wscale: ptr8(6)}); !u32Eq(d, 0) {
		t.Errorf("equal = %v", d)
	}
	if d := distanceWscale(obs, &tcpSignature{wscale: ptr8(7)}); !u32Eq(d, 1) {
		t.Errorf("differ = %v, want 1", d)
	}
	if d := distanceWscale(&tcpObservation{}, &tcpSignature{wscale: ptr8(6)}); !u32Eq(d, 1) {
		t.Errorf("obs nil = %v, want 1", d)
	}
}

func TestDistanceOlayout(t *testing.T) {
	a := []tcpOption{{kind: optMss}, {kind: optSok}}
	if d := distanceOlayout(a, a); !u32Eq(d, 0) {
		t.Errorf("equal = %v", d)
	}
	if d := distanceOlayout(a, []tcpOption{{kind: optMss}}); d != nil {
		t.Errorf("len differ = %v, want nil", d)
	}
	if d := distanceOlayout(a, []tcpOption{{kind: optMss}, {kind: optTS}}); d != nil {
		t.Errorf("same len differ = %v, want nil", d)
	}
	if d := distanceOlayout(nil, nil); !u32Eq(d, 0) {
		t.Errorf("empty = %v", d)
	}
}

func TestDistanceQuirks(t *testing.T) {
	a := []quirk{quirkDf, quirkNonZeroID}
	if d := distanceQuirks(a, a); !u32Eq(d, 0) {
		t.Errorf("equal = %v", d)
	}
	if d := distanceQuirks(a, []quirk{quirkDf}); d != nil {
		t.Errorf("len differ = %v, want nil", d)
	}
	if d := distanceQuirks(a, []quirk{quirkDf, quirkZeroID}); d != nil {
		t.Errorf("same len differ = %v, want nil", d)
	}
}

func TestDistancePayloadSize(t *testing.T) {
	if d := distancePayloadSize(payloadZero, payloadAny); !u32Eq(d, 0) {
		t.Errorf("any = %v", d)
	}
	if d := distancePayloadSize(payloadZero, payloadZero); !u32Eq(d, 0) {
		t.Errorf("equal = %v", d)
	}
	if d := distancePayloadSize(payloadZero, payloadNonZero); d != nil {
		t.Errorf("differ = %v, want nil", d)
	}
}

// linuxSignature 与 p0f.fp 中 Linux 3.1-3.10 的 TCP 签名一致，用于构造匹配观测。
func linuxSignature() tcpSignature {
	return tcpSignature{
		version: ipV4,
		ittl:    ttl{kind: ttlDistance, first: 64},
		olen:    0,
		mss:     ptr16(1460),
		wsize:   windowSize{kind: winMss, value: 10},
		wscale:  ptr8(6),
		olayout: []tcpOption{{kind: optMss}, {kind: optSok}, {kind: optTS}, {kind: optNop}, {kind: optWs}},
		quirks:  []quirk{quirkDf, quirkNonZeroID},
		pclass:  payloadZero,
	}
}

func TestCalculateTCPDistance(t *testing.T) {
	// 完全匹配 → 0
	obs := &tcpObservation{
		version: ipV4,
		ittl:    ttl{kind: ttlDistance, first: 64},
		olen:    0,
		mss:     ptr16(1460),
		wsize:   windowSize{kind: winMss, value: 10},
		wscale:  ptr8(6),
		olayout: []tcpOption{{kind: optMss}, {kind: optSok}, {kind: optTS}, {kind: optNop}, {kind: optWs}},
		quirks:  []quirk{quirkDf, quirkNonZeroID},
		pclass:  payloadZero,
	}
	sig := linuxSignature()
	if d := calculateTCPDistance(obs, &sig); !u32Eq(d, 0) {
		t.Errorf("exact = %v, want 0", d)
	}

	// 版本不兼容 → nil
	bad := *obs
	bad.version = ipV6
	if d := calculateTCPDistance(&bad, &sig); d != nil {
		t.Errorf("version mismatch = %v, want nil", d)
	}

	// 怪癖不一致 → nil
	bad = *obs
	bad.quirks = []quirk{quirkDf}
	if d := calculateTCPDistance(&bad, &sig); d != nil {
		t.Errorf("quirks mismatch = %v, want nil", d)
	}

	// TTL 偏移 → 2 分
	bad = *obs
	bad.ittl = ttl{kind: ttlDistance, first: 63, second: 1}
	if d := calculateTCPDistance(&bad, &sig); !u32Eq(d, 2) {
		t.Errorf("ttl differ = %v, want 2", d)
	}

	// MSS 缺失 → 2 分
	bad = *obs
	bad.mss = nil
	if d := calculateTCPDistance(&bad, &sig); !u32Eq(d, 2) {
		t.Errorf("mss missing = %v, want 2", d)
	}

	// 窗口不兼容（winMod vs winMss）→ nil
	bad = *obs
	bad.wsize = windowSize{kind: winMod, value: 1024}
	if d := calculateTCPDistance(&bad, &sig); d != nil {
		t.Errorf("wsize incompatible = %v, want nil", d)
	}

	// 载荷分类不一致 → nil
	bad = *obs
	bad.pclass = payloadNonZero
	if d := calculateTCPDistance(&bad, &sig); d != nil {
		t.Errorf("pclass mismatch = %v, want nil", d)
	}
}

func TestTcpDistanceToScore(t *testing.T) {
	cases := []struct {
		d    uint32
		want float64
	}{
		{0, 1.0}, {1, 0.95}, {2, 0.90}, {3, 0.80}, {4, 0.80},
		{5, 0.70}, {6, 0.70}, {7, 0.60}, {9, 0.60}, {10, 0.40},
		{12, 0.40}, {13, 0.20}, {15, 0.20}, {16, 0.10}, {18, 0.10},
		{19, 0.05}, {100, 0.05},
	}
	for _, tc := range cases {
		if got := tcpDistanceToScore(tc.d); got != tc.want {
			t.Errorf("tcpDistanceToScore(%d) = %v, want %v", tc.d, got, tc.want)
		}
	}
}

func TestDistanceHTTPVersion(t *testing.T) {
	if d := distanceHTTPVersion(httpV11, httpV11); !u32Eq(d, 0) {
		t.Errorf("equal = %v", d)
	}
	if d := distanceHTTPVersion(httpV11, httpAny); !u32Eq(d, 0) {
		t.Errorf("any = %v", d)
	}
	if d := distanceHTTPVersion(httpV11, httpV10); d != nil {
		t.Errorf("differ = %v, want nil", d)
	}
}

func TestHTTPHeaderValueEqual(t *testing.T) {
	if !httpHeaderValueEqual(nil, nil) {
		t.Errorf("nil/nil should be equal")
	}
	if httpHeaderValueEqual(strptr("x"), nil) || httpHeaderValueEqual(nil, strptr("x")) {
		t.Errorf("nil mismatch should not be equal")
	}
	if !httpHeaderValueEqual(strptr("x"), strptr("x")) {
		t.Errorf("equal values")
	}
	if httpHeaderValueEqual(strptr("x"), strptr("y")) {
		t.Errorf("differing values")
	}
}

func TestDistanceHeader(t *testing.T) {
	h := func(name string, opt bool, value *string) httpHeader {
		return httpHeader{optional: opt, name: name, value: value}
	}

	// 精确匹配 → 0
	obs := []httpHeader{h("Host", false, nil), h("Accept", false, strptr("x"))}
	sig := []httpHeader{h("Host", false, nil), h("Accept", false, strptr("x"))}
	if d := distanceHeader(obs, sig); !u32Eq(d, 0) {
		t.Errorf("exact = %v", d)
	}

	// 观测头多余 → 1 错误 → 0 分
	obs = append(obs, h("Extra", false, nil))
	if d := distanceHeader(obs, sig); !u32Eq(d, 0) {
		t.Errorf("extra obs = %v, want 0", d)
	}

	// 签名可选头跳过 → 0
	sig = append(sig, h("?Keep-Alive", true, nil))
	if d := distanceHeader(obs, sig); !u32Eq(d, 0) {
		t.Errorf("optional skip = %v", d)
	}

	// 值不一致且非可选 → 1 错误
	obs = []httpHeader{h("Accept", false, strptr("y"))}
	sig = []httpHeader{h("Accept", false, strptr("x"))}
	if d := distanceHeader(obs, sig); !u32Eq(d, 0) {
		t.Errorf("value mismatch = %v", d)
	}

	// 值不一致但可选 → 无错误
	sig = []httpHeader{h("?Accept", true, strptr("x"))}
	if d := distanceHeader(obs, sig); !u32Eq(d, 0) {
		t.Errorf("optional value mismatch = %v", d)
	}

	// 签名剩余非可选 → 错误
	obs = []httpHeader{h("A", false, nil)}
	sig = []httpHeader{h("A", false, nil), h("B", false, nil)}
	if d := distanceHeader(obs, sig); !u32Eq(d, 0) {
		t.Errorf("sig leftover = %v", d)
	}

	// 4 个错误 → 距离 1（3-5 带）
	obs = []httpHeader{h("E1", false, nil), h("E2", false, nil), h("E3", false, nil), h("E4", false, nil)}
	sig = []httpHeader{h("S1", false, nil)}
	if d := distanceHeader(obs, sig); !u32Eq(d, 1) {
		t.Errorf("4 errors = %v, want 1", d)
	}

	// 7 个错误 → 距离 2（6-8 带）
	obs = []httpHeader{h("E1", false, nil), h("E2", false, nil), h("E3", false, nil),
		h("E4", false, nil), h("E5", false, nil), h("E6", false, nil), h("E7", false, nil)}
	if d := distanceHeader(obs, sig); !u32Eq(d, 2) {
		t.Errorf("7 errors = %v, want 2", d)
	}

	// 12 个错误 → 不可匹配
	obs = make([]httpHeader, 12)
	for i := range obs {
		obs[i] = h("E", false, nil)
	}
	if d := distanceHeader(obs, sig); d != nil {
		t.Errorf("12 errors = %v, want nil", d)
	}

	// 空双方 → 0
	if d := distanceHeader(nil, nil); !u32Eq(d, 0) {
		t.Errorf("empty = %v", d)
	}
}

func TestDistanceExpsw(t *testing.T) {
	if d := distanceExpsw("nginx/1.18.0", "nginx/"); !u32Eq(d, 0) {
		t.Errorf("contains = %v", d)
	}
	if d := distanceExpsw("Apache/2.4", "nginx/"); !u32Eq(d, 3) {
		t.Errorf("missing = %v, want 3", d)
	}
}

func TestCalculateHTTPDistance(t *testing.T) {
	obs := &httpObservation{
		version: httpV11,
		horder:  []httpHeader{{name: "Host"}},
		habsent: []httpHeader{{name: "Accept-Charset"}},
		expsw:   "nginx/1.18.0",
	}
	sig := &httpSignature{
		version: httpV11,
		horder:  []httpHeader{{name: "Host"}},
		habsent: []httpHeader{{name: "Accept-Charset"}},
		expsw:   "nginx/",
	}
	if d := calculateHTTPDistance(obs, sig); !u32Eq(d, 0) {
		t.Errorf("exact = %v", d)
	}

	// 版本不兼容 → nil
	bad := *obs
	bad.version = httpV10
	if d := calculateHTTPDistance(&bad, sig); d != nil {
		t.Errorf("version mismatch = %v", d)
	}

	// 头顺序错误过多 → nil
	bad = *obs
	bad.horder = []httpHeader{{name: "X"}, {name: "Y"}, {name: "Z"}, {name: "W"},
		{name: "V"}, {name: "U"}, {name: "T"}, {name: "S"}, {name: "R"}, {name: "Q"},
		{name: "P"}, {name: "O"}, {name: "N"}, {name: "M"}}
	if d := calculateHTTPDistance(&bad, sig); d != nil {
		t.Errorf("header mismatch = %v, want nil", d)
	}

	// 期望子串不匹配 → 3 分
	bad = *obs
	bad.expsw = "Apache/2.4"
	if d := calculateHTTPDistance(&bad, sig); !u32Eq(d, 3) {
		t.Errorf("expsw mismatch = %v, want 3", d)
	}
}

func TestHTTPDistanceToScore(t *testing.T) {
	cases := []struct {
		d    uint32
		want float64
	}{
		{0, 1.0}, {1, 0.95}, {2, 0.90}, {3, 0.80}, {4, 0.70},
		{5, 0.70}, {6, 0.60}, {7, 0.60}, {8, 0.40}, {9, 0.40},
		{10, 0.20}, {11, 0.20}, {12, 0.10}, {13, 0.05}, {100, 0.05},
	}
	for _, tc := range cases {
		if got := httpDistanceToScore(tc.d); got != tc.want {
			t.Errorf("httpDistanceToScore(%d) = %v, want %v", tc.d, got, tc.want)
		}
	}
}
