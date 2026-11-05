package provider

import (
	"context"

	ut "github.com/go-playground/universal-translator"
	uv "github.com/go-playground/validator/v10"
	"github.com/samber/do/v2"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
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
			if err := validator.Validate.RegisterValidation(tag, fn); err != nil {
				return nil, errors.WithStack(err)
			}
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

// InjectUniversalTranslator 向依赖注入器注册通用翻译器 provider，并登记校验规则及其多语言翻译。
func InjectUniversalTranslator(ctx context.Context, i do.Injector) {
	do.Provide(i, newUniversalTranslator(ctx))
}

// InvokeUniversalTranslator 从依赖注入器解析通用翻译器，错误交由调用方处理。
func InvokeUniversalTranslator(i do.Injector) (*ut.UniversalTranslator, error) {
	return do.Invoke[*ut.UniversalTranslator](i)
}

// MustInvokeUniversalTranslator 从依赖注入器解析通用翻译器，失败时 panic。
func MustInvokeUniversalTranslator(i do.Injector) *ut.UniversalTranslator {
	return do.MustInvoke[*ut.UniversalTranslator](i)
}
