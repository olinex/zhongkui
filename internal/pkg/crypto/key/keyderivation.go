package key

import (
	"crypto/sha256"
	"io"
	"math"
	"sync"

	"golang.org/x/crypto/hkdf"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

const (
	MIN_KEY_LENGTH = 16
)

var (
	globalKeyDerivation *KeyDerivation
	keyDerivationOnce   sync.Once
)

type KeyDerivation struct {
	secretKeyProvider SecretKeyProvider
	passwordBook      *PasswordBook
}

func NewKeyDerivation(secretKeyProvider SecretKeyProvider, passwordBook *PasswordBook) *KeyDerivation {
	return &KeyDerivation{
		secretKeyProvider: secretKeyProvider,
		passwordBook:      passwordBook,
	}
}

// InitGlobalKeyDerivation 使用 sync.Once 初始化全局唯一的 KeyDerivation 实例。
// 多次调用仅首次生效，后续调用不会重复初始化。
func InitGlobalKeyDerivation(secretKeyProvider SecretKeyProvider, passwordBook *PasswordBook) {
	keyDerivationOnce.Do(func() {
		globalKeyDerivation = NewKeyDerivation(secretKeyProvider, passwordBook)
	})
}

// GetKeyDerivation 返回全局唯一的 KeyDerivation 实例。
// 如果尚未调用 InitGlobalKeyDerivation 初始化，则返回 nil。
func GetKeyDerivation() *KeyDerivation {
	return globalKeyDerivation
}

func (k *KeyDerivation) DeriveChildKey(offset, length int) ([]byte, error) {
	if length <= MIN_KEY_LENGTH {
		return nil, errors.Newf("key length must be greater than %d, got: %d", MIN_KEY_LENGTH, length)
	}

	parentKey, err := k.secretKeyProvider.GetSecretKey()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	bookBytes := k.passwordBook.Bytes()
	bookLen := k.passwordBook.Len()
	saltLen := int(math.Max(MIN_KEY_LENGTH, float64(length)))

	var salt []byte
	if offset+saltLen > bookLen {
		firstPart := bookBytes[offset:]
		remaining := (offset + saltLen) - bookLen
		secondPart := bookBytes[:remaining]
		salt = make([]byte, 0, saltLen)
		salt = append(salt, firstPart...)
		salt = append(salt, secondPart...)
	} else {
		salt = bookBytes[offset : offset+saltLen]
	}

	hkdfReader := hkdf.New(sha256.New, parentKey, salt, nil)

	key := make([]byte, length)
	if _, err := io.ReadFull(hkdfReader, key); err != nil {
		return nil, errors.WithStack(err)
	}

	return key, nil
}
