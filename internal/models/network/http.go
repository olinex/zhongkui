package network

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gopacket/gopacket/layers"
	"gorm.io/datatypes"
	"zhongkui.com/zhongkui/internal/pkg/crypto/combine"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
	"zhongkui.com/zhongkui/internal/pkg/errgroupx"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/s3"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

const (
	CRYPTO_KEY_OFFSET = 41827
	CRYPTO_KEY_LENGTH = 32
)

// sortedFrameStack 返回按时间戳排序的帧栈，合并客户端和服务端的帧信息。
func sortedFrameStack(reqFrameStack, rspFrameStack FrameStack) FrameStack {
	fs := make(FrameStack, 0, len(reqFrameStack)+len(rspFrameStack))
	clientIndex, serverIndex := 0, 0
	for {
		if clientIndex >= len(reqFrameStack) {
			fs = append(fs, rspFrameStack[serverIndex:]...)
			break
		}
		if serverIndex >= len(rspFrameStack) {
			fs = append(fs, reqFrameStack[clientIndex:]...)
			break
		}
		clientFrameInfo := reqFrameStack[clientIndex]
		serverFrameInfo := rspFrameStack[serverIndex]
		if clientFrameInfo.Timestamp.Before(serverFrameInfo.Timestamp) {
			fs = append(fs, clientFrameInfo)
			clientIndex++
		} else {
			fs = append(fs, serverFrameInfo)
			serverIndex++
		}
	}
	return fs
}

func objectKey(name string, req bool) string {
	if req {
		return fmt.Sprintf(`body/request/%s.%s`, name, combine.FILE_EXT)
	}
	return fmt.Sprintf(`body/response/%s.%s`, name, combine.FILE_EXT)
}

// parseInt32 解析字符串为 int32，解析失败返回 0
func parseInt32(s string) int32 {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0
	}
	return int32(v)
}

// parseInt64 解析字符串为 int64，解析失败返回 0
func parseInt64(s string) int64 {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
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

// HttpBase 表示HTTP基本信息
// 包含HTTP通信的基础网络连接信息，用于标识通信的端点和协议版本
type HttpBase struct {
	// ClientIP 客户端IP地址，发起HTTP请求的源地址
	ClientIP serializer.IPAddr `gorm:"type:FixedString(16)" json:"client_ip"`
	// ClientPort 客户端端口号，发起HTTP请求的源端口
	ClientPort layers.TCPPort `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"client_port"`
	// ServerIP 服务器IP地址，HTTP服务的目标地址
	ServerIP serializer.IPAddr `gorm:"type:FixedString(16)" json:"server_ip"`
	// ServerPort 服务器端口号，HTTP服务的目标端口
	ServerPort layers.TCPPort `gorm:"type:UInt16 CODEC(ZSTD(3))" json:"server_port"`
	// IPVersion IP协议版本，标识使用的IP协议（IPv4或IPv6）
	IPVersion layers.IPProtocol `gorm:"type:UInt8 CODEC(ZSTD(3))" json:"ip_version"`
	// HttpVersion HTTP协议版本，如HTTP/1.0、HTTP/1.1、HTTP/2.0等
	HttpVersion string `gorm:"type:LowCardinality(String) CODEC(ZSTD(3))" json:"http_version"`
}

// HttpURI 表示HTTP URI信息
// 包含HTTP请求的统一资源标识符（URI）的各个组成部分
type HttpURI struct {
	// URL 完整的请求URL，包含协议、主机、路径、查询参数和片段
	URL string `gorm:"type:String CODEC(ZSTD(3))" json:"url"`
	// Scheme 协议方案，如http、https、ftp等
	Scheme string `gorm:"type:LowCardinality(String) CODEC(ZSTD(3))" json:"scheme"`
	// Host 主机名或IP地址，请求的目标服务器
	Host string `gorm:"type:String CODEC(ZSTD(3))" json:"host"`
	// Path 请求路径，URL中主机名之后的部分
	Path string `gorm:"type:String CODEC(ZSTD(3))" json:"path"`
	// Queries 查询参数，URL中问号（?）后面的参数部分，以JSON格式存储
	Queries datatypes.JSON `gorm:"type:String CODEC(ZSTD(3))" json:"queries"`
	// Fragment 片段标识符，URL中井号（#）后面的部分
	Fragment string `gorm:"type:String CODEC(ZSTD(3))" json:"fragment"`
}

func NewHttpURI(uri *url.URL) (*HttpURI, error) {
	query, err := json.Marshal(uri.Query())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &HttpURI{
		URL:      uri.String(),
		Scheme:   uri.Scheme,
		Host:     uri.Host,
		Path:     uri.Path,
		Queries:  datatypes.JSON(query),
		Fragment: uri.Fragment,
	}, nil
}

// HttpTLS 表示TLS安全认证信息
// 包含HTTPS连接的安全传输层协议（TLS）相关信息
type HttpTLS struct {
	// TLSVersion TLS协议版本，如TLS 1.0、TLS 1.1、TLS 1.2、TLS 1.3等
	TLSVersion string
	// TLSCipher 加密套件，协商使用的加密算法组合
	TLSCipher string
	// TLSEncrypted 是否启用TLS加密，标识连接是否使用HTTPS
	TLSEncrypted bool
}

// HttpRequestHeaders 表示HTTP请求头信息
// 包含所有标准HTTP请求头字段，参考 MDN HTTP Headers 文档标准
// (https://developer.mozilla.org/zh-CN/docs/Web/HTTP/Reference/Headers)
// 该结构体以大宽表形式定义，每个字段对应一个标准HTTP请求头
//
// 头部分类:
//   - 认证 (Authentication): Authorization, Proxy-Authorization
//   - 缓存 (Caching): Cache-Control
//   - 条件请求 (Conditionals): If-Match, If-None-Match, If-Modified-Since, If-Unmodified-Since, If-Range
//   - 连接管理 (Connection Management): Connection, Keep-Alive
//   - 内容协商 (Content Negotiation): Accept, Accept-Encoding, Accept-Language
//   - 控制 (Control): Expect, Max-Forwards
//   - Cookie: Cookie
//   - CORS: Origin, Access-Control-Request-Headers, Access-Control-Request-Method
//   - 消息主体 (Message Body): Content-Type, Content-Length, Content-Encoding, Content-Language
//   - 代理 (Proxies): Forwarded, Via
//   - 范围请求 (Range Requests): Range
//   - 请求上下文 (Request Context): From, Host, Referer, Referrer-Policy, User-Agent
//   - 传输编码 (Transfer Coding): TE, Transfer-Encoding, Trailer
//   - WebSocket: Sec-WebSocket-Key, Sec-WebSocket-Version, Sec-WebSocket-Protocol, Sec-WebSocket-Extensions
//   - Fetch 元数据 (Fetch Metadata): Sec-Fetch-Site, Sec-Fetch-Mode, Sec-Fetch-User, Sec-Fetch-Dest, Sec-Purpose
//   - 客户端提示 (Client Hints): Sec-CH-UA 系列
//   - 摘要 (Digest): Want-Content-Digest, Want-Repr-Digest
//   - 安全 (Security): Upgrade-Insecure-Requests
//   - 其他 (Other): Date, Link, Upgrade, Priority, Alt-Used
//
// 使用场景:
//   - HTTP 请求流量分析和监控
//   - 安全审计与入侵检测
//   - Web 应用性能分析与调试
type HttpRequestHeaders struct {
	// ======================== 认证 (Authentication) ========================

	// ReqHdrAuthorization 认证信息，包含用于向服务器验证用户代理身份的凭据，如 Bearer token、Basic 认证等
	// 使用 Crypto 加密存储，防止涉敏信息泄露后被逆向破解
	ReqHdrAuthorization *serializer.Crypto `gorm:"column:req_hdr_authorization;type:String CODEC(ZSTD(3));serializer:crypto" json:"req_hdr_authorization" secure:"mode=\"aes-gcm\",offset=5123,length=32"`
	// ReqHdrProxyAuthorization 代理认证信息，包含用于向代理服务器验证用户代理的凭据
	// 使用 Crypto 加密存储，防止涉敏信息泄露后被逆向破解
	ReqHdrProxyAuthorization *serializer.Crypto `gorm:"column:req_hdr_proxy_authorization;type:String CODEC(ZSTD(3));serializer:crypto" json:"req_hdr_proxy_authorization" secure:"mode=\"aes-gcm\",offset=18456,length=32"`

	// ======================== 缓存 (Caching) ========================

	// ReqHdrCacheControl 缓存控制指令，指定请求/响应链中所有缓存机制必须遵守的指令，如 no-cache、max-age 等
	ReqHdrCacheControl string `gorm:"column:req_hdr_cache_control;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_cache_control"`

	// ======================== 条件请求 (Conditionals) ========================

	// ReqHdrIfMatch 条件匹配，仅当存储的资源与给定的 ETag 之一匹配时才执行请求
	ReqHdrIfMatch string `gorm:"column:req_hdr_if_match;type:String CODEC(ZSTD(3))" json:"req_hdr_if_match"`
	// ReqHdrIfNoneMatch 条件不匹配，仅当存储的资源与给定的所有 ETag 都不匹配时才执行请求，常用于缓存更新
	ReqHdrIfNoneMatch string `gorm:"column:req_hdr_if_none_match;type:String CODEC(ZSTD(3))" json:"req_hdr_if_none_match"`
	// ReqHdrIfModifiedSince 条件修改时间，仅当资源在给定日期后被修改时才传输资源，用于缓存验证
	ReqHdrIfModifiedSince string `gorm:"column:req_hdr_if_modified_since;type:String CODEC(ZSTD(3))" json:"req_hdr_if_modified_since"`
	// ReqHdrIfUnmodifiedSince 条件未修改时间，仅当资源在给定日期后未被修改时才传输资源，用于乐观并发控制
	ReqHdrIfUnmodifiedSince string `gorm:"column:req_hdr_if_unmodified_since;type:String CODEC(ZSTD(3))" json:"req_hdr_if_unmodified_since"`
	// ReqHdrIfRange 条件范围请求，仅在给定的 ETag 或日期与远程资源匹配时请求指定范围，防止下载不兼容版本
	ReqHdrIfRange string `gorm:"column:req_hdr_if_range;type:String CODEC(ZSTD(3))" json:"req_hdr_if_range"`

	// ======================== 连接管理 (Connection Management) ========================

	// ReqHdrConnection 连接控制，控制当前事务完成后网络连接是否保持打开状态，如 keep-alive、close
	ReqHdrConnection string `gorm:"column:req_hdr_connection;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_connection"`
	// ReqHdrKeepAlive 持久连接控制，控制持久连接应保持打开状态的时间及最大请求数
	ReqHdrKeepAlive string `gorm:"column:req_hdr_keep_alive;type:String CODEC(ZSTD(3))" json:"req_hdr_keep_alive"`

	// ======================== 内容协商 (Content Negotiation) ========================

	// ReqHdrAccept 可接受的响应内容类型（MIME 类型），如 application/json、text/html 等
	ReqHdrAccept string `gorm:"column:req_hdr_accept;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_accept"`
	// ReqHdrAcceptEncoding 可接受的内容编码算法，通常是压缩算法，如 gzip、deflate、br 等
	ReqHdrAcceptEncoding string `gorm:"column:req_hdr_accept_encoding;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_accept_encoding"`
	// ReqHdrAcceptLanguage 可接受的自然语言，如 zh-CN、en-US 等，仅作为提示
	ReqHdrAcceptLanguage string `gorm:"column:req_hdr_accept_language;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_accept_language"`

	// ======================== 控制 (Control) ========================

	// ReqHdrExpect 期望条件，表示服务器正确处理请求需要满足的期望，如 100-continue
	ReqHdrExpect string `gorm:"column:req_hdr_expect;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_expect"`
	// ReqHdrMaxForwards 最大转发跳数，使用 TRACE 方法时指示请求在被返回前可以经过的最大代理数
	ReqHdrMaxForwards int32 `gorm:"column:req_hdr_max_forwards;type:Int32 CODEC(ZSTD(3))" json:"req_hdr_max_forwards"`

	// ======================== Cookie ========================

	// ReqHdrCookie 客户端发送的 HTTP Cookie，包含先前由服务器通过 Set-Cookie 标头设置并存储的 cookie
	// 使用 Crypto 加密存储，Cookie 常包含会话令牌等涉敏信息
	ReqHdrCookie *serializer.Crypto `gorm:"column:req_hdr_cookie;type:String CODEC(ZSTD(3));serializer:crypto" json:"req_hdr_cookie" secure:"mode=\"aes-gcm\",offset=27901,length=32"`

	// ======================== CORS ========================

	// ReqHdrOrigin 请求源，指示获取请求是从什么源（协议+主机+端口）发起的
	ReqHdrOrigin string `gorm:"column:req_hdr_origin;type:String CODEC(ZSTD(3))" json:"req_hdr_origin"`
	// ReqHdrAccessControlRequestHeaders CORS 预检请求头，告知服务器正式请求会使用哪些 HTTP 标头
	ReqHdrAccessControlRequestHeaders string `gorm:"column:req_hdr_access_control_request_headers;type:String CODEC(ZSTD(3))" json:"req_hdr_access_control_request_headers"`
	// ReqHdrAccessControlRequestMethod CORS 预检方法，告知服务器正式请求会使用哪种 HTTP 方法
	ReqHdrAccessControlRequestMethod string `gorm:"column:req_hdr_access_control_request_method;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_access_control_request_method"`

	// ======================== 消息主体 (Message Body Information) ========================

	// ReqHdrContentType 请求内容类型（MIME 类型），标识请求主体的媒体格式
	ReqHdrContentType string `gorm:"column:req_hdr_content_type;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_content_type"`
	// ReqHdrContentLength 请求内容长度，请求主体的大小（以十进制字节数表示）
	ReqHdrContentLength int64 `gorm:"column:req_hdr_content_length;type:Int64 CODEC(ZSTD(3))" json:"req_hdr_content_length"`
	// ReqHdrContentEncoding 请求内容编码，用于指定请求主体的压缩算法
	ReqHdrContentEncoding string `gorm:"column:req_hdr_content_encoding;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_content_encoding"`
	// ReqHdrContentLanguage 请求内容语言，描述面向受众的人类语言
	ReqHdrContentLanguage string `gorm:"column:req_hdr_content_language;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_content_language"`

	// ======================== 代理 (Proxies) ========================

	// ReqHdrForwarded 转发信息，包含来自代理服务器面向客户端一侧的信息，当请求经过代理时原始信息可能被更改或丢失
	ReqHdrForwarded string `gorm:"column:req_hdr_forwarded;type:String CODEC(ZSTD(3))" json:"req_hdr_forwarded"`
	// ReqHdrVia 代理链信息，由代理（正向和反向代理）添加，记录请求经过的中间节点
	ReqHdrVia string `gorm:"column:req_hdr_via;type:String CODEC(ZSTD(3))" json:"req_hdr_via"`

	// ======================== 范围请求 (Range Requests) ========================

	// ReqHdrRange 范围请求，指示服务器应返回文档的哪一部分，用于断点续传和大文件分片下载
	ReqHdrRange string `gorm:"column:req_hdr_range;type:String CODEC(ZSTD(3))" json:"req_hdr_range"`

	// ======================== 请求上下文 (Request Context) ========================

	// ReqHdrFrom 发送者邮箱，包含发送请求的用户代理的实际掌控者的电子邮箱地址
	ReqHdrFrom string `gorm:"column:req_hdr_from;type:String CODEC(ZSTD(3))" json:"req_hdr_from"`
	// ReqHdrHost 目标主机，指定服务器的域名（用于虚拟主机）和服务器侦听的 TCP 端口号（可选）
	ReqHdrHost string `gorm:"column:req_hdr_host;type:String CODEC(ZSTD(3))" json:"req_hdr_host"`
	// ReqHdrReferer 引用来源，前一个网页的地址，表示从该网页链接到当前请求的页面
	ReqHdrReferer string `gorm:"column:req_hdr_referer;type:String CODEC(ZSTD(3))" json:"req_hdr_referer"`
	// ReqHdrReferrerPolicy 引用策略，规定了在发出的请求中应包含哪些引用信息
	ReqHdrReferrerPolicy string `gorm:"column:req_hdr_referrer_policy;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_referrer_policy"`
	// ReqHdrUserAgent 用户代理字符串，标识发起请求的应用程序类型、操作系统、软件供应商或软件版本
	ReqHdrUserAgent string `gorm:"column:req_hdr_user_agent;type:String CODEC(ZSTD(3))" json:"req_hdr_user_agent"`

	// ======================== 传输编码 (Transfer Coding) ========================

	// ReqHdrTE 传输编码接受，指定用户代理愿意接受的传输编码（trailers 及传输编码类型）
	ReqHdrTE string `gorm:"column:req_hdr_te;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_te"`
	// ReqHdrTransferEncoding 传输编码，指定用于将资源安全传输给用户的编码形式，如 chunked
	ReqHdrTransferEncoding string `gorm:"column:req_hdr_transfer_encoding;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_transfer_encoding"`
	// ReqHdrTrailer 尾部字段声明，允许发送方在分块消息的末尾包含额外的元数据字段
	ReqHdrTrailer string `gorm:"column:req_hdr_trailer;type:String CODEC(ZSTD(3))" json:"req_hdr_trailer"`

	// ======================== WebSocket ========================

	// ReqHdrSecWebSocketKey WebSocket 握手密钥，包含一个密钥用于验证客户端确实意图打开 WebSocket 连接
	// 使用 Crypto 加密存储，WebSocket 握手密钥涉及连接安全
	ReqHdrSecWebSocketKey *serializer.Crypto `gorm:"column:req_hdr_sec_websocket_key;type:String CODEC(ZSTD(3));serializer:crypto" json:"req_hdr_sec_websocket_key" secure:"mode=\"aes-gcm\",offset=36234,length=32"`
	// ReqHdrSecWebSocketVersion WebSocket 协议版本，指示客户端使用的 WebSocket 协议版本号
	ReqHdrSecWebSocketVersion string `gorm:"column:req_hdr_sec_websocket_version;type:String CODEC(ZSTD(3))" json:"req_hdr_sec_websocket_version"`
	// ReqHdrSecWebSocketProtocol WebSocket 子协议，指示客户端支持的子协议列表，按优先顺序排列
	ReqHdrSecWebSocketProtocol string `gorm:"column:req_hdr_sec_websocket_protocol;type:String CODEC(ZSTD(3))" json:"req_hdr_sec_websocket_protocol"`
	// ReqHdrSecWebSocketExtensions WebSocket 扩展，指示客户端支持的 WebSocket 扩展列表，按优先顺序排列
	ReqHdrSecWebSocketExtensions string `gorm:"column:req_hdr_sec_websocket_extensions;type:String CODEC(ZSTD(3))" json:"req_hdr_sec_websocket_extensions"`

	// ======================== Fetch 元数据 (Fetch Metadata) ========================

	// ReqHdrSecFetchSite Fetch 元数据——源关系，指示请求发起者的源与目标源之间的关系 (cross-site/same-origin/same-site/none)
	ReqHdrSecFetchSite string `gorm:"column:req_hdr_sec_fetch_site;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_sec_fetch_site"`
	// ReqHdrSecFetchMode Fetch 元数据——请求模式，向服务器指示请求的模式 (cors/navigate/no-cors/same-origin/websocket)
	ReqHdrSecFetchMode string `gorm:"column:req_hdr_sec_fetch_mode;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_sec_fetch_mode"`
	// ReqHdrSecFetchUser Fetch 元数据——用户激活，指示导航请求是否由用户激活触发
	ReqHdrSecFetchUser string `gorm:"column:req_hdr_sec_fetch_user;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_sec_fetch_user"`
	// ReqHdrSecFetchDest Fetch 元数据——请求目标，指示请求到服务器的目的地类型 (document/image/script/style 等)
	ReqHdrSecFetchDest string `gorm:"column:req_hdr_sec_fetch_dest;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_sec_fetch_dest"`
	// ReqHdrSecPurpose 请求目的，指示请求的服务端目的，如 prefetch 表示资源被提前获取用于未来导航
	ReqHdrSecPurpose string `gorm:"column:req_hdr_sec_purpose;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_sec_purpose"`
	// ReqHdrServiceWorkerNavigationPreload Service Worker 导航预加载，在 SW 启动期间提前请求中发送的标头
	ReqHdrServiceWorkerNavigationPreload string `gorm:"column:req_hdr_service_worker_navigation_preload;type:String CODEC(ZSTD(3))" json:"req_hdr_service_worker_navigation_preload"`

	// ======================== 客户端提示 (Client Hints) ========================

	// ReqHdrSecCHUA 用户代理的品牌（brand）和版本信息
	ReqHdrSecCHUA string `gorm:"column:req_hdr_sec_ch_ua;type:String CODEC(ZSTD(3))" json:"req_hdr_sec_ch_ua"`
	// ReqHdrSecCHUAArch 用户代理底层平台的 CPU 架构，如 x86、arm
	ReqHdrSecCHUAArch string `gorm:"column:req_hdr_sec_ch_ua_arch;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_sec_ch_ua_arch"`
	// ReqHdrSecCHUABitness 用户代理底层 CPU 架构位数，如 64、32
	ReqHdrSecCHUABitness string `gorm:"column:req_hdr_sec_ch_ua_bitness;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_sec_ch_ua_bitness"`
	// ReqHdrSecCHUAFullVersion 用户代理的完整语义版本字符串
	ReqHdrSecCHUAFullVersion string `gorm:"column:req_hdr_sec_ch_ua_full_version;type:String CODEC(ZSTD(3))" json:"req_hdr_sec_ch_ua_full_version"`
	// ReqHdrSecCHUAFullVersionList 用户代理品牌列表中每个品牌的完整版本
	ReqHdrSecCHUAFullVersionList string `gorm:"column:req_hdr_sec_ch_ua_full_version_list;type:String CODEC(ZSTD(3))" json:"req_hdr_sec_ch_ua_full_version_list"`
	// ReqHdrSecCHUAMobile 用户代理是否在移动设备上运行
	ReqHdrSecCHUAMobile string `gorm:"column:req_hdr_sec_ch_ua_mobile;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_sec_ch_ua_mobile"`
	// ReqHdrSecCHUAModel 用户代理的设备型号
	ReqHdrSecCHUAModel string `gorm:"column:req_hdr_sec_ch_ua_model;type:String CODEC(ZSTD(3))" json:"req_hdr_sec_ch_ua_model"`
	// ReqHdrSecCHUAPlatform 用户代理的底层操作系统/平台，如 Windows、macOS、Android
	ReqHdrSecCHUAPlatform string `gorm:"column:req_hdr_sec_ch_ua_platform;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_sec_ch_ua_platform"`
	// ReqHdrSecCHUAPlatformVersion 用户代理底层操作系统的版本号
	ReqHdrSecCHUAPlatformVersion string `gorm:"column:req_hdr_sec_ch_ua_platform_version;type:String CODEC(ZSTD(3))" json:"req_hdr_sec_ch_ua_platform_version"`
	// ReqHdrSecCHUAWoW64 用户代理二进制文件是否在 64 位 Windows 上以 32 位模式运行
	ReqHdrSecCHUAWoW64 string `gorm:"column:req_hdr_sec_ch_ua_wow64;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_sec_ch_ua_wow64"`
	// ReqHdrSecCHPrefersColorScheme 用户的深色/浅色模式偏好 (dark/light)
	ReqHdrSecCHPrefersColorScheme string `gorm:"column:req_hdr_sec_ch_prefers_color_scheme;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_sec_ch_prefers_color_scheme"`
	// ReqHdrSecCHPrefersReducedMotion 用户减少动画和布局变化的偏好
	ReqHdrSecCHPrefersReducedMotion string `gorm:"column:req_hdr_sec_ch_prefers_reduced_motion;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_sec_ch_prefers_reduced_motion"`
	// ReqHdrSecCHPrefersReducedTransparency 用户降低透明度的偏好
	ReqHdrSecCHPrefersReducedTransparency string `gorm:"column:req_hdr_sec_ch_prefers_reduced_transparency;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_sec_ch_prefers_reduced_transparency"`

	// ======================== 摘要 (Digest) ========================

	// ReqHdrWantContentDigest 期望内容摘要，表达希望使用 Content-Digest 标头接收内容摘要
	ReqHdrWantContentDigest string `gorm:"column:req_hdr_want_content_digest;type:String CODEC(ZSTD(3))" json:"req_hdr_want_content_digest"`
	// ReqHdrWantReprDigest 期望表示摘要，表达希望使用 Repr-Digest 标头接收表示形式的校验摘要
	ReqHdrWantReprDigest string `gorm:"column:req_hdr_want_repr_digest;type:String CODEC(ZSTD(3))" json:"req_hdr_want_repr_digest"`

	// ======================== 安全 (Security) ========================

	// ReqHdrUpgradeInsecureRequests 升级不安全请求，向服务器发送信号表达客户端偏好加密和身份验证的响应
	ReqHdrUpgradeInsecureRequests string `gorm:"column:req_hdr_upgrade_insecure_requests;type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_hdr_upgrade_insecure_requests"`

	// ======================== 其他 (Other) ========================

	// ReqHdrDate 消息创建日期和时间
	ReqHdrDate string `gorm:"column:req_hdr_date;type:String CODEC(ZSTD(3))" json:"req_hdr_date"`
	// ReqHdrLink 链接关系，提供在 HTTP 标头中序列化一个或多个链接的方法，语义上等同于 HTML <link> 元素
	ReqHdrLink string `gorm:"column:req_hdr_link;type:String CODEC(ZSTD(3))" json:"req_hdr_link"`
	// ReqHdrUpgrade 协议升级，可用于将已建立的连接升级为不同的协议（如 HTTP/1.1 → HTTP/2 或 WebSocket）
	ReqHdrUpgrade string `gorm:"column:req_hdr_upgrade;type:String CODEC(ZSTD(3))" json:"req_hdr_upgrade"`
	// ReqHdrPriority 资源优先级，提示特定资源请求在连接上的优先级
	ReqHdrPriority string `gorm:"column:req_hdr_priority;type:String CODEC(ZSTD(3))" json:"req_hdr_priority"`
	// ReqHdrAltUsed 使用的替代服务，用于标识当前正在使用的替代服务
	ReqHdrAltUsed string `gorm:"column:req_hdr_alt_used;type:String CODEC(ZSTD(3))" json:"req_hdr_alt_used"`
}

// newCrypto 将字符串包装为 Crypto 用于加密存储，空值交由 Serializer 层自动判断处理
func newCrypto(s string) *serializer.Crypto {
	return serializer.NewCrypto([]byte(s))
}

func NewHttpRequestHeaders(req *http.Request) *HttpRequestHeaders {
	return &HttpRequestHeaders{
		// 认证（加密存储）
		ReqHdrAuthorization:      newCrypto(req.Header.Get("Authorization")),
		ReqHdrProxyAuthorization: newCrypto(req.Header.Get("Proxy-Authorization")),
		// 缓存
		ReqHdrCacheControl: req.Header.Get("Cache-Control"),
		// 条件请求
		ReqHdrIfMatch:           req.Header.Get("If-Match"),
		ReqHdrIfNoneMatch:       req.Header.Get("If-None-Match"),
		ReqHdrIfModifiedSince:   req.Header.Get("If-Modified-Since"),
		ReqHdrIfUnmodifiedSince: req.Header.Get("If-Unmodified-Since"),
		ReqHdrIfRange:           req.Header.Get("If-Range"),
		// 连接管理
		ReqHdrConnection: req.Header.Get("Connection"),
		ReqHdrKeepAlive:  req.Header.Get("Keep-Alive"),
		// 内容协商
		ReqHdrAccept:         req.Header.Get("Accept"),
		ReqHdrAcceptEncoding: req.Header.Get("Accept-Encoding"),
		ReqHdrAcceptLanguage: req.Header.Get("Accept-Language"),
		// 控制
		ReqHdrExpect:      req.Header.Get("Expect"),
		ReqHdrMaxForwards: parseInt32(req.Header.Get("Max-Forwards")),
		// Cookie（加密存储）
		ReqHdrCookie: newCrypto(req.Header.Get("Cookie")),
		// CORS
		ReqHdrOrigin:                      req.Header.Get("Origin"),
		ReqHdrAccessControlRequestHeaders: req.Header.Get("Access-Control-Request-Headers"),
		ReqHdrAccessControlRequestMethod:  req.Header.Get("Access-Control-Request-Method"),
		// 消息主体
		ReqHdrContentType:     req.Header.Get("Content-Type"),
		ReqHdrContentLength:   req.ContentLength,
		ReqHdrContentEncoding: req.Header.Get("Content-Encoding"),
		ReqHdrContentLanguage: req.Header.Get("Content-Language"),
		// 代理
		ReqHdrForwarded: req.Header.Get("Forwarded"),
		ReqHdrVia:       req.Header.Get("Via"),
		// 范围请求
		ReqHdrRange: req.Header.Get("Range"),
		// 请求上下文
		ReqHdrFrom:           req.Header.Get("From"),
		ReqHdrHost:           req.Host,
		ReqHdrReferer:        req.Referer(),
		ReqHdrReferrerPolicy: req.Header.Get("Referrer-Policy"),
		ReqHdrUserAgent:      req.UserAgent(),
		// 传输编码
		ReqHdrTE:               req.Header.Get("TE"),
		ReqHdrTransferEncoding: req.Header.Get("Transfer-Encoding"),
		ReqHdrTrailer:          req.Header.Get("Trailer"),
		// WebSocket（加密存储）
		ReqHdrSecWebSocketKey:        newCrypto(req.Header.Get("Sec-WebSocket-Key")),
		ReqHdrSecWebSocketVersion:    req.Header.Get("Sec-WebSocket-Version"),
		ReqHdrSecWebSocketProtocol:   req.Header.Get("Sec-WebSocket-Protocol"),
		ReqHdrSecWebSocketExtensions: req.Header.Get("Sec-WebSocket-Extensions"),
		// Fetch 元数据
		ReqHdrSecFetchSite:                   req.Header.Get("Sec-Fetch-Site"),
		ReqHdrSecFetchMode:                   req.Header.Get("Sec-Fetch-Mode"),
		ReqHdrSecFetchUser:                   req.Header.Get("Sec-Fetch-User"),
		ReqHdrSecFetchDest:                   req.Header.Get("Sec-Fetch-Dest"),
		ReqHdrSecPurpose:                     req.Header.Get("Sec-Purpose"),
		ReqHdrServiceWorkerNavigationPreload: req.Header.Get("Service-Worker-Navigation-Preload"),
		// 客户端提示
		ReqHdrSecCHUA:                         req.Header.Get("Sec-CH-UA"),
		ReqHdrSecCHUAArch:                     req.Header.Get("Sec-CH-UA-Arch"),
		ReqHdrSecCHUABitness:                  req.Header.Get("Sec-CH-UA-Bitness"),
		ReqHdrSecCHUAFullVersion:              req.Header.Get("Sec-CH-UA-Full-Version"),
		ReqHdrSecCHUAFullVersionList:          req.Header.Get("Sec-CH-UA-Full-Version-List"),
		ReqHdrSecCHUAMobile:                   req.Header.Get("Sec-CH-UA-Mobile"),
		ReqHdrSecCHUAModel:                    req.Header.Get("Sec-CH-UA-Model"),
		ReqHdrSecCHUAPlatform:                 req.Header.Get("Sec-CH-UA-Platform"),
		ReqHdrSecCHUAPlatformVersion:          req.Header.Get("Sec-CH-UA-Platform-Version"),
		ReqHdrSecCHUAWoW64:                    req.Header.Get("Sec-CH-UA-WoW64"),
		ReqHdrSecCHPrefersColorScheme:         req.Header.Get("Sec-CH-Prefers-Color-Scheme"),
		ReqHdrSecCHPrefersReducedMotion:       req.Header.Get("Sec-CH-Prefers-Reduced-Motion"),
		ReqHdrSecCHPrefersReducedTransparency: req.Header.Get("Sec-CH-Prefers-Reduced-Transparency"),
		// 摘要
		ReqHdrWantContentDigest: req.Header.Get("Want-Content-Digest"),
		ReqHdrWantReprDigest:    req.Header.Get("Want-Repr-Digest"),
		// 安全
		ReqHdrUpgradeInsecureRequests: req.Header.Get("Upgrade-Insecure-Requests"),
		// 其他
		ReqHdrDate:     req.Header.Get("Date"),
		ReqHdrLink:     req.Header.Get("Link"),
		ReqHdrUpgrade:  req.Header.Get("Upgrade"),
		ReqHdrPriority: req.Header.Get("Priority"),
		ReqHdrAltUsed:  req.Header.Get("Alt-Used"),
	}
}

// HttpResponseHeaders 表示HTTP响应头信息
// 包含所有标准HTTP响应头字段，参考 MDN HTTP Headers 文档标准
// (https://developer.mozilla.org/zh-CN/docs/Web/HTTP/Reference/Headers)
// 该结构体以大宽表形式定义，每个字段对应一个标准HTTP响应头
//
// 头部分类:
//   - 认证 (Authentication): WWW-Authenticate, Proxy-Authenticate
//   - 缓存 (Caching): Age, Cache-Control, Clear-Site-Data, Expires, No-Vary-Search
//   - 条件请求 (Conditionals): Last-Modified, ETag, Vary
//   - 连接管理 (Connection Management): Connection, Keep-Alive
//   - 内容协商 (Content Negotiation): Accept-Patch, Accept-Post
//   - Cookie: Set-Cookie
//   - CORS: Access-Control-Allow-Origin 等
//   - 下载 (Downloads): Content-Disposition
//   - 摘要 (Digest): Content-Digest, Repr-Digest
//   - 消息主体 (Message Body): Content-Type, Content-Length, Content-Encoding, Content-Language, Content-Location
//   - 代理 (Proxies): Forwarded, Via
//   - 范围请求 (Range Requests): Accept-Ranges, Content-Range
//   - 重定向 (Redirects): Location, Refresh
//   - 响应上下文 (Response Context): Allow, Server
//   - 安全 (Security): Content-Security-Policy, Strict-Transport-Security 等
//   - 服务器发送事件 (Server-Sent Events): Reporting-Endpoints, Report-To
//   - 传输编码 (Transfer Coding): Transfer-Encoding, Trailer
//   - WebSocket: Sec-WebSocket-Accept, Sec-WebSocket-Protocol, Sec-WebSocket-Extensions
//   - 客户端提示 (Client Hints): Accept-CH, Critical-CH
//   - 其他 (Other): Date, Link, Retry-After, Server-Timing, Alt-Svc 等
//
// 使用场景:
//   - HTTP 响应流量分析和监控
//   - 安全策略审计（CSP、HSTS、CORS 等）
//   - Web 应用性能分析与缓存策略优化
type HttpResponseHeaders struct {
	// ======================== 认证 (Authentication) ========================

	// RspHdrWWWAuthenticate 认证质询，定义应该用于访问资源的身份验证方法和参数
	// 使用 Crypto 加密存储，防止涉敏信息泄露后被逆向破解
	RspHdrWWWAuthenticate *serializer.Crypto `gorm:"column:rsp_hdr_www_authenticate;type:String CODEC(ZSTD(3));serializer:crypto" json:"rsp_hdr_www_authenticate" secure:"mode=\"aes-gcm\",offset=44789,length=32"`
	// RspHdrProxyAuthenticate 代理认证质询，定义应用于访问代理服务器后面资源的身份验证方法
	// 使用 Crypto 加密存储，防止涉敏信息泄露后被逆向破解
	RspHdrProxyAuthenticate *serializer.Crypto `gorm:"column:rsp_hdr_proxy_authenticate;type:String CODEC(ZSTD(3));serializer:crypto" json:"rsp_hdr_proxy_authenticate" secure:"mode=\"aes-gcm\",offset=53123,length=32"`

	// ======================== 缓存 (Caching) ========================

	// RspHdrAge 缓存年龄，对象在代理缓存中存储的时间（以秒为单位）
	RspHdrAge int64 `gorm:"column:rsp_hdr_age;type:Int64 CODEC(ZSTD(3))" json:"rsp_hdr_age"`
	// RspHdrCacheControl 缓存控制指令，指定请求/响应链中所有缓存机制必须遵守的指令
	RspHdrCacheControl string `gorm:"column:rsp_hdr_cache_control;type:LowCardinality(String) CODEC(ZSTD(3))" json:"rsp_hdr_cache_control"`
	// RspHdrClearSiteData 清除站点数据，清除与请求网站相关联的浏览器数据（Cookie、Storage、Cache 等）
	RspHdrClearSiteData string `gorm:"column:rsp_hdr_clear_site_data;type:String CODEC(ZSTD(3))" json:"rsp_hdr_clear_site_data"`
	// RspHdrExpires 过期时间，响应被视为过时的日期/时间，在此之后缓存副本将被视为过期
	RspHdrExpires string `gorm:"column:rsp_hdr_expires;type:String CODEC(ZSTD(3))" json:"rsp_hdr_expires"`
	// RspHdrNoVarySearch 无变化搜索，定义 URL 查询参数如何影响缓存匹配的规则集
	RspHdrNoVarySearch string `gorm:"column:rsp_hdr_no_vary_search;type:String CODEC(ZSTD(3))" json:"rsp_hdr_no_vary_search"`

	// ======================== 条件请求 (Conditionals) ========================

	// RspHdrLastModified 最后修改时间，资源的最后修改日期，用于比较同一资源的多个版本
	RspHdrLastModified string `gorm:"column:rsp_hdr_last_modified;type:String CODEC(ZSTD(3))" json:"rsp_hdr_last_modified"`
	// RspHdrETag 实体标签，标识资源版本的唯一字符串，比 Last-Modified 更精确
	RspHdrETag string `gorm:"column:rsp_hdr_etag;type:String CODEC(ZSTD(3))" json:"rsp_hdr_etag"`
	// RspHdrVary 响应变化依据，确定如何匹配请求头来决定是否可使用缓存响应，还是从源服务器请求新响应
	RspHdrVary string `gorm:"column:rsp_hdr_vary;type:LowCardinality(String) CODEC(ZSTD(3))" json:"rsp_hdr_vary"`

	// ======================== 连接管理 (Connection Management) ========================

	// RspHdrConnection 连接控制，控制当前事务完成后网络连接是否保持打开状态
	RspHdrConnection string `gorm:"column:rsp_hdr_connection;type:LowCardinality(String) CODEC(ZSTD(3))" json:"rsp_hdr_connection"`
	// RspHdrKeepAlive 持久连接控制，设置持久连接的超时时间和最大请求数
	RspHdrKeepAlive string `gorm:"column:rsp_hdr_keep_alive;type:String CODEC(ZSTD(3))" json:"rsp_hdr_keep_alive"`

	// ======================== 内容协商 (Content Negotiation) ========================

	// RspHdrAcceptPatch 可接受的 PATCH 媒体类型，在 PATCH 响应中通告服务器能够理解的媒体类型
	RspHdrAcceptPatch string `gorm:"column:rsp_hdr_accept_patch;type:String CODEC(ZSTD(3))" json:"rsp_hdr_accept_patch"`
	// RspHdrAcceptPost 可接受的 POST 媒体类型，在 POST 响应中通告服务器能够理解的媒体类型
	RspHdrAcceptPost string `gorm:"column:rsp_hdr_accept_post;type:String CODEC(ZSTD(3))" json:"rsp_hdr_accept_post"`

	// ======================== Cookie ========================

	// RspHdrSetCookie 设置 Cookie，将 cookie 从服务器发送到用户代理，以便后续请求中回传
	// 使用 Crypto 加密存储，Set-Cookie 常包含会话令牌等涉敏信息
	RspHdrSetCookie *serializer.Crypto `gorm:"column:rsp_hdr_set_cookie;type:String CODEC(ZSTD(3));serializer:crypto" json:"rsp_hdr_set_cookie" secure:"mode=\"aes-gcm\",offset=61234,length=32"`

	// ======================== CORS ========================

	// RspHdrAccessControlAllowCredentials CORS 允许凭据，指示当凭据标志为 true 时是否可以暴露请求的响应
	RspHdrAccessControlAllowCredentials string `gorm:"column:rsp_hdr_access_control_allow_credentials;type:LowCardinality(String) CODEC(ZSTD(3))" json:"rsp_hdr_access_control_allow_credentials"`
	// RspHdrAccessControlAllowHeaders CORS 允许标头，指示实际请求中可以使用哪些 HTTP 标头
	RspHdrAccessControlAllowHeaders string `gorm:"column:rsp_hdr_access_control_allow_headers;type:String CODEC(ZSTD(3))" json:"rsp_hdr_access_control_allow_headers"`
	// RspHdrAccessControlAllowMethods CORS 允许方法，指定访问资源允许的 HTTP 方法
	RspHdrAccessControlAllowMethods string `gorm:"column:rsp_hdr_access_control_allow_methods;type:String CODEC(ZSTD(3))" json:"rsp_hdr_access_control_allow_methods"`
	// RspHdrAccessControlAllowOrigin CORS 允许源，指示响应是否可以被共享，值为 * 或具体的源
	RspHdrAccessControlAllowOrigin string `gorm:"column:rsp_hdr_access_control_allow_origin;type:String CODEC(ZSTD(3))" json:"rsp_hdr_access_control_allow_origin"`
	// RspHdrAccessControlExposeHeaders CORS 暴露标头，通过列出标头名称指示哪些标头可以作为响应的一部分暴露给脚本
	RspHdrAccessControlExposeHeaders string `gorm:"column:rsp_hdr_access_control_expose_headers;type:String CODEC(ZSTD(3))" json:"rsp_hdr_access_control_expose_headers"`
	// RspHdrAccessControlMaxAge CORS 最大缓存时间，指示预检请求的结果能被缓存多久（秒）
	RspHdrAccessControlMaxAge int64 `gorm:"column:rsp_hdr_access_control_max_age;type:Int64 CODEC(ZSTD(3))" json:"rsp_hdr_access_control_max_age"`
	// RspHdrTimingAllowOrigin 时间允许源，指定允许访问 Resource Timing API 属性值的源
	RspHdrTimingAllowOrigin string `gorm:"column:rsp_hdr_timing_allow_origin;type:String CODEC(ZSTD(3))" json:"rsp_hdr_timing_allow_origin"`

	// ======================== 下载 (Downloads) ========================

	// RspHdrContentDisposition 内容处置，指示传输的资源应内联显示还是作为下载处理，可指定文件名
	RspHdrContentDisposition string `gorm:"column:rsp_hdr_content_disposition;type:String CODEC(ZSTD(3))" json:"rsp_hdr_content_disposition"`

	// ======================== 摘要 (Digest) ========================

	// RspHdrContentDigest 内容摘要，提供 HTTP 消息中构建的字节流的校验摘要（考虑 Content-Encoding 和 Content-Range）
	RspHdrContentDigest string `gorm:"column:rsp_hdr_content_digest;type:String CODEC(ZSTD(3))" json:"rsp_hdr_content_digest"`
	// RspHdrReprDigest 表示摘要，在传输前提供指定形式目标资源的校验摘要（不考虑 Content-Encoding 和 Content-Range）
	RspHdrReprDigest string `gorm:"column:rsp_hdr_repr_digest;type:String CODEC(ZSTD(3))" json:"rsp_hdr_repr_digest"`

	// ======================== 消息主体 (Message Body Information) ========================

	// RspHdrContentType 响应内容类型（MIME 类型），指示响应主体的媒体格式
	RspHdrContentType string `gorm:"column:rsp_hdr_content_type;type:LowCardinality(String) CODEC(ZSTD(3))" json:"rsp_hdr_content_type"`
	// RspHdrContentLength 响应内容长度，响应主体的大小（以十进制字节数表示）
	RspHdrContentLength int64 `gorm:"column:rsp_hdr_content_length;type:Int64 CODEC(ZSTD(3))" json:"rsp_hdr_content_length"`
	// RspHdrContentEncoding 响应内容编码，用于指定响应主体的压缩算法，如 gzip、br
	RspHdrContentEncoding string `gorm:"column:rsp_hdr_content_encoding;type:LowCardinality(String) CODEC(ZSTD(3))" json:"rsp_hdr_content_encoding"`
	// RspHdrContentLanguage 响应内容语言，描述面向受众的人类语言
	RspHdrContentLanguage string `gorm:"column:rsp_hdr_content_language;type:LowCardinality(String) CODEC(ZSTD(3))" json:"rsp_hdr_content_language"`
	// RspHdrContentLocation 内容位置，指示返回数据的备用位置（直接访问 URL）
	RspHdrContentLocation string `gorm:"column:rsp_hdr_content_location;type:String CODEC(ZSTD(3))" json:"rsp_hdr_content_location"`

	// ======================== 代理 (Proxies) ========================

	// RspHdrForwarded 转发信息，包含来自代理服务器面向客户端一侧的信息（原始 IP、协议等）
	RspHdrForwarded string `gorm:"column:rsp_hdr_forwarded;type:String CODEC(ZSTD(3))" json:"rsp_hdr_forwarded"`
	// RspHdrVia 代理链信息，由代理（正向和反向代理）添加，记录响应经过的中间节点
	RspHdrVia string `gorm:"column:rsp_hdr_via;type:String CODEC(ZSTD(3))" json:"rsp_hdr_via"`

	// ======================== 范围请求 (Range Requests) ========================

	// RspHdrAcceptRanges 接受范围请求，指示服务器是否支持范围请求及使用的单位（如 bytes、none）
	RspHdrAcceptRanges string `gorm:"column:rsp_hdr_accept_ranges;type:LowCardinality(String) CODEC(ZSTD(3))" json:"rsp_hdr_accept_ranges"`
	// RspHdrContentRange 内容范围，指示部分消息在完整主体中的位置（起始-结束/总大小）
	RspHdrContentRange string `gorm:"column:rsp_hdr_content_range;type:String CODEC(ZSTD(3))" json:"rsp_hdr_content_range"`

	// ======================== 重定向 (Redirects) ========================

	// RspHdrLocation 重定向位置，指示要将页面重定向到的目标 URL（配合 3xx 状态码使用）
	RspHdrLocation string `gorm:"column:rsp_hdr_location;type:String CODEC(ZSTD(3))" json:"rsp_hdr_location"`
	// RspHdrRefresh 刷新/重定向，指示浏览器重新加载页面或重定向到另一页面，可指定延迟秒数
	RspHdrRefresh string `gorm:"column:rsp_hdr_refresh;type:String CODEC(ZSTD(3))" json:"rsp_hdr_refresh"`

	// ======================== 响应上下文 (Response Context) ========================

	// RspHdrAllow 允许的方法，列出目标资源所支持的 HTTP 方法集合
	RspHdrAllow string `gorm:"column:rsp_hdr_allow;type:String CODEC(ZSTD(3))" json:"rsp_hdr_allow"`
	// RspHdrServer 服务器标识，包含处理请求的源服务器所用到的软件相关信息
	RspHdrServer string `gorm:"column:rsp_hdr_server;type:LowCardinality(String) CODEC(ZSTD(3))" json:"rsp_hdr_server"`

	// ======================== 安全 (Security) ========================

	// RspHdrCrossOriginEmbedderPolicy 跨源嵌入器策略（COEP），为给定文档声明嵌入器策略
	RspHdrCrossOriginEmbedderPolicy string `gorm:"column:rsp_hdr_cross_origin_embedder_policy;type:LowCardinality(String) CODEC(ZSTD(3))" json:"rsp_hdr_cross_origin_embedder_policy"`
	// RspHdrCrossOriginOpenerPolicy 跨源打开器策略（COOP），防止其他域打开/控制窗口
	RspHdrCrossOriginOpenerPolicy string `gorm:"column:rsp_hdr_cross_origin_opener_policy;type:LowCardinality(String) CODEC(ZSTD(3))" json:"rsp_hdr_cross_origin_opener_policy"`
	// RspHdrCrossOriginResourcePolicy 跨源资源策略（CORP），防止从其他域读取应用此标头的资源的响应
	RspHdrCrossOriginResourcePolicy string `gorm:"column:rsp_hdr_cross_origin_resource_policy;type:LowCardinality(String) CODEC(ZSTD(3))" json:"rsp_hdr_cross_origin_resource_policy"`
	// RspHdrContentSecurityPolicy 内容安全策略（CSP），控制用户代理可以为给定页面加载哪些资源
	RspHdrContentSecurityPolicy string `gorm:"column:rsp_hdr_content_security_policy;type:String CODEC(ZSTD(3))" json:"rsp_hdr_content_security_policy"`
	// RspHdrContentSecurityPolicyReportOnly CSP 仅报告模式，允许通过监控（不强制执行）策略效果进行实验
	RspHdrContentSecurityPolicyReportOnly string `gorm:"column:rsp_hdr_content_security_policy_report_only;type:String CODEC(ZSTD(3))" json:"rsp_hdr_content_security_policy_report_only"`
	// RspHdrExpectCT 证书透明度期望，允许网站报告和执行证书透明度要求
	RspHdrExpectCT string `gorm:"column:rsp_hdr_expect_ct;type:String CODEC(ZSTD(3))" json:"rsp_hdr_expect_ct"`
	// RspHdrPermissionsPolicy 权限策略，允许和拒绝在网站自身框架和嵌入 iframe 中使用浏览器特性
	RspHdrPermissionsPolicy string `gorm:"column:rsp_hdr_permissions_policy;type:String CODEC(ZSTD(3))" json:"rsp_hdr_permissions_policy"`
	// RspHdrStrictTransportSecurity 严格传输安全（HSTS），强制使用 HTTPS 而不是 HTTP 进行通信
	RspHdrStrictTransportSecurity string `gorm:"column:rsp_hdr_strict_transport_security;type:String CODEC(ZSTD(3))" json:"rsp_hdr_strict_transport_security"`
	// RspHdrXContentTypeOptions 内容类型选项，禁用 MIME 嗅探并强制浏览器使用 Content-Type 中声明的类型
	RspHdrXContentTypeOptions string `gorm:"column:rsp_hdr_x_content_type_options;type:LowCardinality(String) CODEC(ZSTD(3))" json:"rsp_hdr_x_content_type_options"`
	// RspHdrXFrameOptions 框架选项（XFO），指示是否允许浏览器在 frame/iframe/embed/object 中渲染页面
	RspHdrXFrameOptions string `gorm:"column:rsp_hdr_x_frame_options;type:LowCardinality(String) CODEC(ZSTD(3))" json:"rsp_hdr_x_frame_options"`
	// RspHdrXPermittedCrossDomainPolicies 允许的跨域策略，覆盖跨域策略文件以防止不希望的请求
	RspHdrXPermittedCrossDomainPolicies string `gorm:"column:rsp_hdr_x_permitted_cross_domain_policies;type:LowCardinality(String) CODEC(ZSTD(3))" json:"rsp_hdr_x_permitted_cross_domain_policies"`
	// RspHdrXPoweredBy 技术引擎标识，可能由托管环境或框架设置，包含有关技术栈的信息
	RspHdrXPoweredBy string `gorm:"column:rsp_hdr_x_powered_by;type:LowCardinality(String) CODEC(ZSTD(3))" json:"rsp_hdr_x_powered_by"`
	// RspHdrXXSSProtection XSS 保护，启用浏览器的跨站脚本攻击（XSS）过滤机制
	RspHdrXXSSProtection string `gorm:"column:rsp_hdr_x_xss_protection;type:LowCardinality(String) CODEC(ZSTD(3))" json:"rsp_hdr_x_xss_protection"`

	// ======================== 服务器发送事件 (Server-Sent Events) ========================

	// RspHdrReportingEndpoints 报告端点，指定浏览器应使用 Reporting API 发送警告和错误报告的服务器端点
	RspHdrReportingEndpoints string `gorm:"column:rsp_hdr_reporting_endpoints;type:String CODEC(ZSTD(3))" json:"rsp_hdr_reporting_endpoints"`
	// RspHdrReportTo 报告目标，指定浏览器应使用 Reporting API 发送报告的服务器端点（旧版）
	RspHdrReportTo string `gorm:"column:rsp_hdr_report_to;type:String CODEC(ZSTD(3))" json:"rsp_hdr_report_to"`

	// ======================== 传输编码 (Transfer Coding) ========================

	// RspHdrTransferEncoding 传输编码，指定用于将资源安全传输给用户的编码形式，如 chunked
	RspHdrTransferEncoding string `gorm:"column:rsp_hdr_transfer_encoding;type:LowCardinality(String) CODEC(ZSTD(3))" json:"rsp_hdr_transfer_encoding"`
	// RspHdrTrailer 尾部字段声明，允许发送方在分块消息的末尾包含额外的元数据字段
	RspHdrTrailer string `gorm:"column:rsp_hdr_trailer;type:String CODEC(ZSTD(3))" json:"rsp_hdr_trailer"`

	// ======================== WebSocket ========================

	// RspHdrSecWebSocketAccept WebSocket 握手接受，指示服务器愿意升级到 WebSocket 连接
	// 使用 Crypto 加密存储，WebSocket 握手响应涉及连接安全
	RspHdrSecWebSocketAccept *serializer.Crypto `gorm:"column:rsp_hdr_sec_websocket_accept;type:String CODEC(ZSTD(3));serializer:crypto" json:"rsp_hdr_sec_websocket_accept" secure:"mode=\"aes-gcm\",offset=70278,length=32"`
	// RspHdrSecWebSocketProtocol WebSocket 子协议，指示服务器从客户端的偏好中选择的子协议
	RspHdrSecWebSocketProtocol string `gorm:"column:rsp_hdr_sec_websocket_protocol;type:String CODEC(ZSTD(3))" json:"rsp_hdr_sec_websocket_protocol"`
	// RspHdrSecWebSocketExtensions WebSocket 扩展，指示服务器从客户端的偏好中选择的扩展
	RspHdrSecWebSocketExtensions string `gorm:"column:rsp_hdr_sec_websocket_extensions;type:String CODEC(ZSTD(3))" json:"rsp_hdr_sec_websocket_extensions"`

	// ======================== 客户端提示 (Client Hints) ========================

	// RspHdrAcceptCH 接受客户端提示，服务器宣告支持哪些客户端提示标头
	RspHdrAcceptCH string `gorm:"column:rsp_hdr_accept_ch;type:String CODEC(ZSTD(3))" json:"rsp_hdr_accept_ch"`
	// RspHdrCriticalCH 关键客户端提示，服务器指定哪些接受的客户端提示为关键提示
	RspHdrCriticalCH string `gorm:"column:rsp_hdr_critical_ch;type:String CODEC(ZSTD(3))" json:"rsp_hdr_critical_ch"`

	// ======================== 归因报告 (Attribution Reporting) ========================

	// RspHdrAttributionReportingEligible 归因报告资格，指示当前响应有资格注册归因来源或触发器
	RspHdrAttributionReportingEligible string `gorm:"column:rsp_hdr_attribution_reporting_eligible;type:String CODEC(ZSTD(3))" json:"rsp_hdr_attribution_reporting_eligible"`
	// RspHdrAttributionReportingRegisterSource 归因报告注册来源，用于注册归因来源
	RspHdrAttributionReportingRegisterSource string `gorm:"column:rsp_hdr_attribution_reporting_register_source;type:String CODEC(ZSTD(3))" json:"rsp_hdr_attribution_reporting_register_source"`
	// RspHdrAttributionReportingRegisterTrigger 归因报告注册触发器，用于注册归因触发器
	RspHdrAttributionReportingRegisterTrigger string `gorm:"column:rsp_hdr_attribution_reporting_register_trigger;type:String CODEC(ZSTD(3))" json:"rsp_hdr_attribution_reporting_register_trigger"`

	// ======================== 其他 (Other) ========================

	// RspHdrDate 消息创建日期和时间
	RspHdrDate string `gorm:"column:rsp_hdr_date;type:String CODEC(ZSTD(3))" json:"rsp_hdr_date"`
	// RspHdrLink 链接关系，提供在 HTTP 标头中序列化链接的方法，语义上等同于 HTML <link> 元素
	RspHdrLink string `gorm:"column:rsp_hdr_link;type:String CODEC(ZSTD(3))" json:"rsp_hdr_link"`
	// RspHdrRetryAfter 重试时间，指示用户代理在进行后续请求前应等待的时间（秒或日期）
	RspHdrRetryAfter string `gorm:"column:rsp_hdr_retry_after;type:String CODEC(ZSTD(3))" json:"rsp_hdr_retry_after"`
	// RspHdrServerTiming 服务器时间，传达请求-响应周期中的一个或多个性能参数或描述
	RspHdrServerTiming string `gorm:"column:rsp_hdr_server_timing;type:String CODEC(ZSTD(3))" json:"rsp_hdr_server_timing"`
	// RspHdrServiceWorker Service Worker 标识，包含在 SW 脚本资源的获取请求中
	RspHdrServiceWorker string `gorm:"column:rsp_hdr_service_worker;type:String CODEC(ZSTD(3))" json:"rsp_hdr_service_worker"`
	// RspHdrServiceWorkerAllowed Service Worker 允许路径，移除 SW 注册的路径限制
	RspHdrServiceWorkerAllowed string `gorm:"column:rsp_hdr_service_worker_allowed;type:String CODEC(ZSTD(3))" json:"rsp_hdr_service_worker_allowed"`
	// RspHdrSourceMap 源代码映射，链接到源代码映射文件以便调试器可逐步执行原始源代码
	RspHdrSourceMap string `gorm:"column:rsp_hdr_source_map;type:String CODEC(ZSTD(3))" json:"rsp_hdr_source_map"`
	// RspHdrUpgrade 协议升级，用于将已建立的连接升级为不同协议
	RspHdrUpgrade string `gorm:"column:rsp_hdr_upgrade;type:String CODEC(ZSTD(3))" json:"rsp_hdr_upgrade"`
	// RspHdrAltSvc 替代服务，用于列出访问该服务的替代方式（不同协议/端口）
	RspHdrAltSvc string `gorm:"column:rsp_hdr_alt_svc;type:String CODEC(ZSTD(3))" json:"rsp_hdr_alt_svc"`
	// RspHdrPriority 资源优先级，服务器在响应中可用于重新调整请求的优先级
	RspHdrPriority string `gorm:"column:rsp_hdr_priority;type:String CODEC(ZSTD(3))" json:"rsp_hdr_priority"`
}

func NewHttpResponseHeaders(resp *http.Response) *HttpResponseHeaders {
	return &HttpResponseHeaders{
		// 认证（加密存储）
		RspHdrWWWAuthenticate:   newCrypto(resp.Header.Get("WWW-Authenticate")),
		RspHdrProxyAuthenticate: newCrypto(resp.Header.Get("Proxy-Authenticate")),
		// 缓存
		RspHdrAge:           parseInt64(resp.Header.Get("Age")),
		RspHdrCacheControl:  resp.Header.Get("Cache-Control"),
		RspHdrClearSiteData: resp.Header.Get("Clear-Site-Data"),
		RspHdrExpires:       resp.Header.Get("Expires"),
		RspHdrNoVarySearch:  resp.Header.Get("No-Vary-Search"),
		// 条件请求
		RspHdrLastModified: resp.Header.Get("Last-Modified"),
		RspHdrETag:         resp.Header.Get("ETag"),
		RspHdrVary:         resp.Header.Get("Vary"),
		// 连接管理
		RspHdrConnection: resp.Header.Get("Connection"),
		RspHdrKeepAlive:  resp.Header.Get("Keep-Alive"),
		// 内容协商
		RspHdrAcceptPatch: resp.Header.Get("Accept-Patch"),
		RspHdrAcceptPost:  resp.Header.Get("Accept-Post"),
		// Cookie（加密存储）
		RspHdrSetCookie: newCrypto(resp.Header.Get("Set-Cookie")),
		// CORS
		RspHdrAccessControlAllowCredentials: resp.Header.Get("Access-Control-Allow-Credentials"),
		RspHdrAccessControlAllowHeaders:     resp.Header.Get("Access-Control-Allow-Headers"),
		RspHdrAccessControlAllowMethods:     resp.Header.Get("Access-Control-Allow-Methods"),
		RspHdrAccessControlAllowOrigin:      resp.Header.Get("Access-Control-Allow-Origin"),
		RspHdrAccessControlExposeHeaders:    resp.Header.Get("Access-Control-Expose-Headers"),
		RspHdrAccessControlMaxAge:           parseInt64(resp.Header.Get("Access-Control-Max-Age")),
		RspHdrTimingAllowOrigin:             resp.Header.Get("Timing-Allow-Origin"),
		// 下载
		RspHdrContentDisposition: resp.Header.Get("Content-Disposition"),
		// 摘要
		RspHdrContentDigest: resp.Header.Get("Content-Digest"),
		RspHdrReprDigest:    resp.Header.Get("Repr-Digest"),
		// 消息主体
		RspHdrContentType:     resp.Header.Get("Content-Type"),
		RspHdrContentLength:   resp.ContentLength,
		RspHdrContentEncoding: resp.Header.Get("Content-Encoding"),
		RspHdrContentLanguage: resp.Header.Get("Content-Language"),
		RspHdrContentLocation: resp.Header.Get("Content-Location"),
		// 代理
		RspHdrForwarded: resp.Header.Get("Forwarded"),
		RspHdrVia:       resp.Header.Get("Via"),
		// 范围请求
		RspHdrAcceptRanges: resp.Header.Get("Accept-Ranges"),
		RspHdrContentRange: resp.Header.Get("Content-Range"),
		// 重定向
		RspHdrLocation: resp.Header.Get("Location"),
		RspHdrRefresh:  resp.Header.Get("Refresh"),
		// 响应上下文
		RspHdrAllow:  resp.Header.Get("Allow"),
		RspHdrServer: resp.Header.Get("Server"),
		// 安全
		RspHdrCrossOriginEmbedderPolicy:       resp.Header.Get("Cross-Origin-Embedder-Policy"),
		RspHdrCrossOriginOpenerPolicy:         resp.Header.Get("Cross-Origin-Opener-Policy"),
		RspHdrCrossOriginResourcePolicy:       resp.Header.Get("Cross-Origin-Resource-Policy"),
		RspHdrContentSecurityPolicy:           resp.Header.Get("Content-Security-Policy"),
		RspHdrContentSecurityPolicyReportOnly: resp.Header.Get("Content-Security-Policy-Report-Only"),
		RspHdrExpectCT:                        resp.Header.Get("Expect-CT"),
		RspHdrPermissionsPolicy:               resp.Header.Get("Permissions-Policy"),
		RspHdrStrictTransportSecurity:         resp.Header.Get("Strict-Transport-Security"),
		RspHdrXContentTypeOptions:             resp.Header.Get("X-Content-Type-Options"),
		RspHdrXFrameOptions:                   resp.Header.Get("X-Frame-Options"),
		RspHdrXPermittedCrossDomainPolicies:   resp.Header.Get("X-Permitted-Cross-Domain-Policies"),
		RspHdrXPoweredBy:                      resp.Header.Get("X-Powered-By"),
		RspHdrXXSSProtection:                  resp.Header.Get("X-XSS-Protection"),
		// 服务器发送事件
		RspHdrReportingEndpoints: resp.Header.Get("Reporting-Endpoints"),
		RspHdrReportTo:           resp.Header.Get("Report-To"),
		// 传输编码
		RspHdrTransferEncoding: resp.Header.Get("Transfer-Encoding"),
		RspHdrTrailer:          resp.Header.Get("Trailer"),
		// WebSocket（加密存储）
		RspHdrSecWebSocketAccept:     newCrypto(resp.Header.Get("Sec-WebSocket-Accept")),
		RspHdrSecWebSocketProtocol:   resp.Header.Get("Sec-WebSocket-Protocol"),
		RspHdrSecWebSocketExtensions: resp.Header.Get("Sec-WebSocket-Extensions"),
		// 客户端提示
		RspHdrAcceptCH:   resp.Header.Get("Accept-CH"),
		RspHdrCriticalCH: resp.Header.Get("Critical-CH"),
		// 归因报告
		RspHdrAttributionReportingEligible:        resp.Header.Get("Attribution-Reporting-Eligible"),
		RspHdrAttributionReportingRegisterSource:  resp.Header.Get("Attribution-Reporting-Register-Source"),
		RspHdrAttributionReportingRegisterTrigger: resp.Header.Get("Attribution-Reporting-Register-Trigger"),
		// 其他
		RspHdrDate:                 resp.Header.Get("Date"),
		RspHdrLink:                 resp.Header.Get("Link"),
		RspHdrRetryAfter:           resp.Header.Get("Retry-After"),
		RspHdrServerTiming:         resp.Header.Get("Server-Timing"),
		RspHdrServiceWorker:        resp.Header.Get("Service-Worker"),
		RspHdrServiceWorkerAllowed: resp.Header.Get("Service-Worker-Allowed"),
		RspHdrSourceMap:            resp.Header.Get("SourceMap"),
		RspHdrUpgrade:              resp.Header.Get("Upgrade"),
		RspHdrAltSvc:               resp.Header.Get("Alt-Svc"),
		RspHdrPriority:             resp.Header.Get("Priority"),
	}
}

// HttpRequestBody 表示HTTP请求正文信息
// 包含HTTP请求的方法、主体内容和时间信息，头部字段已迁移至 HttpRequestHeaders
type HttpRequestBody struct {
	// Method HTTP请求方法，如GET、POST、PUT、DELETE等
	Method string `gorm:"type:LowCardinality(String) CODEC(ZSTD(3))" json:"method"`
	// ReqBody 请求主体内容，保存至 oss，若为空字符串，则表示内容为空，避免在 oss 内存储大量无效空文件
	ReqBody string `gorm:"type:String CODEC(ZSTD(3))" json:"req_body"`
	// ReqTime 请求时间，HTTP请求开始的时间戳
	ReqTime time.Time `gorm:"type:DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3))" json:"req_time"`
	// ReqDuration 请求持续时间，从请求开始到请求完成的时间
	ReqDuration time.Duration `gorm:"type:Int64 CODEC(ZSTD(3))" json:"req_duration"`
}

func NewHttpRequestBody(ctx context.Context, client *s3.S3Client, req *http.Request, startTime, endTime time.Time, cryptoKey []byte) (*HttpRequestBody, error) {
	defer req.Body.Close()
	body, err := decompressRequestBody(req)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer body.Close()
	id := serializer.NewHexUUIDv4()
	key := objectKey(id.String(), true)
	zkcn := s3.NewZkcn(client, key, cryptoKey, 0)
	if _, err := zkcn.Store(ctx, body); err != nil {
		return nil, errors.WithStack(err)
	}
	return &HttpRequestBody{
		Method:      req.Method,
		ReqBody:     key,
		ReqTime:     startTime,
		ReqDuration: endTime.Sub(startTime),
	}, nil
}

// HttpResponseBody 表示HTTP响应正文信息
// 包含HTTP响应的主体内容和时间信息，头部字段已迁移至 HttpResponseHeaders
type HttpResponseBody struct {
	// RspBody 响应主体内容，保存至 oss，若为空字符串，则表示内容为空，避免在 oss 内存储大量无效空文件
	RspBody string `gorm:"type:String CODEC(ZSTD(3))" json:"rsp_body"`
	// RspTime 响应时间，HTTP响应开始的时间戳
	RspTime time.Time `gorm:"type:DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3))" json:"rsp_time"`
	// RspDuration 响应持续时间，从响应开始到响应完成的时间
	RspDuration time.Duration `gorm:"type:Int64 CODEC(ZSTD(3))" json:"rsp_duration"`
}

func NewHttpResponseBody(ctx context.Context, client *s3.S3Client, resp *http.Response, startTime, endTime time.Time, cryptoKey []byte) (*HttpResponseBody, error) {
	defer resp.Body.Close()
	body, err := decompressResponseBody(resp)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer body.Close()
	id := serializer.NewHexUUIDv4()
	key := objectKey(id.String(), false)
	zkcn := s3.NewZkcn(client, key, cryptoKey, 0)
	if _, err := zkcn.Store(ctx, body); err != nil {
		return nil, errors.WithStack(err)
	}
	return &HttpResponseBody{
		RspBody:     key,
		RspTime:     startTime,
		RspDuration: endTime.Sub(startTime),
	}, nil
}

// HttpTransport 表示HTTP传输层信息
// 完整的HTTP传输数据模型，组合了所有HTTP相关的元数据和内容信息
// 该结构体对应数据库中的http_transport表，用于存储完整的HTTP会话数据
//
// 结构组成:
//   - BaseMeta:             基础元数据，包含命名空间、捕获器、版本和会话ID
//   - FusedMeta:            融合元数据，包含帧堆栈信息
//   - HttpBase:             HTTP基础信息，包含客户端/服务器IP端口和协议版本
//   - HttpURI:              HTTP URI信息，包含URL的各个组成部分
//   - HttpRequestBody:      HTTP请求正文信息，包含请求方法、主体和时间信息
//   - HttpRequestHeaders:   HTTP请求头信息，以宽表形式存储所有标准请求头字段
//   - HttpResponse:         HTTP响应正文信息，包含响应主体和时间信息
//   - HttpResponseHeaders:  HTTP响应头信息，以宽表形式存储所有标准响应头字段
//
// 使用场景:
//   - HTTP流量分析和监控
//   - Web应用性能分析
//   - API调用追踪和调试
//   - 安全审计和入侵检测
type HttpTransport struct {
	// BaseMeta 基础元数据，提供数据隔离和会话标识
	BaseMeta
	// FusedMeta 融合元数据，提供帧堆栈信息
	FusedMeta
	// HttpBase HTTP基础连接信息
	HttpBase
	// HttpURI HTTP请求URI信息
	HttpURI
	// HttpRequestBody HTTP请求正文信息
	HttpRequestBody
	// HttpRequestHeaders HTTP请求头信息
	HttpRequestHeaders
	// HttpResponseBody HTTP响应正文信息
	HttpResponseBody
	// HttpResponseHeaders HTTP响应头信息
	HttpResponseHeaders
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
func NewHttpTransport(
	ctx context.Context,
	s3Client *s3.S3Client,
	meta *BaseMeta,
	clientAddr, serverAddr net.IP,
	clientPort, serverPort layers.TCPPort,
	clientFrameStack, serverFrameStack FrameStack,
	request *http.Request, response *http.Response,
	kd *key.KeyDerivation,
) (transport *HttpTransport, err error) {
	if meta == nil {
		return nil, errors.New("meta is nil")
	}
	var hBase HttpBase
	hBase.ClientIP = serializer.IPAddr(clientAddr)
	hBase.ClientPort = clientPort
	hBase.ServerIP = serializer.IPAddr(serverAddr)
	hBase.ServerPort = serverPort
	if clientAddr.To4() != nil {
		hBase.IPVersion = layers.IPProtocolIPv4
	} else {
		hBase.IPVersion = layers.IPProtocolIPv6
	}
	cryptoKey, err := kd.DeriveChildKey(CRYPTO_KEY_OFFSET, CRYPTO_KEY_LENGTH)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var uri *HttpURI
	var requestBody *HttpRequestBody
	var requestHeaders *HttpRequestHeaders
	var responseBody *HttpResponseBody
	var responseHeaders *HttpResponseHeaders
	wg, ctx := errgroupx.WithContext(ctx)
	wg.GoNamed("req-build", func() error {
		if request != nil && len(clientFrameStack) != 0 {
			uri, err = NewHttpURI(request.URL)
			if err != nil {
				return errors.WithStack(err)
			}
			startTime := clientFrameStack[0].Timestamp
			endTime := clientFrameStack[len(clientFrameStack)-1].Timestamp
			requestBody, err = NewHttpRequestBody(ctx, s3Client, request, startTime, endTime, cryptoKey)
			if err != nil {
				return errors.WithStack(err)
			}
			requestHeaders = NewHttpRequestHeaders(request)
			hBase.HttpVersion = request.Proto
		}
		return nil
	})
	wg.GoNamed("rsp-build", func() error {
		if response != nil && len(serverFrameStack) != 0 {
			startTime := serverFrameStack[0].Timestamp
			endTime := serverFrameStack[len(serverFrameStack)-1].Timestamp
			responseBody, err = NewHttpResponseBody(ctx, s3Client, response, startTime, endTime, cryptoKey)
			if err != nil {
				return errors.WithStack(err)
			}
		}
		responseHeaders = NewHttpResponseHeaders(response)
		return nil
	})
	if err := wg.Wait(); err != nil {
		return nil, errors.WithStack(err)
	}
	if uri == nil {
		uri = &HttpURI{}
	}
	if requestBody == nil {
		requestBody = &HttpRequestBody{}
	}
	if requestHeaders == nil {
		requestHeaders = &HttpRequestHeaders{}
	}
	if responseBody == nil {
		responseBody = &HttpResponseBody{}
	}
	if responseHeaders == nil {
		responseHeaders = &HttpResponseHeaders{}
	}
	return &HttpTransport{
		BaseMeta: *meta,
		FusedMeta: FusedMeta{
			FrameStack: sortedFrameStack(clientFrameStack, serverFrameStack),
		},
		HttpBase:            hBase,
		HttpURI:             *uri,
		HttpRequestBody:     *requestBody,
		HttpRequestHeaders:  *requestHeaders,
		HttpResponseBody:    *responseBody,
		HttpResponseHeaders: *responseHeaders,
	}, nil
}
