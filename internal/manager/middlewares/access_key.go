package middlewares

import (
	// 标准库
	"net/http"

	// 第三方
	"github.com/gin-gonic/gin"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/models/core"
	authpkg "zhongkuisec.com/zhongkui/internal/pkg/auth"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// ErrBadAccessKey 表示访问密钥令牌无效、密钥不存在/停用/过期或签名校验失败。
var ErrBadAccessKey = response.CreateCodeError(http.StatusUnauthorized, response.CodeBadAccessKey, nil)

// AccessKeyAuthenticator 使用 X-Access-Key 请求头完成访问密钥签名鉴权。
// 令牌解析、方法比对、防重放、密钥状态与签名校验统一委托 internal/pkg/auth 的共享实现，
// 本类型只负责把 Web 请求的方法标识传入共享鉴权，并按用户标识加载用户。
type AccessKeyAuthenticator struct {
	// keys 访问密钥池，提供密钥查询、防重放与最后访问时间更新
	keys *core.AccessKeyPool
	// db 核心数据库连接，用于按密钥所属用户标识加载用户
	db *database.DB
}

// 编译期断言：确保 AccessKeyAuthenticator 实现了统一的凭据认证器接口。
var _ CredentialAuthenticator = (*AccessKeyAuthenticator)(nil)

// NewAccessKeyAuthenticator 创建访问密钥认证器。
// 参数:
//   - keys: 访问密钥池
//   - db: 核心数据库连接
//
// 返回:
//   - *AccessKeyAuthenticator: 新创建的认证器实例
func NewAccessKeyAuthenticator(keys *core.AccessKeyPool, db *database.DB) *AccessKeyAuthenticator {
	return &AccessKeyAuthenticator{keys: keys, db: db}
}

// Name 返回本认证器的名称，用于日志与可观测性。
func (a *AccessKeyAuthenticator) Name() string {
	return "access-key"
}

// Match 报告当前请求是否携带访问密钥签名令牌。
func (a *AccessKeyAuthenticator) Match(c *gin.Context) bool {
	return c.GetHeader(authpkg.AccessKeyHeaderName) != ""
}

// AuthenticateRequest 认证当前请求，满足 CredentialAuthenticator 接口。
//
// 参数:
//   - c: Gin 上下文
//
// 返回:
//   - *core.User: 认证成功的用户对象
//   - *response.CodeError: 认证失败时返回 ErrBadAccessKey
func (a *AccessKeyAuthenticator) AuthenticateRequest(c *gin.Context) (*core.User, *response.CodeError) {
	if a.keys == nil || a.db == nil {
		return nil, ErrBadAccessKey
	}
	ctx := c.Request.Context()
	// 令牌的方法标识把 HTTP 方法与路径绑定在一起，签发侧复用 auth.SignHTTPAccessKey。
	expectedMethod := authpkg.HTTPRequestMethod(c.Request.Method, c.Request.URL.Path)
	userID, err := authpkg.AuthenticateAccessKey(
		ctx, a.keys, c.GetHeader(authpkg.AccessKeyHeaderName), expectedMethod,
	)
	if err != nil {
		return nil, ErrBadAccessKey
	}
	unionID, err := serializer.HexUUIDv4FromString(userID)
	if err != nil {
		return nil, ErrBadAccessKey
	}
	user := core.User{}
	if err := a.db.WithContext(ctx).Take(&user, &core.User{UnionID: unionID}).Error; err != nil || user.ID == 0 {
		return nil, ErrBadAccessKey
	}
	return &user, nil
}
