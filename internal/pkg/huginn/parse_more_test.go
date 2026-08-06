// 词法解析器补充单元测试：parse.go 的各 token 解析函数。
//
// 覆盖 p0f.fp 中较少出现的选项 / 怪癖 / 载荷分类标记与各错误拒绝路径。
package huginn

import (
	"testing"
)

func TestParseIPVersion(t *testing.T) {
	cases := []struct {
		in   string
		want ipVersion
		ok   bool
	}{
		{"4", ipV4, true},
		{"6", ipV6, true},
		{"*", ipAny, true},
		{"5", 0, false},
		{"x", 0, false},
		{"", 0, false},
	}
	for _, tc := range cases {
		got, err := parseIPVersion(tc.in)
		if tc.ok && (err != nil || got != tc.want) {
			t.Errorf("parseIPVersion(%q) = %v,%v want %v", tc.in, got, err, tc.want)
		}
		if !tc.ok && err == nil {
			t.Errorf("parseIPVersion(%q) = %v, want error", tc.in, got)
		}
	}
}

func TestParseUintErrors(t *testing.T) {
	if _, err := parseUint8("256"); err == nil {
		t.Error("parseUint8(256): want error")
	}
	if _, err := parseUint8("abc"); err == nil {
		t.Error("parseUint8(abc): want error")
	}
	if _, err := parseUint8(""); err == nil {
		t.Error("parseUint8(empty): want error")
	}
	if _, err := parseUint16("70000"); err == nil {
		t.Error("parseUint16(70000): want error")
	}
	if _, err := parseUint16("abc"); err == nil {
		t.Error("parseUint16(abc): want error")
	}
}

func TestParseTTLErrors(t *testing.T) {
	for _, in := range []string{"", "64+", "64++", "abc", "x-"} {
		if _, err := parseTTL(in); err == nil {
			t.Errorf("parseTTL(%q): want error", in)
		}
	}
}

func TestParseWindowSizeErrors(t *testing.T) {
	for _, in := range []string{"mss*", "mtu*x", "%", "65536", "abc"} {
		if _, err := parseWindowSize(in); err == nil {
			t.Errorf("parseWindowSize(%q): want error", in)
		}
	}
}

func TestParseTCPOptionAll(t *testing.T) {
	cases := []struct {
		in   string
		want tcpOption
	}{
		{"eol+3", tcpOption{kind: optEol, num: 3}},
		{"nop", tcpOption{kind: optNop}},
		{"mss", tcpOption{kind: optMss}},
		{"ws", tcpOption{kind: optWs}},
		{"sok", tcpOption{kind: optSok}},
		{"sack", tcpOption{kind: optSack}},
		{"ts", tcpOption{kind: optTS}},
		{"?29", tcpOption{kind: optUnknown, num: 29}},
	}
	for _, tc := range cases {
		got, err := parseTCPOption(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("parseTCPOption(%q) = %+v,%v want %+v", tc.in, got, err, tc.want)
		}
	}
}

func TestParseTCPOptionErrors(t *testing.T) {
	for _, in := range []string{"", "eol+x", "eol+", "?x", "?", "badopt", "mss2"} {
		if _, err := parseTCPOption(in); err == nil {
			t.Errorf("parseTCPOption(%q): want error", in)
		}
	}
}

func TestParseQuirkAll(t *testing.T) {
	cases := []struct {
		in   string
		want quirk
	}{
		{"df", quirkDf},
		{"id+", quirkNonZeroID},
		{"id-", quirkZeroID},
		{"ecn", quirkEcn},
		{"0+", quirkMustBeZero},
		{"flow", quirkFlowID},
		{"seq-", quirkSeqNumZero},
		{"ack+", quirkAckNumNonZero},
		{"ack-", quirkAckNumZero},
		{"uptr+", quirkNonZeroURG},
		{"urgf+", quirkUrg},
		{"pushf+", quirkPush},
		{"ts1-", quirkOwnTimestampZero},
		{"ts2+", quirkPeerTimestampNonZero},
		{"opt+", quirkTrailingNonZero},
		{"exws", quirkExcessiveWindowScaling},
		{"bad", quirkOptBad},
	}
	for _, tc := range cases {
		got, err := parseQuirk(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("parseQuirk(%q) = %v,%v want %v", tc.in, got, err, tc.want)
		}
	}
	if _, err := parseQuirk("unknown"); err == nil {
		t.Error("parseQuirk(unknown): want error")
	}
}

func TestParsePayloadSizeAll(t *testing.T) {
	cases := []struct {
		in   string
		want payloadSize
	}{
		{"0", payloadZero},
		{"+", payloadNonZero},
		{"*", payloadAny},
	}
	for _, tc := range cases {
		got, err := parsePayloadSize(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("parsePayloadSize(%q) = %v,%v want %v", tc.in, got, err, tc.want)
		}
	}
	if _, err := parsePayloadSize("x"); err == nil {
		t.Error("parsePayloadSize(x): want error")
	}
}

func TestParseTCPSignatureErrors(t *testing.T) {
	// 段数不对
	if _, err := parseTCPSignature("1:2:3"); err == nil {
		t.Error("short segments: want error")
	}
	// 各段非法
	bad := []string{
		"x:64:0:*:*,*:mss:df:0",    // 版本
		"*:x:0:*:*,*:mss:df:0",     // TTL
		"*:64:x:*:*,*:mss:df:0",    // olen
		"*:64:0:x:*,*:mss:df:0",    // mss
		"*:64:0:*:x:df:0",          // 窗口对缺逗号
		"*:64:0:*:*,x:mss:df:0",    // wscale
		"*:64:0:*:*,*:badopt:df:0", // 选项
		"*:64:0:*:*,*:mss:badq:0",  // 怪癖
		"*:64:0:*:*,*:mss:df:x",    // 载荷分类
	}
	for _, in := range bad {
		if _, err := parseTCPSignature(in); err == nil {
			t.Errorf("parseTCPSignature(%q): want error", in)
		}
	}
}

func TestParseHTTPVersionErrors(t *testing.T) {
	if _, err := parseHTTPVersion("x"); err == nil {
		t.Error("parseHTTPVersion(x): want error")
	}
	if _, err := parseHTTPVersion("1.0"); err == nil {
		t.Error("parseHTTPVersion(1.0): want error")
	}
}

func TestParseSectionsErrorsExtra(t *testing.T) {
	// sig 先于 label / 非法 HTTP 签名
	for _, in := range []string{
		"[tcp:request]\nsig = *:64:0:*:*,*:mss:df:0\nlabel = s:x:y:z\n",
		"[http:request]\nlabel = s:!:X:1.0\nsig = bad\n",
	} {
		if _, err := parseSections(in); err == nil {
			t.Errorf("parseSections(%q): want error", in)
		}
	}
}
