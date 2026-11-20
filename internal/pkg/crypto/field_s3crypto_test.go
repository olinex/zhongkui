package crypto_test

import (
	"bytes"
	"testing"

	"zhongkuisec.com/zhongkui/internal/pkg/crypto"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto/key"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// s3CryptoFixture 是 s3crypto 标签解析与密钥派生的测试模型。
type s3CryptoFixture struct {
	// ReqBody 请求正文对象键字段（offset 44001）
	ReqBody serializer.S3Crypto `json:"reqBody" s3crypto:"offset=44001,length=32"`
	// RspBody 响应正文对象键字段（offset 44002）
	RspBody serializer.S3Crypto `json:"rspBody" s3crypto:"offset=44002,length=32"`
	// ReqBodyHash 摘要字段，用于对比 S3 密钥与摘要密钥互不相同
	ReqBodyHash serializer.Hash `json:"reqBodyHash" hash:"offset=12808,length=32"`
	// Plain 普通字段，无 s3crypto 标签
	Plain string `json:"plain"`
}

// initTestKeyDerivation 初始化全局 KeyDerivation（sync.Once，多次调用安全）。
func initTestKeyDerivation(t *testing.T) {
	t.Helper()
	pb, err := key.LoadPasswordBook("")
	if err != nil {
		t.Fatalf("load password book: %v", err)
	}
	key.InitGlobalKeyDerivation(key.NewConfigFileSecretKeyProvider("this-is-a-32-byte-secret-key!!"), pb)
}

// TestS3CryptoKeyForField 验证按字段标签派生 S3 加密子密钥：
// 不同字段派生不同密钥，且与摘要字段密钥、其他字段互不相同。
func TestS3CryptoKeyForField(t *testing.T) {
	initTestKeyDerivation(t)
	model := &s3CryptoFixture{}

	reqKey, err := crypto.S3CryptoKeyForField(model, "reqBody", "json")
	if err != nil {
		t.Fatalf("S3CryptoKeyForField(reqBody) error = %v", err)
	}
	if len(reqKey) != 32 {
		t.Fatalf("reqKey length = %d, want 32", len(reqKey))
	}

	rspKey, err := crypto.S3CryptoKeyForField(model, "rspBody", "json")
	if err != nil {
		t.Fatalf("S3CryptoKeyForField(rspBody) error = %v", err)
	}
	if bytes.Equal(reqKey, rspKey) {
		t.Fatal("reqBody and rspBody derived the same key, want distinct per-field keys")
	}

	hashKey, err := crypto.HashKeyForField(model, "reqBodyHash", "json")
	if err != nil {
		t.Fatalf("HashKeyForField(reqBodyHash) error = %v", err)
	}
	if bytes.Equal(reqKey, hashKey) || bytes.Equal(rspKey, hashKey) {
		t.Fatal("S3 key collides with hash key, want distinct keys across tag kinds")
	}
}

// TestS3CryptoKeyForFieldWithoutTag 验证无 s3crypto 标签的字段返回明确错误。
func TestS3CryptoKeyForFieldWithoutTag(t *testing.T) {
	initTestKeyDerivation(t)
	if _, err := crypto.S3CryptoKeyForField(&s3CryptoFixture{}, "plain", "json"); err == nil {
		t.Fatal("expected error for field without s3crypto tag, got nil")
	}
}

// TestParseS3CryptoTag 验证 s3crypto 标签解析。
func TestParseS3CryptoTag(t *testing.T) {
	sTag, err := crypto.ParseS3CryptoTag(`offset=44001,length=32`)
	if err != nil {
		t.Fatalf("ParseS3CryptoTag error = %v", err)
	}
	if sTag.Offset != 44001 || sTag.Length != 32 {
		t.Fatalf("parsed tag = %+v, want offset=44001 length=32", sTag)
	}
	if _, err := crypto.ParseS3CryptoTag(`offset=bad`); err == nil {
		t.Fatal("expected error for malformed s3crypto tag, got nil")
	}
}
