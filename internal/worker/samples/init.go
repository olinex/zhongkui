package samples

import (
	"bufio"
	"compress/gzip"
	"context"
	"io"
	"net"
	"net/http"

	"github.com/gopacket/gopacket/layers"
	n "zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/network/detectors"
	"zhongkui.com/zhongkui/internal/network/tcp"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// init 注册测试用的 HTTP 协议检测器
// 生产环境通过 DI 容器在 initDetectors() 中注册，测试环境需在此手动注册
func init() {
	tcp.RegisterAppProtocolDetector(
		detectors.AppHTTP,
		&testDetector{},
	)
}

// testDetector 测试用 HTTP 协议检测器
// Detect/Extract 复用生产代码逻辑，Build 阶段不依赖 S3 存储
type testDetector struct{}

func (d *testDetector) Detect(ctx context.Context, peek []byte, c2s bool) (ok bool, err error) {
	if len(peek) < 15 {
		return false, nil
	}
	if c2s {
		return isClient(peek), nil
	}
	return isServer(peek), nil
}

func (d *testDetector) Extract(
	ctx context.Context,
	clientReader, serverReader *bufio.Reader,
) (clientPayload, serverPayload any, err error) {
	var req *http.Request
	var rsp *http.Response

	// 先读取请求，再读取响应。将请求对象传给 http.ReadResponse，
	// 当请求方法为 HEAD 时，Go 标准库会跳过响应体的读取，
	// 避免因 Content-Length 不匹配（HEAD 响应不应有 body）而错误消费后续流量。
	if req, err = http.ReadRequest(clientReader); err != nil {
		return nil, nil, errors.WithStack(err)
	}
	if rsp, err = http.ReadResponse(serverReader, req); err != nil {
		return nil, nil, errors.WithStack(err)
	}
	return req, rsp, nil
}

func (d *testDetector) Build(
	ctx context.Context,
	meta *n.BaseMeta,
	clientAddr, serverAddr net.IP,
	clientPort, serverPort layers.TCPPort,
	clientFrameStack, serverFrameStack n.FrameStack,
	clientPayload, serverPayload any,
) (app any, err error) {
	request, ok := clientPayload.(*http.Request)
	if !ok {
		return nil, errors.New("clientPayload is not *http.Request")
	}
	response, ok := serverPayload.(*http.Response)
	if !ok {
		return nil, errors.New("serverPayload is not *http.Response")
	}
	return buildTestHttpTransport(
		meta,
		clientAddr, serverAddr,
		clientPort, serverPort,
		clientFrameStack, serverFrameStack,
		request, response,
	)
}

func buildTestHttpTransport(
	meta *n.BaseMeta,
	clientAddr, serverAddr net.IP,
	clientPort, serverPort layers.TCPPort,
	clientFrameStack, serverFrameStack n.FrameStack,
	request *http.Request, response *http.Response,
) (*n.HttpTransport, error) {
	if meta == nil {
		return nil, errors.New("meta is nil")
	}
	var hBase n.HttpBase
	hBase.ClientIP = serializer.IPAddr(clientAddr)
	hBase.ClientPort = clientPort
	hBase.ServerIP = serializer.IPAddr(serverAddr)
	hBase.ServerPort = serverPort
	if clientAddr.To4() != nil {
		hBase.IPVersion = layers.IPProtocolIPv4
	} else {
		hBase.IPVersion = layers.IPProtocolIPv6
	}

	var uri n.HttpURI
	var requestBody n.HttpRequestBody
	var requestHeaders n.HttpRequestHeaders
	if request != nil && len(clientFrameStack) != 0 {
		if u := request.URL; u != nil {
			uri = n.HttpURI{
				URL:    u.String(),
				Scheme: u.Scheme,
				Host:   u.Host,
				Path:   u.Path,
			}
		}
		startTime := clientFrameStack[0].Timestamp
		endTime := clientFrameStack[len(clientFrameStack)-1].Timestamp
		requestBody = n.HttpRequestBody{
			Method:      request.Method,
			ReqBody:     readAllBody(request.Body),
			ReqTime:     startTime,
			ReqDuration: endTime.Sub(startTime),
		}
		requestHeaders = *n.NewHttpRequestHeaders(request)
		hBase.HttpVersion = request.Proto
	}

	var responseBody n.HttpResponseBody
	var responseHeaders n.HttpResponseHeaders
	if response != nil && len(serverFrameStack) != 0 {
		startTime := serverFrameStack[0].Timestamp
		endTime := serverFrameStack[len(serverFrameStack)-1].Timestamp
		responseBody = n.HttpResponseBody{
			RspBody:     readBodyWithEncoding(response.Body, response.Header.Get("Content-Encoding")),
			RspTime:     startTime,
			RspDuration: endTime.Sub(startTime),
		}
		responseHeaders = *n.NewHttpResponseHeaders(response)
	}

	return &n.HttpTransport{
		BaseMeta:            *meta,
		FusedMeta:           n.FusedMeta{FrameStack: sortedFrameStack(clientFrameStack, serverFrameStack)},
		HttpBase:            hBase,
		HttpURI:             uri,
		HttpRequestBody:     requestBody,
		HttpRequestHeaders:  requestHeaders,
		HttpResponseBody:    responseBody,
		HttpResponseHeaders: responseHeaders,
	}, nil
}

// isClient 检查数据是否为 HTTP 请求
func isClient(peek []byte) bool {
	methods := []string{"GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS", "PATCH", "CONNECT", "TRACE"}
	for _, method := range methods {
		if len(peek) > len(method) && string(peek[:len(method)]) == method && peek[len(method)] == ' ' {
			return true
		}
	}
	return false
}

// isServer 检查数据是否为 HTTP 响应
func isServer(peek []byte) bool {
	if len(peek) < 9 {
		return false
	}
	prefix := string(peek[:8])
	return (prefix == "HTTP/1.0" || prefix == "HTTP/1.1") && peek[8] == ' '
}

func readAllBody(body io.ReadCloser) string {
	return readBodyWithEncoding(body, "")
}

func readBodyWithEncoding(body io.ReadCloser, contentEncoding string) string {
	if body == nil {
		return ""
	}
	defer body.Close()
	var reader io.Reader = body
	if contentEncoding == "gzip" {
		if gzipReader, err := gzip.NewReader(body); err == nil {
			defer gzipReader.Close()
			reader = gzipReader
		}
	}
	data, _ := io.ReadAll(reader)
	return string(data)
}

func sortedFrameStack(clientStack, serverStack n.FrameStack) n.FrameStack {
	result := make(n.FrameStack, 0, len(clientStack)+len(serverStack))
	result = append(result, clientStack...)
	result = append(result, serverStack...)
	return result
}
