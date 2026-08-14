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
