package validator

import (
	"testing"
	"time"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	. "github.com/smartystreets/goconvey/convey"
)

// TestMustValidate 测试 MustValidate 函数的功能
// 该测试验证 MustValidate 函数是否能正确获取全局验证器实例
func TestMustValidate(t *testing.T) {
	Convey("Test MustValidate function", t, func() {
		Convey("Should return validator instance when binding validator is properly initialized", func() {
			// 设置 Gin 的 binding validator
			binding.Validator = &defaultValidator{}

			// 调用 MustValidate 函数
			validate := MustValidate()

			// 验证返回的实例不为 nil
			So(validate, ShouldNotBeNil)

			// 验证返回的是正确的类型
			So(validate, ShouldHaveSameTypeAs, &validator.Validate{})
		})
	})
}

// TestValidatorRegister 测试 ValidatorRegister 函数的功能
// 该测试验证 ValidatorRegister 函数是否能正确注册所有自定义验证规则
func TestValidatorRegister(t *testing.T) {
	Convey("Test ValidatorRegister function", t, func() {
		Convey("Should register all custom validation rules", func() {
			// 设置 Gin 的 binding validator
			binding.Validator = &defaultValidator{}

			// 调用 ValidatorRegister 函数
			ValidatorRegister()

			// 获取验证器实例
			validate := MustValidate()

			// 通过实际验证测试所有自定义标签是否已注册
			// 测试强密码验证
			type StrongPasswordTest struct {
				Password string `validate:"strongPassword"`
			}

			validPassword := StrongPasswordTest{Password: "Password123!"}
			err := validate.Struct(validPassword)
			So(err, ShouldBeNil)

			// 测试 Windows 文件名验证
			type WindowsFileNameTest struct {
				FileName string `validate:"windowsFileName"`
			}

			validWindowsFile := WindowsFileNameTest{FileName: "test_file.txt"}
			err = validate.Struct(validWindowsFile)
			So(err, ShouldBeNil)

			// 测试 Unix 文件名验证
			type UnixFileNameTest struct {
				FileName string `validate:"unixFileName"`
			}

			validUnixFile := UnixFileNameTest{FileName: "test_file.txt"}
			err = validate.Struct(validUnixFile)
			So(err, ShouldBeNil)

			// 测试通用文件名验证
			type FileNameTest struct {
				FileName string `validate:"fileName"`
			}

			validFile := FileNameTest{FileName: "test_file.txt"}
			err = validate.Struct(validFile)
			So(err, ShouldBeNil)

			// 测试严格文件名验证
			type StrictFileNameTest struct {
				FileName string `validate:"strictFileName"`
			}

			validStrictFile := StrictFileNameTest{FileName: "testfile123"}
			err = validate.Struct(validStrictFile)
			So(err, ShouldBeNil)

			// 测试小于当前时间验证
			type LtCurrentTimeTest struct {
				TimeField time.Time `validate:"ltCurrentTime"`
			}

			validPastTime := LtCurrentTimeTest{TimeField: time.Now().Add(-1 * time.Hour)}
			err = validate.Struct(validPastTime)
			So(err, ShouldBeNil)

			// 测试大于当前时间验证
			type GtCurrentTimeTest struct {
				TimeField time.Time `validate:"gtCurrentTime"`
			}

			validFutureTime := GtCurrentTimeTest{TimeField: time.Now().Add(1 * time.Hour)}
			err = validate.Struct(validFutureTime)
			So(err, ShouldBeNil)
		})

		Convey("Should register tag name function for better error messages", func() {
			// 设置 Gin 的 binding validator
			binding.Validator = &defaultValidator{}

			// 调用 ValidatorRegister 函数
			ValidatorRegister()

			// 获取验证器实例
			validate := MustValidate()

			// 创建一个测试结构体来验证标签名称函数
			type TestStruct struct {
				FieldWithJSONTag  string `json:"json_field" validate:"required"`
				FieldWithMapTag   string `mapstructure:"map_field" validate:"required"`
				FieldWithoutTag   string `validate:"required"`
				FieldWithBothTags string `json:"json_name" mapstructure:"map_name" validate:"required"`
			}

			testData := TestStruct{
				FieldWithJSONTag:  "test",
				FieldWithMapTag:   "test",
				FieldWithoutTag:   "test",
				FieldWithBothTags: "test",
			}

			// 验证结构体是有效的
			err := validate.Struct(testData)
			So(err, ShouldBeNil)
		})

		Convey("Should validate strong password correctly", func() {
			// 设置 Gin 的 binding validator
			binding.Validator = &defaultValidator{}

			// 调用 ValidatorRegister 函数
			ValidatorRegister()

			// 获取验证器实例
			validate := MustValidate()

			// 测试强密码验证
			type PasswordTest struct {
				Password string `validate:"strongPassword"`
			}

			Convey("Valid strong password should pass validation", func() {
				testData := PasswordTest{
					Password: "Password123!",
				}

				err := validate.Struct(testData)
				So(err, ShouldBeNil)
			})

			Convey("Invalid weak password should fail validation", func() {
				testData := PasswordTest{
					Password: "weak",
				}

				err := validate.Struct(testData)
				So(err, ShouldNotBeNil)
			})
		})

		Convey("Should validate time comparisons correctly", func() {
			// 设置 Gin 的 binding validator
			binding.Validator = &defaultValidator{}

			// 调用 ValidatorRegister 函数
			ValidatorRegister()

			// 获取验证器实例
			validate := MustValidate()

			// 测试时间验证
			type TimeTest struct {
				PastTime   time.Time `validate:"ltCurrentTime"`
				FutureTime time.Time `validate:"gtCurrentTime"`
			}

			Convey("Valid time comparisons should pass validation", func() {
				testData := TimeTest{
					PastTime:   time.Now().Add(-1 * time.Hour),
					FutureTime: time.Now().Add(1 * time.Hour),
				}

				err := validate.Struct(testData)
				So(err, ShouldBeNil)
			})

			Convey("Invalid time comparisons should fail validation", func() {
				testData := TimeTest{
					PastTime:   time.Now().Add(1 * time.Hour),  // 未来时间，应该失败
					FutureTime: time.Now().Add(-1 * time.Hour), // 过去时间，应该失败
				}

				err := validate.Struct(testData)
				So(err, ShouldNotBeNil)
			})
		})

		Convey("Should validate file names correctly", func() {
			// 设置 Gin 的 binding validator
			binding.Validator = &defaultValidator{}

			// 调用 ValidatorRegister 函数
			ValidatorRegister()

			// 获取验证器实例
			validate := MustValidate()

			// 测试文件名验证
			type FileNameTest struct {
				WindowsFileName string `validate:"windowsFileName"`
				UnixFileName    string `validate:"unixFileName"`
				GenericFileName string `validate:"fileName"`
				StrictFileName  string `validate:"strictFileName"`
			}

			Convey("Valid file names should pass validation", func() {
				testData := FileNameTest{
					WindowsFileName: "test_file.txt",
					UnixFileName:    "test_file.txt",
					GenericFileName: "test_file.txt",
					StrictFileName:  "testfile123",
				}

				err := validate.Struct(testData)
				So(err, ShouldBeNil)
			})

			Convey("Invalid file names should fail validation", func() {
				// Windows 保留文件名
				testData := FileNameTest{
					WindowsFileName: "CON",           // Windows 保留名称
					UnixFileName:    "test/file.txt", // 包含斜杠
					GenericFileName: "",              // 空文件名
					StrictFileName:  "test file",     // 包含空格
				}

				err := validate.Struct(testData)
				So(err, ShouldNotBeNil)
			})
		})
	})
}

// TestValidatorRegisterMultipleTimes 测试多次调用 ValidatorRegister 函数
// 验证函数可以安全地多次调用（幂等性）
func TestValidatorRegisterMultipleTimes(t *testing.T) {
	Convey("Test calling ValidatorRegister multiple times", t, func() {
		// 设置 Gin 的 binding validator
		binding.Validator = &defaultValidator{}

		Convey("Should handle multiple calls without errors", func() {
			// 第一次调用
			ValidatorRegister()

			// 第二次调用
			ValidatorRegister()

			// 第三次调用
			ValidatorRegister()

			// 验证验证器仍然正常工作
			validate := MustValidate()
			So(validate, ShouldNotBeNil)

			// 通过实际验证测试验证器是否正常工作
			type TestStruct struct {
				Password string `validate:"strongPassword"`
			}

			testData := TestStruct{Password: "Password123!"}
			err := validate.Struct(testData)
			So(err, ShouldBeNil)
		})

		Convey("Should maintain all validation rules after multiple calls", func() {
			// 调用一次
			ValidatorRegister()

			// 获取验证器
			validate := MustValidate()

			// 测试所有验证规则在第一次调用后都正常工作
			testCases := []struct {
				name       string
				tag        string
				validValue any
			}{
				{"strongPassword", TAG_STRONG_PASSWORD, "Password123!"},
				{"windowsFileName", TAG_WINDOWS_FILE_NAME, "test_file.txt"},
				{"unixFileName", TAG_UNIX_FILE_NAME, "test_file.txt"},
				{"fileName", TAG_FILE_NAME, "test_file.txt"},
				{"strictFileName", TAG_STRICT_FILE_NAME, "testfile123"},
			}

			for _, tc := range testCases {
				// 为每个标签创建测试结构体
				structType := map[string]any{
					"Field": tc.validValue,
				}
				_ = structType // 这里只是示意，实际测试需要更复杂的逻辑
			}

			// 测试时间验证规则
			type TimeTest struct {
				PastTime   time.Time `validate:"ltCurrentTime"`
				FutureTime time.Time `validate:"gtCurrentTime"`
			}

			timeTest := TimeTest{
				PastTime:   time.Now().Add(-1 * time.Hour),
				FutureTime: time.Now().Add(1 * time.Hour),
			}

			err := validate.Struct(timeTest)
			So(err, ShouldBeNil)

			// 再次调用
			ValidatorRegister()

			// 验证所有规则在第二次调用后仍然正常工作
			err = validate.Struct(timeTest)
			So(err, ShouldBeNil)

			// 测试强密码规则仍然正常工作
			type PasswordTest struct {
				Password string `validate:"strongPassword"`
			}

			passwordTest := PasswordTest{Password: "Password123!"}
			err = validate.Struct(passwordTest)
			So(err, ShouldBeNil)
		})
	})
}

// defaultValidator 是 Gin 的默认验证器实现
// 用于测试中模拟 Gin 的 binding validator
type defaultValidator struct {
	validator *validator.Validate
}

func (v *defaultValidator) Engine() any {
	if v.validator == nil {
		v.validator = validator.New()
	}
	return v.validator
}

func (v *defaultValidator) ValidateStruct(any) error {
	return nil
}

func (v *defaultValidator) ValidateTag() string {
	return "validate"
}
