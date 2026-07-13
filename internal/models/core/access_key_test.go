package core

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/auth"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/redis"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// TestAccessKey_UserID 测试 UserID 方法
func TestAccessKey_UserID(t *testing.T) {
	convey.Convey("Test AccessKey UserID method", t, func() {
		convey.Convey("Should return user's UnionID as string", func() {
			// 创建测试用户
			user := &User{
				UnionID: serializer.HexUUID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
					0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10},
			}

			// 创建访问键
			accessKey := &AccessKey{
				User: *user,
			}

			// 验证 UserID 方法返回正确的 UnionID 字符串
			// HexUUID 的 String() 方法返回带连字符的标准 UUID 格式
			expectedID := "01020304-0506-0708-090a-0b0c0d0e0f10"
			convey.So(accessKey.UserID(), convey.ShouldEqual, expectedID)
		})
	})
}

// TestAccessKey_IsEnabled 测试 IsEnabled 方法
func TestAccessKey_IsEnabled(t *testing.T) {
	convey.Convey("Test AccessKey IsEnabled method", t, func() {
		convey.Convey("Should return true when access key is enabled", func() {
			accessKey := &AccessKey{
				Enabled: true,
			}
			convey.So(accessKey.IsEnabled(), convey.ShouldBeTrue)
		})

		convey.Convey("Should return false when access key is disabled", func() {
			accessKey := &AccessKey{
				Enabled: false,
			}
			convey.So(accessKey.IsEnabled(), convey.ShouldBeFalse)
		})
	})
}

// TestAccessKey_Secret 测试 Secret 方法
func TestAccessKey_Secret(t *testing.T) {
	convey.Convey("Test AccessKey Secret method", t, func() {
		convey.Convey("Should return access key secret", func() {
			expectedSecret := "test-secret-key-1234567890"
			accessKey := &AccessKey{
				AccessKeySecret: serializer.NewCrypto([]byte(expectedSecret)),
			}
			convey.So(accessKey.Secret(), convey.ShouldEqual, expectedSecret)
		})
	})
}

// TestAccessKey_ExpiredAt 测试 ExpiredAt 方法
func TestAccessKey_ExpiredAt(t *testing.T) {
	convey.Convey("Test AccessKey ExpiredAt method", t, func() {
		convey.Convey("Should return nil when no expiration time is set", func() {
			accessKey := &AccessKey{
				RawExpiredAt: nil,
			}
			convey.So(accessKey.ExpiredAt(), convey.ShouldBeNil)
		})

		convey.Convey("Should return correct expiration time when set", func() {
			expirationTime := time.Now().Add(24 * time.Hour)
			accessKey := &AccessKey{
				RawExpiredAt: &expirationTime,
			}
			convey.So(accessKey.ExpiredAt(), convey.ShouldEqual, &expirationTime)
		})
	})
}

// TestNewAccessKey 测试 NewAccessKey 函数
func TestNewAccessKey(t *testing.T) {
	convey.Convey("Test NewAccessKey function", t, func() {
		convey.Convey("Should create access key with valid parameters", func() {
			// 创建测试用户
			user := &User{
				UnionID: serializer.HexUUID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
					0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10},
			}

			description := "Test access key for API integration"
			expirationTime := time.Now().Add(7 * 24 * time.Hour)

			// 测试启用状态的访问键
			accessKey, err := NewAccessKey(user, true, description, &expirationTime)
			convey.So(err, convey.ShouldBeNil)
			convey.So(accessKey, convey.ShouldNotBeNil)
			convey.So(accessKey.User.UnionID, convey.ShouldEqual, user.UnionID)
			convey.So(accessKey.Enabled, convey.ShouldBeTrue)
			convey.So(accessKey.Description, convey.ShouldEqual, description)
			convey.So(accessKey.RawExpiredAt, convey.ShouldEqual, &expirationTime)
			convey.So(accessKey.LastAccessAt, convey.ShouldBeNil)

			// 验证访问键 ID 和密钥的长度
			convey.So(len(accessKey.AccessKeyID), convey.ShouldEqual, 20)
			convey.So(len(accessKey.AccessKeySecret.Value()), convey.ShouldEqual, 40)

			// 测试禁用状态的访问键
			accessKey2, err := NewAccessKey(user, false, "Disabled key", nil)
			convey.So(err, convey.ShouldBeNil)
			convey.So(accessKey2.Enabled, convey.ShouldBeFalse)
			convey.So(accessKey2.RawExpiredAt, convey.ShouldBeNil)
		})

		convey.Convey("Should create access key without expiration time", func() {
			user := &User{
				UnionID: serializer.HexUUID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
					0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10},
			}

			accessKey, err := NewAccessKey(user, true, "No expiration", nil)
			convey.So(err, convey.ShouldBeNil)
			convey.So(accessKey.RawExpiredAt, convey.ShouldBeNil)
		})

		convey.Convey("Should generate different IDs and secrets for different keys", func() {
			user := &User{
				UnionID: serializer.HexUUID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
					0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10},
			}

			accessKey1, err := NewAccessKey(user, true, "Key 1", nil)
			convey.So(err, convey.ShouldBeNil)

			accessKey2, err := NewAccessKey(user, true, "Key 2", nil)
			convey.So(err, convey.ShouldBeNil)

			// 验证两个访问键的 ID 和密钥不同
			convey.So(accessKey1.AccessKeyID, convey.ShouldNotEqual, accessKey2.AccessKeyID)
			convey.So(accessKey1.Secret(), convey.ShouldNotEqual, accessKey2.Secret())
		})
	})
}

// TestAccessKeyImplementsInterface 测试 AccessKey 是否实现了 auth.AccessKey 接口
func TestAccessKeyImplementsInterface(t *testing.T) {
	convey.Convey("Test AccessKey implements auth.AccessKey interface", t, func() {
		convey.Convey("AccessKey should implement auth.AccessKey interface", func() {
			// 创建 AccessKey 实例
			accessKey := &AccessKey{}

			// 验证类型转换是否成功
			var _ auth.AccessKey = accessKey
			convey.So(true, convey.ShouldBeTrue) // 如果编译通过，测试通过
		})
	})
}

// TestAccessKeyPoolImplementsInterface 测试 AccessKeyPool 是否实现了 auth.AccessKeyPool 接口
func TestAccessKeyPoolImplementsInterface(t *testing.T) {
	convey.Convey("Test AccessKeyPool implements auth.AccessKeyPool interface", t, func() {
		convey.Convey("AccessKeyPool should implement auth.AccessKeyPool interface", func() {
			// 创建 AccessKeyPool 实例（使用 nil 参数，仅用于接口检查）
			pool := &AccessKeyPool{}

			// 验证类型转换是否成功
			var _ auth.AccessKeyPool = pool
			convey.So(true, convey.ShouldBeTrue) // 如果编译通过，测试通过
		})
	})
}

// TestNonceKey 测试 nonceKey 函数
func TestNonceKey(t *testing.T) {
	convey.Convey("Test nonceKey function", t, func() {
		convey.Convey("Should generate correct Redis key format", func() {
			accessKeyID := "test-access-key-id-123"
			nonce := "random-nonce-456"

			expectedKey := "akp:test-access-key-id-123:random-nonce-456"
			actualKey := nonceKey(accessKeyID, nonce)

			convey.So(actualKey, convey.ShouldEqual, expectedKey)
		})

		convey.Convey("Should handle different access key IDs and nonces", func() {
			testCases := []struct {
				accessKeyID string
				nonce       string
				expected    string
			}{
				{"id1", "nonce1", "akp:id1:nonce1"},
				{"", "nonce", "akp::nonce"},
				{"id", "", "akp:id:"},
				{"", "", "akp::"},
				{"long-access-key-id-with-special-chars", "nonce-with-numbers-123", "akp:long-access-key-id-with-special-chars:nonce-with-numbers-123"},
			}

			for _, tc := range testCases {
				actualKey := nonceKey(tc.accessKeyID, tc.nonce)
				convey.So(actualKey, convey.ShouldEqual, tc.expected)
			}
		})
	})
}

// TestNewAccessKeyPool 测试 NewAccessKeyPool 函数
func TestNewAccessKeyPool(t *testing.T) {
	convey.Convey("Test NewAccessKeyPool function", t, func() {
		convey.Convey("Should create AccessKeyPool with provided parameters", func() {
			// 创建模拟的数据库和缓存（实际测试中可能需要使用 mock）
			var db database.DB
			var cache redis.Client
			ttl := 5 * time.Minute

			pool := NewAccessKeyPool(&db, &cache, ttl)

			convey.So(pool, convey.ShouldNotBeNil)
			convey.So(pool.ttl, convey.ShouldEqual, ttl)
		})

		convey.Convey("Should handle different TTL values", func() {
			var db database.DB
			var cache redis.Client

			testCases := []time.Duration{
				0,
				time.Second,
				time.Minute,
				time.Hour,
				24 * time.Hour,
			}

			for _, ttl := range testCases {
				pool := NewAccessKeyPool(&db, &cache, ttl)
				convey.So(pool.ttl, convey.ShouldEqual, ttl)
			}
		})
	})
}

// TestAccessKeyPool_GetKey 测试 GetKey 方法
// 注意：这是一个单元测试框架，实际数据库操作需要 mock
func TestAccessKeyPool_GetKey(t *testing.T) {
	convey.Convey("Test AccessKeyPool GetKey method", t, func() {
		convey.Convey("Should return error for non-existent access key", func() {
			// 在实际测试中，这里应该使用 mock 数据库
			convey.SkipConvey("This test requires database mock setup", func() {})
		})

		convey.Convey("Should return access key for valid ID", func() {
			// 在实际测试中，这里应该使用 mock 数据库
			convey.SkipConvey("This test requires database mock setup", func() {})
		})
	})
}

// TestAccessKeyPool_IsDuplicated 测试 IsDuplicated 方法
// 注意：这是一个单元测试框架，实际缓存操作需要 mock
func TestAccessKeyPool_IsDuplicated(t *testing.T) {
	convey.Convey("Test AccessKeyPool IsDuplicated method", t, func() {
		convey.Convey("Should return true for expired timestamp", func() {
			// 在实际测试中，这里应该使用 mock 缓存
			convey.SkipConvey("This test requires cache mock setup", func() {})
		})

		convey.Convey("Should return true for duplicate nonce", func() {
			// 在实际测试中，这里应该使用 mock 缓存
			convey.SkipConvey("This test requires cache mock setup", func() {})
		})

		convey.Convey("Should return false for valid nonce", func() {
			// 在实际测试中，这里应该使用 mock 缓存
			convey.SkipConvey("This test requires cache mock setup", func() {})
		})
	})
}

// TestAccessKeyPool_UpdateLastAccess 测试 UpdateLastAccess 方法
// 注意：这是一个单元测试框架，实际数据库操作需要 mock
func TestAccessKeyPool_UpdateLastAccess(t *testing.T) {
	convey.Convey("Test AccessKeyPool UpdateLastAccess method", t, func() {
		convey.Convey("Should update last access time", func() {
			// 在实际测试中，这里应该使用 mock 数据库
			convey.SkipConvey("This test requires database mock setup", func() {})
		})

		convey.Convey("Should handle non-existent access key", func() {
			// 在实际测试中，这里应该使用 mock 数据库
			convey.SkipConvey("This test requires database mock setup", func() {})
		})
	})
}

// TestConstants 测试常量定义
func TestConstants(t *testing.T) {
	convey.Convey("Test constants", t, func() {
		convey.Convey("ACCESS_KEY_PREFIX should have correct value", func() {
			convey.So(ACCESS_KEY_PREFIX, convey.ShouldEqual, "akp")
		})
	})
}

// TestAccessKey_Secret_EdgeCases 测试 Secret 方法的边界情况
func TestAccessKey_Secret_EdgeCases(t *testing.T) {
	convey.Convey("Test AccessKey Secret method edge cases", t, func() {
		convey.Convey("Should handle empty secret", func() {
			accessKey := &AccessKey{
				AccessKeySecret: serializer.NewCrypto([]byte("")),
			}
			convey.So(accessKey.Secret(), convey.ShouldEqual, "")
		})

		convey.Convey("Should handle secret with special characters", func() {
			testCases := []string{
				"key-with-dashes-1234567890",
				"key.with.dots.1234567890",
				"key_with_underscores_12345",
				"key/with/slashes/123456789",
				"key@with#symbols!1234567890",
				"密钥包含中文字符测试12345678",
				"🔑emoji-key-123456789012345",
			}
			for _, secret := range testCases {
				accessKey := &AccessKey{
					AccessKeySecret: serializer.NewCrypto([]byte(secret)),
				}
				convey.So(accessKey.Secret(), convey.ShouldEqual, secret)
			}
		})

		convey.Convey("Should handle secret with control characters", func() {
			controlChars := "key\twith\ncontrol\r\nchars"
			accessKey := &AccessKey{
				AccessKeySecret: serializer.NewCrypto([]byte(controlChars)),
			}
			convey.So(accessKey.Secret(), convey.ShouldEqual, controlChars)
		})

		convey.Convey("Should handle very long secret", func() {
			longSecret := make([]byte, 10000)
			for i := range longSecret {
				longSecret[i] = byte('A' + (i % 26))
			}
			accessKey := &AccessKey{
				AccessKeySecret: serializer.NewCrypto(longSecret),
			}
			convey.So(accessKey.Secret(), convey.ShouldEqual, string(longSecret))
		})

		convey.Convey("Should handle access key with new user entity (no pre-saved ID)", func() {
			user := &User{
				UnionID: serializer.NewHexUUIDv4(),
			}
			accessKey := &AccessKey{
				User:            *user,
				RawUserID:       0,
				AccessKeySecret: serializer.NewCrypto([]byte("newuser-secret")),
			}
			convey.So(accessKey.Secret(), convey.ShouldEqual, "newuser-secret")
			convey.So(accessKey.RawUserID, convey.ShouldEqual, 0)
		})
	})
}

// TestAccessKey_AccessKeyID_EdgeCases 测试 AccessKeyID 边界情况
func TestAccessKey_AccessKeyID_EdgeCases(t *testing.T) {
	convey.Convey("Test AccessKey AccessKeyID edge cases", t, func() {
		convey.Convey("Should handle empty AccessKeyID", func() {
			accessKey := &AccessKey{
				AccessKeyID: "",
			}
			convey.So(accessKey.AccessKeyID, convey.ShouldEqual, "")
			convey.So(len(accessKey.AccessKeyID), convey.ShouldEqual, 0)
		})

		convey.Convey("Should handle AccessKeyID with special characters", func() {
			specialID := "akid-with!#$%&'()*+,-./0123"
			accessKey := &AccessKey{
				AccessKeyID: specialID,
			}
			convey.So(accessKey.AccessKeyID, convey.ShouldEqual, specialID)
		})
	})
}

// TestNewAccessKey_EdgeCases 测试 NewAccessKey 边界情况
func TestNewAccessKey_EdgeCases(t *testing.T) {
	convey.Convey("Test NewAccessKey edge cases", t, func() {
		user := &User{
			UnionID: serializer.NewHexUUIDv4(),
		}

		convey.Convey("Should handle empty description", func() {
			accessKey, err := NewAccessKey(user, true, "", nil)
			convey.So(err, convey.ShouldBeNil)
			convey.So(accessKey.Description, convey.ShouldEqual, "")
			convey.So(accessKey.AccessKeyID, convey.ShouldNotBeEmpty)
			convey.So(len(accessKey.AccessKeyID), convey.ShouldEqual, 20)
			convey.So(len(accessKey.AccessKeySecret.Value()), convey.ShouldEqual, 40)
		})

		convey.Convey("Should handle description with special characters", func() {
			descriptions := []string{
				"包含中文的描述信息",
				"description with emoji 🚀",
				"<script>alert('xss')</script>",
				"SQL injection'; DROP TABLE users; --",
				"unicode: \u0000\u0001\u0002 end",
				"    leading and trailing spaces   ",
				"multi\nline\ndescription",
				"tab\tseparated\tdescription",
			}
			for _, desc := range descriptions {
				accessKey, err := NewAccessKey(user, true, desc, nil)
				convey.So(err, convey.ShouldBeNil)
				convey.So(accessKey.Description, convey.ShouldEqual, desc)
			}
		})

		convey.Convey("Should handle expired access key (past expiration time)", func() {
			pastTime := time.Now().Add(-24 * time.Hour)
			accessKey, err := NewAccessKey(user, false, "expired key", &pastTime)
			convey.So(err, convey.ShouldBeNil)
			convey.So(accessKey.ExpiredAt(), convey.ShouldNotBeNil)
			convey.So(accessKey.ExpiredAt().Before(time.Now()), convey.ShouldBeTrue)
		})

		convey.Convey("Should handle far future expiration time", func() {
			farFuture := time.Now().Add(365 * 24 * 100 * time.Hour) // ~100 年后
			accessKey, err := NewAccessKey(user, true, "long-lived key", &farFuture)
			convey.So(err, convey.ShouldBeNil)
			convey.So(accessKey.ExpiredAt(), convey.ShouldNotBeNil)
			convey.So(accessKey.ExpiredAt().After(time.Now()), convey.ShouldBeTrue)
		})

		convey.Convey("Should handle immediate expiration (zero duration)", func() {
			now := time.Now()
			accessKey, err := NewAccessKey(user, true, "instant-expire", &now)
			convey.So(err, convey.ShouldBeNil)
			convey.So(accessKey.ExpiredAt(), convey.ShouldNotBeNil)
		})

		convey.Convey("Should generate random ID and secret that are different per call", func() {
			keys := make([]*AccessKey, 0, 100)
			for i := 0; i < 100; i++ {
				key, err := NewAccessKey(user, true, "batch", nil)
				convey.So(err, convey.ShouldBeNil)
				keys = append(keys, key)
			}

			// 验证所有生成的 ID 和 Secret 都唯一
			ids := make(map[string]bool)
			secrets := make(map[string]bool)
			for _, k := range keys {
				convey.So(ids[k.AccessKeyID], convey.ShouldBeFalse)
				convey.So(secrets[k.Secret()], convey.ShouldBeFalse)
				ids[k.AccessKeyID] = true
				secrets[k.Secret()] = true
			}
		})
	})
}

// TestAccessKey_CreateWithLimit_EdgeCases 测试 CreateWithLimit 边界情况
func TestAccessKey_CreateWithLimit_EdgeCases(t *testing.T) {
	convey.Convey("Test AccessKey CreateWithLimit edge cases", t, func() {
		convey.Convey("Should have CreateWithLimit method with correct signature", func() {
			accessKey := &AccessKey{}
			convey.So(accessKey.CreateWithLimit, convey.ShouldNotBeNil)
		})

		convey.Convey("ErrTooManyAccessKey should be a defined error", func() {
			convey.So(ErrTooManyAccessKey, convey.ShouldNotBeNil)
			convey.So(ErrTooManyAccessKey.Error(), convey.ShouldEqual, "too many access keys")
		})
	})
}

// TestAccessKey_ExpiredAt_EdgeCases 测试 ExpiredAt 边界情况
func TestAccessKey_ExpiredAt_EdgeCases(t *testing.T) {
	convey.Convey("Test AccessKey ExpiredAt edge cases", t, func() {
		convey.Convey("Should handle very old expiration time", func() {
			oldTime := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
			accessKey := &AccessKey{
				RawExpiredAt: &oldTime,
			}
			convey.So(accessKey.ExpiredAt(), convey.ShouldEqual, &oldTime)
		})

		convey.Convey("Should handle zero time expiration", func() {
			zeroTime := time.Time{}
			accessKey := &AccessKey{
				RawExpiredAt: &zeroTime,
			}
			convey.So(accessKey.ExpiredAt(), convey.ShouldEqual, &zeroTime)
			convey.So(accessKey.ExpiredAt().IsZero(), convey.ShouldBeTrue)
		})
	})
}

// TestAccessKey_LastAccessAt_EdgeCases 测试 LastAccessAt 边界情况
func TestAccessKey_LastAccessAt_EdgeCases(t *testing.T) {
	convey.Convey("Test AccessKey LastAccessAt edge cases", t, func() {
		convey.Convey("Should be nil for newly created key", func() {
			user := &User{
				UnionID: serializer.NewHexUUIDv4(),
			}
			accessKey, err := NewAccessKey(user, true, "new key", nil)
			convey.So(err, convey.ShouldBeNil)
			convey.So(accessKey.LastAccessAt, convey.ShouldBeNil)
		})

		convey.Convey("Should handle non-nil last access time", func() {
			lastAccess := time.Now()
			accessKey := &AccessKey{
				LastAccessAt: &lastAccess,
			}
			convey.So(accessKey.LastAccessAt, convey.ShouldEqual, &lastAccess)
			convey.So(accessKey.LastAccessAt.IsZero(), convey.ShouldBeFalse)
		})
	})
}
