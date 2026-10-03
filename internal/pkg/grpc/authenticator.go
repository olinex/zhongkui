// Package grpc 提供gRPC服务器和客户端的核心功能，包括认证、授权、配置管理等。
// 本文件定义了基于访问密钥(AccessKey)的gRPC认证器，用于验证请求的合法性。
package grpc

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"zhongkuisec.com/zhongkui/internal/pkg/auth"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto/signature"
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
// 认证令牌格式为：accessKeyID:timestamp:nonce:base64(method):signature
// 其中：
//   - accessKeyID: 访问密钥ID，用于从密钥池中查找对应的密钥信息
//   - timestamp: 请求时间戳（毫秒），用于防重放攻击和过期检查
//   - nonce: 随机字符串，防止同一请求被重复提交
//   - base64(method): 经URL安全Base64编码的gRPC方法名
//   - signature: HMAC-SHA256签名，格式为 "hmac-sha256:<base64签名值>"
//
// 签名的原始数据格式为：timestamp/nonce/method/body（当前body为空字符串）
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
// 验证流程如下：
//  1. 解析令牌：将令牌按冒号分隔为5部分，校验格式是否正确
//  2. 验证方法名：解码Base64编码的方法名，与实际调用的gRPC方法比对
//  3. 解析时间戳：将时间戳字符串解析为整数
//  4. 防重放检测：通过密钥池检查nonce是否已被使用过（重放攻击防护）
//  5. 查询密钥：根据accessKeyID从密钥池中获取对应的密钥信息
//  6. 密钥状态检查：验证密钥是否存在、是否启用、是否已过期
//  7. 签名验证：使用密钥的Secret生成HMAC-SHA256签名，与请求中的签名比对
//  8. 更新访问记录：认证成功后更新密钥的最后访问时间
//
// 参数：
//   - ctx: 请求上下文
//   - method: 实际调用的gRPC方法完整路径
//   - token: 认证令牌，格式为 accessKeyID:timestamp:nonce:base64(method):signature
//
// 返回：
//   - userID: 认证成功时返回密钥所属的用户ID
//   - err: 认证失败时返回的gRPC错误，可能的错误包括：
//   - ErrInvalidToken: 令牌格式错误、方法名不匹配、密钥不存在、签名不匹配
//   - ErrNonceDuplicated: nonce重复，疑似重放攻击
//   - ErrAccessKeyDisabled: 访问密钥已被禁用
//   - ErrAccessKeyExpired: 访问密钥已过期
//   - ErrInternal: 内部错误（如密钥池操作失败）
func (a *AccessKeyAuthenticator) Handle(ctx context.Context, method, token string) (string, error) {
	// 第1步：解析令牌，按冒号分隔为5部分
	parts := strings.SplitN(token, ":", 5)
	if len(parts) != 5 {
		return "", ErrInvalidToken
	}
	accessKeyID, timestampStr, nonce, base64Method, rawSignature := parts[0], parts[1], parts[2], parts[3], parts[4]

	// 第2步：验证方法名，解码Base64编码的方法名并与实际方法比对
	if rawMethod, err := base64.URLEncoding.DecodeString(base64Method); err != nil {
		return "", ErrInvalidToken
	} else if method != string(rawMethod) {
		// 方法名不匹配，令牌可能被篡改或用于错误的接口
		return "", ErrInvalidToken
		// 第3步：解析时间戳
	} else if timestampInt, err := strconv.ParseInt(timestampStr, 10, 64); err != nil {
		return "", ErrInvalidToken
		// 第4步：防重放检测，检查nonce是否已被使用
	} else if duplicated, err := a.pool.IsDuplicated(ctx, accessKeyID, time.UnixMilli(timestampInt), nonce); err != nil {
		return "", ErrInternal
	} else if duplicated {
		// nonce重复，可能是重放攻击
		return "", ErrNonceDuplicated
		// 第5步：根据accessKeyID查询密钥信息
	} else if key, err := a.pool.GetKey(ctx, accessKeyID); err != nil {
		return "", ErrInternal
	} else if key == nil {
		// 密钥不存在
		return "", ErrInvalidToken
		// 第6步：检查密钥状态
	} else if !key.IsEnabled() {
		// 密钥已被禁用
		return "", ErrAccessKeyDisabled
	} else if expiredAt := key.ExpiredAt(); expiredAt != nil && expiredAt.Before(time.UnixMilli(timestampInt)) {
		// 密钥在请求时间戳之前已过期
		return "", ErrAccessKeyExpired
	} else {
		// 第7步：签名验证
		signaturer := signature.NewHMACSHA256Signaturer(key.Secret())
		// TODO: 需要请求体加入签名
		// 构建待签名的原始数据，格式为：timestamp/nonce/method/body
		// 当前body部分为空字符串，后续需要将请求体加入签名以增强安全性
		raw := fmt.Sprintf("%s/%s/%s/%s", timestampStr, nonce, method, "")
		signature, err := signaturer.Sign(raw)
		if err != nil {
			return "", errors.WithStack(err)
		}
		if signature != rawSignature {
			// 签名不匹配，令牌可能被篡改
			return "", ErrInvalidToken
		}
		// 第8步：更新密钥的最后访问时间
		if err := a.pool.UpdateLastAccess(ctx, accessKeyID); err != nil {
			return "", ErrInternal
		}
		return key.UserID(), nil
	}
}
