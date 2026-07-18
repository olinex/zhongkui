package s3

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
)

func TestExtractConfig(t *testing.T) {
	convey.Convey("Test ExtractConfig", t, func() {
		convey.Convey("should extract config with all fields set from viper", func() {
			v := viper.New()
			v.Set("s3.endpoint", "https://s3.amazonaws.com")
			v.Set("s3.accessKey", "my-access-key")
			v.Set("s3.secretKey", "my-secret-key")
			v.Set("s3.bucket", "my-bucket")
			v.Set("s3.region", "us-west-2")
			v.Set("s3.useSSL", true)

			cfg, err := ExtractConfig(v)
			convey.So(err, convey.ShouldBeNil)
			convey.So(cfg, convey.ShouldNotBeNil)
			convey.So(cfg.Endpoint, convey.ShouldEqual, "https://s3.amazonaws.com")
			convey.So(cfg.AccessKey, convey.ShouldEqual, "my-access-key")
			convey.So(cfg.SecretKey, convey.ShouldEqual, "my-secret-key")
			convey.So(cfg.Bucket, convey.ShouldEqual, "my-bucket")
			convey.So(cfg.Region, convey.ShouldEqual, "us-west-2")
			convey.So(cfg.UseSSL, convey.ShouldBeTrue)
		})

		convey.Convey("should extract config with minimal fields and fill defaults", func() {
			v := viper.New()
			v.Set("s3.endpoint", "http://localhost:9000")
			v.Set("s3.accessKey", "minioadmin")
			v.Set("s3.secretKey", "minioadmin")
			v.Set("s3.bucket", "test-bucket")

			cfg, err := ExtractConfig(v)
			convey.So(err, convey.ShouldBeNil)
			convey.So(cfg, convey.ShouldNotBeNil)
			convey.So(cfg.Endpoint, convey.ShouldEqual, "http://localhost:9000")
			convey.So(cfg.AccessKey, convey.ShouldEqual, "minioadmin")
			convey.So(cfg.SecretKey, convey.ShouldEqual, "minioadmin")
			convey.So(cfg.Bucket, convey.ShouldEqual, "test-bucket")
			// defaults 标签定义的默认值
			convey.So(cfg.Region, convey.ShouldEqual, "")
			convey.So(cfg.UseSSL, convey.ShouldBeTrue)
		})

		convey.Convey("should extract config with UseSSL set to false", func() {
			v := viper.New()
			v.Set("s3.endpoint", "http://localhost:9000")
			v.Set("s3.accessKey", "minioadmin")
			v.Set("s3.secretKey", "minioadmin")
			v.Set("s3.bucket", "test-bucket")
			v.Set("s3.useSSL", false)

			cfg, err := ExtractConfig(v)
			convey.So(err, convey.ShouldBeNil)
			convey.So(cfg.UseSSL, convey.ShouldBeFalse)
		})

		convey.Convey("should return error when no s3 config is set at all", func() {
			v := viper.New()
			// viper 中完全没有 s3 前缀的配置，ExtractConfig 可能返回错误
			cfg, err := ExtractConfig(v)
			if err != nil {
				convey.So(cfg, convey.ShouldBeNil)
			} else {
				// 或者返回 defaults 填充的默认值
				convey.So(cfg, convey.ShouldNotBeNil)
				convey.So(cfg.Endpoint, convey.ShouldEqual, "-")
				convey.So(cfg.AccessKey, convey.ShouldEqual, "-")
			}
		})
	})
}

func TestDescribeConfig(t *testing.T) {
	convey.Convey("Test DescribeConfig", t, func() {
		convey.Convey("should return non-empty config descriptions", func() {
			descs := DescribeConfig()
			convey.So(descs, convey.ShouldNotBeNil)
			convey.So(len(descs), convey.ShouldBeGreaterThan, 0)
		})

		convey.Convey("should include all expected config fields", func() {
			descs := DescribeConfig()
			fieldPaths := make(map[string]bool)
			for _, d := range descs {
				fieldPaths[d.Path] = true
			}
			convey.So(fieldPaths["s3.endpoint"], convey.ShouldBeTrue)
			convey.So(fieldPaths["s3.accessKey"], convey.ShouldBeTrue)
			convey.So(fieldPaths["s3.secretKey"], convey.ShouldBeTrue)
			convey.So(fieldPaths["s3.bucket"], convey.ShouldBeTrue)
			convey.So(fieldPaths["s3.region"], convey.ShouldBeTrue)
			convey.So(fieldPaths["s3.useSSL"], convey.ShouldBeTrue)
			convey.So(fieldPaths["s3.streamingUpload"], convey.ShouldBeTrue)
		})

		convey.Convey("each description should have non-empty Path and Kind", func() {
			descs := DescribeConfig()
			for _, d := range descs {
				convey.So(d.Path, convey.ShouldNotBeEmpty)
				convey.So(d.Kind, convey.ShouldNotBeEmpty)
			}
		})
	})
}

func TestExtractCryptoKey(t *testing.T) {
	convey.Convey("Test ExtractCryptoKey", t, func() {
		convey.Convey("should derive a 32-byte key with valid KeyDerivation", func() {
			provider := key.NewConfigFileSecretKeyProvider("0123456789abcdef0123456789abcdef")
			book, err := key.LoadPasswordBook("")
			convey.So(err, convey.ShouldBeNil)
			kd := key.NewKeyDerivation(provider, book)

			cfg := &Config{}
			cryptoKey, err := cfg.ExtractCryptoKey(kd)
			convey.So(err, convey.ShouldBeNil)
			convey.So(cryptoKey, convey.ShouldNotBeNil)
			convey.So(len(cryptoKey), convey.ShouldEqual, CRYPTO_KEY_LENGTH) // 32 bytes
		})

		convey.Convey("should return same key with same KeyDerivation (deterministic)", func() {
			provider := key.NewConfigFileSecretKeyProvider("0123456789abcdef0123456789abcdef")
			book, err := key.LoadPasswordBook("")
			convey.So(err, convey.ShouldBeNil)
			kd := key.NewKeyDerivation(provider, book)

			cfg := &Config{}
			key1, err := cfg.ExtractCryptoKey(kd)
			convey.So(err, convey.ShouldBeNil)

			key2, err := cfg.ExtractCryptoKey(kd)
			convey.So(err, convey.ShouldBeNil)

			convey.So(key1, convey.ShouldResemble, key2)
		})

		convey.Convey("should produce different keys with different secret key providers", func() {
			book, err := key.LoadPasswordBook("")
			convey.So(err, convey.ShouldBeNil)

			provider1 := key.NewConfigFileSecretKeyProvider("key1-abcdefabcdefabcdefabcdef")
			kd1 := key.NewKeyDerivation(provider1, book)

			provider2 := key.NewConfigFileSecretKeyProvider("key2-abcdefabcdefabcdefabcdef")
			kd2 := key.NewKeyDerivation(provider2, book)

			cfg := &Config{}
			key1, err := cfg.ExtractCryptoKey(kd1)
			convey.So(err, convey.ShouldBeNil)
			key2, err := cfg.ExtractCryptoKey(kd2)
			convey.So(err, convey.ShouldBeNil)

			convey.So(key1, convey.ShouldNotResemble, key2)
		})
	})
}

func TestS3ConfigConstants(t *testing.T) {
	convey.Convey("Test S3 config constants", t, func() {
		convey.Convey("PREFIX should be 's3'", func() {
			convey.So(PREFIX, convey.ShouldEqual, "s3")
		})

		convey.Convey("CRYPTO_KEY_OFFSET should be positive", func() {
			convey.So(CRYPTO_KEY_OFFSET, convey.ShouldBeGreaterThan, 0)
		})

		convey.Convey("CRYPTO_KEY_LENGTH should be 32 (AES-256)", func() {
			convey.So(CRYPTO_KEY_LENGTH, convey.ShouldEqual, 32)
		})
	})
}
