// HTTP 观测值提取单元测试：observe_http.go。
//
// 覆盖 HTTP/1.x 请求 / 响应解析、头格式转换、缺失头构建、语言偏好提取
// 与各错误拒绝路径。
package huginn

import (
	"testing"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// strptr 返回字符串指针，用于构造带值的 HTTP 头。
func strptr(s string) *string { return &s }

// eqHeaders 比较两个 httpHeader 列表（顺序敏感；值按内容比较）。
func eqHeaders(a, b []httpHeader) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].optional != b[i].optional || a[i].name != b[i].name || !httpHeaderValueEqual(a[i].value, b[i].value) {
			return false
		}
	}
	return true
}

func TestSplitHTTPLines(t *testing.T) {
	if got := splitHTTPLines("a\r\nb"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("crlf: %v", got)
	}
	if got := splitHTTPLines("a\nb\n"); len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "" {
		t.Errorf("lf: %v", got)
	}
	// 同时含 \r\n 与 \n 时优先 \r\n 分隔
	if got := splitHTTPLines("a\r\nb\nc"); len(got) != 2 || got[0] != "a" {
		t.Errorf("mixed: %v", got)
	}
}

func TestParseHTTPHeaders(t *testing.T) {
	lines := []string{
		"Host: example.com",
		"  X-Pad  :  value  ",
		"no-colon-line",
		": empty-name",
		"",
		"After-Blank: 1",
	}
	got := parseHTTPHeaders(lines)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].name != "Host" || *got[0].value != "example.com" {
		t.Errorf("host = %+v", got[0])
	}
	if got[1].name != "X-Pad" || *got[1].value != "value" {
		t.Errorf("x-pad = %+v", got[1])
	}
}

func TestParseHTTPRequest(t *testing.T) {
	data := []byte("GET /index.html HTTP/1.1\r\n" +
		"Host: example.com\r\n" +
		"User-Agent: Mozilla/5.0 (X11; Linux x86_64; rv:115.0) Gecko/20100101 Firefox/115.0\r\n" +
		"Accept-Language: en-US,en;q=0.5\r\n" +
		"Accept: text/html\r\n" +
		"Cookie: session=1\r\n" +
		"Referer: http://example.com/\r\n" +
		"Connection: keep-alive\r\n\r\n")
	obs, err := parseHTTPRequest(data)
	if err != nil {
		t.Fatalf("parseHTTPRequest: %v", err)
	}
	if obs.matching.version != httpV11 {
		t.Errorf("version = %v", obs.matching.version)
	}
	// Cookie / Referer 从指纹头列表中剔除
	for _, h := range obs.matching.horder {
		if h.name == "Cookie" || h.name == "Referer" {
			t.Errorf("horder contains %s", h.name)
		}
	}
	// 期望子串取 User-Agent 完整值
	wantExpsw := "Mozilla/5.0 (X11; Linux x86_64; rv:115.0) Gecko/20100101 Firefox/115.0"
	if obs.matching.expsw != wantExpsw {
		t.Errorf("expsw = %s", obs.matching.expsw)
	}
	// 语言偏好取最高质量
	if obs.lang != "English" {
		t.Errorf("lang = %s, want English", obs.lang)
	}
	// 头顺序与格式：Host/User-Agent 忽略值，Accept-Language 为可选头
	want := []httpHeader{
		{name: "Host"},
		{name: "User-Agent"},
		{name: "Accept-Language", value: strptr("en-US,en;q=0.5")},
		{name: "Accept", value: strptr("text/html")},
		{name: "Connection", value: strptr("keep-alive")},
	}
	if !eqHeaders(obs.matching.horder, want) {
		t.Errorf("horder = %+v, want %+v", obs.matching.horder, want)
	}
	// 缺失头：常见头中未出现的
	if !containsHeaderName(obs.matching.habsent, "Accept-Encoding") {
		t.Errorf("habsent = %+v, want Accept-Encoding", obs.matching.habsent)
	}
	if containsHeaderName(obs.matching.habsent, "Host") {
		t.Errorf("habsent = %+v, Host should be present", obs.matching.habsent)
	}
}

func containsHeaderName(headers []httpHeader, name string) bool {
	for _, h := range headers {
		if h.name == name {
			return true
		}
	}
	return false
}

func TestParseHTTPRequestErrors(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want error
	}{
		{"invalid utf8", []byte{0xff, 0xfe, 0xfd}, ErrInvalidUTF8},
		{"incomplete", []byte("GET / HTTP/1.1\r\nHost: x"), ErrIncompleteData},
		{"bad request line", []byte("GET / HTTP/1.1 extra\r\n\r\n"), ErrInvalidHTTPRequestLine},
		{"two tokens", []byte("GET /\r\n\r\n"), ErrInvalidHTTPRequestLine},
		{"bad method", []byte("FOO / HTTP/1.1\r\n\r\n"), ErrInvalidHTTPMethod},
		{"bad version token", []byte("GET / FOO\r\n\r\n"), ErrInvalidHTTPVersion},
		{"http2 not accepted", []byte("GET / HTTP/2.0\r\n\r\n"), ErrInvalidHTTPVersion},
	}
	for _, tc := range cases {
		if _, err := parseHTTPRequest(tc.data); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestParseHTTPResponse(t *testing.T) {
	data := []byte("HTTP/1.1 200 OK\r\n" +
		"Server: nginx/1.18.0\r\n" +
		"Date: Mon, 01 Jan 2024 00:00:00 GMT\r\n" +
		"Content-Type: text/html\r\n\r\n")
	obs, err := parseHTTPResponse(data)
	if err != nil {
		t.Fatalf("parseHTTPResponse: %v", err)
	}
	if obs.matching.version != httpV11 {
		t.Errorf("version = %v", obs.matching.version)
	}
	if obs.matching.expsw != "nginx/1.18.0" {
		t.Errorf("expsw = %s", obs.matching.expsw)
	}
	// Server/Date/Content-Type 均忽略值
	for _, h := range obs.matching.horder {
		if h.value != nil {
			t.Errorf("horder %s has value %s, want skip-value", h.name, *h.value)
		}
	}

	// HTTP/1.0 响应
	v10 := []byte("HTTP/1.0 404 Not Found\r\nServer: nginx/1.18.0\r\n\r\n")
	obs10, err := parseHTTPResponse(v10)
	if err != nil {
		t.Fatalf("parseHTTPResponse v10: %v", err)
	}
	if obs10.matching.version != httpV10 {
		t.Errorf("version = %v, want 0", obs10.matching.version)
	}
}

func TestParseHTTPResponseErrors(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want error
	}{
		{"invalid utf8", []byte{0xff, 0xfe}, ErrInvalidUTF8},
		{"incomplete", []byte("HTTP/1.1 200 OK"), ErrIncompleteData},
		{"no status", []byte("HTTP/1.1\r\n\r\n"), ErrInvalidHTTPStatusLine},
		{"short status", []byte("HTTP/1.1 20 OK\r\n\r\n"), ErrInvalidHTTPStatusLine},
		{"non digit status", []byte("HTTP/1.1 abc OK\r\n\r\n"), ErrInvalidHTTPStatusLine},
		{"bad version", []byte("HTTP/2.0 200 OK\r\n\r\n"), ErrInvalidHTTPVersion},
	}
	for _, tc := range cases {
		if _, err := parseHTTPResponse(tc.data); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestConvertHeadersToHTTPFormat(t *testing.T) {
	// 请求：Cookie → 可选；Host/User-Agent → 忽略值；其余保留值
	req := []httpHeaderRaw{
		{name: "Host", value: strptr("example.com")},
		{name: "User-Agent", value: strptr("ua")},
		{name: "Cookie", value: strptr("c=1")},
		{name: "Accept", value: strptr("text/html")},
		{name: "X-Custom", value: strptr("v")},
	}
	wantReq := []httpHeader{
		{name: "Host"},
		{name: "User-Agent"},
		{optional: true, name: "Cookie"},
		{name: "Accept", value: strptr("text/html")},
		{name: "X-Custom", value: strptr("v")},
	}
	if got := convertHeadersToHTTPFormat(req, true); !eqHeaders(got, wantReq) {
		t.Errorf("request = %+v, want %+v", got, wantReq)
	}

	// 响应：Set-Cookie → 可选；Date/Content-Type/Server → 忽略值
	resp := []httpHeaderRaw{
		{name: "Date", value: strptr("d")},
		{name: "Content-Type", value: strptr("text/html")},
		{name: "Server", value: strptr("nginx")},
		{name: "Set-Cookie", value: strptr("c=1")},
		{name: "Connection", value: strptr("keep-alive")},
		{name: "X-Custom", value: strptr("y")},
	}
	wantResp := []httpHeader{
		{name: "Date"},
		{name: "Content-Type"},
		{name: "Server"},
		{optional: true, name: "Set-Cookie"},
		{name: "Connection", value: strptr("keep-alive")},
		{name: "X-Custom", value: strptr("y")},
	}
	if got := convertHeadersToHTTPFormat(resp, false); !eqHeaders(got, wantResp) {
		t.Errorf("response = %+v, want %+v", got, wantResp)
	}
}

func TestBuildAbsentHeaders(t *testing.T) {
	// 请求：Host/User-Agent/Accept 已出现
	req := []httpHeaderRaw{
		{name: "Host", value: strptr("x")},
		{name: "User-Agent", value: strptr("ua")},
		{name: "Accept", value: strptr("*/*")},
	}
	got := buildAbsentHeaders(req, true)
	if len(got) != 5 { // Connection, Accept-Encoding, Accept-Language, Accept-Charset, Keep-Alive
		t.Fatalf("request absent len = %d, want 5: %+v", len(got), got)
	}
	for _, name := range []string{"Connection", "Accept-Encoding", "Accept-Language", "Accept-Charset", "Keep-Alive"} {
		if !containsHeaderName(got, name) {
			t.Errorf("request absent missing %s", name)
		}
	}
	if containsHeaderName(got, "Host") {
		t.Errorf("request absent should not contain Host")
	}

	// 响应：Content-Type/Date 已出现 → 缺 Connection/Keep-Alive/Accept-Ranges
	resp := []httpHeaderRaw{
		{name: "Content-Type", value: strptr("text/html")},
		{name: "Date", value: strptr("d")},
	}
	got = buildAbsentHeaders(resp, false)
	if len(got) != 3 {
		t.Fatalf("response absent len = %d, want 3: %+v", len(got), got)
	}

	// 空头 → 全部常见头
	if got := buildAbsentHeaders(nil, true); len(got) != len(requestCommonHeaders) {
		t.Errorf("empty request absent len = %d", len(got))
	}
	if got := buildAbsentHeaders(nil, false); len(got) != len(responseCommonHeaders) {
		t.Errorf("empty response absent len = %d", len(got))
	}
}

func TestExpswValue(t *testing.T) {
	if got := expswValue(""); got != "???" {
		t.Errorf("empty = %s", got)
	}
	if got := expswValue("nginx/1.18"); got != "nginx/1.18" {
		t.Errorf("value = %s", got)
	}
}

func TestGetHighestQualityLanguage(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"en-US,en;q=0.5", "English"},
		{"fr;q=0.9,en;q=0.5,de;q=0.3", "French"}, // 质量降序
		{"en;q=0.5,fr;q=0.5", "English"},         // 同质量取更靠前
		{"fr,en;q=0.7", "French"},                // 无质量项默认 1.0
		{"en;q=0.8,zh-Hans;q=0.9", "Chinese"},    // 语言代码取主部分
		{"xx,yy", ""},                            // 全部未知
		{"en;q=abc,fr;q=0.5", "English"},         // 非法质量回退 1.0
		{" ,en;q=0.5,", "English"},               // 跳过空条目
	}
	for _, tc := range cases {
		if got := getHighestQualityLanguage(tc.in); got != tc.want {
			t.Errorf("getHighestQualityLanguage(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
