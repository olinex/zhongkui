package grpc

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"zhongkui.com/zhongkui/internal/pkg/auth"
	"zhongkui.com/zhongkui/internal/pkg/crypto/signature"
)

// mockAccessKey 是 auth.AccessKey 接口的模拟实现，用于单元测试
type mockAccessKey struct {
	userID    string
	enabled   bool
	secret    string
	expiredAt *time.Time
}

func (m *mockAccessKey) UserID() string        { return m.userID }
func (m *mockAccessKey) IsEnabled() bool       { return m.enabled }
func (m *mockAccessKey) Secret() string        { return m.secret }
func (m *mockAccessKey) ExpiredAt() *time.Time { return m.expiredAt }

// mockAccessKeyPool 是 auth.AccessKeyPool 接口的模拟实现，用于单元测试
type mockAccessKeyPool struct {
	key             *mockAccessKey
	getKeyErr       error
	isDuplicated    bool
	isDuplicatedErr error
	updateLastErr   error
}

func (m *mockAccessKeyPool) GetKey(_ context.Context, _ string) (auth.AccessKey, error) {
	if m.getKeyErr != nil {
		return nil, m.getKeyErr
	}
	if m.key == nil {
		return nil, nil
	}
	return m.key, nil
}

func (m *mockAccessKeyPool) IsDuplicated(_ context.Context, _ string, _ time.Time, _ string) (bool, error) {
	return m.isDuplicated, m.isDuplicatedErr
}

func (m *mockAccessKeyPool) UpdateLastAccess(_ context.Context, _ string) error {
	return m.updateLastErr
}

// buildToken 构建认证令牌，格式为 accessKeyID:timestamp:nonce:base64(method):signature
func buildToken(accessKeyID, secret, method string, timestamp int64, nonce string) string {
	timestampStr := strconv.FormatInt(timestamp, 10)
	base64Method := base64.URLEncoding.EncodeToString([]byte(method))
	signaturer := signature.NewHMACSHA256Signaturer(secret)
	raw := fmt.Sprintf("%s/%s/%s/%s", timestampStr, nonce, method, "")
	signature, _ := signaturer.Sign(raw)
	return fmt.Sprintf("%s:%s:%s:%s:%s", accessKeyID, timestampStr, nonce, base64Method, signature)
}

func TestNewAccessKeyAuthenticator(t *testing.T) {
	Convey("TestNewAccessKeyAuthenticator", t, func() {
		Convey("应该成功创建认证器实例", func() {
			pool := &mockAccessKeyPool{}
			authenticator := NewAccessKeyAuthenticator(pool)
			So(authenticator, ShouldNotBeNil)
			So(authenticator.pool, ShouldEqual, pool)
		})
	})
}

func TestAccessKeyAuthenticator_Handle_Success(t *testing.T) {
	Convey("TestAccessKeyAuthenticator_Handle_Success", t, func() {
		now := time.Now().UTC()
		expiredAt := now.Add(24 * time.Hour)
		key := &mockAccessKey{
			userID:    "user-123",
			enabled:   true,
			secret:    "test-secret",
			expiredAt: &expiredAt,
		}
		pool := &mockAccessKeyPool{key: key}
		authenticator := NewAccessKeyAuthenticator(pool)
		method := "/test.Service/Method"

		Convey("当提供有效的令牌时，应该认证成功并返回用户ID", func() {
			token := buildToken("ak-001", "test-secret", method, now.UnixMilli(), "random-nonce")
			userID, err := authenticator.Handle(context.Background(), method, token)
			So(err, ShouldBeNil)
			So(userID, ShouldEqual, "user-123")
		})

		Convey("当密钥没有设置过期时间时，也应该认证成功", func() {
			keyNoExpiry := &mockAccessKey{
				userID:    "user-456",
				enabled:   true,
				secret:    "another-secret",
				expiredAt: nil,
			}
			poolNoExpiry := &mockAccessKeyPool{key: keyNoExpiry}
			authNoExpiry := NewAccessKeyAuthenticator(poolNoExpiry)
			token := buildToken("ak-002", "another-secret", method, now.UnixMilli(), "nonce-abc")
			userID, err := authNoExpiry.Handle(context.Background(), method, token)
			So(err, ShouldBeNil)
			So(userID, ShouldEqual, "user-456")
		})

		Convey("当密钥的过期时间在请求时间戳之后时，应该认证成功", func() {
			token := buildToken("ak-001", "test-secret", method, now.UnixMilli(), "nonce-future")
			userID, err := authenticator.Handle(context.Background(), method, token)
			So(err, ShouldBeNil)
			So(userID, ShouldEqual, "user-123")
		})
	})
}

func TestAccessKeyAuthenticator_Handle_InvalidTokenFormat(t *testing.T) {
	Convey("TestAccessKeyAuthenticator_Handle_InvalidTokenFormat", t, func() {
		pool := &mockAccessKeyPool{}
		authenticator := NewAccessKeyAuthenticator(pool)

		Convey("当令牌为空字符串时，应该返回ErrInvalidToken", func() {
			_, err := authenticator.Handle(context.Background(), "/test.Service/Method", "")
			So(err, ShouldNotBeNil)
			So(status.Code(err), ShouldEqual, codes.Unauthenticated)
		})

		Convey("当令牌缺少部分字段时，应该返回ErrInvalidToken", func() {
			_, err := authenticator.Handle(context.Background(), "/test.Service/Method", "ak:123:nonce")
			So(err, ShouldNotBeNil)
			So(status.Code(err), ShouldEqual, codes.Unauthenticated)
		})

		Convey("当令牌有过多部分时，应该返回ErrInvalidToken", func() {
			_, err := authenticator.Handle(context.Background(), "/test.Service/Method", "a:b:c:d:e:f")
			So(err, ShouldNotBeNil)
			So(status.Code(err), ShouldEqual, codes.Unauthenticated)
		})
	})
}

func TestAccessKeyAuthenticator_Handle_InvalidBase64Method(t *testing.T) {
	Convey("TestAccessKeyAuthenticator_Handle_InvalidBase64Method", t, func() {
		pool := &mockAccessKeyPool{}
		authenticator := NewAccessKeyAuthenticator(pool)

		Convey("当Base64编码的方法名无效时，应该返回ErrInvalidToken", func() {
			token := "ak-001:1234567890:nonce:!!!invalid-base64!!!:signature"
			_, err := authenticator.Handle(context.Background(), "/test.Service/Method", token)
			So(err, ShouldNotBeNil)
			So(status.Code(err), ShouldEqual, codes.Unauthenticated)
		})
	})
}

func TestAccessKeyAuthenticator_Handle_MethodMismatch(t *testing.T) {
	Convey("TestAccessKeyAuthenticator_Handle_MethodMismatch", t, func() {
		key := &mockAccessKey{
			userID:  "user-123",
			enabled: true,
			secret:  "test-secret",
		}
		pool := &mockAccessKeyPool{key: key}
		authenticator := NewAccessKeyAuthenticator(pool)

		Convey("当令牌中的方法名与实际调用的方法名不匹配时，应该返回ErrInvalidToken", func() {
			tokenMethod := "/test.Service/Get"
			actualMethod := "/test.Service/Create"
			token := buildToken("ak-001", "test-secret", tokenMethod, time.Now().UTC().UnixMilli(), "nonce-xyz")
			_, err := authenticator.Handle(context.Background(), actualMethod, token)
			So(err, ShouldNotBeNil)
			So(status.Code(err), ShouldEqual, codes.Unauthenticated)
		})
	})
}

func TestAccessKeyAuthenticator_Handle_InvalidTimestamp(t *testing.T) {
	Convey("TestAccessKeyAuthenticator_Handle_InvalidTimestamp", t, func() {
		key := &mockAccessKey{
			userID:  "user-123",
			enabled: true,
			secret:  "test-secret",
		}
		pool := &mockAccessKeyPool{key: key}
		authenticator := NewAccessKeyAuthenticator(pool)
		method := "/test.Service/Method"
		base64Method := base64.URLEncoding.EncodeToString([]byte(method))

		Convey("当时间戳不是有效整数时，应该返回ErrInvalidToken", func() {
			token := fmt.Sprintf("ak-001:not-a-number:nonce:%s:signature", base64Method)
			_, err := authenticator.Handle(context.Background(), method, token)
			So(err, ShouldNotBeNil)
			So(status.Code(err), ShouldEqual, codes.Unauthenticated)
		})
	})
}

func TestAccessKeyAuthenticator_Handle_NonceDuplicated(t *testing.T) {
	Convey("TestAccessKeyAuthenticator_Handle_NonceDuplicated", t, func() {
		pool := &mockAccessKeyPool{
			isDuplicated: true,
			key: &mockAccessKey{
				userID:  "user-123",
				enabled: true,
				secret:  "test-secret",
			},
		}
		authenticator := NewAccessKeyAuthenticator(pool)
		method := "/test.Service/Method"

		Convey("当nonce已被使用时，应该返回ErrNonceDuplicated", func() {
			token := buildToken("ak-001", "test-secret", method, time.Now().UTC().UnixMilli(), "reused-nonce")
			_, err := authenticator.Handle(context.Background(), method, token)
			So(err, ShouldNotBeNil)
			So(status.Code(err), ShouldEqual, codes.Unauthenticated)
		})
	})
}

func TestAccessKeyAuthenticator_Handle_IsDuplicatedError(t *testing.T) {
	Convey("TestAccessKeyAuthenticator_Handle_IsDuplicatedError", t, func() {
		pool := &mockAccessKeyPool{
			isDuplicatedErr: fmt.Errorf("database error"),
		}
		authenticator := NewAccessKeyAuthenticator(pool)
		method := "/test.Service/Method"

		Convey("当nonce重复检查发生内部错误时，应该返回ErrInternal", func() {
			token := buildToken("ak-001", "test-secret", method, time.Now().UTC().UnixMilli(), "nonce")
			_, err := authenticator.Handle(context.Background(), method, token)
			So(err, ShouldNotBeNil)
			So(status.Code(err), ShouldEqual, codes.Internal)
		})
	})
}

func TestAccessKeyAuthenticator_Handle_GetKeyError(t *testing.T) {
	Convey("TestAccessKeyAuthenticator_Handle_GetKeyError", t, func() {
		pool := &mockAccessKeyPool{
			getKeyErr: fmt.Errorf("database error"),
		}
		authenticator := NewAccessKeyAuthenticator(pool)
		method := "/test.Service/Method"

		Convey("当查询密钥发生内部错误时，应该返回ErrInternal", func() {
			token := buildToken("ak-001", "test-secret", method, time.Now().UTC().UnixMilli(), "nonce")
			_, err := authenticator.Handle(context.Background(), method, token)
			So(err, ShouldNotBeNil)
			So(status.Code(err), ShouldEqual, codes.Internal)
		})
	})
}

func TestAccessKeyAuthenticator_Handle_KeyNotFound(t *testing.T) {
	Convey("TestAccessKeyAuthenticator_Handle_KeyNotFound", t, func() {
		pool := &mockAccessKeyPool{key: nil}
		authenticator := NewAccessKeyAuthenticator(pool)
		method := "/test.Service/Method"

		Convey("当密钥不存在时，应该返回ErrInvalidToken", func() {
			token := buildToken("ak-001", "test-secret", method, time.Now().UTC().UnixMilli(), "nonce")
			_, err := authenticator.Handle(context.Background(), method, token)
			So(err, ShouldNotBeNil)
			So(status.Code(err), ShouldEqual, codes.Unauthenticated)
		})
	})
}

func TestAccessKeyAuthenticator_Handle_KeyDisabled(t *testing.T) {
	Convey("TestAccessKeyAuthenticator_Handle_KeyDisabled", t, func() {
		key := &mockAccessKey{
			userID:  "user-123",
			enabled: false,
			secret:  "test-secret",
		}
		pool := &mockAccessKeyPool{key: key}
		authenticator := NewAccessKeyAuthenticator(pool)
		method := "/test.Service/Method"

		Convey("当密钥已被禁用时，应该返回ErrAccessKeyDisabled", func() {
			token := buildToken("ak-001", "test-secret", method, time.Now().UTC().UnixMilli(), "nonce")
			_, err := authenticator.Handle(context.Background(), method, token)
			So(err, ShouldNotBeNil)
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
		})
	})
}

func TestAccessKeyAuthenticator_Handle_KeyExpired(t *testing.T) {
	Convey("TestAccessKeyAuthenticator_Handle_KeyExpired", t, func() {
		now := time.Now().UTC()
		expiredAt := now.Add(-1 * time.Hour)
		key := &mockAccessKey{
			userID:    "user-123",
			enabled:   true,
			secret:    "test-secret",
			expiredAt: &expiredAt,
		}
		pool := &mockAccessKeyPool{key: key}
		authenticator := NewAccessKeyAuthenticator(pool)
		method := "/test.Service/Method"

		Convey("当密钥已过期（过期时间在请求时间戳之前）时，应该返回ErrAccessKeyExpired", func() {
			token := buildToken("ak-001", "test-secret", method, now.UnixMilli(), "nonce")
			_, err := authenticator.Handle(context.Background(), method, token)
			So(err, ShouldNotBeNil)
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
		})

		Convey("当密钥过期时间在请求时间戳之后时，应该认证成功", func() {
			futureExpiredAt := now.Add(1 * time.Hour)
			keyNotExpired := &mockAccessKey{
				userID:    "user-123",
				enabled:   true,
				secret:    "test-secret",
				expiredAt: &futureExpiredAt,
			}
			poolNotExpired := &mockAccessKeyPool{key: keyNotExpired}
			authNotExpired := NewAccessKeyAuthenticator(poolNotExpired)
			token := buildToken("ak-001", "test-secret", method, now.UnixMilli(), "nonce")
			userID, err := authNotExpired.Handle(context.Background(), method, token)
			So(err, ShouldBeNil)
			So(userID, ShouldEqual, "user-123")
		})
	})
}

func TestAccessKeyAuthenticator_Handle_InvalidSignature(t *testing.T) {
	Convey("TestAccessKeyAuthenticator_Handle_InvalidSignature", t, func() {
		key := &mockAccessKey{
			userID:  "user-123",
			enabled: true,
			secret:  "test-secret",
		}
		pool := &mockAccessKeyPool{key: key}
		authenticator := NewAccessKeyAuthenticator(pool)
		method := "/test.Service/Method"
		now := time.Now().UTC()

		Convey("当使用错误的密钥生成签名时，应该返回ErrInvalidToken", func() {
			token := buildToken("ak-001", "wrong-secret", method, now.UnixMilli(), "nonce")
			_, err := authenticator.Handle(context.Background(), method, token)
			So(err, ShouldNotBeNil)
			So(status.Code(err), ShouldEqual, codes.Unauthenticated)
		})

		Convey("当签名被篡改时，应该返回ErrInvalidToken", func() {
			token := buildToken("ak-001", "test-secret", method, now.UnixMilli(), "nonce")
			parts := splitToken(token)
			parts[4] = "hmac-sha256:invalidsignature=="
			tamperedToken := joinToken(parts)
			_, err := authenticator.Handle(context.Background(), method, tamperedToken)
			So(err, ShouldNotBeNil)
			So(status.Code(err), ShouldEqual, codes.Unauthenticated)
		})
	})
}

func TestAccessKeyAuthenticator_Handle_UpdateLastAccessError(t *testing.T) {
	Convey("TestAccessKeyAuthenticator_Handle_UpdateLastAccessError", t, func() {
		now := time.Now().UTC()
		key := &mockAccessKey{
			userID:  "user-123",
			enabled: true,
			secret:  "test-secret",
		}
		pool := &mockAccessKeyPool{
			key:           key,
			updateLastErr: fmt.Errorf("database error"),
		}
		authenticator := NewAccessKeyAuthenticator(pool)
		method := "/test.Service/Method"

		Convey("当更新最后访问时间发生内部错误时，应该返回ErrInternal", func() {
			token := buildToken("ak-001", "test-secret", method, now.UnixMilli(), "nonce")
			_, err := authenticator.Handle(context.Background(), method, token)
			So(err, ShouldNotBeNil)
			So(status.Code(err), ShouldEqual, codes.Internal)
		})
	})
}

func TestAccessKeyAuthenticator_Handle_EdgeCases(t *testing.T) {
	Convey("TestAccessKeyAuthenticator_Handle_EdgeCases", t, func() {
		key := &mockAccessKey{
			userID:  "user-123",
			enabled: true,
			secret:  "test-secret",
		}
		pool := &mockAccessKeyPool{key: key}
		authenticator := NewAccessKeyAuthenticator(pool)

		Convey("当gRPC方法名包含特殊字符时，应该能正确认证", func() {
			method := "/test.Service/Special-Method_v2"
			token := buildToken("ak-001", "test-secret", method, time.Now().UTC().UnixMilli(), "nonce")
			userID, err := authenticator.Handle(context.Background(), method, token)
			So(err, ShouldBeNil)
			So(userID, ShouldEqual, "user-123")
		})

		Convey("当时间戳为零时，应该能正常处理", func() {
			method := "/test.Service/Method"
			expiredAt := time.UnixMilli(86400000)
			keyWithExpiry := &mockAccessKey{
				userID:    "user-123",
				enabled:   true,
				secret:    "test-secret",
				expiredAt: &expiredAt,
			}
			poolZero := &mockAccessKeyPool{key: keyWithExpiry}
			authZero := NewAccessKeyAuthenticator(poolZero)
			token := buildToken("ak-001", "test-secret", method, 0, "nonce-zero")
			userID, err := authZero.Handle(context.Background(), method, token)
			So(err, ShouldBeNil)
			So(userID, ShouldEqual, "user-123")
		})

		Convey("当nonce为空字符串时，仍应正常处理", func() {
			method := "/test.Service/Method"
			token := buildToken("ak-001", "test-secret", method, time.Now().UTC().UnixMilli(), "")
			userID, err := authenticator.Handle(context.Background(), method, token)
			So(err, ShouldBeNil)
			So(userID, ShouldEqual, "user-123")
		})

		Convey("当accessKeyID为空字符串时，密钥不存在应返回ErrInvalidToken", func() {
			emptyPool := &mockAccessKeyPool{key: nil}
			emptyAuth := NewAccessKeyAuthenticator(emptyPool)
			method := "/test.Service/Method"
			token := buildToken("", "test-secret", method, time.Now().UTC().UnixMilli(), "nonce")
			_, err := emptyAuth.Handle(context.Background(), method, token)
			So(err, ShouldNotBeNil)
			So(status.Code(err), ShouldEqual, codes.Unauthenticated)
		})
	})
}

func splitToken(token string) []string {
	result := make([]string, 0, 5)
	start := 0
	for i, c := range token {
		if c == ':' {
			result = append(result, token[start:i])
			start = i + 1
			if len(result) == 4 {
				break
			}
		}
	}
	result = append(result, token[start:])
	return result
}

func joinToken(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += ":"
		}
		result += p
	}
	return result
}
