package grpc

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

// mockPerRPCCredentials 是 credentials.PerRPCCredentials 接口的模拟实现
// 用于测试 AuthWrapperPerRPCCredentials
type mockPerRPCCredentials struct {
	metadata   map[string]string
	err        error
	requireTLS bool
}

// GetRequestMetadata 返回预设的元数据或错误
func (m *mockPerRPCCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.metadata, nil
}

// RequireTransportSecurity 返回预设的传输安全要求
func (m *mockPerRPCCredentials) RequireTransportSecurity() bool {
	return m.requireTLS
}

// TestNewAccessKeySecretPerRPCCredentials 测试 AccessKeySecretPerRPCCredentials 的构造函数
func TestNewAccessKeySecretPerRPCCredentials(t *testing.T) {
	Convey("TestNewAccessKeySecretPerRPCCredentials", t, func() {
		Convey("当提供有效的访问密钥ID和密钥时", func() {
			accessKeyID := "test-access-key-id"
			accessKeySecret := "test-access-key-secret"

			creds := NewAccessKeySecretPerRPCCredentials(accessKeyID, accessKeySecret)

			Convey("应该成功创建实例", func() {
				So(creds, ShouldNotBeNil)
				So(creds.AccessKeyID, ShouldEqual, accessKeyID)
				So(creds.AccessKeySecret, ShouldEqual, accessKeySecret)
			})
		})

		Convey("当提供空的访问密钥ID时", func() {
			accessKeyID := ""
			accessKeySecret := "test-access-key-secret"

			creds := NewAccessKeySecretPerRPCCredentials(accessKeyID, accessKeySecret)

			Convey("应该成功创建实例", func() {
				So(creds, ShouldNotBeNil)
				So(creds.AccessKeyID, ShouldEqual, accessKeyID)
				So(creds.AccessKeySecret, ShouldEqual, accessKeySecret)
			})
		})

		Convey("当提供空的访问密钥时", func() {
			accessKeyID := "test-access-key-id"
			accessKeySecret := ""

			creds := NewAccessKeySecretPerRPCCredentials(accessKeyID, accessKeySecret)

			Convey("应该成功创建实例", func() {
				So(creds, ShouldNotBeNil)
				So(creds.AccessKeyID, ShouldEqual, accessKeyID)
				So(creds.AccessKeySecret, ShouldEqual, accessKeySecret)
			})
		})
	})
}

// TestAccessKeySecretPerRPCCredentials_GetRequestMetadata 测试 GetRequestMetadata 方法
func TestAccessKeySecretPerRPCCredentials_GetRequestMetadata(t *testing.T) {
	Convey("TestAccessKeySecretPerRPCCredentials_GetRequestMetadata", t, func() {
		accessKeyID := "test-access-key-id"
		accessKeySecret := "test-access-key-secret"
		creds := NewAccessKeySecretPerRPCCredentials(accessKeyID, accessKeySecret)

		Convey("当提供有效的上下文和URI时", func() {
			ctx := context.WithValue(context.Background(), "method", "/test.Service/TestMethod")
			uri := "grpc://localhost:50051"

			metadata, err := creds.GetRequestMetadata(ctx, uri)

			Convey("应该成功生成元数据", func() {
				So(err, ShouldBeNil)
				So(metadata, ShouldNotBeNil)
				So(metadata, ShouldContainKey, MetadataKeyAuthorization)

				authHeader := metadata[MetadataKeyAuthorization]
				So(authHeader, ShouldNotBeEmpty)

				Convey("认证头应该包含正确的访问密钥ID", func() {
					So(authHeader, ShouldStartWith, accessKeyID+":")
				})

				Convey("认证头应该包含时间戳", func() {
					// 验证时间戳格式
					parts := splitAuthHeader(authHeader)
					So(len(parts), ShouldEqual, 5)
					So(parts[0], ShouldEqual, accessKeyID)
					So(parts[1], ShouldNotBeEmpty) // 时间戳
				})

				Convey("认证头应该包含随机数", func() {
					parts := splitAuthHeader(authHeader)
					So(parts[2], ShouldNotBeEmpty)     // nonce
					So(len(parts[2]), ShouldEqual, 16) // 16位随机数
				})

				Convey("认证头应该包含Base64编码的方法名", func() {
					parts := splitAuthHeader(authHeader)
					So(parts[3], ShouldNotBeEmpty) // base64(fullMethod)

					// 验证是有效的Base64编码
					decoded, decodeErr := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(parts[3])
					So(decodeErr, ShouldBeNil)
					So(string(decoded), ShouldContainSubstring, "grpc://localhost:50051/test.Service/TestMethod")
				})

				Convey("认证头应该包含签名", func() {
					parts := splitAuthHeader(authHeader)
					So(parts[4], ShouldNotBeEmpty) // 签名
					So(parts[4], ShouldStartWith, "hmac-sha256:")
				})
			})
		})

		Convey("当上下文缺少method值时", func() {
			ctx := context.Background() // 没有method值
			uri := "grpc://localhost:50051"

			metadata, err := creds.GetRequestMetadata(ctx, uri)

			Convey("应该返回错误", func() {
				So(err, ShouldNotBeNil)
				So(metadata, ShouldBeNil)
			})
		})

		Convey("当提供无效的URI时", func() {
			ctx := context.WithValue(context.Background(), "method", "/test.Service/TestMethod")
			uri := "://invalid-uri" // 无效的URI

			metadata, err := creds.GetRequestMetadata(ctx, uri)

			Convey("应该返回错误", func() {
				So(err, ShouldNotBeNil)
				So(metadata, ShouldBeNil)
			})
		})

		Convey("当提供多个URI时", func() {
			ctx := context.WithValue(context.Background(), "method", "/test.Service/TestMethod")
			uri1 := "grpc://localhost:50051"
			uri2 := "grpc://localhost:50052"

			metadata, err := creds.GetRequestMetadata(ctx, uri1, uri2)

			Convey("应该使用第一个URI", func() {
				So(err, ShouldBeNil)
				So(metadata, ShouldNotBeNil)
				So(metadata, ShouldContainKey, MetadataKeyAuthorization)

				authHeader := metadata[MetadataKeyAuthorization]
				parts := splitAuthHeader(authHeader)
				decoded, _ := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(parts[3])
				So(string(decoded), ShouldContainSubstring, "grpc://localhost:50051")
			})
		})

		Convey("当使用不同的协议时", func() {
			ctx := context.WithValue(context.Background(), "method", "/test.Service/TestMethod")
			uri := "https://api.example.com:443"

			metadata, err := creds.GetRequestMetadata(ctx, uri)

			Convey("应该成功生成元数据", func() {
				So(err, ShouldBeNil)
				So(metadata, ShouldNotBeNil)

				authHeader := metadata[MetadataKeyAuthorization]
				parts := splitAuthHeader(authHeader)
				decoded, _ := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(parts[3])
				So(string(decoded), ShouldContainSubstring, "https://api.example.com:443")
			})
		})
	})
}

// TestAccessKeySecretPerRPCCredentials_RequireTransportSecurity 测试 RequireTransportSecurity 方法
func TestAccessKeySecretPerRPCCredentials_RequireTransportSecurity(t *testing.T) {
	Convey("TestAccessKeySecretPerRPCCredentials_RequireTransportSecurity", t, func() {
		accessKeyID := "test-access-key-id"
		accessKeySecret := "test-access-key-secret"
		creds := NewAccessKeySecretPerRPCCredentials(accessKeyID, accessKeySecret)

		Convey("应该要求传输安全", func() {
			requireTLS := creds.RequireTransportSecurity()
			So(requireTLS, ShouldBeTrue)
		})

		Convey("对于不同的实例也应该要求传输安全", func() {
			creds2 := NewAccessKeySecretPerRPCCredentials("id2", "secret2")
			requireTLS := creds2.RequireTransportSecurity()
			So(requireTLS, ShouldBeTrue)
		})
	})
}

// TestNewAuthWrapperPerRPCCredentials 测试 AuthWrapperPerRPCCredentials 的构造函数
func TestNewAuthWrapperPerRPCCredentials(t *testing.T) {
	Convey("TestNewAuthWrapperPerRPCCredentials", t, func() {
		Convey("当提供有效的协议和内部凭证时", func() {
			protocol := "accesskeySecret"
			inner := &mockPerRPCCredentials{
				metadata:   map[string]string{"test": "value"},
				requireTLS: true,
			}

			wrapper := NewAuthWrapperPerRPCCredentials(protocol, inner)

			Convey("应该成功创建包装器实例", func() {
				So(wrapper, ShouldNotBeNil)
				So(wrapper.protocol, ShouldEqual, protocol)
				So(wrapper.inner, ShouldEqual, inner)
			})
		})

		Convey("当内部凭证为nil时", func() {
			protocol := "accesskeySecret"

			wrapper := NewAuthWrapperPerRPCCredentials(protocol, nil)

			Convey("应该成功创建包装器实例", func() {
				So(wrapper, ShouldNotBeNil)
				So(wrapper.protocol, ShouldEqual, protocol)
				So(wrapper.inner, ShouldBeNil)
			})
		})
	})
}

// TestAuthWrapperPerRPCCredentials_GetRequestMetadata 测试包装器的 GetRequestMetadata 方法
func TestAuthWrapperPerRPCCredentials_GetRequestMetadata(t *testing.T) {
	Convey("TestAuthWrapperPerRPCCredentials_GetRequestMetadata", t, func() {
		Convey("当内部凭证成功返回元数据时", func() {
			protocol := "accesskeySecret"
			innerMetadata := map[string]string{
				"authorization": "test-auth-token",
				"other-key":     "other-value",
			}
			inner := &mockPerRPCCredentials{
				metadata: innerMetadata,
				err:      nil,
			}

			wrapper := NewAuthWrapperPerRPCCredentials(protocol, inner)
			ctx := context.Background()
			uri := "grpc://localhost:50051"

			metadata, err := wrapper.GetRequestMetadata(ctx, uri)

			Convey("应该成功返回元数据", func() {
				So(err, ShouldBeNil)
				So(metadata, ShouldNotBeNil)

				Convey("应该包含原始内部凭证的所有元数据", func() {
					for key, value := range innerMetadata {
						So(metadata, ShouldContainKey, key)
						So(metadata[key], ShouldEqual, value)
					}
				})

				Convey("应该添加协议标识", func() {
					So(metadata, ShouldContainKey, MetadataKeyAuthorize)
					So(metadata[MetadataKeyAuthorize], ShouldEqual, protocol)
				})
			})
		})

		Convey("当内部凭证返回错误时", func() {
			protocol := "accesskeySecret"
			inner := &mockPerRPCCredentials{
				metadata: nil,
				err:      fmt.Errorf("inner credential error"),
			}

			wrapper := NewAuthWrapperPerRPCCredentials(protocol, inner)
			ctx := context.Background()
			uri := "grpc://localhost:50051"

			metadata, err := wrapper.GetRequestMetadata(ctx, uri)

			Convey("应该返回错误", func() {
				So(err, ShouldNotBeNil)
				So(metadata, ShouldBeNil)
			})
		})

		Convey("当内部凭证为nil时", func() {
			protocol := "accesskeySecret"
			wrapper := NewAuthWrapperPerRPCCredentials(protocol, nil)
			ctx := context.Background()
			uri := "grpc://localhost:50051"

			metadata, err := wrapper.GetRequestMetadata(ctx, uri)

			Convey("应该返回错误", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "inner credentials is nil")
				So(metadata, ShouldBeNil)
			})
		})
	})
}

// TestAuthWrapperPerRPCCredentials_RequireTransportSecurity 测试包装器的 RequireTransportSecurity 方法
func TestAuthWrapperPerRPCCredentials_RequireTransportSecurity(t *testing.T) {
	Convey("TestAuthWrapperPerRPCCredentials_RequireTransportSecurity", t, func() {
		Convey("当内部凭证要求传输安全时", func() {
			protocol := "accesskeySecret"
			inner := &mockPerRPCCredentials{
				requireTLS: true,
			}

			wrapper := NewAuthWrapperPerRPCCredentials(protocol, inner)

			Convey("包装器也应该要求传输安全", func() {
				requireTLS := wrapper.RequireTransportSecurity()
				So(requireTLS, ShouldBeTrue)
			})
		})

		Convey("当内部凭证不要求传输安全时", func() {
			protocol := "accesskeySecret"
			inner := &mockPerRPCCredentials{
				requireTLS: false,
			}

			wrapper := NewAuthWrapperPerRPCCredentials(protocol, inner)

			Convey("包装器也不应该要求传输安全", func() {
				requireTLS := wrapper.RequireTransportSecurity()
				So(requireTLS, ShouldBeFalse)
			})
		})

		Convey("当内部凭证为nil时", func() {
			protocol := "accesskeySecret"
			wrapper := NewAuthWrapperPerRPCCredentials(protocol, nil)

			Convey("应该返回默认值true", func() {
				requireTLS := wrapper.RequireTransportSecurity()
				So(requireTLS, ShouldBeTrue)
			})
		})
	})
}

// TestConcurrentAccess 测试并发访问的安全性
func TestConcurrentAccess(t *testing.T) {
	Convey("TestConcurrentAccess", t, func() {
		accessKeyID := "test-access-key-id"
		accessKeySecret := "test-access-key-secret"
		creds := NewAccessKeySecretPerRPCCredentials(accessKeyID, accessKeySecret)

		ctx := context.WithValue(context.Background(), "method", "/test.Service/TestMethod")
		uri := "grpc://localhost:50051"

		Convey("多个goroutine同时调用GetRequestMetadata应该安全", func() {
			done := make(chan bool, 10)
			errors := make(chan error, 10)

			for i := 0; i < 10; i++ {
				go func() {
					_, err := creds.GetRequestMetadata(ctx, uri)
					if err != nil {
						errors <- err
					}
					done <- true
				}()
			}

			// 等待所有goroutine完成
			for i := 0; i < 10; i++ {
				<-done
			}

			Convey("不应该有错误发生", func() {
				So(len(errors), ShouldEqual, 0)
			})
		})
	})
}

// TestSignatureConsistency 测试签名的一致性
func TestSignatureConsistency(t *testing.T) {
	Convey("TestSignatureConsistency", t, func() {
		accessKeyID := "test-access-key-id"
		accessKeySecret := "test-access-key-secret"
		creds := NewAccessKeySecretPerRPCCredentials(accessKeyID, accessKeySecret)

		ctx := context.WithValue(context.Background(), "method", "/test.Service/TestMethod")
		uri := "grpc://localhost:50051"

		Convey("相同输入应该生成不同的nonce和签名", func() {
			// 由于时间戳和nonce不同，即使其他输入相同，签名也应该不同
			metadata1, err1 := creds.GetRequestMetadata(ctx, uri)
			So(err1, ShouldBeNil)

			// 等待一小段时间确保时间戳不同
			time.Sleep(1 * time.Millisecond)

			metadata2, err2 := creds.GetRequestMetadata(ctx, uri)
			So(err2, ShouldBeNil)

			authHeader1 := metadata1[MetadataKeyAuthorization]
			authHeader2 := metadata2[MetadataKeyAuthorization]

			Convey("认证头应该不同", func() {
				So(authHeader1, ShouldNotEqual, authHeader2)
			})

			parts1 := splitAuthHeader(authHeader1)
			parts2 := splitAuthHeader(authHeader2)

			Convey("访问密钥ID应该相同", func() {
				So(parts1[0], ShouldEqual, parts2[0])
			})

			Convey("时间戳应该不同", func() {
				So(parts1[1], ShouldNotEqual, parts2[1])
			})

			Convey("nonce应该不同", func() {
				So(parts1[2], ShouldNotEqual, parts2[2])
			})

			Convey("Base64编码的方法名应该相同", func() {
				So(parts1[3], ShouldEqual, parts2[3])
			})

			Convey("签名应该不同", func() {
				So(parts1[4], ShouldNotEqual, parts2[4])
			})
		})
	})
}

// TestDifferentCredentials 测试不同凭证生成不同的签名
func TestDifferentCredentials(t *testing.T) {
	Convey("TestDifferentCredentials", t, func() {
		ctx := context.WithValue(context.Background(), "method", "/test.Service/TestMethod")
		uri := "grpc://localhost:50051"

		Convey("不同访问密钥应该生成不同的签名", func() {
			creds1 := NewAccessKeySecretPerRPCCredentials("id1", "secret1")
			creds2 := NewAccessKeySecretPerRPCCredentials("id2", "secret2")

			metadata1, err1 := creds1.GetRequestMetadata(ctx, uri)
			So(err1, ShouldBeNil)

			metadata2, err2 := creds2.GetRequestMetadata(ctx, uri)
			So(err2, ShouldBeNil)

			authHeader1 := metadata1[MetadataKeyAuthorization]
			authHeader2 := metadata2[MetadataKeyAuthorization]

			parts1 := splitAuthHeader(authHeader1)
			parts2 := splitAuthHeader(authHeader2)

			Convey("访问密钥ID应该不同", func() {
				So(parts1[0], ShouldNotEqual, parts2[0])
			})

			Convey("签名应该不同", func() {
				So(parts1[4], ShouldNotEqual, parts2[4])
			})
		})

		Convey("相同密钥ID不同密钥应该生成不同的签名", func() {
			creds1 := NewAccessKeySecretPerRPCCredentials("same-id", "secret1")
			creds2 := NewAccessKeySecretPerRPCCredentials("same-id", "secret2")

			metadata1, err1 := creds1.GetRequestMetadata(ctx, uri)
			So(err1, ShouldBeNil)

			metadata2, err2 := creds2.GetRequestMetadata(ctx, uri)
			So(err2, ShouldBeNil)

			authHeader1 := metadata1[MetadataKeyAuthorization]
			authHeader2 := metadata2[MetadataKeyAuthorization]

			parts1 := splitAuthHeader(authHeader1)
			parts2 := splitAuthHeader(authHeader2)

			Convey("访问密钥ID应该相同", func() {
				So(parts1[0], ShouldEqual, parts2[0])
			})

			Convey("签名应该不同", func() {
				So(parts1[4], ShouldNotEqual, parts2[4])
			})
		})
	})
}

// TestEdgeCases 测试边缘情况
func TestEdgeCases(t *testing.T) {
	Convey("TestEdgeCases", t, func() {
		Convey("当使用特殊字符的访问密钥时", func() {
			accessKeyID := "test!@#$%^&*()_+-=[]{}|;':\",./<>?"
			accessKeySecret := "secret!@#$%^&*()_+-=[]{}|;':\",./<>?"
			creds := NewAccessKeySecretPerRPCCredentials(accessKeyID, accessKeySecret)

			ctx := context.WithValue(context.Background(), "method", "/test.Service/TestMethod")
			uri := "grpc://localhost:50051"

			metadata, err := creds.GetRequestMetadata(ctx, uri)

			Convey("应该成功生成元数据", func() {
				So(err, ShouldBeNil)
				So(metadata, ShouldNotBeNil)
				So(metadata, ShouldContainKey, MetadataKeyAuthorization)

				authHeader := metadata[MetadataKeyAuthorization]
				So(authHeader, ShouldNotBeEmpty)
				So(authHeader, ShouldStartWith, accessKeyID+":")
			})
		})

		Convey("当使用Unicode字符的访问密钥时", func() {
			accessKeyID := "测试访问密钥ID"
			accessKeySecret := "测试访问密钥"
			creds := NewAccessKeySecretPerRPCCredentials(accessKeyID, accessKeySecret)

			ctx := context.WithValue(context.Background(), "method", "/test.Service/TestMethod")
			uri := "grpc://localhost:50051"

			metadata, err := creds.GetRequestMetadata(ctx, uri)

			Convey("应该成功生成元数据", func() {
				So(err, ShouldBeNil)
				So(metadata, ShouldNotBeNil)
				So(metadata, ShouldContainKey, MetadataKeyAuthorization)

				authHeader := metadata[MetadataKeyAuthorization]
				So(authHeader, ShouldNotBeEmpty)
				So(authHeader, ShouldStartWith, accessKeyID+":")
			})
		})

		Convey("当方法名包含特殊字符时", func() {
			accessKeyID := "test-id"
			accessKeySecret := "test-secret"
			creds := NewAccessKeySecretPerRPCCredentials(accessKeyID, accessKeySecret)

			ctx := context.WithValue(context.Background(), "method", "/test.Service/Test!@#$%Method")
			uri := "grpc://localhost:50051"

			metadata, err := creds.GetRequestMetadata(ctx, uri)

			Convey("应该成功生成元数据", func() {
				So(err, ShouldBeNil)
				So(metadata, ShouldNotBeNil)
				So(metadata, ShouldContainKey, MetadataKeyAuthorization)
			})
		})

		Convey("当URI包含查询参数时", func() {
			accessKeyID := "test-id"
			accessKeySecret := "test-secret"
			creds := NewAccessKeySecretPerRPCCredentials(accessKeyID, accessKeySecret)

			ctx := context.WithValue(context.Background(), "method", "/test.Service/TestMethod")
			uri := "grpc://localhost:50051?param1=value1&param2=value2"

			metadata, err := creds.GetRequestMetadata(ctx, uri)

			Convey("应该成功生成元数据", func() {
				So(err, ShouldBeNil)
				So(metadata, ShouldNotBeNil)
				So(metadata, ShouldContainKey, MetadataKeyAuthorization)
			})
		})

		Convey("当使用非常长的方法名时", func() {
			accessKeyID := "test-id"
			accessKeySecret := "test-secret"
			creds := NewAccessKeySecretPerRPCCredentials(accessKeyID, accessKeySecret)

			// 生成一个很长的方法名
			longMethod := "/" + string(make([]byte, 1000)) + "/TestMethod"
			ctx := context.WithValue(context.Background(), "method", longMethod)
			uri := "grpc://localhost:50051"

			metadata, err := creds.GetRequestMetadata(ctx, uri)

			Convey("应该成功生成元数据", func() {
				So(err, ShouldBeNil)
				So(metadata, ShouldNotBeNil)
				So(metadata, ShouldContainKey, MetadataKeyAuthorization)
			})
		})
	})
}

// splitAuthHeader 辅助函数，用于分割认证头
func splitAuthHeader(header string) []string {
	var parts []string
	start := 0
	count := 0

	for i, ch := range header {
		if ch == ':' {
			parts = append(parts, header[start:i])
			start = i + 1
			count++
			if count == 4 { // 我们已经找到了4个冒号，最后一个部分是签名
				break
			}
		}
	}

	// 添加最后一个部分（签名）
	if start < len(header) {
		parts = append(parts, header[start:])
	}

	return parts
}
