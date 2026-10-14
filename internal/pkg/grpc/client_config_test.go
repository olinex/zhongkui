// Package grpc 提供gRPC客户端配置的单元测试。
// 本文件包含对client_config.go中所有结构体和函数的单元测试，确保配置转换功能正确工作。
package grpc

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"zhongkuisec.com/zhongkui/internal/pkg/tls"
)

// ============================================
// TLS配置测试
// ============================================

// TestTlsClientConfig_DefaultValues 测试TlsClientConfig的默认值。
func TestTlsClientConfig_DefaultValues(t *testing.T) {
	Convey("TestTlsClientConfig_DefaultValues", t, func() {
		Convey("当TlsClientConfig使用默认值创建时", func() {
			config := TlsClientConfig{}
			So(config.Enabled, ShouldBeFalse)
			So(config.Domain, ShouldEqual, "")
			So(config.Cert, ShouldEqual, "")
			So(config.CertFilePath, ShouldEqual, "")
		})

		Convey("当TlsClientConfig使用自定义值初始化时", func() {
			config := TlsClientConfig{
				Enabled:      true,
				Domain:       "example.com",
				Cert:         "test-cert-data",
				CertFilePath: "/path/to/cert.pem",
			}
			So(config.Enabled, ShouldBeTrue)
			So(config.Domain, ShouldEqual, "example.com")
			So(config.Cert, ShouldEqual, "test-cert-data")
			So(config.CertFilePath, ShouldEqual, "/path/to/cert.pem")
		})
	})
}

// TestTlsClientConfig_ToOption_NilConfig 测试TlsClientConfig为nil时的行为。
func TestTlsClientConfig_ToOption_NilConfig(t *testing.T) {
	Convey("TestTlsClientConfig_ToOption_NilConfig", t, func() {
		Convey("当TlsClientConfig为nil时", func() {
			var config *TlsClientConfig
			option, err := config.ToOption()
			So(err, ShouldBeNil)
			So(option, ShouldNotBeNil) // 应该返回不安全的连接选项
		})
	})
}

// TestTlsClientConfig_ToOption_Disabled 测试TLS禁用时的行为。
func TestTlsClientConfig_ToOption_Disabled(t *testing.T) {
	Convey("TestTlsClientConfig_ToOption_Disabled", t, func() {
		Convey("当TLS被禁用时", func() {
			config := &TlsClientConfig{Enabled: false}
			option, err := config.ToOption()
			So(err, ShouldBeNil)
			So(option, ShouldNotBeNil) // 应该返回不安全的连接选项
		})
	})
}

// TestTlsClientConfig_ToOption_ValidCertString 测试提供有效证书字符串时的行为。
func TestTlsClientConfig_ToOption_ValidCertString(t *testing.T) {
	Convey("TestTlsClientConfig_ToOption_ValidCertString", t, func() {
		Convey("当提供有效的证书字符串时", func() {
			// 生成测试证书
			_, cert, err := tls.GenerateTestSignedCert()
			So(err, ShouldBeNil)

			config := &TlsClientConfig{
				Enabled: true,
				Domain:  "test.example.com",
				Cert:    cert,
			}
			option, err := config.ToOption()
			So(err, ShouldBeNil)
			So(option, ShouldNotBeNil)
		})
	})
}

// TestTlsClientConfig_ToOption_InvalidCertString 测试提供无效证书字符串时的行为。
func TestTlsClientConfig_ToOption_InvalidCertString(t *testing.T) {
	Convey("TestTlsClientConfig_ToOption_InvalidCertString", t, func() {
		Convey("当提供无效的证书字符串时", func() {
			config := &TlsClientConfig{
				Enabled: true,
				Domain:  "test.example.com",
				Cert:    "invalid-cert-data",
			}
			option, err := config.ToOption()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "failed to append certs")
			So(option, ShouldBeNil)
		})
	})
}

// TestTlsClientConfig_ToOption_NoCertProvided 测试未提供任何证书时的行为。
func TestTlsClientConfig_ToOption_NoCertProvided(t *testing.T) {
	Convey("TestTlsClientConfig_ToOption_NoCertProvided", t, func() {
		Convey("当启用TLS但未提供证书时", func() {
			config := &TlsClientConfig{
				Enabled: true,
				Domain:  "test.example.com",
				// 既不提供Cert也不提供CertFilePath
			}
			option, err := config.ToOption()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "invalid tls config")
			So(option, ShouldBeNil)
		})
	})
}

// ============================================
// 身份验证配置测试
// ============================================

// TestAuthClientConfig_DefaultValues 测试AuthClientConfig的默认值。
func TestAuthClientConfig_DefaultValues(t *testing.T) {
	Convey("TestAuthClientConfig_DefaultValues", t, func() {
		Convey("当AuthClientConfig使用默认值创建时", func() {
			config := AuthClientConfig{}
			So(config.Enabled, ShouldBeFalse)
			So(config.Authorize, ShouldEqual, "")
			So(config.AccessKeyID, ShouldEqual, "")
			So(config.AccessKeySecret, ShouldEqual, "")
		})

		Convey("当AuthClientConfig使用自定义值初始化时", func() {
			config := AuthClientConfig{
				Enabled:         true,
				Authorize:       ProtocolAccessKeySecret,
				AccessKeyID:     "custom-key-id",
				AccessKeySecret: "custom-key-secret",
			}
			So(config.Enabled, ShouldBeTrue)
			So(config.Authorize, ShouldEqual, ProtocolAccessKeySecret)
			So(config.AccessKeyID, ShouldEqual, "custom-key-id")
			So(config.AccessKeySecret, ShouldEqual, "custom-key-secret")
		})
	})
}

// TestAuthClientConfig_ToOption_NilConfig 测试AuthClientConfig为nil时的行为。
func TestAuthClientConfig_ToOption_NilConfig(t *testing.T) {
	Convey("TestAuthClientConfig_ToOption_NilConfig", t, func() {
		Convey("当AuthClientConfig为nil时", func() {
			var config *AuthClientConfig
			option, err := config.ToOption()
			So(err, ShouldBeNil)
			So(option, ShouldBeNil) // 应该返回nil选项
		})
	})
}

// TestAuthClientConfig_ToOption_Disabled 测试身份验证禁用时的行为。
func TestAuthClientConfig_ToOption_Disabled(t *testing.T) {
	Convey("TestAuthClientConfig_ToOption_Disabled", t, func() {
		Convey("当身份验证被禁用时", func() {
			config := &AuthClientConfig{Enabled: false}
			option, err := config.ToOption()
			So(err, ShouldBeNil)
			So(option, ShouldBeNil) // 应该返回nil选项
		})
	})
}

// TestAuthClientConfig_ToOption_AccessKeySecret 测试使用accessKeySecret协议时的行为。
func TestAuthClientConfig_ToOption_AccessKeySecret(t *testing.T) {
	Convey("TestAuthClientConfig_ToOption_AccessKeySecret", t, func() {
		Convey("当使用accessKeySecret协议时", func() {
			config := &AuthClientConfig{
				Enabled:         true,
				Authorize:       ProtocolAccessKeySecret,
				AccessKeyID:     "test-access-key-id",
				AccessKeySecret: "test-access-key-secret",
			}
			option, err := config.ToOption()
			So(err, ShouldBeNil)
			So(option, ShouldNotBeNil)
		})
	})
}

// TestAuthClientConfig_ToOption_InvalidProtocol 测试使用无效协议时的行为。
func TestAuthClientConfig_ToOption_InvalidProtocol(t *testing.T) {
	Convey("TestAuthClientConfig_ToOption_InvalidProtocol", t, func() {
		Convey("当使用无效的身份验证协议时", func() {
			config := &AuthClientConfig{
				Enabled:   true,
				Authorize: "invalid-protocol",
			}
			option, err := config.ToOption()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "invalid auth protocol")
			So(option, ShouldBeNil)
		})
	})
}

// ============================================
// 保活配置测试
// ============================================

// TestKeepaliveClientConfig_DefaultValues 测试KeepaliveClientConfig的默认值。
func TestKeepaliveClientConfig_DefaultValues(t *testing.T) {
	Convey("TestKeepaliveClientConfig_DefaultValues", t, func() {
		Convey("当KeepaliveClientConfig使用默认值创建时", func() {
			config := KeepaliveClientConfig{}
			So(config.Time, ShouldEqual, 0)
			So(config.Timeout, ShouldEqual, 0)
			So(config.PermitWithoutStream, ShouldBeFalse) // bool的零值是false
		})

		Convey("当KeepaliveClientConfig使用自定义值初始化时", func() {
			config := KeepaliveClientConfig{
				Time:                1 * time.Minute,
				Timeout:             20 * time.Second,
				PermitWithoutStream: true,
			}
			So(config.Time, ShouldEqual, 1*time.Minute)
			So(config.Timeout, ShouldEqual, 20*time.Second)
			So(config.PermitWithoutStream, ShouldBeTrue)
		})
	})
}

// TestKeepaliveClientConfig_ToOption 测试KeepaliveClientConfig的ToOption方法。
func TestKeepaliveClientConfig_ToOption(t *testing.T) {
	Convey("TestKeepaliveClientConfig_ToOption", t, func() {
		Convey("当提供有效的保活配置时", func() {
			config := &KeepaliveClientConfig{
				Time:                30 * time.Second,
				Timeout:             10 * time.Second,
				PermitWithoutStream: true,
			}
			option := config.ToOption()
			So(option, ShouldNotBeNil)
		})

		Convey("当使用零值配置时", func() {
			config := &KeepaliveClientConfig{}
			option := config.ToOption()
			So(option, ShouldNotBeNil)
		})
	})
}

// ============================================
// ClientConfig整体测试
// ============================================

// TestClientConfig_DefaultValues 测试ClientConfig的默认值。
func TestClientConfig_DefaultValues(t *testing.T) {
	Convey("TestClientConfig_DefaultValues", t, func() {
		Convey("当ClientConfig使用默认值创建时", func() {
			config := ClientConfig{}
			So(config.Host, ShouldEqual, "")
			So(config.Port, ShouldEqual, 0)
			So(config.TLS.Enabled, ShouldBeFalse)
			So(config.Auth.Enabled, ShouldBeFalse)
			So(config.Keepalive.PermitWithoutStream, ShouldBeFalse) // bool的零值是false
			So(config.ServiceConfig, ShouldBeNil)
		})

		Convey("当ClientConfig使用自定义值初始化时", func() {
			config := ClientConfig{
				BaseConfig: BaseConfig{
					Host: "0.0.0.0",
					Port: 9090,
				},
				TLS: TlsClientConfig{
					Enabled: true,
					Domain:  "example.com",
					Cert:    "test-cert",
				},
				Auth: AuthClientConfig{
					Enabled:         true,
					Authorize:       ProtocolAccessKeySecret,
					AccessKeyID:     "test-id",
					AccessKeySecret: "test-secret",
				},
				Keepalive: KeepaliveClientConfig{
					Time:                1 * time.Minute,
					PermitWithoutStream: true,
				},
				ServiceConfig: map[string]any{
					"retryPolicy": map[string]any{
						"maxAttempts": 3,
					},
				},
			}
			So(config.Host, ShouldEqual, "0.0.0.0")
			So(config.Port, ShouldEqual, 9090)
			So(config.TLS.Enabled, ShouldBeTrue)
			So(config.TLS.Domain, ShouldEqual, "example.com")
			So(config.TLS.Cert, ShouldEqual, "test-cert")
			So(config.Auth.Enabled, ShouldBeTrue)
			So(config.Auth.Authorize, ShouldEqual, ProtocolAccessKeySecret)
			So(config.Auth.AccessKeyID, ShouldEqual, "test-id")
			So(config.Auth.AccessKeySecret, ShouldEqual, "test-secret")
			So(config.Keepalive.Time, ShouldEqual, 1*time.Minute)
			So(config.Keepalive.PermitWithoutStream, ShouldBeTrue)
			So(config.ServiceConfig, ShouldNotBeNil)
			retryPolicy, ok := config.ServiceConfig["retryPolicy"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(retryPolicy["maxAttempts"], ShouldEqual, 3)
		})
	})
}

// TestClientConfig_ToOptions_NilConfig 测试ClientConfig为nil时的行为。
func TestClientConfig_ToOptions_NilConfig(t *testing.T) {
	Convey("TestClientConfig_ToOptions_NilConfig", t, func() {
		Convey("当ClientConfig为nil时", func() {
			var config *ClientConfig
			lis, options, err := config.ToOptions()
			So(err, ShouldBeNil)
			So(lis, ShouldEqual, "")
			So(options, ShouldBeNil)
		})
	})
}

// TestClientConfig_ToOptions_MinimalConfig 测试最小配置时的行为。
func TestClientConfig_ToOptions_MinimalConfig(t *testing.T) {
	Convey("TestClientConfig_ToOptions_MinimalConfig", t, func() {
		Convey("当提供最小配置时", func() {
			config := &ClientConfig{
				BaseConfig: BaseConfig{
					Host: "127.0.0.1",
					Port: 9090,
				},
				// 其他配置使用默认值(禁用)
			}

			lis, options, err := config.ToOptions()
			So(err, ShouldBeNil)
			So(lis, ShouldEqual, "127.0.0.1:9090")
			// 即使所有功能都禁用，也应该有TLS选项(不安全的连接)
			So(options, ShouldNotBeNil)
		})
	})
}

// TestClientConfig_ToOptions_ValidConfig 测试有效配置时的行为。
func TestClientConfig_ToOptions_ValidConfig(t *testing.T) {
	Convey("TestClientConfig_ToOptions_ValidConfig", t, func() {
		Convey("当提供有效的完整配置时", func() {
			// 生成测试证书
			_, cert, err := tls.GenerateTestSignedCert()
			So(err, ShouldBeNil)

			config := &ClientConfig{
				BaseConfig: BaseConfig{
					Host: "localhost",
					Port: 8080,
				},
				TLS: TlsClientConfig{
					Enabled: true,
					Domain:  "localhost",
					Cert:    cert,
				},
				Auth: AuthClientConfig{
					Enabled:         true,
					Authorize:       ProtocolAccessKeySecret,
					AccessKeyID:     "test-key-id",
					AccessKeySecret: "test-key-secret",
				},
				Keepalive: KeepaliveClientConfig{
					Time:                30 * time.Second,
					Timeout:             10 * time.Second,
					PermitWithoutStream: true,
				},
				ServiceConfig: map[string]any{
					"loadBalancingConfig": []map[string]any{
						{"round_robin": map[string]any{}},
					},
					"methodConfig": []map[string]any{
						{
							"name": []map[string]any{
								{"service": "test.Service"},
							},
							"retryPolicy": map[string]any{
								"maxAttempts":          3,
								"initialBackoff":       "0.1s",
								"maxBackoff":           "1s",
								"backoffMultiplier":    2.0,
								"retryableStatusCodes": []string{"UNAVAILABLE"},
							},
						},
					},
				},
			}

			lis, options, err := config.ToOptions()
			So(err, ShouldBeNil)
			So(lis, ShouldEqual, "localhost:8080")
			So(options, ShouldNotBeNil)
			So(len(options), ShouldBeGreaterThan, 0)
		})
	})
}

// TestClientConfig_ToOptions_InvalidTLSConfig 测试TLS配置无效时的行为。
func TestClientConfig_ToOptions_InvalidTLSConfig(t *testing.T) {
	Convey("TestClientConfig_ToOptions_InvalidTLSConfig", t, func() {
		Convey("当TLS配置无效时", func() {
			config := &ClientConfig{
				BaseConfig: BaseConfig{
					Host: "localhost",
					Port: 8080,
				},
				TLS: TlsClientConfig{
					Enabled: true,
					Domain:  "localhost",
					// 不提供Cert或CertFilePath，这是无效的
				},
			}

			lis, options, err := config.ToOptions()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "invalid tls config")
			So(lis, ShouldEqual, "localhost:8080")
			So(options, ShouldBeNil)
		})
	})
}

// TestClientConfig_ToOptions_InvalidAuthConfig 测试身份验证配置无效时的行为。
func TestClientConfig_ToOptions_InvalidAuthConfig(t *testing.T) {
	Convey("TestClientConfig_ToOptions_InvalidAuthConfig", t, func() {
		Convey("当身份验证配置无效时", func() {
			config := &ClientConfig{
				BaseConfig: BaseConfig{
					Host: "localhost",
					Port: 8080,
				},
				Auth: AuthClientConfig{
					Enabled:   true,
					Authorize: "invalid-protocol", // 无效的协议
				},
			}

			lis, options, err := config.ToOptions()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "invalid auth protocol")
			So(lis, ShouldEqual, "localhost:8080")
			// 注意：即使身份验证配置无效，TLS选项可能已经被添加到options中
			// 所以options可能不为nil
			_ = options // 避免未使用变量错误
		})
	})
}

// TestClientConfig_ToOptions_InvalidServiceConfig 测试服务配置无效时的行为。
func TestClientConfig_ToOptions_InvalidServiceConfig(t *testing.T) {
	Convey("TestClientConfig_ToOptions_InvalidServiceConfig", t, func() {
		Convey("当服务配置包含无法序列化的数据时", func() {
			// 创建一个包含无法序列化值的map
			invalidMap := make(map[string]any)
			invalidMap["channel"] = make(chan int) // channel无法被JSON序列化

			config := &ClientConfig{
				BaseConfig: BaseConfig{
					Host: "localhost",
					Port: 8080,
				},
				ServiceConfig: invalidMap,
			}

			lis, options, err := config.ToOptions()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "json: unsupported type")
			So(lis, ShouldEqual, "localhost:8080")
			// 注意：即使服务配置无效，TLS选项可能已经被添加到options中
			// 所以options可能不为nil
			_ = options // 避免未使用变量错误
		})
	})
}

// ============================================
// 辅助函数测试
// ============================================

// TestDescribeClientConfig 测试DescribeClientConfig函数。
// 注意：由于DescribeClientConfig依赖于config.DescribeConfig，这里主要测试函数调用不 panic。
func TestDescribeClientConfig(t *testing.T) {
	Convey("TestDescribeClientConfig", t, func() {
		Convey("当不提供名称参数时", func() {
			descriptions := DescribeClientConfig()
			So(descriptions, ShouldNotBeNil)
		})

		Convey("当提供名称参数时", func() {
			descriptions := DescribeClientConfig("client1", "client2")
			So(descriptions, ShouldNotBeNil)
		})
	})
}

// TestExtractClientConfig 测试ExtractClientConfig函数。
// 注意：由于ExtractClientConfig依赖于config.ExtractConfig和Viper，这里主要测试函数签名和基本行为。
func TestExtractClientConfig(t *testing.T) {
	Convey("TestExtractClientConfig", t, func() {
		Convey("ExtractClientConfig函数应该存在", func() {
			// 这个测试主要验证函数可以正常编译和调用
			// 实际的数据测试需要完整的Viper配置环境
			So(ExtractClientConfig, ShouldNotBeNil)
		})
	})
}
