package key

import (
	"fmt"
	"os"
	"sync"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// 主密钥来源常量
const (
	// CRYPTO_SOURCE_CONFIG 从配置文件读取主密钥
	// 适用于开发环境，生产环境应使用 KMS
	CRYPTO_SOURCE_CONFIG = "config"

	// CRYPTO_SOURCE_KMS 从密钥管理服务获取主密钥
	// 推荐用于生产环境，提供更高的安全性
	CRYPTO_SOURCE_KMS = "kms"
)

var (
	// 编译期断言：确保各 Provider 实现了 SecretKeyProvider 接口
	_ SecretKeyProvider = (*ConfigFileSecretKeyProvider)(nil)
	_ SecretKeyProvider = (*KMSSecretKeyProvider)(nil)
)

// SecretKeyProvider 定义主密钥提供者接口。
//
// 主密钥是密钥派生的根密钥，用于派生出多个子密钥。
// 不同的实现支持从不同来源获取主密钥：
//   - 配置文件：适用于开发环境
//   - KMS（密钥管理服务）：适用于生产环境
type SecretKeyProvider interface {
	// GetSecretKey 获取主密钥
	//
	// 返回值：
	//   - []byte: 主密钥字节数组
	//   - error: 获取失败时返回错误
	GetSecretKey() ([]byte, error)
}

// ConfigFileSecretKeyProvider 从配置文件读取主密钥的提供者。
//
// 这是最简单的实现，直接从配置文件中读取明文主密钥。
// 适用于开发环境，不推荐用于生产环境。
//
// 安全注意事项：
//   - 主密钥以明文形式存储在配置文件中，存在泄露风险
//   - 首次调用 GetSecretKey 时会输出警告信息
//   - 生产环境应使用 KMSSecretKeyProvider
type ConfigFileSecretKeyProvider struct {
	// parentSecretKey 配置文件中的主密钥明文
	parentSecretKey string

	// warnOnce 确保警告信息只输出一次
	warnOnce sync.Once
}

// NewConfigFileSecretKeyProvider 创建从配置文件读取主密钥的提供者。
//
// 参数：
//   - parentSecretKey: 配置文件中的主密钥明文
//
// 返回值：
//   - *ConfigFileSecretKeyProvider: 提供者实例
func NewConfigFileSecretKeyProvider(parentSecretKey string) *ConfigFileSecretKeyProvider {
	return &ConfigFileSecretKeyProvider{parentSecretKey: parentSecretKey}
}

// GetSecretKey 返回配置文件中的主密钥。
//
// 首次调用时会输出警告信息到 stderr，提醒用户这不是安全的做法。
//
// 返回值：
//   - []byte: 主密钥字节数组
//   - error: 主密钥为空时返回错误
func (p *ConfigFileSecretKeyProvider) GetSecretKey() ([]byte, error) {
	if p.parentSecretKey == "" {
		return nil, errors.New("parent secret key is empty in config file")
	}
	p.warnOnce.Do(func() {
		fmt.Fprintf(os.Stderr, "WARNING: Using parent secret key from config file. This is NOT safe for production. Please switch to KMS as soon as possible.\n")
	})
	return []byte(p.parentSecretKey), nil
}

// KMSSecretKeyProvider 从 KMS 服务获取主密钥的提供者。
//
// KMS（Key Management Service）是专业的密钥管理服务，提供：
//   - 硬件级别的安全保护
//   - 密钥轮换和版本管理
//   - 审计日志和访问控制
//
// 注意：当前实现尚未完成，调用 GetSecretKey 会返回错误。
type KMSSecretKeyProvider struct{}

// NewKMSSecretKeyProvider 创建 KMS 主密钥提供者。
//
// 返回值：
//   - *KMSSecretKeyProvider: 提供者实例
func NewKMSSecretKeyProvider() *KMSSecretKeyProvider {
	return &KMSSecretKeyProvider{}
}

// GetSecretKey 从 KMS 服务获取主密钥。
//
// 当前实现返回错误，提示 KMS 尚未实现。
//
// 返回值：
//   - []byte: 主密钥字节数组（当前不会返回）
//   - error: 总是返回"未实现"错误
func (p *KMSSecretKeyProvider) GetSecretKey() ([]byte, error) {
	return nil, errors.New("KMS secret key provider is not yet implemented, please configure and integrate your KMS service")
}

// NewSecretKeyProvider 根据来源创建对应的主密钥提供者。
//
// 参数：
//   - source: 主密钥来源，必须是 "config" 或 "kms"
//   - parentSecretKey: 当 source 为 "config" 时，提供主密钥明文
//
// 返回值：
//   - SecretKeyProvider: 对应的提供者实例
//   - error: 来源无效时返回错误
//
// 示例：
//
//	provider, err := NewSecretKeyProvider("config", "my-secret-key")
//	if err != nil {
//	    return err
//	}
//	key, err := provider.GetSecretKey()
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
