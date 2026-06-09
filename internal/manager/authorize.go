package manager

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"zhongkui.com/zhongkui/internal/depend"
)

// RBAC_POLICY_CTX_NAME 是 Gin 上下文中存储 RBAC 策略的键名
const RBAC_POLICY_CTX_NAME = "policy"

var (
	// ErrAuthorizeBlankFullPath 表示授权时路径为空错误
	ErrAuthorizeBlankFullPath = CreateCodeError(http.StatusForbidden, CodeAuthorizeBlankFullPath, nil)
	// ErrAuthorizeFailed 表示授权失败错误
	ErrAuthorizeFailed = CreateCodeError(http.StatusForbidden, CodeAuthorizeFailed, nil)
)

// RbacPolicyMiddleware 创建一个 Gin 中间件，将 RBAC 策略设置到 Gin 上下文中
// 参数 policy: RBAC 策略实例
// 返回: Gin 中间件函数
func RbacPolicyMiddleware(policy *depend.RbacPolicy) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(RBAC_POLICY_CTX_NAME, policy)
		c.Next()
	}
}

// GetRbacPolicy 从 Gin 上下文中获取 RBAC 策略实例
// 参数 c: Gin 上下文
// 返回: RBAC 策略实例
// 注意: 如果上下文不存在 RBAC 策略，会触发 panic
func GetRbacPolicy(c *gin.Context) *depend.RbacPolicy {
	if policy, ok := c.Get(RBAC_POLICY_CTX_NAME); ok {
		return policy.(*depend.RbacPolicy)
	}
	panic("rbac policy does not exists")
}

// Authorize 执行动态授权检查，基于当前请求的路径和方法进行权限验证
// 参数 c: Gin 上下文
// 返回: 授权错误码，nil 表示授权成功
// 流程:
// 1. 获取当前命名空间
// 2. 获取当前用户
// 3. 获取请求的完整路径作为资源对象
// 4. 使用 RBAC 策略验证用户是否有权访问该资源
func Authorize(c *gin.Context) *CodeError {
	policy := GetRbacPolicy(c)
	if namespace, err := GetNamespace(c); err != nil {
		return err
	} else if user, err := GetUser(c); err != nil {
		return err
	} else if object := c.FullPath(); object == "" {
		return ErrAuthorizeBlankFullPath
	} else if ok, err := policy.Enforce(
		user.UnionID.String(), // 用户标识
		object,                // 资源路径
		c.Request.Method,      // HTTP 方法
		namespace,             // 命名空间
	); err != nil || !ok {
		GetLogger(c).Infof(
			c,
			nil,
			"authorize failed wtih %s:%s:%s:%s, cause: %v",
			user.UnionID.String(),
			object,
			c.Request.Method,
			namespace,
			err,
		)
		return ErrAuthorizeFailed
	} else {
		return nil
	}
}

// StaticAuthorize 执行静态授权检查，使用指定的命名空间、对象和操作进行权限验证
// 参数 c: Gin 上下文
// 参数 namespace: 命名空间
// 参数 object: 资源对象
// 参数 action: 操作类型
// 返回: 授权错误码，nil 表示授权成功
// 注意: 此函数用于固定资源的授权检查，不依赖于请求路径
func StaticAuthorize(c *gin.Context, namespace, object, action string) *CodeError {
	policy := GetRbacPolicy(c)
	if user, err := GetUser(c); err != nil {
		return err
	} else if ok, err := policy.Enforce(
		user.UnionID.String(),
		namespace,
		object,
		action,
	); err != nil {
		// TODO: log
		return ErrAuthorizeFailed
	} else if !ok {
		return ErrAuthorizeFailed
	} else {
		return nil
	}
}

// RbacPolicyAuthorizeMiddleware 创建一个动态授权的 Gin 中间件
// 返回: Gin 中间件函数
// 功能: 对每个请求自动执行 Authorize 函数进行权限检查
func RbacPolicyAuthorizeMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := Authorize(c); err != nil {
			err.AbortWithGinH(c)
			return
		} else {
			c.Next()
		}
	}
}

// StaticRbacPolicyAuthorizeMiddleware 创建一个静态授权的 Gin 中间件
// 参数 namespace: 命名空间
// 参数 object: 资源对象
// 参数 action: 操作类型
// 返回: Gin 中间件函数
// 功能: 对每个请求自动执行 StaticAuthorize 函数进行固定资源的权限检查
func StaticRbacPolicyAuthorizeMiddleware(namespace, object, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := StaticAuthorize(c, namespace, object, action); err != nil {
			err.AbortWithGinH(c)
			return
		} else {
			c.Next()
		}
	}
}
