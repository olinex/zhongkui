package key

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/translate"
)

// 主密钥来源常量
const (
	// CryptoSourceConfig 从配置文件读取主密钥
	// 适用于开发环境，生产环境应使用 KMS
	CryptoSourceConfig = "config"

	// CryptoSourceKms 通过外部 KMS 命令获取主密钥
	// 推荐用于生产环境，提供更高的安全性
	CryptoSourceKms = "kms"
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
		fmt.Fprintln(os.Stderr, translate.T("cli.crypto.parent-secret-key-warning", nil))
	})
	return []byte(p.parentSecretKey), nil
}

// defaultKMSCommandTimeout 是 KMS 命令未配置有效超时时间时使用的默认值。
const defaultKMSCommandTimeout = 10 * time.Second

// KMSSecretKeyProvider 通过外部命令从 KMS 获取主密钥的提供者。
//
// 该实现不绑定具体厂商：命令可以是 aws/aliyun/vault 等 KMS CLI，也可以是封装好的
// 取密钥脚本。命令的标准输出（去掉首尾空白）即主密钥明文。
//
// 特性：
//   - 命令以 argv 形式执行，不经过 shell，避免注入；
//   - 受超时时间约束，避免启动被 KMS 长时间阻塞；
//   - 首次取值后缓存结果（含错误），后续派生密钥不再重复执行命令。
type KMSSecretKeyProvider struct {
	// command 获取主密钥的外部命令及其参数
	command []string

	// timeout 命令执行超时时间
	timeout time.Duration

	// once 确保命令只执行一次
	once sync.Once

	// secretKey 缓存的主密钥
	secretKey []byte

	// err 缓存取值错误
	err error
}

// NewKMSSecretKeyProvider 创建 KMS 主密钥提供者。
//
// 参数：
//   - command: 获取主密钥的外部命令及其参数（argv 形式）
//   - timeout: 命令执行超时时间，非正数时使用默认值 10s
//
// 返回值：
//   - *KMSSecretKeyProvider: 提供者实例
//   - error: command 为空时返回错误
func NewKMSSecretKeyProvider(command []string, timeout time.Duration) (*KMSSecretKeyProvider, error) {
	if len(command) == 0 {
		return nil, errors.New("crypto.kms.command is required when crypto.source is kms")
	}
	if timeout <= 0 {
		timeout = defaultKMSCommandTimeout
	}
	return &KMSSecretKeyProvider{command: command, timeout: timeout}, nil
}

// GetSecretKey 获取主密钥。
//
// 首次调用时执行外部命令并缓存结果（成功或失败），后续调用直接返回缓存。
//
// 返回值：
//   - []byte: 主密钥字节数组
//   - error: 命令执行失败、超时或输出为空时返回错误
func (p *KMSSecretKeyProvider) GetSecretKey() ([]byte, error) {
	p.once.Do(func() {
		p.secretKey, p.err = p.fetchSecretKey()
	})
	return p.secretKey, p.err
}

// fetchSecretKey 执行外部命令并解析主密钥。
func (p *KMSSecretKeyProvider) fetchSecretKey() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, p.command[0], p.command[1:]...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, errors.Newf("crypto.kms.command %q timed out after %s", p.command[0], p.timeout)
		}
		if stderrMsg := strings.TrimSpace(stderr.String()); stderrMsg != "" {
			return nil, errors.Newf("crypto.kms.command %q failed: %v: %s", p.command[0], err, stderrMsg)
		}
		return nil, errors.Newf("crypto.kms.command %q failed: %v", p.command[0], err)
	}

	secretKey := strings.TrimSpace(stdout.String())
	if secretKey == "" {
		return nil, errors.New("crypto.kms.command returned empty secret key")
	}
	return []byte(secretKey), nil
}

// NewSecretKeyProvider 根据来源创建对应的主密钥提供者。
//
// 参数：
//   - cfg: 密钥管理配置，Source 决定具体实现
//
// 返回值：
//   - SecretKeyProvider: 对应的提供者实例
//   - error: 来源无效或 KMS 命令未配置时返回错误
//
// 示例：
//
//	cfg := Config{Source: CryptoSourceConfig, ParentSecretKey: "my-secret-key"}
//	provider, err := NewSecretKeyProvider(cfg)
//	if err != nil {
//	    return errors.WithStack(err)
//	}
//	key, err := provider.GetSecretKey()
func NewSecretKeyProvider(cfg Config) (SecretKeyProvider, error) {
	switch cfg.Source {
	case CryptoSourceConfig:
		return NewConfigFileSecretKeyProvider(cfg.ParentSecretKey), nil
	case CryptoSourceKms:
		return NewKMSSecretKeyProvider(cfg.KMS.Command, cfg.KMS.Timeout)
	default:
		return nil, errors.Newf(`unknown crypto.source: %s, must be '%s' or '%s'`, cfg.Source, CryptoSourceConfig, CryptoSourceKms)
	}
}
