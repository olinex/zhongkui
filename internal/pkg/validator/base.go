package validator

import (
	"fmt"
	"reflect"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

// Validate 是全局共享的校验器实例，绑定 gin 的 binding.Validator 引擎，
// 已注册自定义标签名称函数：优先取 json 标签名，其次 mapstructure 标签名，最后回退字段名。
var (
	Validate *validator.Validate
)

func newValidate() *validator.Validate {
	v, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		panic("binding validator is not valid")
	}
	v.SetTagName("binding")
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

// Funcs 返回全部自定义校验规则的登记表（tag → 校验函数），是自定义规则的唯一登记处：
// provider 用它把规则注册到全局校验器并为每个 tag 挂本地化文案，测试用它注册到自己的校验器。
// 集中登记可避免各调用方重复抄写 tag 清单——漏抄某项会让该 tag 在对应场景被判为未注册规则，
// 表现为绑定失败而非编译错误。
func Funcs() map[string]validator.Func {
	return map[string]validator.Func{
		TagStrongPassword:         ValidateStrongPassword,
		TagWindowsFileName:        ValidateWindowsFileName,
		TagUnixFileName:           ValidateUnixFileName,
		TagFileName:               ValidateFileName,
		TagStrictFileName:         ValidateStrictFileName,
		TagDirpath:                ValidateDirPath,
		TagAlphanumSpecial:        ValidateAlphanumSpecial,
		TagEnum:                   ValidateEnum,
		TagHTTPURL:                ValidateHTTPURL,
		TagJSONObject:             ValidateJSONObject,
		TagTrimmed:                ValidateTrimmed,
		TagNotBlank:               ValidateNotBlank,
		TagGreaterThanCurrentTime: ValidateGtCurrentTime,
		TagLesserThanCurrentTime:  ValidateLtCurrentTime,
		TagCron:                   ValidateCronSpec,
	}
}

func init() {
	Validate = newValidate()
	// 全局引擎自注册自定义校验规则：任何包（worker / CLI / 测试等）无需重复登记即可执行自定义 tag；
	// 同名 tag 的 RegisterValidation 是覆盖写入，provider 与测试中的重复注册不会 panic。
	for tag, fn := range Funcs() {
		// 注册失败说明 tag 名非法或函数为 nil，属编程错误：启动期直接暴露，避免规则静默失效。
		if err := Validate.RegisterValidation(tag, fn); err != nil {
			panic(fmt.Sprintf("validator: register custom tag %q failed: %v", tag, err))
		}
	}
}
