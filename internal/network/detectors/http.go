package detectors

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/gopacket/gopacket/layers"
	n "zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/network/tcp"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
	"zhongkui.com/zhongkui/internal/pkg/errgroupx"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/s3"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

const (
	// AppHTTP 表示HTTP协议。
	// 当识别到HTTP请求或响应时使用此值，表示Payload中包含HTTP协议数据。
	// HTTP协议识别基于请求行、响应行、头部字段等HTTP协议特征进行判断。
	AppHTTP tcp.AppProtocol = "http"
	// HTTPVersion10 表示 HTTP/1.0 版本。
	HTTPVersion10 = "HTTP/1.0"
	// HTTPVersion11 表示 HTTP/1.1 版本。
	HTTPVersion11 = "HTTP/1.1"
	// 换行字符串
	NewLine = "\r\n"
)

// 编译期断言：确保 HttpDetector 实现了 tcp.AppProtocolDetector 接口
var _ tcp.AppProtocolDetector = (*HttpDetector)(nil)

// HttpDetector HTTP协议检测器
// 实现tcp.AppProtocolDetector接口，用于检测和提取TCP流中的HTTP协议数据
// 该检测器负责识别HTTP请求和响应，解析HTTP协议头部，并构建完整的HTTP传输对象
type HttpDetector struct {
	client *s3.S3Client
	kd     *key.KeyDerivation
}

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
	wg, ctx := errgroupx.WithContext(ctx)
	wg.GoNamed("client", func() (rErr error) {
		if req, rErr = http.ReadRequest(clientReader); rErr != nil {
			return errors.WithStack(rErr)
		}
		// 不再 io.ReadAll body，保留 req.Body 为原始流式 reader
		// body 将由 NewHttpRequestBody 直接流式上传到 S3
		return rErr
	})
	wg.GoNamed("server", func() (rErr error) {
		if rsp, rErr = http.ReadResponse(serverReader, req); rErr != nil {
			return errors.WithStack(rErr)
		}
		// 不再 io.ReadAll body，保留 rsp.Body 为原始流式 reader
		return rErr
	})
	if err = wg.Wait(); err != nil {
		return
	} else {
		clientPayload = req
		serverPayload = rsp
		return
	}
}

func (s *HttpDetector) Build(
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
	transport, err := n.NewHttpTransport(
		ctx,
		s.client,
		meta,
		clientAddr,
		serverAddr,
		clientPort,
		serverPort,
		clientFrameStack,
		serverFrameStack,
		request,
		response,
		s.kd,
	)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return transport, nil
}
