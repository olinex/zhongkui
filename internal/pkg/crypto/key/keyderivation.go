package key

import (
	"crypto/sha256"
	"io"
	"math"
	"sync"

	"golang.org/x/crypto/hkdf"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// MinKeyLength 定义派生密钥的最小长度（字节）
// 密钥长度必须大于此值，以确保足够的安全性
const (
	MinKeyLength = 16
)

var (
	// globalKeyDerivation 全局唯一的密钥派生器实例
	globalKeyDerivation *KeyDerivation

	// keyDerivationOnce 确保全局实例只初始化一次
	keyDerivationOnce sync.Once

	// ErrNotInitialized 表示密钥派生器未初始化
	ErrNotInitialized = errors.New("crypto: KeyDerivation not initialized")
)

// KeyDerivation 密钥派生器，使用 HKDF 算法从主密钥派生子密钥。
//
// HKDF（HMAC-based Key Derivation Function）是一种基于 HMAC 的密钥派生函数，
// 定义在 RFC 5869 中。它接受三个输入：
//   - IKM（Input Keying Material）：输入密钥材料，即主密钥
//   - salt：可选的盐值，用于增加密钥派生的随机性
//   - info：可选的上下文信息，用于派生不同用途的密钥
//
// 本实现使用：
//   - 主密钥作为 IKM
//   - 密码本中指定偏移位置的字节作为 salt
//   - info 参数为 nil
//
// 密钥派生流程：
//  1. 从 SecretKeyProvider 获取主密钥
//  2. 从 PasswordBook 的 offset 位置读取 salt（支持循环读取）
//  3. 使用 HKDF-SHA256 派生指定长度的子密钥
type KeyDerivation struct {
	// secretKeyProvider 主密钥提供者
	secretKeyProvider SecretKeyProvider

	// passwordBook 密码本，提供密钥派生的盐值
	passwordBook *PasswordBook
}

// NewKeyDerivation 创建新的密钥派生器实例。
//
// 参数：
//   - secretKeyProvider: 主密钥提供者，用于获取根密钥
//   - passwordBook: 密码本，提供密钥派生的盐值数据
//
// 返回值：
//   - *KeyDerivation: 密钥派生器实例
func NewKeyDerivation(secretKeyProvider SecretKeyProvider, passwordBook *PasswordBook) *KeyDerivation {
	return &KeyDerivation{
		secretKeyProvider: secretKeyProvider,
		passwordBook:      passwordBook,
	}
}

// InitGlobalKeyDerivation 初始化全局唯一的密钥派生器实例。
//
// 使用 sync.Once 确保多次调用仅首次生效，后续调用不会重复初始化。
// 这在应用启动时调用，确保全局只有一个密钥派生器实例。
//
// 参数：
//   - secretKeyProvider: 主密钥提供者
//   - passwordBook: 密码本
func InitGlobalKeyDerivation(secretKeyProvider SecretKeyProvider, passwordBook *PasswordBook) {
	keyDerivationOnce.Do(func() {
		globalKeyDerivation = NewKeyDerivation(secretKeyProvider, passwordBook)
	})
}

// GetKeyDerivation 返回全局唯一的密钥派生器实例。
//
// 如果尚未调用 InitGlobalKeyDerivation 初始化，返回错误，调用方无需再做 nil 判断。
//
// 返回值：
//   - *KeyDerivation: 全局密钥派生器实例
//   - error: 未初始化时返回 "crypto: KeyDerivation not initialized"
func GetKeyDerivation() (*KeyDerivation, error) {
	if globalKeyDerivation == nil {
		return nil, ErrNotInitialized
	}
	return globalKeyDerivation, nil
}

// DeriveChildKey 从主密钥派生子密钥。
//
// 派生过程：
//  1. 从密码本的 offset 位置读取 salt（长度为 max(MinKeyLength, length)）
//  2. 如果 offset+saltLen 超过密码本长度，则循环读取（从开头继续）
//  3. 使用 HKDF-SHA256 算法派生指定长度的子密钥
//
// 参数：
//   - offset: 从密码本的哪个位置开始读取 salt
//   - length: 期望派生的子密钥长度（字节），必须大于 MinKeyLength
//
// 返回值：
//   - []byte: 派生的子密钥，长度为 length
//   - error: 密钥长度无效、获取主密钥失败或派生失败时返回错误
//
// 错误场景：
//   - length <= MinKeyLength：密钥长度过短
//   - 主密钥提供者返回错误
//   - HKDF 派生过程出错
//
// 示例：
//
//	key, err := kd.DeriveChildKey(100, 32)
//	if err != nil {
//	    return err
//	}
//	// key 是 32 字节的派生密钥
func (k *KeyDerivation) DeriveChildKey(offset, length int) ([]byte, error) {
	// 检查密钥长度
	if length <= MinKeyLength {
		return nil, errors.Newf("key length should greater than %d, got: %d", MinKeyLength, length)
	}

	// 获取主密钥
	parentKey, err := k.secretKeyProvider.GetSecretKey()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// 准备 salt
	bookBytes := k.passwordBook.Bytes()
	bookLen := k.passwordBook.Len()
	saltLen := int(math.Max(MinKeyLength, float64(length)))

	var salt []byte
	// 如果 offset+saltLen 超过密码本长度，则循环读取
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

	// 使用 HKDF-SHA256 派生密钥
	hkdfReader := hkdf.New(sha256.New, parentKey, salt, nil)

	key := make([]byte, length)
	if _, err := io.ReadFull(hkdfReader, key); err != nil {
		return nil, errors.WithStack(err)
	}

	return key, nil
}
