package etcd

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// generateSelfSignedCertPEM 生成一个自签名的 PEM 编码证书，用于测试。
func generateSelfSignedCertPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{Organization: []string{"Test"}},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
}

// TestTLSConfig_ToStandardConfig 测试 TLSConfig.ToStandardConfig 方法的各种情况。
func TestTLSConfig_ToStandardConfig(t *testing.T) {
	Convey("测试 TLSConfig.ToStandardConfig", t, func() {
		Convey("当接收者为 nil 时应返回 nil 配置且无错误", func() {
			var t *TLSConfig
			cfg, err := t.ToStandardConfig()
			So(err, ShouldBeNil)
			So(cfg, ShouldBeNil)
		})

		Convey("当 Enabled 为 false 时应返回 nil 配置且无错误", func() {
			tc := &TLSConfig{Enabled: false}
			cfg, err := tc.ToStandardConfig()
			So(err, ShouldBeNil)
			So(cfg, ShouldBeNil)
		})

		Convey("当 Cert 和 CertFilePath 均为空时应返回 invalid tls config 错误", func() {
			tc := &TLSConfig{Enabled: true}
			cfg, err := tc.ToStandardConfig()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "invalid tls config")
			So(cfg, ShouldBeNil)
		})

		Convey("当 Cert 为有效 PEM 证书时应返回正确的 tls.Config", func() {
			certPEM := generateSelfSignedCertPEM(t)
			tc := &TLSConfig{
				Enabled: true,
				Domain:  "test.example.com",
				Cert:    string(certPEM),
			}
			cfg, err := tc.ToStandardConfig()
			So(err, ShouldBeNil)
			So(cfg, ShouldNotBeNil)
			So(cfg.ServerName, ShouldEqual, "test.example.com")
			So(cfg.RootCAs, ShouldNotBeNil)
		})

		Convey("当 CertFilePath 指向有效证书文件时应返回正确的 tls.Config", func() {
			certPEM := generateSelfSignedCertPEM(t)
			tmpDir := t.TempDir()
			certFile := filepath.Join(tmpDir, "ca.pem")
			err := os.WriteFile(certFile, certPEM, 0600)
			So(err, ShouldBeNil)

			tc := &TLSConfig{
				Enabled:      true,
				Domain:       "file.example.com",
				CertFilePath: certFile,
			}
			cfg, err := tc.ToStandardConfig()
			So(err, ShouldBeNil)
			So(cfg, ShouldNotBeNil)
			So(cfg.ServerName, ShouldEqual, "file.example.com")
			So(cfg.RootCAs, ShouldNotBeNil)
		})

		Convey("当 CertFilePath 指向不存在的文件时应返回错误", func() {
			tc := &TLSConfig{
				Enabled:      true,
				CertFilePath: "/nonexistent/path/ca.pem",
			}
			cfg, err := tc.ToStandardConfig()
			So(err, ShouldNotBeNil)
			So(cfg, ShouldBeNil)
		})

		Convey("当 Cert 为无效 PEM 内容时应返回 failed to append certs 错误", func() {
			tc := &TLSConfig{
				Enabled: true,
				Cert:    "this is not a valid PEM certificate",
			}
			cfg, err := tc.ToStandardConfig()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "failed to append certs")
			So(cfg, ShouldBeNil)
		})

		Convey("当 Cert 为格式正确但内容无效的 PEM 时应返回 failed to append certs 错误", func() {
			invalidPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("invalid")})
			tc := &TLSConfig{
				Enabled: true,
				Cert:    string(invalidPEM),
			}
			cfg, err := tc.ToStandardConfig()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "failed to append certs")
			So(cfg, ShouldBeNil)
		})

		Convey("当 Cert 和 CertFilePath 同时提供时应优先使用 Cert", func() {
			certPEM := generateSelfSignedCertPEM(t)
			tc := &TLSConfig{
				Enabled:      true,
				Domain:       "priority.example.com",
				Cert:         string(certPEM),
				CertFilePath: "/nonexistent/path/ca.pem",
			}
			cfg, err := tc.ToStandardConfig()
			So(err, ShouldBeNil)
			So(cfg, ShouldNotBeNil)
			So(cfg.ServerName, ShouldEqual, "priority.example.com")
		})
	})
}

// TestConfig_ToEtcdConfig 测试 Config.ToEtcdConfig 方法的各种情况。
func TestConfig_ToEtcdConfig(t *testing.T) {
	logger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	Convey("测试 Config.ToEtcdConfig", t, func() {
		Convey("当接收者为 nil 时应返回空的 clientv3.Config", func() {
			var c *Config
			cfg, err := c.ToEtcdConfig(context.Background(), logger)
			So(err, ShouldBeNil)
			So(cfg.Endpoints, ShouldBeNil)
			So(cfg.DialTimeout, ShouldEqual, time.Duration(0))
		})

		Convey("当配置完整时应正确映射所有字段", func() {
			c := &Config{
				Endpoints:            []string{"localhost:2379", "localhost:2380"},
				Username:             "admin",
				Password:             "secret",
				AutoSyncInterval:     10 * time.Second,
				DialTimeout:          5 * time.Second,
				DialKeepAliveTime:    30 * time.Second,
				DialKeepAliveTimeout: 15 * time.Second,
				MaxCallSendMsgSize:   1024 * 1024,
				MaxCallRecvMsgSize:   2 * 1024 * 1024,
				RejectOldCluster:     true,
			}
			ctx := context.Background()
			cfg, err := c.ToEtcdConfig(ctx, logger)
			So(err, ShouldBeNil)
			So(cfg.Endpoints, ShouldResemble, []string{"localhost:2379", "localhost:2380"})
			So(cfg.Username, ShouldEqual, "admin")
			So(cfg.Password, ShouldEqual, "secret")
			So(cfg.AutoSyncInterval, ShouldEqual, 10*time.Second)
			So(cfg.DialTimeout, ShouldEqual, 5*time.Second)
			So(cfg.DialKeepAliveTime, ShouldEqual, 30*time.Second)
			So(cfg.DialKeepAliveTimeout, ShouldEqual, 15*time.Second)
			So(cfg.MaxCallSendMsgSize, ShouldEqual, 1024*1024)
			So(cfg.MaxCallRecvMsgSize, ShouldEqual, 2*1024*1024)
			So(cfg.RejectOldCluster, ShouldBeTrue)
			So(cfg.Context, ShouldEqual, ctx)
			So(cfg.Logger, ShouldNotBeNil)
			So(cfg.TLS, ShouldBeNil)
		})

		Convey("当 TLS 配置有效时应包含 TLS 配置", func() {
			certPEM := generateSelfSignedCertPEM(t)
			c := &Config{
				Endpoints:   []string{"localhost:2379"},
				DialTimeout: 5 * time.Second,
				TLS: TLSConfig{
					Enabled: true,
					Domain:  "etcd.example.com",
					Cert:    string(certPEM),
				},
			}
			cfg, err := c.ToEtcdConfig(context.Background(), logger)
			So(err, ShouldBeNil)
			So(cfg.TLS, ShouldNotBeNil)
			So(cfg.TLS.ServerName, ShouldEqual, "etcd.example.com")
		})

		Convey("当 TLS 配置无效时应返回错误", func() {
			c := &Config{
				Endpoints: []string{"localhost:2379"},
				TLS: TLSConfig{
					Enabled: true,
					// Cert 和 CertFilePath 均为空，会触发 "invalid tls config" 错误
				},
			}
			cfg, err := c.ToEtcdConfig(context.Background(), logger)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "invalid tls config")
			So(cfg.Endpoints, ShouldBeNil)
		})

		Convey("当 TLS 未启用时 TLS 配置应为 nil", func() {
			c := &Config{
				Endpoints: []string{"localhost:2379"},
				TLS: TLSConfig{
					Enabled: false,
				},
			}
			cfg, err := c.ToEtcdConfig(context.Background(), logger)
			So(err, ShouldBeNil)
			So(cfg.TLS, ShouldBeNil)
		})
	})
}

// TestDescribeConfig 测试 DescribeConfig 函数。
func TestDescribeConfig(t *testing.T) {
	Convey("测试 DescribeConfig", t, func() {
		Convey("应返回非空的配置描述", func() {
			desc := DescribeConfig()
			So(desc, ShouldNotBeNil)
			So(len(desc), ShouldBeGreaterThan, 0)
		})
	})
}
