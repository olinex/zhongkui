package validator

import (
	"reflect"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

var validations = map[string]func(fl validator.FieldLevel) bool{}

// MustValidate 获取全局验证器实例，用于数据校验。
// 从 Gin 框架的 binding.Validator.Engine() 中提取 validator.Validate 实例，
// 该函数通常在应用启动时调用，确保验证器可用。
//
// 使用场景：
// 1. 在应用启动时初始化验证器
// 2. 注册自定义验证规则
// 3. 获取验证器实例进行手动验证
//
// 注意事项：
// - 如果类型断言失败，函数会触发 panic，因此应在应用启动时确保验证器正确配置
// - 该函数依赖于 Gin 框架的 binding 包，确保 Gin 已正确初始化
//
// 返回值：
// *validator.Validate - 全局验证器实例，可用于注册自定义验证规则或执行验证
func MustValidate() *validator.Validate {
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		v.SetTagName("binding")
		return v
	} else {
		panic("binding validator is not valid")
	}
}

// ValidatorRegister 注册所有自定义验证规则到全局验证器。
// 该函数应在应用启动时调用一次，注册所有自定义验证标签和对应的验证函数。
//
// 同时注册标签名称函数，优先使用 json 标签作为字段名称，其次使用 mapstructure 标签，
// 最后使用字段本身的名称。这提高了验证错误消息的可读性。
//
// 使用示例：
// 在应用启动时调用：validator.ValidatorRegister()
// 在结构体中使用：`validate:"strongPassword"` 或 `validate:"gtCurrentTime"`
//
// 注意事项：
// - 该函数应在应用启动时只调用一次，避免重复注册
// - 确保所有依赖的验证函数已正确定义
// - 注册后可在整个应用中使用这些自定义验证规则
func ValidatorRegister(validate *validator.Validate) error {
	for tag, fn := range validations {
		if err := validate.RegisterValidation(tag, fn); err != nil {
			return err
		}
	}
	// 注册标签名称函数，提高错误消息的可读性
	// 优先使用 json 标签，其次使用 mapstructure 标签，最后使用字段名称
	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		if name, ok := fld.Tag.Lookup("json"); ok {
			return name
		} else if name, ok := fld.Tag.Lookup("mapstructure"); ok {
			return name
		} else {
			return fld.Name
		}
	})
	return nil
}
