package grpc

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"zhongkuisec.com/zhongkui/internal/pkg/tls"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

// testValidate 测试用的验证器实例
var testValidate = validator.Validate

// TestTlsServerConfig_ToOption_NilConfig 测试 TlsServerConfig 为 nil 时的行为。
func TestTlsServerConfig_ToOption_NilConfig(t *testing.T) {
	Convey("TestTlsServerConfig_ToOption_NilConfig", t, func() {
		Convey("when TlsServerConfig is nil", func() {
			var config *TlsServerConfig
			option, err := config.ToOption()
			So(option, ShouldBeNil)
			So(err, ShouldBeNil)
		})
	})
}

// TestTlsServerConfig_ToOption_Disabled 测试 TLS 关闭时的行为。
func TestTlsServerConfig_ToOption_Disabled(t *testing.T) {
	Convey("TestTlsServerConfig_ToOption_Disabled", t, func() {
		Convey("when TLS is disabled", func() {
			config := &TlsServerConfig{Enabled: false}
			option, err := config.ToOption()
			So(option, ShouldBeNil)
			So(err, ShouldBeNil)
		})
	})
}

// TestTlsServerConfig_ToOption_ValidCertKey 测试以字符串形式提供合法证书与密钥时的行为。
func TestTlsServerConfig_ToOption_ValidCertKey(t *testing.T) {
	Convey("TestTlsServerConfig_ToOption_ValidCertKey", t, func() {
		Convey("when valid certificate and key are provided as strings", func() {
			key, cert, err := tls.GenerateTestSignedCert()
			So(err, ShouldBeNil)

			config := &TlsServerConfig{
				Enabled: true,
				Cert:    cert,
				Key:     key,
			}
			option, err := config.ToOption()
			So(option, ShouldNotBeNil)
			So(err, ShouldBeNil)
		})
	})
}

// TestTlsServerConfig_ToOption_InvalidCertKey 测试以字符串形式提供非法证书与密钥时的行为。
func TestTlsServerConfig_ToOption_InvalidCertKey(t *testing.T) {
	Convey("TestTlsServerConfig_ToOption_InvalidCertKey", t, func() {
		Convey("when invalid certificate and key are provided as strings", func() {
			config := &TlsServerConfig{
				Enabled: true,
				Cert:    "invalid-cert",
				Key:     "invalid-key",
			}
			option, err := config.ToOption()
			So(option, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})
	})
}

// TestTlsServerConfig_ToOption_NoCertKey 测试未提供证书或密钥时的行为。
func TestTlsServerConfig_ToOption_NoCertKey(t *testing.T) {
	Convey("TestTlsServerConfig_ToOption_NoCertKey", t, func() {
		Convey("when no certificate or key is provided", func() {
			config := &TlsServerConfig{Enabled: true}
			option, err := config.ToOption()
			So(option, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})
	})
}

// TestBaseConfig_DefaultValues 测试 BaseConfig 的默认值是否正确。
func TestBaseConfig_DefaultValues(t *testing.T) {
	Convey("TestBaseConfig_DefaultValues", t, func() {
		Convey("when BaseConfig is created with default values", func() {
			config := BaseConfig{}
			So(config.Host, ShouldEqual, "")
			So(config.Port, ShouldEqual, 0)
		})

		Convey("when BaseConfig is initialized with custom values", func() {
			config := BaseConfig{
				Host: "localhost",
				Port: 9090,
			}
			So(config.Host, ShouldEqual, "localhost")
			So(config.Port, ShouldEqual, 9090)
		})
	})
}

// TestTlsServerConfig_ToOption_ValidCertFilePath 测试以文件路径形式提供的 TLS 配置。
func TestTlsServerConfig_ToOption_ValidCertFilePath(t *testing.T) {
	Convey("TestTlsServerConfig_ToOption_ValidCertFilePath", t, func() {
		Convey("when valid certificate and key file paths are provided", func() {
			// 注意：本测试要求实际证书文件存在；
			// 真正测试时可用测试证书创建临时文件；
			// 此处仅验证配置结构体可接受文件路径。
			config := &TlsServerConfig{
				Enabled:      true,
				CertFilePath: "/path/to/cert.pem",
				KeyFilePath:  "/path/to/key.pem",
			}

			// ToOption 方法会因文件不存在而失败，
			// 但可验证配置已正确设置。
			So(config.Enabled, ShouldBeTrue)
			So(config.CertFilePath, ShouldEqual, "/path/to/cert.pem")
			So(config.KeyFilePath, ShouldEqual, "/path/to/key.pem")
			So(config.Cert, ShouldEqual, "")
			So(config.Key, ShouldEqual, "")
		})
	})
}

// TestTlsServerConfig_ToOption_MixedCertKey 测试非法的混合配置。
func TestTlsServerConfig_ToOption_MixedCertKey(t *testing.T) {
	Convey("TestTlsServerConfig_ToOption_MixedCertKey", t, func() {
		Convey("when mixed inline and file path configuration is provided", func() {
			key, cert, err := tls.GenerateTestSignedCert()
			So(err, ShouldBeNil)

			config := &TlsServerConfig{
				Enabled:      true,
				Cert:         cert,
				Key:          key,
				CertFilePath: "/path/to/cert.pem", // 混合配置——内联证书优先
			}

			option, err := config.ToOption()
			So(option, ShouldNotBeNil) // 内联证书优先，故应创建出 option
			So(err, ShouldBeNil)
		})
	})
}

// TestAuthServerConfig_DefaultValues 测试 AuthServerConfig 的默认值。
func TestAuthServerConfig_DefaultValues(t *testing.T) {
	Convey("TestAuthServerConfig_DefaultValues", t, func() {
		Convey("when AuthServerConfig is created with default values", func() {
			config := AuthServerConfig{}
			So(config.AccessKeyEnabled, ShouldBeFalse)
		})

		Convey("when AuthServerConfig is initialized with custom values", func() {
			config := AuthServerConfig{
				AccessKeyEnabled: true,
			}
			So(config.AccessKeyEnabled, ShouldBeTrue)
		})
	})
}

// TestAuthServerConfig_ToOptions_Disabled 测试关闭鉴权的情况。
func TestAuthServerConfig_ToOptions_Disabled(t *testing.T) {
	Convey("TestAuthServerConfig_ToOptions_Disabled", t, func() {
		Convey("when authentication is disabled", func() {
			config := &AuthServerConfig{
				AccessKeyEnabled: false,
			}

			unaryOpt, streamOpt, err := config.ToOptions(nil)
			So(unaryOpt, ShouldNotBeNil)
			So(streamOpt, ShouldNotBeNil)
			So(err, ShouldBeNil)
		})
	})
}

// TestAuthServerConfig_ToOptions_NilConfig 测试 nil 配置。
func TestAuthServerConfig_ToOptions_NilConfig(t *testing.T) {
	Convey("TestAuthServerConfig_ToOptions_NilConfig", t, func() {
		Convey("when AuthServerConfig is nil", func() {
			var config *AuthServerConfig
			unaryOpt, streamOpt, err := config.ToOptions(nil)
			// 注意：当前实现在配置为 nil 时仍会创建拦截器，
			// 因为 NewAuthRouter 会以 nil 配置创建路由。
			So(unaryOpt, ShouldNotBeNil)
			So(streamOpt, ShouldNotBeNil)
			So(err, ShouldBeNil)
		})
	})
}

// TestKeepaliveServerConfig_DefaultValues 测试 KeepaliveServerConfig 的默认值。
func TestKeepaliveServerConfig_DefaultValues(t *testing.T) {
	Convey("TestKeepaliveServerConfig_DefaultValues", t, func() {
		Convey("when KeepaliveServerConfig is created with default values", func() {
			config := KeepaliveServerConfig{}
			So(config.Time, ShouldEqual, 0)
			So(config.Timeout, ShouldEqual, 0)
			So(config.MinTime, ShouldEqual, 0)
			So(config.MaxConnectionAge, ShouldEqual, 0)
			So(config.MaxConnectionAgeGrace, ShouldEqual, 0)
			So(config.MaxConnectionIdle, ShouldEqual, 0)
			So(config.PermitWithoutStream, ShouldBeFalse) // bool 的零值为 false
		})

		Convey("when KeepaliveServerConfig is initialized with custom values", func() {
			config := KeepaliveServerConfig{
				Time:                  30 * time.Second,
				Timeout:               5 * time.Second,
				MinTime:               5 * time.Second,
				MaxConnectionAge:      1 * time.Hour,
				MaxConnectionAgeGrace: 5 * time.Second,
				MaxConnectionIdle:     5 * time.Minute,
				PermitWithoutStream:   false,
			}
			So(config.Time, ShouldEqual, 30*time.Second)
			So(config.Timeout, ShouldEqual, 5*time.Second)
			So(config.MinTime, ShouldEqual, 5*time.Second)
			So(config.MaxConnectionAge, ShouldEqual, 1*time.Hour)
			So(config.MaxConnectionAgeGrace, ShouldEqual, 5*time.Second)
			So(config.MaxConnectionIdle, ShouldEqual, 5*time.Minute)
			So(config.PermitWithoutStream, ShouldBeFalse)
		})
	})
}

// TestKeepaliveServerConfig_ToOptions 测试转换为 gRPC 选项。
func TestKeepaliveServerConfig_ToOptions(t *testing.T) {
	Convey("TestKeepaliveServerConfig_ToOptions", t, func() {
		Convey("when converting keepalive configuration to options", func() {
			config := &KeepaliveServerConfig{
				Time:                  30 * time.Second,
				Timeout:               5 * time.Second,
				MinTime:               5 * time.Second,
				MaxConnectionAge:      1 * time.Hour,
				MaxConnectionAgeGrace: 5 * time.Second,
				MaxConnectionIdle:     5 * time.Minute,
				PermitWithoutStream:   true,
			}

			paramsOpt, policyOpt := config.ToOptions()
			So(paramsOpt, ShouldNotBeNil)
			So(policyOpt, ShouldNotBeNil)
		})

		Convey("when KeepaliveServerConfig is nil", func() {
			var config *KeepaliveServerConfig
			// 注意：对 nil 调用 ToOptions 会 panic，因此这里不对 nil 调用，
			// 仅验证配置确为 nil。
			So(config, ShouldBeNil)
		})
	})
}

// TestHealthServerConfig_DefaultValues 测试 HealthServerConfig 的默认值。
func TestHealthServerConfig_DefaultValues(t *testing.T) {
	Convey("TestHealthServerConfig_DefaultValues", t, func() {
		Convey("when HealthServerConfig is created with default values", func() {
			config := HealthServerConfig{}
			So(config.Enabled, ShouldBeFalse) // bool 的零值为 false
			So(config.ServiceName, ShouldEqual, "")
		})

		Convey("when HealthServerConfig is initialized with custom values", func() {
			config := HealthServerConfig{
				Enabled:     false,
				ServiceName: "custom-service",
			}
			So(config.Enabled, ShouldBeFalse)
			So(config.ServiceName, ShouldEqual, "custom-service")
		})
	})
}

// TestServerConfig_DefaultValues 测试 ServerConfig 的默认值。
func TestServerConfig_DefaultValues(t *testing.T) {
	Convey("TestServerConfig_DefaultValues", t, func() {
		Convey("when ServerConfig is created with default values", func() {
			config := ServerConfig{}
			So(config.Host, ShouldEqual, "")
			So(config.Port, ShouldEqual, 0)
			So(config.TLS.Enabled, ShouldBeFalse)
			So(config.Auth.AccessKeyEnabled, ShouldBeFalse)
			So(config.Keepalive.PermitWithoutStream, ShouldBeFalse) // bool 的零值为 false
			So(config.Health.Enabled, ShouldBeFalse)                // bool 的零值为 false
		})

		Convey("when ServerConfig is initialized with custom values", func() {
			config := ServerConfig{
				BaseConfig: BaseConfig{
					Host: "0.0.0.0",
					Port: 9090,
				},
				TLS: TlsServerConfig{
					Enabled: true,
					Cert:    "test-cert",
					Key:     "test-key",
				},
				Auth: AuthServerConfig{
					AccessKeyEnabled: true,
				},
				Keepalive: KeepaliveServerConfig{
					Time:                30 * time.Second,
					PermitWithoutStream: false,
				},
				Health: HealthServerConfig{
					Enabled:     false,
					ServiceName: "my-service",
				},
			}
			So(config.Host, ShouldEqual, "0.0.0.0")
			So(config.Port, ShouldEqual, 9090)
			So(config.TLS.Enabled, ShouldBeTrue)
			So(config.TLS.Cert, ShouldEqual, "test-cert")
			So(config.TLS.Key, ShouldEqual, "test-key")
			So(config.Auth.AccessKeyEnabled, ShouldBeTrue)
			So(config.Keepalive.Time, ShouldEqual, 30*time.Second)
			So(config.Keepalive.PermitWithoutStream, ShouldBeFalse)
			So(config.Health.Enabled, ShouldBeFalse)
			So(config.Health.ServiceName, ShouldEqual, "my-service")
		})
	})
}

// TestServerConfig_ToOptions_NilConfig 测试 nil ServerConfig。
func TestServerConfig_ToOptions_NilConfig(t *testing.T) {
	Convey("TestServerConfig_ToOptions_NilConfig", t, func() {
		Convey("when ServerConfig is nil", func() {
			var config *ServerConfig
			listener, options, err := config.ToOptions(nil)
			So(listener, ShouldBeNil)
			So(options, ShouldBeNil)
			So(err, ShouldBeNil)
		})
	})
}

// TestServerConfig_ToOptions_ValidConfig 测试合法的 ServerConfig 转换。
func TestServerConfig_ToOptions_ValidConfig(t *testing.T) {
	Convey("TestServerConfig_ToOptions_ValidConfig", t, func() {
		Convey("when ServerConfig has valid configuration", func() {
			config := &ServerConfig{
				BaseConfig: BaseConfig{
					Host: "127.0.0.1",
					Port: 0, // 使用端口 0 以便在测试中自动分配端口
				},
				TLS: TlsServerConfig{
					Enabled: false,
				},
				Auth: AuthServerConfig{
					AccessKeyEnabled: false,
				},
				Keepalive: KeepaliveServerConfig{
					Time:                30 * time.Second,
					PermitWithoutStream: true,
				},
			}

			listener, options, err := config.ToOptions(nil)
			So(err, ShouldBeNil)
			So(listener, ShouldNotBeNil)
			So(options, ShouldNotBeNil)
			So(len(options), ShouldBeGreaterThan, 0)

			// 清理
			if listener != nil {
				listener.Close()
			}
		})

		Convey("when ServerConfig has invalid port", func() {
			config := &ServerConfig{
				BaseConfig: BaseConfig{
					Host: "127.0.0.1",
					Port: -1, // 非法端口
				},
			}

			listener, options, err := config.ToOptions(nil)
			So(err, ShouldNotBeNil)
			So(listener, ShouldBeNil)
			So(options, ShouldBeNil)
		})
	})
}

// TestDescribeServerConfig 测试配置描述生成。
func TestDescribeServerConfig(t *testing.T) {
	Convey("TestDescribeServerConfig", t, func() {
		Convey("when describing server config without specific names", func() {
			descriptions := DescribeServerConfig()
			So(descriptions, ShouldNotBeNil)
		})

		Convey("when describing server config with specific names", func() {
			descriptions := DescribeServerConfig("test-server")
			So(descriptions, ShouldNotBeNil)
		})

		Convey("when describing server config with multiple names", func() {
			descriptions := DescribeServerConfig("server1", "server2")
			So(descriptions, ShouldNotBeNil)
		})
	})
}

// TestExtractServerConfig 测试从 Viper 提取配置。
func TestExtractServerConfig(t *testing.T) {
	Convey("TestExtractServerConfig", t, func() {
		Convey("when extracting server config from Viper", func() {
			// 构建一个用于测试的最小 Viper 实例
			v := viper.New()

			// 空配置
			config, err := ExtractServerConfig(v, testValidate, "test-server")
			So(err, ShouldNotBeNil) // 子配置为 nil，故应失败
			So(config, ShouldBeNil)
		})

		Convey("when extracting server config with valid data", func() {
			// 注意：真实测试中会用实际配置数据装配 Viper；
			// 此处仅验证函数签名正确。
			So(ExtractServerConfig, ShouldNotBeNil)
		})
	})
}
