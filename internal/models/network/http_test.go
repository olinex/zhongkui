package network

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	. "github.com/smartystreets/goconvey/convey"
	"gorm.io/datatypes"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// TestHttpBase_Basic 测试HttpBase结构体的基本功能
// 验证HTTP基础信息的字段设置和JSON序列化
func TestHttpBase_Basic(t *testing.T) {
	Convey("Test HttpBase basic functionality", t, func() {
		Convey("Should create HttpBase with valid network information", func() {
			clientIP := net.ParseIP("192.168.1.100")
			serverIP := net.ParseIP("10.0.0.1")

			httpBase := HttpBase{
				ClientIP:    serializer.IPAddr(clientIP),
				ClientPort:  54321,
				ServerIP:    serializer.IPAddr(serverIP),
				ServerPort:  80,
				IPVersion:   layers.IPProtocolIPv4,
				HttpVersion: "HTTP/1.1",
			}

			So(httpBase.ClientIP.String(), ShouldEqual, "192.168.1.100")
			So(httpBase.ClientPort, ShouldEqual, layers.TCPPort(54321))
			So(httpBase.ServerIP.String(), ShouldEqual, "10.0.0.1")
			So(httpBase.ServerPort, ShouldEqual, layers.TCPPort(80))
			So(httpBase.IPVersion, ShouldEqual, layers.IPProtocolIPv4)
			So(httpBase.HttpVersion, ShouldEqual, "HTTP/1.1")
		})

		Convey("Should support IPv6 addresses", func() {
			clientIP := net.ParseIP("2001:db8::1")
			serverIP := net.ParseIP("2001:db8::2")

			httpBase := HttpBase{
				ClientIP:    serializer.IPAddr(clientIP),
				ClientPort:  54321,
				ServerIP:    serializer.IPAddr(serverIP),
				ServerPort:  443,
				IPVersion:   layers.IPProtocolIPv6,
				HttpVersion: "HTTP/2.0",
			}

			So(httpBase.ClientIP.String(), ShouldEqual, "2001:db8::1")
			So(httpBase.ServerIP.String(), ShouldEqual, "2001:db8::2")
			So(httpBase.IPVersion, ShouldEqual, layers.IPProtocolIPv6)
			So(httpBase.HttpVersion, ShouldEqual, "HTTP/2.0")
		})

		Convey("Should support JSON serialization and deserialization", func() {
			clientIP := net.ParseIP("192.168.1.100")
			serverIP := net.ParseIP("10.0.0.1")

			original := HttpBase{
				ClientIP:    serializer.IPAddr(clientIP),
				ClientPort:  54321,
				ServerIP:    serializer.IPAddr(serverIP),
				ServerPort:  8080,
				IPVersion:   layers.IPProtocolIPv4,
				HttpVersion: "HTTP/1.1",
			}

			jsonData, err := json.Marshal(original)
			So(err, ShouldBeNil)

			var decoded HttpBase
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)

			So(decoded.ClientIP.String(), ShouldEqual, "192.168.1.100")
			So(decoded.ClientPort, ShouldEqual, layers.TCPPort(54321))
			So(decoded.ServerIP.String(), ShouldEqual, "10.0.0.1")
			So(decoded.ServerPort, ShouldEqual, layers.TCPPort(8080))
			So(decoded.IPVersion, ShouldEqual, layers.IPProtocolIPv4)
			So(decoded.HttpVersion, ShouldEqual, "HTTP/1.1")
		})
	})
}

// TestHttpURI_Basic 测试HttpURI结构体的基本功能
// 验证URI各个组成部分的解析和存储
func TestHttpURI_Basic(t *testing.T) {
	Convey("Test HttpURI basic functionality", t, func() {
		Convey("Should create HttpURI with complete URL components", func() {
			queryJSON := datatypes.JSON(`{"page":"1","limit":"10"}`)
			uri := HttpURI{
				URL:      "https://example.com/api/users?page=1&limit=10#section",
				Scheme:   "https",
				Host:     "example.com",
				Path:     "/api/users",
				Queries:  queryJSON,
				Fragment: "section",
			}

			So(uri.URL, ShouldEqual, "https://example.com/api/users?page=1&limit=10#section")
			So(uri.Scheme, ShouldEqual, "https")
			So(uri.Host, ShouldEqual, "example.com")
			So(uri.Path, ShouldEqual, "/api/users")
			So(string(uri.Queries), ShouldEqual, `{"page":"1","limit":"10"}`)
			So(uri.Fragment, ShouldEqual, "section")
		})

		Convey("Should handle URLs without query parameters or fragments", func() {
			emptyJSON := datatypes.JSON(`{}`)
			uri := HttpURI{
				URL:      "http://localhost:8080/",
				Scheme:   "http",
				Host:     "localhost:8080",
				Path:     "/",
				Queries:  emptyJSON,
				Fragment: "",
			}

			So(uri.URL, ShouldEqual, "http://localhost:8080/")
			So(uri.Scheme, ShouldEqual, "http")
			So(uri.Host, ShouldEqual, "localhost:8080")
			So(uri.Path, ShouldEqual, "/")
			So(string(uri.Queries), ShouldEqual, "{}")
			So(uri.Fragment, ShouldBeEmpty)
		})

		Convey("Should support JSON serialization with complex queries", func() {
			queryJSON := datatypes.JSON(`{"search":"test","sort":"desc","filter":{"status":"active"}}`)
			original := HttpURI{
				URL:      "https://api.example.com/search?search=test&sort=desc&filter[status]=active",
				Scheme:   "https",
				Host:     "api.example.com",
				Path:     "/search",
				Queries:  queryJSON,
				Fragment: "",
			}

			jsonData, err := json.Marshal(original)
			So(err, ShouldBeNil)

			var decoded HttpURI
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)

			So(decoded.URL, ShouldEqual, original.URL)
			So(decoded.Scheme, ShouldEqual, "https")
			So(decoded.Host, ShouldEqual, "api.example.com")
			So(decoded.Path, ShouldEqual, "/search")
			So(string(decoded.Queries), ShouldEqual, string(queryJSON))
			So(decoded.Fragment, ShouldBeEmpty)
		})
	})
}

// TestHttpTLS_Basic 测试HttpTLS结构体的基本功能
// 验证TLS安全连接信息的存储和验证
func TestHttpTLS_Basic(t *testing.T) {
	Convey("Test HttpTLS basic functionality", t, func() {
		Convey("Should create HttpTLS with TLS 1.2 information", func() {
			tlsInfo := HttpTLS{
				TLSVersion:   "TLS 1.2",
				TLSCipher:    "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
				TLSEncrypted: true,
			}

			So(tlsInfo.TLSVersion, ShouldEqual, "TLS 1.2")
			So(tlsInfo.TLSCipher, ShouldEqual, "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256")
			So(tlsInfo.TLSEncrypted, ShouldBeTrue)
		})

		Convey("Should create HttpTLS with TLS 1.3 information", func() {
			tlsInfo := HttpTLS{
				TLSVersion:   "TLS 1.3",
				TLSCipher:    "TLS_AES_256_GCM_SHA384",
				TLSEncrypted: true,
			}

			So(tlsInfo.TLSVersion, ShouldEqual, "TLS 1.3")
			So(tlsInfo.TLSCipher, ShouldEqual, "TLS_AES_256_GCM_SHA384")
			So(tlsInfo.TLSEncrypted, ShouldBeTrue)
		})

		Convey("Should handle non-TLS (HTTP) connections", func() {
			tlsInfo := HttpTLS{
				TLSVersion:   "",
				TLSCipher:    "",
				TLSEncrypted: false,
			}

			So(tlsInfo.TLSVersion, ShouldBeEmpty)
			So(tlsInfo.TLSCipher, ShouldBeEmpty)
			So(tlsInfo.TLSEncrypted, ShouldBeFalse)
		})
	})
}

// TestHttpRequest_Basic 测试HttpRequest结构体的基本功能
// 验证HTTP请求的完整信息存储和序列化
func TestHttpRequest_Basic(t *testing.T) {
	Convey("Test HttpRequest basic functionality", t, func() {
		Convey("Should create complete HTTP GET request", func() {
			reqTime := time.Now()
			headersJSON := datatypes.JSON(`{"X-Request-ID":"12345","X-API-Key":"secret"}`)

			request := HttpRequest{
				Method:           "GET",
				UserAgent:        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
				Referer:          "https://google.com",
				Accept:           "application/json",
				AcceptEncoding:   "gzip, deflate",
				AcceptLanguage:   "zh-CN,zh;q=0.9,en;q=0.8",
				Connection:       "keep-alive",
				Cookie:           "session=abc123; user=john",
				Authorization:    "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
				ReqContentType:   "",
				ReqContentLength: 0,
				ReqHeaders:       headersJSON,
				ReqBody:          []byte{},
				ReqTime:          reqTime,
				ReqDuration:      150 * time.Millisecond,
				IsReqCompleted:   true,
			}

			So(request.Method, ShouldEqual, "GET")
			So(request.UserAgent, ShouldContainSubstring, "Mozilla/5.0")
			So(request.Referer, ShouldEqual, "https://google.com")
			So(request.Accept, ShouldEqual, "application/json")
			So(request.AcceptEncoding, ShouldEqual, "gzip, deflate")
			So(request.AcceptLanguage, ShouldEqual, "zh-CN,zh;q=0.9,en;q=0.8")
			So(request.Connection, ShouldEqual, "keep-alive")
			So(request.Cookie, ShouldContainSubstring, "session=abc123")
			So(request.Authorization, ShouldStartWith, "Bearer")
			So(string(request.ReqHeaders), ShouldEqual, string(headersJSON))
			So(request.ReqTime.Unix(), ShouldEqual, reqTime.Unix())
			So(request.ReqDuration, ShouldEqual, 150*time.Millisecond)
			So(request.IsReqCompleted, ShouldBeTrue)
		})

		Convey("Should create complete HTTP POST request with body", func() {
			reqTime := time.Now()
			headersJSON := datatypes.JSON(`{"Content-Type":"application/json","Authorization":"Basic dXNlcjpwYXNz"}`)
			reqBody := []byte(`{"name":"John","age":30,"email":"john@example.com"}`)

			request := HttpRequest{
				Method:           "POST",
				UserAgent:        "curl/7.68.0",
				Referer:          "",
				Accept:           "*/*",
				AcceptEncoding:   "",
				AcceptLanguage:   "",
				Connection:       "close",
				Cookie:           "",
				Authorization:    "Basic dXNlcjpwYXNz",
				ReqContentType:   "application/json",
				ReqContentLength: uint32(len(reqBody)),
				ReqHeaders:       headersJSON,
				ReqBody:          reqBody,
				ReqTime:          reqTime,
				ReqDuration:      500 * time.Millisecond,
				IsReqCompleted:   true,
			}

			So(request.Method, ShouldEqual, "POST")
			So(request.UserAgent, ShouldEqual, "curl/7.68.0")
			So(request.ReqContentType, ShouldEqual, "application/json")
			So(request.ReqContentLength, ShouldEqual, uint32(len(reqBody)))
			So(string(request.ReqBody), ShouldEqual, string(reqBody))
			So(request.ReqDuration, ShouldEqual, 500*time.Millisecond)
		})

		Convey("Should support JSON serialization for complex requests", func() {
			reqTime := time.Now()
			headersJSON := datatypes.JSON(`{"Custom-Header":"value","Another-Header":"test"}`)
			reqBody := []byte(`{"action":"create","data":{"id":1}}`)

			original := HttpRequest{
				Method:           "PUT",
				UserAgent:        "CustomClient/1.0",
				Referer:          "https://app.example.com",
				Accept:           "application/json",
				AcceptEncoding:   "gzip",
				AcceptLanguage:   "en-US",
				Connection:       "keep-alive",
				Cookie:           "token=xyz789",
				Authorization:    "",
				ReqContentType:   "application/json",
				ReqContentLength: uint32(len(reqBody)),
				ReqHeaders:       headersJSON,
				ReqBody:          reqBody,
				ReqTime:          reqTime,
				ReqDuration:      200 * time.Millisecond,
				IsReqCompleted:   true,
			}

			jsonData, err := json.Marshal(original)
			So(err, ShouldBeNil)

			var decoded HttpRequest
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)

			So(decoded.Method, ShouldEqual, "PUT")
			So(decoded.UserAgent, ShouldEqual, "CustomClient/1.0")
			So(decoded.ReqContentLength, ShouldEqual, uint32(len(reqBody)))
			So(string(decoded.ReqBody), ShouldEqual, string(reqBody))
			So(decoded.ReqDuration, ShouldEqual, 200*time.Millisecond)
			So(decoded.IsReqCompleted, ShouldBeTrue)
		})

		Convey("Should handle incomplete requests", func() {
			request := HttpRequest{
				Method:         "GET",
				IsReqCompleted: false,
			}

			So(request.Method, ShouldEqual, "GET")
			So(request.IsReqCompleted, ShouldBeFalse)
		})
	})
}

// TestHttpResponse_Basic 测试HttpResponse结构体的基本功能
// 验证HTTP响应的完整信息存储和序列化
func TestHttpResponse_Basic(t *testing.T) {
	Convey("Test HttpResponse basic functionality", t, func() {
		Convey("Should create successful HTTP 200 response", func() {
			rspTime := time.Now()
			headersJSON := datatypes.JSON(`{"Server":"nginx/1.18.0","Date":"Mon, 19 Apr 2026 10:00:00 GMT"}`)
			rspBody := []byte(`{"status":"success","data":{"id":123}}`)

			response := HttpResponse{
				Server:           "nginx/1.18.0",
				SetCookie:        "session=abc123; Path=/; HttpOnly",
				Location:         "",
				CacheControl:     "no-cache",
				ContentEncoding:  "gzip",
				RspContentType:   "application/json",
				RspContentLength: uint32(len(rspBody)),
				RspHeaders:       headersJSON,
				RspBody:          rspBody,
				RspTime:          rspTime,
				RspDuration:      100 * time.Millisecond,
				IsRspCompleted:   true,
			}

			So(response.Server, ShouldEqual, "nginx/1.18.0")
			So(response.SetCookie, ShouldContainSubstring, "session=abc123")
			So(response.CacheControl, ShouldEqual, "no-cache")
			So(response.ContentEncoding, ShouldEqual, "gzip")
			So(response.RspContentType, ShouldEqual, "application/json")
			So(response.RspContentLength, ShouldEqual, uint32(len(rspBody)))
			So(string(response.RspBody), ShouldEqual, string(rspBody))
			So(response.RspTime.Unix(), ShouldEqual, rspTime.Unix())
			So(response.RspDuration, ShouldEqual, 100*time.Millisecond)
			So(response.IsRspCompleted, ShouldBeTrue)
		})

		Convey("Should create HTTP 302 redirect response", func() {
			rspTime := time.Now()
			response := HttpResponse{
				Server:           "Apache/2.4.41",
				SetCookie:        "",
				Location:         "https://new.example.com/login",
				CacheControl:     "no-store",
				ContentEncoding:  "",
				RspContentType:   "text/html",
				RspContentLength: 0,
				RspHeaders:       datatypes.JSON(`{}`),
				RspBody:          []byte{},
				RspTime:          rspTime,
				RspDuration:      50 * time.Millisecond,
				IsRspCompleted:   true,
			}

			So(response.Server, ShouldEqual, "Apache/2.4.41")
			So(response.Location, ShouldEqual, "https://new.example.com/login")
			So(response.CacheControl, ShouldEqual, "no-store")
			So(response.RspContentType, ShouldEqual, "text/html")
			So(response.RspDuration, ShouldEqual, 50*time.Millisecond)
		})

		Convey("Should create HTTP 404 error response", func() {
			rspBody := []byte(`<!DOCTYPE html><html><body><h1>404 Not Found</h1></body></html>`)
			response := HttpResponse{
				Server:           "CustomServer/1.0",
				SetCookie:        "",
				Location:         "",
				CacheControl:     "no-cache",
				ContentEncoding:  "",
				RspContentType:   "text/html",
				RspContentLength: uint32(len(rspBody)),
				RspHeaders:       datatypes.JSON(`{"X-Powered-By":"Go"}`),
				RspBody:          rspBody,
				RspTime:          time.Now(),
				RspDuration:      80 * time.Millisecond,
				IsRspCompleted:   true,
			}

			So(response.Server, ShouldEqual, "CustomServer/1.0")
			So(response.RspContentType, ShouldEqual, "text/html")
			So(response.RspContentLength, ShouldEqual, uint32(len(rspBody)))
			So(string(response.RspBody), ShouldContainSubstring, "404 Not Found")
		})

		Convey("Should support JSON serialization for complex responses", func() {
			rspTime := time.Now()
			headersJSON := datatypes.JSON(`{"X-Custom-Header":"value","X-RateLimit-Limit":"1000"}`)
			rspBody := []byte(`{"items":[{"id":1,"name":"Item 1"},{"id":2,"name":"Item 2"}]}`)

			original := HttpResponse{
				Server:           "Go/1.25.1",
				SetCookie:        "auth=token123; Secure; HttpOnly",
				Location:         "",
				CacheControl:     "public, max-age=3600",
				ContentEncoding:  "gzip",
				RspContentType:   "application/json",
				RspContentLength: uint32(len(rspBody)),
				RspHeaders:       headersJSON,
				RspBody:          rspBody,
				RspTime:          rspTime,
				RspDuration:      120 * time.Millisecond,
				IsRspCompleted:   true,
			}

			jsonData, err := json.Marshal(original)
			So(err, ShouldBeNil)

			var decoded HttpResponse
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)

			So(decoded.Server, ShouldEqual, "Go/1.25.1")
			So(decoded.SetCookie, ShouldContainSubstring, "auth=token123")
			So(decoded.CacheControl, ShouldEqual, "public, max-age=3600")
			So(decoded.RspContentLength, ShouldEqual, uint32(len(rspBody)))
			So(string(decoded.RspBody), ShouldEqual, string(rspBody))
			So(decoded.IsRspCompleted, ShouldBeTrue)
		})

		Convey("Should handle incomplete responses", func() {
			response := HttpResponse{
				Server:         "nginx",
				IsRspCompleted: false,
			}

			So(response.Server, ShouldEqual, "nginx")
			So(response.IsRspCompleted, ShouldBeFalse)
		})
	})
}

// TestHttpTransport_Structure 测试HttpTransport结构体的组合关系
// 验证完整的HTTP传输数据模型的组合和功能
func TestHttpTransport_Structure(t *testing.T) {
	Convey("Test HttpTransport structure", t, func() {
		Convey("Should create complete HttpTransport with all components", func() {
			sessionID, _ := serializer.HexUUIDv4FromString("12345678-1234-1234-1234-123456789012")
			clientIP := net.ParseIP("192.168.1.100")
			serverIP := net.ParseIP("10.0.0.1")
			reqTime := time.Now()
			rspTime := reqTime.Add(100 * time.Millisecond)

			queryJSON := datatypes.JSON(`{"id":"123"}`)
			reqHeadersJSON := datatypes.JSON(`{"User-Agent":"TestClient","Accept":"application/json"}`)
			rspHeadersJSON := datatypes.JSON(`{"Server":"TestServer","Content-Type":"application/json"}`)
			reqBody := []byte(`{"action":"test"}`)
			rspBody := []byte(`{"result":"success"}`)

			transport := HttpTransport{
				BaseMeta: BaseMeta{
					Namespace: "test-namespace",
					Capture:   "eth0",
					Version:   "v1.0",
					SessionID: sessionID,
				},
				FusedMeta: FusedMeta{
					FrameStack: FrameStack{
						{
							Timestamp: reqTime,
							LayerStack: LayerStack{
								{
									Level:   LayerLevelApplication,
									Proto:   gopacket.LayerTypePayload,
									Headers: map[string]any{"protocol": "http"},
								},
							},
						},
					},
				},
				HttpBase: HttpBase{
					ClientIP:    serializer.IPAddr(clientIP),
					ClientPort:  54321,
					ServerIP:    serializer.IPAddr(serverIP),
					ServerPort:  80,
					IPVersion:   layers.IPProtocolIPv4,
					HttpVersion: "HTTP/1.1",
				},
				HttpURI: HttpURI{
					URL:      "http://example.com/api/test?id=123",
					Scheme:   "http",
					Host:     "example.com",
					Path:     "/api/test",
					Queries:  queryJSON,
					Fragment: "",
				},
				HttpRequest: HttpRequest{
					Method:           "POST",
					UserAgent:        "TestClient/1.0",
					Referer:          "",
					Accept:           "application/json",
					AcceptEncoding:   "gzip",
					AcceptLanguage:   "en-US",
					Connection:       "keep-alive",
					Cookie:           "",
					Authorization:    "",
					ReqContentType:   "application/json",
					ReqContentLength: uint32(len(reqBody)),
					ReqHeaders:       reqHeadersJSON,
					ReqBody:          reqBody,
					ReqTime:          reqTime,
					ReqDuration:      50 * time.Millisecond,
					IsReqCompleted:   true,
				},
				HttpResponse: HttpResponse{
					Server:           "TestServer/1.0",
					SetCookie:        "",
					Location:         "",
					CacheControl:     "no-cache",
					ContentEncoding:  "gzip",
					RspContentType:   "application/json",
					RspContentLength: uint32(len(rspBody)),
					RspHeaders:       rspHeadersJSON,
					RspBody:          rspBody,
					RspTime:          rspTime,
					RspDuration:      50 * time.Millisecond,
					IsRspCompleted:   true,
				},
			}

			// 验证基础元数据
			So(transport.Namespace, ShouldEqual, "test-namespace")
			So(transport.Capture, ShouldEqual, "eth0")
			So(transport.Version, ShouldEqual, "v1.0")
			So(transport.SessionID.String(), ShouldEqual, "12345678-1234-1234-1234-123456789012")

			// 验证帧堆栈
			So(len(transport.FrameStack), ShouldEqual, 1)
			So(transport.FrameStack[0].Timestamp.Unix(), ShouldEqual, reqTime.Unix())

			// 验证HTTP基础信息
			So(transport.ClientIP.String(), ShouldEqual, "192.168.1.100")
			So(transport.ServerIP.String(), ShouldEqual, "10.0.0.1")
			So(transport.HttpVersion, ShouldEqual, "HTTP/1.1")

			// 验证URI信息
			So(transport.URL, ShouldEqual, "http://example.com/api/test?id=123")
			So(transport.Path, ShouldEqual, "/api/test")
			So(string(transport.Queries), ShouldEqual, string(queryJSON))

			// 验证请求信息
			So(transport.Method, ShouldEqual, "POST")
			So(transport.ReqContentLength, ShouldEqual, uint32(len(reqBody)))
			So(string(transport.ReqBody), ShouldEqual, string(reqBody))
			So(transport.IsReqCompleted, ShouldBeTrue)

			// 验证响应信息
			So(transport.Server, ShouldEqual, "TestServer/1.0")
			So(transport.RspContentLength, ShouldEqual, uint32(len(rspBody)))
			So(string(transport.RspBody), ShouldEqual, string(rspBody))
			So(transport.IsRspCompleted, ShouldBeTrue)
		})

		Convey("Should support JSON serialization for complete HttpTransport", func() {
			sessionID := serializer.NewHexUUIDv4()
			clientIP := net.ParseIP("10.0.0.2")
			serverIP := net.ParseIP("192.168.1.1")

			original := HttpTransport{
				BaseMeta: BaseMeta{
					Namespace: "json-test",
					Capture:   "eth1",
					Version:   "v2.0",
					SessionID: sessionID,
				},
				HttpBase: HttpBase{
					ClientIP:    serializer.IPAddr(clientIP),
					ClientPort:  12345,
					ServerIP:    serializer.IPAddr(serverIP),
					ServerPort:  443,
					IPVersion:   layers.IPProtocolIPv4,
					HttpVersion: "HTTP/2.0",
				},
				HttpURI: HttpURI{
					URL:      "https://api.example.com/v2/users",
					Scheme:   "https",
					Host:     "api.example.com",
					Path:     "/v2/users",
					Queries:  datatypes.JSON(`{}`),
					Fragment: "",
				},
				HttpRequest: HttpRequest{
					Method:         "GET",
					IsReqCompleted: true,
				},
				HttpResponse: HttpResponse{
					IsRspCompleted: true,
				},
			}

			jsonData, err := json.Marshal(original)
			So(err, ShouldBeNil)

			var decoded HttpTransport
			err = json.Unmarshal(jsonData, &decoded)
			So(err, ShouldBeNil)

			So(decoded.Namespace, ShouldEqual, "json-test")
			So(decoded.ClientIP.String(), ShouldEqual, "10.0.0.2")
			So(decoded.ServerIP.String(), ShouldEqual, "192.168.1.1")
			So(decoded.HttpVersion, ShouldEqual, "HTTP/2.0")
			So(decoded.URL, ShouldEqual, "https://api.example.com/v2/users")
			So(decoded.Method, ShouldEqual, "GET")
			So(decoded.IsReqCompleted, ShouldBeTrue)
			So(decoded.IsRspCompleted, ShouldBeTrue)
		})

		Convey("Should handle HTTPS connections with TLS information", func() {
			transport := HttpTransport{
				HttpBase: HttpBase{
					ClientPort:  54321,
					ServerPort:  443,
					HttpVersion: "HTTP/1.1",
				},
				HttpURI: HttpURI{
					Scheme: "https",
					Host:   "secure.example.com",
				},
			}

			So(transport.Scheme, ShouldEqual, "https")
			So(transport.ServerPort, ShouldEqual, layers.TCPPort(443))
			So(transport.HttpVersion, ShouldEqual, "HTTP/1.1")
		})

		Convey("Should handle partial HTTP transactions", func() {
			transport := HttpTransport{
				HttpRequest: HttpRequest{
					Method:         "GET",
					IsReqCompleted: false,
				},
				HttpResponse: HttpResponse{
					IsRspCompleted: false,
				},
			}

			So(transport.Method, ShouldEqual, "GET")
			So(transport.IsReqCompleted, ShouldBeFalse)
			So(transport.IsRspCompleted, ShouldBeFalse)
		})
	})
}

// TestHttpTransport_EdgeCases 测试HttpTransport的边缘情况
// 验证异常情况和边界条件的处理
func TestHttpTransport_EdgeCases(t *testing.T) {
	Convey("Test HttpTransport edge cases", t, func() {
		Convey("Should handle empty or missing fields", func() {
			transport := HttpTransport{}

			So(transport.Namespace, ShouldBeEmpty)
			So(transport.ClientPort, ShouldEqual, layers.TCPPort(0))
			So(transport.Method, ShouldBeEmpty)
			So(transport.URL, ShouldBeEmpty)
			So(transport.Server, ShouldBeEmpty)
			So(transport.IsReqCompleted, ShouldBeFalse)
			So(transport.IsRspCompleted, ShouldBeFalse)
		})

		Convey("Should handle very large request/response bodies", func() {
			largeBody := make([]byte, 1024*1024) // 1MB body
			for i := range largeBody {
				largeBody[i] = byte(i % 256)
			}

			transport := HttpTransport{
				HttpRequest: HttpRequest{
					ReqContentLength: uint32(len(largeBody)),
					ReqBody:          largeBody,
					IsReqCompleted:   true,
				},
				HttpResponse: HttpResponse{
					RspContentLength: uint32(len(largeBody)),
					RspBody:          largeBody,
					IsRspCompleted:   true,
				},
			}

			So(transport.ReqContentLength, ShouldEqual, 1024*1024)
			So(len(transport.ReqBody), ShouldEqual, 1024*1024)
			So(transport.RspContentLength, ShouldEqual, 1024*1024)
			So(len(transport.RspBody), ShouldEqual, 1024*1024)
		})

		Convey("Should handle very long durations", func() {
			transport := HttpTransport{
				HttpRequest: HttpRequest{
					ReqDuration:    24 * time.Hour, // 24小时
					IsReqCompleted: true,
				},
				HttpResponse: HttpResponse{
					RspDuration:    30 * time.Minute, // 30分钟
					IsRspCompleted: true,
				},
			}

			So(transport.ReqDuration, ShouldEqual, 24*time.Hour)
			So(transport.RspDuration, ShouldEqual, 30*time.Minute)
		})

		Convey("Should handle complex JSON in query parameters", func() {
			complexQuery := datatypes.JSON(`{"filters":[{"field":"status","op":"eq","value":"active"},{"field":"date","op":"gte","value":"2026-01-01"}],"sort":"-created_at","page":1,"limit":50}`)

			transport := HttpTransport{
				HttpURI: HttpURI{
					Queries: complexQuery,
				},
			}

			So(string(transport.Queries), ShouldEqual, string(complexQuery))
		})

		Convey("Should handle special characters in URLs and headers", func() {
			transport := HttpTransport{
				HttpURI: HttpURI{
					URL:      "https://example.com/search?q=测试&lang=中文",
					Path:     "/search",
					Fragment: "结果",
				},
				HttpRequest: HttpRequest{
					UserAgent: "测试客户端/1.0",
					Accept:    "application/json; charset=utf-8",
				},
			}

			So(transport.URL, ShouldContainSubstring, "测试")
			So(transport.URL, ShouldContainSubstring, "中文")
			So(transport.Fragment, ShouldEqual, "结果")
			So(transport.UserAgent, ShouldContainSubstring, "测试客户端")
			So(transport.Accept, ShouldContainSubstring, "charset=utf-8")
		})
	})
}
