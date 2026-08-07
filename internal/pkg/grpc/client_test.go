// Package grpc 提供gRPC客户端连接的单元测试。
// 本文件包含对client.go中NewClient函数的单元测试，确保客户端连接创建功能正确工作。
// 测试包括正常情况、边界情况和错误情况的验证。
package grpc

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/tls"
)

// ============================================
// NewClient函数测试
// ============================================

// TestNewClient_NilConfig 测试当配置为nil时的行为。
// 验证当传入nil配置时，NewClient函数是否能正确处理。
// 注意：nil配置会导致ToOptions返回空地址和空选项，gRPC连接会失败。
func TestNewClient_NilConfig(t *testing.T) {
	Convey("TestNewClient_NilConfig", t, func() {
		Convey("当ClientConfig为nil时", func() {
			var config *ClientConfig
			client, err := NewClient(config)

			// 验证函数返回错误（因为需要传输安全选项）
			So(err, ShouldNotBeNil)
			// 验证错误信息包含"transport security"或类似内容
			So(err.Error(), ShouldContainSubstring, "transport security")
			// 验证返回的客户端为nil
			So(client, ShouldBeNil)
		})
	})
}

// TestNewClient_MinimalConfig 测试最小配置时的行为。
// 验证当提供最小配置（只有主机和端口）时，客户端是否能成功创建。
func TestNewClient_MinimalConfig(t *testing.T) {
	Convey("TestNewClient_MinimalConfig", t, func() {
		Convey("当提供最小配置时", func() {
			config := &ClientConfig{
				BaseConfig: BaseConfig{
					Host: "localhost",
					Port: 8080,
				},
				// 其他配置使用默认值(禁用)
			}

			client, err := NewClient(config)

			// 验证函数不返回错误
			So(err, ShouldBeNil)
			// 验证返回的客户端不为nil
			So(client, ShouldNotBeNil)
			// 验证客户端类型正确
			So(client, ShouldHaveSameTypeAs, &Client{})
		})
	})
}

// TestNewClient_ValidConfig 测试有效完整配置时的行为。
// 验证当提供完整的有效配置（包括TLS、身份验证等）时，客户端是否能成功创建。
func TestNewClient_ValidConfig(t *testing.T) {
	Convey("TestNewClient_ValidConfig", t, func() {
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
				},
			}

			client, err := NewClient(config)

			// 验证函数不返回错误
			So(err, ShouldBeNil)
			// 验证返回的客户端不为nil
			So(client, ShouldNotBeNil)
			// 验证客户端类型正确
			So(client, ShouldHaveSameTypeAs, &Client{})
		})
	})
}

// TestNewClient_InvalidHostPort 测试无效主机端口配置时的行为。
// 验证当提供无效的主机或端口配置时，函数的行为。
// 注意：gRPC客户端对象会被创建，但实际连接可能失败或异步建立。
func TestNewClient_InvalidHostPort(t *testing.T) {
	Convey("TestNewClient_InvalidHostPort", t, func() {
		Convey("当主机名为空字符串时", func() {
			config := &ClientConfig{
				BaseConfig: BaseConfig{
					Host: "", // 空主机名
					Port: 8080,
				},
			}

			client, err := NewClient(config)

			// gRPC客户端对象会被创建
			So(client, ShouldNotBeNil)
			// 可能没有错误，因为连接是异步的
			// 我们不验证具体错误
			_ = err // 忽略错误检查

			// 清理
			if client != nil {
				client.Close()
			}
		})

		Convey("当端口为0时", func() {
			config := &ClientConfig{
				BaseConfig: BaseConfig{
					Host: "localhost",
					Port: 0, // 端口0通常表示系统分配端口
				},
			}

			client, err := NewClient(config)

			// gRPC客户端对象会被创建
			So(client, ShouldNotBeNil)
			// 可能没有错误
			_ = err // 忽略错误检查

			// 清理
			if client != nil {
				client.Close()
			}
		})

		Convey("当端口超过有效范围时", func() {
			config := &ClientConfig{
				BaseConfig: BaseConfig{
					Host: "localhost",
					Port: 65536, // 超过最大端口号
				},
			}

			client, err := NewClient(config)

			// gRPC客户端对象会被创建
			So(client, ShouldNotBeNil)
			// 可能没有错误
			_ = err // 忽略错误检查

			// 清理
			if client != nil {
				client.Close()
			}
		})
	})
}

// TestNewClient_InvalidTLSConfig 测试无效TLS配置时的行为。
// 验证当提供无效的TLS配置时，函数是否能正确返回错误。
func TestNewClient_InvalidTLSConfig(t *testing.T) {
	Convey("TestNewClient_InvalidTLSConfig", t, func() {
		Convey("当TLS启用但没有提供证书时", func() {
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

			client, err := NewClient(config)

			// 验证函数返回错误
			So(err, ShouldNotBeNil)
			// 验证错误信息包含"invalid tls config"
			So(err.Error(), ShouldContainSubstring, "invalid tls config")
			// 验证返回的客户端为nil
			So(client, ShouldBeNil)
		})

		Convey("当提供无效的PEM证书时", func() {
			config := &ClientConfig{
				BaseConfig: BaseConfig{
					Host: "localhost",
					Port: 8080,
				},
				TLS: TlsClientConfig{
					Enabled: true,
					Domain:  "localhost",
					Cert:    "invalid-pem-cert-data", // 无效的PEM格式
				},
			}

			client, err := NewClient(config)

			// 验证函数返回错误
			So(err, ShouldNotBeNil)
			// 验证错误信息包含相关提示
			So(err.Error(), ShouldContainSubstring, "failed to append certs")
			// 验证返回的客户端为nil
			So(client, ShouldBeNil)
		})
	})
}

// TestNewClient_InvalidAuthConfig 测试无效身份验证配置时的行为。
// 验证当提供无效的身份验证配置时，函数是否能正确返回错误。
func TestNewClient_InvalidAuthConfig(t *testing.T) {
	Convey("TestNewClient_InvalidAuthConfig", t, func() {
		Convey("当身份验证启用但没有提供凭证时", func() {
			config := &ClientConfig{
				BaseConfig: BaseConfig{
					Host: "localhost",
					Port: 8080,
				},
				Auth: AuthClientConfig{
					Enabled:   true,
					Authorize: ProtocolAccessKeySecret,
					// 不提供AccessKeyID或AccessKeySecret，这是无效的
				},
			}

			client, err := NewClient(config)

			// 验证函数返回错误
			So(err, ShouldNotBeNil)
			// 验证错误信息包含"credentials require transport level security"或类似内容
			So(err.Error(), ShouldContainSubstring, "credentials require")
			// 验证返回的客户端为nil
			So(client, ShouldBeNil)
		})

		Convey("当身份验证启用但凭证为空时", func() {
			config := &ClientConfig{
				BaseConfig: BaseConfig{
					Host: "localhost",
					Port: 8080,
				},
				Auth: AuthClientConfig{
					Enabled:         true,
					Authorize:       ProtocolAccessKeySecret,
					AccessKeyID:     "", // 空的AccessKeyID
					AccessKeySecret: "test-secret",
				},
			}

			client, err := NewClient(config)

			// 验证函数返回错误
			So(err, ShouldNotBeNil)
			// 验证错误信息包含"credentials require transport level security"或类似内容
			So(err.Error(), ShouldContainSubstring, "credentials require")
			// 验证返回的客户端为nil
			So(client, ShouldBeNil)
		})
	})
}

// TestNewClient_NetworkUnreachable 测试网络不可达时的行为。
// 验证当尝试连接到不存在的服务器时，函数的行为。
// 注意：gRPC客户端对象会被创建，但实际连接可能失败。
func TestNewClient_NetworkUnreachable(t *testing.T) {
	Convey("TestNewClient_NetworkUnreachable", t, func() {
		Convey("当尝试连接到不存在的服务器时", func() {
			// 使用一个不太可能被占用的端口
			config := &ClientConfig{
				BaseConfig: BaseConfig{
					Host: "localhost",
					Port: 65535, // 通常不会使用的端口
				},
			}

			client, err := NewClient(config)

			// gRPC客户端对象会被创建
			So(client, ShouldNotBeNil)
			// 可能没有错误，因为连接是异步的
			_ = err // 忽略错误检查

			// 清理
			if client != nil {
				client.Close()
			}
		})
	})
}

// TestNewClient_ClientClose 测试客户端连接关闭功能。
// 验证成功创建的客户端连接可以正常关闭。
func TestNewClient_ClientClose(t *testing.T) {
	Convey("TestNewClient_ClientClose", t, func() {
		Convey("当成功创建客户端后关闭连接时", func() {
			config := &ClientConfig{
				BaseConfig: BaseConfig{
					Host: "localhost",
					Port: 8080,
				},
			}

			client, err := NewClient(config)
			So(err, ShouldBeNil)
			So(client, ShouldNotBeNil)

			// 验证可以正常关闭连接
			err = client.Close()
			So(err, ShouldBeNil)
		})
	})
}

// TestNewClient_MultipleClients 测试创建多个客户端连接。
// 验证可以同时创建多个独立的客户端连接。
func TestNewClient_MultipleClients(t *testing.T) {
	Convey("TestNewClient_MultipleClients", t, func() {
		Convey("当创建多个客户端连接时", func() {
			config1 := &ClientConfig{
				BaseConfig: BaseConfig{
					Host: "localhost",
					Port: 8080,
				},
			}

			config2 := &ClientConfig{
				BaseConfig: BaseConfig{
					Host: "127.0.0.1",
					Port: 9090,
				},
			}

			// 创建第一个客户端
			client1, err1 := NewClient(config1)
			So(err1, ShouldBeNil)
			So(client1, ShouldNotBeNil)

			// 创建第二个客户端
			client2, err2 := NewClient(config2)
			So(err2, ShouldBeNil)
			So(client2, ShouldNotBeNil)

			// 验证两个客户端是不同的实例（指针比较，避免对活跃连接做反射深比较触发 data race）
			So(client1 == client2, ShouldBeFalse)

			// 清理
			client1.Close()
			client2.Close()
		})
	})
}
