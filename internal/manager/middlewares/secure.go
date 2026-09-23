package middlewares

import (
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"zhongkui.com/zhongkui/internal/manager/config"
	"zhongkui.com/zhongkui/internal/pkg/base"
)

// SecureHeadersMiddleware 创建 HTTP 安全响应头中间件。
//
// 该中间件基于 SecureConfig 配置，为每个 HTTP 响应自动添加一组
// OWASP 推荐的安全响应头，用于防御点击劫持、XSS 攻击、代码注入、
// MIME 嗅探等常见 Web 安全威胁。
//
// 设置的安全响应头包括:
//   - Content-Security-Policy: 内容安全策略，控制页面可加载的资源来源
//   - Permissions-Policy: 权限策略，控制浏览器功能（摄像头、麦克风等）的使用
//   - Referrer-Policy: 引用策略，控制 Referer 请求头的发送行为
//   - Strict-Transport-Security: 严格传输安全，强制使用 HTTPS 访问
//   - X-Content-Type-Options: 禁止 MIME 类型嗅探
//   - X-Frame-Options: 帧选项，防止页面被 iframe 嵌套（点击劫持防御）
//   - X-XSS-Protection: XSS 过滤器，启用浏览器内置的 XSS 防护
//
// 参数:
//   - cfg: 安全响应头配置指针，各字段含义参见 SecureConfig 结构体定义
//
// 返回:
//   - gin.HandlerFunc: Gin 框架中间件处理函数
func SecureHeadersMiddleware(cfg *config.SecureConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header(base.HdrContentSecurityPolicy, cfg.ContentSecurityPolicy)
		c.Header(base.HdrPermissionsPolicy, cfg.PermissionsPolicy)
		c.Header(base.HdrReferrerPolicy, cfg.ReferrerPolicy)
		c.Header(base.HdrStrictTransportSecurity, cfg.StrictTransportSecurity)
		c.Header(base.HdrXContentTypeOptions, cfg.XContentTypeOptions)
		c.Header(base.HdrXFrameOptions, cfg.XFrameOptions)
		c.Header(base.HdrXXssProtection, cfg.XXSSProtection)
		c.Next()
	}
}

// CORSHeadersMiddleware 创建跨域资源共享（CORS）响应头中间件。
//
// 该中间件基于 CORSConfig 配置，为每个 HTTP 响应添加 CORS 相关的响应头，
// 并根据请求类型进行不同的处理：
//   - 非 OPTIONS 请求: 仅设置 CORS 响应头后继续处理后续中间件
//   - OPTIONS 请求（无 Access-Control-Request-Method 头）: 视为普通请求，继续处理
//   - OPTIONS 预检请求（请求方法在允许列表中）: 返回 204 No Content，告知浏览器预检通过
//   - OPTIONS 预检请求（请求方法不在允许列表中）: 返回 403 Forbidden，拒绝跨域访问
//
// 对于 Access-Control-Allow-Origin，中间件检查请求的 Origin 头：
//   - Origin 位于 AllowOrigins 允许列表（或列表包含 *）时，回显该 Origin。
//     由于 AllowCredentials 开启时浏览器禁止使用 *，回显具体来源是
//     允许携带凭证的跨域请求的唯一正确方式，同时可支持多个允许来源
//   - Origin 不在允许列表或请求未携带 Origin 头时，不设置该响应头，
//     浏览器将据此阻止跨域访问
//
// 设置的 CORS 响应头包括:
//   - Access-Control-Allow-Origin: 允许的跨域请求来源
//   - Access-Control-Allow-Credentials: 是否允许携带凭证
//   - Access-Control-Allow-Headers: 允许的请求头列表
//   - Access-Control-Allow-Methods: 允许的 HTTP 方法列表
//
// 参数:
//   - cfg: CORS 策略配置指针，各字段含义参见 CORSConfig 结构体定义
//
// 返回:
//   - gin.HandlerFunc: Gin 框架中间件处理函数
func CORSHeadersMiddleware(cfg *config.CORSConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header(base.HdrAccessControlAllowCredentials, strconv.FormatBool(cfg.AllowCredentials))
		c.Header(base.HdrAccessControlAllowHeaders, strings.Join(cfg.AllowHeaders, ", "))
		c.Header(base.HdrAccessControlAllowMethods, strings.Join(cfg.AllowMethods, ", "))

		if origin := c.Request.Header.Get(base.HdrOrigin); origin != "" &&
			(slices.Contains(cfg.AllowOrigins, origin) || slices.Contains(cfg.AllowOrigins, "*")) {
			c.Header(base.HdrAccessControlAllowOrigin, origin)
		}

		if c.Request.Method != http.MethodOptions {
			c.Next()
		} else if requestMethod := c.Request.Header.Get(base.HdrAccessControlRequestMethod); requestMethod == "" {
			c.Next()
		} else if slices.Contains(cfg.AllowMethods, requestMethod) {
			c.AbortWithStatus(http.StatusNoContent)
		} else {
			c.AbortWithStatus(http.StatusForbidden)
		}
	}
}
