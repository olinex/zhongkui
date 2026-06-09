package detectors

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/gopacket/gopacket/layers"
	"golang.org/x/sync/errgroup"
	"gorm.io/datatypes"
	n "zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/network/tcp"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

const (
	// AppHTTP 表示HTTP协议。
	// 当识别到HTTP请求或响应时使用此值，表示Payload中包含HTTP协议数据。
	// HTTP协议识别基于请求行、响应行、头部字段等HTTP协议特征进行判断。
	AppHTTP tcp.AppProtocol = "http"
)

const (
	// HTTPVersion10 表示 HTTP/1.0 版本。
	HTTPVersion10 = "HTTP/1.0"
	// HTTPVersion11 表示 HTTP/1.1 版本。
	HTTPVersion11 = "HTTP/1.1"
	// 换行字符串
	NewLine = "\r\n"
)

// HttpDetector HTTP协议检测器
// 实现tcp.AppProtocolDetector接口，用于检测和提取TCP流中的HTTP协议数据
// 该检测器负责识别HTTP请求和响应，解析HTTP协议头部，并构建完整的HTTP传输对象
type HttpDetector struct{}

// isClient 判断给定的字节数据是否为HTTP客户端请求
// 该方法通过分析数据的前几个字节来判断是否为有效的HTTP请求行
// 遵循HTTP协议规范：请求行格式为 "METHOD URI HTTP/VERSION"
//
// 参数:
//   - peek: 待检测的字节数据，通常为TCP流的开头部分
//
// 返回值:
//   - ok: 如果数据符合HTTP请求格式返回true，否则返回false
//   - err: 处理过程中发生的错误，正常情况返回nil
//
// 检测逻辑:
//  1. 按换行符分割，提取第一行（请求行）
//  2. 将请求行按空格分割为3部分：方法、URI、协议版本
//  3. 验证方法是否为合法的HTTP方法（GET、POST等）
//  4. 验证协议版本是否为HTTP/1.0或HTTP/1.1
func isClient(peek []byte) (ok bool, err error) {
	// 从缓存中提取第一行
	first := bytes.SplitN(peek, []byte(NewLine), 2)
	// 尝试拆分协议
	parts := strings.SplitN(string(first[0]), " ", 3)
	if len(parts) != 3 {
		return false, nil
	}
	// 匹配 http 方法
	switch parts[0] {
	case http.MethodConnect, http.MethodDelete, http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPatch, http.MethodPost, http.MethodPut, http.MethodTrace:
	default:
		return false, nil
	}
	// 匹配 http 版本
	switch strings.TrimRight(parts[2], NewLine) {
	case HTTPVersion10, HTTPVersion11:
	default:
		return false, nil
	}
	return true, nil
}

// isServer 判断给定的字节数据是否为HTTP服务器响应
// 该方法通过分析数据的前几个字节来判断是否为有效的HTTP响应行
// 遵循HTTP协议规范：响应行格式为 "HTTP/VERSION STATUS_CODE STATUS_TEXT"
//
// 参数:
//   - peek: 待检测的字节数据，通常为TCP流的开头部分
//
// 返回值:
//   - ok: 如果数据符合HTTP响应格式返回true，否则返回false
//   - err: 处理过程中发生的错误，正常情况返回nil
//
// 检测逻辑:
//  1. 检查数据长度是否足够（至少9字节）
//  2. 检查第9个字符是否为空格（HTTP/1.x的第9个字符是空格）
//  3. 检查前8个字符是否为"HTTP/1.0"或"HTTP/1.1"
//  4. 验证响应行格式符合HTTP协议规范
func isServer(peek []byte) (ok bool, err error) {
	// 检查数据长度是否足够
	if len(peek) < 9 {
		return false, nil
	}
	// 第 9 个字符必须为空格
	if peek[8] != ' ' {
		return false, nil
	}
	// 前 8 个字符
	switch string(peek[:8]) {
	case HTTPVersion10, HTTPVersion11:
		return true, nil
	default:
		return false, nil
	}
}

// decompressRequestBody 根据 HTTP 请求的 Content-Encoding 头部自动解压请求体
// 该方法支持 gzip 压缩格式，其他压缩格式将返回原始请求体
//
// 参数:
//   - req: HTTP 请求对象，包含请求头部和原始请求体
//
// 返回值:
//   - io.ReadCloser: 解压后的请求体读取器，调用者负责关闭
//   - error: 解压过程中发生的错误，如压缩格式不支持或解压失败
//
// 支持的压缩格式:
//   - "gzip": 使用 gzip.NewReader 解压请求体
//   - 其他: 返回原始请求体，不做解压处理
func decompressRequestBody(req *http.Request) (io.ReadCloser, error) {
	switch req.Header.Get("Content-Encoding") {
	case "gzip":
		return gzip.NewReader(req.Body)
	default:
		return req.Body, nil
	}
}

// decompressResponseBody 根据 HTTP 响应的 Content-Encoding 头部自动解压响应体
// 该方法支持 gzip 压缩格式，其他压缩格式将返回原始响应体
//
// 参数:
//   - resp: HTTP 响应对象，包含响应头部和原始响应体
//
// 返回值:
//   - io.ReadCloser: 解压后的响应体读取器，调用者负责关闭
//   - error: 解压过程中发生的错误，如压缩格式不支持或解压失败
//
// 支持的压缩格式:
//   - "gzip": 使用 gzip.NewReader 解压响应体
//   - 其他: 返回原始响应体，不做解压处理
func decompressResponseBody(resp *http.Response) (io.ReadCloser, error) {
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		return gzip.NewReader(resp.Body)
	default:
		return resp.Body, nil
	}
}

// Detect 检测TCP流中的数据是否为HTTP协议
// 实现tcp.AppProtocolDetector接口的Detect方法，用于判断当前TCP流是否包含HTTP协议数据
//
// 参数:
//   - ctx: 上下文对象，用于传递取消信号和超时控制
//   - rb: 环形缓冲区，包含待检测的TCP流数据
//   - frameStack: 帧栈信息，包含数据包的捕获时间和网络层信息
//   - c2s: 方向标识，true表示客户端到服务端方向，false表示服务端到客户端方向
//
// 返回值:
//   - ok: 如果检测到HTTP协议返回true，否则返回false
//   - err: 处理过程中发生的错误，如缓冲区读取失败等
//
// 检测流程:
//  1. 检查缓冲区长度是否足够（至少15字节，用于识别HTTP协议头）
//  2. 从缓冲区中读取所有数据（不消耗缓冲区）
//  3. 根据方向调用相应的检测方法：
//     - c2s=true: 调用isClient检测HTTP请求
//     - c2s=false: 调用isServer检测HTTP响应
func (s *HttpDetector) Detect(ctx context.Context, peek []byte, c2s bool) (ok bool, err error) {
	if len(peek) < 15 {
		return false, nil
	}
	if c2s {
		return isClient(peek)
	} else {
		return isServer(peek)
	}
}

// Extract 从TCP流中提取HTTP协议数据
// 实现tcp.AppProtocolDetector接口的Extract方法，用于从已识别的HTTP流中提取完整的HTTP请求和响应对象
// 该方法同时处理客户端到服务端和服务端到客户端两个方向的HTTP数据，构建完整的HTTP会话
//
// 参数:
//   - ctx: 上下文对象，用于传递取消信号和超时控制
//   - clientRb: 客户端到服务端方向的环形缓冲区，包含HTTP请求数据
//   - serverRb: 服务端到客户端方向的环形缓冲区，包含HTTP响应数据
//   - clientFrameStack: 客户端帧栈信息，包含HTTP请求数据包的捕获时间和网络层堆栈
//   - serverFrameStack: 服务端帧栈信息，包含HTTP响应数据包的捕获时间和网络层堆栈
//
// 返回值:
//   - clientPayload: 提取的HTTP请求对象，类型为*http.Request，包含HTTP请求的所有信息
//   - serverPayload: 提取的HTTP响应对象，类型为*http.Response，包含HTTP响应的所有信息
//   - err: 提取过程中发生的错误，如协议解析失败、数据不完整、缓冲区读取失败等
//
// 提取流程:
//  1. 将客户端和服务端的环形缓冲区分别包装为bufio.Reader以便逐行读取
//  2. 首先使用http.ReadRequest从客户端缓冲区解析HTTP请求
//  3. 然后使用http.ReadResponse从服务端缓冲区解析HTTP响应，需要传入已解析的请求作为参考
//  4. 处理可能的错误情况：
//     - 客户端请求解析遇到io.EOF: 返回已解析的部分请求数据，serverPayload为nil
//     - 客户端请求解析遇到其他错误: 返回nil, nil, err，终止提取过程
//     - 服务端响应解析遇到io.EOF: 返回nil, nil, nil，表示需要更多数据
//     - 服务端响应解析遇到其他错误: 返回nil, nil, err，终止提取过程
//
// 注意事项:
//   - 该方法假设HTTP请求和响应在TCP流中是按顺序出现的（先请求后响应）
//   - 解析HTTP响应时需要已解析的HTTP请求对象，用于正确处理响应状态
//   - 错误处理策略:
//   - 客户端请求解析成功但响应不完整时，返回请求对象和nil响应
//   - 客户端请求解析失败时，立即返回错误，不尝试解析响应
//   - 服务端响应解析失败时，返回错误，即使请求已成功解析
//   - 调用者需要检查返回的clientPayload和serverPayload是否为nil来判断是否解析成功
func (s *HttpDetector) Extract(
	ctx context.Context,
	clientRb, serverRb *serializer.RingBuffer,
) (clientPayload, serverPayload any, err error) {
	clientReader := bufio.NewReader(clientRb)
	serverReader := bufio.NewReader(serverRb)
	var req *http.Request
	var rsp *http.Response
	if req, err = http.ReadRequest(clientReader); err == io.EOF {
		err = errors.WithStack(err)
		return
	} else if err != nil {
		err = errors.WithStack(err)
		return
	} else if rsp, err = http.ReadResponse(serverReader, req); err == io.EOF {
		err = errors.WithStack(err)
		return
	} else if err != nil {
		err = errors.WithStack(err)
		return
	}
	wg, ctx := errgroup.WithContext(ctx)
	wg.Go(func() (rErr error) {
		defer req.Body.Close()
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return errors.WithStack(err)
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		return nil
	})
	wg.Go(func() (rErr error) {
		defer rsp.Body.Close()
		body, err := io.ReadAll(rsp.Body)
		if err != nil {
			return errors.WithStack(err)
		}
		rsp.Body = io.NopCloser(bytes.NewReader(body))
		return nil
	})
	if err = wg.Wait(); err != nil {
		return
	} else {
		clientPayload = req
		serverPayload = rsp
		return
	}
}

// Build 构建完整的HTTP传输对象
// 实现tcp.AppProtocolDetector接口的Build方法，用于将提取的HTTP数据构建为应用层可用的HTTP传输对象
// 该方法将原始的HTTP请求/响应、网络层信息和时间信息整合到一个统一的HttpTransport结构中
//
// 参数:
//   - ctx: 上下文对象，用于传递取消信号和超时控制
//   - meta: 基础元数据，包含命名空间、捕获器名称、版本号等系统信息
//   - clientAddr: 客户端IP地址
//   - serverAddr: 服务端IP地址
//   - clientPort: 客户端端口号
//   - serverPort: 服务端端口号
//   - clientFrameStack: 客户端帧栈信息，包含HTTP请求的捕获时间和网络层堆栈
//   - serverFrameStack: 服务端帧栈信息，包含HTTP响应的捕获时间和网络层堆栈
//   - clientPayload: 客户端负载，类型为*http.Request，从Extract方法提取的HTTP请求
//   - serverPayload: 服务端负载，类型为*http.Response，从Extract方法提取的HTTP响应
//
// 返回值:
//   - app: 构建的HTTP传输对象，类型为*n.HttpTransport
//   - err: 构建过程中发生的错误，如数据转换失败、JSON序列化失败等
//
// 构建流程:
//  1. 验证基础元数据不为nil
//  2. 构建HTTP基础信息（网络层信息）：
//     - 设置客户端和服务端的IP地址、端口号
//     - 根据客户端IP地址判断IP版本（IPv4或IPv6）
//  3. 如果有HTTP请求数据，构建HTTP请求信息：
//     - 验证类型转换，确保clientPayload为*http.Request
//     - 序列化HTTP头部为JSON格式
//     - 读取HTTP请求体内容
//     - 序列化URL查询参数为JSON格式
//     - 计算请求持续时间和时间戳
//     - 提取关键HTTP头部字段（User-Agent、Referer、Cookie等）
//     - 构建URI信息（URL、协议、主机、路径、查询参数、片段）
//     - 设置HTTP协议版本
//  4. 如果有HTTP响应数据，构建HTTP响应信息：
//     - 验证类型转换，确保serverPayload为*http.Response
//     - 序列化HTTP响应头部为JSON格式
//     - 读取HTTP响应体内容
//     - 计算响应持续时间和时间戳
//     - 提取关键HTTP响应头部字段（Server、Set-Cookie、Location等）
//  5. 合并客户端和服务端的帧栈信息，按时间戳排序
//  6. 返回完整的HttpTransport对象，包含所有HTTP传输信息
func (s *HttpDetector) Build(
	ctx context.Context,
	meta *n.BaseMeta,
	clientAddr, serverAddr net.IP,
	clientPort, serverPort layers.TCPPort,
	clientFrameStack, serverFrameStack n.FrameStack,
	clientPayload, serverPayload any,
) (app any, err error) {
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
	var request n.HttpRequest
	var uri n.HttpURI
	if clientPayload != nil && len(clientFrameStack) != 0 {
		rawReq, ok := clientPayload.(*http.Request)
		if !ok {
			return nil, errors.New("client payload is not http.Request pointer")
		}
		rawReqHeaders, err := json.Marshal(rawReq.Header)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		body, err := decompressRequestBody(rawReq)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		defer body.Close()
		content, err := io.ReadAll(body)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		query, err := json.Marshal(rawReq.URL.Query())
		if err != nil {
			return nil, errors.WithStack(err)
		}
		startTime := clientFrameStack[0].Timestamp
		endTime := clientFrameStack[len(clientFrameStack)-1].Timestamp
		request = n.HttpRequest{
			Method:           rawReq.Method,
			UserAgent:        rawReq.UserAgent(),
			Referer:          rawReq.Referer(),
			Accept:           rawReq.Header.Get(base.HDR_ACCEPT),
			AcceptEncoding:   rawReq.Header.Get(base.HDR_ACCEPT_ENCODING),
			AcceptLanguage:   rawReq.Header.Get(base.HDR_ACCEPT_LANGUAGE),
			Connection:       rawReq.Header.Get(base.HDR_CONNECTION),
			Cookie:           rawReq.Header.Get(base.HDR_COOKIE),
			Authorization:    rawReq.Header.Get(base.HDR_AUTHORIZATION),
			ReqContentType:   rawReq.Header.Get(base.HDR_CONTENT_TYPE),
			ReqContentLength: uint32(rawReq.ContentLength),
			ReqHeaders:       datatypes.JSON(rawReqHeaders),
			ReqBody:          content,
			ReqTime:          startTime,
			ReqDuration:      endTime.Sub(startTime),
			IsReqCompleted:   true,
		}
		uri = n.HttpURI{
			URL:      rawReq.URL.String(),
			Scheme:   rawReq.URL.Scheme,
			Host:     rawReq.URL.Host,
			Path:     rawReq.URL.Path,
			Queries:  datatypes.JSON(query),
			Fragment: rawReq.URL.Fragment,
		}
		hBase.HttpVersion = rawReq.Proto
	}
	var response n.HttpResponse
	if serverPayload != nil && len(serverFrameStack) != 0 {
		rawRsp, ok := serverPayload.(*http.Response)
		if !ok {
			return nil, errors.New("server payload is not http.Response pointer")
		}
		rawRspHeaders, err := json.Marshal(rawRsp.Header)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		body, err := decompressResponseBody(rawRsp)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		defer body.Close()
		content, err := io.ReadAll(body)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		startTime := serverFrameStack[0].Timestamp
		endTime := serverFrameStack[len(serverFrameStack)-1].Timestamp
		response = n.HttpResponse{
			Server:           rawRsp.Header.Get(base.HDR_SERVER),
			SetCookie:        rawRsp.Header.Get(base.HDR_SET_COOKIE),
			Location:         rawRsp.Header.Get(base.HDR_LOCATION),
			CacheControl:     rawRsp.Header.Get(base.HDR_CACHE_CONTROL),
			ContentEncoding:  rawRsp.Header.Get(base.HDR_CONTENT_ENCODING),
			RspContentType:   rawRsp.Header.Get(base.HDR_CONTENT_TYPE),
			RspContentLength: uint32(rawRsp.ContentLength),
			RspHeaders:       datatypes.JSON(rawRspHeaders),
			RspBody:          content,
			RspTime:          startTime,
			RspDuration:      endTime.Sub(startTime),
			IsRspCompleted:   true,
		}
	}

	return &n.HttpTransport{
		BaseMeta: *meta,
		FusedMeta: n.FusedMeta{
			FrameStack: sortedFrameStack(clientFrameStack, serverFrameStack),
		},
		HttpBase:     hBase,
		HttpURI:      uri,
		HttpRequest:  request,
		HttpResponse: response,
	}, nil
}

// init 初始化函数，在包加载时自动执行
// 该函数负责验证常量定义的正确性，并将HTTP协议检测器注册到TCP协议检测器系统中
//
// 主要功能:
//  1. 验证换行符常量NewLine的长度是否为2字节（\r\n）
//  2. 将HttpDetector实例注册到全局检测器注册表中，关联AppHTTP协议标识
//
// 注意事项:
//   - 注册操作确保系统能够识别和处理HTTP协议数据流
//   - 该函数在包导入时自动执行，无需手动调用
func init() {
	tcp.RegisterAppProtocolDetector(AppHTTP, &HttpDetector{})
}
