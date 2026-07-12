package grpc

// 导入必要的包
import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"time"

	"google.golang.org/grpc/credentials"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/crypto/signature"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// AccessKeySecretPerRPCCredentials 实现了 gRPC 的 PerRPCCredentials 接口
// 用于基于访问密钥和密钥进行身份验证
type AccessKeySecretPerRPCCredentials struct {
	AccessKeyID     string // 访问密钥 ID
	AccessKeySecret string // 访问密钥
}

// NewAccessKeySecretPerRPCCredentials 创建并返回一个新的 AccessKeySecretPerRPCCredentials 实例
// 参数:
//   - id: 访问密钥 ID
//   - secret: 访问密钥
//
// 返回值: *AccessKeySecretPerRPCCredentials 实例
func NewAccessKeySecretPerRPCCredentials(id, secret string) *AccessKeySecretPerRPCCredentials {
	return &AccessKeySecretPerRPCCredentials{
		AccessKeyID:     id,
		AccessKeySecret: secret,
	}
}

// GetRequestMetadata 实现 PerRPCCredentials 接口，生成请求的认证元数据
// 该方法会生成包含时间戳、随机数、完整方法名和签名的认证头
// 认证头格式: AccessKeyID:timestamp:nonce:base64(fullMethod):signature
// 参数:
//   - ctx: 上下文，需要包含 "method" 键值对
//   - uri: gRPC 服务 URI
//
// 返回值:
//   - map[string]string: 包含认证头的元数据
//   - error: 错误信息，如果生成过程中出现错误
func (pc *AccessKeySecretPerRPCCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	// 获取当前 UTC 时间戳（毫秒）
	timestamp := time.Now().UTC().UnixMilli()

	// 生成 16 位随机数作为 nonce，防止重放攻击
	nonce, err := crypto.GenerateRandomString(16)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// 解析 URI 获取服务信息
	service, err := url.Parse(uri[0])
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// 从上下文中获取方法名
	method, ok := ctx.Value("method").(string)
	if !ok {
		return nil, errors.New("method not found in context")
	}

	// 构建完整的 gRPC 方法名（协议://主机/方法）
	fullMethod := fmt.Sprintf(`%s://%s%s`, service.Scheme, service.Host, method)

	// 创建 HMAC-SHA256 签名器
	signaturer := signature.NewHMACSHA256Signaturer(pc.AccessKeySecret)

	// TODO: 需要请求体加入签名
	// 构建待签名的原始数据：时间戳/随机数/完整方法名/请求体（目前为空）
	raw := fmt.Sprintf("%d/%s/%s/%s", timestamp, nonce, fullMethod, "")

	// 生成签名
	signature, err := signaturer.Sign(raw)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// 返回认证元数据
	return map[string]string{
		MetadataKeyAuthorization: fmt.Sprintf(
			"%s:%d:%s:%s:%s",
			pc.AccessKeyID,
			timestamp,
			nonce,
			base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString([]byte(fullMethod)),
			signature,
		),
	}, nil
}

// RequireTransportSecurity 实现 PerRPCCredentials 接口
// 返回 true 表示要求使用 TLS 传输安全
// 对于访问密钥认证，必须使用安全传输以防止密钥泄露
func (pc *AccessKeySecretPerRPCCredentials) RequireTransportSecurity() bool {
	return true
}

// AuthWrapperPerRPCCredentials 是一个包装器，用于在现有 PerRPCCredentials 基础上添加认证协议标识
// 主要用于在认证头中添加协议类型信息，便于服务器端识别认证方式
type AuthWrapperPerRPCCredentials struct {
	protocol string                        // 认证协议标识
	inner    credentials.PerRPCCredentials // 被包装的原始 PerRPCCredentials 实例
}

// NewAuthWrapperPerRPCCredentials 创建并返回一个新的 AuthWrapperPerRPCCredentials 实例
// 参数:
//   - protocol: 认证协议标识（如 "accesskeySecret"）
//   - inner: 被包装的原始 PerRPCCredentials 实例
//
// 返回值: *AuthWrapperPerRPCCredentials 实例
func NewAuthWrapperPerRPCCredentials(protocol string, inner credentials.PerRPCCredentials) *AuthWrapperPerRPCCredentials {
	return &AuthWrapperPerRPCCredentials{
		protocol: protocol,
		inner:    inner,
	}
}

// GetRequestMetadata 实现 PerRPCCredentials 接口，生成请求的认证元数据
// 该方法首先调用被包装实例的 GetRequestMetadata，然后在返回的元数据中添加协议标识
// 参数:
//   - ctx: 上下文
//   - uri: gRPC 服务 URI
//
// 返回值:
//   - map[string]string: 包含认证头和协议标识的元数据
//   - error: 错误信息，如果生成过程中出现错误
func (a *AuthWrapperPerRPCCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	// 如果内部凭证为nil，返回错误
	if a.inner == nil {
		return nil, errors.New("inner credentials is nil")
	}

	// 调用被包装实例的 GetRequestMetadata 方法
	md, err := a.inner.GetRequestMetadata(ctx, uri...)
	if err != nil {
		return md, errors.WithStack(err)
	}

	// 在元数据中添加协议标识，便于服务器端识别认证方式
	md[MetadataKeyAuthorize] = a.protocol
	return md, nil
}

// RequireTransportSecurity 实现 PerRPCCredentials 接口
// 返回被包装实例的传输安全要求
// 包装器不改变原始实例的传输安全要求
func (a *AuthWrapperPerRPCCredentials) RequireTransportSecurity() bool {
	// 如果内部凭证为nil，默认返回true要求传输安全
	if a.inner == nil {
		return true
	}
	return a.inner.RequireTransportSecurity()
}
