package detectors

import (
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
