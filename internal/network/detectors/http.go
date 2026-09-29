package detectors

import (
	"bufio"
	"bytes"
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gopacket/gopacket/layers"
	n "zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
	"zhongkui.com/zhongkui/internal/pkg/errgroupx"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/huginn"
	"zhongkui.com/zhongkui/internal/pkg/s3"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

const (
	// AppHttp 表示HTTP协议。
	// 当识别到HTTP请求或响应时使用此值，表示Payload中包含HTTP协议数据。
	// HTTP协议识别基于请求行、响应行、头部字段等HTTP协议特征进行判断。
	AppHttp AppProtocol = "http"
	// HttpVersion10 表示 HTTP/1.0 版本。
	HttpVersion10 = "HTTP/1.0"
	// HttpVersion11 表示 HTTP/1.1 版本。
	HttpVersion11 = "HTTP/1.1"
	// 换行字符串
	NewLine = "\r\n"
	// CryptoKeyOffset 加密密钥偏移量
	CryptoKeyOffset = 41827
	// CryptoKeyLength 加密密钥长度
	CryptoKeyLength = 32
)

// 编译期断言：确保 HttpDetector 实现了 tcp.AppProtocolDetector 接口
var _ AppProtocolDetector = (*HttpDetector)(nil)

// HttpDetector HTTP协议检测器
// 实现tcp.AppProtocolDetector接口，用于检测和提取TCP流中的HTTP协议数据
// 该检测器负责识别HTTP请求和响应，解析HTTP协议头部，并构建完整的HTTP传输对象
type HttpDetector struct {
	// client S3 客户端，用于正文内容的存储与去重。
	client *s3.S3Client
	// kd 密钥派生器，用于生成正文加密所需的子密钥。
	kd *key.KeyDerivation
}

// NewHttpDetector 创建 HTTP 协议检测器。
func NewHttpDetector(client *s3.S3Client, kd *key.KeyDerivation) *HttpDetector {
	return &HttpDetector{
		client: client,
		kd:     kd,
	}
}

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
	case HttpVersion10, HttpVersion11:
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
	case HttpVersion10, HttpVersion11:
		return true, nil
	default:
		return false, nil
	}
}

// Detect 检测TCP流中的数据是否为HTTP协议
// 实现tcp.AppProtocolDetector接口的Detect方法，用于判断当前TCP流是否包含HTTP协议数据
//
// 参数:
//   - ctx: 上下文对象，用于传递取消信号和超时控制
//   - peek: 待检测的TCP流数据（已预取，不消耗缓冲区）
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
	clientReader, serverReader *bufio.Reader,
) (clientPayload, serverPayload any, err error) {
	var req *http.Request
	var rsp *http.Response

	// 捕获当前缓冲中的原始请求/响应头字节，供 Build 阶段的 Http 指纹识别使用。
	// 使用 Peek（不消费）捕获而非 recordingReader + bufio.NewReader 二次包裹：
	// 二次包裹会在解析时从共享 bufio 超读最多 4096 字节并滞留于临时 buffer，
	// 这些字节从此从共享流中消失，导致 keep-alive 流中紧随其后的请求/响应
	// 永久丢失、请求-响应配对错位，进而引发等待永不到达数据的 5s 阻塞读
	// （ringbuffer 被永久置为 DeadlineExceeded，最终拖垮整个 router）。
	// 捕获结果截断至首个空行（头部结束），只传递纯头部字节给指纹识别，
	// 避免连带 body 前段或后续请求字节。不能用 req.Header.Write() 重新序列化：
	// http.Header 是 map，Write 按 key 字典序输出，会破坏 p0f 指纹依赖的
	// 原始头顺序（horder 维度），导致指纹失真。
	rawRequest := captureBuffered(clientReader)
	rawResponse := captureBuffered(serverReader)

	// 先读取请求，再读取响应。将请求对象传给 http.ReadResponse，
	// 当请求方法为 HEAD 时，Go 标准库会跳过响应体的读取，
	// 避免因 Content-Length 不匹配（HEAD 响应不应有 body）而错误消费后续流量。
	// 注意：ReadRequest/ReadResponse 接收 *bufio.Reader 时不二次包装，
	// 超读字节仍留在共享 reader 中，供下一轮 Detect/Extract 继续消费。
	if req, err = http.ReadRequest(clientReader); err != nil {
		return nil, nil, errors.WithStack(err)
	}
	if rsp, err = http.ReadResponse(serverReader, req); err != nil {
		return nil, nil, errors.WithStack(err)
	}
	// 将捕获的原始头字节挂载到请求上下文，供 Build 阶段指纹识别读取；
	// WithContext 返回的新请求与原请求共享 Body，不影响后续正文读取。
	if len(rawRequest) > 0 || len(rawResponse) > 0 {
		req = req.WithContext(context.WithValue(req.Context(), rawHTTPKey{}, &rawHTTP{
			request:  rawRequest,
			response: rawResponse,
		}))
	}
	return req, rsp, nil
}

// captureBuffered 返回 bufio.Reader 当前缓冲中、截至头部结束（首个 \r\n\r\n）的原始字节副本，
// 不消费任何字节。通过 Peek 实现，不移动读位置，不会破坏后续 http.ReadRequest/ReadResponse
// 对共享 reader 的连续读取语义；缓冲为空或头部未完整到达缓冲时返回当前全部缓冲字节（尽力而为）。
func captureBuffered(br *bufio.Reader) []byte {
	n := br.Buffered()
	if n == 0 {
		// bufio.Reader 惰性填充：Peek(1) 触发一次填充，从底层拉取可用数据。
		// 真实流水线中 Extract 前已由 Detect(peekAll) 填充缓冲，此分支仅在
		// 直接调用（如单元测试）或极端时序下生效，行为与 ReadRequest 一致。
		if _, err := br.Peek(1); err != nil {
			return nil
		}
		n = br.Buffered()
	}
	if n <= 0 {
		return nil
	}
	peeked, err := br.Peek(n)
	if err != nil {
		return nil
	}
	// 截断至头部边界（\r\n\r\n），确保只把纯头部字节交给指纹识别，
	// 避免 body 前段或 keep-alive 流中紧随的后续请求/响应字节混入。
	if i := bytes.Index(peeked, []byte("\r\n\r\n")); i >= 0 {
		return append([]byte(nil), peeked[:i+4]...)
	}
	return append([]byte(nil), peeked...)
}

// rawHTTPKey 携带原始 Http 头字节（用于指纹识别）的 context 键类型
type rawHTTPKey struct{}

// rawHTTP 记录请求/响应侧的原始头字节，由 Extract 捕获、Build 消费
type rawHTTP struct {
	request  []byte
	response []byte
}

// Build 将提取到的 HTTP 请求与响应组装为 HttpTransport 对象。
func (s *HttpDetector) Build(
	ctx context.Context,
	baseMeta *n.BaseMeta,
	streamMeta *StreamMeta,
	streamIndex uint32,
	clientPayload, serverPayload any,
	tailer BuildTailer,
) (app any, err error) {
	request, ok := clientPayload.(*http.Request)
	if !ok {
		return nil, errors.New("clientPayload is not *http.Request")
	}
	response, ok := serverPayload.(*http.Response)
	if !ok {
		return nil, errors.New("serverPayload is not *http.Response")
	}
	return NewHttpTransport(
		ctx,
		s.client,
		baseMeta,
		streamMeta,
		streamIndex,
		request,
		response,
		s.kd,
		tailer,
	)
}

// buildTcpFingerprint 汇总流两侧的被动 TCP 指纹（p0f 风格），供 HttpTransport.TcpFingerprint 使用。
// 客户端指纹来自组包器透传的首个 SYN 包识别结果，服务端指纹来自 SYN-ACK 包识别结果；
// MTU 优先取客户端，客户端缺失时回退服务端；两侧均无指纹时返回零值结构。
func buildTcpFingerprint(streamMeta *StreamMeta) n.TcpFingerprint {
	tcpFP := n.TcpFingerprint{}
	if fp := streamMeta.ClientFingerprint; fp != nil {
		tcpFP.ClientOS = fp.OS
		tcpFP.ClientOSQuality = fp.OSQuality
		tcpFP.ClientSig = fp.Sig
		tcpFP.MTU = fp.MTU
	}
	if fp := streamMeta.ServerFingerprint; fp != nil {
		tcpFP.ServerOS = fp.OS
		tcpFP.ServerOSQuality = fp.OSQuality
		tcpFP.ServerSig = fp.Sig
		if tcpFP.MTU == 0 {
			tcpFP.MTU = fp.MTU
		}
	}
	return tcpFP
}

// buildHttpFingerprint 分析 Extract 阶段捕获的原始 Http 头字节并推导设备类别猜测。
// 请求头识别浏览器与语言偏好（huginn.AnalyzeHTTPRequest），响应头识别服务端软件
// （huginn.AnalyzeHTTPResponse），最终经 guessDevice 生成设备类别；
// 请求未携带原始头字节（rawHTTPKey 缺失）或 FFI 不可用时对应字段保持零值。
func buildHttpFingerprint(streamMeta *StreamMeta, request *http.Request) n.HttpFingerprint {
	httpFP := n.HttpFingerprint{}
	raw, ok := request.Context().Value(rawHTTPKey{}).(*rawHTTP)
	if !ok {
		return httpFP
	}
	if len(raw.request) > 0 {
		if fp, ferr := huginn.AnalyzeHTTPRequest(raw.request); ferr == nil && fp != nil {
			httpFP.Browser = fp.Browser
			httpFP.BrowserQuality = fp.Quality
			httpFP.Language = fp.Language
		}
	}
	if len(raw.response) > 0 {
		if fp, ferr := huginn.AnalyzeHTTPResponse(raw.response); ferr == nil && fp != nil {
			httpFP.Server = fp.Server
			httpFP.ServerQuality = fp.Quality
		}
	}
	httpFP.DeviceGuess = guessDevice(streamMeta, httpFP)
	return httpFP
}

// guessDevice 基于 TCP/Http 指纹与流元数据推导设备类别猜测（启发式）。
// 判定优先级：移动设备（客户端 OS 含移动平台关键词）> 桌面端（识别出浏览器）>
// 服务端（识别出服务端软件/系统）> IoT（无浏览器、无客户端 OS 指纹）> 未知。
func guessDevice(streamMeta *StreamMeta, httpFP n.HttpFingerprint) string {
	var clientOS string
	if fp := streamMeta.ClientFingerprint; fp != nil {
		clientOS = strings.ToLower(fp.OS)
	}
	switch {
	case strings.Contains(clientOS, "android"),
		strings.Contains(clientOS, "ios"),
		strings.Contains(clientOS, "ipad"),
		strings.Contains(clientOS, "iphone"),
		strings.Contains(clientOS, "symbian"),
		strings.Contains(clientOS, "blackberry"),
		strings.Contains(clientOS, "windows phone"):
		return "mobile"
	case httpFP.Browser != "" && httpFP.BrowserQuality > 0:
		return "desktop"
	case httpFP.Server != "" && httpFP.ServerQuality > 0:
		return "server"
	case httpFP.Browser == "" && clientOS == "":
		// 无浏览器与客户端 OS 指纹，通常为非浏览器客户端（脚本 / 嵌入式 / IoT 设备）
		return "iot"
	default:
		return "unknown"
	}
}

// Build 构建完整的HTTP传输对象
// 实现tcp.AppProtocolDetector接口的Build方法，用于将提取的HTTP数据构建为应用层可用的HTTP传输对象
// 该方法将原始的HTTP请求/响应、网络层信息和时间信息整合到一个统一的HttpTransport结构中
//
// 参数:
//   - ctx: 上下文对象，用于传递取消信号和超时控制
//   - meta: 基础元数据，包含命名空间、捕获器名称、版本号等系统信息
//   - streamMeta: 流元数据，携带客户端/服务端 TCP 指纹，组装阶段回填到 HttpTransport
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
func NewHttpTransport(
	ctx context.Context,
	s3Client *s3.S3Client,
	meta *n.BaseMeta,
	streamMeta *StreamMeta,
	streamIndex uint32,
	request *http.Request, response *http.Response,
	kd *key.KeyDerivation,
	buildTailer BuildTailer,
) (transport *n.HttpTransport, err error) {
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
		hBase.IPVersion = layers.IPProtocolIPv4
	} else {
		hBase.IPVersion = layers.IPProtocolIPv6
	}
	cryptoKey, err := kd.DeriveChildKey(CryptoKeyOffset, CryptoKeyLength)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var uri *n.HttpURI
	var requestBody *n.HttpRequestBody
	var requestHeaders *n.HttpRequestHeaders
	var responseBody *n.HttpResponseBody
	var responseHeaders *n.HttpResponseHeaders
	wg, ctx := errgroupx.WithContext(ctx)
	wg.GoNamed("req-build", func() error {
		if request != nil {
			uri, err = n.NewHttpURI(request)
			if err != nil {
				return errors.WithStack(err)
			}
			requestBody, err = n.NewHttpRequestBody(ctx, s3Client, request, cryptoKey)
			if err != nil {
				return errors.WithStack(err)
			}
			requestHeaders = n.NewHttpRequestHeaders(request)
			hBase.HttpVersion = request.Proto
		}
		return nil
	})
	wg.GoNamed("rsp-build", func() error {
		if response != nil {
			responseBody, err = n.NewHttpResponseBody(ctx, s3Client, response, cryptoKey)
			if err != nil {
				return errors.WithStack(err)
			}
		}
		responseHeaders = n.NewHttpResponseHeaders(response)
		return nil
	})
	if err := wg.Wait(); err != nil {
		return nil, errors.WithStack(err)
	}
	if uri == nil {
		uri = &n.HttpURI{}
	}
	if requestBody == nil {
		requestBody = &n.HttpRequestBody{}
	}
	if requestHeaders == nil {
		requestHeaders = &n.HttpRequestHeaders{}
	}
	if responseBody == nil {
		responseBody = &n.HttpResponseBody{}
	}
	if responseHeaders == nil {
		responseHeaders = &n.HttpResponseHeaders{}
	}
	tailer, err := buildTailer(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// TCP 指纹透传：客户端来自 SYN 包，服务端来自 SYN-ACK 包
	tcpFP := buildTcpFingerprint(streamMeta)
	// Http 指纹分析：使用 Extract 阶段捕获的原始头字节
	httpFP := buildHttpFingerprint(streamMeta, request)
	transport = &n.HttpTransport{
		// 行详情接口依赖非空的全局唯一 ID，构造时即生成，落库路径无需再补
		RowMeta: n.RowMeta{
			ID: serializer.NewHexUUIDv4(),
		},
		BaseMeta: *meta,
		TimestampMeta: n.TimestampMeta{
			Timestamp: streamMeta.Timestamp,
			CreatedAt: time.Now(),
		},
		TraceMeta: n.TraceMeta{
			TraceID: streamMeta.TraceID,
		},
		TcpFingerprint:      tcpFP,
		HttpBase:            hBase,
		HttpURI:             *uri,
		HttpRequestBody:     *requestBody,
		HttpRequestHeaders:  *requestHeaders,
		HttpResponseBody:    *responseBody,
		HttpResponseHeaders: *responseHeaders,
		HttpFingerprint:     httpFP,
		AppTailer: n.AppTailer{
			ClientConsumed:           tailer.ClientConsumed,
			ClientStartPayloadOffset: tailer.ClientStartPayloadOffset,
			ClientEndPayloadOffset:   tailer.ClientEndPayloadOffset,
			ClientStartTimestamp:     tailer.ClientStartTimestamp,
			ClientEndTimestamp:       tailer.ClientEndTimestamp,
			ServerConsumed:           tailer.ServerConsumed,
			ServerStartPayloadOffset: tailer.ServerStartPayloadOffset,
			ServerEndPayloadOffset:   tailer.ServerEndPayloadOffset,
			ServerStartTimestamp:     tailer.ServerStartTimestamp,
			ServerEndTimestamp:       tailer.ServerEndTimestamp,
		},
	}
	return transport, nil
}
