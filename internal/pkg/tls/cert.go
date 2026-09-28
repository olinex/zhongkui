// Package tls 提供 TLS 证书相关工具，包括自签名证书的生成。
package tls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// GenerateSelfSignedCert 生成一张自签名 TLS 证书及其 PEM 格式私钥。
// 返回证书 PEM 与私钥 PEM，任一步骤失败时返回错误。
func GenerateSelfSignedCert(notBefore, notAfter time.Time, dnsNames []string) (keyPEM, certPEM string, err error) {
	// 生成新的 RSA 私钥
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", errors.WithStack(err)
	}

	// 构造证书模板
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
		},
		DNSNames:              dnsNames,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// 创建证书
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return "", "", errors.WithStack(err)
	}

	// 将证书编码为 PEM 格式
	certPEMBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: derBytes,
	})

	// 将私钥编码为 PEM 格式
	keyPEMBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})

	return string(keyPEMBytes), string(certPEMBytes), nil
}

// GenerateTestSignedCert 生成一张供测试使用的自签名证书（有效期 24 小时，DNS 名为 test.com）。
func GenerateTestSignedCert() (keyPEM, certPEM string, err error) {
	return GenerateSelfSignedCert(time.Now(), time.Now().Add(24*time.Hour), []string{"test.com"})
}
