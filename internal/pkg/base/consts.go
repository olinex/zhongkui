package base

const (
	GLOBAL_PREFIX = "zki"
	// rbac model
	RBAC_MODEL = `
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
	WIDE_MATCH_LABEL = "*"
	POLICY_LABEL     = "p"
	GROUP_LABEL      = "g"
	EFFECT_ALLOW     = "allow"
	EFFECT_DENY      = "deny"

	// buildin group
	SUPERUSER_GROUP = "superuser"
	ADMIN_GROUP     = "admin"
	DEFAULT_GROUP   = "default"

	// buildin namespace
	DEFAULT_NAMESPACE = "default"

	// http headers
	HDR_HOST                             = "Host"
	HDR_ACCEPT                           = "Accept"
	HDR_ACCEPT_ENCODING                  = "Accept-Encoding"
	HDR_ACCEPT_LANGUAGE                  = "Accept-Language"
	HDR_TRANSFER_ENCODING                = "Transfer-Encoding"
	HDR_CONNECTION                       = "Connection"
	HDR_COOKIE                           = "Cookie"
	HDR_AUTHORIZATION                    = "Authorization"
	HDR_CONTENT_TYPE                     = "Content-Type"
	HDR_CONTENT_LENGTH                   = "Content-Length"
	HDR_SERVER                           = "Server"
	HDR_SET_COOKIE                       = "Set-Cookie"
	HDR_LOCATION                         = "Location"
	HDR_CACHE_CONTROL                    = "Cache-Control"
	HDR_CONTENT_ENCODING                 = "Content-Encoding"
	HDR_ACCESS_CONTROL_ALLOW_CREDENTIALS = "Access-Control-Allow-Credentials"
	HDR_ACCESS_CONTROL_ALLOW_HEADERS     = "Access-Control-Allow-Headers"
	HDR_ACCESS_CONTROL_ALLOW_METHODS     = "Access-Control-Allow-Methods"
	HDR_ACCESS_CONTROL_ALLOW_ORIGIN      = "Access-Control-Allow-Origin"
	HDR_ACCESS_CONTROL_REQUEST_METHOD    = "Access-Control-Request-Method"
	HDR_CONTENT_SECURITY_POLICY          = "Content-Security-Policy"
	HDR_PERMISSIONS_POLICY               = "Permissions-Policy"
	HDR_REFERRER_POLICY                  = "Referrer-Policy"
	HDR_STRICT_TRANSPORT_SECURITY        = "Strict-Transport-Security"
	HDR_X_CONTENT_TYPE_OPTIONS           = "X-Content-Type-Options"
	HDR_X_FRAME_OPTIONS                  = "X-Frame-Options"
	HDR_X_XSS_PROTECTION                 = "X-XSS-Protection"
)
