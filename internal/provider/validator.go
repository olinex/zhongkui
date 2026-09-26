package provider

import (
	"context"

	ut "github.com/go-playground/universal-translator"
	uv "github.com/go-playground/validator/v10"
	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

func genValidatorFuncs(_ do.Injector) (map[string]uv.Func, error) {
	// 规则清单的唯一登记点在 validator.Funcs，此处只负责取用
	return validator.Funcs(), nil
}

func newUniversalTranslator(_ context.Context) do.Provider[*ut.UniversalTranslator] {
	return func(i do.Injector) (*ut.UniversalTranslator, error) {
		uni := validator.UniversalTranslator
		funcs, err := genValidatorFuncs(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		for tag, fn := range funcs {
			validator.Validate.RegisterValidation(tag, fn)
		}
		for _, lang := range base.SupportLanguages {
			for tag := range funcs {
				if err := validator.RegisterTranslation(tag, &lang); err != nil {
					return nil, errors.WithStack(err)
				}
			}
		}
		return uni, nil
	}
}

func InjectUniversalTranslator(ctx context.Context, i do.Injector) {
	do.Provide(i, newUniversalTranslator(ctx))
}

func InvokeUniversalTranslator(i do.Injector) (*ut.UniversalTranslator, error) {
	return do.Invoke[*ut.UniversalTranslator](i)
}

func MustInvokeUniversalTranslator(i do.Injector) *ut.UniversalTranslator {
	return do.MustInvoke[*ut.UniversalTranslator](i)
}
