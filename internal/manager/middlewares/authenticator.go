package middlewares

import (
	// 标准库
	"net/http"

	// 第三方
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/models/core"
)

// CredentialAuthenticator 是一路凭据的认证器抽象。聚合器按注册顺序选择第一个匹配当前请求的
// 认证器执行认证；一旦匹配，认证失败即终止请求，不回退到其他方式或匿名身份。
type CredentialAuthenticator interface {
	// Name 返回认证方式名称，用于日志与可观测性。
	Name() string
	// Match 报告当前请求是否携带本认证方式的凭据。
	Match(c *gin.Context) bool
	// AuthenticateRequest 认证当前请求；成功返回用户，失败返回业务错误。
	AuthenticateRequest(c *gin.Context) (*core.User, *response.CodeError)
}

const (
	// AnonymousUsername 定义匿名用户的用户名
	AnonymousUsername = "anon"
	// AuthCtxName 定义在 Gin 上下文中存储统一鉴权聚合器的键名
	AuthCtxName = "auth"
	// UserCtxName 定义在 Gin 上下文中存储用户信息的键名
	UserCtxName = "user"
)

var (
	// ErrNoCredential 表示请求未携带任何受支持的认证凭据。
	// 业务码沿用 CodeBadJwtToken，保持既有 401 响应契约不变。
	ErrNoCredential = response.CreateCodeError(http.StatusUnauthorized, response.CodeBadJwtToken, nil)
	// ErrUserNotExist 表示请求上下文中没有已认证用户。
	ErrUserNotExist = response.CreateCodeError(http.StatusUnauthorized, response.CodeUserNotExist, nil)

	// anonymous 匿名用户对象，用于处理不需要认证的请求
	anonymous = &core.User{
		Username: AnonymousUsername,
		UnionID: [16]byte{
			0xff, 0xff, 0xff, 0xff,
			0xff, 0xff, 0xff, 0xff,
			0xff, 0xff, 0xff, 0xff,
			0xff, 0xff, 0xff, 0xff,
		},
	}
)

// Authenticator 是统一的多路鉴权聚合器：按注册顺序遍历 CredentialAuthenticator，
// 命中第一个携带凭据的认证器后只执行该路认证，从而实现可扩展的多路鉴权中间件。
type Authenticator struct {
	// authenticators 按注册顺序排列的凭据认证器
	authenticators []CredentialAuthenticator
}

// NewAuthenticator 创建多路鉴权聚合器。传入 nil 认证器属于装配错误，直接 panic（fail fast）。
func NewAuthenticator(authenticators ...CredentialAuthenticator) *Authenticator {
	items := make([]CredentialAuthenticator, 0, len(authenticators))
	for _, item := range authenticators {
		if item == nil {
			panic("middlewares: nil credential authenticator")
		}
		items = append(items, item)
	}
	return &Authenticator{authenticators: items}
}

// JWT 返回聚合器中注册的 JWT 认证器；未注册时返回 nil。
// 登录、刷新等需要签发令牌的处理器经此取用，认证选择本身仍由聚合器负责。
func (a *Authenticator) JWT() *JwtAuthenticator {
	for _, item := range a.authenticators {
		if jwt, ok := item.(*JwtAuthenticator); ok {
			return jwt
		}
	}
	return nil
}

// AuthenticatorMiddleware 把统一鉴权聚合器注入请求上下文，供后续处理器通过 GetAuth 取用。
// 需要签发 JWT 的处理器经 GetJWT 从聚合器中取回 JWT 认证器，不再单独注入第二份实例。
//
// 返回:
//   - gin.HandlerFunc: Gin 中间件函数
func (a *Authenticator) AuthenticatorMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(AuthCtxName, a)
		c.Next()
	}
}

// AuthenticateMiddleware 认证中间件：按注册顺序选择第一个匹配的认证器执行认证，
// 成功时把用户写入上下文；无任何凭据命中时按 allowAnonymous 注入匿名用户或返回 401。
//
// 参数:
//   - allowAnonymous: 是否允许匿名访问
//
// 返回:
//   - gin.HandlerFunc: Gin 中间件函数
func (a *Authenticator) AuthenticateMiddleware(allowAnonymous bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, span := StartMiddlewareSpan(c, "authenticate")
		defer span.End()
		attrs := []attribute.KeyValue{attribute.Bool("allow-anon", allowAnonymous)}
		for _, item := range a.authenticators {
			if !item.Match(c) {
				continue
			}
			user, codeErr := item.AuthenticateRequest(c)
			if codeErr != nil {
				attrs = append(attrs, attribute.String("method", item.Name()))
				span.SetAttributes(attrs...)
				span.SetStatus(codes.Error, item.Name()+" auth failed")
				codeErr.AbortWithGinH(c)
				return
			}
			attrs = append(attrs,
				attribute.String("method", item.Name()),
				attribute.String("user", user.UnionID.String()),
			)
			c.Set(UserCtxName, user)
			span.SetAttributes(attrs...)
			span.SetStatus(codes.Ok, "auth success")
			c.Next()
			return
		}
		if allowAnonymous {
			attrs = append(attrs, attribute.String("user", AnonymousUsername))
			c.Set(UserCtxName, anonymous)
			span.SetAttributes(attrs...)
			span.SetStatus(codes.Ok, "auth success")
			c.Next()
			return
		}
		span.SetAttributes(attrs...)
		span.SetStatus(codes.Error, "auth failed")
		ErrNoCredential.AbortWithGinH(c)
	}
}

// GetUser 从 Gin 上下文中获取用户信息。
// 参数:
//   - c: Gin 上下文
//
// 返回:
//   - *core.User: 用户对象
//   - *CodeError: 如果用户不存在返回错误
func GetUser(c *gin.Context) (*core.User, *response.CodeError) {
	if user, ok := c.Get(UserCtxName); ok {
		return user.(*core.User), nil
	} else {
		return nil, ErrUserNotExist
	}
}

// RequireUserID 从请求上下文获取当前请求用户 ID（users.id）。
//
// 与 GetUser 的区别在于：用户缺失时不会返回错误值由调用方处理，而是直接中断请求并返回 false，
// 适用于必须已认证才能继续执行的处理器。
//
// 参数：
//   - c: Gin 请求上下文
//
// 返回值：
//   - uint: 当前请求用户 ID；获取失败时为 0
//   - bool: 是否成功获取；false 表示用户缺失，请求已被中断
func RequireUserID(c *gin.Context) (uint, bool) {
	user, err := GetUser(c)
	if err != nil {
		err.AbortWithGinH(c)
		return 0, false
	}
	return user.ID, true
}

// GetAuth 从 Gin 上下文中获取统一鉴权聚合器。
// 参数:
//   - c: Gin 上下文
//
// 返回:
//   - *Authenticator: 聚合器实例，如果不存在返回 nil
func GetAuth(c *gin.Context) *Authenticator {
	if auth, ok := c.Get(AuthCtxName); ok {
		if aggregator, ok := auth.(*Authenticator); ok {
			return aggregator
		}
	}
	return nil
}

// GetJWT 从请求上下文中的统一鉴权聚合器里取回 JWT 认证器，供登录 / 刷新等需要签发令牌的
// 处理器使用。JWT 实例不再单独注入上下文，始终由聚合器这一份注册实例派生。
// 参数:
//   - c: Gin 上下文
//
// 返回:
//   - *JwtAuthenticator: JWT 认证器实例，聚合器或 JWT 认证器不存在时返回 nil
func GetJWT(c *gin.Context) *JwtAuthenticator {
	aggregator := GetAuth(c)
	if aggregator == nil {
		return nil
	}
	return aggregator.JWT()
}
