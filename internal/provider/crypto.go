package provider

import (
	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

const CRYPTO_PREFIX = "crypto"

func newKeyDerivationProvider() do.Provider[*key.KeyDerivation] {
	return func(i do.Injector) (*key.KeyDerivation, error) {
		if viper, err := InvokeViper(i); err != nil {
			return nil, errors.WithStack(err)
		} else if cfg, err := key.ExtractConfig(viper); err != nil {
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

func InjectKeyDerivation(i do.Injector) {
	do.Provide(i, newKeyDerivationProvider())
}

func InvokeKeyDerivation(i do.Injector) (*key.KeyDerivation, error) {
	return do.Invoke[*key.KeyDerivation](i)
}

func MustInvokeKeyDerivation(i do.Injector) *key.KeyDerivation {
	return do.MustInvoke[*key.KeyDerivation](i)
}

func DescribeCryptoConfig() config.ConfigDescriptions {
	return key.DescribeConfig()
}
