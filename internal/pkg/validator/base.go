package validator

import (
	"reflect"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

var (
	validations = map[string]func(fl validator.FieldLevel) bool{}
	Validate    *validator.Validate
)

// mustValidate 获取全局验证器实例，用于数据校验。
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
func mustValidate() *validator.Validate {
	v, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		panic("binding validator is not valid")
	}
	v.SetTagName("binding")
	for tag, fn := range validations {
		if err := v.RegisterValidation(tag, fn); err != nil {
			panic(err)
		}
	}
	// 注册标签名称函数，提高错误消息的可读性
	// 优先使用 json 标签，其次使用 mapstructure 标签，最后使用字段名称
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		if name, ok := fld.Tag.Lookup("json"); ok {
			return name
		} else if name, ok := fld.Tag.Lookup("mapstructure"); ok {
			return name
		} else {
			return fld.Name
		}
	})
	return v
}

func init() {
	Validate = mustValidate()
}
