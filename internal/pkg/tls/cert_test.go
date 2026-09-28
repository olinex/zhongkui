// Package tls 提供 TLS 证书相关工具：生成自签名证书与其私钥。
package tls

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

// TestGenerateTestSignedCert 测试 GenerateTestSignedCert：验证其生成的自签名 TLS 证书与私钥
// 均为有效的 PEM 编码字符串，且该证书与私钥可用于加解密。
func TestGenerateTestSignedCert(t *testing.T) {
	Convey("TestGenerateTestSignedCert", t, func() {
		Convey("should generate valid certificate and private key", func() {
			keyPEM, certPEM, err := GenerateTestSignedCert()
			So(err, ShouldBeNil)
			So(keyPEM, ShouldNotBeEmpty)
			So(certPEM, ShouldNotBeEmpty)

			// 校验证书的有效期
			certBlock, _ := pem.Decode([]byte(certPEM))
			So(certBlock, ShouldNotBeNil)

			cert, err := x509.ParseCertificate(certBlock.Bytes)
			So(err, ShouldBeNil)

			now := time.Now()
			So(cert.NotBefore.After(now), ShouldBeFalse)
			So(cert.NotAfter.Before(now), ShouldBeFalse)
			So(cert.NotAfter.Sub(cert.NotBefore), ShouldEqual, 24*time.Hour)

			// 校验私钥可被解析
			keyBlock, _ := pem.Decode([]byte(keyPEM))
			So(keyBlock, ShouldNotBeNil)

			_, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
			So(err, ShouldBeNil)

			// 用生成的密钥对验证证书与私钥可组成有效密钥对
			privKey, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
			So(err, ShouldBeNil)
			So(privKey, ShouldNotBeNil)
		})
	})
}
