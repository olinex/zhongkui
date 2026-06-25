package provider

import (
	"github.com/go-playground/validator/v10"
	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	v "zhongkui.com/zhongkui/internal/pkg/validator"
)

func newValidateProvider() do.Provider[*validator.Validate] {
	return func(i do.Injector) (*validator.Validate, error) {
		validate := v.MustValidate()
		if err := v.ValidatorRegister(validate); err != nil {
			return nil, errors.WithStack(err)
		}
		return validate, nil
	}
}

func InjectValidate(i do.Injector) {
	do.Provide(i, newValidateProvider())
}

func InvokeValidate(i do.Injector) (*validator.Validate, error) {
	return do.Invoke[*validator.Validate](i)
}

func MustInvokeValidate(i do.Injector) *validator.Validate {
	return do.MustInvoke[*validator.Validate](i)
}
