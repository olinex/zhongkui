package validator

import (
	"reflect"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

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
		return v
	} else {
		panic("binding validator is not valid")
	}
}

// ValidatorRegister 注册所有自定义验证规则到全局验证器。
// 该函数应在应用启动时调用一次，注册所有自定义验证标签和对应的验证函数。
//
// 注册的验证标签包括：
// 1. strongPassword - 强密码验证，要求包含大小写字母、数字和特殊字符
// 2. windowsFileName - Windows 文件名验证，确保文件名符合 Windows 系统规范
// 3. unixFileName - Unix/Linux 文件名验证，确保文件名符合 Unix 系统规范
// 4. fileName - 通用文件名验证，适用于跨平台场景
// 5. strictFileName - 严格文件名验证，包含更多限制条件
// 6. ltCurrentTime - 小于当前时间验证，确保时间字段在当前时间之前
// 7. gtCurrentTime - 大于当前时间验证，确保时间字段在当前时间之后
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
func ValidatorRegister() {
	validate := MustValidate()

	// 注册强密码验证规则
	validate.RegisterValidation(TAG_STRONG_PASSWORD, validateStrongPassword)

	// 注册 Windows 文件名验证规则
	validate.RegisterValidation(TAG_WINDOWS_FILE_NAME, validateWindowsFileName)

	// 注册 Unix/Linux 文件名验证规则
	validate.RegisterValidation(TAG_UNIX_FILE_NAME, validateUnixFileName)

	// 注册通用文件名验证规则
	validate.RegisterValidation(TAG_FILE_NAME, validateFileName)

	// 注册严格文件名验证规则
	validate.RegisterValidation(TAG_STRICT_FILE_NAME, validateStrictFileName)

	// 注册目录路径验证规则
	validate.RegisterValidation(TAG_DIRPATH, validateDirPath)

	// 注册小于当前时间验证规则
	validate.RegisterValidation(TAG_LESSER_THAN_CURRENT_TIME, validateLtCurrentTime)

	// 注册大于当前时间验证规则
	validate.RegisterValidation(TAG_GREATER_THAN_CURRENT_TIME, validateGtCurrentTime)

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
}
