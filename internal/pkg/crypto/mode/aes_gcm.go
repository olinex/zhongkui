package mode

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

const (
	AES_GCM            = "aes-gcm"
	AES_GCM_TAG_SIZE   = 16 // GCM 认证标签长度
	AES_GCM_NONCE_SIZE = 12
)

// 编译期断言：确保 AESGCMMode 实现了 Mode 接口
var _ Mode = (*AESGCMMode)(nil)

type AESGCMMode struct {
	gcm            cipher.AEAD
	additionalData []byte
}

func NewAESGCMMode(cryptoKey, additionalData []byte) (*AESGCMMode, error) {
	block, err := aes.NewCipher(cryptoKey)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &AESGCMMode{gcm: gcm, additionalData: additionalData}, nil
}

func (e *AESGCMMode) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, AES_GCM_NONCE_SIZE)
	if _, err := rand.Read(nonce); err != nil {
		return nil, errors.WithStack(err)
	}
	ciphertext := e.gcm.Seal(nil, nonce, plaintext, e.additionalData)
	return append(nonce, ciphertext...), nil
}

func (e *AESGCMMode) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < AES_GCM_NONCE_SIZE+AES_GCM_TAG_SIZE {
		return nil, errors.Newf("ciphertext too short: expected at least %d bytes, got %d",
			AES_GCM_NONCE_SIZE+AES_GCM_TAG_SIZE, len(ciphertext))
	}
	nonce := ciphertext[:AES_GCM_NONCE_SIZE]
	ciphertext = ciphertext[AES_GCM_NONCE_SIZE:]
	return e.gcm.Open(nil, nonce, ciphertext, e.additionalData)
}
