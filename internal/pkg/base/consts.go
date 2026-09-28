// Package base 提供应用的基础定义：环境配置（Env）、全局常量、语言类型及其解析与序列化、
// 主机名工具，以及语言与策略动作的枚举注册。
package base

// 通用常量：资源前缀、RBAC 模型与标签、内置分组/命名空间、权限点前缀与语义动作、HTTP 头部名。
const (
	// GlobalPrefix 全局资源名称统一前缀。
	GlobalPrefix = "zki"
	// rbac 模型定义（casbin 模型文本）。
	RbacModel = `
[request_definition]
r = sub, obj, act, nsp

[policy_definition]
p = sub, obj, act, eft

[role_definition]
g = _, _, _

[policy_effect]
e = some(where (p.eft == allow)) && !some(where (p.eft == deny))

[matchers]
m = (p.obj == "*" || r.obj == p.obj) && \
    (p.act == "*" || r.act == p.act) && \
	(g(r.sub, p.sub, "*") || g(r.sub, p.sub, r.nsp))
`
	// rbac 标签
	WideMatchLabel = "*"
	PolicyLabel    = "p"
	GroupLabel     = "g"
	EffectAllow    = "allow"
	EffectDeny     = "deny"

	// 内置分组
	SuperuserGroup = "superuser"
	AdminGroup     = "admin"
	DefaultGroup   = "default"

	// 内置命名空间
	DefaultNamespace = "default"

	// 全局权限点（permission point）格式规范：
	//   object = {prefix}:{subtype}:{model}:{field}
	//   action = 语义动作（大写），如 READ / WRITE
	// 说明：
	//   - prefix 区分权限点大类（如 plaintext 明文查看）
	//   - subtype 区分同一大类下的资源形态（crypto 加密字段 / s3 对象）
	//   - model 为模型注册名（RegisterPlaintextModel 时指定，如 ipv4-tcp、http-transport）
	//   - field 为模型字段的 json 名
	// 示例：plaintext:crypto:http-transport:req_hdr_authorization（READ）
	//       plaintext:s3:http-transport:req_body（READ）

	// 权限点前缀：api 为接口层（object = api:{FullPath}，action = HTTP 方法），
	// plaintext 为明文查看数据层（object = plaintext:{subtype}:{model}:{field}，action = 语义动作）
	APIPrefix       = "api"
	PlaintextPrefix = "plaintext"
	PlaintextCrypto = "crypto"
	PlaintextS3     = "s3"

	// 全局语义动作词汇表（后续权限点可复用）
	ActionRead  = "READ"
	ActionWrite = "WRITE"

	// HTTP 请求头名
	HdrHost                          = "Host"
	HdrOrigin                        = "Origin"
	HdrAccept                        = "Accept"
	HdrAcceptEncoding                = "Accept-Encoding"
	HdrAcceptLanguage                = "Accept-Language"
	HdrTransferEncoding              = "Transfer-Encoding"
	HdrConnection                    = "Connection"
	HdrCookie                        = "Cookie"
	HdrAuthorization                 = "Authorization"
	HdrContentType                   = "Content-Type"
	HdrContentLength                 = "Content-Length"
	HdrServer                        = "Server"
	HdrSetCookie                     = "Set-Cookie"
	HdrLocation                      = "Location"
	HdrCacheControl                  = "Cache-Control"
	HdrContentEncoding               = "Content-Encoding"
	HdrAccessControlAllowCredentials = "Access-Control-Allow-Credentials"
	HdrAccessControlAllowHeaders     = "Access-Control-Allow-Headers"
	HdrAccessControlAllowMethods     = "Access-Control-Allow-Methods"
	HdrAccessControlAllowOrigin      = "Access-Control-Allow-Origin"
	HdrAccessControlRequestMethod    = "Access-Control-Request-Method"
	HdrContentSecurityPolicy         = "Content-Security-Policy"
	HdrPermissionsPolicy             = "Permissions-Policy"
	HdrReferrerPolicy                = "Referrer-Policy"
	HdrStrictTransportSecurity       = "Strict-Transport-Security"
	HdrXContentTypeOptions           = "X-Content-Type-Options"
	HdrXFrameOptions                 = "X-Frame-Options"
	HdrXXssProtection                = "X-XSS-Protection"
)

// 数据库名（database 配置节点名 / migration 归属统一标识）：核心库、原始库及其测试库。
// 作为单一事实来源，供 migrations 与 provider 共同引用，避免重复字面量引发不一致。
const (
	CoreDbName     = "core"
	CoreTestDbName = "core_test"
	RawDbName      = "raw"
	RawTestDbName  = "raw_test"
)

// logger 配置前缀，作为单一事实来源供 provider 引用。
const (
	DefaultLoggerName  = "default"
	ApiLoggerName      = "api"
	DatabaseLoggerName = "database"
	CaptureLoggerName  = "capture"
	EtcdLoggerName     = "etcd"
	GrpcLoggerName     = "grpc"
	PipelineLoggerName = "pipeline"
)

// redis 配置前缀，作为单一事实来源供 provider 引用。
const (
	CacheRedisName    = "cache"
	PipelineRedisName = "pipeline"
)

// 上报服务 gRPC 配置前缀，作为单一事实来源供 provider 引用。
const ReporterGrpcName = "reporter"

// 注入器上下文键名，用于在任务上下文或注入器中定位 do.Scope。
const ContextScopeName = "ctx_scope"
