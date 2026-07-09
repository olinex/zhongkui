package provider

import (
	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

const CRYPTO_PREFIX = "crypto"

func newKeyDerivationProvider() do.Provider[*crypto.KeyDerivation] {
	return func(i do.Injector) (*crypto.KeyDerivation, error) {
		if viper, err := InvokeViper(i); err != nil {
			return nil, errors.WithStack(err)
		} else if cfg, err := crypto.ExtractConfig(viper); err != nil {
			return nil, errors.WithStack(err)
		} else if pb, err := crypto.LoadPasswordBook(cfg.PasswordBookPath); err != nil {
			return nil, errors.WithStack(err)
		} else if sp, err := crypto.NewSecretKeyProvider(cfg.Source, cfg.ParentSecretKey); err != nil {
			return nil, errors.WithStack(err)
		} else {
			return crypto.NewKeyDerivation(sp, pb), nil
		}
	}
}

func InjectKeyDerivation(i do.Injector) {
	do.Provide(i, newKeyDerivationProvider())
}

func InvokeKeyDerivation(i do.Injector) (*crypto.KeyDerivation, error) {
	return do.Invoke[*crypto.KeyDerivation](i)
}

func MustInvokeKeyDerivation(i do.Injector) *crypto.KeyDerivation {
	return do.MustInvoke[*crypto.KeyDerivation](i)
}

func DescribeCryptoConfig() config.ConfigDescriptions {
	return crypto.DescribeConfig()
}
