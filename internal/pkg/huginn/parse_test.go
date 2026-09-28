package huginn

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// TestLoadGlobalDatabase 验证内嵌 p0f.fp + p0f.local.fp 拼接后能完整解析并构建索引。
func TestLoadGlobalDatabase(t *testing.T) {
	Convey("global database loads from embedded files", t, func() {
		db, err := globalDatabase()
		So(err, ShouldBeNil)
		So(db, ShouldNotBeNil)

		Convey("tcp collections have indexed entries", func() {
			So(len(db.tcpRequest.entries), ShouldBeGreaterThan, 0)
			So(len(db.tcpResponse.entries), ShouldBeGreaterThan, 0)
			So(len(db.tcpRequest.index), ShouldBeGreaterThan, 0)
			So(len(db.tcpResponse.index), ShouldBeGreaterThan, 0)
		})

		Convey("http collections have indexed entries", func() {
			So(len(db.httpRequest.entries), ShouldBeGreaterThan, 0)
			So(len(db.httpResponse.entries), ShouldBeGreaterThan, 0)
			So(len(db.httpRequest.index), ShouldBeGreaterThan, 0)
			So(len(db.httpResponse.index), ShouldBeGreaterThan, 0)
		})

		Convey("known signatures present", func() {
			// Linux 3.1-3.10: *:64:0:*:mss*10,4:mss,sok,ts,nop,ws:df,id+:0
			// Mac OS X 10.x 回环：*:64:0:*:65535,6:mss,nop,ws,nop,nop,ts,sok,eol+1:df:0
			// Firefox 115 或更新版本（http 请求）
			req, err := parseTCPSignature("*:64:0:*:mss*10,4:mss,sok,ts,nop,ws:df,id+:0")
			So(err, ShouldBeNil)
			olayoutKey := tcpOlayoutKey(req.olayout)
			So(len(db.tcpRequest.index[tcpIndexKey{ipV4, olayoutKey, payloadZero}]), ShouldBeGreaterThan, 0)
			So(len(db.tcpRequest.index[tcpIndexKey{ipV6, olayoutKey, payloadZero}]), ShouldBeGreaterThan, 0)
		})
	})
}

// TestParseSections 验证行式解析器的 section 跟踪与分发。
func TestParseSections(t *testing.T) {
	Convey("parse sections dispatches by module", t, func() {
		input := `
; comment line
classes = win,unix,other

[mtu]
label = Ethernet or modem
sig   = 576
sig   = 1500

[tcp:request]
label = s:unix:Linux:3.1-3.10
sys   = @unix
sig   = *:64:0:*:mss*10,4:mss,sok,ts,nop,ws:df,id+:0

[tcp:response]
label = g:win:Windows:NT kernel
sig   = *:128:0:*:*,*:mss,nop,nop,sok:df,id+:0

[http:request]
label = s:!:Firefox:115 or newer
sig   = 1:Host,User-Agent,?Accept-Language,Accept,?Connection:Accept-Charset:Firefox/

[http:response]
label = s:!:nginx:1.x
sig   = 1:Date,Server=[nginx/]:Server:nginx/
`
		out, err := parseSections(input)
		So(err, ShouldBeNil)
		So(out.classes, ShouldResemble, []string{"win", "unix", "other"})
		So(len(out.mtu), ShouldEqual, 1)
		So(out.mtu[0].label, ShouldEqual, "Ethernet or modem")
		So(out.mtu[0].values, ShouldResemble, []uint16{576, 1500})
		So(len(out.tcpRequest), ShouldEqual, 1)
		So(out.tcpRequest[0].label.name, ShouldEqual, "Linux")
		So(*out.tcpRequest[0].label.flavor, ShouldEqual, "3.1-3.10")
		So(len(out.tcpRequest[0].sigs), ShouldEqual, 1)
		So(out.tcpRequest[0].sigs[0].String(), ShouldEqual, "*:64:0:*:mss*10,4:mss,sok,ts,nop,ws:df,id+:0")
		So(len(out.tcpResponse), ShouldEqual, 1)
		So(len(out.httpRequest), ShouldEqual, 1)
		So(out.httpRequest[0].sigs[0].expsw, ShouldEqual, "Firefox/")
		So(out.httpRequest[0].sigs[0].horder[2].optional, ShouldBeTrue)
		So(out.httpRequest[0].sigs[0].horder[2].name, ShouldEqual, "Accept-Language")
		So(len(out.httpResponse), ShouldEqual, 1)
		So(out.httpResponse[0].sigs[0].habsent, ShouldResemble, []httpHeader{{name: "Server"}})
		So(out.httpResponse[0].sigs[0].horder[1].value, ShouldNotBeNil)
		So(*out.httpResponse[0].sigs[0].horder[1].value, ShouldEqual, "nginx/")
	})
}

// TestParseSectionsErrors 验证解析错误路径。
func TestParseSectionsErrors(t *testing.T) {
	Convey("errors on invalid input", t, func() {
		Convey("line outside module", func() {
			_, err := parseSections("garbage = 1")
			So(err, ShouldNotBeNil)
		})
		Convey("bad label type", func() {
			_, err := parseSections("[tcp:request]\nlabel = x:unix:Linux:\n")
			So(err, ShouldNotBeNil)
		})
		Convey("sig without label", func() {
			_, err := parseSections("[tcp:request]\nsig = *:64:0:*:*,*:mss:df:0\n")
			So(err, ShouldNotBeNil)
		})
		Convey("bad mtu value", func() {
			_, err := parseSections("[mtu]\nlabel = x\nsig = abc\n")
			So(err, ShouldNotBeNil)
		})
		Convey("bad tcp signature segment count", func() {
			_, err := parseSections("[tcp:request]\nlabel = s:unix:Linux:\nsig = *:64:0\n")
			So(err, ShouldNotBeNil)
		})
	})
}

// TestParseTTL 验证 TTL 各表示形式解析。
func TestParseTTL(t *testing.T) {
	cases := []struct {
		in            string
		kind          ttlKind
		first, second uint8
	}{
		{"64", ttlValue, 64, 0},
		{"64+3", ttlDistance, 64, 3},
		{"64+?", ttlGuess, 64, 0},
		{"0-", ttlBad, 0, 0},
	}
	for _, tc := range cases {
		got, err := parseTTL(tc.in)
		if err != nil {
			t.Fatalf("parseTTL(%q): %v", tc.in, err)
		}
		if got.kind != tc.kind || got.first != tc.first || got.second != tc.second {
			t.Errorf("parseTTL(%q) = %+v, want kind=%v first=%d second=%d", tc.in, got, tc.kind, tc.first, tc.second)
		}
	}
}

// TestParseWindowSize 验证窗口大小各表示形式解析。
func TestParseWindowSize(t *testing.T) {
	cases := []struct {
		in    string
		kind  windowSizeKind
		value uint16
	}{
		{"*", winAny, 0},
		{"mss*10", winMss, 10},
		{"mtu*2", winMtu, 2},
		{"%1024", winMod, 1024},
		{"65535", winValue, 65535},
	}
	for _, tc := range cases {
		got, err := parseWindowSize(tc.in)
		if err != nil {
			t.Fatalf("parseWindowSize(%q): %v", tc.in, err)
		}
		if got.kind != tc.kind || got.value != tc.value {
			t.Errorf("parseWindowSize(%q) = %+v, want kind=%v value=%d", tc.in, got, tc.kind, tc.value)
		}
	}
}

// TestParseHTTPHeader 验证 HTTP 头解析（可选标记与 =[值]）。
func TestParseHTTPHeader(t *testing.T) {
	h, rest := parseHTTPHeader("?Accept-Language=[en]")
	if !h.optional || h.name != "Accept-Language" || h.value == nil || *h.value != "en" || rest != "" {
		t.Errorf("parseHTTPHeader(optional+value) = %+v, rest=%q", h, rest)
	}

	h2, rest2 := parseHTTPHeader("Host,X")
	if h2.optional || h2.name != "Host" || h2.value != nil || rest2 != ",X" {
		t.Errorf("parseHTTPHeader(plain) = %+v, rest=%q", h2, rest2)
	}

	// 无 `]` 时不消费 `=[` 前缀
	h3, rest3 := parseHTTPHeader("Server=[nginx")
	if h3.name != "Server" || h3.value != nil || rest3 != "=[nginx" {
		t.Errorf("parseHTTPHeader(no-close) = %+v, rest=%q", h3, rest3)
	}
}

// TestParseLabel 验证 label 解析。
func TestParseLabel(t *testing.T) {
	l, err := parseLabel("s:unix:Linux:3.1-3.10")
	if err != nil {
		t.Fatal(err)
	}
	if l.ty != labelSpecified || l.class == nil || *l.class != "unix" || l.name != "Linux" || l.flavor == nil || *l.flavor != "3.1-3.10" {
		t.Errorf("parseLabel(full) = %+v", l)
	}
	if got := l.String(); got != "s:unix:Linux:3.1-3.10" {
		t.Errorf("label.String() = %q", got)
	}

	l2, err := parseLabel("s:!:NMap:SYN scan")
	if err != nil {
		t.Fatal(err)
	}
	if l2.class != nil || l2.name != "NMap" || l2.flavor == nil || *l2.flavor != "SYN scan" {
		t.Errorf("parseLabel(no-class) = %+v", l2)
	}

	l3, err := parseLabel("g:win:Windows:")
	if err != nil {
		t.Fatal(err)
	}
	if l3.flavor != nil {
		t.Errorf("parseLabel(empty-flavor) = %+v", l3)
	}
}

// TestParseUAOS 验证 ua_os 表解析（截断语义）。
func TestParseUAOS(t *testing.T) {
	entries, err := parseUAOS("ua_os = Firefox,Chrome=Mozilla,IE")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("len(entries) = %d, want 3", len(entries))
	}
	if entries[0].ua != "Firefox" || entries[0].family != nil {
		t.Errorf("entries[0] = %+v", entries[0])
	}
	if entries[1].ua != "Chrome" || entries[1].family == nil || *entries[1].family != "Mozilla" {
		t.Errorf("entries[1] = %+v", entries[1])
	}
	if entries[2].ua != "IE" || entries[2].family != nil {
		t.Errorf("entries[2] = %+v", entries[2])
	}
}
