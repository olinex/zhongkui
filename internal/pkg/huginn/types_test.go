// 类型渲染与解析单元测试：types.go / signature.go。
//
// 覆盖 String() 的各分支与 parseHTTPVersionToken。
package huginn

import "testing"

func TestTTLString(t *testing.T) {
	cases := []struct {
		in   ttl
		want string
	}{
		{ttl{kind: ttlValue, first: 64}, "64"},
		{ttl{kind: ttlDistance, first: 64, second: 2}, "64+2"},
		{ttl{kind: ttlGuess, first: 64}, "64+?"},
		{ttl{kind: ttlBad, first: 0}, "0-"},
	}
	for _, tc := range cases {
		if got := tc.in.String(); got != tc.want {
			t.Errorf("ttl(%+v).String() = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestWindowSizeString(t *testing.T) {
	cases := []struct {
		in   windowSize
		want string
	}{
		{windowSize{kind: winMss, value: 10}, "mss*10"},
		{windowSize{kind: winMtu, value: 2}, "mtu*2"},
		{windowSize{kind: winMod, value: 1024}, "%1024"},
		{windowSize{kind: winValue, value: 65535}, "65535"},
		{windowSize{kind: winAny}, "*"},
	}
	for _, tc := range cases {
		if got := tc.in.String(); got != tc.want {
			t.Errorf("windowSize(%+v).String() = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTcpOptionString(t *testing.T) {
	cases := []struct {
		in   tcpOption
		want string
	}{
		{tcpOption{kind: optEol, num: 3}, "eol+3"},
		{tcpOption{kind: optNop}, "nop"},
		{tcpOption{kind: optMss}, "mss"},
		{tcpOption{kind: optWs}, "ws"},
		{tcpOption{kind: optSok}, "sok"},
		{tcpOption{kind: optSack}, "sack"},
		{tcpOption{kind: optTS}, "ts"},
		{tcpOption{kind: optUnknown, num: 29}, "?29"},
	}
	for _, tc := range cases {
		if got := tc.in.String(); got != tc.want {
			t.Errorf("tcpOption(%+v).String() = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestQuirkString(t *testing.T) {
	all := []quirk{quirkDf, quirkNonZeroID, quirkZeroID, quirkEcn, quirkMustBeZero,
		quirkFlowID, quirkSeqNumZero, quirkAckNumNonZero, quirkAckNumZero,
		quirkNonZeroURG, quirkUrg, quirkPush, quirkOwnTimestampZero,
		quirkPeerTimestampNonZero, quirkTrailingNonZero, quirkExcessiveWindowScaling,
		quirkOptBad}
	want := []string{"df", "id+", "id-", "ecn", "0+", "flow", "seq-", "ack+", "ack-",
		"uptr+", "urgf+", "pushf+", "ts1-", "ts2+", "opt+", "exws", "bad"}
	for i, q := range all {
		if got := q.String(); got != want[i] {
			t.Errorf("quirk(%d).String() = %q, want %q", q, got, want[i])
		}
	}
}

func TestPayloadSizeString(t *testing.T) {
	cases := []struct {
		in   payloadSize
		want string
	}{
		{payloadZero, "0"},
		{payloadNonZero, "+"},
		{payloadAny, "*"},
	}
	for _, tc := range cases {
		if got := tc.in.String(); got != tc.want {
			t.Errorf("payloadSize(%d).String() = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestHTTPVersionString(t *testing.T) {
	cases := []struct {
		in   httpVersion
		want string
	}{
		{httpV10, "0"},
		{httpV11, "1"},
		{httpV20, "2"},
		{httpV30, "3"},
		{httpAny, "*"},
	}
	for _, tc := range cases {
		if got := tc.in.String(); got != tc.want {
			t.Errorf("httpVersion(%d).String() = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseHTTPVersionToken(t *testing.T) {
	cases := []struct {
		in   string
		want httpVersion
		ok   bool
	}{
		{"HTTP/1.0", httpV10, true},
		{"HTTP/1.1", httpV11, true},
		{"HTTP/2", httpV20, true},
		{"HTTP/2.0", httpV20, true},
		{"HTTP/3", httpV30, true},
		{"HTTP/3.0", httpV30, true},
		{"HTTP/9.9", 0, false},
		{"HTTP/2.5", 0, false},
		{"garbage", 0, false},
		{"", 0, false},
	}
	for _, tc := range cases {
		got, ok := parseHTTPVersionToken(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("parseHTTPVersionToken(%q) = %v,%v want %v,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestHTTPHeaderString(t *testing.T) {
	cases := []struct {
		in   httpHeader
		want string
	}{
		{httpHeader{name: "Host"}, "Host"},
		{httpHeader{optional: true, name: "Accept"}, "?Accept"},
		{httpHeader{name: "Server", value: strptr("nginx/")}, "Server=[nginx/]"},
	}
	for _, tc := range cases {
		if got := tc.in.String(); got != tc.want {
			t.Errorf("httpHeader(%+v).String() = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestHTTPSignatureString(t *testing.T) {
	s := httpSignature{
		version: httpV11,
		horder:  []httpHeader{{name: "Host"}, {optional: true, name: "Accept"}},
		habsent: []httpHeader{{name: "Keep-Alive"}},
		expsw:   "nginx/",
	}
	if got := s.String(); got != "1:Host,?Accept:Keep-Alive:nginx/" {
		t.Errorf("httpSignature.String() = %q", got)
	}
}

func TestLabelString(t *testing.T) {
	l := label{ty: labelSpecified, class: strptr("unix"), name: "Linux", flavor: strptr("3.1-3.10")}
	if got := l.String(); got != "s:unix:Linux:3.1-3.10" {
		t.Errorf("label.String() = %q", got)
	}
	g := label{ty: labelGeneric, name: "generic"}
	if got := g.String(); got != "g::generic:" {
		t.Errorf("generic label.String() = %q", got)
	}
}

func TestFormatGuess(t *testing.T) {
	if got := formatGuess("Firefox", strptr("!"), strptr("115 or newer")); got != "Firefox 115 or newer" {
		t.Errorf("formatGuess = %q", got)
	}
	// 变体为空 → 仅名字
	if got := formatGuess("Linux", strptr("unix"), strptr("")); got != "Linux" {
		t.Errorf("empty variant = %q", got)
	}
	// 名字为空 → 回退族
	if got := formatGuess("", strptr("unix"), strptr("")); got != "unix" {
		t.Errorf("fallback family = %q", got)
	}
	if got := formatGuess("", nil, nil); got != "" {
		t.Errorf("empty = %q", got)
	}
}
