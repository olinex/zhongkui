package validator

import (
	"reflect"

	uv "github.com/go-playground/validator/v10"
)

// testValidations 测试用的自定义验证规则注册表，直接取用规则清单的唯一登记点
var testValidations = Funcs()

// newTestValidate 创建注册了全部自定义验证规则与 binding 标签名的验证器实例
// 每次调用返回全新实例，避免在共享验证器上重复注册自定义规则触发 panic
func newTestValidate() *uv.Validate {
	v := uv.New()
	v.SetTagName("binding")
	// 注册标签名称函数，优先使用 json 标签，其次使用 mapstructure 标签，最后使用字段名称
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		if name, ok := fld.Tag.Lookup("json"); ok {
			return name
		} else if name, ok := fld.Tag.Lookup("mapstructure"); ok {
			return name
		} else {
			return fld.Name
		}
	})
	for tag, fn := range testValidations {
		if err := v.RegisterValidation(tag, fn); err != nil {
			panic(err)
		}
	}
	return v
}
