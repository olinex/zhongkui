package provider

import (
	"github.com/samber/do/v2"
	"zhongkuisec.com/zhongkui/internal/pkg/config"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto/key"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

func newKeyDerivationProvider() do.Provider[*key.KeyDerivation] {
	return func(i do.Injector) (*key.KeyDerivation, error) {
		if viper, err := InvokeViper(i); err != nil {
			return nil, errors.WithStack(err)
		} else if cfg, err := key.ExtractConfig(viper, validator.Validate); err != nil {
			return nil, errors.WithStack(err)
		} else if pb, err := key.LoadPasswordBook(cfg.PasswordBookPath); err != nil {
			return nil, errors.WithStack(err)
		} else if sp, err := key.NewSecretKeyProvider(cfg.Source, cfg.ParentSecretKey); err != nil {
			return nil, errors.WithStack(err)
		} else {
			kd := key.NewKeyDerivation(sp, pb)
			key.InitGlobalKeyDerivation(sp, pb)
			return kd, nil
		}
	}
}

// InjectKeyDerivation 向依赖注入器注册密钥派生器 provider，并初始化全局密钥派生器。
func InjectKeyDerivation(i do.Injector) {
	do.Provide(i, newKeyDerivationProvider())
}

// InvokeKeyDerivation 从依赖注入器解析密钥派生器，错误交由调用方处理。
func InvokeKeyDerivation(i do.Injector) (*key.KeyDerivation, error) {
	return do.Invoke[*key.KeyDerivation](i)
}

// MustInvokeKeyDerivation 从依赖注入器解析密钥派生器，失败时 panic。
func MustInvokeKeyDerivation(i do.Injector) *key.KeyDerivation {
	return do.MustInvoke[*key.KeyDerivation](i)
}

// DescribeCryptoConfig 返回加密密钥相关配置项描述。
func DescribeCryptoConfig() config.ConfigDescriptions {
	return key.DescribeConfig()
}
