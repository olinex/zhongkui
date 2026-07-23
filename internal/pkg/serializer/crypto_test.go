package serializer

import (
	"context"
	"os"
	"reflect"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"gorm.io/gorm/schema"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
)

func TestMain(m *testing.M) {
	// 初始化全局 KeyDerivation，供 CryptoSerializer.getTagMode 使用
	pb, err := key.LoadPasswordBook("")
	if err != nil {
		panic(err)
	}
	provider := key.NewConfigFileSecretKeyProvider("this-is-a-32-byte-secret-key!!")
	key.InitGlobalKeyDerivation(provider, pb)

	os.Exit(m.Run())
}

// CryptoTestModel 用于 GORM schema 解析，获取带 secure 标签的 Field
// 注意：Scan 要求字段为指针类型
type CryptoTestModel struct {
	Secret *Crypto `gorm:"serializer:crypto" secure:"mode=\"aes-gcm\",offset=0,length=32"`
}

// getCryptoField 从 CryptoTestModel 中解析出 schema.Field
func getCryptoField() *schema.Field {
	s, err := schema.Parse(&CryptoTestModel{}, &sync.Map{}, schema.NamingStrategy{})
	So(err, ShouldBeNil)
	field := s.FieldsByDBName["secret"]
	So(field, ShouldNotBeNil)
	return field
}

func TestCrypto_getTagMode(t *testing.T) {
	Convey("Test getTagMode", t, func() {
		cs := CryptoSerializer{}

		Convey("valid aes-gcm tag", func() {
			m, err := cs.getTagMode(getCryptoField())
			So(err, ShouldBeNil)
			So(m, ShouldNotBeNil)

			// 验证加密/解密可用
			plaintext := []byte("hello crypto serializer!")
			ciphertext, err := m.Encrypt(plaintext)
			So(err, ShouldBeNil)
			So(ciphertext, ShouldNotBeEmpty)

			decrypted, err := m.Decrypt(ciphertext)
			So(err, ShouldBeNil)
			So(decrypted, ShouldResemble, plaintext)
		})

		Convey("unknown mode in tag", func() {
			type BadModel struct {
				Secret *Crypto `gorm:"serializer:crypto" secure:"mode=\"unknown\",offset=0,length=32"`
			}
			s, err := schema.Parse(&BadModel{}, &sync.Map{}, schema.NamingStrategy{})
			So(err, ShouldBeNil)
			field := s.FieldsByDBName["secret"]
			So(field, ShouldNotBeNil)

			_, err = cs.getTagMode(field)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "unknown crypto mode")
		})

		Convey("invalid tag format", func() {
			type BadModel struct {
				Secret *Crypto `gorm:"serializer:crypto" secure:"invalid_tag"`
			}
			s, err := schema.Parse(&BadModel{}, &sync.Map{}, schema.NamingStrategy{})
			So(err, ShouldBeNil)
			field := s.FieldsByDBName["secret"]
			So(field, ShouldNotBeNil)

			_, err = cs.getTagMode(field)
			So(err, ShouldNotBeNil)
		})

		Convey("key length too short from tag", func() {
			type BadModel struct {
				// length=16 = MIN_KEY_LENGTH, should fail
				Secret *Crypto `gorm:"serializer:crypto" secure:"mode=\"aes-gcm\",offset=0,length=16"`
			}
			s, err := schema.Parse(&BadModel{}, &sync.Map{}, schema.NamingStrategy{})
			So(err, ShouldBeNil)
			field := s.FieldsByDBName["secret"]
			So(field, ShouldNotBeNil)

			_, err = cs.getTagMode(field)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "key length should greater than")
		})

		Convey("empty tag", func() {
			type BadModel struct {
				Secret *Crypto `gorm:"serializer:crypto" secure:""`
			}
			s, err := schema.Parse(&BadModel{}, &sync.Map{}, schema.NamingStrategy{})
			So(err, ShouldBeNil)
			field := s.FieldsByDBName["secret"]
			So(field, ShouldNotBeNil)

			_, err = cs.getTagMode(field)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestCrypto_getTagMode_EncryptDecrypt(t *testing.T) {
	Convey("Test encryption/decryption via getTagMode", t, func() {
		cs := CryptoSerializer{}
		m, err := cs.getTagMode(getCryptoField())
		So(err, ShouldBeNil)

		Convey("encrypt and decrypt empty plaintext", func() {
			plaintext := []byte{}
			ciphertext, err := m.Encrypt(plaintext)
			So(err, ShouldBeNil)
			So(ciphertext, ShouldNotBeEmpty) // nonce + tag 占空间

			decrypted, err := m.Decrypt(ciphertext)
			So(err, ShouldBeNil)
			// AES-GCM Open 对空明文解密返回 nil
			So(decrypted, ShouldBeNil)
		})

		Convey("encrypt and decrypt normal data", func() {
			plaintext := []byte("hello world")
			ciphertext, err := m.Encrypt(plaintext)
			So(err, ShouldBeNil)
			decrypted, err := m.Decrypt(ciphertext)
			So(err, ShouldBeNil)
			So(decrypted, ShouldResemble, plaintext)
		})

		Convey("encrypt and decrypt large binary data", func() {
			plaintext := make([]byte, 4096)
			for i := range plaintext {
				plaintext[i] = byte(i % 256)
			}
			ciphertext, err := m.Encrypt(plaintext)
			So(err, ShouldBeNil)
			decrypted, err := m.Decrypt(ciphertext)
			So(err, ShouldBeNil)
			So(decrypted, ShouldResemble, plaintext)
		})

		Convey("decrypt corrupted ciphertext should fail", func() {
			plaintext := []byte("secret data")
			ciphertext, err := m.Encrypt(plaintext)
			So(err, ShouldBeNil)

			// 修改密文末尾（tag）导致认证失败
			corrupted := make([]byte, len(ciphertext))
			copy(corrupted, ciphertext)
			corrupted[len(corrupted)-1] ^= 0xFF

			_, err = m.Decrypt(corrupted)
			So(err, ShouldNotBeNil)
		})

		Convey("decrypt too-short ciphertext should fail", func() {
			_, err := m.Decrypt([]byte("short"))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "ciphertext too short")
		})
	})
}

func TestCrypto_Scan(t *testing.T) {
	Convey("Test CryptoSerializer.Scan", t, func() {
		cs := CryptoSerializer{}
		field := getCryptoField()
		ctx := context.Background()

		Convey("nil dbValue sets zero value", func() {
			model := &CryptoTestModel{Secret: &Crypto{value: []byte("old")}}
			dst := reflect.ValueOf(model).Elem()
			err := cs.Scan(ctx, field, dst, nil)
			So(err, ShouldBeNil)
			// nil dbValue 将字段置零
			So(model.Secret, ShouldBeNil)
		})

		Convey("invalid dbValue type returns error", func() {
			model := &CryptoTestModel{Secret: &Crypto{}}
			dst := reflect.ValueOf(model).Elem()
			err := cs.Scan(ctx, field, dst, 12345) // int 类型触发 unknown type 错误
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "expected []byte or string")
		})

		Convey("dst is not a pointer returns error", func() {
			// 使用非指针类型的 Crypto 字段，才能触发 "field must be a pointer to Crypto" 错误
			type NonPointerModel struct {
				Secret Crypto `gorm:"serializer:crypto" secure:"mode=\"aes-gcm\",offset=0,length=32"`
			}
			s, err := schema.Parse(&NonPointerModel{}, &sync.Map{}, schema.NamingStrategy{})
			So(err, ShouldBeNil)
			npField := s.FieldsByDBName["secret"]
			So(npField, ShouldNotBeNil)

			// 生成合法密文
			m, err := cs.getTagMode(npField)
			So(err, ShouldBeNil)
			validCT, err := m.Encrypt([]byte("test"))
			So(err, ShouldBeNil)

			model := &NonPointerModel{}
			dst := reflect.ValueOf(model).Elem()
			err = cs.Scan(ctx, npField, dst, validCT)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "field must be a pointer to Crypto")
		})

		Convey("dst points to wrong struct type", func() {
			m, err := cs.getTagMode(field)
			So(err, ShouldBeNil)
			validCT, err := m.Encrypt([]byte("test"))
			So(err, ShouldBeNil)

			type WrongModel struct{ Name string }
			model := &WrongModel{Name: "test"}
			dst := reflect.ValueOf(model).Elem()
			err = cs.Scan(ctx, field, dst, validCT)
			So(err, ShouldNotBeNil)
		})

		Convey("valid ciphertext roundtrip via Scan/Value", func() {
			// 使用 value 加密
			modelWrite := &CryptoTestModel{
				Secret: &Crypto{value: []byte("my secret message")},
			}
			dstWrite := reflect.ValueOf(modelWrite).Elem()
			dbValue, err := cs.Value(ctx, field, dstWrite, modelWrite.Secret)
			So(err, ShouldBeNil)
			So(dbValue, ShouldNotBeNil)

			cipherBytes, ok := dbValue.([]byte)
			So(ok, ShouldBeTrue)

			// 使用 Scan 解密
			modelRead := &CryptoTestModel{Secret: &Crypto{}}
			dstRead := reflect.ValueOf(modelRead).Elem()
			err = cs.Scan(ctx, field, dstRead, cipherBytes)
			So(err, ShouldBeNil)
			So(modelRead.Secret, ShouldNotBeNil)
			So(string(modelRead.Secret.value), ShouldEqual, "my secret message")
		})
	})
}

func TestCrypto_Value(t *testing.T) {
	Convey("Test CryptoSerializer.Value", t, func() {
		cs := CryptoSerializer{}
		field := getCryptoField()
		ctx := context.Background()

		Convey("nil fieldValue returns nil", func() {
			model := &CryptoTestModel{Secret: &Crypto{}}
			dst := reflect.ValueOf(model).Elem()
			result, err := cs.Value(ctx, field, dst, nil)
			So(err, ShouldBeNil)
			So(result, ShouldBeNil)
		})

		Convey("valid fieldValue produces ciphertext", func() {
			model := &CryptoTestModel{
				Secret: &Crypto{value: []byte("hello crypto")},
			}
			dst := reflect.ValueOf(model).Elem()
			result, err := cs.Value(ctx, field, dst, model.Secret)
			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)

			cipherBytes, ok := result.([]byte)
			So(ok, ShouldBeTrue)
			So(len(cipherBytes), ShouldBeGreaterThan, 0)
		})

		Convey("fieldValue is not a pointer returns error", func() {
			model := &CryptoTestModel{
				Secret: &Crypto{value: []byte("data")},
			}
			dst := reflect.ValueOf(model).Elem()
			// 传值而非指针
			result, err := cs.Value(ctx, field, dst, *model.Secret)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "field value must be a pointer to Crypto")
			So(result, ShouldBeNil)
		})

		Convey("fieldValue points to wrong struct type", func() {
			type WrongStruct struct{ X int }
			wrong := &WrongStruct{X: 42}
			model := &CryptoTestModel{Secret: &Crypto{}}
			dst := reflect.ValueOf(model).Elem()
			result, err := cs.Value(ctx, field, dst, wrong)
			So(err, ShouldNotBeNil)
			// WrongStruct 没有名为 "Value" 的字段
			So(err.Error(), ShouldContainSubstring, "Crypto missing value field")
			So(result, ShouldBeNil)
		})

		Convey("empty testBytes value returns nil (skip encrypt)", func() {
			model := &CryptoTestModel{
				Secret: &Crypto{value: []byte{}},
			}
			dst := reflect.ValueOf(model).Elem()
			result, err := cs.Value(ctx, field, dst, model.Secret)
			So(err, ShouldBeNil)
			// 空数据跳过加密，直接返回 nil
			So(result, ShouldBeNil)
		})

		Convey("large testBytes value", func() {
			largeData := make([]byte, 65536)
			for i := range largeData {
				largeData[i] = byte(i % 256)
			}
			model := &CryptoTestModel{
				Secret: &Crypto{value: largeData},
			}
			dst := reflect.ValueOf(model).Elem()
			result, err := cs.Value(ctx, field, dst, model.Secret)
			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)

			cipherBytes, ok := result.([]byte)
			So(ok, ShouldBeTrue)
			// ciphertext should be larger than plaintext (nonce + tag overhead)
			So(len(cipherBytes), ShouldBeGreaterThan, len(largeData))
		})
	})
}

func TestCrypto_ModeEncryptDecrypt_Roundtrip(t *testing.T) {
	Convey("Test full encrypt/decrypt roundtrip via mode", t, func() {
		cs := CryptoSerializer{}
		m, err := cs.getTagMode(getCryptoField())
		So(err, ShouldBeNil)

		Convey("multiple roundtrip with random data", func() {
			for i := range 10 {
				size := (i + 1) * 13
				plaintext := make([]byte, size)
				for j := range plaintext {
					plaintext[j] = byte((i + j) % 256)
				}
				ciphertext, err := m.Encrypt(plaintext)
				So(err, ShouldBeNil)
				decrypted, err := m.Decrypt(ciphertext)
				So(err, ShouldBeNil)
				So(decrypted, ShouldResemble, plaintext)
			}
		})

		Convey("different plaintexts produce different ciphertexts", func() {
			ct1, err := m.Encrypt([]byte("message one"))
			So(err, ShouldBeNil)
			ct2, err := m.Encrypt([]byte("message two"))
			So(err, ShouldBeNil)
			So(ct1, ShouldNotResemble, ct2)
		})

		Convey("same plaintext twice produces different ciphertexts (random nonce)", func() {
			plaintext := []byte("test message")
			ct1, err := m.Encrypt(plaintext)
			So(err, ShouldBeNil)
			ct2, err := m.Encrypt(plaintext)
			So(err, ShouldBeNil)
			So(ct1, ShouldNotResemble, ct2)
		})
	})
}

func TestCrypto_CryptoTag_Defaults(t *testing.T) {
	Convey("Test CryptoTag default values", t, func() {
		cs := CryptoSerializer{}

		Convey("only mode specified uses default offset=0, length=0 (fails)", func() {
			// length=0 会触发 key derivation 校验：key length must be greater than 16, got: 0
			type DefaultModel struct {
				Secret *Crypto `gorm:"serializer:crypto" secure:"mode=\"aes-gcm\""`
			}
			s, err := schema.Parse(&DefaultModel{}, &sync.Map{}, schema.NamingStrategy{})
			So(err, ShouldBeNil)
			field := s.FieldsByDBName["secret"]
			So(field, ShouldNotBeNil)

			_, err = cs.getTagMode(field)
			So(err, ShouldNotBeNil)
		})

		Convey("custom offset in tag", func() {
			type CustomModel struct {
				Secret *Crypto `gorm:"serializer:crypto" secure:"mode=\"aes-gcm\",offset=100,length=32"`
			}
			s, err := schema.Parse(&CustomModel{}, &sync.Map{}, schema.NamingStrategy{})
			So(err, ShouldBeNil)
			field := s.FieldsByDBName["secret"]
			So(field, ShouldNotBeNil)

			m, err := cs.getTagMode(field)
			So(err, ShouldBeNil)
			So(m, ShouldNotBeNil)

			plaintext := []byte("offset test")
			ciphertext, err := m.Encrypt(plaintext)
			So(err, ShouldBeNil)
			decrypted, err := m.Decrypt(ciphertext)
			So(err, ShouldBeNil)
			So(decrypted, ShouldResemble, plaintext)
		})

		Convey("custom length in tag (AES-192)", func() {
			// AES-192 = 24 bytes key, AES-256 = 32 bytes
			type CustomModel struct {
				Secret *Crypto `gorm:"serializer:crypto" secure:"mode=\"aes-gcm\",offset=0,length=24"`
			}
			s, err := schema.Parse(&CustomModel{}, &sync.Map{}, schema.NamingStrategy{})
			So(err, ShouldBeNil)
			field := s.FieldsByDBName["secret"]
			So(field, ShouldNotBeNil)

			m, err := cs.getTagMode(field)
			So(err, ShouldBeNil)
			So(m, ShouldNotBeNil)

			plaintext := []byte("aes-192 test")
			ciphertext, err := m.Encrypt(plaintext)
			So(err, ShouldBeNil)
			decrypted, err := m.Decrypt(ciphertext)
			So(err, ShouldBeNil)
			So(decrypted, ShouldResemble, plaintext)
		})
	})
}

func TestCrypto_ScanValue_Roundtrip(t *testing.T) {
	Convey("Test Scan/Value encrypt then decrypt", t, func() {
		cs := CryptoSerializer{}
		field := getCryptoField()
		ctx := context.Background()

		Convey("encrypt via Value, decrypt via Scan succeeds", func() {
			original := []byte("roundtrip test data here")
			modelWrite := &CryptoTestModel{
				Secret: &Crypto{value: original},
			}
			dstWrite := reflect.ValueOf(modelWrite).Elem()

			// 加密
			dbValue, err := cs.Value(ctx, field, dstWrite, modelWrite.Secret)
			So(err, ShouldBeNil)
			cipherBytes, ok := dbValue.([]byte)
			So(ok, ShouldBeTrue)

			// 解密后验证数据一致
			modelRead := &CryptoTestModel{Secret: &Crypto{}}
			dstRead := reflect.ValueOf(modelRead).Elem()
			err = cs.Scan(ctx, field, dstRead, cipherBytes)
			So(err, ShouldBeNil)
			So(modelRead.Secret, ShouldNotBeNil)
			So(string(modelRead.Secret.Value()), ShouldEqual, string(original))
		})

		Convey("encrypt empty data returns nil, Scan handles nil", func() {
			modelWrite := &CryptoTestModel{
				Secret: &Crypto{value: []byte{}},
			}
			dstWrite := reflect.ValueOf(modelWrite).Elem()

			dbValue, err := cs.Value(ctx, field, dstWrite, modelWrite.Secret)
			So(err, ShouldBeNil)
			// 空数据跳过加密，返回 nil
			So(dbValue, ShouldBeNil)

			// Scan 接收到 nil 时置零字段
			modelRead := &CryptoTestModel{Secret: &Crypto{value: []byte("should be cleared")}}
			dstRead := reflect.ValueOf(modelRead).Elem()
			err = cs.Scan(ctx, field, dstRead, nil)
			So(err, ShouldBeNil)
			So(modelRead.Secret, ShouldBeNil)
		})
	})
}
