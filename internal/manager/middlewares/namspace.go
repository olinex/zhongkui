// Package middlewares 提供了 HTTP 请求处理所需的核心中间件与管理功能，
// 包括命名空间识别、认证鉴权、数据库连接管理、响应序列化等。
package middlewares

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"zhongkui.com/zhongkui/internal/manager/config"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/models/core"
)

// DefaultNamespace 定义默认命名空间名称。
// 当请求中未携带任何命名空间标识时，将使用此值作为当前请求所属的命名空间。
const DefaultNamespace = "default"

// NamespaceCtxName 定义命名空间在 Gin 上下文中存储时使用的键名。
// 通过 c.Set / c.Get 与此键名进行命名空间的存取操作。
const NamespaceCtxName = "namespace"

var (
	// ErrNampspaceDoesNotExist 表示请求中指定的命名空间不存在的错误。
	// 当 NamespaceMiddleware 校验命名空间是否存在时，若不存在则使用此错误中断请求。
	// HTTP 状态码为 400（Bad Request），业务错误码为 CodeNamespaceNotExist（1009）。
	ErrNampspaceDoesNotExist = response.CreateCodeError(http.StatusBadRequest, response.CodeNamespaceNotExist, nil)
)

// decodeNamespace 将命名空间标识从 HTTP 传输编码（URL 百分号编码）还原为原始名称。
//
// 命名空间名称允许包含中文等非 ASCII 字符，而 HTTP 请求头值仅允许 ISO-8859-1（Latin-1）
// 字符，前端在写入请求头前会先调用 encodeURIComponent 编码；否则浏览器会抛出
// "String contains non ISO-8859-1 code point" 异常导致请求从未发出。
// 解码失败（如旧客户端直接写入未编码的原始值）时回退使用原始值，保证兼容性。
func decodeNamespace(raw string) string {
	if decoded, err := url.PathUnescape(raw); err == nil {
		return decoded
	}
	return raw
}

// NamespaceMiddleware 创建一个命名空间识别中间件。
//
// 该中间件从 HTTP 请求中提取命名空间标识，按以下优先级依次查找：
//  1. HTTP 请求头（由 cfg.HeaderName 指定，默认 "X-Namespace"）
//  2. URL 查询参数（由 cfg.QueryName 指定，默认 "namespace"）
//  3. Cookie（由 cfg.CookieName 指定，默认 "namespace"）
//
// 如果以上三个来源均未提供命名空间标识，则使用 DefaultNamespace（"default"）作为默认值。
// 提取到命名空间后，会调用 core.NamespaceExist 校验其是否存在于系统缓存中；
// 若不存在则中断请求并返回 ErrNampspaceDoesNotExist 错误响应；
// 若存在则将命名空间名称写入 Gin 上下文（键名为 NamespaceCtxName），并继续执行后续处理函数。
//
// 参数：
//   - cfg: 命名空间配置，定义了从请求头、查询参数和 Cookie 中获取命名空间标识的键名
//
// 返回值：
//   - gin.HandlerFunc: 可直接注册到 Gin 路由中的中间件处理函数
func NamespaceMiddleware(cfg *config.NamespaceConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, span := StartMiddlewareSpan(c, "namespace")
		defer span.End()
		namespace := DefaultNamespace
		if temp := c.GetHeader(cfg.HeaderName); temp != "" {
			namespace = decodeNamespace(temp)
		} else if temp, ok := c.GetQuery(cfg.QueryName); ok && temp != "" {
			namespace = decodeNamespace(temp)
		} else if temp, err := c.Cookie(cfg.CookieName); err == nil {
			namespace = decodeNamespace(temp)
		}
		if ok := core.NamespaceExist(namespace); !ok {
			span.SetAttributes(
				attribute.String("namespace", namespace),
				attribute.Bool("exists", false),
			)
			span.SetStatus(codes.Error, "namespace does not exist")
			ErrNampspaceDoesNotExist.AbortWithGinH(c)
			return
		}
		span.SetAttributes(
			attribute.String("namespace", namespace),
			attribute.Bool("exists", true),
		)
		c.Set(NamespaceCtxName, namespace)
		c.Next()
	}
}

// GetNamespace 从 Gin 上下文中获取当前请求所属的命名空间名称。
//
// 该函数应在 NamespaceMiddleware 执行之后调用，用于在后续处理函数中
// 获取由中间件解析并写入上下文的命名空间标识。
//
// 参数：
//   - c: Gin 请求上下文
//
// 返回值：
//   - string: 当前请求所属的命名空间名称
//   - *CodeError: 如果上下文中不存在命名空间信息，则返回 ErrNampspaceDoesNotExist
func GetNamespace(c *gin.Context) (string, *response.CodeError) {
	if namespace, ok := c.Get(NamespaceCtxName); ok {
		return namespace.(string), nil
	} else {
		return "", ErrNampspaceDoesNotExist
	}
}

// RequireNamespace 从请求上下文获取当前请求所属的命名空间名称。
//
// 与 GetNamespace 的区别在于：命名空间缺失时不会返回错误值由调用方处理，
// 而是直接中断请求并返回 false，适用于需要命名空间才能继续执行的处理器。
//
// 参数：
//   - c: Gin 请求上下文
//
// 返回值：
//   - string: 当前请求所属的命名空间名称；获取失败时为空串
//   - bool: 是否成功获取；false 表示命名空间缺失，请求已被中断
func RequireNamespace(c *gin.Context) (string, bool) {
	namespace, err := GetNamespace(c)
	if err != nil {
		err.AbortWithGinH(c)
		return "", false
	}
	return namespace, true
}
