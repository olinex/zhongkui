package samples

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"

	n "zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/network/detectors"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// init 注册测试用的 HTTP 协议检测器
// 生产环境通过 DI 容器在 initDetectors() 中注册，测试环境需在此手动注册
func init() {
	detectors.RegisterAppProtocolDetector(
		detectors.AppHttp,
		&testDetector{
			HttpDetector: &detectors.HttpDetector{},
		},
	)
}

// testDetector 测试用 HTTP 协议检测器
// 嵌入检测逻辑复用生产代码，仅覆盖 Build 阶段（不依赖 S3 存储与加密密钥）
type testDetector struct {
	*detectors.HttpDetector
}

func (d *testDetector) Build(
	ctx context.Context,
	baseMeta *n.BaseMeta,
	streamMeta *detectors.StreamMeta,
	streamIndex uint32,
	clientPayload, serverPayload any,
	buildTailer detectors.BuildTailer,
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
		baseMeta, streamMeta, streamIndex,
		request, response,
	)
}

func buildTestHttpTransport(
	meta *n.BaseMeta,
	streamMeta *detectors.StreamMeta,
	streamIndex uint32,
	request *http.Request, response *http.Response,
) (*n.HttpTransport, error) {
	if meta == nil {
		return nil, errors.New("meta is nil")
	}
	var hBase n.HttpBase
	hBase.ClientIP = serializer.IPAddr(streamMeta.ClientAddr)
	hBase.ClientPort = streamMeta.ClientPort
	hBase.ClientISN = streamMeta.ClientISN
	hBase.ServerIP = serializer.IPAddr(streamMeta.ServerAddr)
	hBase.ServerPort = streamMeta.ServerPort
	hBase.ServerISN = streamMeta.ServerISN
	hBase.StreamIndex = streamIndex

	if streamMeta.ClientAddr.To4() != nil {
		hBase.IPVersion = 4 // layers.IPProtocolIPv4
	} else {
		hBase.IPVersion = 6 // layers.IPProtocolIPv6
	}

	var uri n.HttpURI
	var requestBody n.HttpRequestBody
	var requestHeaders n.HttpRequestHeaders
	if request != nil {
		if u := request.URL; u != nil {
			uri = n.HttpURI{
				URL:    u.String(),
				Scheme: u.Scheme,
				Host:   u.Host,
				Path:   u.Path,
			}
		}
		requestBody = n.HttpRequestBody{
			Method:  request.Method,
			ReqBody: readAllBody(request.Body),
		}
		requestHeaders = *n.NewHttpRequestHeaders(request)
		hBase.HttpVersion = request.Proto
	}

	var responseBody n.HttpResponseBody
	var responseHeaders n.HttpResponseHeaders
	if response != nil {
		responseBody = n.HttpResponseBody{
			RspBody: readBodyWithEncoding(response.Body, response.Header.Get("Content-Encoding")),
		}
		responseHeaders = *n.NewHttpResponseHeaders(response)
	}

	return &n.HttpTransport{
		BaseMeta:            *meta,
		HttpBase:            hBase,
		HttpURI:             uri,
		HttpRequestBody:     requestBody,
		HttpRequestHeaders:  requestHeaders,
		HttpResponseBody:    responseBody,
		HttpResponseHeaders: responseHeaders,
	}, nil
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
