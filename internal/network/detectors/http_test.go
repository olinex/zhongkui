package detectors

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// TestIsClient 测试 HTTP 客户端请求检测函数
// 验证 isClient 函数对各种 HTTP 请求行格式的识别能力
func TestIsClient(t *testing.T) {
	Convey("Test isClient function", t, func() {
		Convey("Should detect a valid GET request", func() {
			ok, err := isClient([]byte("GET /index.html HTTP/1.1\r\nHost: example.com\r\n\r\n"))
			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)
		})

		Convey("Should detect a valid POST request", func() {
			ok, err := isClient([]byte("POST /api/users HTTP/1.1\r\nContent-Type: application/json\r\n\r\n"))
			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)
		})

		Convey("Should detect HTTP/1.0 request", func() {
			ok, err := isClient([]byte("GET /old-page HTTP/1.0\r\n\r\n"))
			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)
		})

		Convey("Should detect all standard HTTP methods", func() {
			methods := []string{"GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS", "PATCH", "CONNECT", "TRACE"}
			for _, method := range methods {
				peek := []byte(method + " / HTTP/1.1\r\nHost: example.com\r\n\r\n")
				ok, err := isClient(peek)
				So(err, ShouldBeNil)
				So(ok, ShouldBeTrue)
			}
		})

		Convey("Should reject non-HTTP protocol data", func() {
			ok, err := isClient([]byte("SSH-2.0-OpenSSH_8.9p1"))
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)
		})

		Convey("Should reject malformed request line (no protocol version)", func() {
			ok, err := isClient([]byte("GET /path"))
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)
		})

		Convey("Should reject unsupported HTTP version (HTTP/2.0)", func() {
			ok, err := isClient([]byte("GET / HTTP/2.0\r\n"))
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)
		})

		Convey("Should reject empty data", func() {
			ok, err := isClient([]byte{})
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)
		})

		Convey("Should reject non-standard HTTP method", func() {
			ok, err := isClient([]byte("FOO / HTTP/1.1\r\n"))
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)
		})

		Convey("Should handle request with query string", func() {
			ok, err := isClient([]byte("GET /search?q=hello+world HTTP/1.1\r\nHost: example.com\r\n\r\n"))
			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)
		})
	})
}

// TestIsServer 测试 HTTP 服务器响应检测函数
// 验证 isServer 函数对各种 HTTP 响应行格式的识别能力
func TestIsServer(t *testing.T) {
	Convey("Test isServer function", t, func() {
		Convey("Should detect a valid HTTP/1.1 response", func() {
			ok, err := isServer([]byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n"))
			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)
		})

		Convey("Should detect a valid HTTP/1.0 response", func() {
			ok, err := isServer([]byte("HTTP/1.0 404 Not Found\r\n\r\n"))
			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)
		})

		Convey("Should detect 301 redirect response", func() {
			ok, err := isServer([]byte("HTTP/1.1 301 Moved Permanently\r\nLocation: https://example.com\r\n\r\n"))
			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)
		})

		Convey("Should detect 500 server error response", func() {
			ok, err := isServer([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)
		})

		Convey("Should reject data shorter than 9 bytes", func() {
			ok, err := isServer([]byte("HTTP/1.1"))
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)
		})

		Convey("Should reject response without space after version", func() {
			ok, err := isServer([]byte("HTTP/1.1200 OK\r\n"))
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)
		})

		Convey("Should reject HTTP/2 response", func() {
			ok, err := isServer([]byte("HTTP/2.0 200 OK\r\n"))
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)
		})

		Convey("Should reject non-HTTP data", func() {
			ok, err := isServer([]byte("SSH-2.0-OpenSSH_8.9p1"))
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)
		})

		Convey("Should reject client request data", func() {
			ok, err := isServer([]byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"))
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)
		})

		Convey("Should reject empty data", func() {
			ok, err := isServer([]byte{})
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)
		})
	})
}

// TestHttpDetectorDetect 测试 HttpDetector.Detect 方法
// 验证 Detect 方法正确分发到 isClient/isServer 并处理边界条件
func TestHttpDetectorDetect(t *testing.T) {
	Convey("Test HttpDetector.Detect", t, func() {
		detector := &HttpDetector{}

		Convey("Should detect client-to-server HTTP request", func() {
			ok, err := detector.Detect(nil, []byte("GET /api/data HTTP/1.1\r\nHost: localhost\r\n\r\n"), true)
			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)
		})

		Convey("Should detect server-to-client HTTP response", func() {
			ok, err := detector.Detect(nil, []byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n"), false)
			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)
		})

		Convey("Should return false when peek is too short for client", func() {
			ok, err := detector.Detect(nil, []byte("GE"), true)
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)
		})

		Convey("Should return false when peek is too short for server", func() {
			ok, err := detector.Detect(nil, []byte("HTT"), false)
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)
		})

		Convey("Should return false when peek length is exactly 15 for non-HTTP data", func() {
			ok, err := detector.Detect(nil, []byte("this is not htp"), true)
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)
		})

		Convey("Should return false for non-HTTP client data", func() {
			ok, err := detector.Detect(nil, []byte("UNKNOWN_PROTOCOL_DATA_HERE"), true)
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)
		})

		Convey("Should return false for non-HTTP server data", func() {
			ok, err := detector.Detect(nil, []byte("NOT_HTTP_RESPONSE_DATA_HERE"), false)
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)
		})
	})
}

// TestHttpDetectorExtractKeepAliveSequential 验证 Extract 在 keep-alive 长连接上的顺序消费语义：
// 共享 bufio 中连续排布的多个请求/响应，每次 Extract 恰好消费一对，后续数据不丢失。
// 回归：huginn 指纹接入曾用 bufio.NewReader(recordingReader) 二次包裹共享 reader，
// 内层 bufio 会把随后的请求/响应字节超读并滞留于临时 buffer，导致下一次 Extract 读到 EOF、
// 请求-响应配对错位，最终触发 ringbuffer 5s 读超时（context deadline exceeded）。
func TestHttpDetectorExtractKeepAliveSequential(t *testing.T) {
	Convey("Test Extract on keep-alive stream with sequential messages", t, func() {
		detector := &HttpDetector{}

		Convey("Should consume exactly one request/response pair per Extract call", func() {
			req1 := "GET /a HTTP/1.1\r\nHost: x\r\n\r\n"
			req2 := "GET /b HTTP/1.1\r\nHost: x\r\n\r\n"
			rsp1 := "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"
			rsp2 := "HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n\r\n"

			clientReader := bufio.NewReader(bytes.NewBufferString(req1 + req2))
			serverReader := bufio.NewReader(bytes.NewBufferString(rsp1 + rsp2))

			// 第一次 Extract：消费 req1 + rsp1
			clientPayload1, serverPayload1, err := detector.Extract(context.Background(), clientReader, serverReader)
			So(err, ShouldBeNil)
			req, ok := clientPayload1.(*http.Request)
			So(ok, ShouldBeTrue)
			So(req.URL.Path, ShouldEqual, "/a")
			rsp, ok := serverPayload1.(*http.Response)
			So(ok, ShouldBeTrue)
			So(rsp.StatusCode, ShouldEqual, 200)
			// 原始头字节捕获供指纹识别使用
			raw, ok := req.Context().Value(rawHTTPKey{}).(*rawHTTP)
			So(ok, ShouldBeTrue)
			So(string(raw.request), ShouldContainSubstring, "GET /a")
			So(string(raw.response), ShouldContainSubstring, "200 OK")
			// 只捕获头部字节，不得混入紧随其后的第二个请求
			So(string(raw.request), ShouldNotContainSubstring, "GET /b")
			So(string(raw.response), ShouldNotContainSubstring, "404")

			// 第二次 Extract：req2/rsp2 未被上一轮超读丢失，仍可完整解析
			clientPayload2, serverPayload2, err := detector.Extract(context.Background(), clientReader, serverReader)
			So(err, ShouldBeNil)
			req2p, ok := clientPayload2.(*http.Request)
			So(ok, ShouldBeTrue)
			So(req2p.URL.Path, ShouldEqual, "/b")
			rsp2p, ok := serverPayload2.(*http.Response)
			So(ok, ShouldBeTrue)
			So(rsp2p.StatusCode, ShouldEqual, 404)
		})

		Convey("Should preserve request/response pairing under over-read", func() {
			// 单个请求 + 带头部的响应，响应体（首个分块）与头同段到达
			req := "POST /upload HTTP/1.1\r\nHost: x\r\nContent-Length: 5\r\n\r\nhello"
			rsp := "HTTP/1.1 200 OK\r\nContent-Length: 7\r\n\r\nwelcome"

			clientReader := bufio.NewReader(bytes.NewBufferString(req))
			serverReader := bufio.NewReader(bytes.NewBufferString(rsp))

			clientPayload, serverPayload, err := detector.Extract(context.Background(), clientReader, serverReader)
			So(err, ShouldBeNil)
			req1, ok := clientPayload.(*http.Request)
			So(ok, ShouldBeTrue)
			So(req1.Method, ShouldEqual, "POST")
			// 请求体应从共享 reader 中保留，供 Build 阶段读取
			body, err := readAllBody(req1.Body)
			So(err, ShouldBeNil)
			So(string(body), ShouldEqual, "hello")
			rsp1, ok := serverPayload.(*http.Response)
			So(ok, ShouldBeTrue)
			So(rsp1.StatusCode, ShouldEqual, 200)
			body, err = readAllBody(rsp1.Body)
			So(err, ShouldBeNil)
			So(string(body), ShouldEqual, "welcome")
		})

		Convey("Should preserve original header order for fingerprinting", func() {
			// p0f HTTP 指纹依赖原始头顺序（horder 维度）；
			// http.Header 是 map，Header.Write 按 key 字典序输出会破坏顺序，
			// 因此指纹输入必须来自原始字节流，而非重新序列化。
			req := "GET / HTTP/1.1\r\nZ-Last: 1\r\nA-First: 2\r\nHost: x\r\n\r\n"
			clientReader := bufio.NewReader(bytes.NewBufferString(req))
			serverReader := bufio.NewReader(bytes.NewBufferString("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"))

			clientPayload, _, err := detector.Extract(context.Background(), clientReader, serverReader)
			So(err, ShouldBeNil)
			req1, ok := clientPayload.(*http.Request)
			So(ok, ShouldBeTrue)
			raw, ok := req1.Context().Value(rawHTTPKey{}).(*rawHTTP)
			So(ok, ShouldBeTrue)
			// 原始顺序：Z-Last 必须先于 A-First 出现（与字典序相反）
			zIdx := bytes.Index(raw.request, []byte("Z-Last"))
			aIdx := bytes.Index(raw.request, []byte("A-First"))
			So(zIdx >= 0, ShouldBeTrue)
			So(aIdx >= 0, ShouldBeTrue)
			So(zIdx < aIdx, ShouldBeTrue)
		})
	})
}

// readAllBody 读取并关闭响应体，测试辅助函数
func readAllBody(r io.ReadCloser) ([]byte, error) {
	defer r.Close()
	return io.ReadAll(r)
}
