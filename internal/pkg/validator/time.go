package validator

import (
	"time"

	"github.com/go-playground/validator/v10"
)

const (
	// TAG_GREATER_THAN_CURRENT_TIME 是自定义验证标签，用于验证时间字段是否大于当前时间。
	// 该标签在结构体字段的 validate 标签中使用，例如：`validate:"gtCurrentTime"`
	// 验证规则：检查时间字段的值是否在当前时间之后（未来时间）
	TAG_GREATER_THAN_CURRENT_TIME = "gtCurrentTime"

	// TAG_LESSER_THAN_CURRENT_TIME 是自定义验证标签，用于验证时间字段是否小于当前时间。
	// 该标签在结构体字段的 validate 标签中使用，例如：`validate:"ltCurrentTime"`
	// 验证规则：检查时间字段的值是否在当前时间之前（过去时间）
	TAG_LESSER_THAN_CURRENT_TIME = "ltCurrentTime"
)

// compareWithCurrentTime 是时间比较的核心函数，用于比较字段值与当前时间的关系。
// 该函数支持两种时间类型：time.Time 和 *time.Time（指针类型），
// 通过类型断言获取字段的实际值，然后使用提供的比较函数进行判断。
//
// 参数：
//   - fl: validator.FieldLevel 接口，提供对字段值的访问
//   - comparer: 比较函数，接受两个 time.Time 参数并返回布尔值，定义比较逻辑
//
// 返回值：
//   - bool: 如果字段值满足比较条件则返回 true，否则返回 false
//
// 注意事项：
//   - 如果字段类型不是 time.Time 或 *time.Time，函数返回 false
//   - 当前时间在函数调用时动态获取，使用 time.Now()
//   - 该函数是 validateGtCurrentTime 和 validateLtCurrentTime 的公共实现
func compareWithCurrentTime(fl validator.FieldLevel, comparer func(a, b time.Time) bool) bool {
	// 处理 time.Time 类型的字段值
	if v, ok := fl.Field().Interface().(time.Time); ok {
		currentTime := time.Now()
		return comparer(v, currentTime)
	}

	// 处理 *time.Time 类型的字段值（指针类型）
	if v, ok := fl.Field().Interface().(*time.Time); ok {
		currentTime := time.Now()
		return comparer(*v, currentTime)
	}

	// 如果字段类型不是时间类型，返回验证失败
	return false
}

// validateGtCurrentTime 是 gtCurrentTime 标签的验证函数，用于验证时间字段是否大于当前时间。
// 该函数检查字段值是否在当前时间之后，即是否为未来时间。
//
// 使用场景：
//   - 验证预约时间、截止时间等未来时间字段
//   - 确保时间字段不会早于当前时间
//
// 参数：
//   - fl: validator.FieldLevel 接口，提供对字段值的访问
//
// 返回值：
//   - bool: 如果字段值在当前时间之后返回 true，否则返回 false
//
// 示例：
//
//	type Event struct {
//	    StartTime time.Time `validate:"gtCurrentTime"` // 必须晚于当前时间
//	}
func validateGtCurrentTime(fl validator.FieldLevel) bool {
	return compareWithCurrentTime(fl, func(a, b time.Time) bool {
		return a.After(b)
	})
}

// validateLtCurrentTime 是 ltCurrentTime 标签的验证函数，用于验证时间字段是否小于当前时间。
// 该函数检查字段值是否在当前时间之前，即是否为过去时间。
//
// 使用场景：
//   - 验证出生日期、历史事件时间等过去时间字段
//   - 确保时间字段不会晚于当前时间
//
// 参数：
//   - fl: validator.FieldLevel 接口，提供对字段值的访问
//
// 返回值：
//   - bool: 如果字段值在当前时间之前返回 true，否则返回 false
//
// 示例：
//
//	type User struct {
//	    BirthDate time.Time `validate:"ltCurrentTime"` // 必须早于当前时间
//	}
func validateLtCurrentTime(fl validator.FieldLevel) bool {
	return compareWithCurrentTime(fl, func(a, b time.Time) bool {
		return b.After(a)
	})
}
