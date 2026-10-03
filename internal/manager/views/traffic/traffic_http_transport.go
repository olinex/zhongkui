package traffic

import (
	"time"

	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/manager/views"
	"zhongkuisec.com/zhongkui/internal/models/network"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/pkg/utils"
)

// HttpTransportListGetListItemOut HTTP 传输列表输出项
// 独立扁平结构体，逐一镜像 network.HttpTransport 的全部顶层 json 字段
// （body 已转存 S3，单条返回字节量可控），BaseTotalOut 承接 COUNT(*) OVER() 的总数
type HttpTransportListGetListItemOut struct {
	utils.BaseTotalOut
	// ID 行全局唯一标识，供行详情接口按 namespace + id 定位
	ID serializer.HexUUID `json:"id"`
	// ---- BaseMeta ----
	// Namespace 命名空间
	Namespace string `json:"namespace"`
	// Capture 抓包器
	Capture string `json:"capture"`
	// Version 抓包器版本
	Version string `json:"version"`
	// SessionID 会话标识
	SessionID serializer.HexUUID `json:"sessionId"`
	// ---- TimestampMeta ----
	// Timestamp 采集时间
	Timestamp time.Time `json:"timestamp"`
	// CreatedAt 入库时间
	CreatedAt time.Time `json:"createdAt"`
	// ---- TraceMeta ----
	// TraceID 跟踪标识，用于串联相关数据
	TraceID serializer.HexUUID `json:"traceId"`
	// ---- HttpBase ----
	// ClientIP 客户端 IP 地址
	ClientIP serializer.IPAddr `json:"clientIp"`
	// ClientPort 客户端端口号
	ClientPort layers.TCPPort `json:"clientPort"`
	// ClientISN 客户端 ISN
	ClientISN uint32 `json:"clientIsn"`
	// ServerIP 服务器 IP 地址
	ServerIP serializer.IPAddr `json:"serverIp"`
	// ServerPort 服务器端口号
	ServerPort layers.TCPPort `json:"serverPort"`
	// ServerISN 服务器 ISN
	ServerISN uint32 `json:"serverIsn"`
	// IPVersion IP 协议版本
	IPVersion layers.IPProtocol `json:"ipVersion"`
	// HttpVersion HTTP 协议版本
	HttpVersion string `json:"httpVersion"`
	// StreamIndex 流索引
	StreamIndex uint32 `json:"streamIndex"`
	// ---- TcpFingerprint ----
	// ClientOS 客户端操作系统
	ClientOS string `json:"fpTcpClientOs"`
	// ClientOSQuality 客户端操作系统识别置信度
	ClientOSQuality float64 `json:"fpTcpClientOsQuality"`
	// ClientSig 客户端 TCP 签名
	ClientSig string `json:"fpTcpClientSig"`
	// ServerOS 服务器操作系统
	ServerOS string `json:"fpTcpServerOs"`
	// ServerOSQuality 服务器操作系统识别置信度
	ServerOSQuality float64 `json:"fpTcpServerOsQuality"`
	// ServerSig 服务器 TCP 签名
	ServerSig string `json:"fpTcpServerSig"`
	// MTU 最大传输单元
	MTU uint16 `json:"fpTcpMtu"`
	// ---- HttpURI ----
	// URL 完整请求 URL
	URL string `json:"url"`
	// Scheme 协议方案
	Scheme string `json:"scheme"`
	// Host 主机名或 IP 地址
	Host string `json:"host"`
	// Path 请求路径
	Path string `json:"path"`
	// FileExt 文件扩展名
	FileExt string `json:"fileExt"`
	// Queries 查询参数（JSON 格式）
	Queries datatypes.JSON `json:"queries"`
	// Fragment 片段标识符
	Fragment string `json:"fragment"`
	// ---- HttpRequestBody ----
	// Method HTTP 请求方法
	Method string `json:"method"`
	// ReqBody 请求主体（S3 对象键，空字符串表示内容为空）
	ReqBody string `json:"reqBody"`
	// ReqBodyHash 请求主体 HMAC-SHA256 摘要（hex，带密钥防离线爆破）
	ReqBodyHash string `json:"reqBodyHash"`
	// ReqBodyLength 请求主体字节长度
	ReqBodyLength int64 `json:"reqBodyLength"`
	// ---- HttpRequestHeaders ----
	// ReqHdrAuthorization 认证信息（加密存储）
	ReqHdrAuthorization *serializer.Crypto `json:"reqHdrAuthorization" form:"kind:crypto"`
	// ReqHdrProxyAuthorization 代理认证信息（加密存储）
	ReqHdrProxyAuthorization *serializer.Crypto `json:"reqHdrProxyAuthorization" form:"kind:crypto"`
	// ReqHdrCacheControl 缓存控制指令
	ReqHdrCacheControl string `json:"reqHdrCacheControl"`
	// ReqHdrIfMatch 条件匹配
	ReqHdrIfMatch string `json:"reqHdrIfMatch"`
	// ReqHdrIfNoneMatch 条件不匹配
	ReqHdrIfNoneMatch string `json:"reqHdrIfNoneMatch"`
	// ReqHdrIfModifiedSince 条件修改时间
	ReqHdrIfModifiedSince string `json:"reqHdrIfModifiedSince"`
	// ReqHdrIfUnmodifiedSince 条件未修改时间
	ReqHdrIfUnmodifiedSince string `json:"reqHdrIfUnmodifiedSince"`
	// ReqHdrIfRange 条件范围请求
	ReqHdrIfRange string `json:"reqHdrIfRange"`
	// ReqHdrConnection 连接控制
	ReqHdrConnection string `json:"reqHdrConnection"`
	// ReqHdrKeepAlive 持久连接控制
	ReqHdrKeepAlive string `json:"reqHdrKeepAlive"`
	// ReqHdrAccept 可接受的响应内容类型
	ReqHdrAccept string `json:"reqHdrAccept"`
	// ReqHdrAcceptEncoding 可接受的内容编码算法
	ReqHdrAcceptEncoding string `json:"reqHdrAcceptEncoding"`
	// ReqHdrAcceptLanguage 可接受的自然语言
	ReqHdrAcceptLanguage string `json:"reqHdrAcceptLanguage"`
	// ReqHdrExpect 期望条件
	ReqHdrExpect string `json:"reqHdrExpect"`
	// ReqHdrMaxForwards 最大转发跳数
	ReqHdrMaxForwards int32 `json:"reqHdrMaxForwards"`
	// ReqHdrCookie 客户端 Cookie（加密存储）
	ReqHdrCookie *serializer.Crypto `json:"reqHdrCookie" form:"kind:crypto"`
	// ReqHdrOrigin 请求源
	ReqHdrOrigin string `json:"reqHdrOrigin"`
	// ReqHdrAccessControlRequestHeaders CORS 预检请求头
	ReqHdrAccessControlRequestHeaders string `json:"reqHdrAccessControlRequestHeaders"`
	// ReqHdrAccessControlRequestMethod CORS 预检方法
	ReqHdrAccessControlRequestMethod string `json:"reqHdrAccessControlRequestMethod"`
	// ReqHdrContentType 请求内容类型
	ReqHdrContentType string `json:"reqHdrContentType"`
	// ReqHdrContentLength 请求内容长度
	ReqHdrContentLength int64 `json:"reqHdrContentLength"`
	// ReqHdrContentEncoding 请求内容编码
	ReqHdrContentEncoding string `json:"reqHdrContentEncoding"`
	// ReqHdrContentLanguage 请求内容语言
	ReqHdrContentLanguage string `json:"reqHdrContentLanguage"`
	// ReqHdrForwarded 转发信息
	ReqHdrForwarded string `json:"reqHdrForwarded"`
	// ReqHdrVia 代理链信息
	ReqHdrVia string `json:"reqHdrVia"`
	// ReqHdrRange 范围请求
	ReqHdrRange string `json:"reqHdrRange"`
	// ReqHdrFrom 发送者邮箱
	ReqHdrFrom string `json:"reqHdrFrom"`
	// ReqHdrHost 目标主机
	ReqHdrHost string `json:"reqHdrHost"`
	// ReqHdrReferer 引用来源
	ReqHdrReferer string `json:"reqHdrReferer"`
	// ReqHdrReferrerPolicy 引用策略
	ReqHdrReferrerPolicy string `json:"reqHdrReferrerPolicy"`
	// ReqHdrUserAgent 用户代理字符串
	ReqHdrUserAgent string `json:"reqHdrUserAgent"`
	// ReqHdrTE 传输编码接受
	ReqHdrTE string `json:"reqHdrTe"`
	// ReqHdrTransferEncoding 传输编码
	ReqHdrTransferEncoding string `json:"reqHdrTransferEncoding"`
	// ReqHdrTrailer 尾部字段声明
	ReqHdrTrailer string `json:"reqHdrTrailer"`
	// ReqHdrSecWebSocketKey WebSocket 握手密钥（加密存储）
	ReqHdrSecWebSocketKey *serializer.Crypto `json:"reqHdrSecWebsocketKey" form:"kind:crypto"`
	// ReqHdrSecWebSocketVersion WebSocket 协议版本
	ReqHdrSecWebSocketVersion string `json:"reqHdrSecWebsocketVersion"`
	// ReqHdrSecWebSocketProtocol WebSocket 子协议
	ReqHdrSecWebSocketProtocol string `json:"reqHdrSecWebsocketProtocol"`
	// ReqHdrSecWebSocketExtensions WebSocket 扩展
	ReqHdrSecWebSocketExtensions string `json:"reqHdrSecWebsocketExtensions"`
	// ReqHdrSecFetchSite Fetch 元数据——源关系
	ReqHdrSecFetchSite string `json:"reqHdrSecFetchSite"`
	// ReqHdrSecFetchMode Fetch 元数据——请求模式
	ReqHdrSecFetchMode string `json:"reqHdrSecFetchMode"`
	// ReqHdrSecFetchUser Fetch 元数据——用户激活
	ReqHdrSecFetchUser string `json:"reqHdrSecFetchUser"`
	// ReqHdrSecFetchDest Fetch 元数据——请求目标
	ReqHdrSecFetchDest string `json:"reqHdrSecFetchDest"`
	// ReqHdrSecPurpose 请求目的
	ReqHdrSecPurpose string `json:"reqHdrSecPurpose"`
	// ReqHdrServiceWorkerNavigationPreload Service Worker 导航预加载
	ReqHdrServiceWorkerNavigationPreload string `json:"reqHdrServiceWorkerNavigationPreload"`
	// ReqHdrSecCHUA 用户代理品牌和版本信息
	ReqHdrSecCHUA string `json:"reqHdrSecChUa"`
	// ReqHdrSecCHUAArch 用户代理 CPU 架构
	ReqHdrSecCHUAArch string `json:"reqHdrSecChUaArch"`
	// ReqHdrSecCHUABitness 用户代理 CPU 位数
	ReqHdrSecCHUABitness string `json:"reqHdrSecChUaBitness"`
	// ReqHdrSecCHUAFullVersion 用户代理完整版本
	ReqHdrSecCHUAFullVersion string `json:"reqHdrSecChUaFullVersion"`
	// ReqHdrSecCHUAFullVersionList 品牌完整版本列表
	ReqHdrSecCHUAFullVersionList string `json:"reqHdrSecChUaFullVersionList"`
	// ReqHdrSecCHUAMobile 是否移动设备
	ReqHdrSecCHUAMobile string `json:"reqHdrSecChUaMobile"`
	// ReqHdrSecCHUAModel 设备型号
	ReqHdrSecCHUAModel string `json:"reqHdrSecChUaModel"`
	// ReqHdrSecCHUAPlatform 操作系统/平台
	ReqHdrSecCHUAPlatform string `json:"reqHdrSecChUaPlatform"`
	// ReqHdrSecCHUAPlatformVersion 操作系统版本号
	ReqHdrSecCHUAPlatformVersion string `json:"reqHdrSecChUaPlatformVersion"`
	// ReqHdrSecCHUAWoW64 是否在 64 位 Windows 上以 32 位模式运行
	ReqHdrSecCHUAWoW64 string `json:"reqHdrSecChUaWow64"`
	// ReqHdrSecCHPrefersColorScheme 深色/浅色模式偏好
	ReqHdrSecCHPrefersColorScheme string `json:"reqHdrSecChPrefersColorScheme"`
	// ReqHdrSecCHPrefersReducedMotion 减少动画偏好
	ReqHdrSecCHPrefersReducedMotion string `json:"reqHdrSecChPrefersReducedMotion"`
	// ReqHdrSecCHPrefersReducedTransparency 降低透明度偏好
	ReqHdrSecCHPrefersReducedTransparency string `json:"reqHdrSecChPrefersReducedTransparency"`
	// ReqHdrWantContentDigest 期望内容摘要
	ReqHdrWantContentDigest string `json:"reqHdrWantContentDigest"`
	// ReqHdrWantReprDigest 期望表示摘要
	ReqHdrWantReprDigest string `json:"reqHdrWantReprDigest"`
	// ReqHdrUpgradeInsecureRequests 升级不安全请求
	ReqHdrUpgradeInsecureRequests string `json:"reqHdrUpgradeInsecureRequests"`
	// ReqHdrDate 消息创建日期和时间
	ReqHdrDate string `json:"reqHdrDate"`
	// ReqHdrLink 链接关系
	ReqHdrLink string `json:"reqHdrLink"`
	// ReqHdrUpgrade 协议升级
	ReqHdrUpgrade string `json:"reqHdrUpgrade"`
	// ReqHdrPriority 资源优先级
	ReqHdrPriority string `json:"reqHdrPriority"`
	// ReqHdrAltUsed 使用的替代服务
	ReqHdrAltUsed string `json:"reqHdrAltUsed"`
	// ---- HttpResponseBody ----
	// RspBody 响应主体（S3 对象键，空字符串表示内容为空）
	RspBody string `json:"rspBody"`
	// RspBodyHash 响应主体 HMAC-SHA256 摘要（hex，带密钥防离线爆破）
	RspBodyHash string `json:"rspBodyHash"`
	// RspBodyLength 响应主体字节长度
	RspBodyLength int64 `json:"rspBodyLength"`
	// ---- HttpResponseHeaders ----
	// RspHdrWWWAuthenticate 认证质询（加密存储）
	RspHdrWWWAuthenticate *serializer.Crypto `json:"rspHdrWwwAuthenticate" form:"kind:crypto"`
	// RspHdrProxyAuthenticate 代理认证质询（加密存储）
	RspHdrProxyAuthenticate *serializer.Crypto `json:"rspHdrProxyAuthenticate" form:"kind:crypto"`
	// RspHdrAge 缓存年龄（秒）
	RspHdrAge int64 `json:"rspHdrAge"`
	// RspHdrCacheControl 缓存控制指令
	RspHdrCacheControl string `json:"rspHdrCacheControl"`
	// RspHdrClearSiteData 清除站点数据
	RspHdrClearSiteData string `json:"rspHdrClearSiteData"`
	// RspHdrExpires 过期时间
	RspHdrExpires string `json:"rspHdrExpires"`
	// RspHdrNoVarySearch 无变化搜索
	RspHdrNoVarySearch string `json:"rspHdrNoVarySearch"`
	// RspHdrLastModified 最后修改时间
	RspHdrLastModified string `json:"rspHdrLastModified"`
	// RspHdrETag 实体标签
	RspHdrETag string `json:"rspHdrEtag"`
	// RspHdrVary 响应变化依据
	RspHdrVary string `json:"rspHdrVary"`
	// RspHdrConnection 连接控制
	RspHdrConnection string `json:"rspHdrConnection"`
	// RspHdrKeepAlive 持久连接控制
	RspHdrKeepAlive string `json:"rspHdrKeepAlive"`
	// RspHdrAcceptPatch 可接受的 PATCH 媒体类型
	RspHdrAcceptPatch string `json:"rspHdrAcceptPatch"`
	// RspHdrAcceptPost 可接受的 POST 媒体类型
	RspHdrAcceptPost string `json:"rspHdrAcceptPost"`
	// RspHdrSetCookie 设置 Cookie（加密存储）
	RspHdrSetCookie *serializer.Crypto `json:"rspHdrSetCookie" form:"kind:crypto"`
	// RspHdrAccessControlAllowCredentials CORS 允许凭据
	RspHdrAccessControlAllowCredentials string `json:"rspHdrAccessControlAllowCredentials"`
	// RspHdrAccessControlAllowHeaders CORS 允许标头
	RspHdrAccessControlAllowHeaders string `json:"rspHdrAccessControlAllowHeaders"`
	// RspHdrAccessControlAllowMethods CORS 允许方法
	RspHdrAccessControlAllowMethods string `json:"rspHdrAccessControlAllowMethods"`
	// RspHdrAccessControlAllowOrigin CORS 允许源
	RspHdrAccessControlAllowOrigin string `json:"rspHdrAccessControlAllowOrigin"`
	// RspHdrAccessControlExposeHeaders CORS 暴露标头
	RspHdrAccessControlExposeHeaders string `json:"rspHdrAccessControlExposeHeaders"`
	// RspHdrAccessControlMaxAge CORS 最大缓存时间（秒）
	RspHdrAccessControlMaxAge int64 `json:"rspHdrAccessControlMaxAge"`
	// RspHdrTimingAllowOrigin 时间允许源
	RspHdrTimingAllowOrigin string `json:"rspHdrTimingAllowOrigin"`
	// RspHdrContentDisposition 内容处置
	RspHdrContentDisposition string `json:"rspHdrContentDisposition"`
	// RspHdrContentDigest 内容摘要
	RspHdrContentDigest string `json:"rspHdrContentDigest"`
	// RspHdrReprDigest 表示摘要
	RspHdrReprDigest string `json:"rspHdrReprDigest"`
	// RspHdrContentType 响应内容类型
	RspHdrContentType string `json:"rspHdrContentType"`
	// RspHdrContentLength 响应内容长度
	RspHdrContentLength int64 `json:"rspHdrContentLength"`
	// RspHdrContentEncoding 响应内容编码
	RspHdrContentEncoding string `json:"rspHdrContentEncoding"`
	// RspHdrContentLanguage 响应内容语言
	RspHdrContentLanguage string `json:"rspHdrContentLanguage"`
	// RspHdrContentLocation 内容位置
	RspHdrContentLocation string `json:"rspHdrContentLocation"`
	// RspHdrForwarded 转发信息
	RspHdrForwarded string `json:"rspHdrForwarded"`
	// RspHdrVia 代理链信息
	RspHdrVia string `json:"rspHdrVia"`
	// RspHdrAcceptRanges 接受范围请求
	RspHdrAcceptRanges string `json:"rspHdrAcceptRanges"`
	// RspHdrContentRange 内容范围
	RspHdrContentRange string `json:"rspHdrContentRange"`
	// RspHdrLocation 重定向位置
	RspHdrLocation string `json:"rspHdrLocation"`
	// RspHdrRefresh 刷新/重定向
	RspHdrRefresh string `json:"rspHdrRefresh"`
	// RspHdrAllow 允许的方法
	RspHdrAllow string `json:"rspHdrAllow"`
	// RspHdrServer 服务器标识
	RspHdrServer string `json:"rspHdrServer"`
	// RspHdrCrossOriginEmbedderPolicy 跨源嵌入器策略
	RspHdrCrossOriginEmbedderPolicy string `json:"rspHdrCrossOriginEmbedderPolicy"`
	// RspHdrCrossOriginOpenerPolicy 跨源打开器策略
	RspHdrCrossOriginOpenerPolicy string `json:"rspHdrCrossOriginOpenerPolicy"`
	// RspHdrCrossOriginResourcePolicy 跨源资源策略
	RspHdrCrossOriginResourcePolicy string `json:"rspHdrCrossOriginResourcePolicy"`
	// RspHdrContentSecurityPolicy 内容安全策略
	RspHdrContentSecurityPolicy string `json:"rspHdrContentSecurityPolicy"`
	// RspHdrContentSecurityPolicyReportOnly CSP 仅报告模式
	RspHdrContentSecurityPolicyReportOnly string `json:"rspHdrContentSecurityPolicyReportOnly"`
	// RspHdrExpectCT 证书透明度期望
	RspHdrExpectCT string `json:"rspHdrExpectCt"`
	// RspHdrPermissionsPolicy 权限策略
	RspHdrPermissionsPolicy string `json:"rspHdrPermissionsPolicy"`
	// RspHdrStrictTransportSecurity 严格传输安全
	RspHdrStrictTransportSecurity string `json:"rspHdrStrictTransportSecurity"`
	// RspHdrXContentTypeOptions 内容类型选项
	RspHdrXContentTypeOptions string `json:"rspHdrXContentTypeOptions"`
	// RspHdrXFrameOptions 框架选项
	RspHdrXFrameOptions string `json:"rspHdrXFrameOptions"`
	// RspHdrXPermittedCrossDomainPolicies 允许的跨域策略
	RspHdrXPermittedCrossDomainPolicies string `json:"rspHdrXPermittedCrossDomainPolicies"`
	// RspHdrXPoweredBy 技术引擎标识
	RspHdrXPoweredBy string `json:"rspHdrXPoweredBy"`
	// RspHdrXXSSProtection XSS 保护
	RspHdrXXSSProtection string `json:"rspHdrXXssProtection"`
	// RspHdrReportingEndpoints 报告端点
	RspHdrReportingEndpoints string `json:"rspHdrReportingEndpoints"`
	// RspHdrReportTo 报告目标
	RspHdrReportTo string `json:"rspHdrReportTo"`
	// RspHdrTransferEncoding 传输编码
	RspHdrTransferEncoding string `json:"rspHdrTransferEncoding"`
	// RspHdrTrailer 尾部字段声明
	RspHdrTrailer string `json:"rspHdrTrailer"`
	// RspHdrSecWebSocketAccept WebSocket 握手接受（加密存储）
	RspHdrSecWebSocketAccept *serializer.Crypto `json:"rspHdrSecWebsocketAccept" form:"kind:crypto"`
	// RspHdrSecWebSocketProtocol WebSocket 子协议
	RspHdrSecWebSocketProtocol string `json:"rspHdrSecWebsocketProtocol"`
	// RspHdrSecWebSocketExtensions WebSocket 扩展
	RspHdrSecWebSocketExtensions string `json:"rspHdrSecWebsocketExtensions"`
	// RspHdrAcceptCH 接受客户端提示
	RspHdrAcceptCH string `json:"rspHdrAcceptCh"`
	// RspHdrCriticalCH 关键客户端提示
	RspHdrCriticalCH string `json:"rspHdrCriticalCh"`
	// RspHdrAttributionReportingEligible 归因报告资格
	RspHdrAttributionReportingEligible string `json:"rspHdrAttributionReportingEligible"`
	// RspHdrAttributionReportingRegisterSource 归因报告注册来源
	RspHdrAttributionReportingRegisterSource string `json:"rspHdrAttributionReportingRegisterSource"`
	// RspHdrAttributionReportingRegisterTrigger 归因报告注册触发器
	RspHdrAttributionReportingRegisterTrigger string `json:"rspHdrAttributionReportingRegisterTrigger"`
	// RspHdrDate 消息创建日期和时间
	RspHdrDate string `json:"rspHdrDate"`
	// RspHdrLink 链接关系
	RspHdrLink string `json:"rspHdrLink"`
	// RspHdrRetryAfter 重试时间
	RspHdrRetryAfter string `json:"rspHdrRetryAfter"`
	// RspHdrServerTiming 服务器时间
	RspHdrServerTiming string `json:"rspHdrServerTiming"`
	// RspHdrServiceWorker Service Worker 标识
	RspHdrServiceWorker string `json:"rspHdrServiceWorker"`
	// RspHdrServiceWorkerAllowed Service Worker 允许路径
	RspHdrServiceWorkerAllowed string `json:"rspHdrServiceWorkerAllowed"`
	// RspHdrSourceMap 源代码映射
	RspHdrSourceMap string `json:"rspHdrSourceMap"`
	// RspHdrUpgrade 协议升级
	RspHdrUpgrade string `json:"rspHdrUpgrade"`
	// RspHdrAltSvc 替代服务
	RspHdrAltSvc string `json:"rspHdrAltSvc"`
	// RspHdrPriority 资源优先级
	RspHdrPriority string `json:"rspHdrPriority"`
	// ---- HttpFingerprint ----
	// Browser 浏览器指纹
	Browser string `json:"fpHttpBrowser"`
	// BrowserQuality 浏览器指纹识别置信度
	BrowserQuality float64 `json:"fpHttpBrowserQuality"`
	// Server 服务端指纹
	Server string `json:"fpHttpServer"`
	// ServerQuality 服务端指纹识别置信度
	ServerQuality float64 `json:"fpHttpServerQuality"`
	// Language 语言指纹
	Language string `json:"fpHttpLanguage"`
	// DeviceGuess 设备类别猜测
	DeviceGuess string `json:"fpDeviceGuess"`
	// ---- AppTailer ----
	// ClientConsumed 客户端方向已消费字节数
	ClientConsumed uint64 `json:"clientConsumed"`
	// ClientStartPayloadOffset 客户端方向载荷起始偏移（字节）
	ClientStartPayloadOffset uint64 `json:"clientStartPayloadOffset"`
	// ClientEndPayloadOffset 客户端方向载荷结束偏移（字节，独占）
	ClientEndPayloadOffset uint64 `json:"clientEndPayloadOffset"`
	// ClientStartTimestamp 客户端方向起始时间戳
	ClientStartTimestamp time.Time `json:"clientStartTimestamp"`
	// ClientEndTimestamp 客户端方向结束时间戳
	ClientEndTimestamp time.Time `json:"clientEndTimestamp"`
	// ServerConsumed 服务端方向已消费字节数
	ServerConsumed uint64 `json:"serverConsumed"`
	// ServerStartPayloadOffset 服务端方向载荷起始偏移（字节）
	ServerStartPayloadOffset uint64 `json:"serverStartPayloadOffset"`
	// ServerEndPayloadOffset 服务端方向载荷结束偏移（字节，独占）
	ServerEndPayloadOffset uint64 `json:"serverEndPayloadOffset"`
	// ServerStartTimestamp 服务端方向起始时间戳
	ServerStartTimestamp time.Time `json:"serverStartTimestamp"`
	// ServerEndTimestamp 服务端方向结束时间戳
	ServerEndTimestamp time.Time `json:"serverEndTimestamp"`
	// ---- TagsMeta ----
	// Tags 风险标签组合
	Tags []string `json:"tags"`
}

// NetworkStackListGetListItemOut 网络堆栈列表输出项
// 独立扁平结构体，逐一镜像 network.NetworkStack 的全部顶层 json 字段
// （含载荷字节偏移），BaseTotalOut 承接 COUNT(*) OVER() 的总数
type NetworkStackListGetListItemOut struct {
	utils.BaseTotalOut
	// ---- BaseMeta ----
	// Namespace 命名空间
	Namespace string `json:"namespace"`
	// Capture 抓包器
	Capture string `json:"capture"`
	// Version 抓包器版本
	Version string `json:"version"`
	// SessionID 会话标识
	SessionID serializer.HexUUID `json:"sessionId"`
	// ---- TraceMeta ----
	// TraceID 跟踪标识，用于串联相关数据
	TraceID serializer.HexUUID `json:"traceId"`
	// ---- TimestampMeta ----
	// Timestamp 采集时间
	Timestamp time.Time `json:"timestamp"`
	// CreatedAt 入库时间
	CreatedAt time.Time `json:"createdAt"`
	// ---- DirectionMeta ----
	// Cli2Srv 是否客户端到服务端方向
	Cli2Srv bool `json:"cli2Srv"`
	// ---- 帧信息 ----
	// TopProto 顶层协议类型（如 Ethernet、IPv4、TCP 等）
	TopProto gopacket.LayerType `json:"topProto"`
	// Proto 当前协议类型（如 Ethernet、IPv4、TCP 等）
	Proto gopacket.LayerType `json:"proto"`
	// Seq 帧序号，保持帧在 FrameStack 中的顺序
	Seq uint32 `json:"seq"`
	// Level 协议层级（如链路层、网络层等）
	Level uint8 `json:"level"`
	// Headers 协议头部信息，具体类型由 Proto 决定
	Headers string `json:"headers"`
	// PayloadStartOffset 该帧所在 TCP 包的载荷起始偏移（字节）
	PayloadStartOffset uint64 `json:"payloadStartOffset"`
	// PayloadEndOffset 该帧所在 TCP 包的载荷结束偏移（字节，独占）
	PayloadEndOffset uint64 `json:"payloadEndOffset"`
}

// HttpTransportListGet 处理 HTTP 传输列表查询请求
// 通过 HttpTransportListGetSearchIn 应用过滤、排序与分页，查询 raw 库（ClickHouse）中的传输数据，
// 以局部扫描容器承载 gorm 模型与 total 列，再映射为独立 Out 结构（含全部字段）
func HttpTransportListGet(c *gin.Context) {
	// 初始化搜索输入参数，设置默认值
	var in views.HttpTransportListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 绑定并验证 JSON 请求体
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}

	// 获取 raw 库（ClickHouse）连接并指定查询模型
	db := middlewares.GetRawDatabase(c).Model(&network.HttpTransport{})

	// 先以轻量 count 统计总数：ClickHouse 只扫过滤列与 granule 标记，
	// 避免 COUNT(*) OVER() 窗口函数在 LIMIT 前物化全部命中行的所有列（body/headers 等宽列）。
	var total int64
	if countQuery, err := utils.BuildSearchCondQuery(db, &in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := countQuery.Count(&total).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 再查询当前页数据：ORDER BY timestamp 命中 MergeTree 排序键，可倒序读 + LIMIT 早停，
	// 仅物化当前页行；扫描容器与模型同型，避免 GORM 按目的地枚举额外列（如 total）。
	var rows []network.HttpTransport
	if dataQuery, err := utils.BuildSearchDataQuery(db, &in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := dataQuery.Find(&rows).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 字段投影：按请求 fields 投影，未指定时回退全部白名单字段（封装于 utils.ProjectSearchItems）
	projected := utils.ProjectSearchItems(rows, &in, func(o *network.HttpTransport) any {
		return NewHttpTransportListGetListItemOut(o)
	})
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(uint(total), projected))
}

// HttpTransportStackListGet 处理 HTTP 传输关联网络堆栈列表查询请求
// 以 HttpTransport 携带的 traceId（+ namespace/sessionId/时间范围）为键，
// 查询 raw 库（ClickHouse）中该 TCP 流的全部 NetworkStack 记录；
// 前端借助 HttpTransport.AppTailer 的 payload offset 与记录的
// payloadStartOffset/payloadEndOffset 相交，定位请求/响应各自的数据包。
func HttpTransportStackListGet(c *gin.Context) {
	// 初始化搜索输入参数，设置默认值
	var in views.NetworkStackListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 绑定并验证 JSON 请求体
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}

	// 获取 raw 库（ClickHouse）连接并指定查询模型
	db := middlewares.GetRawDatabase(c).Model(&network.NetworkStack{})

	// 先以轻量 count 统计总数：ClickHouse 只扫过滤列与 granule 标记，
	// 避免 COUNT(*) OVER() 窗口函数在 LIMIT 前物化全部命中行的所有列（payload 等宽列）。
	var total int64
	if countQuery, err := utils.BuildSearchCondQuery(db, &in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := countQuery.Count(&total).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 再查询当前页数据：ORDER BY timestamp 命中 MergeTree 排序键，可倒序读 + LIMIT 早停，
	// 仅物化当前页行；扫描容器与模型同型，避免 GORM 按目的地枚举额外列（如 total）。
	var rows []network.NetworkStack
	if dataQuery, err := utils.BuildSearchDataQuery(db, &in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := dataQuery.Find(&rows).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}

	// 字段投影：按请求 fields 投影，未指定时回退全部白名单字段（封装于 utils.ProjectSearchItems）
	projected := utils.ProjectSearchItems(rows, &in, func(o *network.NetworkStack) any {
		return newNetworkStackListGetListItemOut(o)
	})
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(uint(total), projected))
}

// NewHttpTransportListGetListItemOut 将 network.HttpTransport 映射为独立 Out 结构。
// 模型字段均为共享/第三方类型，逐字段直接复制，无类型转换。
func NewHttpTransportListGetListItemOut(p *network.HttpTransport) *HttpTransportListGetListItemOut {
	return &HttpTransportListGetListItemOut{
		// RowMeta
		ID: p.ID,
		// BaseMeta
		Namespace: p.Namespace,
		Capture:   p.Capture,
		Version:   p.Version,
		SessionID: p.SessionID,
		// TimestampMeta
		Timestamp: p.Timestamp,
		CreatedAt: p.CreatedAt,
		// TraceMeta
		TraceID: p.TraceID,
		// HttpBase
		ClientIP:    p.ClientIP,
		ClientPort:  p.ClientPort,
		ClientISN:   p.ClientISN,
		ServerIP:    p.ServerIP,
		ServerPort:  p.ServerPort,
		ServerISN:   p.ServerISN,
		IPVersion:   p.IPVersion,
		HttpVersion: p.HttpVersion,
		StreamIndex: p.StreamIndex,
		// TcpFingerprint
		ClientOS:        p.ClientOS,
		ClientOSQuality: p.ClientOSQuality,
		ClientSig:       p.ClientSig,
		ServerOS:        p.ServerOS,
		ServerOSQuality: p.ServerOSQuality,
		ServerSig:       p.ServerSig,
		MTU:             p.MTU,
		// HttpURI
		URL:      p.URL,
		Scheme:   p.Scheme,
		Host:     p.Host,
		Path:     p.Path,
		FileExt:  p.FileExt,
		Queries:  p.Queries,
		Fragment: p.Fragment,
		// HttpRequestBody
		Method:  p.Method,
		ReqBody: p.ReqBody,
		// ReqBodyHash 请求主体 HMAC-SHA256 摘要（hex，带密钥防离线爆破）
		ReqBodyHash:   p.ReqBodyHash.Digest(),
		ReqBodyLength: p.ReqBodyLength,
		// HttpRequestHeaders
		ReqHdrAuthorization:                   p.ReqHdrAuthorization,
		ReqHdrProxyAuthorization:              p.ReqHdrProxyAuthorization,
		ReqHdrCacheControl:                    p.ReqHdrCacheControl,
		ReqHdrIfMatch:                         p.ReqHdrIfMatch,
		ReqHdrIfNoneMatch:                     p.ReqHdrIfNoneMatch,
		ReqHdrIfModifiedSince:                 p.ReqHdrIfModifiedSince,
		ReqHdrIfUnmodifiedSince:               p.ReqHdrIfUnmodifiedSince,
		ReqHdrIfRange:                         p.ReqHdrIfRange,
		ReqHdrConnection:                      p.ReqHdrConnection,
		ReqHdrKeepAlive:                       p.ReqHdrKeepAlive,
		ReqHdrAccept:                          p.ReqHdrAccept,
		ReqHdrAcceptEncoding:                  p.ReqHdrAcceptEncoding,
		ReqHdrAcceptLanguage:                  p.ReqHdrAcceptLanguage,
		ReqHdrExpect:                          p.ReqHdrExpect,
		ReqHdrMaxForwards:                     p.ReqHdrMaxForwards,
		ReqHdrCookie:                          p.ReqHdrCookie,
		ReqHdrOrigin:                          p.ReqHdrOrigin,
		ReqHdrAccessControlRequestHeaders:     p.ReqHdrAccessControlRequestHeaders,
		ReqHdrAccessControlRequestMethod:      p.ReqHdrAccessControlRequestMethod,
		ReqHdrContentType:                     p.ReqHdrContentType,
		ReqHdrContentLength:                   p.ReqHdrContentLength,
		ReqHdrContentEncoding:                 p.ReqHdrContentEncoding,
		ReqHdrContentLanguage:                 p.ReqHdrContentLanguage,
		ReqHdrForwarded:                       p.ReqHdrForwarded,
		ReqHdrVia:                             p.ReqHdrVia,
		ReqHdrRange:                           p.ReqHdrRange,
		ReqHdrFrom:                            p.ReqHdrFrom,
		ReqHdrHost:                            p.ReqHdrHost,
		ReqHdrReferer:                         p.ReqHdrReferer,
		ReqHdrReferrerPolicy:                  p.ReqHdrReferrerPolicy,
		ReqHdrUserAgent:                       p.ReqHdrUserAgent,
		ReqHdrTE:                              p.ReqHdrTE,
		ReqHdrTransferEncoding:                p.ReqHdrTransferEncoding,
		ReqHdrTrailer:                         p.ReqHdrTrailer,
		ReqHdrSecWebSocketKey:                 p.ReqHdrSecWebSocketKey,
		ReqHdrSecWebSocketVersion:             p.ReqHdrSecWebSocketVersion,
		ReqHdrSecWebSocketProtocol:            p.ReqHdrSecWebSocketProtocol,
		ReqHdrSecWebSocketExtensions:          p.ReqHdrSecWebSocketExtensions,
		ReqHdrSecFetchSite:                    p.ReqHdrSecFetchSite,
		ReqHdrSecFetchMode:                    p.ReqHdrSecFetchMode,
		ReqHdrSecFetchUser:                    p.ReqHdrSecFetchUser,
		ReqHdrSecFetchDest:                    p.ReqHdrSecFetchDest,
		ReqHdrSecPurpose:                      p.ReqHdrSecPurpose,
		ReqHdrServiceWorkerNavigationPreload:  p.ReqHdrServiceWorkerNavigationPreload,
		ReqHdrSecCHUA:                         p.ReqHdrSecCHUA,
		ReqHdrSecCHUAArch:                     p.ReqHdrSecCHUAArch,
		ReqHdrSecCHUABitness:                  p.ReqHdrSecCHUABitness,
		ReqHdrSecCHUAFullVersion:              p.ReqHdrSecCHUAFullVersion,
		ReqHdrSecCHUAFullVersionList:          p.ReqHdrSecCHUAFullVersionList,
		ReqHdrSecCHUAMobile:                   p.ReqHdrSecCHUAMobile,
		ReqHdrSecCHUAModel:                    p.ReqHdrSecCHUAModel,
		ReqHdrSecCHUAPlatform:                 p.ReqHdrSecCHUAPlatform,
		ReqHdrSecCHUAPlatformVersion:          p.ReqHdrSecCHUAPlatformVersion,
		ReqHdrSecCHUAWoW64:                    p.ReqHdrSecCHUAWoW64,
		ReqHdrSecCHPrefersColorScheme:         p.ReqHdrSecCHPrefersColorScheme,
		ReqHdrSecCHPrefersReducedMotion:       p.ReqHdrSecCHPrefersReducedMotion,
		ReqHdrSecCHPrefersReducedTransparency: p.ReqHdrSecCHPrefersReducedTransparency,
		ReqHdrWantContentDigest:               p.ReqHdrWantContentDigest,
		ReqHdrWantReprDigest:                  p.ReqHdrWantReprDigest,
		ReqHdrUpgradeInsecureRequests:         p.ReqHdrUpgradeInsecureRequests,
		ReqHdrDate:                            p.ReqHdrDate,
		ReqHdrLink:                            p.ReqHdrLink,
		ReqHdrUpgrade:                         p.ReqHdrUpgrade,
		ReqHdrPriority:                        p.ReqHdrPriority,
		ReqHdrAltUsed:                         p.ReqHdrAltUsed,
		// HttpResponseBody
		RspBody: p.RspBody,
		// RspBodyHash 响应主体 HMAC-SHA256 摘要（hex，带密钥防离线爆破）
		RspBodyHash:   p.RspBodyHash.Digest(),
		RspBodyLength: p.RspBodyLength,
		// HttpResponseHeaders
		RspHdrWWWAuthenticate:                     p.RspHdrWWWAuthenticate,
		RspHdrProxyAuthenticate:                   p.RspHdrProxyAuthenticate,
		RspHdrAge:                                 p.RspHdrAge,
		RspHdrCacheControl:                        p.RspHdrCacheControl,
		RspHdrClearSiteData:                       p.RspHdrClearSiteData,
		RspHdrExpires:                             p.RspHdrExpires,
		RspHdrNoVarySearch:                        p.RspHdrNoVarySearch,
		RspHdrLastModified:                        p.RspHdrLastModified,
		RspHdrETag:                                p.RspHdrETag,
		RspHdrVary:                                p.RspHdrVary,
		RspHdrConnection:                          p.RspHdrConnection,
		RspHdrKeepAlive:                           p.RspHdrKeepAlive,
		RspHdrAcceptPatch:                         p.RspHdrAcceptPatch,
		RspHdrAcceptPost:                          p.RspHdrAcceptPost,
		RspHdrSetCookie:                           p.RspHdrSetCookie,
		RspHdrAccessControlAllowCredentials:       p.RspHdrAccessControlAllowCredentials,
		RspHdrAccessControlAllowHeaders:           p.RspHdrAccessControlAllowHeaders,
		RspHdrAccessControlAllowMethods:           p.RspHdrAccessControlAllowMethods,
		RspHdrAccessControlAllowOrigin:            p.RspHdrAccessControlAllowOrigin,
		RspHdrAccessControlExposeHeaders:          p.RspHdrAccessControlExposeHeaders,
		RspHdrAccessControlMaxAge:                 p.RspHdrAccessControlMaxAge,
		RspHdrTimingAllowOrigin:                   p.RspHdrTimingAllowOrigin,
		RspHdrContentDisposition:                  p.RspHdrContentDisposition,
		RspHdrContentDigest:                       p.RspHdrContentDigest,
		RspHdrReprDigest:                          p.RspHdrReprDigest,
		RspHdrContentType:                         p.RspHdrContentType,
		RspHdrContentLength:                       p.RspHdrContentLength,
		RspHdrContentEncoding:                     p.RspHdrContentEncoding,
		RspHdrContentLanguage:                     p.RspHdrContentLanguage,
		RspHdrContentLocation:                     p.RspHdrContentLocation,
		RspHdrForwarded:                           p.RspHdrForwarded,
		RspHdrVia:                                 p.RspHdrVia,
		RspHdrAcceptRanges:                        p.RspHdrAcceptRanges,
		RspHdrContentRange:                        p.RspHdrContentRange,
		RspHdrLocation:                            p.RspHdrLocation,
		RspHdrRefresh:                             p.RspHdrRefresh,
		RspHdrAllow:                               p.RspHdrAllow,
		RspHdrServer:                              p.RspHdrServer,
		RspHdrCrossOriginEmbedderPolicy:           p.RspHdrCrossOriginEmbedderPolicy,
		RspHdrCrossOriginOpenerPolicy:             p.RspHdrCrossOriginOpenerPolicy,
		RspHdrCrossOriginResourcePolicy:           p.RspHdrCrossOriginResourcePolicy,
		RspHdrContentSecurityPolicy:               p.RspHdrContentSecurityPolicy,
		RspHdrContentSecurityPolicyReportOnly:     p.RspHdrContentSecurityPolicyReportOnly,
		RspHdrExpectCT:                            p.RspHdrExpectCT,
		RspHdrPermissionsPolicy:                   p.RspHdrPermissionsPolicy,
		RspHdrStrictTransportSecurity:             p.RspHdrStrictTransportSecurity,
		RspHdrXContentTypeOptions:                 p.RspHdrXContentTypeOptions,
		RspHdrXFrameOptions:                       p.RspHdrXFrameOptions,
		RspHdrXPermittedCrossDomainPolicies:       p.RspHdrXPermittedCrossDomainPolicies,
		RspHdrXPoweredBy:                          p.RspHdrXPoweredBy,
		RspHdrXXSSProtection:                      p.RspHdrXXSSProtection,
		RspHdrReportingEndpoints:                  p.RspHdrReportingEndpoints,
		RspHdrReportTo:                            p.RspHdrReportTo,
		RspHdrTransferEncoding:                    p.RspHdrTransferEncoding,
		RspHdrTrailer:                             p.RspHdrTrailer,
		RspHdrSecWebSocketAccept:                  p.RspHdrSecWebSocketAccept,
		RspHdrSecWebSocketProtocol:                p.RspHdrSecWebSocketProtocol,
		RspHdrSecWebSocketExtensions:              p.RspHdrSecWebSocketExtensions,
		RspHdrAcceptCH:                            p.RspHdrAcceptCH,
		RspHdrCriticalCH:                          p.RspHdrCriticalCH,
		RspHdrAttributionReportingEligible:        p.RspHdrAttributionReportingEligible,
		RspHdrAttributionReportingRegisterSource:  p.RspHdrAttributionReportingRegisterSource,
		RspHdrAttributionReportingRegisterTrigger: p.RspHdrAttributionReportingRegisterTrigger,
		RspHdrDate:                                p.RspHdrDate,
		RspHdrLink:                                p.RspHdrLink,
		RspHdrRetryAfter:                          p.RspHdrRetryAfter,
		RspHdrServerTiming:                        p.RspHdrServerTiming,
		RspHdrServiceWorker:                       p.RspHdrServiceWorker,
		RspHdrServiceWorkerAllowed:                p.RspHdrServiceWorkerAllowed,
		RspHdrSourceMap:                           p.RspHdrSourceMap,
		RspHdrUpgrade:                             p.RspHdrUpgrade,
		RspHdrAltSvc:                              p.RspHdrAltSvc,
		RspHdrPriority:                            p.RspHdrPriority,
		// HttpFingerprint
		Browser:        p.Browser,
		BrowserQuality: p.BrowserQuality,
		Server:         p.Server,
		ServerQuality:  p.ServerQuality,
		Language:       p.Language,
		DeviceGuess:    p.DeviceGuess,
		// AppTailer
		ClientConsumed:           p.ClientConsumed,
		ClientStartPayloadOffset: p.ClientStartPayloadOffset,
		ClientEndPayloadOffset:   p.ClientEndPayloadOffset,
		ClientStartTimestamp:     p.ClientStartTimestamp,
		ClientEndTimestamp:       p.ClientEndTimestamp,
		ServerConsumed:           p.ServerConsumed,
		ServerStartPayloadOffset: p.ServerStartPayloadOffset,
		ServerEndPayloadOffset:   p.ServerEndPayloadOffset,
		ServerStartTimestamp:     p.ServerStartTimestamp,
		ServerEndTimestamp:       p.ServerEndTimestamp,
		// TagsMeta
		Tags: p.Tags,
	}
}

// newNetworkStackListGetListItemOut 将 network.NetworkStack 映射为独立 Out 结构。
// 模型层专属类型 Level（network.LayerLevel）在此转换为视图层 uint8。
func newNetworkStackListGetListItemOut(p *network.NetworkStack) *NetworkStackListGetListItemOut {
	return &NetworkStackListGetListItemOut{
		// BaseMeta
		Namespace: p.Namespace,
		Capture:   p.Capture,
		Version:   p.Version,
		SessionID: p.SessionID,
		// TraceMeta
		TraceID: p.TraceID,
		// TimestampMeta
		Timestamp: p.Timestamp,
		CreatedAt: p.CreatedAt,
		// DirectionMeta
		Cli2Srv: p.Cli2Srv,
		// 帧信息
		TopProto: p.TopProto,
		Proto:    p.Proto,
		Seq:      p.Seq,
		Level:    uint8(p.Level),
		Headers:  p.Headers,
		// 载荷偏移
		PayloadStartOffset: p.PayloadStartOffset,
		PayloadEndOffset:   p.PayloadEndOffset,
	}
}

// HttpTransportDetailGetIn HTTP 传输详情查询入参
// 按命名空间 + 行唯一 ID 精确查询单条 HTTP 传输记录
type HttpTransportDetailGetIn struct {
	// Namespace 命名空间
	Namespace string `json:"namespace" binding:"required"`
	// ID 行全局唯一标识
	ID serializer.HexUUID `json:"id" binding:"required"`
}

// HttpTransportDetailGet 处理 HTTP 传输详情查询请求
// 以 namespace + id 精确定位单条记录；列表投影后行详情/收藏样例/堆栈下钻等
// 消费方经此接口补齐全量字段
func HttpTransportDetailGet(c *gin.Context) {
	var in HttpTransportDetailGetIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	db := middlewares.GetRawDatabase(c).Model(&network.HttpTransport{})
	var row network.HttpTransport
	if err := db.Where("namespace = ? AND id = ?", in.Namespace, in.ID).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, NewHttpTransportListGetListItemOut(&row))
}
