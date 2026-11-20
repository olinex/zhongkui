package vtab

import (
	"bytes"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto/key"
	"zhongkuisec.com/zhongkui/internal/pkg/s3"
)

// initSQLiteCryptoKeyDerivation 初始化全局密钥派生器并返回实例。
// InitGlobalKeyDerivation 内部由 sync.Once 保护，重复调用安全。
func initSQLiteCryptoKeyDerivation(t *testing.T) *key.KeyDerivation {
	t.Helper()
	pb, err := key.LoadPasswordBook("")
	if err != nil {
		t.Fatalf("load password book: %v", err)
	}
	key.InitGlobalKeyDerivation(key.NewConfigFileSecretKeyProvider("this-is-a-32-byte-secret-key!!"), pb)
	kd, err := key.GetKeyDerivation()
	if err != nil {
		t.Fatalf("get key derivation: %v", err)
	}
	return kd
}

// TestSQLiteObjectCryptoKey 验证 SQLite 数据源文件密钥来自密码本派生：
// 长度正确、结果稳定，且与 S3 默认管道密钥互不相同。
func TestSQLiteObjectCryptoKey(t *testing.T) {
	Convey("SQLite 数据源文件密钥派生", t, func() {
		kd := initSQLiteCryptoKeyDerivation(t)

		Convey("派生 32 字节 AES-256 密钥且多次调用稳定", func() {
			first, err := sqliteObjectCryptoKey()
			So(err, ShouldBeNil)
			So(len(first), ShouldEqual, sqliteObjectCryptoKeyLength)
			So(len(first), ShouldEqual, 32)

			second, err := sqliteObjectCryptoKey()
			So(err, ShouldBeNil)
			So(bytes.Equal(first, second), ShouldBeTrue)
		})

		Convey("与 S3 默认管道密钥（offset 43036）不同", func() {
			sqliteKey, err := sqliteObjectCryptoKey()
			So(err, ShouldBeNil)

			defaultKey, err := (&s3.Config{}).ExtractCryptoKey(kd)
			So(err, ShouldBeNil)
			So(bytes.Equal(sqliteKey, defaultKey), ShouldBeFalse)
		})

		Convey("offset 与长度不越出密码本边界", func() {
			span := sqliteObjectCryptoKeyOffset + max(key.MinKeyLength, sqliteObjectCryptoKeyLength)
			So(span, ShouldBeLessThanOrEqualTo, key.PasswordBookSize)
		})
	})
}
