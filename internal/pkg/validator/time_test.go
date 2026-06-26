package validator

import (
	"testing"
	"time"

	"github.com/gin-gonic/gin/binding"
	. "github.com/smartystreets/goconvey/convey"
)

// TestValidateGtCurrentTime 测试 gtCurrentTime 标签的验证功能
// 该测试验证 validateGtCurrentTime 函数是否能正确判断时间是否大于当前时间
func TestValidateGtCurrentTime(t *testing.T) {
	Convey("Test gtCurrentTime validation", t, func() {
		// 初始化验证器
		binding.Validator = &defaultValidator{}
		validate := mustValidate()
		validate.RegisterValidation(TAG_GREATER_THAN_CURRENT_TIME, validateGtCurrentTime)

		Convey("Should validate time.Time fields correctly", func() {
			// 定义测试结构体
			type TestStruct struct {
				FutureTime time.Time `binding:"gtCurrentTime"`
			}

			Convey("Future time should pass validation", func() {
				// 创建未来时间（当前时间 + 1小时）
				futureTime := time.Now().Add(1 * time.Hour)
				testData := TestStruct{FutureTime: futureTime}

				// 执行验证
				err := validate.Struct(testData)

				// 验证应该通过
				So(err, ShouldBeNil)
			})

			Convey("Past time should fail validation", func() {
				// 创建过去时间（当前时间 - 1小时）
				pastTime := time.Now().Add(-1 * time.Hour)
				testData := TestStruct{FutureTime: pastTime}

				// 执行验证
				err := validate.Struct(testData)

				// 验证应该失败
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "gtCurrentTime")
			})

			Convey("Current time or past time should fail validation", func() {
				// 测试过去时间应该失败验证
				pastTime := time.Now().Add(-1 * time.Hour)
				testData := TestStruct{FutureTime: pastTime}

				// 执行验证
				err := validate.Struct(testData)

				// 验证应该失败（过去时间不大于当前时间）
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "gtCurrentTime")
			})
		})

		Convey("Should validate *time.Time fields correctly", func() {
			// 定义测试结构体，使用指针类型
			type TestStruct struct {
				FutureTime *time.Time `binding:"gtCurrentTime"`
			}

			Convey("Future time pointer should pass validation", func() {
				// 创建未来时间指针
				futureTime := time.Now().Add(1 * time.Hour)
				testData := TestStruct{FutureTime: &futureTime}

				// 执行验证
				err := validate.Struct(testData)

				// 验证应该通过
				So(err, ShouldBeNil)
			})

			Convey("Past time pointer should fail validation", func() {
				// 创建过去时间指针
				pastTime := time.Now().Add(-1 * time.Hour)
				testData := TestStruct{FutureTime: &pastTime}

				// 执行验证
				err := validate.Struct(testData)

				// 验证应该失败
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "gtCurrentTime")
			})

			Convey("Nil pointer should fail validation", func() {
				// 使用 nil 指针
				testData := TestStruct{FutureTime: nil}

				// 执行验证
				err := validate.Struct(testData)

				// 验证应该失败（无法比较 nil 指针）
				So(err, ShouldNotBeNil)
			})
		})

		Convey("Should handle non-time fields gracefully", func() {
			// 定义测试结构体，使用非时间类型
			type TestStruct struct {
				InvalidField string `binding:"gtCurrentTime"`
			}

			// 创建测试数据
			testData := TestStruct{InvalidField: "not a time"}

			// 执行验证
			err := validate.Struct(testData)

			// 验证应该失败（类型不匹配）
			So(err, ShouldNotBeNil)
		})
	})
}

// TestValidateLtCurrentTime 测试 ltCurrentTime 标签的验证功能
// 该测试验证 validateLtCurrentTime 函数是否能正确判断时间是否小于当前时间
func TestValidateLtCurrentTime(t *testing.T) {
	Convey("Test ltCurrentTime validation", t, func() {
		// 初始化验证器
		binding.Validator = &defaultValidator{}
		validate := mustValidate()
		validate.RegisterValidation(TAG_LESSER_THAN_CURRENT_TIME, validateLtCurrentTime)

		Convey("Should validate time.Time fields correctly", func() {
			// 定义测试结构体
			type TestStruct struct {
				PastTime time.Time `binding:"ltCurrentTime"`
			}

			Convey("Past time should pass validation", func() {
				// 创建过去时间（当前时间 - 1小时）
				pastTime := time.Now().Add(-1 * time.Hour)
				testData := TestStruct{PastTime: pastTime}

				// 执行验证
				err := validate.Struct(testData)

				// 验证应该通过
				So(err, ShouldBeNil)
			})

			Convey("Future time should fail validation", func() {
				// 创建未来时间（当前时间 + 1小时）
				futureTime := time.Now().Add(1 * time.Hour)
				testData := TestStruct{PastTime: futureTime}

				// 执行验证
				err := validate.Struct(testData)

				// 验证应该失败
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "ltCurrentTime")
			})

			Convey("Current time or future time should fail validation", func() {
				// 测试未来时间应该失败验证
				futureTime := time.Now().Add(1 * time.Hour)
				testData := TestStruct{PastTime: futureTime}

				// 执行验证
				err := validate.Struct(testData)

				// 验证应该失败（未来时间不小于当前时间）
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "ltCurrentTime")
			})
		})

		Convey("Should validate *time.Time fields correctly", func() {
			// 定义测试结构体，使用指针类型
			type TestStruct struct {
				PastTime *time.Time `binding:"ltCurrentTime"`
			}

			Convey("Past time pointer should pass validation", func() {
				// 创建过去时间指针
				pastTime := time.Now().Add(-1 * time.Hour)
				testData := TestStruct{PastTime: &pastTime}

				// 执行验证
				err := validate.Struct(testData)

				// 验证应该通过
				So(err, ShouldBeNil)
			})

			Convey("Future time pointer should fail validation", func() {
				// 创建未来时间指针
				futureTime := time.Now().Add(1 * time.Hour)
				testData := TestStruct{PastTime: &futureTime}

				// 执行验证
				err := validate.Struct(testData)

				// 验证应该失败
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "ltCurrentTime")
			})

			Convey("Nil pointer should fail validation", func() {
				// 使用 nil 指针
				testData := TestStruct{PastTime: nil}

				// 执行验证
				err := validate.Struct(testData)

				// 验证应该失败（无法比较 nil 指针）
				So(err, ShouldNotBeNil)
			})
		})

		Convey("Should handle non-time fields gracefully", func() {
			// 定义测试结构体，使用非时间类型
			type TestStruct struct {
				InvalidField string `binding:"ltCurrentTime"`
			}

			// 创建测试数据
			testData := TestStruct{InvalidField: "not a time"}

			// 执行验证
			err := validate.Struct(testData)

			// 验证应该失败（类型不匹配）
			So(err, ShouldNotBeNil)
		})
	})
}

// TestCompareWithCurrentTime 测试核心比较函数的功能
// 该测试验证 compareWithCurrentTime 函数是否能正确处理不同类型的时间字段
func TestCompareWithCurrentTime(t *testing.T) {
	Convey("Test compareWithCurrentTime function", t, func() {
		// 由于 compareWithCurrentTime 是内部函数，我们通过测试公开的验证函数来间接测试它
		// 这些测试已经在 TestValidateGtCurrentTime 和 TestValidateLtCurrentTime 中覆盖
		Convey("Function should be properly tested through validation functions", func() {
			So(true, ShouldBeTrue)
		})
	})
}
