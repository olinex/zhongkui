package base

const (
	GlobalPrefix = "zki"
	// rbac model
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
	// rbac label
	WideMatchLabel = "*"
	PolicyLabel    = "p"
	GroupLabel     = "g"
	EffectAllow    = "allow"
	EffectDeny     = "deny"

	// buildin group
	SuperuserGroup = "superuser"
	AdminGroup     = "admin"
	DefaultGroup   = "default"

	// buildin namespace
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

	// http headers
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
