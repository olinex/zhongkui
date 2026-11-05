package middlewares

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"zhongkuisec.com/zhongkui/internal/manager/config"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto/key"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// 编译期断言：确保 JwtAuthenticator 实现了统一的凭据认证器接口。
var _ CredentialAuthenticator = (*JwtAuthenticator)(nil)

var (
	// ErrJwtBadToken JWT 令牌格式错误或无效
	ErrJwtBadToken = response.CreateCodeError(http.StatusUnauthorized, response.CodeBadJwtToken, nil)
	// ErrJwtTimeoutToken JWT 令牌已过期且超过刷新时间限制
	ErrJwtTimeoutToken = response.CreateCodeError(http.StatusUnauthorized, response.CodeTimeoutJwtToken, nil)
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
	cfg *config.AuthConfig
	// db 数据库连接，用于验证用户信息
	db *database.DB
	// kd 密钥派生器，用于从密码派生密钥
	kd *key.KeyDerivation
}

// NewJwtAuthenticator 创建新的 JWT 认证器实例
// 参数:
//   - cfg: 认证配置
//   - db: 数据库连接
//   - kd: 密钥派生器
//
// 返回:
//   - *JwtAuthenticator: 新创建的认证器实例
func NewJwtAuthenticator(cfg *config.AuthConfig, db *database.DB, kd *key.KeyDerivation) *JwtAuthenticator {
	return &JwtAuthenticator{
		cfg: cfg,
		db:  db,
		kd:  kd,
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
		} else if cryptoKey, err := j.cfg.ExtractCryptoKey(j.kd); err != nil {
			return nil, errors.WithStack(err)
		} else {
			return cryptoKey, nil
		}
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

// Name 返回本认证器的名称，用于日志与可观测性。
func (j *JwtAuthenticator) Name() string {
	return "jwt"
}

// Match 报告当前请求是否携带 JWT 令牌。
func (j *JwtAuthenticator) Match(c *gin.Context) bool {
	return j.getPureToken(c) != ""
}

// AuthenticateRequest 认证当前请求，满足 CredentialAuthenticator 接口：
// 令牌缺失或非法时统一返回 ErrJwtBadToken，由聚合器终止请求。
func (j *JwtAuthenticator) AuthenticateRequest(c *gin.Context) (*core.User, *response.CodeError) {
	if user := j.Authenticate(c); user != nil {
		return user, nil
	}
	return nil, ErrJwtBadToken
}

// Sign 为用户创建 JWT 令牌
// 根据用户信息生成包含声明的 JWT 令牌
// 参数:
//   - user: 用户对象
//
// 返回:
//   - string: 生成的 JWT 令牌字符串（包含前缀）
//   - *CodeError: 生成过程中出现的错误
func (j *JwtAuthenticator) Sign(user *core.User) (string, *response.CodeError) {
	claims := UserClaims{
		RegisteredClaims: j.cfg.ToJwtRegisterClaims(),
		UnionID:          user.UnionID,
		NickleName:       user.NickleName,
		Email:            user.Email,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS512, claims)
	if cryptoKey, err := j.cfg.ExtractCryptoKey(j.kd); err != nil {
		return "", response.CreateAccidentError(err)
	} else if tokenStr, err := token.SignedString(cryptoKey); err != nil {
		// TODO: 记录日志
		return "", ErrJwtBadToken
	} else {
		return j.appendPrefix(tokenStr), nil
	}
}

// parseWithoutTimeValidation 解析 JWT 令牌但不校验时间类声明（exp/nbf/iat），
// 仅供 Refresh 使用：过期令牌在刷新窗口内仍可被解析并重新签发，
// 从而支持"持续访问则令牌持续有效"的滑动过期策略。
func (j *JwtAuthenticator) parseWithoutTimeValidation(token string) (*jwt.Token, *UserClaims, error) {
	claims := UserClaims{}
	jToken, err := jwt.ParseWithClaims(token, &claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrHashUnavailable
		} else if cryptoKey, err := j.cfg.ExtractCryptoKey(j.kd); err != nil {
			return nil, errors.WithStack(err)
		} else {
			return cryptoKey, nil
		}
	}, jwt.WithoutClaimsValidation())
	return jToken, &claims, errors.WithStack(err)
}

// Refresh 刷新 JWT 令牌
// 检查当前令牌是否有效且在可刷新时间范围内，返回新令牌或原令牌
// 参数:
//   - c: Gin 上下文
//
// 返回:
//   - string: 新的或原始的 JWT 令牌字符串
//   - *CodeError: 刷新过程中出现的错误
func (j *JwtAuthenticator) Refresh(c *gin.Context) (string, *response.CodeError) {
	// 使用不校验时间声明的解析，否则过期令牌会在解析阶段被拒绝，
	// 导致"过期后在刷新窗口内重新签发"的分支永远不可达
	token, claims, err := j.parseWithoutTimeValidation(j.getPureToken(c))
	if err != nil ||
		!token.Valid ||
		!j.managable(claims) {
		return "", ErrJwtBadToken
	}
	now := time.Now()
	if now.Sub(claims.ExpiresAt.Time) > j.cfg.TokenRefreshTimeout {
		return "", ErrJwtTimeoutToken
	}
	// 在刷新窗口内一律重新签发（滑动过期）。前端在令牌剩余寿命的 90% 处触发刷新，
	// 若此处对未过期令牌原样返回，exp 不会延后，前端按「剩余寿命 × 0.9」重排的续期
	// 定时会等比收缩至毫秒级，既产生密集的刷新请求，也会在过期瞬间让在途请求拿到
	// 401 而被强制登出（进而卸载 AI 抽屉、中止在途 SSE 流）。
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
