package detectors

import (
	"bufio"
	"bytes"
	"context"
	"net/http"
	"testing"

	n "zhongkui.com/zhongkui/internal/models/network"

	. "github.com/smartystreets/goconvey/convey"
)

// TestGuessDevice 测试设备类别猜测启发式（guessDevice）。
// 判定优先级：移动设备（客户端 OS 含移动平台关键词）> 桌面端（识别出浏览器）>
// 服务端（识别出服务端软件/系统）> IoT（无浏览器、无客户端 OS 指纹）> 未知。
func TestGuessDevice(t *testing.T) {
	Convey("Test guessDevice heuristic", t, func() {
		// clientOS 辅助：按客户端 OS 构建 StreamMeta
		metaWithOS := func(os string) *StreamMeta {
			return &StreamMeta{ClientFingerprint: &n.PacketTCPFingerprint{OS: os}}
		}

		Convey("Android client OS should be mobile", func() {
			So(guessDevice(metaWithOS("Android 10"), n.HttpFingerprint{}), ShouldEqual, "mobile")
		})

		Convey("iOS client OS should be mobile even with server fingerprint", func() {
			So(guessDevice(
				metaWithOS("iOS 16.0"),
				n.HttpFingerprint{Server: "nginx", ServerQuality: 0.9},
			), ShouldEqual, "mobile")
		})

		Convey("Browser matched should be desktop", func() {
			So(guessDevice(metaWithOS("Windows 10"), n.HttpFingerprint{
				Browser:        "Chrome 120.x",
				BrowserQuality: 0.9,
			}), ShouldEqual, "desktop")
		})

		Convey("Browser matched without client OS should be desktop", func() {
			So(guessDevice(&StreamMeta{}, n.HttpFingerprint{
				Browser:        "Firefox 115.x",
				BrowserQuality: 0.8,
			}), ShouldEqual, "desktop")
		})

		Convey("Browser quality zero should not be desktop", func() {
			So(guessDevice(&StreamMeta{}, n.HttpFingerprint{
				Browser: "Chrome 120.x",
			}), ShouldNotEqual, "desktop")
		})

		Convey("No browser but server matched should be server", func() {
			So(guessDevice(metaWithOS("Windows 10"), n.HttpFingerprint{
				Server:        "IIS 7.x",
				ServerQuality: 0.6,
			}), ShouldEqual, "server")
		})

		Convey("No browser and no client OS should be iot", func() {
			So(guessDevice(&StreamMeta{}, n.HttpFingerprint{}), ShouldEqual, "iot")
		})

		Convey("No browser but client OS present and no server should be unknown", func() {
			So(guessDevice(metaWithOS("Windows 10"), n.HttpFingerprint{}), ShouldEqual, "unknown")
		})
	})
}

// TestBuildTcpFingerprint 测试 TCP 指纹汇总逻辑（buildTcpFingerprint）。
// 客户端指纹来自 SYN 包、服务端指纹来自 SYN-ACK 包，MTU 优先取客户端。
func TestBuildTcpFingerprint(t *testing.T) {
	Convey("Test buildTcpFingerprint aggregation", t, func() {
		Convey("Client fingerprint only should fill client fields and MTU", func() {
			fp := buildTcpFingerprint(&StreamMeta{
				ClientFingerprint: &n.PacketTCPFingerprint{
					OS: "Linux 3.1-3.10", OSQuality: 0.9, Sig: "4:64:0", MTU: 1500,
				},
			})
			So(fp.ClientOS, ShouldEqual, "Linux 3.1-3.10")
			So(fp.ClientOSQuality, ShouldAlmostEqual, 0.9, 1e-6)
			So(fp.ClientSig, ShouldEqual, "4:64:0")
			So(fp.MTU, ShouldEqual, uint16(1500))
			So(fp.ServerOS, ShouldEqual, "")
		})

		Convey("Server fingerprint only should fill server fields and fallback MTU", func() {
			fp := buildTcpFingerprint(&StreamMeta{
				ServerFingerprint: &n.PacketTCPFingerprint{
					OS: "Linux 3.x", OSQuality: 0.8, Sig: "4:64:0", MTU: 1400,
				},
			})
			So(fp.ServerOS, ShouldEqual, "Linux 3.x")
			So(fp.ServerSig, ShouldEqual, "4:64:0")
			So(fp.MTU, ShouldEqual, uint16(1400))
			So(fp.ClientOS, ShouldEqual, "")
		})

		Convey("Both sides present should prefer client MTU", func() {
			fp := buildTcpFingerprint(&StreamMeta{
				ClientFingerprint: &n.PacketTCPFingerprint{OS: "Windows 7 or 8", MTU: 1500},
				ServerFingerprint: &n.PacketTCPFingerprint{OS: "Linux 3.x", MTU: 1400},
			})
			So(fp.ClientOS, ShouldEqual, "Windows 7 or 8")
			So(fp.ServerOS, ShouldEqual, "Linux 3.x")
			So(fp.MTU, ShouldEqual, uint16(1500))
		})

		Convey("No fingerprint should return zero value", func() {
			fp := buildTcpFingerprint(&StreamMeta{})
			So(fp, ShouldResemble, n.TcpFingerprint{})
		})
	})
}

// TestBuildHttpFingerprint 测试 Http 指纹分析链路（buildHttpFingerprint）：
// 请求头识别浏览器/语言、响应头识别服务端、最终推导设备类别。
// 指纹识别由 huginn cgo 绑定提供（构建期强制链接，无降级路径）。
func TestBuildHttpFingerprint(t *testing.T) {
	Convey("Test buildHttpFingerprint analysis chain", t, func() {
		// requestWithRaw 构造携带原始头字节（rawHTTPKey）的 *http.Request
		requestWithRaw := func(reqRaw, rspRaw []byte) *http.Request {
			// reqRaw 为空（无请求头）时 http.ReadRequest 无法解析，直接构造空请求
			req := &http.Request{Header: make(http.Header)}
			if len(reqRaw) > 0 {
				parsed, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(reqRaw)))
				So(err, ShouldBeNil)
				req = parsed
			}
			return req.WithContext(context.WithValue(req.Context(), rawHTTPKey{}, &rawHTTP{
				request:  reqRaw,
				response: rspRaw,
			}))
		}

		Convey("Firefox request + nginx response should be recognized as desktop", func() {
			reqRaw := []byte("GET /index.html HTTP/1.1\r\n" +
				"Host: example.com\r\n" +
				"User-Agent: Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/115.0\r\n" +
				"Accept-Language: en-US,en;q=0.5\r\n" +
				"Accept: text/html\r\n\r\n")
			rspRaw := []byte("HTTP/1.1 200 OK\r\n" +
				"Server: nginx/1.18.0\r\n" +
				"Content-Type: text/html\r\n" +
				"Content-Length: 10\r\n\r\nhello world")

			fp := buildHttpFingerprint(&StreamMeta{}, requestWithRaw(reqRaw, rspRaw))
			t.Logf("http fingerprint: %+v", fp)
			// 本单测仅验证分析链路字段搬运（原始头存在时能填充浏览器/语言/服务端并推导设备类别）；
			// 具体识别值由 huginn 指纹库决定，基线缺口（Firefox→BaiduSpider、nginx→Apache 2.x 误判）
			// 在 fingerprintCases（fingerprint_pcap_test.go）中固定并跟踪，不在此锁定误判值。
			So(fp.Browser, ShouldNotBeEmpty)
			So(fp.BrowserQuality, ShouldBeGreaterThan, 0)
			So(fp.Language, ShouldNotBeEmpty)
			So(fp.Server, ShouldNotBeEmpty)
			So(fp.ServerQuality, ShouldBeGreaterThan, 0)
			So(fp.DeviceGuess, ShouldEqual, "desktop")
		})

		Convey("Request without raw headers should return zero fingerprint", func() {
			req, err := http.ReadRequest(bufio.NewReader(bytes.NewBufferString(
				"GET / HTTP/1.1\r\nHost: x\r\n\r\n")))
			So(err, ShouldBeNil)
			fp := buildHttpFingerprint(&StreamMeta{}, req)
			So(fp, ShouldResemble, n.HttpFingerprint{})
		})

		Convey("Empty raw request with valid response should only fill server fields", func() {
			rspRaw := []byte("HTTP/1.1 200 OK\r\nServer: nginx/1.18.0\r\n\r\n")
			fp := buildHttpFingerprint(&StreamMeta{}, requestWithRaw(nil, rspRaw))
			So(fp.Browser, ShouldEqual, "")
			So(fp.Server, ShouldNotBeEmpty) // 基线：nginx 头当前被误判为 Apache 2.x
		})
	})
}
