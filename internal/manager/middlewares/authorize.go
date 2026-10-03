package middlewares

import (
	"net/http"

	"github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/pkg/base"
)

// RbacPolicyCtxName 是 Gin 上下文中存储 RBAC 策略的键名
const RbacPolicyCtxName = "policy"

var (
	// ErrAuthorizeBlankFullPath 表示授权时路径为空错误
	ErrAuthorizeBlankFullPath = response.CreateCodeError(http.StatusForbidden, response.CodeAuthorizeBlankFullPath, nil)
	// ErrAuthorizeFailed 表示授权失败错误
	ErrAuthorizeFailed = response.CreateCodeError(http.StatusForbidden, response.CodeAuthorizeFailed, nil)
)

// RbacPolicyMiddleware 创建一个 Gin 中间件，将 RBAC 策略设置到 Gin 上下文中
// 参数 policy: RBAC 策略实例
// 返回: Gin 中间件函数
func RbacPolicyMiddleware(policy *casbin.Enforcer) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(RbacPolicyCtxName, policy)
		c.Next()
	}
}

// GetRbacPolicy 从 Gin 上下文中获取 RBAC 策略实例
// 参数 c: Gin 上下文
// 返回: RBAC 策略实例
// 注意: 如果上下文不存在 RBAC 策略，会触发 panic
func GetRbacPolicy(c *gin.Context) *casbin.Enforcer {
	if policy, ok := c.Get(RbacPolicyCtxName); ok {
		return policy.(*casbin.Enforcer)
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
func Authorize(c *gin.Context, span trace.Span) *response.CodeError {
	policy := GetRbacPolicy(c)
	attrs := []attribute.KeyValue{attribute.String("action", c.Request.Method)}
	namespace, err := GetNamespace(c)
	if err != nil {
		span.SetAttributes(attrs...)
		span.SetStatus(codes.Error, "namespace does not exist")
		return err
	}
	attrs = append(attrs, attribute.String("namespace", namespace))
	user, err := GetUser(c)
	if err != nil {
		span.SetAttributes(attrs...)
		span.SetStatus(codes.Error, "user does not exist")
		return err
	}
	attrs = append(attrs, attribute.String("user", user.UnionID.String()))
	// 接口层权限点 object = api:{FullPath}（规范见 internal/pkg/base/consts.go）
	path := c.FullPath()
	if path == "" {
		span.SetAttributes(attrs...)
		span.SetStatus(codes.Error, "object is blank")
		return ErrAuthorizeBlankFullPath
	}
	object := base.APIPrefix + ":" + path
	attrs = append(attrs, attribute.String("object", object))
	if ok, err := policy.Enforce(
		user.UnionID.String(), // 用户标识
		object,                // 资源路径
		c.Request.Method,      // HTTP 方法
		namespace,             // 命名空间
	); err != nil || !ok {
		span.SetAttributes(attrs...)
		span.SetStatus(codes.Error, "authorize failed")
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
		span.SetAttributes(attrs...)
		span.SetStatus(codes.Ok, "authorize success")
		return nil
	}
}

// StaticAuthorize 执行静态授权检查，使用指定的命名空间、对象和操作进行权限验证
// 参数 c: Gin 上下文
// 参数 namespace: 命名空间
// 参数 object: 资源对象（权限点 object）
// 参数 action: 操作类型（语义动作，如 READ / WRITE）
// 返回: 授权错误码，nil 表示授权成功
// 注意: 此函数用于固定资源的授权检查，不依赖于请求路径；
// 与 Casbin 模型 r = sub, obj, act, nsp 对应，Enforce 参数顺序为 (sub, obj, act, nsp)
func StaticAuthorize(c *gin.Context, span trace.Span, namespace, object, action string) *response.CodeError {
	attrs := []attribute.KeyValue{
		attribute.String("action", action),
		attribute.String("namespace", namespace),
		attribute.String("object", object),
	}
	policy := GetRbacPolicy(c)
	if user, err := GetUser(c); err != nil {
		span.SetAttributes(attrs...)
		span.SetStatus(codes.Error, "user does not exist")
		return err
	} else if ok, err := policy.Enforce(
		user.UnionID.String(), // 用户标识
		object,                // 资源对象（权限点 object）
		action,                // 操作类型（语义动作）
		namespace,             // 命名空间
	); err != nil {
		span.SetAttributes(attrs...)
		span.SetStatus(codes.Error, err.Error())
		return ErrAuthorizeFailed
	} else if !ok {
		span.SetAttributes(attrs...)
		span.SetStatus(codes.Error, "authorize failed")
		return ErrAuthorizeFailed
	} else {
		span.SetAttributes(attrs...)
		span.SetStatus(codes.Ok, "authorize success")
		return nil
	}
}

// RbacPolicyAuthorizeMiddleware 创建一个动态授权的 Gin 中间件
// 返回: Gin 中间件函数
// 功能: 对每个请求自动执行 Authorize 函数进行权限检查
func RbacPolicyAuthorizeMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		_, span := StartMiddlewareSpan(c, "authorize")
		defer span.End()
		if err := Authorize(c, span); err != nil {
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
		_, span := StartMiddlewareSpan(c, "static-authorize")
		defer span.End()
		if err := StaticAuthorize(c, span, namespace, object, action); err != nil {
			err.AbortWithGinH(c)
			return
		} else {
			c.Next()
		}
	}
}
