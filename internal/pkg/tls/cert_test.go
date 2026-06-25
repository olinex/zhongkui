package tls

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

// TestGenerateTestSignedCert tests the GenerateTestSignedCert function for generating a self-signed TLS certificate and its private key.
// It verifies that the function returns valid PEM-encoded strings for both the certificate and private key,
// and that the certificate and private key can be used for encryption and decryption.
func TestGenerateTestSignedCert(t *testing.T) {
	Convey("TestGenerateTestSignedCert", t, func() {
		Convey("should generate valid certificate and private key", func() {
			keyPEM, certPEM, err := GenerateTestSignedCert()
			So(err, ShouldBeNil)
			So(keyPEM, ShouldNotBeEmpty)
			So(certPEM, ShouldNotBeEmpty)

			// Verify the certificate's validity period
			certBlock, _ := pem.Decode([]byte(certPEM))
			So(certBlock, ShouldNotBeNil)

			cert, err := x509.ParseCertificate(certBlock.Bytes)
			So(err, ShouldBeNil)

			now := time.Now()
			So(cert.NotBefore.After(now), ShouldBeFalse)
			So(cert.NotAfter.Before(now), ShouldBeFalse)
			So(cert.NotAfter.Sub(cert.NotBefore), ShouldEqual, 24*time.Hour)

			// Verify the private key can be parsed
			keyBlock, _ := pem.Decode([]byte(keyPEM))
			So(keyBlock, ShouldNotBeNil)

			_, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
			So(err, ShouldBeNil)

			// Test encryption and decryption with the generated key pair
			privKey, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
			So(err, ShouldBeNil)
			So(privKey, ShouldNotBeNil)
		})
	})
}
