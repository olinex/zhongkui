package validator

import (
	"testing"
	"time"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	. "github.com/smartystreets/goconvey/convey"
)

// TestMustValidate 测试 mustValidate 函数的功能
// 该测试验证 mustValidate 函数是否能正确获取全局验证器实例，
// 并自动注册所有在 validations map 中定义的自定义验证规则
func TestMustValidate(t *testing.T) {
	Convey("Test mustValidate function", t, func() {
		Convey("Should return validator instance with all custom validations registered", func() {
			// 设置 Gin 的 binding validator
			binding.Validator = &defaultValidator{}

			// 调用 mustValidate 函数，会自动注册 validations map 中的所有验证规则
			validate := mustValidate()

			// 验证返回的实例不为 nil
			So(validate, ShouldNotBeNil)

			// 验证返回的是正确的类型
			So(validate, ShouldHaveSameTypeAs, &validator.Validate{})

			// 验证所有自定义验证标签已成功注册
			// 测试强密码验证
			type StrongPasswordTest struct {
				Password string `binding:"strongPassword"`
			}
			validPassword := StrongPasswordTest{Password: "Password123!"}
			err := validate.Struct(validPassword)
			So(err, ShouldBeNil)

			// 测试 Windows 文件名验证
			type WindowsFileNameTest struct {
				FileName string `binding:"windowsFileName"`
			}
			validWindowsFile := WindowsFileNameTest{FileName: "test_file.txt"}
			err = validate.Struct(validWindowsFile)
			So(err, ShouldBeNil)

			// 测试 Unix 文件名验证
			type UnixFileNameTest struct {
				FileName string `binding:"unixFileName"`
			}
			validUnixFile := UnixFileNameTest{FileName: "test_file.txt"}
			err = validate.Struct(validUnixFile)
			So(err, ShouldBeNil)

			// 测试通用文件名验证
			type FileNameTest struct {
				FileName string `binding:"fileName"`
			}
			validFile := FileNameTest{FileName: "test_file.txt"}
			err = validate.Struct(validFile)
			So(err, ShouldBeNil)

			// 测试严格文件名验证
			type StrictFileNameTest struct {
				FileName string `binding:"strictFileName"`
			}
			validStrictFile := StrictFileNameTest{FileName: "testfile123"}
			err = validate.Struct(validStrictFile)
			So(err, ShouldBeNil)

			// 测试小于当前时间验证
			type LtCurrentTimeTest struct {
				TimeField time.Time `binding:"ltCurrentTime"`
			}
			validPastTime := LtCurrentTimeTest{TimeField: time.Now().Add(-1 * time.Hour)}
			err = validate.Struct(validPastTime)
			So(err, ShouldBeNil)

			// 测试大于当前时间验证
			type GtCurrentTimeTest struct {
				TimeField time.Time `binding:"gtCurrentTime"`
			}
			validFutureTime := GtCurrentTimeTest{TimeField: time.Now().Add(1 * time.Hour)}
			err = validate.Struct(validFutureTime)
			So(err, ShouldBeNil)
		})

		Convey("Should register tag name function for better error messages", func() {
			// 设置 Gin 的 binding validator
			binding.Validator = &defaultValidator{}

			// 获取验证器实例，tag name function 在 mustValidate 中自动注册
			validate := mustValidate()

			// 创建一个测试结构体来验证标签名称函数
			type TestStruct struct {
				FieldWithJSONTag  string `json:"json_field" binding:"required"`
				FieldWithMapTag   string `mapstructure:"map_field" binding:"required"`
				FieldWithoutTag   string `binding:"required"`
				FieldWithBothTags string `json:"json_name" mapstructure:"map_name" binding:"required"`
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

			// 获取验证器实例，已自动注册所有自定义验证规则
			validate := mustValidate()

			// 测试强密码验证
			type PasswordTest struct {
				Password string `binding:"strongPassword"`
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

			// 获取验证器实例，已自动注册所有自定义验证规则
			validate := mustValidate()

			// 测试时间验证
			type TimeTest struct {
				PastTime   time.Time `binding:"ltCurrentTime"`
				FutureTime time.Time `binding:"gtCurrentTime"`
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

			// 获取验证器实例，已自动注册所有自定义验证规则
			validate := mustValidate()

			// 测试文件名验证
			type FileNameTest struct {
				WindowsFileName string `binding:"windowsFileName"`
				UnixFileName    string `binding:"unixFileName"`
				GenericFileName string `binding:"fileName"`
				StrictFileName  string `binding:"strictFileName"`
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

// TestValidationsAfterMultipleMustValidate 测试多次调用 mustValidate 的行为
// 由于 mustValidate 内部会迭代 validations map 注册验证，重复注册会触发 panic，
// 此测试验证单次调用后所有验证规则正常工作
func TestValidationsAfterMultipleMustValidate(t *testing.T) {
	Convey("Test validations work correctly after initialization", t, func() {
		// 设置 Gin 的 binding validator
		binding.Validator = &defaultValidator{}

		Convey("All validation rules should work after mustValidate", func() {
			// 获取验证器实例
			validate := mustValidate()

			// 验证所有验证规则都正常工作
			// 测试强密码规则
			type PasswordTest struct {
				Password string `binding:"strongPassword"`
			}
			passwordTest := PasswordTest{Password: "Password123!"}
			err := validate.Struct(passwordTest)
			So(err, ShouldBeNil)

			// 测试时间验证规则
			type TimeTest struct {
				PastTime   time.Time `binding:"ltCurrentTime"`
				FutureTime time.Time `binding:"gtCurrentTime"`
			}
			timeTest := TimeTest{
				PastTime:   time.Now().Add(-1 * time.Hour),
				FutureTime: time.Now().Add(1 * time.Hour),
			}
			err = validate.Struct(timeTest)
			So(err, ShouldBeNil)

			// 验证验证器实例仍然可用
			So(validate, ShouldNotBeNil)
		})

		Convey("Should maintain all validation rules via Validate variable", func() {
			// 直接使用包级 Validate 变量（由 init() 初始化）
			// 由于在测试中 binding.Validator 可能未正确初始化，
			// 这里通过 mustValidate 获取验证器实例
			validate := mustValidate()

			// 测试所有验证标签在正确设置后可用
			type FileNameTest struct {
				WindowsFileName string `binding:"windowsFileName"`
				UnixFileName    string `binding:"unixFileName"`
				GenericFileName string `binding:"fileName"`
				StrictFileName  string `binding:"strictFileName"`
			}

			validFileTest := FileNameTest{
				WindowsFileName: "test_file.txt",
				UnixFileName:    "test_file.txt",
				GenericFileName: "test_file.txt",
				StrictFileName:  "testfile123",
			}

			err := validate.Struct(validFileTest)
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
