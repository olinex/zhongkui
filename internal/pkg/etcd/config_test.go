package etcd

import (
	"context"
	"os"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	clientv3 "go.etcd.io/etcd/client/v3"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/tls"
)

// TestPREFIX 测试 etcd 配置前缀常量
func TestPREFIX(t *testing.T) {
	Convey("测试 etcd 配置前缀常量", t, func() {
		Convey("PREFIX 常量值应为 etcd", func() {
			So(PREFIX, ShouldEqual, "etcd")
		})
	})
}

// TestToStandardConfig_Disabled 测试 TLS 未启用时的行为
func TestToStandardConfig_Disabled(t *testing.T) {
	Convey("测试 TLS 未启用时的行为", t, func() {
		Convey("当 Enabled 为 false 时应返回 nil 且无错误", func() {
			tlsConfig := &TLSConfig{Enabled: false}
			config, err := tlsConfig.ToStandardConfig()
			So(config, ShouldBeNil)
			So(err, ShouldBeNil)
		})
	})
}

// TestToStandardConfig_NilReceiver 测试接收者为 nil 时的行为
func TestToStandardConfig_NilReceiver(t *testing.T) {
	Convey("测试接收者为 nil 时的行为", t, func() {
		Convey("当 TLSConfig 为 nil 时应返回 nil 且无错误", func() {
			var tlsConfig *TLSConfig
			config, err := tlsConfig.ToStandardConfig()
			So(config, ShouldBeNil)
			So(err, ShouldBeNil)
		})
	})
}

// TestToStandardConfig_WithCert 测试直接提供证书时的行为
func TestToStandardConfig_WithCert(t *testing.T) {
	Convey("测试直接提供内联证书时的行为", t, func() {
		Convey("当提供有效的内联 PEM 证书时应成功构建 TLS 配置", func() {
			_, cert, err := tls.GenerateTestSignedCert()
			So(err, ShouldBeNil)
			tlsConfig := &TLSConfig{
				Enabled: true,
				Cert:    cert,
				Domain:  "example.com",
			}
			config, err := tlsConfig.ToStandardConfig()
			So(err, ShouldBeNil)
			So(config, ShouldNotBeNil)
			So(config.ServerName, ShouldEqual, tlsConfig.Domain)
			So(config.RootCAs, ShouldNotBeNil)
		})
	})
}

// TestToStandardConfig_WithCertFile 测试提供证书文件路径时的行为
func TestToStandardConfig_WithCertFile(t *testing.T) {
	Convey("测试提供证书文件路径时的行为", t, func() {
		Convey("当提供有效的证书文件路径时应成功构建 TLS 配置", func() {
			certFile, err := os.CreateTemp("", "test-cert-*.pem")
			So(err, ShouldBeNil)
			defer os.Remove(certFile.Name())

			_, cert, err := tls.GenerateTestSignedCert()
			So(err, ShouldBeNil)
			_, err = certFile.WriteString(cert)
			So(err, ShouldBeNil)
			err = certFile.Close()
			So(err, ShouldBeNil)

			tlsConfig := &TLSConfig{
				Enabled:      true,
				CertFilePath: certFile.Name(),
				Domain:       "example.com",
			}
			config, err := tlsConfig.ToStandardConfig()
			So(err, ShouldBeNil)
			So(config, ShouldNotBeNil)
			So(config.ServerName, ShouldEqual, tlsConfig.Domain)
			So(config.RootCAs, ShouldNotBeNil)
		})
	})
}

// TestToStandardConfig_CertPriorityOverFilePath 测试 Cert 优先级高于 CertFilePath
func TestToStandardConfig_CertPriorityOverFilePath(t *testing.T) {
	Convey("测试内联证书优先级高于证书文件路径", t, func() {
		Convey("当同时提供 Cert 和 CertFilePath 时应优先使用 Cert", func() {
			_, cert, err := tls.GenerateTestSignedCert()
			So(err, ShouldBeNil)
			tlsConfig := &TLSConfig{
				Enabled:      true,
				Cert:         cert,
				CertFilePath: "/nonexistent/path/cert.pem",
				Domain:       "example.com",
			}
			config, err := tlsConfig.ToStandardConfig()
			So(err, ShouldBeNil)
			So(config, ShouldNotBeNil)
			So(config.ServerName, ShouldEqual, tlsConfig.Domain)
		})
	})
}

// TestToStandardConfig_InvalidConfig 测试无效配置时的行为
func TestToStandardConfig_InvalidConfig(t *testing.T) {
	Convey("测试无效配置时的行为", t, func() {
		Convey("当启用 TLS 但未提供证书时应返回 invalid tls config 错误", func() {
			tlsConfig := &TLSConfig{Enabled: true}
			_, err := tlsConfig.ToStandardConfig()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "invalid tls config")
		})
	})
}

// TestToStandardConfig_InvalidCert 测试证书解析失败时的行为
func TestToStandardConfig_InvalidCert(t *testing.T) {
	Convey("测试无效证书内容时的行为", t, func() {
		Convey("当证书内容无法被 PEM 解析时应返回 failed to append certs 错误", func() {
			tlsConfig := &TLSConfig{
				Enabled: true,
				Cert:    "invalid-cert-content",
			}
			_, err := tlsConfig.ToStandardConfig()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "failed to append certs")
		})
	})
}

// TestToStandardConfig_EmptyCertFile 测试证书文件路径为空时的行为
func TestToStandardConfig_EmptyCertFile(t *testing.T) {
	Convey("测试证书文件路径为空时的行为", t, func() {
		Convey("当启用 TLS 且 CertFilePath 为空字符串时应返回 invalid tls config 错误", func() {
			tlsConfig := &TLSConfig{
				Enabled:      true,
				CertFilePath: "",
			}
			_, err := tlsConfig.ToStandardConfig()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "invalid tls config")
		})
	})
}

// TestToStandardConfig_EmptyCert 测试证书内容为空时的行为
func TestToStandardConfig_EmptyCert(t *testing.T) {
	Convey("测试证书内容为空时的行为", t, func() {
		Convey("当启用 TLS 且 Cert 为空字符串时应返回 invalid tls config 错误", func() {
			tlsConfig := &TLSConfig{
				Enabled: true,
				Cert:    "",
			}
			_, err := tlsConfig.ToStandardConfig()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "invalid tls config")
		})
	})
}

// TestToStandardConfig_NonExistentCertFile 测试证书文件不存在时的行为
func TestToStandardConfig_NonExistentCertFile(t *testing.T) {
	Convey("测试证书文件不存在时的行为", t, func() {
		Convey("当证书文件路径不存在时应返回文件读取错误", func() {
			tlsConfig := &TLSConfig{
				Enabled:      true,
				CertFilePath: "/nonexistent/path/cert.pem",
			}
			_, err := tlsConfig.ToStandardConfig()
			So(err, ShouldNotBeNil)
		})
	})
}

// TestToStandardConfig_WithDomain 测试 TLS Domain 字段
func TestToStandardConfig_WithDomain(t *testing.T) {
	Convey("测试 TLS Domain 字段映射", t, func() {
		Convey("当指定 Domain 时应正确映射到 tls.Config.ServerName", func() {
			_, cert, err := tls.GenerateTestSignedCert()
			So(err, ShouldBeNil)
			tlsConfig := &TLSConfig{
				Enabled: true,
				Cert:    cert,
				Domain:  "my-etcd-cluster.local",
			}
			config, err := tlsConfig.ToStandardConfig()
			So(err, ShouldBeNil)
			So(config.ServerName, ShouldEqual, "my-etcd-cluster.local")
		})

		Convey("当 Domain 为空时 ServerName 应为空字符串", func() {
			_, cert, err := tls.GenerateTestSignedCert()
			So(err, ShouldBeNil)
			tlsConfig := &TLSConfig{
				Enabled: true,
				Cert:    cert,
				Domain:  "",
			}
			config, err := tlsConfig.ToStandardConfig()
			So(err, ShouldBeNil)
			So(config.ServerName, ShouldEqual, "")
		})
	})
}

// TestToEtcdConfig_NilConfig 测试输入为 nil 时的行为
func TestToEtcdConfig_NilConfig(t *testing.T) {
	Convey("测试 Config 为 nil 时的行为", t, func() {
		Convey("当 Config 为 nil 时应返回空 clientv3.Config 且无错误", func() {
			var cfg *Config
			ctx := context.Background()

			result, err := cfg.ToEtcdConfig(ctx, nil)
			So(err, ShouldBeNil)
			So(result, ShouldResemble, clientv3.Config{})
		})
	})
}

// TestToEtcdConfig_ValidConfig 测试有效配置时的行为
func TestToEtcdConfig_ValidConfig(t *testing.T) {
	Convey("测试有效配置的转换", t, func() {
		Convey("当配置有效时应正确转换为 clientv3.Config", func() {
			cfg := &Config{
				Endpoints:            []string{"localhost:2379"},
				Username:             "user",
				Password:             "pass",
				AutoSyncInterval:     10 * time.Second,
				DialTimeout:          5 * time.Second,
				DialKeepAliveTime:    30 * time.Second,
				DialKeepAliveTimeout: 10 * time.Second,
				MaxCallSendMsgSize:   2097152,
				MaxCallRecvMsgSize:   4294967296,
				RejectOldCluster:     true,
				TLS: TLSConfig{
					Enabled: false,
				},
			}
			ctx := context.Background()
			logger, err := logging.NewDefaultLogger()
			So(err, ShouldBeNil)
			result, err := cfg.ToEtcdConfig(ctx, logger)
			So(err, ShouldBeNil)
			So(result.Endpoints[0], ShouldEqual, "localhost:2379")
			So(result.Username, ShouldEqual, "user")
			So(result.Password, ShouldEqual, "pass")
		})
	})
}

// TestToEtcdConfig_AllFieldsMapping 测试所有字段映射的完整性
func TestToEtcdConfig_AllFieldsMapping(t *testing.T) {
	Convey("测试所有字段映射的完整性", t, func() {
		Convey("应将 Config 所有字段正确映射到 clientv3.Config", func() {
			cfg := &Config{
				Endpoints:            []string{"10.0.0.1:2379", "10.0.0.2:2379", "10.0.0.3:2379"},
				Username:             "admin",
				Password:             "secret",
				AutoSyncInterval:     30 * time.Second,
				DialTimeout:          10 * time.Second,
				DialKeepAliveTime:    60 * time.Second,
				DialKeepAliveTimeout: 20 * time.Second,
				MaxCallSendMsgSize:   10485760,
				MaxCallRecvMsgSize:   52428800,
				RejectOldCluster:     false,
				TLS: TLSConfig{
					Enabled: false,
				},
			}
			ctx := context.Background()
			logger, err := logging.NewDefaultLogger()
			So(err, ShouldBeNil)

			result, err := cfg.ToEtcdConfig(ctx, logger)
			So(err, ShouldBeNil)

			So(result.Endpoints, ShouldResemble, []string{"10.0.0.1:2379", "10.0.0.2:2379", "10.0.0.3:2379"})
			So(result.Username, ShouldEqual, "admin")
			So(result.Password, ShouldEqual, "secret")
			So(result.AutoSyncInterval, ShouldEqual, 30*time.Second)
			So(result.DialTimeout, ShouldEqual, 10*time.Second)
			So(result.DialKeepAliveTime, ShouldEqual, 60*time.Second)
			So(result.DialKeepAliveTimeout, ShouldEqual, 20*time.Second)
			So(result.MaxCallSendMsgSize, ShouldEqual, 10485760)
			So(result.MaxCallRecvMsgSize, ShouldEqual, 52428800)
			So(result.RejectOldCluster, ShouldBeFalse)
			So(result.TLS, ShouldBeNil)
			So(result.Context, ShouldEqual, ctx)
		})
	})
}

// TestToEtcdConfig_WithValidTLS 测试带有效 TLS 配置的转换
func TestToEtcdConfig_WithValidTLS(t *testing.T) {
	Convey("测试带有效 TLS 配置的转换", t, func() {
		Convey("当 TLS 配置有效时应在 clientv3.Config 中包含 TLS 配置", func() {
			_, cert, err := tls.GenerateTestSignedCert()
			So(err, ShouldBeNil)

			cfg := &Config{
				Endpoints: []string{"localhost:2379"},
				TLS: TLSConfig{
					Enabled: true,
					Cert:    cert,
					Domain:  "secure.etcd.local",
				},
			}
			ctx := context.Background()
			logger, err := logging.NewDefaultLogger()
			So(err, ShouldBeNil)

			result, err := cfg.ToEtcdConfig(ctx, logger)
			So(err, ShouldBeNil)
			So(result.TLS, ShouldNotBeNil)
			So(result.TLS.ServerName, ShouldEqual, "secure.etcd.local")
			So(result.TLS.RootCAs, ShouldNotBeNil)
		})
	})
}

// TestToEtcdConfig_TLSEnabled 测试 TLS 启用但证书文件不存在时的行为
func TestToEtcdConfig_TLSEnabled(t *testing.T) {
	Convey("测试 TLS 启用但证书文件不存在时的行为", t, func() {
		Convey("当证书文件不存在时应返回错误", func() {
			cfg := &Config{
				Endpoints: []string{"localhost:2379"},
				TLS: TLSConfig{
					Enabled:      true,
					Domain:       "example.com",
					CertFilePath: "/path/to/cert.pem",
				},
			}
			ctx := context.Background()

			logger, err := logging.NewDefaultLogger()
			So(err, ShouldBeNil)
			_, err = cfg.ToEtcdConfig(ctx, logger)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "no such file or directory")
		})
	})
}

// TestToEtcdConfig_InvalidTLS 测试无效 TLS 配置时的行为
func TestToEtcdConfig_InvalidTLS(t *testing.T) {
	Convey("测试无效 TLS 配置时的行为", t, func() {
		Convey("当启用 TLS 但未提供证书时应返回 invalid tls config 错误", func() {
			cfg := &Config{
				Endpoints: []string{"localhost:2379"},
				TLS: TLSConfig{
					Enabled: true,
				},
			}
			ctx := context.Background()

			logger, err := logging.NewDefaultLogger()
			So(err, ShouldBeNil)
			_, err = cfg.ToEtcdConfig(ctx, logger)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "invalid tls config")
		})
	})
}

// TestToEtcdConfig_EmptyEndpoints 测试空端点列表的转换
func TestToEtcdConfig_EmptyEndpoints(t *testing.T) {
	Convey("测试空端点列表的转换", t, func() {
		Convey("当端点列表为空时应正确转换（etcd SDK 会在连接时报错而非此处）", func() {
			cfg := &Config{
				Endpoints: []string{},
				TLS: TLSConfig{
					Enabled: false,
				},
			}
			ctx := context.Background()
			logger, err := logging.NewDefaultLogger()
			So(err, ShouldBeNil)

			result, err := cfg.ToEtcdConfig(ctx, logger)
			So(err, ShouldBeNil)
			So(result.Endpoints, ShouldBeEmpty)
		})
	})
}

// TestDescribeConfig 测试配置描述信息生成
func TestDescribeConfig(t *testing.T) {
	Convey("测试配置描述信息生成", t, func() {
		Convey("当传入单个名称时应生成正确前缀的配置描述", func() {
			descs := DescribeConfig("default")
			So(descs, ShouldNotBeEmpty)

			foundEndpoints := false
			for _, desc := range descs {
				if desc.Kind == "slice" && desc.Path != "" {
					foundEndpoints = true
					So(desc.Path, ShouldContainSubstring, "etcd")
					So(desc.Path, ShouldContainSubstring, "default")
					break
				}
			}
			So(foundEndpoints, ShouldBeTrue)
		})

		Convey("当传入多个名称时应生成用逗号分隔的前缀", func() {
			descs := DescribeConfig("primary", "secondary")
			So(descs, ShouldNotBeEmpty)

			hasMultiName := false
			for _, desc := range descs {
				if desc.Path != "" {
					So(desc.Path, ShouldContainSubstring, "etcd")
					if desc.Path[:6] == "etcd.{" {
						hasMultiName = true
					}
				}
			}
			So(hasMultiName, ShouldBeTrue)
		})

		Convey("当传入空名称列表时应仍生成配置描述", func() {
			descs := DescribeConfig()
			So(descs, ShouldNotBeEmpty)
		})

		Convey("配置描述应包含所有 Config 的关键字段", func() {
			descs := DescribeConfig("test")
			paths := make(map[string]bool)
			for _, desc := range descs {
				paths[desc.Path] = true
			}
			So(len(descs), ShouldBeGreaterThan, 10)
		})
	})
}

// TestExtractConfig 测试从 viper 提取配置
func TestExtractConfig(t *testing.T) {
	Convey("测试从 viper 提取配置", t, func() {
		Convey("当 viper 中不存在指定前缀时应返回错误", func() {
			v := viper.New()
			_, err := ExtractConfig(v, "nonexistent")
			So(err, ShouldNotBeNil)
		})

		Convey("当 viper 中包含有效配置时应成功提取", func() {
			v := viper.New()
			v.SetConfigType("toml")
			v.Set("etcd.main.endpoints", []string{"10.0.0.1:2379"})
			v.Set("etcd.main.dialTimeout", "10s")
			v.Set("etcd.main.dialKeepAliveTime", "30s")
			v.Set("etcd.main.dialKeepAliveTimeout", "15s")

			cfg, err := ExtractConfig(v, "main")
			So(err, ShouldBeNil)
			So(cfg, ShouldNotBeNil)
			So(cfg.Endpoints, ShouldResemble, []string{"10.0.0.1:2379"})
		})
	})
}

// TestTLSConfig_DefaultValues 测试 TLSConfig 结构体的默认值行为
func TestTLSConfig_DefaultValues(t *testing.T) {
	Convey("测试 TLSConfig 默认值", t, func() {
		Convey("零值 TLSConfig 的 Enabled 应为 false", func() {
			tlsConfig := TLSConfig{}
			So(tlsConfig.Enabled, ShouldBeFalse)
		})

		Convey("零值 TLSConfig 的 Domain 应为空字符串", func() {
			tlsConfig := TLSConfig{}
			So(tlsConfig.Domain, ShouldEqual, "")
		})

		Convey("零值 TLSConfig 转换应返回 nil 且无错误", func() {
			tlsConfig := TLSConfig{}
			config, err := tlsConfig.ToStandardConfig()
			So(config, ShouldBeNil)
			So(err, ShouldBeNil)
		})
	})
}

// TestConfig_DefaultValues 测试 Config 结构体的默认值行为
func TestConfig_DefaultValues(t *testing.T) {
	Convey("测试 Config 默认值", t, func() {
		Convey("零值 Config 的 Endpoints 应为 nil", func() {
			cfg := Config{}
			So(cfg.Endpoints, ShouldBeNil)
		})

		Convey("零值 Config 的 RejectOldCluster 应为 false", func() {
			cfg := Config{}
			So(cfg.RejectOldCluster, ShouldBeFalse)
		})

		Convey("零值 Config 转换应成功且字段为零值", func() {
			cfg := Config{}
			ctx := context.Background()
			logger, err := logging.NewDefaultLogger()
			So(err, ShouldBeNil)

			result, err := cfg.ToEtcdConfig(ctx, logger)
			So(err, ShouldBeNil)
			So(result.Endpoints, ShouldBeNil)
			So(result.Username, ShouldEqual, "")
			So(result.Password, ShouldEqual, "")
			So(result.RejectOldCluster, ShouldBeFalse)
		})
	})
}

// TestToEtcdConfig_ContextPropagation 测试上下文传播
func TestToEtcdConfig_ContextPropagation(t *testing.T) {
	Convey("测试上下文对象传播到 clientv3.Config", t, func() {
		Convey("应将传入的 context 正确设置到 clientv3.Config 中", func() {
			cfg := &Config{
				Endpoints: []string{"localhost:2379"},
				TLS:       TLSConfig{Enabled: false},
			}
			ctx := context.Background()
			logger, err := logging.NewDefaultLogger()
			So(err, ShouldBeNil)

			result, err := cfg.ToEtcdConfig(ctx, logger)
			So(err, ShouldBeNil)
			So(result.Context, ShouldEqual, ctx)
		})

		Convey("应支持可取消的上下文", func() {
			cfg := &Config{
				Endpoints: []string{"localhost:2379"},
				TLS:       TLSConfig{Enabled: false},
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			logger, err := logging.NewDefaultLogger()
			So(err, ShouldBeNil)

			result, err := cfg.ToEtcdConfig(ctx, logger)
			So(err, ShouldBeNil)
			So(result.Context, ShouldEqual, ctx)
		})
	})
}

// TestToEtcdConfig_LoggerDesugar 测试日志记录器 Desugar
func TestToEtcdConfig_LoggerDesugar(t *testing.T) {
	Convey("测试日志记录器通过 Desugar 转换", t, func() {
		Convey("应将 SugaredLogger 转换为 zap.Logger", func() {
			cfg := &Config{
				Endpoints: []string{"localhost:2379"},
				TLS:       TLSConfig{Enabled: false},
			}
			ctx := context.Background()
			logger, err := logging.NewDefaultLogger()
			So(err, ShouldBeNil)

			result, err := cfg.ToEtcdConfig(ctx, logger)
			So(err, ShouldBeNil)
			So(result.Logger, ShouldNotBeNil)
		})
	})
}
