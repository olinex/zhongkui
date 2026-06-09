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

// GenerateTestSignedCert generates a self-signed TLS certificate and its private key in PEM format.
// Returns the certificate PEM and private key PEM, or an error if any step fails.
func GenerateSelfSignedCert(notBefore, notAfter time.Time, dnsNames []string) (keyPEM, certPEM string, err error) {
	// Generate a new RSA private key
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", errors.WithStack(err)
	}

	// Create a certificate template
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

	// Create the certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return "", "", errors.WithStack(err)
	}

	// Encode the certificate to PEM format
	certPEMBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: derBytes,
	})

	// Encode the private key to PEM format
	keyPEMBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})

	return string(keyPEMBytes), string(certPEMBytes), nil
}

func GenerateTestSignedCert() (keyPEM, certPEM string, err error) {
	return GenerateSelfSignedCert(time.Now(), time.Now().Add(24*time.Hour), []string{"test.com"})
}
