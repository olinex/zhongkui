// Package auth（本文件）定义访问密钥（Access Key）及其存储池的抽象接口，并给出访问密钥签名令牌的
// 共享鉴权实现：令牌解析/签发、待签名原文构造、密钥状态与签名校验。
//
// gRPC 服务与 Web 接口层（X-Access-Key 请求头）共用这里的同一份实现，禁止在各自的认证器里重复
// 解析令牌、校验密钥状态或拼装签名原文，避免两套鉴权逻辑产生语义漂移。
//
// 令牌格式与 gRPC 既有协议一致：
//
//	accessKeyID:timestampMs:nonce:base64(method):signature
//
// 待签名原文为 timestampMs/nonce/method/body，其中 body 当前为空字符串（与 gRPC 协议一致）。
package auth

import (
	// 标准库
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/crypto"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto/signature"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

const (
	// AccessKeyHeaderName 是 Web 接口层携带访问密钥签名令牌的请求头名称。
	AccessKeyHeaderName = "X-Access-Key"
	// accessKeyTokenParts 是访问密钥令牌按冒号分隔后的固定段数：
	// accessKeyID:timestampMs:nonce:base64(method):signature。签名值内部含冒号，故解析必须限制段数。
	accessKeyTokenParts = 5
	// AccessKeyNonceLength 是签发令牌时自动生成的 nonce 长度（字母数字随机串）。
	AccessKeyNonceLength = 16
)

// 访问密钥鉴权失败的原因哨兵。调用方（gRPC / Web 中间件）按业务语义映射为自己的错误码。
var (
	// ErrAccessKeyTokenMalformed 表示令牌格式非法（段数、时间戳或 method 编码不合法）。
	ErrAccessKeyTokenMalformed = errors.New("malformed access key token")
	// ErrAccessKeyMethodMismatch 表示令牌声明的方法与实际请求方法不一致。
	ErrAccessKeyMethodMismatch = errors.New("access key token method mismatch")
	// ErrAccessKeyNotFound 表示令牌对应的访问密钥不存在。
	ErrAccessKeyNotFound = errors.New("access key not found")
	// ErrAccessKeyDisabled 表示访问密钥已被停用。
	ErrAccessKeyDisabled = errors.New("access key is disabled")
	// ErrAccessKeyExpired 表示访问密钥在请求时间戳之前已过期。
	ErrAccessKeyExpired = errors.New("access key is expired")
	// ErrAccessKeyNonceDuplicated 表示 nonce 已被使用，疑似重放攻击。
	ErrAccessKeyNonceDuplicated = errors.New("access key nonce duplicated")
	// ErrAccessKeySignatureInvalid 表示签名与请求内容不匹配。
	ErrAccessKeySignatureInvalid = errors.New("access key signature invalid")
	// ErrAccessKeyInternal 表示密钥池查询、防重放或状态更新等基础设施失败。
	ErrAccessKeyInternal = errors.New("access key authentication internal error")
)

// AccessKey 表示单个访问密钥的只读视图。
type AccessKey interface {
	// UserID 返回密钥所属用户标识。
	UserID() string
	// IsEnabled 返回密钥是否处于启用状态。
	IsEnabled() bool
	// Secret 返回用于签名校验的密钥明文。
	Secret() string
	// ExpiredAt 返回密钥的过期时间，未设置过期时间时返回 nil。
	ExpiredAt() *time.Time
}

// AccessKeyPool 定义访问密钥的查询与状态维护能力。
type AccessKeyPool interface {
	// GetKey 根据密钥 ID 查询对应的访问密钥，不存在时返回 nil。
	GetKey(ctx context.Context, id string) (key AccessKey, err error)
	// IsDuplicated 判断给定密钥 ID 与时间戳、随机串组合是否已使用过。
	IsDuplicated(ctx context.Context, id string, timestamp time.Time, nonce string) (bool, error)
	// UpdateLastAccess 更新密钥的最后访问时间。
	UpdateLastAccess(ctx context.Context, id string) error
}

// AccessKeyToken 是访问密钥签名令牌的解析结果，格式与 gRPC 协议一致。
type AccessKeyToken struct {
	// AccessKeyID 是访问密钥的唯一标识。
	AccessKeyID string
	// TimestampMs 是请求时间戳（毫秒）。
	TimestampMs int64
	// Nonce 是请求随机串，用于防重放。
	Nonce string
	// Method 是从 base64 段解码出的方法标识：gRPC 为完整方法名，Web 为 "METHOD path"。
	Method string
	// Signature 是签名值，格式为 "hmac-sha256:<base64>"。
	Signature string
}

// String 将令牌格式化为请求头取值形式。
func (t *AccessKeyToken) String() string {
	return fmt.Sprintf(
		"%s:%d:%s:%s:%s",
		t.AccessKeyID,
		t.TimestampMs,
		t.Nonce,
		base64.URLEncoding.EncodeToString([]byte(t.Method)),
		t.Signature,
	)
}

// ParseAccessKeyToken 解析访问密钥签名令牌。
// 签名值内部含冒号，因此使用 strings.SplitN(raw, ":", 5)，仅切出固定五段。
func ParseAccessKeyToken(raw string) (*AccessKeyToken, error) {
	parts := strings.SplitN(strings.TrimSpace(raw), ":", accessKeyTokenParts)
	if len(parts) != accessKeyTokenParts {
		return nil, ErrAccessKeyTokenMalformed
	}
	accessKeyID, timestampStr, nonce, encodedMethod, sig := parts[0], parts[1], parts[2], parts[3], parts[4]
	if accessKeyID == "" || encodedMethod == "" || sig == "" {
		return nil, ErrAccessKeyTokenMalformed
	}
	method, err := base64.URLEncoding.DecodeString(encodedMethod)
	if err != nil {
		return nil, ErrAccessKeyTokenMalformed
	}
	timestampMs, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return nil, ErrAccessKeyTokenMalformed
	}
	return &AccessKeyToken{
		AccessKeyID: accessKeyID,
		TimestampMs: timestampMs,
		Nonce:       nonce,
		Method:      string(method),
		Signature:   sig,
	}, nil
}

// NewAccessKeyToken 为一次请求签发访问密钥签名令牌（含自动生成 nonce）。
// method 是方法标识：gRPC 传完整方法名，Web 用 HTTPRequestMethod 生成。
func NewAccessKeyToken(secret, accessKeyID, method string, timestampMs int64) (*AccessKeyToken, error) {
	nonce, err := crypto.GenerateRandomString(AccessKeyNonceLength)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	sig, err := signature.NewHMACSHA256Signaturer(secret).Sign(
		accessKeySignPayload(timestampMs, nonce, method),
	)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &AccessKeyToken{
		AccessKeyID: accessKeyID,
		TimestampMs: timestampMs,
		Nonce:       nonce,
		Method:      method,
		Signature:   sig,
	}, nil
}

// HTTPRequestMethod 返回 Web 接口层令牌使用的方法标识，把 HTTP 方法与路径绑定在一起，
// 避免签名被复用到其他接口。
func HTTPRequestMethod(method, path string) string {
	return method + " " + path
}

// SignHTTPAccessKey 为一次 Web 接口调用签发 X-Access-Key 请求头取值。
// 与 gRPC 的 AccessKeySecretPerRPCCredentials 共用同一令牌格式与签名原文。
func SignHTTPAccessKey(secret, accessKeyID, httpMethod, path string, timestampMs int64) (string, error) {
	token, err := NewAccessKeyToken(secret, accessKeyID, HTTPRequestMethod(httpMethod, path), timestampMs)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return token.String(), nil
}

// accessKeySignPayload 构造待签名原文，与 gRPC 既有协议一致：timestampMs/nonce/method/body。
// body 当前恒为空字符串（与 gRPC 客户端实现保持一致）。
func accessKeySignPayload(timestampMs int64, nonce, method string) string {
	return fmt.Sprintf("%s/%s/%s/%s", strconv.FormatInt(timestampMs, 10), nonce, method, "")
}

// VerifyAccessKeySignature 校验令牌签名与令牌内容是否匹配。
func VerifyAccessKeySignature(secret string, token *AccessKeyToken) (bool, error) {
	if token == nil {
		return false, ErrAccessKeyTokenMalformed
	}
	return signature.NewHMACSHA256Signaturer(secret).Check(
		token.Signature,
		accessKeySignPayload(token.TimestampMs, token.Nonce, token.Method),
	)
}

// AuthenticateAccessKey 执行访问密钥签名鉴权的完整校验流程，gRPC 与 Web 接口层共用。
//
// 流程与 gRPC 既有实现一致：解析令牌 → 校验方法标识 → 防重放（时间戳窗口 + nonce）→ 查询密钥 →
// 启用 / 过期状态 → 签名校验 → 更新最后访问时间 → 返回密钥所属用户标识。
//
// 参数：
//   - ctx: 请求上下文
//   - pool: 访问密钥池
//   - rawToken: 请求携带的签名令牌原文
//   - expectedMethod: 期望的方法标识（gRPC 完整方法名 / Web 的 "METHOD path"）
//
// 返回值：
//   - string: 认证成功时返回密钥所属用户标识
//   - error: 失败时返回上面的原因哨兵，调用方据此映射具体错误码
func AuthenticateAccessKey(
	ctx context.Context,
	pool AccessKeyPool,
	rawToken, expectedMethod string,
) (string, error) {
	if pool == nil {
		return "", ErrAccessKeyInternal
	}
	token, err := ParseAccessKeyToken(rawToken)
	if err != nil {
		return "", err
	}
	if token.Method != expectedMethod {
		return "", ErrAccessKeyMethodMismatch
	}
	requestTime := time.UnixMilli(token.TimestampMs)
	if duplicated, err := pool.IsDuplicated(ctx, token.AccessKeyID, requestTime, token.Nonce); err != nil {
		return "", ErrAccessKeyInternal
	} else if duplicated {
		return "", ErrAccessKeyNonceDuplicated
	}
	key, err := pool.GetKey(ctx, token.AccessKeyID)
	if err != nil {
		return "", ErrAccessKeyInternal
	}
	if key == nil {
		return "", ErrAccessKeyNotFound
	}
	if !key.IsEnabled() {
		return "", ErrAccessKeyDisabled
	}
	if expiredAt := key.ExpiredAt(); expiredAt != nil && expiredAt.Before(requestTime) {
		return "", ErrAccessKeyExpired
	}
	valid, err := VerifyAccessKeySignature(key.Secret(), token)
	if err != nil {
		return "", ErrAccessKeyInternal
	}
	if !valid {
		return "", ErrAccessKeySignatureInvalid
	}
	if err := pool.UpdateLastAccess(ctx, token.AccessKeyID); err != nil {
		return "", ErrAccessKeyInternal
	}
	return key.UserID(), nil
}
