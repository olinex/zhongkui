package key

import (
	"fmt"
	"os"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

const (
	CRYPTO_SOURCE_CONFIG = "config"
	CRYPTO_SOURCE_KMS    = "kms"
)

var (
	// 编译期断言：确保各 Provider 实现了 SecretKeyProvider 接口
	_ SecretKeyProvider = (*ConfigFileSecretKeyProvider)(nil)
	_ SecretKeyProvider = (*KMSSecretKeyProvider)(nil)
)

// SecretKeyProvider 定义主密钥提供者接口。
type SecretKeyProvider interface {
	GetSecretKey() ([]byte, error)
}

// ConfigFileSecretKeyProvider 从配置文件中读取主密钥。
type ConfigFileSecretKeyProvider struct {
	parentSecretKey string
}

// NewConfigFileSecretKeyProvider 创建从配置文件读取主密钥的提供者。
func NewConfigFileSecretKeyProvider(parentSecretKey string) *ConfigFileSecretKeyProvider {
	return &ConfigFileSecretKeyProvider{parentSecretKey: parentSecretKey}
}

// GetSecretKey 返回配置文件中的主密钥。
func (p *ConfigFileSecretKeyProvider) GetSecretKey() ([]byte, error) {
	if p.parentSecretKey == "" {
		return nil, errors.New("parent secret key is empty in config file")
	}
	fmt.Fprintf(os.Stderr, "WARNING: Using parent secret key from config file. This is NOT safe for production. Please switch to KMS as soon as possible.\n")
	return []byte(p.parentSecretKey), nil
}

// KMSSecretKeyProvider 从 KMS 服务获取主密钥（暂未实现）。
type KMSSecretKeyProvider struct{}

// NewKMSSecretKeyProvider 创建 KMS 主密钥提供者。
func NewKMSSecretKeyProvider() *KMSSecretKeyProvider {
	return &KMSSecretKeyProvider{}
}

// GetSecretKey 返回错误，提示 KMS 尚未实现。
func (p *KMSSecretKeyProvider) GetSecretKey() ([]byte, error) {
	return nil, errors.New("KMS secret key provider is not yet implemented, please configure and integrate your KMS service")
}

// NewSecretKeyProvider 根据 source 参数创建对应的主密钥提供者。
// source: "config" 或 "kms"。
func NewSecretKeyProvider(source string, parentSecretKey string) (SecretKeyProvider, error) {
	switch source {
	case CRYPTO_SOURCE_CONFIG:
		return NewConfigFileSecretKeyProvider(parentSecretKey), nil
	case CRYPTO_SOURCE_KMS:
		return NewKMSSecretKeyProvider(), nil
	default:
		return nil, errors.Newf(`unknown crypto.source: %s, must be '%s' or '%s'`, source, CRYPTO_SOURCE_CONFIG, CRYPTO_SOURCE_KMS)
	}
}
