package manager

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

const (
	// ANONYMOUS_USERNAME 定义匿名用户的用户名
	ANONYMOUS_USERNAME = "anon"
	// AUTH_CTX_NAME 定义在 Gin 上下文中存储认证器的键名
	AUTH_CTX_NAME = "auth"
	// USER_CTX_NAME 定义在 Gin 上下文中存储用户信息的键名
	USER_CTX_NAME = "user"
)

var (
	// ErrUserNotExist 用户不存在的错误
	ErrUserNotExist = CreateCodeError(http.StatusUnauthorized, CodeUserNotExist, nil)
	// ErrJwtBadToken JWT 令牌格式错误或无效
	ErrJwtBadToken = CreateCodeError(http.StatusUnauthorized, CodeBadJwtToken, nil)
	// ErrJwtTimeoutToken JWT 令牌已过期且超过刷新时间限制
	ErrJwtTimeoutToken = CreateCodeError(http.StatusUnauthorized, CodeTimeoutJwtToken, nil)

	// anonymous 匿名用户对象，用于处理不需要认证的请求
	anonymous = &core.User{
		Username: ANONYMOUS_USERNAME,
		UnionID: [16]byte{
			0xff, 0xff, 0xff, 0xff,
			0xff, 0xff, 0xff, 0xff,
			0xff, 0xff, 0xff, 0xff,
			0xff, 0xff, 0xff, 0xff,
		},
	}
)

// UserClaims 自定义 JWT 声明结构
// 继承 jwt.RegisteredClaims 标准声明，并添加用户相关字段
type UserClaims struct {
	jwt.RegisteredClaims
	// UnionID 用户的全局唯一标识符
	UnionID serializer.HexUUID `json:"unionID"`
	// NickleName 用户昵称
	NickleName string `json:"nickleName"`
	// Email 用户邮箱地址
	Email string `json:"email"`
}

// JwtAuthenticator JWT 认证器
// 负责 JWT 令牌的创建、验证、刷新和管理
type JwtAuthenticator struct {
	// cfg 认证配置，包含密钥、超时时间等参数
	cfg *AuthConfig
	// db 数据库连接，用于验证用户信息
	db *database.DB
}

// NewJwtAuthenticator 创建新的 JWT 认证器实例
// 参数:
//   - cfg: 认证配置
//   - db: 数据库连接
//
// 返回:
//   - *JwtAuthenticator: 新创建的认证器实例
func NewJwtAuthenticator(cfg *AuthConfig, db *database.DB) *JwtAuthenticator {
	return &JwtAuthenticator{
		cfg: cfg,
		db:  db,
	}
}

// hasPrefix 检查令牌是否包含配置的前缀
// 参数:
//   - token: 待检查的令牌字符串
//
// 返回:
//   - bool: 如果令牌以配置的前缀开头返回 true，否则返回 false
func (j *JwtAuthenticator) hasPrefix(token string) bool {
	return strings.HasPrefix(token, j.cfg.TokenPrefix)
}

// removePrefix 移除令牌中的前缀
// 参数:
//   - token: 原始令牌字符串
//
// 返回:
//   - string: 移除前缀后的令牌字符串
func (j *JwtAuthenticator) removePrefix(token string) string {
	return strings.TrimPrefix(token, j.cfg.TokenPrefix)
}

// appendPrefix 为令牌添加前缀
// 参数:
//   - token: 原始令牌字符串
//
// 返回:
//   - string: 添加前缀后的令牌字符串
func (j *JwtAuthenticator) appendPrefix(token string) string {
	return j.cfg.TokenPrefix + token
}

// managable 检查声明是否可管理
// 验证令牌的签发者和主题是否与配置匹配
// 参数:
//   - claims: 用户声明
//
// 返回:
//   - bool: 如果声明可管理返回 true，否则返回 false
func (j *JwtAuthenticator) managable(claims *UserClaims) bool {
	return (claims.Issuer == j.cfg.Issuer &&
		claims.Subject == j.cfg.Subject)
}

// getPureToken 从 Gin 上下文中获取纯净的令牌（移除前缀）
// 参数:
//   - c: Gin 上下文
//
// 返回:
//   - string: 纯净的令牌字符串
func (j *JwtAuthenticator) getPureToken(c *gin.Context) string {
	if temp := c.GetHeader(j.cfg.HeaderName); j.hasPrefix(temp) {
		return j.removePrefix(temp)
	} else {
		return temp
	}
}

// Parse 解析 JWT 令牌
// 使用配置的密钥验证令牌签名，并提取用户声明
// 参数:
//   - token: 待解析的 JWT 令牌字符串
//
// 返回:
//   - *jwt.Token: 解析后的 JWT 令牌对象
//   - *UserClaims: 提取的用户声明
//   - error: 解析过程中出现的错误
func (j *JwtAuthenticator) Parse(token string) (*jwt.Token, *UserClaims, error) {
	claims := UserClaims{}
	jToken, err := jwt.ParseWithClaims(token, &claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrHashUnavailable
		}
		return []byte(j.cfg.SecretKey), nil
	})
	return jToken, &claims, errors.WithStack(err)
}

// Authenticate 认证用户请求
// 从请求中提取令牌，验证有效性，并返回对应的用户信息
// 参数:
//   - c: Gin 上下文
//
// 返回:
//   - *core.User: 认证成功的用户对象，认证失败返回 nil
func (j *JwtAuthenticator) Authenticate(c *gin.Context) *core.User {
	token := j.getPureToken(c)
	if token == "" {
		return nil
	}
	jToken, claims, err := j.Parse(token)
	if err != nil || !jToken.Valid {
		GetLogger(c).Debugf(c, nil, "jwt authenticate failed cause: %v", err)
		return nil
	}
	user := core.User{}
	j.db.Take(&user, &core.User{UnionID: claims.UnionID})
	if user.ID != 0 {
		return &user
	} else {
		return nil
	}
}

// Sign 为用户创建 JWT 令牌
// 根据用户信息生成包含声明的 JWT 令牌
// 参数:
//   - user: 用户对象
//
// 返回:
//   - string: 生成的 JWT 令牌字符串（包含前缀）
//   - *CodeError: 生成过程中出现的错误
func (j *JwtAuthenticator) Sign(user *core.User) (string, *CodeError) {
	claims := UserClaims{
		RegisteredClaims: j.cfg.ToJwtRegisterClaims(),
		UnionID:          user.UnionID,
		NickleName:       user.NickleName,
		Email:            user.Email,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS512, claims)
	if tokenStr, err := token.SignedString([]byte(j.cfg.SecretKey)); err != nil {
		// TODO: log
		return "", ErrJwtBadToken
	} else {
		return j.appendPrefix(tokenStr), nil
	}
}

// Refresh 刷新 JWT 令牌
// 检查当前令牌是否有效且在可刷新时间范围内，返回新令牌或原令牌
// 参数:
//   - c: Gin 上下文
//
// 返回:
//   - string: 新的或原始的 JWT 令牌字符串
//   - *CodeError: 刷新过程中出现的错误
func (j *JwtAuthenticator) Refresh(c *gin.Context) (string, *CodeError) {
	token, claims, err := j.Parse(j.getPureToken(c))
	if err != nil ||
		!token.Valid ||
		!j.managable(claims) {
		return "", ErrJwtBadToken
	}
	now := time.Now()
	if now.Sub(claims.ExpiresAt.Time) > j.cfg.TokenRefreshTimeout {
		return "", ErrJwtTimeoutToken
	}
	if claims.ExpiresAt.Sub(now) > 0 || claims.NotBefore.Sub(now) > 0 {
		return j.appendPrefix(token.Raw), nil
	}
	return j.Sign(&core.User{
		UnionID:    claims.UnionID,
		NickleName: claims.NickleName,
		Email:      claims.Email,
	})
}

// TokenCookieInject 将令牌注入到 Cookie 中
// 参数:
//   - c: Gin 上下文
//   - token: JWT 令牌字符串
func (j *JwtAuthenticator) TokenCookieInject(c *gin.Context, token string) {
	c.SetCookie(
		j.cfg.Cookie.Name,
		token,
		int(j.cfg.TokenRefreshTimeout.Seconds()),
		j.cfg.Cookie.Path,
		j.cfg.Cookie.Domain,
		j.cfg.Cookie.Secure,
		j.cfg.Cookie.HttpOnly,
	)
}

// AuthenticatorMiddleware 认证器中间件
// 将认证器实例设置到 Gin 上下文中，供后续处理使用
// 返回:
//   - gin.HandlerFunc: Gin 中间件函数
func (j *JwtAuthenticator) AuthenticatorMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(AUTH_CTX_NAME, j)
		c.Next()
	}
}

// AuthenticateMiddleware 认证中间件
// 验证请求中的 JWT 令牌，将用户信息设置到上下文中
// 参数:
//   - allowAnonymous: 是否允许匿名访问
//
// 返回:
//   - gin.HandlerFunc: Gin 中间件函数
func (j *JwtAuthenticator) AuthenticateMiddleware(allowAnonymous bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, span := StartMiddlewareSpan(c, "authenticate")
		defer span.End()
		attrs := []attribute.KeyValue{attribute.Bool("allow-anon", allowAnonymous)}
		if user := j.Authenticate(c); user != nil {
			attrs = append(attrs, attribute.String("user", user.UnionID.String()))
			c.Set(USER_CTX_NAME, user)
		} else if allowAnonymous {
			attrs = append(attrs, attribute.String("user", "anon"))
			c.Set(USER_CTX_NAME, anonymous)
		} else {
			span.SetAttributes(attrs...)
			span.SetStatus(codes.Error, "auth failed")
			ErrJwtBadToken.AbortWithGinH(c)
			return
		}
		span.SetAttributes(attrs...)
		span.SetStatus(codes.Ok, "auth success")
		c.Next()
	}
}

// GetUser 从 Gin 上下文中获取用户信息
// 参数:
//   - c: Gin 上下文
//
// 返回:
//   - *core.User: 用户对象
//   - *CodeError: 如果用户不存在返回错误
func GetUser(c *gin.Context) (*core.User, *CodeError) {
	if user, ok := c.Get(USER_CTX_NAME); ok {
		return user.(*core.User), nil
	} else {
		return nil, ErrUserNotExist
	}
}

// GetAuth 从 Gin 上下文中获取认证器实例
// 参数:
//   - c: Gin 上下文
//
// 返回:
//   - *JwtAuthenticator: 认证器实例，如果不存在返回 nil
func GetAuth(c *gin.Context) *JwtAuthenticator {
	if auth, ok := c.Get(AUTH_CTX_NAME); ok {
		return auth.(*JwtAuthenticator)
	} else {
		return nil
	}
}
