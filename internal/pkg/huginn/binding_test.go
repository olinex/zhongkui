// 对外绑定单元测试：binding.go。
//
// 覆盖 4 个公开函数、JSON 契约字段名、空输入 / 解析失败返回 nil、
// HTTP 解析成功但未命中签名仍返回结果的行为。
package huginn

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestAnalyzeTCPIPv4RoundTrip(t *testing.T) {
	fp, err := AnalyzeTCPIPv4(linuxSYNPacket())
	if err != nil {
		t.Fatalf("AnalyzeTCPIPv4: %v", err)
	}
	if fp == nil {
		t.Fatal("nil fingerprint")
	}
	if fp.OS != "Linux 3.1-3.10" {
		t.Errorf("OS = %q", fp.OS)
	}
	if fp.OSQuality != 1.0 {
		t.Errorf("OSQuality = %v, want 1.0", fp.OSQuality)
	}
	if fp.Sig != "4:64+0:0:1460:mss*10,6:mss,sok,ts,nop,ws:df,id+:0" {
		t.Errorf("Sig = %q", fp.Sig)
	}
	if fp.MTU != 1500 {
		t.Errorf("MTU = %d, want 1500", fp.MTU)
	}
}

func TestAnalyzeTCPEmptyAndErrors(t *testing.T) {
	if fp, err := AnalyzeTCPIPv4(nil); fp != nil || err != nil {
		t.Errorf("empty v4: fp=%v err=%v, want nil/nil", fp, err)
	}
	if fp, err := AnalyzeTCPIPv6(nil); fp != nil || err != nil {
		t.Errorf("empty v6: fp=%v err=%v, want nil/nil", fp, err)
	}
	// 过短数据
	if fp, _ := AnalyzeTCPIPv4([]byte{0, 1, 2}); fp != nil {
		t.Errorf("short: fp=%v, want nil", fp)
	}
	// 非 TCP 协议
	udp := buildIPv4(5, 0, 0, 0, 0, 64, 17, nil)
	if fp, _ := AnalyzeTCPIPv4(udp); fp != nil {
		t.Errorf("udp: fp=%v, want nil", fp)
	}
	// 非法标志组合（SYN+FIN）
	tcp := buildTCP(1, 2, 0, 0, tcpFlagSYN|tcpFlagFIN, 100, nil, nil)
	if fp, _ := AnalyzeTCPIPv4(buildIPv4(5, 0, 0, 0, 0, 64, ipProtoTCP, tcp)); fp != nil {
		t.Errorf("syn|fin: fp=%v, want nil", fp)
	}
}

func TestAnalyzeTCPIPv6AndResponse(t *testing.T) {
	// IPv6 SYN：无匹配签名 → OS 为空但 sig/mtu 仍填充
	opts := tcpOptBytes(tcpOptMSS, 0x05, 0xa0)
	opts = append(opts, tcpOptBytes(tcpOptSOK)...)
	opts = append(opts, tcpOptBytes(tcpOptNOP)...)
	opts = append(opts, tcpOptBytes(tcpOptNOP)...)
	tcp := buildTCP(12345, 80, 1, 0, tcpFlagSYN, 2880, opts, nil)
	fp, err := AnalyzeTCPIPv6(buildIPv6(0, 0, ipProtoTCP, 64, tcp))
	if err != nil {
		t.Fatalf("AnalyzeTCPIPv6: %v", err)
	}
	if fp == nil {
		t.Fatal("nil v6 fingerprint")
	}
	if fp.Sig == "" {
		t.Error("v6 Sig is empty")
	}
	if fp.MTU != 1488 {
		t.Errorf("v6 MTU = %d, want 1488", fp.MTU)
	}

	// SYN-ACK → 服务端方向（tcpResponse 分支）
	tcp = buildTCP(80, 12345, 1, 1, tcpFlagSYN|tcpFlagACK, 14600, nil, nil)
	fp, err = AnalyzeTCPIPv4(buildIPv4(5, 0, 0x1234, 0x40, 0, 64, ipProtoTCP, tcp))
	if err != nil {
		t.Fatalf("AnalyzeTCPIPv4 synack: %v", err)
	}
	if fp == nil {
		t.Fatal("nil synack fingerprint")
	}
	if fp.Sig == "" {
		t.Error("synack Sig is empty")
	}
}

func TestAnalyzeHTTPRequestMatch(t *testing.T) {
	// 精确匹配 p0f.fp 中 Firefox 2.x 签名（含全部头值与顺序）→ 总距离 0
	raw := []byte("GET / HTTP/1.1\r\n" +
		"Host: example.com\r\n" +
		"User-Agent: Mozilla/5.0 (X11; Linux x86_64; rv:115.0) Gecko/20100101 Firefox/115.0\r\n" +
		"Accept: ,*/*;q=\r\n" +
		"Accept-Language: en-US,en;q=0.5\r\n" +
		"Accept-Encoding: gzip,deflate\r\n" +
		"Accept-Charset: utf-8;q=0.7,*;q=0.7\r\n" +
		"Keep-Alive: 300\r\n" +
		"Connection: keep-alive\r\n\r\n")
	fp, err := AnalyzeHTTPRequest(raw)
	if err != nil {
		t.Fatalf("AnalyzeHTTPRequest: %v", err)
	}
	if fp == nil {
		t.Fatal("nil request fingerprint")
	}
	if fp.Browser != "Firefox 2.x" {
		t.Errorf("Browser = %q, want Firefox 2.x", fp.Browser)
	}
	if fp.Quality != 1.0 {
		t.Errorf("Quality = %v, want 1.0", fp.Quality)
	}
	if fp.Language != "English" {
		t.Errorf("Language = %q, want English", fp.Language)
	}
}

func TestAnalyzeHTTPRequestNoMatch(t *testing.T) {
	// 大量未知头 → 所有签名头距离均超上限 → 解析成功但未命中
	var sb strings.Builder
	sb.WriteString("GET / HTTP/1.1\r\n")
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&sb, "X-%d: %d\r\n", i, i)
	}
	sb.WriteString("\r\n")
	fp, err := AnalyzeHTTPRequest([]byte(sb.String()))
	if err != nil {
		t.Fatalf("AnalyzeHTTPRequest: %v", err)
	}
	if fp == nil {
		t.Fatal("nil request fingerprint (parse succeeded)")
	}
	if fp.Browser != "" || fp.Quality != 0 {
		t.Errorf("Browser=%q Quality=%v, want empty/0", fp.Browser, fp.Quality)
	}
}

func TestAnalyzeHTTPResponseMatch(t *testing.T) {
	raw := []byte("HTTP/1.1 200 OK\r\n" +
		"Server: nginx/1.18.0\r\n" +
		"Date: Mon, 01 Jan 2024 00:00:00 GMT\r\n" +
		"Content-Type: text/html\r\n" +
		"Connection: keep-alive\r\n" +
		"Keep-Alive: timeout\r\n" +
		"Accept-Ranges: bytes\r\n\r\n")
	fp, err := AnalyzeHTTPResponse(raw)
	if err != nil {
		t.Fatalf("AnalyzeHTTPResponse: %v", err)
	}
	if fp == nil {
		t.Fatal("nil response fingerprint")
	}
	if fp.Server != "nginx 1.x" {
		t.Errorf("Server = %q, want nginx 1.x", fp.Server)
	}
	if fp.Quality != 1.0 {
		t.Errorf("Quality = %v, want 1.0", fp.Quality)
	}
}

func TestAnalyzeHTTPEmptyAndErrors(t *testing.T) {
	if fp, err := AnalyzeHTTPRequest(nil); fp != nil || err != nil {
		t.Errorf("empty request: fp=%v err=%v, want nil/nil", fp, err)
	}
	if fp, err := AnalyzeHTTPResponse(nil); fp != nil || err != nil {
		t.Errorf("empty response: fp=%v err=%v, want nil/nil", fp, err)
	}
	// 解析失败 → nil
	if fp, _ := AnalyzeHTTPRequest([]byte("GET / HTTP/1.1\r\nHost: x")); fp != nil {
		t.Errorf("incomplete request: fp=%v, want nil", fp)
	}
	if fp, _ := AnalyzeHTTPResponse([]byte{0xff, 0xfe}); fp != nil {
		t.Errorf("invalid utf8 response: fp=%v, want nil", fp)
	}
}

func TestTCPFingerprintJSON(t *testing.T) {
	fp, err := AnalyzeTCPIPv4(linuxSYNPacket())
	if err != nil || fp == nil {
		t.Fatalf("AnalyzeTCPIPv4: fp=%v err=%v", fp, err)
	}
	data, err := json.Marshal(fp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"os", "os_quality", "sig", "mtu"} {
		if _, ok := m[key]; !ok {
			t.Errorf("JSON missing key %q: %s", key, data)
		}
	}
	if m["os"] != "Linux 3.1-3.10" {
		t.Errorf("os = %v", m["os"])
	}
	if m["mtu"] != float64(1500) {
		t.Errorf("mtu = %v", m["mtu"])
	}
}

func TestHTTPFingerprintJSON(t *testing.T) {
	raw := []byte("HTTP/1.1 200 OK\r\n" +
		"Server: nginx/1.18.0\r\n" +
		"Date: x\r\n" +
		"Content-Type: text/html\r\n" +
		"Connection: keep-alive\r\n" +
		"Keep-Alive: timeout\r\n" +
		"Accept-Ranges: bytes\r\n\r\n")
	fp, err := AnalyzeHTTPResponse(raw)
	if err != nil || fp == nil {
		t.Fatalf("AnalyzeHTTPResponse: fp=%v err=%v", fp, err)
	}
	data, err := json.Marshal(fp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"browser", "server", "language", "quality"} {
		if _, ok := m[key]; !ok {
			t.Errorf("JSON missing key %q: %s", key, data)
		}
	}
	if m["server"] != "nginx 1.x" {
		t.Errorf("server = %v", m["server"])
	}
}
