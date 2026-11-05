// Package grpc 提供gRPC服务器和客户端的核心功能，包括认证、授权、配置管理等。
// 本文件定义了基于访问密钥(AccessKey)的gRPC认证器，用于验证请求的合法性。
package grpc

import (
	"context"

	"zhongkuisec.com/zhongkui/internal/pkg/auth"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// 编译期断言：确保 AccessKeyAuthenticator 实现了 Authenticator 接口
var _ Authenticator = (*AccessKeyAuthenticator)(nil)

// Authenticator 定义gRPC认证器接口，用于处理请求的身份验证。
// 不同的认证协议可以实现该接口，提供不同的认证逻辑。
// 当认证成功时返回用户ID，失败时返回相应的gRPC错误状态。
type Authenticator interface {
	// Handle 执行身份验证处理
	//
	// 参数：
	//   - ctx: 请求上下文，用于传递超时、取消信号和请求范围的元数据
	//   - method: gRPC方法的完整路径，格式为 "/package.Service/Method"
	//   - token: 认证令牌，格式取决于具体的认证协议实现
	//
	// 返回：
	//   - userID: 认证成功时返回的用户ID
	//   - err: 认证失败时返回的错误，使用gRPC状态码标识错误类型
	Handle(ctx context.Context, method, token string) (userID string, err error)
}

// AccessKeyAuthenticator 基于访问密钥(AccessKey)的认证器实现。
// 它通过验证请求中携带的签名令牌来确认请求者的身份。
//
// 令牌格式与校验流程由 internal/pkg/auth 的 AuthenticateAccessKey 统一实现，本类型只负责把
// 共享鉴权结果映射为 gRPC 状态码；Web 接口层的 X-Access-Key 鉴权复用同一份实现。
type AccessKeyAuthenticator struct {
	// pool 是访问密钥池，提供密钥查询、重放检测和访问记录等功能
	pool auth.AccessKeyPool
}

// NewAccessKeyAuthenticator 创建并返回一个新的基于访问密钥的认证器实例。
//
// 参数：
//   - pool: 访问密钥池接口，用于密钥的查询、重放检测和访问记录更新
//
// 返回：
//   - *AccessKeyAuthenticator: 新创建的认证器实例
func NewAccessKeyAuthenticator(pool auth.AccessKeyPool) *AccessKeyAuthenticator {
	return &AccessKeyAuthenticator{
		pool: pool,
	}
}

// Handle 执行基于访问密钥的身份验证处理。
//
// 认证令牌格式为：accessKeyID:timestamp:nonce:base64(method):signature
// 校验流程（解析、方法比对、防重放、密钥状态、签名校验、最后访问时间更新）统一由
// auth.AuthenticateAccessKey 完成；本方法仅将失败原因映射为既有 gRPC 错误码。
func (a *AccessKeyAuthenticator) Handle(ctx context.Context, method, token string) (string, error) {
	userID, err := auth.AuthenticateAccessKey(ctx, a.pool, token, method)
	if err == nil {
		return userID, nil
	}
	switch {
	case errors.Is(err, auth.ErrAccessKeyInternal):
		return "", ErrInternal
	case errors.Is(err, auth.ErrAccessKeyNonceDuplicated):
		return "", ErrNonceDuplicated
	case errors.Is(err, auth.ErrAccessKeyDisabled):
		return "", ErrAccessKeyDisabled
	case errors.Is(err, auth.ErrAccessKeyExpired):
		return "", ErrAccessKeyExpired
	default:
		return "", ErrInvalidToken
	}
}
