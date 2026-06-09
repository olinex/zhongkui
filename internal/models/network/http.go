package network

import (
	"time"

	"github.com/gopacket/gopacket/layers"
	"gorm.io/datatypes"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

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

// HttpRequest 表示HTTP请求信息
// 包含HTTP请求的完整信息，包括请求方法、头部、主体和时间信息
type HttpRequest struct {
	// Method HTTP请求方法，如GET、POST、PUT、DELETE等
	Method string `gorm:"type:LowCardinality(String) CODEC(ZSTD(3))" json:"method"`
	// UserAgent 用户代理字符串，标识客户端浏览器或应用程序
	UserAgent string `gorm:"type:String CODEC(ZSTD(3))" json:"user_agent"`
	// Referer 来源页面URL，指示请求是从哪个页面发起的
	Referer string `gorm:"type:String CODEC(ZSTD(3))" json:"referer"`
	// Accept 可接受的响应内容类型，如application/json、text/html等
	Accept string `gorm:"type:LowCardinality(String) CODEC(ZSTD(3))" json:"accept"`
	// AcceptEncoding 可接受的内容编码，如gzip、deflate等
	AcceptEncoding string `gorm:"type:LowCardinality(String) CODEC(ZSTD(3))" json:"accept_encoding"`
	// AcceptLanguage 可接受的语言，如zh-CN、en-US等
	AcceptLanguage string `gorm:"type:LowCardinality(String) CODEC(ZSTD(3))" json:"accept_language"`
	// Connection 连接控制头部，如keep-alive、close等
	Connection string `gorm:"type:LowCardinality(String) CODEC(ZSTD(3))" json:"connection"`
	// Cookie 客户端发送的Cookie信息
	Cookie string `gorm:"type:String CODEC(ZSTD(3))" json:"cookie"`
	// Authorization 认证信息，如Bearer token、Basic认证等
	Authorization string `gorm:"type:String CODEC(ZSTD(3))" json:"authorization"`
	// ReqContentType 请求内容类型，标识请求主体的格式
	ReqContentType string `gorm:"type:LowCardinality(String) CODEC(ZSTD(3))" json:"req_content_type"`
	// ReqContentLength 请求内容长度，请求主体的大小（字节）
	ReqContentLength uint32 `gorm:"type:UInt32 CODEC(ZSTD(3))" json:"req_content_length"`
	// ReqHeaders 完整的请求头部，以JSON格式存储所有请求头部
	ReqHeaders datatypes.JSON `gorm:"type:String CODEC(ZSTD(3))" json:"req_headers"`
	// ReqBody 请求主体内容，原始字节数据
	ReqBody []byte `gorm:"type:String CODEC(ZSTD(3))" json:"req_body"`
	// ReqTime 请求时间，HTTP请求开始的时间戳
	ReqTime time.Time `gorm:"type:DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3))" json:"req_time"`
	// ReqDuration 请求持续时间，从请求开始到请求完成的时间
	ReqDuration time.Duration `gorm:"type:Int64 CODEC(ZSTD(3))" json:"req_duration"`
	// IsReqCompleted 请求是否完成，标识HTTP请求是否已完整接收
	IsReqCompleted bool `gorm:"type:Bool" json:"is_req_completed"`
}

// HttpResponse 表示HTTP响应信息
// 包含HTTP响应的完整信息，包括响应头部、主体和时间信息
type HttpResponse struct {
	// Server 服务器标识，响应头中的Server字段
	Server string `gorm:"type:LowCardinality(String) CODEC(ZSTD(3))" json:"server"`
	// SetCookie 设置Cookie信息，响应头中的Set-Cookie字段
	SetCookie string `gorm:"type:String CODEC(ZSTD(3))" json:"set_cookie"`
	// Location 重定向位置，响应头中的Location字段
	Location string `gorm:"type:String CODEC(ZSTD(3))" json:"location"`
	// CacheControl 缓存控制，响应头中的Cache-Control字段
	CacheControl string `gorm:"type:LowCardinality(String) CODEC(ZSTD(3))" json:"cache_control"`
	// ContentEncoding 内容编码，响应头中的Content-Encoding字段
	ContentEncoding string `gorm:"type:LowCardinality(String) CODEC(ZSTD(3))" json:"content_encoding"`
	// RspContentType 响应内容类型，标识响应主体的格式
	RspContentType string `gorm:"type:LowCardinality(String) CODEC(ZSTD(3))" json:"rsp_content_type"`
	// RspContentLength 响应内容长度，响应主体的大小（字节）
	RspContentLength uint32 `gorm:"type:UInt32 CODEC(ZSTD(3))" json:"rsp_content_length"`
	// RspHeaders 完整的响应头部，以JSON格式存储所有响应头部
	RspHeaders datatypes.JSON `gorm:"type:String CODEC(ZSTD(3))" json:"rsp_headers"`
	// RspBody 响应主体内容，原始字节数据
	RspBody []byte `gorm:"type:String CODEC(ZSTD(3))" json:"rsp_body"`
	// RspTime 响应时间，HTTP响应开始的时间戳
	RspTime time.Time `gorm:"type:DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3))" json:"rsp_time"`
	// RspDuration 响应持续时间，从响应开始到响应完成的时间
	RspDuration time.Duration `gorm:"type:Int64 CODEC(ZSTD(3))" json:"rsp_duration"`
	// IsRspCompleted 响应是否完成，标识HTTP响应是否已完整接收
	IsRspCompleted bool `gorm:"type:Bool" json:"is_rsp_completed"`
}

// HttpTransport 表示HTTP传输层信息
// 完整的HTTP传输数据模型，组合了所有HTTP相关的元数据和内容信息
// 该结构体对应数据库中的http_transport表，用于存储完整的HTTP会话数据
//
// 结构组成:
//   - BaseMeta:    基础元数据，包含命名空间、捕获器、版本和会话ID
//   - FusedMeta:   融合元数据，包含帧堆栈信息
//   - HttpBase:    HTTP基础信息，包含客户端/服务器IP端口和协议版本
//   - HttpURI:     HTTP URI信息，包含URL的各个组成部分
//   - HttpRequest: HTTP请求信息，包含请求方法、头部、主体和时间
//   - HttpResponse:HTTP响应信息，包含响应头部、主体和时间
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
	// HttpRequest HTTP请求详细信息
	HttpRequest
	// HttpResponse HTTP响应详细信息
	HttpResponse
}
